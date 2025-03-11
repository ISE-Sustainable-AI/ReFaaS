package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/ollama/ollama/api"
	log "github.com/sirupsen/logrus"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
)

func (cc *CodeConverter) queryLLM(ctx context.Context, code *DeploymentPackage) (string, Metrics, error) {
	var buf bytes.Buffer
	err := cc.PromptTemplate.Execute(&buf, map[string]interface{}{
		"code": code.RootFile,
	})
	if err != nil {
		return "", Metrics{}, err
	}

	response, metrics, err := cc.invokeLLM(ctx, buf)
	if err != nil {
		return "", metrics, err
	}

	return response.Response, metrics, nil
}

var llmOutputSchema = json.RawMessage(`{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": {
    "type": "string"
  }
}`)

func (cc *CodeConverter) invokeLLM(ctx context.Context, buf bytes.Buffer) (api.GenerateResponse, Metrics, error) {
	var metrics = Metrics{}
	steam := new(bool)
	req := api.GenerateRequest{
		Model:  cc.Model,
		Prompt: buf.String(),
		Stream: steam,
		Options: map[string]interface{}{
			"max_tokens":  2 << 13,
			"temperature": 0.90,
			"response_format": map[string]interface{}{
				"type": "json_object",
			},
		},
		Format: llmOutputSchema,
	}

	callback := make(chan api.GenerateResponse)
	go func() {
		err := cc.client.Generate(ctx, &req, func(gr api.GenerateResponse) error {
			callback <- gr
			return nil
		})
		if err != nil {
			callback <- api.GenerateResponse{
				DoneReason: err.Error(),
			}
		}
	}()

	response := <-callback

	metrics.ConversionTime = response.TotalDuration
	metrics.ConversionPromptTime = response.PromptEvalDuration
	metrics.ConversionEvalTime = response.EvalDuration
	metrics.ConversionPromptTokenCount = response.PromptEvalCount
	metrics.ConversionEvalTokenCount = response.EvalCount

	if response.Response == "" {
		return api.GenerateResponse{}, metrics, fmt.Errorf("response is empty - %s", response.DoneReason)
	}

	return response, metrics, nil
}

func CodeBlockReader(filecontents string) map[string]string {
	file := strings.NewReader(filecontents)
	scanner := bufio.NewScanner(file)
	codeBlockRegex := regexp.MustCompile("^```")
	labelRegex := regexp.MustCompile(`^#### (.+)$`)
	insideCodeBlock := false
	var label string
	var codeContent string
	codeBlocks := make(map[string]string)

	for scanner.Scan() {
		line := scanner.Text()
		if labelMatch := labelRegex.FindStringSubmatch(line); labelMatch != nil {
			if insideCodeBlock && label != "" {
				codeBlocks[label] = codeContent
				codeContent = ""
			}
			label = labelMatch[1]
		} else if codeBlockRegex.MatchString(line) {
			if insideCodeBlock {
				if label != "" {
					codeBlocks[label] = codeContent
				} else {
					codeBlocks["main.go"] = codeContent
				}
				codeContent = ""
				insideCodeBlock = false
			} else {
				insideCodeBlock = true
			}
		} else if insideCodeBlock {
			codeContent += line + "\n"
		}
	}

	return codeBlocks
}

func (cc *CodeConverter) makeDeploymentPackageFromLLMResponse(response string, original *DeploymentPackage) (*DeploymentPackage, error) {
	if response == "" {
		return nil, fmt.Errorf("response is empty")
	}

	files := JsonCodeBlockReader(response)
	log.Debugf("found %d files", len(files))
	dp := DeploymentPackage{
		Metrics: original.Metrics, // copy metrics
	}
	if root_file, ok := files["main.go"]; ok {
		root_file, err := PrepareRootFile(root_file)
		if err != nil {
			return nil, fmt.Errorf("failed to prepare root file: %w", err)
		}
		dp.RootFile = root_file
		delete(files, "main.go")
	} else {
		return nil, fmt.Errorf("main.go not found in response")
	}
	dp.BuildFiles = files
	dp.BuildCmd = []string{"go mod tidy", "go build -o fn ."}
	if _, ok := files["go.mod"]; !ok {
		dp.BuildCmd = append([]string{"go mod init example.com"}, dp.BuildCmd...)
	}

	dp.TestFiles = original.TestFiles

	return &dp, nil
}

func JsonCodeBlockReader(response string) map[string]string {
	var content map[string]string
	_ = json.Unmarshal([]byte(response), &content)
	return content
}

func PrepareRootFile(file string) (string, error) {
	if file == "" {
		return "", fmt.Errorf("file is empty")
	}

	if containsMainMethod(file) {
		log.Debugf("file %s contains main method", file)
		return removeMainMethod(file), nil
	} else {
		return file, nil
	}
}

func removeMainMethod(content string) string {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "main.go", content, parser.AllErrors)
	if err != nil {
		log.Debugf("failed to parse main.go content: %v", err)
		return content
	}
	var remove_lambda_import = false
	var output strings.Builder
	for _, decl := range node.Decls {
		if funcDecl, ok := decl.(*ast.FuncDecl); ok {
			if funcDecl.Name.Name == "main" {
				//XXX: fixing a typical mistake but in a somewhat crude way. A better approach would be repromting...
				main_contains_Lambda_api := strings.Contains(content[funcDecl.Pos()-1:funcDecl.End()], "lambda")
				lambda_api_usages := strings.Count(content[:funcDecl.Pos()-1], "lambda.") + strings.Count(content[funcDecl.End():], "lambda.")
				if main_contains_Lambda_api && lambda_api_usages <= 1 {
					//We need to remove the import.
					remove_lambda_import = true
				}
				continue // Skip the main function
			}
		}
		output.WriteString(content[decl.Pos()-1 : decl.End()]) // Append remaining code
	}
	if remove_lambda_import {
		removed_import := strings.Replace(output.String(), "\"github.com/aws/aws-lambda-go/lambda\"", "", 1)
		return removed_import
	}
	return output.String()
}

func containsMainMethod(content string) bool {
	mainMethodRegex := regexp.MustCompile(`func main\(\)`) // Regex to check for main function
	return mainMethodRegex.MatchString(content)
}

func (cc *CodeConverter) repromptOnBuildError(ctx context.Context, code *DeploymentPackage, out string) (*DeploymentPackage, Metrics, error) {
	codeBlock := codeblockGenerator(code)

	var buf bytes.Buffer
	err := cc.ReportTemplate.Execute(&buf, map[string]interface{}{
		"code":  codeBlock.String(),
		"error": out,
	})
	if err != nil {
		return nil, Metrics{}, err
	}

	response, metrics, err := cc.invokeLLM(ctx, buf)
	if err != nil {
		return nil, metrics, err
	}

	cc.logLLMResponse(code.RootFile, response.Response)
	code.Metrics.AddMetric(metrics)

	//TODO: remove metrics side-effect.
	new_code, err := cc.makeDeploymentPackageFromLLMResponse(response.Response, code)
	if err != nil {
		return nil, *code.Metrics, err
	}

	return new_code, *new_code.Metrics, nil
}

func (cc *CodeConverter) cleanup(ctx context.Context, code *DeploymentPackage) (*DeploymentPackage, error) {
	codeBlock := codeblockGenerator(code)
	var buf bytes.Buffer
	err := cc.CleanupTemplate.Execute(&buf, map[string]interface{}{
		"code": codeBlock.String(),
	})
	if err != nil {
		return code, err
	}

	response, metrics, err := cc.invokeLLM(ctx, buf)
	if err != nil {
		code.Metrics.AddMetric(metrics)
		return code, err
	}
	cc.logLLMResponse(code.RootFile, response.Response)
	//TODO: remove metrics side-effect.
	new_code, err := cc.makeDeploymentPackageFromLLMResponse(response.Response, code)
	if err != nil {
		return code, err
	}

	return new_code, nil
}

func codeblockGenerator(code *DeploymentPackage) strings.Builder {
	var codeBlock strings.Builder

	codeBlock.WriteString(fmt.Sprintf("#### main.go\n"))
	codeBlock.WriteString(code.RootFile)
	codeBlock.WriteString("\n\n")
	for _, fname := range code.BuildFiles {
		codeBlock.WriteString(fmt.Sprintf("#### %s\n", fname))
		codeBlock.WriteString(code.BuildFiles[fname])
		codeBlock.WriteString("\n\n")
	}
	return codeBlock
}

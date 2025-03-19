package main

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
)

type GoLLMDeploymentReader struct {
}

func (gr GoLLMDeploymentReader) makeDeploymentFile(response string, original *DeploymentPackage) (*DeploymentPackage, error) {
	if response == "" {
		return nil, fmt.Errorf("response is empty")
	}
	if original == nil {
		return nil, fmt.Errorf("original is empty")
	}

	files := JsonCodeBlockReader(response)
	log.Debugf("found %d files", len(files))
	dp := DeploymentPackage{}
	if root_file, ok := files["main.go"]; ok {
		root_file, err := gr.prepareGoRootFile(root_file)
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
	dp.Suffix = "go"
	dp.TestFiles = original.TestFiles

	return &dp, nil
}

func (gr GoLLMDeploymentReader) prepareGoRootFile(file string) (string, error) {
	if file == "" {
		return "", fmt.Errorf("file is empty")
	}

	if gr.containsGoMainMethod(file) {
		log.Debugf("file %s contains main method", file)
		return gr.removeGoMainMethod(file), nil
	} else {
		return file, nil
	}
}

func getContentByNode(content string, decl ast.Decl) string {
	pos := int(decl.Pos())
	end := int(decl.End())
	if end > len(content) {
		end = len(content)
	}

	return content[pos-1 : end]

}

func (gr GoLLMDeploymentReader) removeGoMainMethod(content string) string {
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
				main_contains_Lambda_api := strings.Contains(getContentByNode(content, decl), "lambda")
				lambda_api_usages := strings.Count(content[:funcDecl.Pos()-1], "lambda.") + strings.Count(content[funcDecl.End()-1:], "lambda.")
				if main_contains_Lambda_api && lambda_api_usages <= 1 {
					//We need to remove the import.
					remove_lambda_import = true
				}
				continue // Skip the main function
			}
		}
		output.WriteString(getContentByNode(content, decl)) // Append remaining code
	}
	if remove_lambda_import {
		removed_import := strings.Replace(output.String(), "\"github.com/aws/aws-lambda-go/lambda\"", "", 1)
		return removed_import
	}
	return output.String()
}

func (gr GoLLMDeploymentReader) containsGoMainMethod(content string) bool {
	mainMethodRegex := regexp.MustCompile(`func main\(\)`) // Regex to check for main function
	return mainMethodRegex.MatchString(content)
}

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/ollama/ollama/api"
	log "github.com/sirupsen/logrus"
	"iter"
	"maps"
	"os"
	"strings"
	"text/template"
	"time"
)

type LLMPackageReader interface {
	makeDeploymentFile(rawLLMResponse string, original *DeploymentPackage) (*DeploymentPackage, error)
}

type LLMConverter struct {
	template       *template.Template
	ModelName      string                 `json:"model_name"`
	RequestOptions map[string]interface{} `json:"request_options"`
	reader         LLMPackageReader
}

func ReaderFactory(name string) LLMPackageReader {
	switch name {
	case "go":
		return GoLLMDeploymentReader{}
	}
	return BasicLLMDeploymentReader{}
}

func makeLLMConverter(args map[string]interface{}) Converter {
	model, ok := args["model_name"].(string)
	if !ok {
		log.Fatal("model_name must be a string")
		return nil
	}

	prompt, ok := args["prompt"].(string)
	if !ok {
		log.Fatal("prompt must be a string")
		return nil
	}

	prompt_tmpl, err := template.New("prompt").Parse(prompt)
	if err != nil {
		log.Fatalf("Failed to parse prompt template: %s", err)
		return nil
	}

	var reader LLMPackageReader
	if readerName, ok := args["reader"].(string); ok {
		reader = ReaderFactory(readerName)
	} else {
		reader = BasicLLMDeploymentReader{}
	}

	delete(args, "model_name")
	delete(args, "prompt")
	delete(args, "reader")

	defaultParams := map[string]interface{}{
		"max_tokens": 2 << 14,
		//"temperature": 1.0,
		//"top_k":       64,
		//"top_p":       0.95,
		//"min_p":       0.0,
		"response_format": map[string]interface{}{
			"type": "json_object",
		},
	}
	maps.Insert(args, maps.All(defaultParams))

	return &LLMConverter{
		template:       prompt_tmpl,
		ModelName:      model,
		RequestOptions: args,
		reader:         reader,
	}
}

func (cc *LLMConverter) Apply(runner *PipelineRunner, code *ConversionRequest) error {
	var buf bytes.Buffer

	codeBlock := codeBlockGenerator(code.WorkingPackage)

	next, stop := iter.Pull2(code.SourcePackage.getTestFiles())
	result, err, valid := next()
	stop()
	if err != nil || !valid {
		result = &TestFile{}
	}
	err = cc.template.Execute(&buf, map[string]interface{}{
		"code":     codeBlock.String(),
		"issue":    fmt.Sprintf("%v", code.err),
		"original": code.SourcePackage.RootFile,
		"input":    result.Input,
		"output":   result.Output,
	})
	if err != nil {
		code.err = err
		return err
	}

	response, metrics, err := cc.invokeLLM(runner, buf)
	code.Metrics.AddMetric(metrics)
	if err != nil {
		return err
	}

	cc.logLLMResponse(code.SourcePackage.RootFile, response.Response, buf.String())
	newPackage, err := cc.reader.makeDeploymentFile(response.Response, code.WorkingPackage)
	code.WorkingPackage = newPackage

	if err != nil {
		code.err = LLMError{err}
		return code.err
	}

	return nil
}

func (cc *LLMConverter) logLLMResponse(srcPkgCode, raw_response, query string) {
	fhash := []byte(srcPkgCode)
	fname := fmt.Sprintf("chatlogs/%s_%8x_%d.log", cc.ModelName, sha256.Sum256(fhash), time.Now().UnixMicro())
	logf, err := os.OpenFile(fname,
		os.O_CREATE|os.O_RDWR, 0644)
	defer logf.Close()
	var written int
	if err == nil {
		logf.WriteString("# Query\n\n")
		logf.WriteString(query)
		logf.WriteString("\n\n# Response\n\n```\n")
		written, _ = logf.WriteString(raw_response)
		logf.WriteString("\n```\n")
	}
	log.Debugf("logged llm response to: %s with %d bytes", fname, written)
}

var llmOutputSchema = json.RawMessage(`{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": {
    "type": "string"
  }
}`)

func (cc *LLMConverter) invokeLLM(runner *PipelineRunner, buf bytes.Buffer) (api.GenerateResponse, Metrics, error) {
	var metrics = Metrics{}
	steam := new(bool)
	req := api.GenerateRequest{
		Model:   cc.ModelName,
		Prompt:  buf.String(),
		Stream:  steam,
		Options: cc.RequestOptions,
		Format:  llmOutputSchema,
	}

	callback := make(chan api.GenerateResponse)
	go func() {
		err := runner.client.Generate(runner, &req, func(gr api.GenerateResponse) error {
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

func JsonCodeBlockReader(response string) map[string]string {
	var content map[string]string
	_ = json.Unmarshal([]byte(response), &content)
	return content
}

func codeBlockGenerator(code *DeploymentPackage) strings.Builder {
	var codeBlock strings.Builder
	if code == nil {
		return codeBlock
	}
	codeBlock.WriteString(fmt.Sprintf("#### main.%s\n```go\n", code.Suffix))
	codeBlock.WriteString(code.RootFile)
	codeBlock.WriteString("```\n\n")
	for _, fname := range code.BuildFiles {
		codeBlock.WriteString(fmt.Sprintf("\n#### %s\n```go\n", fname))
		codeBlock.WriteString(code.BuildFiles[fname])
		codeBlock.WriteString("```\n\n")
	}
	return codeBlock
}

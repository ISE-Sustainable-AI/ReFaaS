package main

import (
	"bytes"
	"fmt"
	log "github.com/sirupsen/logrus"
	"iter"
	"strings"
	"text/template"
)

type LLMPackageReader interface {
	makeDeploymentFile(rawLLMResponse string, original *DeploymentPackage) (*DeploymentPackage, error)
}

type LLMConverter struct {
	template *template.Template
	reader   LLMPackageReader
	args     map[string]interface{}
}

func ReaderFactory(name string) LLMPackageReader {
	switch name {
	case "go":
		return GoJsonOllamaReader{}
	case "deepseek":
		return GoDeepSeekOllamaReader{}
	}
	return BasicLLMDeploymentReader{}
}

func makeLLMConverter(args map[string]interface{}) Converter {

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

	delete(args, "prompt")
	delete(args, "reader")

	log.Debugf("creating LLM converter with params: %v", args)
	return &LLMConverter{
		template: prompt_tmpl,
		reader:   reader,
		args:     args,
	}
}

func (cc *LLMConverter) Apply(runner *PipelineRunner, code *ConversionRequest) error {
	var codePrompt bytes.Buffer

	codeBlock := codeBlockGenerator(code.WorkingPackage)
	result := getFirstTestFile(code)

	srcFile := ""
	if code.SourcePackage != nil {
		srcFile = code.SourcePackage.RootFile
	}
	errStr := ""
	if code.err != nil && len(code.err) > 0 {
		errStr = code.err[len(code.err)-1].Error()
	}

	err := cc.template.Execute(&codePrompt, map[string]interface{}{
		"code":     codeBlock.String(),
		"issue":    errStr,
		"original": srcFile,
		"input":    result.Input,
		"output":   result.Output,
	})
	if err != nil {
		code.err = append(code.err, err)
		return err
	}

	err = runner.client.Prepare(cc.args)
	if err != nil {
		return LLMError{fmt.Errorf("failed to configure LLMClient: %+v", err)}
	}
	//XXX: interface entry point ...
	response, metrics, err := runner.client.InvokeLLM(runner, codePrompt)
	code.Metrics.AddMetric(metrics)
	if err != nil {
		return err
	}

	runner.client.logLLMResponse(srcFile, response, codePrompt.String())
	original := code.WorkingPackage
	if original == nil {
		original = code.SourcePackage
	}
	newPackage, err := cc.reader.makeDeploymentFile(response, original)
	code.WorkingPackage = newPackage

	if err != nil {
		code.err = append(code.err, LLMError{err})
		return err
	}

	return nil
}

func getFirstTestFile(code *ConversionRequest) *TestFile {
	next, stop := iter.Pull2(code.SourcePackage.getTestFiles())
	result, err, valid := next()
	stop()
	if err != nil || !valid {
		result = &TestFile{}
	}
	return result
}

func codeBlockGenerator(code *DeploymentPackage) strings.Builder {
	var codeBlock strings.Builder
	if code == nil {
		return codeBlock
	}
	codeBlock.WriteString(fmt.Sprintf("#### main.%s\n```go\n", code.Suffix))
	codeBlock.WriteString(code.RootFile)
	codeBlock.WriteString("\n```\n\n")
	for _, fname := range code.BuildFiles {
		codeBlock.WriteString(fmt.Sprintf("\n#### %s\n```go\n", fname))
		codeBlock.WriteString(code.BuildFiles[fname])
		codeBlock.WriteString("\n```\n\n")
	}
	return codeBlock
}

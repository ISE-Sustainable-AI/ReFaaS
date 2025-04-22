package main

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"testing"
	"text/template"
)

func TestGemini(t *testing.T) {
	cc, err := MakeCodeConverter(&ConverterOptions{})

	srcDep, err := cc.ReadDeploymentPackageFromFile("test/f5.zip")
	assert.NoError(t, err)
	assert.NotNil(t, srcDep)

	req := MakeConversionRequest(srcDep)

	template, err := template.New("prompt").Parse(defaultPrompt)
	var codePrompt bytes.Buffer

	codeBlock := codeBlockGenerator(req.SourcePackage)
	result := getFirstTestFile(req)
	err = template.Execute(&codePrompt, map[string]interface{}{
		"code":     codeBlock.String(),
		"issue":    "",
		"original": "",
		"input":    result.Input,
		"output":   result.Output,
	})
	assert.NoError(t, err)

	gic := &GeminiInvocationClient{}

	//XXX remove key
	gic.Configure(map[string]interface{}{
		"GEMINI_API_KEY": "AIzaSyBA5DW-Dzo01DGlLwp1J0AbLQ2R-10ThY4",
		"GEMINI_MODEL":   "gemini-1.5-flash-8b",
	})

	gic.Prepare(nil)

	response, metrics, err := gic.InvokeLLM(t.Context(), codePrompt)
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.NotNil(t, metrics)

	reader := GoJsonOllamaReader{}
	out, err := reader.makeDeploymentFile(response, req.SourcePackage)
	if err != nil {
		t.Errorf("error making deployment file: %v", err)
	}

	assert.NotEmpty(t, out)
	t.Logf("output: %s", out)
}

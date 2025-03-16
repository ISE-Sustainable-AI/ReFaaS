package main

import (
	"bytes"
	"context"
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"io"
	"os"
	"path"
	"testing"
	"time"
)

func TestPipelineReader(t *testing.T) {
	reader := bytes.NewReader([]byte(defaultPipelineFile))
	pipeline, err := PipelineReader(reader)
	assert.NoError(t, err)
	assert.NotNil(t, pipeline)

	assert.IsTypef(t, pipeline.FirstTask.Execute, &LLMConverter{}, "FirstTask should be a valid type")
}

func TestFullConversion(t *testing.T) {
	reader := bytes.NewReader([]byte(defaultPipelineFile))
	pipeline, err := PipelineReader(reader)
	assert.NoError(t, err)
	assert.NotNil(t, pipeline)

	log.SetLevel(log.DebugLevel)
	cc, err := MakeCodeConverter(&ConverterOptions{
		OLLAMA_API_URL,
	}, pipeline)

	assert.Nil(t, err)

	req, err := cc.ConvertFromFileBest("test/f3.zip")
	assert.NoError(t, err)
	assert.NotNil(t, req)
	if req != nil {
		assert.Greater(t, len(req.Metrics.TestCases), 0)
		assert.Greater(t, req.Metrics.BuildTime, time.Duration(0))
		assert.Greater(t, req.Metrics.TestTime, time.Duration(0))

		t.Logf("%v", req.Metrics)
	} else {
		t.FailNow()
	}

}

func TestLogCase(t *testing.T) {
	entries, err := os.ReadDir("chatlogs")
	if err != nil {
		t.Fatal(err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			testCompileByResponseLog(t, path.Join("chatlogs", entry.Name()))
		})
	}
}

func testCompileByResponseLog(t *testing.T, test_case string) {
	tc, err := os.OpenFile(test_case, os.O_RDONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}

	defer tc.Close()
	test_bytes, err := io.ReadAll(tc)
	if err != nil {
		t.Fatal(err)
	}

	mk := make(map[string]string)
	if body, err := read("test/f1.json"); err == nil {
		mk["f1"] = body
	} else {
		t.Fatal(fmt.Errorf("failed to read %+v", err))
	}

	if body, err := read("test/f2.json"); err == nil {
		mk["f2"] = body
	} else {
		t.Fatal(fmt.Errorf("failed to read %+v", err))
	}

	original := DeploymentPackage{
		TestFiles:  mk,
		BuildFiles: make(map[string]string),
		BuildCmd:   make([]string, 0),
	}

	reader := GoLLMDeploymentReader{}

	dp, err := reader.makeDeploymentFile(string(test_bytes), &original)
	if err != nil {
		t.Fatal(err)
	}
	assert.NotNil(t, dp)

	task := ConversionTask{
		ID:            "test",
		Execute:       makeGolangBuilder(nil),
		CanApply:      nil,
		RetryCount:    0,
		MaxRetryCount: 1,
		RetryDelay:    0,
		Next:          nil,
		OnFailure:     nil,
		Validation: makeGoPackageTester(map[string]interface{}{
			"strategy": "json",
		}),
	}
	pipeline := NewPipeline(&task)
	runner := &PipelineRunner{
		Context:  context.Background(),
		pipeline: nil,
		client:   nil,
	}

	req := MakeConversionRequest(dp)
	req.WorkingPackage = req.SourcePackage

	err = pipeline.Execute(runner, req)
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, req.err)
}

func read(fname string) (string, error) {
	tc, err := os.OpenFile(fname, os.O_RDONLY, 0644)
	if err != nil {
		return "", err
	}

	defer tc.Close()
	test_bytes, err := io.ReadAll(tc)
	if err != nil {
		return "", err
	}
	return string(test_bytes), nil
}

func TestMakeAwareSimilarityValidation(t *testing.T) {
	jsonValidator := MakeAwareSimilarityValidation(0.8)

	result := jsonValidator.validate("{\"response\":{\"statusCode\":200,\"headers\":null,\"multiValueHeaders\":null,\"body\":\"{\\\"result\\\":20}\"}}",
		"{\"statusCode\":200,\"headers\":null,\"multiValueHeaders\":null,\"body\":{\"result\":20}}")

	assert.True(t, result)
}

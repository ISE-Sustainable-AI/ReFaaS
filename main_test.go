package main

import (
	"archive/zip"
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

var testOpts = ConverterOptions{
	OLLAMA_API_URL: "http://localhost:11434",
	//OLLAMA_API_URL:         "http://swkgpu1.informatik.uni-hamburg.de:11434",
	Model_Name:             "qwen2.5-coder:14b",
	ConversionRetries:      2,
	BuildAttempts:          2,
	EnableBuildRePrompting: true,
}

func TestFullConversion(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	cc, err := MakeCodeConverter(&testOpts)
	assert.Nil(t, err)
	var ctx context.Context
	var cancel context.CancelFunc
	if deadline, ok := t.Deadline(); ok {
		ctx, cancel = context.WithDeadline(context.Background(), deadline)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	defer cancel()

	dp, err := cc.ConvertFromFileBestN(cc.retries, ctx, "test/f5.zip")
	if err != nil {
		t.Fatal(err)
	}
	assert.NotNil(t, dp)
}

func TestModels(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	models := []string{
		"qwen2.5:32b",
		"qwen2.5-coder:32b",
		//"deepseek-r1:8b",
		//"deepseek-coder-v2:latest",
		"codellama:34b",
		"codestral:latest",
	}

	var ctx context.Context
	var cancel context.CancelFunc
	if deadline, ok := t.Deadline(); ok {
		ctx, cancel = context.WithDeadline(context.Background(), deadline)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	defer cancel()

	results := make(map[string]string)
	for _, model := range models {
		testOpts = ConverterOptions{
			OLLAMA_API_URL:         "http://swkgpu1.informatik.uni-hamburg.de:11434",
			Model_Name:             model,
			ConversionRetries:      1,
			EnableBuildRePrompting: true,
			BuildAttempts:          2,
		}
		cc, err := MakeCodeConverter(&testOpts)
		assert.Nil(t, err)

		start := time.Now()
		dp, err := cc.ConvertFromFileBestN(cc.retries, ctx, "test/example.zip")
		duration := time.Since(start)
		if err != nil {
			results[model] = fmt.Sprintf("failed to convert, %d - %+v", duration, err)
		} else {
			results[model] = fmt.Sprintf("successfully converted, %d - %+v", duration, dp.Metrics)
		}
	}

	fs, err := os.OpenFile("ModelTestReport.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	for key, result := range results {
		fs.WriteString(fmt.Sprintf("%s;%s\n\n", key, result))
	}
	fs.Close()
}

func TestFullConversionWithRePrompting(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	ops := testOpts
	ops.EnableBuildRePrompting = true
	ops.BuildAttempts = 3
	ops.ConversionRetries = 2

	cc, err := MakeCodeConverter(&ops)
	assert.Nil(t, err)
	var ctx context.Context
	var cancel context.CancelFunc
	if deadline, ok := t.Deadline(); ok {
		ctx, cancel = context.WithDeadline(context.Background(), deadline)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	defer cancel()

	dp, err := cc.ConvertFromFileBestN(cc.retries, ctx, "test/example.zip")
	if err != nil {
		t.Fatal(err)
	}
	assert.NotNil(t, dp)
	assert.NotNil(t, dp.Metrics)

	log.Debugf("%+v", dp.Metrics)
}

func TestStability(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	for i := 0; i < 10; i++ {
		TestFullConversion(t)
	}
}

func TestCodeConverter_queryLLM(t *testing.T) {
	cc, err := MakeCodeConverter(&testOpts)
	if err != nil {
		t.Fatal(err)
	}
	var ctx context.Context
	var cancel context.CancelFunc
	if deadline, ok := t.Deadline(); ok {
		ctx, cancel = context.WithDeadline(context.Background(), deadline)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}

	testCode, err := read("test/test.py")
	if err != nil {
		t.Fatal(err)
	}

	defer cancel()
	result, _, err := cc.queryLLM(ctx, &DeploymentPackage{
		RootFile: testCode,
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Log(result)

}

func TestCodeParsing(t *testing.T) {
	//open text_case.txt
	var cases = []string{"test/test_case1.txt", "test/test_case2.txt", "test/test_case3.txt"}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			testByResponseLog(t, c)
		})
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

func testByResponseLog(t *testing.T, casefile string) {
	test_case, err := read(casefile)
	if err != nil {
		t.Fatal(err)
	}

	cc, err := MakeCodeConverter(nil)
	if err != nil {
		t.Fatal(err)
	}

	original := DeploymentPackage{
		TestFiles: make(map[string]string),
		Metrics: &Metrics{
			TestCases: make(map[string]bool),
		},
	}

	dp, err := cc.makeDeploymentPackageFromLLMResponse(test_case, &original)
	if err != nil {
		t.Fatalf("failed to make dp for %s, due %+v", casefile, err)
	}
	assert.NotNil(t, dp)
	assert.NotEmpty(t, dp.RootFile)
}

func TestCompile(t *testing.T) {
	//open text_case.txt
	var cases = []string{"test/test_case1.txt", "test/test_case2.txt", "test/test_case3.txt"}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			testCompileByResponseLog(t, c)
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
	cc, err := MakeCodeConverter(&testOpts)
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
		TestFiles: mk,
		Metrics: &Metrics{
			TestCases: make(map[string]bool),
		},
	}

	dp, err := cc.makeDeploymentPackageFromLLMResponse(string(test_bytes), &original)
	if err != nil {
		t.Fatal(err)
	}
	assert.NotNil(t, dp)
	var ctx context.Context
	var cancel context.CancelFunc
	if deadline, ok := t.Deadline(); ok {
		ctx, cancel = context.WithDeadline(context.Background(), deadline)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	defer cancel()
	success, err := cc.runTest(ctx, dp)
	if err != nil {
		t.Fatal(err)
	}

	assert.True(t, success)
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
		"{\"response\":{\"statusCode\":200,\"headers\":null,\"multiValueHeaders\":null,\"body\":{\"result\":20}}}")

	assert.True(t, result)
}

func TestPackageing(t *testing.T) {
	cc, err := MakeCodeConverter(nil)
	assert.Nil(t, err)

	dp, err := cc.ReadDeploymentPackageFromFile("test/example.zip")
	assert.Nil(t, err)
	assert.NotNil(t, dp)

	dp.BuildFiles = map[string]string{"go.mod": "TEST"}
	dp.BuildCmd = []string{"go mod init", "go build ."}

	var buf bytes.Buffer
	err = cc.WriteDeploymentPackage(&buf, dp)
	assert.Nil(t, err)

	reader := bytes.NewReader(buf.Bytes())
	zipfs, err := zip.NewReader(reader, reader.Size())
	assert.Nil(t, err)

	ndp := make(map[string]string)

	for _, file := range zipfs.File {
		if file.FileInfo().IsDir() {
			continue
		}
		r, err := file.Open()
		assert.Nil(t, err)
		data, err := io.ReadAll(r)
		assert.Nil(t, err)
		ndp[file.Name] = string(data)
		_ = r.Close()
	}

	assert.Contains(t, ndp, "main.go")
	assert.Contains(t, ndp, "build.sh")
	assert.Contains(t, ndp, "go.mod")
	assert.Contains(t, ndp, "test/f1.json")
	assert.Contains(t, ndp, "test/f2.json")

	assert.NotEmpty(t, ndp["main.go"], "main.go was empty")
	assert.NotEmpty(t, ndp["build.sh"], "build.sh was empty")
	assert.NotEmpty(t, ndp["go.mod"], "go.mod was empty")
	assert.NotEmpty(t, ndp["test/f1.json"], "test/f1.json was empty")
	assert.NotEmpty(t, ndp["test/f2.json"], "test/f2.json was empty")
}

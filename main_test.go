package main

import (
	"context"
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"io"
	"os"
	"testing"
)

var testOpts = ConverterOptions{
	OLLAMA_API_URL: "http://swkgpu1.informatik.uni-hamburg.de:11434",
	//Model_Name:        "deepseek-r1:1.5B",
	ConversionRetries: 2,
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

	dp, err := cc.ConvertFromFileBestN(cc.retries, ctx, "test/example.zip")
	if err != nil {
		t.Fatal(err)
	}
	assert.NotNil(t, dp)
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
	cc, err := MakeCodeConverter(nil)
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
	cc, err := MakeCodeConverter(nil)
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

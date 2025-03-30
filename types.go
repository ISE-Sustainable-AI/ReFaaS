package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"iter"
	"maps"
	"time"
)

type Converter interface {
	Apply(*PipelineRunner, *ConversionRequest) error
}

// ConversionTask represents a step in the pipeline
type ConversionTask struct {
	ID            string
	Execute       Converter         // Task execution function
	CanApply      Converter         // Checks if the preconditions are met to run this task, otherwise the pipeline will fail
	RetryCount    int               // RetryAttempts
	MaxRetryCount int               // Max retries
	RetryDelay    time.Duration     // Delay between retries
	Next          []*ConversionTask // Next tasks (normal execution flow)
	OnFailure     *ConversionTask   // Recovery task if this task fails
	Validation    Converter
}

type ConverterFactory func(map[string]interface{}) Converter

var ConverterFactories map[string]ConverterFactory = map[string]ConverterFactory{
	"goBuilder":  makeGolangBuilder,
	"goTester":   makeGoPackageTester,
	"llmTask":    makeLLMConverter,
	"cleaner":    makeCleanupConverter,
	"coder":      makeCodeConverter,
	"fixer":      makeRePromptConverter,
	"realign":    makeAlignmentConverter,
	"noop":       makeNoopConverter,
	"canCompile": makeCompilePrecheckConverter,
}

// Pipeline represents the workflow pipeline
type Pipeline struct {
	FirstTask *ConversionTask
}

type LLMInvocationClient interface {
	//Configures the client to serve multiple invocations, e.g., setting up a conncetion pool
	Configure(args map[string]interface{}) error
	//Prepares a request to the client, e.g., changing invocation parameters. WARNNING, could create raise conditions.
	Prepare(map[string]interface{}) error
	//InvokeLLM takes the given prompt and invokes the llm
	InvokeLLM(ctx context.Context, buf bytes.Buffer) (string, Metrics, error)
	//logs details about a llm invocation to a file and console
	logLLMResponse(...string)
}

type LLMFactory func(map[string]interface{}) (LLMInvocationClient, error)

var LLMClientFactories map[string]LLMFactory = map[string]LLMFactory{
	"ollama": func(args map[string]interface{}) (LLMInvocationClient, error) {
		oc := &OllamaInvocationClient{}
		err := oc.Configure(args)
		if err != nil {
			return nil, err
		}
		return oc, nil
	},
	"deepseek": func(args map[string]interface{}) (LLMInvocationClient, error) {
		dc := &DeepSeekInvocationClient{}
		err := dc.Configure(args)
		if err != nil {
			return nil, err
		}
		return dc, nil
	},
	"gemini": func(args map[string]interface{}) (LLMInvocationClient, error) {
		gc := &GeminiInvocationClient{}
		err := gc.Configure(args)
		if err != nil {
			return nil, err
		}
		return gc, nil
	},
}

type ConversionRequest struct {
	Id             uuid.UUID          `json:"id,omitempty"`
	SourcePackage  *DeploymentPackage `json:"sourcePackage,omitempty"`
	WorkingPackage *DeploymentPackage `json:"workingPackage,omitempty"`
	Metrics        *Metrics           `json:"metrics,omitempty"`
	err            []error
	Completed      bool `json:"completed,omitempty"`
}

type DeploymentPackage struct {
	RootFile   string
	TestFiles  map[string]string
	BuildFiles map[string]string
	BuildCmd   []string
	Env        []string
	Suffix     string
}

func (dp *DeploymentPackage) getTestFiles() iter.Seq2[*TestFile, error] {
	return func(yield func(*TestFile, error) bool) {
		for name, v := range dp.TestFiles {
			file := &TestFile{}
			err := json.Unmarshal([]byte(v), file)
			file.Name = name
			file.Env = dp.Env
			if !yield(file, err) {
				return
			}
		}
	}
}

func (dp *DeploymentPackage) copy() *DeploymentPackage {
	testCopy := make(map[string]string)
	maps.Copy(testCopy, dp.TestFiles)
	buildFilesCopy := make(map[string]string)
	maps.Copy(buildFilesCopy, dp.BuildFiles)
	cmdCopy := make([]string, len(dp.BuildCmd))
	copy(cmdCopy, dp.BuildCmd)

	return &DeploymentPackage{
		RootFile:   dp.RootFile,
		TestFiles:  testCopy,
		BuildFiles: buildFilesCopy,
		BuildCmd:   cmdCopy,
		Suffix:     dp.Suffix,
		Env:        dp.Env,
	}
}

type Metrics struct {
	StartTime time.Time
	EndTime   time.Time

	TotalTime time.Duration

	ConversionTime       time.Duration `json:"conversion_time"`
	ConversionPromptTime time.Duration `json:"conversion_prompt_time"`
	ConversionEvalTime   time.Duration `json:"conversion_eval_time"`

	ConversionPromptTokenCount int `json:"conversion_prompt_token_count"`
	ConversionEvalTokenCount   int `json:"conversion_eval_token_count"`

	BuildTime time.Duration `json:"build_time"`
	TestTime  time.Duration `json:"test_time"`

	BuildError int `json:"build_error"`
	TestError  int `json:"test_error"`
	Tasks      int `json:"tasks"`

	TestCases map[string]bool `json:"test_cases"`
	Issues    []string        `json:"issues"`
}

func (m *Metrics) AddMetric(mm Metrics) {
	m.TotalTime += mm.TotalTime
	m.ConversionTime += mm.ConversionTime
	m.ConversionPromptTime += mm.ConversionPromptTime
	m.ConversionEvalTime += mm.ConversionEvalTime
	m.ConversionPromptTokenCount += mm.ConversionPromptTokenCount
	m.ConversionEvalTokenCount += mm.ConversionEvalTokenCount
	m.BuildTime += mm.BuildTime
	m.BuildError += mm.BuildError
	m.Tasks += mm.Tasks

	if m.StartTime.After(mm.StartTime) {
		m.StartTime = mm.StartTime
	}

	if m.EndTime.Before(mm.EndTime) {
		m.EndTime = mm.EndTime
	}
}

type TestFile struct {
	Name   string `json:"name"`
	Input  string `json:"input"`
	Output string `json:"output"`
	//ENV variables to override in case of a test
	Env []string `json:"env"`
	//Services to mock/deploy for the test
	Services map[string]string `json:"services"`
	//If this test will produce deterministicResults
	UndeterministicResults bool `json:"deterministic"
`
}

//go:embed prompts/stage-zero.md
var defaultCleanupPrompt string

func makeCleanupConverter(args map[string]interface{}) Converter {
	args["prompt"] = defaultCleanupPrompt
	return makeLLMConverter(args)
}

//go:embed prompts/stage-one.md
var defaultPrompt string

func makeCodeConverter(args map[string]interface{}) Converter {
	args["prompt"] = defaultPrompt
	return makeLLMConverter(args)
}

//go:embed prompts/stage-two.md
var defaultBuildRePrompt string

func makeRePromptConverter(args map[string]interface{}) Converter {
	args["prompt"] = defaultBuildRePrompt
	return makeLLMConverter(args)
}

//go:embed prompts/stage-three.md
var defaultAlignmentPrompt string

func makeAlignmentConverter(args map[string]interface{}) Converter {
	args["prompt"] = defaultAlignmentPrompt
	return makeLLMConverter(args)
}

//go:embed default.yaml
var defaultPipelineFile string

type NoOpConverter struct{}

func (NoOpConverter) Apply(*PipelineRunner, *ConversionRequest) error { return nil }

func makeNoopConverter(args map[string]interface{}) Converter {
	return &NoOpConverter{}
}

type CanCompileConverter struct{}

func (CanCompileConverter) Apply(run *PipelineRunner, req *ConversionRequest) error {
	if req.SourcePackage == nil {
		return fmt.Errorf("no Package source defined")
	}

	if req.WorkingPackage == nil {
		return fmt.Errorf("no Package working directory defined")
	}

	if req.SourcePackage.RootFile == "" {
		return fmt.Errorf("no soruce root file defined")
	}

	if req.WorkingPackage.RootFile == "" {
		return fmt.Errorf("no working root file defined")
	}

	if len(req.SourcePackage.TestFiles) != len(req.WorkingPackage.TestFiles) {
		return fmt.Errorf("number of test files and test files don't match")
	}

	return nil
}

func makeCompilePrecheckConverter(args map[string]interface{}) Converter {
	return &CanCompileConverter{}
}

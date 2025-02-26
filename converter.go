package main

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"fmt"
	"github.com/ollama/ollama/api"
	log "github.com/sirupsen/logrus"
	"net/http"
	"net/url"
	"os"
	"text/template"
	"time"
)

//TODO: implement a compile REL loop with the LLM to improve the code...
//TODO: turn metrics to pointer and remove from dps.

type CodeConverter struct {
	//internals
	client *api.Client

	//config options for the converter
	buildRePrompting bool
	buildAttempts    int
	retries          int

	//expected template fields: code
	PromptTemplate *template.Template
	ReportTemplate *template.Template
	//e.g., deepseek-r1:32b
	Model string
	//see @GenerateRequest
	RequestOptions map[string]interface{}
	SourceSuffix   string
	TestHandler    string
}

//go:embed prompt.md
var defaultPrompt string

//go:embed reprompt.md
var defaultBuildReprompt string

//go:embed test_handler.txt
var testHandler string

type ConverterOptions struct {
	OLLAMA_API_URL         string                 `json:"ollama_url"`
	Model_Name             string                 `json:"model_name"`
	RequestOptions         map[string]interface{} `json:"request_options"`
	SourceSuffix           string                 `json:"source_suffix"`
	PromptTemplate         string                 `json:"prompt_template"`
	RePromptTemplate       string                 `json:"reprompt_template"`
	TestHandler            string                 `json:"test_handler"`
	EnableBuildRePrompting bool                   `json:"enable_build_repromting"`
	BuildAttempts          int                    `json:"build_attempts"` // only valid with build_reprompting enabled
	ConversionRetries      int                    `json:"conversion_retries"`
}

func (co *ConverterOptions) setDefaults() {
	if co.PromptTemplate == "" {
		co.PromptTemplate = DefaultOptions.PromptTemplate
	}

	if co.RePromptTemplate == "" {
		co.RePromptTemplate = DefaultOptions.RePromptTemplate
	}

	if co.Model_Name == "" {
		co.Model_Name = DefaultOptions.Model_Name
	}

	if co.RequestOptions == nil {
		co.RequestOptions = DefaultOptions.RequestOptions
	}

	if co.SourceSuffix == "" {
		co.SourceSuffix = DefaultOptions.SourceSuffix
	}

	if co.TestHandler == "" {
		co.TestHandler = DefaultOptions.TestHandler
	}

	if co.OLLAMA_API_URL == "" {
		co.OLLAMA_API_URL = DefaultOptions.OLLAMA_API_URL
	}
	if co.BuildAttempts <= 0 {
		co.BuildAttempts = DefaultOptions.BuildAttempts
	}

	if co.ConversionRetries <= 0 {
		co.ConversionRetries = DefaultOptions.ConversionRetries
	}
}

var DefaultOptions = ConverterOptions{
	OLLAMA_API_URL:         OLLAMA_API_URL,
	Model_Name:             "deepseek-r1:32b",
	SourceSuffix:           "py",
	RequestOptions:         map[string]interface{}{},
	PromptTemplate:         defaultPrompt,
	RePromptTemplate:       defaultBuildReprompt,
	TestHandler:            testHandler,
	EnableBuildRePrompting: false,
	BuildAttempts:          1,
	ConversionRetries:      3,
}

func MakeCodeConverter(ops *ConverterOptions) (*CodeConverter, error) {
	if ops == nil {
		ops = &DefaultOptions
	} else {
		ops.setDefaults()
	}

	client := http.Client{}
	url, _ := url.Parse(ops.OLLAMA_API_URL)
	api_client := api.NewClient(url, &client)

	prompt_tmpl, err := template.New("prompt").Parse(ops.PromptTemplate)
	if err != nil {
		return nil, err
	}

	reprompt_tmpl, err := template.New("reprompt").Parse(ops.RePromptTemplate)

	if err != nil {
		return nil, err
	}

	return &CodeConverter{
		client:           api_client,
		buildRePrompting: ops.EnableBuildRePrompting,
		buildAttempts:    ops.BuildAttempts,
		retries:          ops.ConversionRetries,
		PromptTemplate:   prompt_tmpl,
		ReportTemplate:   reprompt_tmpl,
		Model:            ops.Model_Name,
		RequestOptions:   ops.RequestOptions,
		SourceSuffix:     ops.SourceSuffix,
		TestHandler:      ops.TestHandler,
	}, nil
}

func (cc *CodeConverter) ConvertBestN(n int, ctx context.Context, srcPkg *DeploymentPackage) (*DeploymentPackage, Metrics, error) {
	pm := &Metrics{
		TestCases: make(map[string]bool),
	}
	errors := make([]error, 0)
	srcPkg.Metrics = pm
	for range n {

		dp, cm, err := cc.convert(ctx, srcPkg)
		if err == nil {
			return dp, *pm, nil
		} else {
			log.Debugf("failed to convert %+v", err)
			errors = append(errors, err)
		}
		pm.AddMetric(cm)
	}
	return nil, *pm, fmt.Errorf("could not convert after %d attempts, due to %+v", n, errors)

}

func (cc *CodeConverter) Convert(ctx context.Context, srcPkg *DeploymentPackage) (*DeploymentPackage, error) {
	code, _, err := cc.ConvertBestN(cc.retries, ctx, srcPkg)
	return code, err
}

func (cc *CodeConverter) convert(ctx context.Context, srcPkg *DeploymentPackage) (*DeploymentPackage, Metrics, error) {
	raw_response, metrics, err := cc.queryLLM(ctx, srcPkg)
	metrics.AddMetric(*srcPkg.Metrics)
	if err != nil {
		return srcPkg, metrics, err
	}

	logLLMResponse(srcPkg.RootFile, raw_response)

	//TODO: remove metrics side-effect.
	code, err := cc.makeDeploymentPackageFromLLMResponse(raw_response, srcPkg)
	if err != nil {
		return nil, metrics, err
	}

	log.Debugf("New deployment package: %+v", code)
	scussess, err := cc.runTest(ctx, code)
	if err != nil {
		return code, *code.Metrics, err
	}
	log.Debugf("Run %d tests successfuly", len(code.TestFiles))

	if scussess {
		return code, *code.Metrics, nil
	} else {
		return nil, *code.Metrics, fmt.Errorf("conversion failed")
	}
}

func logLLMResponse(srcPkgCode, raw_response string) {
	fhash := []byte(srcPkgCode)
	fhash = append(fhash, []byte(time.Now().String())...)

	logf, err := os.OpenFile(fmt.Sprintf("%x.log", sha256.Sum256(fhash)),
		os.O_CREATE|os.O_RDWR, 0644)
	if err == nil {
		logf.WriteString(raw_response)
		logf.Close()
	}
	log.Debugf("llm response: %+v", raw_response)
}

func (cc *CodeConverter) ConvertFromFileBestN(n int, ctx context.Context, sourceFile string) (*DeploymentPackage, error) {
	dp, err := cc.ReadDeploymentPackageFromFile(sourceFile)
	if err != nil {
		return nil, err
	}
	log.Debugf("got deployment package: %s - %+v", sourceFile, dp)

	code, _, err := cc.ConvertBestN(n, ctx, dp)
	return code, err
}

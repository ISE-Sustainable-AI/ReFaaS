package main

import (
	"context"
	"github.com/google/uuid"
	"github.com/ollama/ollama/api"
	log "github.com/sirupsen/logrus"
	"net/http"
	"net/url"
	"os"
)

//TODO: implement a compile REL loop with the LLM to improve the code...
//TODO: turn metrics to pointer and remove from dps.

type PipelineRunner struct {
	context.Context
	//internals
	client *api.Client

	pipeline   *Pipeline
	WorkingDir string
}

type ConverterOptions struct {
	OLLAMA_API_URL string `json:"ollama_url"`
}

func (co *ConverterOptions) setDefaults() {
	if co.OLLAMA_API_URL == "" {
		co.OLLAMA_API_URL = DefaultOptions.OLLAMA_API_URL
	}
}

var DefaultOptions = ConverterOptions{
	OLLAMA_API_URL: OLLAMA_API_URL,
}

func MakeCodeConverter(ops *ConverterOptions, pipeline *Pipeline) (*PipelineRunner, error) {
	if ops == nil {
		ops = &DefaultOptions
	} else {
		ops.setDefaults()
	}

	client := http.Client{}
	url, _ := url.Parse(ops.OLLAMA_API_URL)
	api_client := api.NewClient(url, &client)

	return &PipelineRunner{
		Context:  context.Background(),
		pipeline: pipeline,
		client:   api_client,
	}, nil
}

func MakeConversionRequest(srcPkg *DeploymentPackage) *ConversionRequest {
	return &ConversionRequest{
		Id:            uuid.New(),
		SourcePackage: srcPkg,
		Metrics: &Metrics{
			TestCases: make(map[string]bool),
		},
	}
}

func (cc *PipelineRunner) Convert(req *ConversionRequest) error {
	req.WorkingPackage = req.SourcePackage.copy()

	return cc.pipeline.Execute(cc, req)
}

func (cc *PipelineRunner) Reconfigure(pipeline *Pipeline) {
	if cc.WorkingDir != "" {
		defer os.RemoveAll(cc.WorkingDir)
	}
	cc.WorkingDir = ""
	cc.pipeline = pipeline
}

func (cc *PipelineRunner) ConvertFromFileBest(sourceFile string) (*ConversionRequest, error) {
	dp, err := cc.ReadDeploymentPackageFromFile(sourceFile)
	if err != nil {
		return nil, err
	}
	dp.Suffix = "py"
	log.Debugf("got deployment package: %s - %+v", sourceFile, dp)

	req := MakeConversionRequest(dp)
	err = cc.Convert(req)
	if err != nil {
		return nil, err
	}

	return req, nil
}

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/ollama/ollama/api"
	log "github.com/sirupsen/logrus"
	"maps"
	"net/http"
	"net/url"
	"os"
	"time"
)

type OllamaInvocationClient struct {
	ModelName      string
	RequestOptions map[string]interface{}
	client         *api.Client
}

var llmOutputSchema = json.RawMessage(`{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": {
    "type": "string"
  }
}`)

func (llm *OllamaInvocationClient) Configure(args map[string]interface{}) error {
	if llm.client == nil {
		urlStr, err := args["OLLAMA_API_URL"]
		if !err {
			return fmt.Errorf("OLLAMA_API_URL could not be found in args")
		}

		client := http.Client{}
		url, _ := url.Parse(urlStr.(string))
		api_client := api.NewClient(url, &client)
		llm.client = api_client
	}

	return nil
}

func (llm *OllamaInvocationClient) Prepare(args map[string]interface{}) error {
	model, ok := args["model_name"]
	if !ok {
		log.Fatal("model_name must be a string")
		return nil
	}

	nargs := make(map[string]interface{})
	maps.Copy(nargs, args)

	delete(nargs, "model_name")

	//XXX depends on LLM Client/Model
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
	maps.Insert(nargs, maps.All(defaultParams))

	llm.ModelName = model.(string)
	llm.RequestOptions = nargs

	return nil
}

func (llm *OllamaInvocationClient) logLLMResponse(args ...string) {
	fhash := []byte(args[0])
	fname := fmt.Sprintf("chatlogs/%s_%8x_%d.log", llm.ModelName, sha256.Sum256(fhash), time.Now().UnixMicro())
	logf, err := os.OpenFile(fname,
		os.O_CREATE|os.O_RDWR, 0644)
	defer logf.Close()
	written := 0
	if err == nil {
		_, _ = logf.WriteString("# Query\n\n")
		wr, _ := logf.WriteString(args[1])
		written += wr
		_, _ = logf.WriteString("\n\n# Response\n\n```\n")
		wr, _ = logf.WriteString(args[2])
		written += wr
		_, _ = logf.WriteString("\n```\n")
	}
	log.Debugf("logged llm response to: %s with %d bytes", fname, written)
}

func (llm *OllamaInvocationClient) InvokeLLM(runner context.Context, buf bytes.Buffer) (string, Metrics, error) {

	var metrics = Metrics{}
	if llm.client == nil {
		return "", metrics, fmt.Errorf("LLM client not initialized")
	}

	steam := new(bool)
	req := api.GenerateRequest{
		Model:   llm.ModelName,
		Prompt:  buf.String(),
		Stream:  steam,
		Options: llm.RequestOptions,
		Format:  llmOutputSchema,
	}

	callback := make(chan api.GenerateResponse)
	//TODO: make configurable
	deadline, cancel := context.WithDeadline(runner, time.Now().Add(time.Minute*5))
	defer cancel()
	go func() {
		err := llm.client.Generate(deadline, &req, func(gr api.GenerateResponse) error {
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

	metrics.ConversionTime += response.TotalDuration
	metrics.ConversionPromptTime += response.PromptEvalDuration
	metrics.ConversionEvalTime += response.EvalDuration
	metrics.ConversionPromptTokenCount += response.PromptEvalCount
	metrics.ConversionEvalTokenCount += response.EvalCount

	if response.Response == "" {
		return "", metrics, fmt.Errorf("response is empty - %s", response.DoneReason)
	}

	return response.Response, metrics, nil
}

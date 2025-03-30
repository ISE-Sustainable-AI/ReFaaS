package main

import (
	"bytes"
	"context"
	"fmt"
	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
	"time"
)

type GeminiInvocationClient struct {
	geminiAPIKey string
	model        string
}

func (g *GeminiInvocationClient) Configure(args map[string]interface{}) error {

	key, err := args["GEMINI_API_KEY"]
	if !err {
		return fmt.Errorf("GEMINI_API_KEY required")
	}
	g.geminiAPIKey = key.(string)

	model, ok := args["GEMINI_MODEL"]
	if ok {
		g.model = model.(string)
	} else {
		g.model = "gemini-2.0-flash"
	}

	return nil
}

func (g *GeminiInvocationClient) Prepare(args map[string]interface{}) error {
	if args == nil {
		return nil
	}
	model, ok := args["GEMINI_MODEL"]
	if ok {
		g.model = model.(string)
	}
	return nil
}

func (g *GeminiInvocationClient) InvokeLLM(ctx context.Context, buf bytes.Buffer) (string, Metrics, error) {
	start := time.Now()
	client, err := genai.NewClient(ctx, option.WithAPIKey(g.geminiAPIKey))
	if err != nil {
		return "", Metrics{}, err
	}
	defer client.Close()

	model := client.GenerativeModel(g.model)

	model.ResponseMIMEType = "application/json"
	//TODO: figure out Schema
	model.ResponseSchema = &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"main.go": &genai.Schema{
				Type:     genai.TypeString,
				Nullable: true,
			},
			"go.mod": &genai.Schema{
				Type:     genai.TypeString,
				Nullable: true,
			},
			"main.py": &genai.Schema{
				Type:     genai.TypeString,
				Nullable: true,
			},
		},
	}
	var metrics = Metrics{}

	resp, err := model.GenerateContent(ctx, genai.Text(buf.String()))
	metrics.ConversionTime = time.Since(start)
	metrics.ConversionPromptTime = time.Since(start)
	metrics.ConversionEvalTime = time.Since(start)

	if resp != nil {
		metrics.ConversionPromptTokenCount += int(resp.UsageMetadata.PromptTokenCount)
		metrics.ConversionEvalTokenCount += int(resp.UsageMetadata.TotalTokenCount)
	}
	if err != nil {
		return "", metrics, err
	}

	var outBuf bytes.Buffer
	if resp != nil {
		for _, part := range resp.Candidates[0].Content.Parts {
			if txt, ok := part.(genai.Text); ok {
				outBuf.WriteString(string(txt))
			}
		}
	}
	return outBuf.String(), metrics, nil
}

func (g GeminiInvocationClient) logLLMResponse(s ...string) {
	//TODO implement me

}

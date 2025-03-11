package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/adrg/strutil"
	"github.com/adrg/strutil/metrics"
	log "github.com/sirupsen/logrus"
	"os/exec"
	"strings"
)

func (cc *CodeConverter) doTest(ctx context.Context, dir string, t *TestFile) (bool, error) {
	cmd := exec.CommandContext(ctx, "go", "run", ".")
	cmd.Dir = dir
	cmd.Env = t.Env
	_in := strings.NewReader(t.Input)
	_out := &bytes.Buffer{}
	_err := &bytes.Buffer{}

	cmd.Stdin = _in
	cmd.Stdout = _out
	cmd.Stderr = _err

	err := cmd.Run()
	if err != nil {
		return false, fmt.Errorf("test failed. %s - %s - %s", _out.String(), _err.String(), err)
	}
	cleanOut := MinimizeString(_out.String())

	assertEquals := cc.validateTestOutput(ctx, cleanOut, t.Output)
	if !assertEquals {
		return false, fmt.Errorf("test failed. %s, expected:%s, errors:%s", cleanOut, t.Output, _err.String())
	}

	return true, nil
}

func (cc *CodeConverter) validateTestOutput(ctx context.Context, testOutput, expectedOutput string) bool {
	if cc.validator != nil {
		return cc.validator.validate(testOutput, expectedOutput)
	} else {
		return SimilarityValidation{}.validate(testOutput, expectedOutput)
	}
}

type ValidationStrategy interface {
	validate(in, expected string) bool
}

type SimilarityValidation struct{}

func (SimilarityValidation) validate(in, expected string) bool {
	sim := strutil.Similarity(in, expected, metrics.NewOverlapCoefficient())
	return sim < 0.9
}

func MakeAwareSimilarityValidation(threshold float64) ValidationStrategy {
	return &JsonAwareSimilarityValidation{
		threshold:          threshold,
		fallBackValidation: SimilarityValidation{},
	}
}

type JsonAwareSimilarityValidation struct {
	threshold          float64
	fallBackValidation ValidationStrategy
}

func (vs JsonAwareSimilarityValidation) validate(in, expected string) bool {
	var expectedJson map[string]interface{}
	err := json.Unmarshal([]byte(expected), &expectedJson)
	if err != nil {
		return vs.fallBackValidation.validate(in, expected)
	}

	var actualJson map[string]interface{}

	err = json.Unmarshal([]byte(in), &actualJson)
	if err != nil {
		return vs.fallBackValidation.validate(in, expected)
	}

	if val, ok := actualJson["error"]; ok {
		log.Debugf("handle function caused error: %s", val)
		return false
	}

	if val, ok := actualJson["response"]; ok {
		return vs.compareMap(expectedJson, val.(map[string]interface{}))
	} else {
		return vs.compareMap(expectedJson, actualJson)
	}

}

func (vs JsonAwareSimilarityValidation) compareMap(expexted, actual map[string]interface{}) bool {
	for k, v := range expexted {
		if vv, ok := actual[k]; ok {
			switch v.(type) {
			case string:
				sim := strutil.Similarity(v.(string), vv.(string), metrics.NewOverlapCoefficient())
				if sim < vs.threshold {
					return false
				}
				break
			case map[string]interface{}:
				switch vv.(type) {
				case map[string]interface{}:
					return vs.compareMap(v.(map[string]interface{}), vv.(map[string]interface{}))
				case string:
					data, _ := json.Marshal(v)
					sim := strutil.Similarity(string(data), vv.(string), metrics.NewOverlapCoefficient())
					if sim < vs.threshold {
						return false
					}
					break
				default:
					return false
				}
			case int:
				if v.(int) != vv.(int) {
					return false
				}
				break
			case float64:
				if v.(float64) != vv.(float64) {
					return false
				}
				break
			default:
				if v == nil && vv == nil {
					return true
				} else {
					return false
				}
			}

		} else {
			return false
		}
	}
	return true
}

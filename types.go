package main

import "time"

type DeploymentPackage struct {
	RootFile   string
	TestFiles  map[string]string
	BuildFiles map[string]string
	BuildCmd   []string

	Metrics *Metrics
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

	TestCases map[string]bool `json:"test_cases"`
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

	if m.StartTime.After(mm.StartTime) {
		m.StartTime = mm.StartTime
	}

	if m.EndTime.Before(mm.EndTime) {
		m.EndTime = mm.EndTime
	}
}

type TestFile struct {
	Input  string `json:"input"`
	Output string `json:"output"`
	//ENV variables to override in case of a test
	Env []string `json:"env"`
	//Services to mock/deploy for the test
	Services map[string]string `json:"services"`
}

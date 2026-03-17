package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"sort"
)

type OllamaRequest struct {
	ModelName string `json:"model"`
	Prompt    string `json:"prompt"`
}

type ModelRequest struct {
	Name string `json:"name"`
}

type OllamaResponse struct {
	Model        string `json:"model"`
	CreatedAt    string `json:"created_at"`
	Response     string `json:"response"`
	Done         bool   `json:"done"`
	EvalCount    int    `json:"eval_count"`
	EvalDuration int64  `json:"eval_duration"`
}

type ModelInfo struct {
	Name         string `json:"name"`
	Parameters   string `json:"parameters"`
	Quantization string `json:"quantization"`
	Size         int64  `json:"size"`
	Details      struct {
		Family            string `json:"family"`
		ParameterSize     string `json:"parameter_size"`
		QuantizationLevel string `json:"quantization_level"`
	} `json:"details"`
	ContextLength int `json:"context_length"`
}

func fetchModels() ([]ModelInfo, error) {
	mainURL := os.Getenv("OLLAMA_API")
	if mainURL == "" {
		mainURL = defaultAPIHost
	}
	resp, err := http.Get(mainURL + "/api/tags")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Models []ModelInfo `json:"models"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return result.Models, nil
}

func initModels() error {
	models, err := fetchModels()
	if err != nil {
		return err
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].Name < models[j].Name
	})
	globalModels = models
	return nil
}

func getOllamaVersion() string {
	mainURL := os.Getenv("OLLAMA_API")
	if mainURL == "" {
		mainURL = defaultAPIHost
	}
	resp, err := http.Get(mainURL + "/api/version")
	if err != nil {
		return "Unknown"
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "Unknown"
	}

	var result struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "Unknown"
	}

	return result.Version
}

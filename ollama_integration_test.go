package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestFetchModels_Integration(t *testing.T) {
	// Create a mock server to simulate Ollama API
	mockModels := struct {
		Models []ModelInfo `json:"models"`
	}{
		Models: []ModelInfo{
			{
				Name: "llama3:latest",
				Size: 4661224679, // ~4.66 GB
				Details: struct {
					Family            string `json:"family"`
					ParameterSize     string `json:"parameter_size"`
					QuantizationLevel string `json:"quantization_level"`
				}{
					Family:            "llama",
					ParameterSize:     "8b",
					QuantizationLevel: "Q4_0",
				},
			},
			{
				Name: "mistral:latest",
				Size: 4109865159, // ~4.11 GB
				Details: struct {
					Family            string `json:"family"`
					ParameterSize     string `json:"parameter_size"`
					QuantizationLevel string `json:"quantization_level"`
				}{
					Family:            "llama",
					ParameterSize:     "7b",
					QuantizationLevel: "Q4_0",
				},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/tags" {
			json.NewEncoder(w).Encode(mockModels)
		} else if r.URL.Path == "/api/show" {
			var body struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("Failed to decode /api/show body: %v", err)
			}
			modelName := body.Name
			var contextLen int
			if modelName == "llama3:latest" {
				contextLen = 8192
			} else if modelName == "mistral:latest" {
				contextLen = 32768
			}
			resp := map[string]any{
				"model_info": map[string]any{
					"llama.context_length": float64(contextLen),
				},
			}
			json.NewEncoder(w).Encode(resp)
		} else {
			t.Errorf("Unexpected request path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	// Temporarily override OLLAMA_API to use our mock server
	oldAPI := os.Getenv("OLLAMA_API")
	os.Setenv("OLLAMA_API", server.URL)
	defer os.Setenv("OLLAMA_API", oldAPI)

	models, err := fetchModels()
	if err != nil {
		t.Fatalf("fetchModels failed: %v", err)
	}

	if len(models) != 2 {
		t.Errorf("Expected 2 models, got %d", len(models))
	}

	// Verify the 'size' parameter and other details for the first model
	if models[0].Name != "llama3:latest" {
		t.Errorf("Expected first model name 'llama3:latest', got '%s'", models[0].Name)
	}

	expectedSize := int64(4661224679)
	if models[0].Size != expectedSize {
		t.Errorf("Expected model size %d, got %d", expectedSize, models[0].Size)
	}

	if models[0].Details.ParameterSize != "8b" {
		t.Errorf("Expected parameter_size '8b', got '%s'", models[0].Details.ParameterSize)
	}

	// Verify quantization level
	if models[0].Details.QuantizationLevel != "Q4_0" {
		t.Errorf("Expected quantization_level 'Q4_0', got '%s'", models[0].Details.QuantizationLevel)
	}

	// Verify ContextLength
	if models[0].ContextLength != 8192 {
		t.Errorf("Expected context_length 8192 for llama3, got %d", models[0].ContextLength)
	}

	if models[1].ContextLength != 32768 {
		t.Errorf("Expected context_length 32768 for mistral, got %d", models[1].ContextLength)
	}
}

func TestGetOllamaVersion_Integration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/version" {
			t.Errorf("Expected to request '/api/version', got: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"version":"0.1.32"}`))
	}))
	defer server.Close()

	// Temporarily override OLLAMA_API
	oldAPI := os.Getenv("OLLAMA_API")
	os.Setenv("OLLAMA_API", server.URL)
	defer os.Setenv("OLLAMA_API", oldAPI)

	version := getOllamaVersion()
	if version != "0.1.32" {
		t.Errorf("Expected version '0.1.32', got '%s'", version)
	}
}

func TestDefaultURLFallback(t *testing.T) {
	// Clear OLLAMA_API environment variable
	oldAPI := os.Getenv("OLLAMA_API")
	os.Unsetenv("OLLAMA_API")
	defer os.Setenv("OLLAMA_API", oldAPI)

	// Since we don't have a server running on localhost:11434,
	// fetchModels should fail, but we want to verify it ATTEMPTS to use the default URL.
	// This is tricky without mocking the http.Client, but we can verify the behavior
	// by ensuring it doesn't use the previously set server URL.

	_, err := fetchModels()
	if err == nil {
		// If it succeeded, it means something is actually running on localhost:11434,
		// which is unexpected but not necessarily an error in our code logic.
		t.Log("Warning: fetchModels succeeded with default URL, is Ollama running locally?")
	} else {
		// The error should contain "localhost:11434"
		if !containsString(err.Error(), "127.0.0.1:11434") && !containsString(err.Error(), "localhost:11434") {
			t.Errorf("Expected error to mention default URL (localhost:11434), got: %v", err)
		}
	}
}

func containsString(s, substr string) bool {
	return (len(s) >= len(substr) && (s[:len(substr)] == substr || containsString(s[1:], substr)))
}

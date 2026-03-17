package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func runBenchmarkCLI(modelName string, ollamaAPI string, iterations int) {
	ollamaAPIURL := ollamaAPI

	var totalTokensPerSecond float64

	if !contains(globalModels, modelName) {
		fmt.Println("Model not supported. Please use a supported model from the list:", globalModels)
		return
	}

	sysinfo, err := getSysInfo()
	if err != nil {
		return
	}
	fmt.Printf("CPU: %+v\n", sysinfo.CPUName)
	fmt.Printf("Memory: %+v\n", sysinfo.Memory)
	fmt.Printf("OS: %+v\n", sysinfo.OS)
	fmt.Printf("Kernel: %+v\n", sysinfo.Kernel)

	gpuinfo, err := getGPUInfo()
	if err != nil {
		return
	}
	fmt.Printf("GPU Name: %+v\n", gpuinfo.Name)
	fmt.Printf("Driver Version: %+v\n", gpuinfo.DriverVersion)
	fmt.Printf("GPU Memory: %+v\n", gpuinfo.Memory)

	modelRequest := ModelRequest{
		Name: modelName,
	}
	jsonData, _ := json.Marshal(modelRequest)
	fullURL := ollamaAPI + "/api/pull"
	fmt.Println("Pulling model " + modelName + ", Please wait...")
	resp, err := http.Post(fullURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		fmt.Println("Error pulling model:", string(body))
		return
	}

	fmt.Println("Model pulled successfully")
	fmt.Println("Benchmarking...")

	for i := 0; i < iterations; i++ {
		requestBody := OllamaRequest{
			ModelName: modelName,
			Prompt:    "Tell me about Llamas in 500 words.",
		}

		jsonData, _ := json.Marshal(requestBody)
		resp, err := http.Post(ollamaAPIURL+"/api/generate", "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		defer resp.Body.Close()

		var response OllamaResponse
		var responseText string
		decoder := json.NewDecoder(resp.Body)

		fmt.Printf("Benchmarking iteration %d in progress..", i+1)
		progressTicker := time.NewTicker(500 * time.Millisecond)
		defer progressTicker.Stop()

		done := make(chan bool)
		go func() {
			for {
				select {
				case <-progressTicker.C:
					fmt.Print(".")
				case <-done:
					fmt.Println()
					return
				}
			}
		}()

		for {
			err := decoder.Decode(&response)
			if err == io.EOF {
				done <- true
				break
			}
			if err != nil {
				fmt.Println("\nError:", err)
				done <- true
				break
			}

			responseText += response.Response
		}

		tokensPerSecond := float64(response.EvalCount) / (float64(response.EvalDuration) / 1e9)
		totalTokensPerSecond += tokensPerSecond
	}

	avgTokensPerSecond := totalTokensPerSecond / float64(iterations)

	fmt.Printf("\nBenchmark completed for %s\n", modelName)
	fmt.Printf("Average Tokens per Second: %.2f\n", avgTokensPerSecond)
}

func contains(models []ModelInfo, modelName string) bool {
	for _, model := range models {
		if model.Name == modelName {
			return true
		}
	}
	return false
}

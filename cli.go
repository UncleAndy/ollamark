package main

import (
	"flag"
	"fmt"
	"os"
)

func parseAndRunCLI() bool {
	// Parse command-line arguments (Ollamark CLI)
	defaultOllamaAPI := os.Getenv("OLLAMA_API")
	if defaultOllamaAPI == "" {
		defaultOllamaAPI = defaultAPIHost
	}

	modelPtr := flag.String("m", "llama3", "Model name to benchmark (default: llama3)")
	ollamaPtr := flag.String("o", defaultOllamaAPI, "Ollama API endpoint (default "+defaultOllamaAPI+")")
	iterationsPtr := flag.Int("i", 2, "Number of benchmark iterations (Min 2, Max 20)")

	flag.Usage = func() {
		fmt.Println("Usage: ollamark [options]")
		fmt.Println("Options:")
		flag.PrintDefaults()
		fmt.Println("Examples:")
		fmt.Println("  For Ollamark GUI mode:")
		fmt.Println("      ollamark (no flags)")
		fmt.Println("  For Ollamark CLI mode:")
		fmt.Println("      ollamark -m llama3 -i 10")
		fmt.Println("      ollamark -m phi3")
		fmt.Println("      ollamark -m phi3 -o " + defaultAPIHost + "/api/generate")
	}

	flag.Parse()

	// Set the global API endpoint
	apiEndpoint = *ollamaPtr

	// Check if CLI arguments are provided
	if flag.NFlag() > 0 {
		if *modelPtr == "" || *ollamaPtr == "" {
			flag.Usage()
			os.Exit(1)
		}

		if flag.NArg() > 0 {
			flag.Usage()
			os.Exit(1)
		}

		if (*iterationsPtr < 2) || (*iterationsPtr > 20) {
			flag.Usage()
			os.Exit(1)
		}

		// Run ollamark in CLI mode
		runBenchmarkCLI(*modelPtr, apiEndpoint, *iterationsPtr)
		return true
	}

	return false
}

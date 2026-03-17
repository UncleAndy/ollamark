// Ollamark By Carsen Klock 2024 under the MIT license
// https://github.com/context-labs/ollamark
// https://ollamark.com
// Ollamark Client

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"image/color"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	xwidget "fyne.io/x/fyne/widget"
	"github.com/context-labs/ollamark/v2/internal/assets"
	"github.com/joho/godotenv"
	"github.com/shirou/gopsutil/mem"
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

type SysInfo struct {
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	Version string `json:"version"`
	Kernel  string `json:"kernel"`
	CPU     string `json:"cpu"`
	CPUName string `json:"cpu_name"`
	Memory  string `json:"memory"`
}

type GPUInfo struct {
	Name          string `json:"name"`
	Vendor        string `json:"vendor"`
	Memory        string `json:"memory"`
	DriverVersion string `json:"driver_version"`
	Count         int    `json:"count"`
}

var (
	globalModels   []ModelInfo
	apiEndpoint    string
	clientVersion  = "0.0.1"
	defaultAPIHost = "http://localhost:11434"
)

// ProofOfWorkChallenge represents a proof-of-work challenge
type ModelInfo struct {
	Name         string `json:"name"`
	Parameters   string `json:"parameters"`
	Quantization string `json:"quantization"`
	Size         int64  `json:"size"`
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

	// Read the raw JSON response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Unmarshal the JSON response
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

func getCPUName() string {
	// get windows cpu name
	if runtime.GOOS == "windows" {
		cmd := exec.Command("wmic", "cpu", "get", "name")
		output, err := cmd.Output()
		if err != nil {
			return "Unknown"
		}
		lines := strings.Split(string(output), "\n")
		if len(lines) > 1 {
			return strings.TrimSpace(lines[1])
		}
		return "Unknown"
	}

	// get linux cpu name
	if runtime.GOOS == "linux" {
		cmd := exec.Command("lshw", "-C", "cpu")
		cmd.Env = append(os.Environ(),
			"LC_ALL=C",
			"LANG=C",
		)
		output, err := cmd.Output()
		if err != nil {
			slog.Error("failed to parse CPU version: %v", err)
			// return "Unknown"
		}
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "product:") {
				return strings.TrimSpace(strings.Split(line, ":")[1])
			}
		}
		return "Unknown"
	}

	return "Unknown"
}

func getKernelVersion() (string, error) {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("wmic", "os", "get", "Version", "/value")
		output, err := cmd.Output()
		if err != nil {
			return "", err
		}
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "Version=") {
				return strings.TrimSpace(strings.Split(line, "=")[1]), nil
			}
		}
		return "", fmt.Errorf("failed to parse Windows version")
	}

	cmd := exec.Command("uname", "-r")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func getSysInfo() (*SysInfo, error) {
	v, _ := mem.VirtualMemory()
	// s, _ := mem.SwapMemory()

	totalMemory := v.Total / 1024 / 1024 / 1024
	// usedMemory := v.Used
	// availableMemory := v.Available
	// swapTotal := s.Total
	// swapUsed := s.Used

	sysInfo := &SysInfo{}
	sysInfo.OS = runtime.GOOS
	sysInfo.Arch = runtime.GOARCH
	sysInfo.Version = "0.0.1"
	kernelVersion, err := getKernelVersion()
	if err != nil {
		return nil, err
	}
	sysInfo.Kernel = kernelVersion
	sysInfo.CPU = strconv.Itoa(runtime.NumCPU())
	// get CPU Name for Windows and Linux

	sysInfo.CPUName = getCPUName()

	sysInfo.Memory = strconv.Itoa(int(totalMemory)) + " GB"

	// Get system information if macOS (darwin) and aarch64 (arm64) then get the info with apple silicon only command: TODO (Test)
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		cmd := exec.Command("system_profiler", "SPHardwareDataType")
		output, err := cmd.Output()
		if err != nil {
			return nil, err
		}
		// Extract the CPU information from the output
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "Chip:") {
				sysInfo.CPUName = strings.TrimSpace(strings.Split(line, ":")[1])
				break
			}
		}
	}

	return sysInfo, nil
}

func getMacGPUInfo() (*GPUInfo, error) {
	cmd := exec.Command("system_profiler", "SPDisplaysDataType")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	gpuInfo := &GPUInfo{}
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "Chipset Model:") {
			gpuInfo.Name = strings.TrimSpace(strings.Split(line, ":")[1])
			gpuInfo.Vendor = "Apple"
			break
		}
	}

	// If we couldn't find GPU info, it's likely integrated with the CPU
	if gpuInfo.Name == "" {
		cpuCmd := exec.Command("system_profiler", "SPHardwareDataType")
		cpuOutput, err := cpuCmd.Output()
		if err != nil {
			return nil, err
		}
		cpuLines := strings.Split(string(cpuOutput), "\n")
		for _, line := range cpuLines {
			if strings.Contains(line, "Chip:") {
				gpuInfo.Name = strings.TrimSpace(strings.Split(line, ":")[1]) + " GPU"
				gpuInfo.Vendor = "Apple"
				break
			}
		}
	}

	// Memory information isn't easily available for integrated GPUs
	gpuInfo.Memory = "Shared"
	gpuInfo.DriverVersion = "N/A"
	gpuInfo.Count = 1

	return gpuInfo, nil
}

func getGPUInfo() (*GPUInfo, error) {
	// First, attempt to use nvidia-smi to fetch Nvidia GPU info
	nvidiaGPU, err := getNvidiaGPUInfo()
	if err == nil {
		return nvidiaGPU, nil
	}

	// If Nvidia GPU info fetching fails, attempt to fetch AMD GPU info
	amdGPU, err := getAMDGPUInfo()
	if err == nil {
		return amdGPU, nil
	}

	// Check if we're on macOS (darwin) and arm64 architecture
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		return getMacGPUInfo()
	}

	// If both methods fail, return the last error
	return nil, err
}

func getNvidiaGPUInfo() (*GPUInfo, error) {
	cmd := exec.Command("nvidia-smi", "--query-gpu=name,memory.total,driver_version", "--format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	outputStr := strings.TrimSpace(string(output))
	lines := strings.Split(outputStr, "\n")[0] // Assuming single GPU
	fields := strings.Split(lines, ",")

	if len(fields) < 2 {
		return nil, fmt.Errorf("failed to parse Nvidia GPU information")
	}

	return &GPUInfo{
		Name:          strings.TrimSpace(fields[0]),
		Vendor:        "NVIDIA",
		Memory:        strings.TrimSpace(fields[1]),
		DriverVersion: strings.TrimSpace(fields[2]),
		Count:         len(lines),
	}, nil
}

func getAMDGPUInfo() (*GPUInfo, error) {
	switch runtime.GOOS {
	case "windows":
		return getAMDGPUInfoWindows()
	case "linux":
		return getAMDGPUInfoLinux()
	case "darwin":
		return nil, fmt.Errorf("macOS, Skipping AMD GPU info")
	default:
		return nil, fmt.Errorf("AMD GPU unsupported operating system")
	}
}

func getAMDGPUInfoWindows() (*GPUInfo, error) {
	cmd := exec.Command("wmic", "path", "win32_VideoController", "get", "Name,DriverVersion", "/format:list")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to execute wmic command: %v", err)
	}

	outputStr := string(output)
	return parseWMICOutput(outputStr)
}

func parseWMICOutput(output string) (*GPUInfo, error) {
	lines := strings.Split(output, "\n")
	info := GPUInfo{}
	gpuNames := make(map[string]bool) // To track unique GPU names

	for _, line := range lines {
		if strings.HasPrefix(line, "Name=") {
			name := strings.TrimSpace(strings.Split(line, "=")[1])
			// Skip integrated and virtual GPUs
			if strings.Contains(name, "Integrated") || strings.Contains(name, "Display Adapter") || strings.Contains(name, "AMD Radeon(TM) Graphics") {
				continue
			}
			if !gpuNames[name] {
				gpuNames[name] = true
				info.Name = name
				info.Vendor = "AMD" // Assuming AMD if we are parsing this on an AMD system check
				info.Count++
			}
		} else if strings.HasPrefix(line, "DriverVersion=") {
			info.DriverVersion = strings.TrimSpace(strings.Split(line, "=")[1])
			info.Memory = "Unknown" // Placeholder for memory, as WMIC does not provide it directly
		}
	}

	if info.Name == "" {
		return nil, fmt.Errorf("no dedicated AMD GPUs found")
	}

	return &info, nil
}

func getAMDGPUInfoLinux() (*GPUInfo, error) {
	cmd := exec.Command("lshw", "-C", "display")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	outputStr := string(output)
	// Example of parsing, adjust according to actual output
	if strings.Contains(outputStr, "Radeon") || strings.Contains(outputStr, "AMD") {
		name := extractField(outputStr, "product")
		// vendor := "AMD"
		memory := extractField(outputStr, "size")

		return &GPUInfo{
			Name: name,
			// Vendor: vendor,
			Memory: memory,
		}, nil
	}

	return nil, fmt.Errorf("no AMD GPU detected")
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

func extractField(data, fieldName string) string {
	// Simple parsing logic, needs to be adjusted based on actual output
	start := strings.Index(data, fieldName+":")
	if start == -1 {
		return ""
	}
	start += len(fieldName) + 1
	end := strings.Index(data[start:], "\n")
	if end == -1 {
		return data[start:]
	}
	return strings.TrimSpace(data[start : start+end])
}

func main() {
	// Load environment variables from .env file
	if _, err := os.Stat(".env"); err == nil {
		err := godotenv.Load()
		if err != nil {
			fmt.Println("Error loading .env file:", err)
		}
	}

	fmt.Println("Loading Ollamark...")

	fmt.Println("Checking Ollama Version...")
	ollamaVersion := getOllamaVersion()
	if ollamaVersion == "Unknown" {
		fmt.Println("Ollama not found, please install Ollama from https://ollama.com/download to Ollamark 😎")
		return
	}
	fmt.Println("Ollama Version:", ollamaVersion)

	err := initModels()
	if err != nil {
		fmt.Println("Failed to initialize models:", err)
		return
	}

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

	// Parse command-line arguments (Ollamark CLI)
	defaultOllamaAPI := os.Getenv("OLLAMA_API")
	if defaultOllamaAPI == "" {
		defaultOllamaAPI = defaultAPIHost
	}
	modelPtr := flag.String("m", "llama3", "Model name to benchmark (default: llama3)")
	ollamaPtr := flag.String("o", defaultOllamaAPI, "Ollama API endpoint (default "+defaultOllamaAPI+")")
	iterationsPtr := flag.Int("i", 2, "Number of benchmark iterations (Min 2, Max 20)")
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
		return
	}

	// Create a new Fyne app
	a := app.NewWithID("Ollamark")
	a.Settings().SetTheme(theme.DarkTheme())
	fyne.CurrentApp().Settings().SetTheme(fyne.CurrentApp().Settings().Theme())
	w := a.NewWindow("Ollamark - Ollama Benchmark")

	// set window size
	w.Resize(fyne.NewSize(400, 300))
	w.CenterOnScreen()

	// create a logo
	logoData, _ := assets.WebResources.ReadFile("logo.svg")
	logo := canvas.NewImageFromResource(fyne.NewStaticResource("logo.svg", logoData))
	logo.FillMode = canvas.ImageFillContain // Use 'Contain' to ensure the image fits well
	logo.SetMinSize(fyne.NewSize(100, 100))

	// Load the SVG icon
	icon := fyne.NewStaticResource("logo.svg", logoData)
	// Set the application icon
	a.SetIcon(icon)

	sysinfo, _ := getSysInfo()
	gpuinfo, _ := getGPUInfo()
	ollamaVersion = getOllamaVersion()

	// create an api entry field
	apiEntry := widget.NewEntry()
	apiEntry.SetText(apiEndpoint)

	// create a title label
	titleLabel := widget.NewLabel("Ollama API Endpoint")
	titleLabel.TextStyle = fyne.TextStyle{Bold: true}

	title2Label := widget.NewLabel("Select a model to benchmark")
	title2Label.TextStyle = fyne.TextStyle{Bold: true}

	// Create a slice of model names for the dropdown
	modelNames := make([]string, len(globalModels))
	for i, model := range globalModels {
		sizeGB := float64(model.Size) / (1024 * 1024 * 1024)
		modelNames[i] = fmt.Sprintf("%s (%.2f GB)", model.Name, sizeGB)
	}

	// Create the select widget with model names
	modelSelect := widget.NewSelect(modelNames, func(value string) {
		// You can add logic here if needed when a model is selected
	})

	// Set the default selected model
	// Find the index of "llama3" in the modelNames slice
	defaultIndex := 0
	for i, model := range globalModels {
		if model.Name == "llama3" {
			defaultIndex = i
			break
		}
	}
	modelSelect.SetSelected(modelNames[defaultIndex])

	resultLabel := widget.NewLabel("")
	resultLabel.Alignment = fyne.TextAlignCenter
	resultLabel.Hide()

	// Custom text field for tokens per second
	tokensPerSecondText := canvas.NewText("", color.White)
	tokensPerSecondText.TextStyle.Bold = true
	tokensPerSecondText.TextSize = 38 // Larger text size
	tokensPerSecondText.Alignment = fyne.TextAlignCenter
	tokensPerSecondText.Hide()

	tpsText := canvas.NewText("", color.White)
	tpsText.TextStyle.Bold = true
	tpsText.TextSize = 16 // Larger text size
	tpsText.Alignment = fyne.TextAlignCenter
	tpsText.Hide()

	sysText := widget.NewLabel("")
	sysText.Hide()

	gpuText := widget.NewLabel("")
	gpuText.Hide()

	ollamaVersionText := widget.NewLabel("")
	ollamaVersionText.Hide()

	iterationsSlider := widget.NewSlider(2, 20)
	iterationsSlider.SetValue(2)
	iterationsSlider.Step = 1

	iterationsLabel := widget.NewLabel("Iterations: 2")
	iterationsSlider.OnChanged = func(value float64) {
		iterationsLabel.SetText(fmt.Sprintf("Iterations: %d", int(value)))
	}

	sysText.SetText(fmt.Sprintf("CPU: %s\nMemory: %s\nOS: %s\nKernel: %s", sysinfo.CPUName, sysinfo.Memory, sysinfo.OS, sysinfo.Kernel))
	sysText.Show()
	sysText.Refresh()

	// if gpu Info is available, show it
	if gpuinfo != nil {
		gpuText.SetText(fmt.Sprintf("GPU Name: %s\nGPU Memory: %s\nDriver Version: %s", gpuinfo.Name, gpuinfo.Memory, gpuinfo.DriverVersion))
		gpuText.Show()
		gpuText.Refresh()
	}

	// set ollama version text make version bold
	ollamaVersionText.SetText(fmt.Sprintf("Ollama Version: %s", ollamaVersion))
	ollamaVersionText.Show()
	ollamaVersionText.Refresh()

	// create a progress bar
	progressBar := widget.NewProgressBarInfinite()
	progressBar.Hide()

	// Load loader.gif from assets
	loaderData, _ := assets.WebResources.ReadFile("loader.gif")
	// Since NewAnimatedGif only takes a URI, we need to provide one.
	// We use a temporary file to serve the embedded content.
	tmpGif, _ := os.CreateTemp("", "loader*.gif")
	tmpGif.Write(loaderData)
	tmpGif.Close()
	gifURI := storage.NewFileURI(tmpGif.Name())
	gif, err := xwidget.NewAnimatedGif(gifURI)
	// Note: Ideally we'd remove it when the app closes
	// defer os.Remove(tmpGif.Name())
	if err != nil {
		fmt.Println("Error loading gif:", err)
	} else {
		gif.Start()
		gif.Show()
	}

	var benchmarkCancel context.CancelFunc
	stopButton := widget.NewButton("Stop", nil)
	stopButton.Disable()
	stopButton.OnTapped = func() {
		if benchmarkCancel != nil {
			benchmarkCancel()
		}
		stopButton.Disable()
	}

	benchmarkButton := widget.NewButton("Benchmark", nil)
	benchmarkButton.OnTapped = func() {
		benchmarkButton.SetText("Benchmarking...")
		benchmarkButton.Disable()
		stopButton.Enable()

		resultLabel.Show()
		resultLabel.SetText("Benchmarks starting...")
		resultLabel.Refresh()

		tokensPerSecondText.Hide()
		tpsText.Hide()
		// sysText.Hide()
		// gpuText.Hide()

		var ctx context.Context
		ctx, benchmarkCancel = context.WithCancel(context.Background())

		go func() {
			defer func() {
				stopButton.Disable()
				if benchmarkCancel != nil {
					benchmarkCancel()
				}
			}()

			progressBar.Show()
			progressBar.Refresh()

			// get api url and model name from entry fields
			apiURL := apiEntry.Text
			selectedModel := modelSelect.Selected
			modelName := ""
			// Extract model name from "Name (Size GB)" format
			if idx := strings.LastIndex(selectedModel, " ("); idx != -1 {
				modelName = selectedModel[:idx]
			} else {
				modelName = selectedModel
			}
			iterations := int(iterationsSlider.Value)

			modelRequest := ModelRequest{
				Name: modelName,
			}
			jsonData, _ := json.Marshal(modelRequest)
			fullURL := apiURL + "/api/pull"
			resultLabel.SetText("Pulling model " + modelName + ", Please wait...")
			resultLabel.Refresh()

			req, err := http.NewRequestWithContext(ctx, "POST", fullURL, bytes.NewBuffer(jsonData))
			if err != nil {
				resultLabel.SetText("Error: " + err.Error())
				benchmarkButton.SetText("Benchmark")
				benchmarkButton.Enable()
				progressBar.Hide()
				progressBar.Refresh()
				gif.Hide()
				return
			}
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				if ctx.Err() == context.Canceled {
					resultLabel.SetText("Benchmark stopped")
				} else {
					resultLabel.SetText("Error: " + err.Error())
				}
				benchmarkButton.SetText("Benchmark")
				benchmarkButton.Enable()
				progressBar.Hide()
				progressBar.Refresh()
				gif.Hide()
				return
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK {
				resultLabel.SetText(fmt.Sprintf("Error pulling model: %s", body))
				benchmarkButton.SetText("Benchmark")
				benchmarkButton.Enable()
				progressBar.Hide()
				progressBar.Refresh()
				gif.Hide()
				return
			}

			// fmt.Println("Model pull response:", string(body)) // Debug print
			resultLabel.SetText("Model pulled successfully")
			resultLabel.Refresh()
			resultLabel.SetText("Benchmarking...")
			resultLabel.Refresh()

			var totalTokensPerSecond float64

			for i := 0; i < iterations; i++ {
				if ctx.Err() != nil {
					resultLabel.SetText("Benchmark stopped")
					break
				}
				requestBody := OllamaRequest{
					ModelName: modelName,
					Prompt:    "Tell me about Llamas in 500 words.",
				}

				jsonData, _ := json.Marshal(requestBody)
				req, err := http.NewRequestWithContext(ctx, "POST", apiURL+"/api/generate", bytes.NewBuffer(jsonData))
				if err != nil {
					resultLabel.SetText("Error: " + err.Error())
					benchmarkButton.SetText("Benchmark")
					benchmarkButton.Enable()
					progressBar.Hide()
					progressBar.Refresh()
					gif.Hide()
					return
				}
				req.Header.Set("Content-Type", "application/json")

				resp, err := client.Do(req)
				if err != nil {
					if ctx.Err() == context.Canceled {
						resultLabel.SetText("Benchmark stopped")
					} else {
						resultLabel.SetText("Error: " + err.Error())
					}
					benchmarkButton.SetText("Benchmark")
					benchmarkButton.Enable()
					progressBar.Hide()
					progressBar.Refresh()
					gif.Hide()
					return
				}
				defer resp.Body.Close()

				// start := time.Now()

				var response OllamaResponse
				var responseText string
				decoder := json.NewDecoder(resp.Body)

				resultLabel.SetText(fmt.Sprintf("Benchmark #%d in progress...", i+1))
				resultLabel.Refresh()

				stopIteration := false
				for {
					if ctx.Err() != nil {
						resultLabel.SetText("Benchmark stopped")
						stopIteration = true
						break
					}
					err := decoder.Decode(&response)
					if err == io.EOF {
						break
					}
					if err != nil {
						if ctx.Err() == context.Canceled {
							resultLabel.SetText("Benchmark stopped")
						} else {
							resultLabel.SetText("Error: " + err.Error())
						}
						progressBar.Hide()
						progressBar.Refresh()
						benchmarkButton.SetText("Benchmark")
						benchmarkButton.Enable()
						return
					}

					responseText += response.Response
					progressBar.Refresh()
				}
				if stopIteration {
					break
				}

				// duration := time.Since(start).Seconds()
				tokensPerSecond := float64(response.EvalCount) / (float64(response.EvalDuration) / 1e9)

				totalTokensPerSecond += tokensPerSecond
			}

			if ctx.Err() != nil {
				progressBar.Hide()
				gif.Hide()
				progressBar.Refresh()
				benchmarkButton.SetText("Benchmark")
				benchmarkButton.Enable()
				return
			}

			avgTokensPerSecond := totalTokensPerSecond / float64(iterations)

			resultLabel.SetText(fmt.Sprintf("Benchmark completed for %s\nAverage Tokens per second: %.2f\nBenchmarked with %d iterations", modelName, avgTokensPerSecond, iterations))
			resultLabel.Alignment = fyne.TextAlignCenter
			resultLabel.Refresh()

			// update custom text
			tokensPerSecondText.Text = fmt.Sprintf("%.2f", avgTokensPerSecond) // Update the custom text
			tokensPerSecondText.Show()
			tpsText.Text = "Tokens per second"
			tokensPerSecondText.Refresh()
			tpsText.Refresh() // Refresh to update the display
			tpsText.Show()

			progressBar.Hide()
			gif.Hide()
			progressBar.Refresh() // Refresh after hiding the ProgressBar
			benchmarkButton.SetText("Benchmark")
			benchmarkButton.Enable()
		}()
	}

	// border/group around systext and gputext
	sysInfoGroup := container.NewVBox(ollamaVersionText, sysText, gpuText)
	sysInfoGroupLabel := widget.NewLabel("System Information")
	sysInfoGroupLabel.TextStyle = fyne.TextStyle{Bold: true}
	sysInfoGroup = container.NewBorder(sysInfoGroupLabel, nil, nil, nil, sysInfoGroup)

	content := container.NewVBox(
		logo,
		sysInfoGroup,
		titleLabel,
		apiEntry,
		title2Label,
		modelSelect,
		iterationsLabel,
		iterationsSlider,
		gif,
		// widget.NewSeparator(),
		tokensPerSecondText,
		tpsText,
		resultLabel,
		progressBar,
		// widget.NewSeparator(),
		benchmarkButton,
		stopButton,
	)

	// Wrap the content with a padded container
	paddedContent := container.NewPadded(container.NewPadded(content))

	w.SetContent(paddedContent)
	w.ShowAndRun()
}

func contains(models []ModelInfo, modelName string) bool {
	for _, model := range models {
		if model.Name == modelName {
			return true
		}
	}
	return false
}

func runBenchmarkCLI(modelName string, ollamaAPI string, iterations int) {
	ollamaAPIURL := ollamaAPI

	var totalTokensPerSecond float64

	// modelName needs to match a model name in MODELS
	if !contains(globalModels, modelName) {
		fmt.Println("Model not supported. Please use a supported model from the list:", globalModels)
		return
	}

	sysinfo, err := getSysInfo()
	if err != nil {
		// fmt.Println("Error:", err)
		return
	}
	fmt.Printf("CPU: %+v\n", sysinfo.CPUName)
	fmt.Printf("Memory: %+v\n", sysinfo.Memory)
	fmt.Printf("OS: %+v\n", sysinfo.OS)
	fmt.Printf("Kernel: %+v\n", sysinfo.Kernel)

	gpuinfo, err := getGPUInfo()
	if err != nil {
		// fmt.Println("Error:", err)
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
				return
			}

			responseText += response.Response
		}

		// duration := time.Since(start).Seconds()
		tokensPerSecond := float64(response.EvalCount) / (float64(response.EvalDuration) / 1e9)

		totalTokensPerSecond += tokensPerSecond
	}

	avgTokensPerSecond := totalTokensPerSecond / float64(iterations)

	fmt.Printf("\nBenchmark completed for %s\n", modelName)
	fmt.Printf("Average Tokens per second: %.2f\n", avgTokensPerSecond)
}

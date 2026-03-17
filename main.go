// Ollamark By Carsen Klock 2024 under the MIT license
// https://github.com/context-labs/ollamark
// https://ollamark.com
// Ollamark Client

package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"github.com/shirou/gopsutil/mem"
)

var (
	globalModels   []ModelInfo
	apiEndpoint    string
	clientVersion  = "0.0.1"
	defaultAPIHost = "http://localhost:11434"
)

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
			slog.Error("failed to parse CPU version", "error", err)
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

	totalMemory := v.Total / 1024 / 1024 / 1024

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

	sysInfo.CPUName = getCPUName()

	sysInfo.Memory = strconv.Itoa(int(totalMemory)) + " GB"

	// Get system information if macOS (darwin) and aarch64 (arm64) then get the info with apple silicon only command
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

	gpuInfo.Memory = "Shared"
	gpuInfo.DriverVersion = "N/A"
	gpuInfo.Count = 1

	return gpuInfo, nil
}

func getGPUInfo() (*GPUInfo, error) {
	nvidiaGPU, err := getNvidiaGPUInfo()
	if err == nil {
		return nvidiaGPU, nil
	}

	amdGPU, err := getAMDGPUInfo()
	if err == nil {
		return amdGPU, nil
	}

	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		return getMacGPUInfo()
	}

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
	gpuNames := make(map[string]bool)

	for _, line := range lines {
		if strings.HasPrefix(line, "Name=") {
			name := strings.TrimSpace(strings.Split(line, "=")[1])
			if strings.Contains(name, "Integrated") || strings.Contains(name, "Display Adapter") || strings.Contains(name, "AMD Radeon(TM) Graphics") {
				continue
			}
			if !gpuNames[name] {
				gpuNames[name] = true
				info.Name = name
				info.Vendor = "AMD"
				info.Count++
			}
		} else if strings.HasPrefix(line, "DriverVersion=") {
			info.DriverVersion = strings.TrimSpace(strings.Split(line, "=")[1])
			info.Memory = "Unknown"
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
	if strings.Contains(outputStr, "Radeon") || strings.Contains(outputStr, "AMD") {
		name := extractField(outputStr, "product")
		memory := extractField(outputStr, "size")

		return &GPUInfo{
			Name:   name,
			Memory: memory,
		}, nil
	}

	return nil, fmt.Errorf("no AMD GPU detected")
}

func extractField(data, fieldName string) string {
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

	if parseAndRunCLI() {
		return
	}

	runGUI()
}

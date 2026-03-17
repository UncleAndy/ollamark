package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"net/http"
	"os"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	xwidget "fyne.io/x/fyne/widget"
	"github.com/context-labs/ollamark/v2/internal/assets"
)

func runGUI() {
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
	ollamaVersion := getOllamaVersion()

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
		contextLen := model.ContextLength
		quantization := model.Details.QuantizationLevel
		if quantization == "" {
			quantization = model.Quantization
		}

		if contextLen == 0 {
			modelNames[i] = fmt.Sprintf("%s (%.2f GB, %s)", model.Name, sizeGB, quantization)
		} else {
			modelNames[i] = fmt.Sprintf("%s (%.2f GB, %s, ctx:%dk)", model.Name, sizeGB, quantization, contextLen/1024)
		}
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
	if len(modelNames) > 0 {
		modelSelect.SetSelected(modelNames[defaultIndex])
	}

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

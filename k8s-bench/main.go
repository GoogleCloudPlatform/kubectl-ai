// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/k8s-bench/pkg/model"
	"sigs.k8s.io/yaml"
)

type Task struct {
	Goal       string `json:"goal"`
	Setup      string `json:"setup,omitempty"`
	Verifier   string `json:"verifier,omitempty"`
	Cleanup    string `json:"cleanup,omitempty"`
	Difficulty string `json:"difficulty"`
	Disabled   bool   `json:"disabled,omitempty"`
}

type EvalConfig struct {
	LLMConfigs  []model.LLMConfig
	KubeConfig  string
	TasksDir    string
	TaskPattern string
	AgentBin    string

	OutputDir string
}

type AnalyzeConfig struct {
	InputDir     string
	OutputFormat string
}

func expandPath(path string) (string, error) {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, path[2:])
	}
	return filepath.Clean(os.ExpandEnv(path)), nil
}

func main() {
	// Print top-level usage if help is requested directly
	if len(os.Args) > 1 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		printUsage()
		return
	}

	ctx := context.Background()
	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

// Define custom usage text to show subcommands
func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s <command> [options]\n\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  run       Run evaluation benchmarks\n")
	fmt.Fprintf(os.Stderr, "  analyze   Analyze results from previous benchmark runs\n\n")
	fmt.Fprintf(os.Stderr, "Run '%s <command> --help' for more information on a command.\n", os.Args[0])
}

type Strings []string

func (f *Strings) String() string {
	return strings.Join(*f, ",")
}

func (f *Strings) Set(s string) error {
	*f = append(*f, s)
	return nil
}

func run(ctx context.Context) error {
	// No need to check for help flags here anymore

	// Default to "run" subcommand if no arguments provided
	subCommand := "run"
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "-") {
		subCommand = os.Args[1]
		// Shift the arguments
		os.Args = append(os.Args[:1], os.Args[2:]...)
	}

	switch subCommand {
	case "run":
		return runEvals(ctx)
	case "analyze":
		return runAnalyze()
	default:
		printUsage()
		return fmt.Errorf("unknown subcommand: %s, valid options are 'run' or 'analyze'", subCommand)
	}
}

func runEvals(ctx context.Context) error {
	config := EvalConfig{
		TasksDir: "./tasks",
	}

	// Set custom usage for 'run' subcommand
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s run [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Run K8s-bench evaluation benchmarks.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	llmProvider := "gemini"
	modelList := ""
	defaultKubeConfig := "~/.kube/config"
	strategyList := "chat-based,react"

	flag.StringVar(&config.TasksDir, "tasks-dir", config.TasksDir, "Directory containing evaluation tasks")
	flag.StringVar(&config.KubeConfig, "kubeconfig", config.KubeConfig, "Path to kubeconfig file")
	flag.StringVar(&config.TaskPattern, "task-pattern", config.TaskPattern, "Pattern to filter tasks (e.g. 'pod' or 'redis')")
	flag.StringVar(&config.AgentBin, "agent-bin", config.AgentBin, "Path to kubernetes agent binary")
	flag.StringVar(&llmProvider, "llm-provider", llmProvider, "Specific LLM provider to evaluate (e.g. 'gemini' or 'ollama')")
	flag.StringVar(&modelList, "models", modelList, "Comma-separated list of models to evaluate (e.g. 'gemini-1.0,gemini-2.0')")
	flag.StringVar(&strategyList, "strategies", strategyList, "Comma-separated list of strategies to evaluate (e.g. 'chat-based,react')")
	flag.StringVar(&config.OutputDir, "output-dir", config.OutputDir, "Directory to write results to")
	flag.Parse()

	if config.KubeConfig == "" {
		config.KubeConfig = defaultKubeConfig
	}

	expandedKubeconfig, err := expandPath(config.KubeConfig)
	if err != nil {
		return fmt.Errorf("failed to expand kubeconfig path %q: %w", config.KubeConfig, err)
	}
	config.KubeConfig = expandedKubeconfig

	defaultModels := map[string][]string{
		"gemini": {"gemini-2.0-flash-thinking-exp-01-21"},
	}

	models := defaultModels
	if modelList != "" {
		if llmProvider == "" {
			return fmt.Errorf("--llm-provider is required when --models is specified")
		}
		modelSlice := strings.Split(modelList, ",")
		models = map[string][]string{
			llmProvider: modelSlice,
		}
	}

	for _, strategy := range strings.Split(strategyList, ",") {
		if strategy == "" {
			continue
		}

		for llmProviderID, models := range models {
			for _, modelID := range models {
				id := fmt.Sprintf("%s-%s-%s", strategy, llmProviderID, modelID)
				config.LLMConfigs = append(config.LLMConfigs, model.LLMConfig{
					ID:         id,
					ProviderID: llmProviderID,
					ModelID:    modelID,
					Strategy:   strategy,
				})
			}
		}
	}

	if err := runEvaluation(ctx, config); err != nil {
		return fmt.Errorf("running evaluation: %w", err)
	}

	return nil
}

func runAnalyze() error {
	config := AnalyzeConfig{
		InputDir:     "",
		OutputFormat: "markdown",
	}

	// Set custom usage for 'analyze' subcommand
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s analyze --input-dir <directory> [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Analyze results from previous K8s-bench runs.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	var resultsFilePath string
	flag.StringVar(&config.InputDir, "input-dir", config.InputDir, "Directory containing evaluation results (required)")
	flag.StringVar(&config.OutputFormat, "output-format", config.OutputFormat, "Output format (markdown or json)")
	flag.StringVar(&resultsFilePath, "results-filepath", "", "Optional file path to write results to")
	flag.Parse()

	// Check if input-dir is provided
	if config.InputDir == "" {
		flag.Usage()
		return fmt.Errorf("--input-dir is required")
	}

	// Check if output format is valid
	if config.OutputFormat != "markdown" && config.OutputFormat != "json" {
		return fmt.Errorf("invalid output format: %s, valid options are 'markdown' or 'json'", config.OutputFormat)
	}

	// Check if input directory exists
	if _, err := os.Stat(config.InputDir); os.IsNotExist(err) {
		return fmt.Errorf("input directory does not exist: %s", config.InputDir)
	}

	allResults, err := collectResults(config.InputDir)
	if err != nil {
		return fmt.Errorf("collecting results: %w", err)
	}

	// Format and output results
	if config.OutputFormat == "markdown" {
		if err := printMarkdownResults(allResults, resultsFilePath); err != nil {
			return fmt.Errorf("printing markdown results: %w", err)
		}
	} else {
		if err := printJSONResults(allResults, resultsFilePath); err != nil {
			return fmt.Errorf("printing JSON results: %w", err)
		}
	}

	return nil
}

func collectResults(inputDir string) ([]model.TaskResult, error) {
	var allResults []model.TaskResult

	// Walk through the directory structure to find all results.yaml files
	err := filepath.Walk(inputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Only process results.yaml files
		if !info.IsDir() && info.Name() == "results.yaml" {
			// Read and parse the results file
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("reading file %s: %w", path, err)
			}

			var result model.TaskResult
			if err := yaml.Unmarshal(data, &result); err != nil {
				return fmt.Errorf("parsing yaml from %s: %w", path, err)
			}

			allResults = append(allResults, result)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return allResults, nil
}

func printMarkdownResults(results []model.TaskResult, resultsFilePath string) error {
	// Create a buffer to hold the output
	var buffer strings.Builder

	buffer.WriteString("# K8s-bench Evaluation Results\n\n")

	// Group results by strategy
	resultsByStrategy := make(map[string][]model.TaskResult)
	allModels := make(map[string]bool) // Track all unique models

	for _, result := range results {
		strategy := result.LLMConfig.Strategy
		resultsByStrategy[strategy] = append(resultsByStrategy[strategy], result)
		allModels[result.LLMConfig.ModelID] = true
	}

	// ---------------------------------------------------------------------
	// MOVED: Create the model performance summary table at the top
	// ---------------------------------------------------------------------
	buffer.WriteString("## Model Performance Summary\n\n")

	// Table header with strategies as columns
	strategies := make([]string, 0, len(resultsByStrategy))
	for strategy := range resultsByStrategy {
		strategies = append(strategies, strategy)
	}

	// Sort strategies for consistent output
	sort.Strings(strategies)

	// Create header row with success/fail columns for each strategy
	buffer.WriteString("| Model |")
	for _, strategy := range strategies {
		buffer.WriteString(fmt.Sprintf(" %s Success | %s Fail |", strategy, strategy))
	}
	buffer.WriteString("\n|-------|")
	for range strategies {
		buffer.WriteString("------------|-----------|")
	}
	buffer.WriteString("\n")

	// Convert allModels map to a sorted slice
	models := make([]string, 0, len(allModels))
	for model := range allModels {
		models = append(models, model)
	}
	sort.Strings(models)

	// Add a row for each model with success/fail counts for each strategy
	for _, model := range models {
		buffer.WriteString(fmt.Sprintf("| %s |", model))

		for _, strategy := range strategies {
			successCount := 0
			failCount := 0

			// Count success/fail for this model and strategy
			for _, result := range resultsByStrategy[strategy] {
				if result.LLMConfig.ModelID == model {
					if strings.Contains(strings.ToLower(result.Result), "success") {
						successCount++
					} else if result.Result != "" {
						failCount++
					}
				}
			}

			buffer.WriteString(fmt.Sprintf(" %d | %d |", successCount, failCount))
		}
		buffer.WriteString("\n")
	}

	// Add a row showing overall totals for each strategy
	buffer.WriteString("| **Total** |")
	for _, strategy := range strategies {
		successCount := 0
		failCount := 0

		for _, result := range resultsByStrategy[strategy] {
			if strings.Contains(strings.ToLower(result.Result), "success") {
				successCount++
			} else if result.Result != "" {
				failCount++
			}
		}

		buffer.WriteString(fmt.Sprintf(" %d | %d |", successCount, failCount))
	}
	buffer.WriteString("\n\n")

	// Overall summary across all strategies
	totalCount := len(results)
	overallSuccessCount := 0
	overallFailCount := 0

	for _, result := range results {
		if strings.Contains(strings.ToLower(result.Result), "success") {
			overallSuccessCount++
		} else if result.Result != "" {
			overallFailCount++
		}
	}

	buffer.WriteString("## Overall Summary\n\n")
	buffer.WriteString(fmt.Sprintf("- Total: %d\n", totalCount))
	buffer.WriteString(fmt.Sprintf("- Success: %d (%d%%)\n", overallSuccessCount, calculatePercentage(overallSuccessCount, totalCount)))
	buffer.WriteString(fmt.Sprintf("- Fail: %d (%d%%)\n\n", overallFailCount, calculatePercentage(overallFailCount, totalCount)))

	// Create a table for each strategy
	for strategy, strategyResults := range resultsByStrategy {
		// Print a header for this strategy
		buffer.WriteString(fmt.Sprintf("## Strategy: %s\n\n", strategy))

		// Create the table header
		buffer.WriteString("| Task | Provider | Model | Result | Error |\n")
		buffer.WriteString("|------|----------|-------|--------|-------|\n")

		// Track success and failure counts for this strategy
		successCount := 0
		failCount := 0
		totalCount := len(strategyResults)

		// Add each result as a row in the table
		for _, result := range strategyResults {
			resultEmoji := "❌" // Default to failure
			if strings.Contains(strings.ToLower(result.Result), "success") {
				resultEmoji = "✅"
				successCount++
			} else if result.Result != "" {
				failCount++
			}

			buffer.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
				result.Task,
				result.LLMConfig.ProviderID,
				result.LLMConfig.ModelID,
				resultEmoji+" "+result.Result,
				result.Error))
		}

		// Add summary for this strategy
		buffer.WriteString(fmt.Sprintf("\n**%s Summary**\n\n", strategy))
		buffer.WriteString(fmt.Sprintf("- Total: %d\n", totalCount))
		buffer.WriteString(fmt.Sprintf("- Success: %d (%d%%)\n", successCount, calculatePercentage(successCount, totalCount)))
		buffer.WriteString(fmt.Sprintf("- Fail: %d (%d%%)\n\n", failCount, calculatePercentage(failCount, totalCount)))
	}

	// Add footer with generation time
	buffer.WriteString("---\n\n")
	buffer.WriteString(fmt.Sprintf("_Report generated on %s_\n", time.Now().Format("January 2, 2006 at 3:04 PM")))

	// Get the final output
	output := buffer.String()

	// Write to file if path is provided, otherwise print to stdout
	if resultsFilePath != "" {
		if err := os.WriteFile(resultsFilePath, []byte(output), 0644); err != nil {
			return fmt.Errorf("writing to file %q: %w", resultsFilePath, err)
		}
		fmt.Printf("Results written to %s\n", resultsFilePath)
	} else {
		// Print to stdout only if no file path is specified
		fmt.Print(output)
	}

	return nil
}

func calculatePercentage(part, total int) int {
	if total == 0 {
		return 0
	}
	return int((float64(part) / float64(total)) * 100)
}

func printJSONResults(results []model.TaskResult, resultsFilePath string) error {
	// Convert the results to JSON
	jsonData, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling results to JSON: %w", err)
	}

	// Write to file if path is provided, otherwise print to stdout
	if resultsFilePath != "" {
		if err := os.WriteFile(resultsFilePath, jsonData, 0644); err != nil {
			return fmt.Errorf("writing to file %q: %w", resultsFilePath, err)
		}
		fmt.Printf("Results written to %s\n", resultsFilePath)
	} else {
		// Print to stdout only if no file path is specified
		fmt.Println(string(jsonData))
	}

	return nil
}

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
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/GoogleCloudPlatform/kubectl-ai/k8s-bench/pkg/model"
	"sigs.k8s.io/yaml"
)

type ClusterCreationPolicy string

const (
	AlwaysCreate     ClusterCreationPolicy = "AlwaysCreate"
	CreateIfNotExist ClusterCreationPolicy = "CreateIfNotExist"
	DoNotCreate      ClusterCreationPolicy = "DoNotCreate"
)

type Task struct {
	Setup      string `json:"setup,omitempty"`
	Verifier   string `json:"verifier,omitempty"`
	Cleanup    string `json:"cleanup,omitempty"`
	Difficulty string `json:"difficulty"`
	Disabled   bool   `json:"disabled,omitempty"`
	Timeout    string `json:"timeout,omitempty"`

	Expect []Expectation `json:"expect,omitempty"`

	Script []ScriptStep `json:"script,omitempty"`

	// Isolation can be set to automatically create an isolated cluster
	// TODO: support namespaces also
	Isolation IsolationMode `json:"isolation,omitempty"`
}

type IsolationMode string

const (
	// IsolationModeCluster will create a cluster for the task evaluation.
	IsolationModeCluster IsolationMode = "cluster"
)

type ScriptStep struct {
	Prompt     string `json:"prompt"`
	PromptFile string `json:"promptFile"`
}

// ResolvePrompt resolves the prompt from either inline or file source
func (s *ScriptStep) ResolvePrompt(baseDir string) (string, error) {
	// Fail if both prompt and promptFile are provided to avoid confusion
	if s.Prompt != "" && s.PromptFile != "" {
		return "", fmt.Errorf("both 'prompt' and 'promptFile' are specified in script step; only one should be provided")
	}

	// If promptFile is provided, read the file
	if s.PromptFile != "" {
		// If the path is relative, resolve it relative to the task directory
		promptPath := s.PromptFile
		if !filepath.IsAbs(promptPath) {
			promptPath = filepath.Join(baseDir, s.PromptFile)
		}

		content, err := os.ReadFile(promptPath)
		if err != nil {
			return "", fmt.Errorf("failed to read prompt file %q: %w", promptPath, err)
		}

		return string(content), nil
	}

	// If prompt is provided, use it
	if s.Prompt != "" {
		return s.Prompt, nil
	}

	// If neither is provided, return an error
	return "", fmt.Errorf("neither 'prompt' nor 'promptFile' is specified in script step")
}

type Expectation struct {
	Contains string `json:"contains,omitempty"`
}

type EvalConfig struct {
	LLMConfigs            []model.LLMConfig
	KubeConfig            string
	TasksDir              string
	TaskPattern           string
	AgentBin              string
	AgentArgs             []string
	Concurrency           int
	CreateKindCluster     bool
	ClusterCreationPolicy ClusterCreationPolicy

	OutputDir        string
	MaxAgentDuration time.Duration
}

type AnalyzeConfig struct {
	InputDir          string
	OutputFormat      string
	IgnoreToolUseShim bool
	ShowFailures      bool
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

func kindClusterExists(clusterName string) (bool, error) {
	cmd := exec.Command("kind", "get", "clusters")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to run 'kind get clusters': %w", err)
	}
	clusters := strings.Split(string(output), "\n")
	for _, cluster := range clusters {
		if cluster == clusterName {
			return true, nil
		}
	}
	return false, nil
}

func createKindCluster(clusterName string) error {
	var createErr error
	for retry := range 3 {
		if retry > 0 {
			fmt.Printf("Retrying cluster creation, attempt %d\n", retry+1)
			time.Sleep(5 * time.Second)
		}
		createCmd := exec.Command("kind", "create", "cluster", "--name", clusterName, "--wait", "5m")
		fmt.Printf("Creating kind cluster %q\n", clusterName)
		createCmd.Stdout = os.Stdout
		createCmd.Stderr = os.Stderr
		createErr = createCmd.Run()
		if createErr == nil {
			return nil
		}
		fmt.Printf("failed to create kind cluster, retrying...: %v\n", createErr)
	}
	return fmt.Errorf("failed to create kind cluster after multiple retries: %w", createErr)
}

func deleteKindCluster(clusterName string) error {
	deleteCmd := exec.Command("kind", "delete", "cluster", "--name", clusterName)
	fmt.Printf("Deleting kind cluster %q\n", clusterName)
	deleteCmd.Stdout = os.Stdout
	deleteCmd.Stderr = os.Stderr
	return deleteCmd.Run()
}

type Strings []string

func (f *Strings) String() string {
	return strings.Join(*f, ",")
}

func (f *Strings) Set(s string) error {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil
	}
	*f = append(*f, fields...)
	return nil
}

type agentDetector func(agentBin string, agentArgs []string, cfg *model.LLMConfig) bool

var agentDetectors = []agentDetector{
	detectKubectlAIAgent,
	detectGeminiAgent,
}

func detectAgentLLMConfig(agentBin string, agentArgs []string) model.LLMConfig {
	llmConfig := model.LLMConfig{
		ID:         "agent",
		ProviderID: "agent",
		ModelID:    "default",
	}

	if agentBin == "" {
		return llmConfig
	}

	binName := strings.ToLower(filepath.Base(agentBin))
	for _, detector := range agentDetectors {
		if detector(binName, agentArgs, &llmConfig) {
			break
		}
	}

	return llmConfig
}

const (
	kubectlAIBinaryToken       = "kubectl-ai"
	kubectlAIDefaultProviderID = "gemini"
	kubectlAIDefaultModelID    = "gemini-2.5-pro"
)

func detectKubectlAIAgent(agentBin string, agentArgs []string, cfg *model.LLMConfig) bool {
	if !strings.Contains(agentBin, kubectlAIBinaryToken) {
		return false
	}

	cfg.ID = kubectlAIBinaryToken

	cfg.ProviderID = kubectlAIDefaultProviderID
	if provider := lookupFlagValue(agentArgs, "--llm-provider"); provider != "" {
		cfg.ProviderID = provider
	}

	cfg.ModelID = kubectlAIDefaultModelID
	if modelID := lookupFlagValue(agentArgs, "--model", "-m"); modelID != "" {
		cfg.ModelID = modelID
	}

	cfg.EnableToolUseShim = kubectlAIBoolFlag(agentArgs, "--enable-tool-use-shim")
	cfg.Quiet = kubectlAIBoolFlag(agentArgs, "--quiet")

	return true
}

func detectGeminiAgent(agentBin string, agentArgs []string, cfg *model.LLMConfig) bool {
	if !strings.Contains(agentBin, "gemini") {
		return false
	}

	cfg.ID = "gemini-cli"
	cfg.ProviderID = "gemini"

	if model := lookupFlagValue(agentArgs, "--model", "-m"); model != "" {
		cfg.ModelID = model
	}

	if version := detectGeminiModelVersion(agentArgs); version != "" {
		cfg.ModelVersion = version
	}

	return true
}

func lookupFlagValue(args []string, names ...string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		for _, name := range names {
			if strings.HasPrefix(arg, name+"=") {
				return strings.TrimPrefix(arg, name+"=")
			}
			if arg == name && i+1 < len(args) {
				return args[i+1]
			}
		}
	}
	return ""
}

func kubectlAIBoolFlag(args []string, name string) bool {
	value := false

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == name:
			value = true
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				if parsed, err := strconv.ParseBool(args[i+1]); err == nil {
					value = parsed
				}
			}
		case strings.HasPrefix(arg, name+"="):
			if parsed, err := strconv.ParseBool(strings.TrimPrefix(arg, name+"=")); err == nil {
				value = parsed
			}
		}
	}

	return value
}

func detectGeminiModelVersion(args []string) string {
	if version := lookupFlagValue(args, "--model-version"); version != "" {
		return version
	}
	if version := lookupFlagValue(args, "--api-version"); version != "" {
		return version
	}
	if options := lookupFlagValue(args, "--client-options"); options != "" {
		for _, field := range strings.FieldsFunc(options, func(r rune) bool {
			switch r {
			case ',', ';':
				return true
			default:
				return unicode.IsSpace(r)
			}
		}) {
			if strings.Contains(field, "=") {
				parts := strings.SplitN(field, "=", 2)
				key := strings.ToLower(parts[0])
				value := parts[1]
				if key == "apiversion" || key == "version" || key == "modelversion" {
					return value
				}
			}
		}
	}
	return ""
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
	start := time.Now()
	config := EvalConfig{
		TasksDir:         "./tasks",
		MaxAgentDuration: 5 * time.Minute,
	}

	// Set custom usage for 'run' subcommand
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s run [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Run K8s-bench evaluation benchmarks.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	var agentArgs Strings
	var kubeConfigFlag string

	flag.StringVar(&config.TasksDir, "tasks-dir", config.TasksDir, "Directory containing evaluation tasks")
	flag.StringVar(&kubeConfigFlag, "kubeconfig", "", "Path to the kubeconfig file to use for the evaluation")
	flag.StringVar(&config.TaskPattern, "task-pattern", config.TaskPattern, "Pattern to filter tasks (e.g. 'pod' or 'redis')")
	flag.StringVar(&config.AgentBin, "agent-bin", config.AgentBin, "Path to kubernetes agent binary")
	flag.Var(&agentArgs, "agent-args", "Additional argument to pass to the agent (can be specified multiple times)")
	flag.IntVar(&config.Concurrency, "concurrency", 0, "Number of tasks to run concurrently (0 = auto, 1 = sequential)")
	flag.StringVar((*string)(&config.ClusterCreationPolicy), "cluster-creation-policy", string(CreateIfNotExist), "Cluster creation policy: AlwaysCreate, CreateIfNotExist, DoNotCreate")
	flag.StringVar(&config.OutputDir, "output-dir", config.OutputDir, "Directory to write results to")
	flag.DurationVar(&config.MaxAgentDuration, "agent-timeout", config.MaxAgentDuration, "Maximum duration to allow each agent run before terminating it")
	flag.Parse()

	if config.MaxAgentDuration <= 0 {
		return fmt.Errorf("--agent-timeout must be greater than zero")
	}

	// Determine KubeConfig path: flag > env var > default
	if kubeConfigFlag != "" {
		config.KubeConfig = kubeConfigFlag
	} else if os.Getenv("KUBECONFIG") != "" {
		config.KubeConfig = os.Getenv("KUBECONFIG")
	} else {
		config.KubeConfig = "~/.kube/config"
	}

	expandedKubeconfig, err := expandPath(config.KubeConfig)
	if err != nil {
		return fmt.Errorf("failed to expand kubeconfig path %q: %w", config.KubeConfig, err)
	}
	config.KubeConfig = expandedKubeconfig

	if err := os.Setenv("KUBECONFIG", config.KubeConfig); err != nil {
		return fmt.Errorf("failed to set KUBECONFIG environment variable: %w", err)
	}

	config.AgentArgs = append(config.AgentArgs, agentArgs...)
	config.LLMConfigs = append(config.LLMConfigs, detectAgentLLMConfig(config.AgentBin, config.AgentArgs))

	tasks, err := loadTasks(config)
	if err != nil {
		return fmt.Errorf("failed to load tasks: %w", err)
	}

	// If concurrency is set to auto (0), use the number of tasks
	if config.Concurrency == 0 {
		config.Concurrency = len(tasks)
		fmt.Printf("Auto-configuring concurrency to %d (number of tasks)\n", config.Concurrency)
	}

	if err := runEvaluation(ctx, config); err != nil {
		return fmt.Errorf("running evaluation: %w", err)
	}

	fmt.Printf("Total evaluation time: %s\n", time.Since(start))
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
	flag.BoolVar(&config.IgnoreToolUseShim, "ignore-tool-use-shim", true, "Ignore tool use shim")
	flag.BoolVar(&config.ShowFailures, "show-failures", false, "Show failure details in markdown output")
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
		if err := printMarkdownResults(config, allResults, resultsFilePath); err != nil {
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

func printFailureAndErrorDetails(buffer *strings.Builder, results []model.TaskResult, title string, showModel bool) {
	hasFailures := false
	for _, result := range results {
		if len(result.Failures) > 0 || len(result.Error) > 0 {
			if !hasFailures {
				buffer.WriteString(fmt.Sprintf("\n**%s Failure Details**\n\n", title))
				hasFailures = true
			}
			// for the IgnoreToolUseShim case
			if showModel {
				buffer.WriteString(fmt.Sprintf("**Task: %s (%s)**\n", result.Task, result.LLMConfig.ModelID))
			} else {
				buffer.WriteString(fmt.Sprintf("**Task: %s**\n", result.Task))
			}
			buffer.WriteString(fmt.Sprintf("**Result: %s**\n", result.Result))
			for _, failure := range result.Failures {
				buffer.WriteString(fmt.Sprintf("```\n%s\n```\n", failure.Message))
			}
			if len(result.Error) > 0 {
				buffer.WriteString(fmt.Sprintf("```\n%s\n```\n", result.Error))
			}
			buffer.WriteString("\n")
		}
	}
}

func printMarkdownResults(config AnalyzeConfig, results []model.TaskResult, resultsFilePath string) error {
	// Create a buffer to hold the output
	var buffer strings.Builder

	buffer.WriteString("# K8s-bench Evaluation Results\n\n")

	allModels := make(map[string]bool) // Track all unique models
	for _, result := range results {
		allModels[result.LLMConfig.ModelID] = true
	}
	// Convert allModels map to a sorted slice
	models := make([]string, 0, len(allModels))
	for model := range allModels {
		models = append(models, model)
	}
	sort.Strings(models)

	// Overall summary across all results
	totalCount := len(results)
	overallSuccessCount := 0
	overallFailCount := 0
	overallErrorCount := 0
	for _, result := range results {
		if strings.Contains(strings.ToLower(result.Result), "success") {
			overallSuccessCount++
		} else if strings.Contains(strings.ToLower(result.Result), "fail") {
			overallFailCount++
		} else {
			overallErrorCount++
		}
	}

	// --- Model Performance Summary ---
	buffer.WriteString("## Model Performance Summary\n\n")

	if config.IgnoreToolUseShim {
		// Simplified table ignoring shim status
		buffer.WriteString("| Model | Success | Fail | Error |\n")
		buffer.WriteString("|-------|---------|------|-------|\n")

		for _, model := range models {
			successCount := 0
			failCount := 0
			errorCount := 0
			for _, result := range results {
				if result.LLMConfig.ModelID == model {
					if strings.Contains(strings.ToLower(result.Result), "success") {
						successCount++
					} else if strings.Contains(strings.ToLower(result.Result), "fail") {
						failCount++
					} else {
						errorCount++
					}
				}
			}
			buffer.WriteString(fmt.Sprintf("| %s | %d | %d | %d |\n", model, successCount, failCount, errorCount))
		}
		// Overall totals row
		buffer.WriteString("| **Total** |")
		buffer.WriteString(fmt.Sprintf(" %d | %d | %d |\n\n", overallSuccessCount, overallFailCount, overallErrorCount))

	} else {
		// Original table grouped by tool use shim status
		resultsByToolUseShim := make(map[string][]model.TaskResult)
		for _, result := range results {
			var toolUseShimStr string
			if result.LLMConfig.EnableToolUseShim {
				toolUseShimStr = "shim_enabled"
			} else {
				toolUseShimStr = "shim_disabled"
			}
			resultsByToolUseShim[toolUseShimStr] = append(resultsByToolUseShim[toolUseShimStr], result)
		}

		toolUseShimStrs := make([]string, 0, len(resultsByToolUseShim))
		for toolUseShimStr := range resultsByToolUseShim {
			toolUseShimStrs = append(toolUseShimStrs, toolUseShimStr)
		}
		sort.Strings(toolUseShimStrs)

		// Create header row with success/fail columns for each toolUseShimStr
		buffer.WriteString("| Model |")
		for _, toolUseShimStr := range toolUseShimStrs {
			buffer.WriteString(fmt.Sprintf(" %s Success | %s Fail |", toolUseShimStr, toolUseShimStr))
		}
		buffer.WriteString("\n|-------|")
		for range toolUseShimStrs {
			buffer.WriteString("------------|-----------|")
		}
		buffer.WriteString("\n")

		// Add a row for each model with success/fail counts for each strategy
		for _, model := range models {
			buffer.WriteString(fmt.Sprintf("| %s |", model))
			for _, toolUseShimStr := range toolUseShimStrs {
				successCount := 0
				failCount := 0
				// Count success/fail for this model and toolUseShimStr
				for _, result := range resultsByToolUseShim[toolUseShimStr] {
					if result.LLMConfig.ModelID == model {
						if strings.Contains(strings.ToLower(result.Result), "success") {
							successCount++
						} else {
							failCount++
						}
					}
				}
				buffer.WriteString(fmt.Sprintf(" %d | %d |", successCount, failCount))
			}
			buffer.WriteString("\n")
		}

		// Add a row showing overall totals for each toolUseShimStr
		buffer.WriteString("| **Total** |")
		for _, toolUseShimStr := range toolUseShimStrs {
			successCount := 0
			failCount := 0
			for _, result := range resultsByToolUseShim[toolUseShimStr] {
				if strings.Contains(strings.ToLower(result.Result), "success") {
					successCount++
				} else {
					failCount++
				}
			}
			buffer.WriteString(fmt.Sprintf(" %d | %d |", successCount, failCount))
		}
		buffer.WriteString("\n\n")
	}

	// --- Overall Summary ---
	buffer.WriteString("## Overall Summary\n\n")
	buffer.WriteString(fmt.Sprintf("- Total Runs: %d\n", totalCount))
	buffer.WriteString(fmt.Sprintf("- Overall Success: %d (%d%%)\n", overallSuccessCount, calculatePercentage(overallSuccessCount, totalCount)))
	buffer.WriteString(fmt.Sprintf("- Overall Fail: %d (%d%%)\n", overallFailCount, calculatePercentage(overallFailCount, totalCount)))
	buffer.WriteString(fmt.Sprintf("- Overall Error: %d (%d%%)\n\n", overallErrorCount, calculatePercentage(overallErrorCount, totalCount)))

	// --- Detailed Results ---
	if config.IgnoreToolUseShim {
		// Group results by model for detailed view
		resultsByModel := make(map[string][]model.TaskResult)
		for _, result := range results {
			resultsByModel[result.LLMConfig.ModelID] = append(resultsByModel[result.LLMConfig.ModelID], result)
		}

		for _, model := range models {
			buffer.WriteString(fmt.Sprintf("## Model: %s\n\n", model))
			buffer.WriteString("| Task | Provider | Result |\n")
			buffer.WriteString("|------|----------|--------|\n")

			modelSuccessCount := 0
			modelFailCount := 0
			modelErrorCount := 0
			modelResults := resultsByModel[model]
			modelTotalCount := len(modelResults)

			// Sort results within the model group for consistent output (e.g., by Task)
			sort.Slice(modelResults, func(i, j int) bool {
				return modelResults[i].Task < modelResults[j].Task
			})

			for _, result := range modelResults {
				resultEmoji := "❌" // Default to failure
				if strings.Contains(strings.ToLower(result.Result), "success") {
					resultEmoji = "✅"
					modelSuccessCount++
				} else if strings.Contains(strings.ToLower(result.Result), "fail") {
					modelFailCount++
				} else {
					modelErrorCount++
				}

				buffer.WriteString(fmt.Sprintf("| %s | %s | %s %s |\n",
					result.Task,
					result.LLMConfig.ProviderID,
					resultEmoji, result.Result))
			}

			// Add summary for this model
			buffer.WriteString(fmt.Sprintf("\n**%s Summary**\n\n", model))
			buffer.WriteString(fmt.Sprintf("- Total: %d\n", modelTotalCount))
			buffer.WriteString(fmt.Sprintf("- Success: %d (%d%%)\n", modelSuccessCount, calculatePercentage(modelSuccessCount, modelTotalCount)))
			buffer.WriteString(fmt.Sprintf("- Fail: %d (%d%%)\n", modelFailCount, calculatePercentage(modelFailCount, modelTotalCount)))
			buffer.WriteString(fmt.Sprintf("- Error: %d (%d%%)\n\n", modelErrorCount, calculatePercentage(modelErrorCount, modelTotalCount)))
			// After the summary, print failure details
			if config.ShowFailures {
				printFailureAndErrorDetails(&buffer, modelResults, model, false)
			}
		}

	} else {
		// Original detailed results grouped by tool use shim status
		resultsByToolUseShim := make(map[string][]model.TaskResult)
		for _, result := range results {
			var toolUseShimStr string
			if result.LLMConfig.EnableToolUseShim {
				toolUseShimStr = "shim_enabled"
			} else {
				toolUseShimStr = "shim_disabled"
			}
			resultsByToolUseShim[toolUseShimStr] = append(resultsByToolUseShim[toolUseShimStr], result)
		}
		toolUseShimStrs := make([]string, 0, len(resultsByToolUseShim))
		for toolUseShimStr := range resultsByToolUseShim {
			toolUseShimStrs = append(toolUseShimStrs, toolUseShimStr)
		}
		sort.Strings(toolUseShimStrs)

		for _, toolUseShimStr := range toolUseShimStrs {
			toolUseShimStrResults := resultsByToolUseShim[toolUseShimStr]
			// Print a header for this toolUseShimStr
			buffer.WriteString(fmt.Sprintf("## Tool Use: %s\n\n", toolUseShimStr))

			// Create the table header
			buffer.WriteString("| Task | Provider | Model | Result |\n")
			buffer.WriteString("|------|----------|-------|--------|\n")

			// Track success and failure counts for this strategy
			successCount := 0
			failCount := 0
			totalCount := len(toolUseShimStrResults)

			// Sort results within the group for consistent output (e.g., by Task)
			sort.Slice(toolUseShimStrResults, func(i, j int) bool {
				if toolUseShimStrResults[i].LLMConfig.ModelID != toolUseShimStrResults[j].LLMConfig.ModelID {
					return toolUseShimStrResults[i].LLMConfig.ModelID < toolUseShimStrResults[j].LLMConfig.ModelID
				}
				return toolUseShimStrResults[i].Task < toolUseShimStrResults[j].Task
			})

			// Add each result as a row in the table
			for _, result := range toolUseShimStrResults {
				resultEmoji := "❌" // Default to failure
				if strings.Contains(strings.ToLower(result.Result), "success") {
					resultEmoji = "✅"
					successCount++
				} else {
					failCount++
				}

				buffer.WriteString(fmt.Sprintf("| %s | %s | %s | %s %s |\n",
					result.Task,
					result.LLMConfig.ProviderID,
					result.LLMConfig.ModelID,
					resultEmoji, result.Result))
			}

			// Add summary for this toolUseShimStr
			buffer.WriteString(fmt.Sprintf("\n**%s Summary**\n\n", toolUseShimStr))
			buffer.WriteString(fmt.Sprintf("- Total: %d\n", totalCount))
			buffer.WriteString(fmt.Sprintf("- Success: %d (%d%%)\n", successCount, calculatePercentage(successCount, totalCount)))
			buffer.WriteString(fmt.Sprintf("- Fail: %d (%d%%)\n\n", failCount, calculatePercentage(failCount, totalCount)))

			// After the summary, print failure details
			if config.ShowFailures {
				printFailureAndErrorDetails(&buffer, toolUseShimStrResults, toolUseShimStr, true)
			}
		}
	}

	// --- Footer ---
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

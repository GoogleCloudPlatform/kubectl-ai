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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/k8s-bench/pkg/model"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"
)

func runEvaluation(ctx context.Context, config EvalConfig) error {
	logger := klog.FromContext(ctx)
	if config.ClusterCreationPolicy != DoNotCreate {
		clusterName := "k8s-bench-eval"
		clusterExists, err := kindClusterExists(clusterName)
		if err != nil {
			return fmt.Errorf("failed to check if kind cluster exists: %w", err)
		}

		if config.ClusterCreationPolicy == AlwaysCreate && clusterExists {
			logger.Info("Deleting existing kind cluster for evaluation run", "name", clusterName)
			if err := deleteKindCluster(clusterName); err != nil {
				return fmt.Errorf("failed to delete existing kind cluster: %w", err)
			}
			clusterExists = false
		}

		if !clusterExists {
			logger.Info("Creating kind cluster for evaluation run", "name", clusterName)
			if err := createKindCluster(clusterName); err != nil {
				return fmt.Errorf("failed to create kind cluster: %w", err)
			}
		}

		// Get kubeconfig
		logger.Info("Getting kubeconfig for kind cluster", "name", clusterName)
		kubeconfigBytes, err := exec.Command("kind", "get", "kubeconfig", "--name", clusterName).Output()
		if err != nil {
			return fmt.Errorf("failed to get kubeconfig for kind cluster: %w", err)
		}

		// Write kubeconfig to a temp file
		kubeconfigFile, err := os.CreateTemp("", "kubeconfig-*.yaml")
		if err != nil {
			return fmt.Errorf("failed to create temp file for kubeconfig: %w", err)
		}
		defer os.Remove(kubeconfigFile.Name()) // Clean up the temp file

		if _, err := kubeconfigFile.Write(kubeconfigBytes); err != nil {
			return fmt.Errorf("failed to write kubeconfig to temp file: %w", err)
		}
		kubeconfigFile.Close()

		logger.Info("Wrote Kubeconfig to", "path", kubeconfigFile.Name())
		config.KubeConfig = kubeconfigFile.Name()
	}

	if config.OutputDir == "" {
		return fmt.Errorf("must set OutputDir")
	}

	tasks, err := loadTasks(config)
	if err != nil {
		return fmt.Errorf("failed to load tasks: %w", err)
	}

	// Fallback to sequential execution if concurrency is not set
	if config.Concurrency <= 0 {
		config.Concurrency = 1
	}

	// Create a channel for tasks to be processed
	type taskJob struct {
		taskID string
		task   Task
	}
	taskCh := make(chan taskJob, len(tasks))

	// Create a channel for collecting results
	resultsCh := make(chan model.TaskResult, len(tasks)*len(config.LLMConfigs))

	// Create a separate channel for errors
	errorsCh := make(chan error, config.Concurrency)

	// Load all tasks into the tasks channel
	for taskID, task := range tasks {
		taskCh <- taskJob{taskID: taskID, task: task}
	}
	close(taskCh)

	// Create a wait group to track all workers
	var wg sync.WaitGroup

	fmt.Printf("Running tasks with concurrency: %d\n", config.Concurrency)

	// Start workers based on concurrency setting
	for i := 0; i < config.Concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for job := range taskCh {
				fmt.Printf("Worker %d: Evaluating task: %s\n", workerID, job.taskID)

				for _, llmConfig := range config.LLMConfigs {
					taskOutputDir := ""
					if config.OutputDir != "" {
						taskOutputDir = filepath.Join(config.OutputDir, job.taskID, llmConfig.ID)
						if err := os.MkdirAll(taskOutputDir, 0755); err != nil {
							errorsCh <- fmt.Errorf("creating directory %q: %w", taskOutputDir, err)
							return
						}
					}

					var log io.Writer
					if taskOutputDir != "" {
						logPath := filepath.Join(taskOutputDir, "log.txt")
						logFile, err := os.Create(logPath)
						if err != nil {
							errorsCh <- fmt.Errorf("creating log file %q: %w", logPath, err)
							return
						}
						defer logFile.Close()
						log = logFile
					}

					start := time.Now()
					fmt.Printf("\033[36mWorker %d: Started %s for %s\033[0m\n", workerID, llmConfig.ID, job.taskID)

					result := evaluateTask(ctx, config, job.taskID, job.task, llmConfig, log)

					fmt.Printf("\033[32mWorker %d: Completed %s for %s in %s\033[0m\n",
						workerID,
						llmConfig.ID,
						job.taskID,
						time.Since(start).Round(time.Second),
					)

					if taskOutputDir != "" {
						if err := writeToYAMLFile(filepath.Join(taskOutputDir, "results.yaml"), result); err != nil {
							errorsCh <- fmt.Errorf("writing results to file: %w", err)
							return
						}
					}
					resultsCh <- result
				}
			}
		}(i)
	}

	// Wait for all workers to complete
	wg.Wait()
	close(resultsCh)
	close(errorsCh)

	// Check if there were any errors
	for err := range errorsCh {
		if err != nil {
			return err
		}
	}

	// Collect and print results
	var allResults []model.TaskResult
	for result := range resultsCh {
		allResults = append(allResults, result)
	}

	printResults(allResults)
	return nil
}

// writeToYAMLFile will encode the specified object as yaml, and write it to the file.
func writeToYAMLFile(p string, obj any) error {
	data, err := yaml.Marshal(obj)
	if err != nil {
		return fmt.Errorf("marshaling to yaml: %w", err)
	}
	if err := os.WriteFile(p, data, 0644); err != nil {
		return fmt.Errorf("writing to file %q: %w", p, err)
	}
	return nil
}

func loadTasks(config EvalConfig) (map[string]Task, error) {
	tasks := make(map[string]Task)

	var taskFilter *regexp.Regexp
	if config.TaskPattern != "" {
		var err error
		taskFilter, err = regexp.Compile(config.TaskPattern)
		if err != nil {
			return nil, fmt.Errorf("compiling task pattern regex %q: %w", config.TaskPattern, err)
		}
	}

	entries, err := os.ReadDir(config.TasksDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		taskID := entry.Name()
		if taskFilter != nil && !taskFilter.MatchString(taskID) {
			continue
		}

		taskFile := filepath.Join(config.TasksDir, taskID, "task.yaml")

		data, err := os.ReadFile(taskFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read task file %s: %w", taskFile, err)
		}

		var task Task
		if err := yaml.Unmarshal(data, &task); err != nil {
			return nil, fmt.Errorf("failed to parse task file %s: %w", taskFile, err)
		}

		// Skip disabled tasks
		if task.Disabled {
			fmt.Printf("Skipping disabled task: %s\n", taskID)
			continue
		}

		tasks[taskID] = task
	}

	return tasks, nil
}

// getLastNLines returns the last n lines of a string.
func getLastNLines(s string, n int) (string, bool) {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		return strings.Join(lines[len(lines)-n:], "\n"), true
	}
	return s, false
}

func evaluateTask(ctx context.Context, config EvalConfig, taskID string, task Task, llmConfig model.LLMConfig, log io.Writer) model.TaskResult {
	result := model.TaskResult{
		Task:      taskID,
		LLMConfig: llmConfig,
	}

	// Timeout limit for the whole task (setup, agent actions, verify)
	timeout := 10 * time.Minute
	if task.Timeout != "" {
		var err error
		timeout, err = time.ParseDuration(task.Timeout)
		if err != nil {
			result.Result = "fail"
			result.Error = fmt.Sprintf("parsing timeout: %v", err)
			return result
		}
	}

	taskCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	taskOutputDir := filepath.Join(config.OutputDir, taskID, llmConfig.ID)

	var logBuffer bytes.Buffer
	multiWriter := io.MultiWriter(&logBuffer)
	if log != nil {
		multiWriter = io.MultiWriter(log, &logBuffer)
	}

	x := &TaskExecution{
		AgentBin:      config.AgentBin,
		kubeConfig:    config.KubeConfig,
		result:        &result,
		llmConfig:     llmConfig,
		log:           multiWriter,
		task:          &task,
		taskID:        taskID,
		taskOutputDir: taskOutputDir,
	}

	taskDir := filepath.Join(config.TasksDir, taskID)
	taskDirAbs, err := filepath.Abs(taskDir)
	if err != nil {
		result.Result = "fail"
		result.Error = err.Error()
		return result
	}
	taskDir = taskDirAbs
	x.taskDir = taskDir

	defer func() {
		if err := x.runCleanup(context.Background()); err != nil {
			fmt.Printf("Warning: cleanup failed for task %s: %v\n", taskID, err)
		}
	}()

	if err := x.runSetup(taskCtx); err != nil {
		// Unexpected error
		result.Error = err.Error()
		return result
	}

	// Run the agent
	agentOutput, err := x.runAgent(taskCtx)
	if err != nil {
		if taskCtx.Err() == context.DeadlineExceeded {
			result.Result = "fail"
			result.AddFailure("task timed out after %v", timeout)
			return result
		}
		// Unexpected error
		result.Result = "error"
		const maxErrLogLines = 3
		logString := logBuffer.String()
		logTail, truncated := getLastNLines(logString, maxErrLogLines)
		// build log file path
		shimSegment := "shim_disabled"
		if x.llmConfig.EnableToolUseShim {
			shimSegment = "shim_enabled"
		}
		logPath := filepath.Join(
			config.OutputDir,
			taskID,
			shimSegment+"-"+x.llmConfig.ProviderID+"-"+x.llmConfig.ModelID,
		)
		errorMessage := fmt.Sprintf("agent encountered error: %v\n---LOG---\n%s", err, logTail)
		if truncated {
			errorMessage += fmt.Sprintf("\n... (log truncated, full log at %s)", logPath)
		}
		result.Error = errorMessage
		return result
	}

	var expectationFailures []model.Failure

	if len(task.Expect) > 0 {
		// find the output after the last run command and search it
		var lastCmdOutput string
		lastToolRunIndex := strings.LastIndex(agentOutput, "Running:")
		if lastToolRunIndex == -1 {
			// if no tool run found, parse the entire output
			lastCmdOutput = agentOutput
		} else {
			remaining := agentOutput[lastToolRunIndex:]
			newlineIndex := strings.Index(remaining, "\n")
			if newlineIndex != -1 {
				lastCmdOutput = remaining[newlineIndex+1:]
			}
			// if no newline, lastCmdOutput is empty string
		}

		for _, expect := range task.Expect {
			if expect.Contains != "" {
				re, err := regexp.Compile(expect.Contains)
				if err != nil {
					expectationFailures = append(expectationFailures, model.Failure{
						Message: fmt.Sprintf("invalid regex %q in task spec: %v", expect.Contains, err),
					})
					continue
				}
				if !re.MatchString(lastCmdOutput) {
					expectationFailures = append(expectationFailures, model.Failure{
						Message: fmt.Sprintf("regex %q did not match output %q", expect.Contains, lastCmdOutput),
					})
				}
			}
		}

		if len(expectationFailures) == 0 {
			fmt.Printf("\nAll output expectations met\n")
		}
	}

	verifierSucceeded := false
	// Run verifier if specified
	if task.Verifier != "" {
		verifierPath := filepath.Join(taskDir, task.Verifier)
		cmd := exec.CommandContext(taskCtx, verifierPath)
		cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", x.kubeConfig))
		fmt.Printf("\nRunning verifier for task %s\n", taskID)

		err := x.runCommand(cmd)
		if err == nil {
			verifierSucceeded = true
		} else {
			const maxLogLines = 20
			logString := logBuffer.String()
			logTail, truncated := getLastNLines(logString, maxLogLines)
			// build log file path
			shimSegment := "shim_disabled"
			if x.llmConfig.EnableToolUseShim {
				shimSegment = "shim_enabled"
			}
			logPath := filepath.Join(
				config.OutputDir,
				taskID,
				shimSegment+"-"+x.llmConfig.ProviderID+"-"+x.llmConfig.ModelID,
			)
			failureMessage := fmt.Sprintf("verifier script failed: %v\n---LOG---\n%s", err, logTail)
			if truncated {
				failureMessage += fmt.Sprintf("\n... (log truncated, full log at %s)", logPath)
			}
			result.AddFailure("%s", failureMessage)
		}
	}

	expectationsMet := len(task.Expect) > 0 && len(expectationFailures) == 0
	if verifierSucceeded || expectationsMet {
		result.Result = "success"
	} else {
		result.Result = "fail"
		result.Failures = append(result.Failures, expectationFailures...)
	}

	return result
}

type TaskExecution struct {
	// kubeConfig is the path to the kubeconfig file we should use.
	// It will be created in IsolationModeCluster
	kubeConfig string

	// AgentBin holds the path to the agent to execute
	AgentBin string

	llmConfig model.LLMConfig
	result    *model.TaskResult
	log       io.Writer
	task      *Task
	taskID    string
	taskDir   string

	// taskOutputDir is where we can create artifacts or write logs while executing the task
	taskOutputDir string

	// cleanupFunctions are a set of cleanupFunctions we run to undo anything we ran
	cleanupFunctions []func() error
}

func (x *TaskExecution) runSetup(ctx context.Context) error {
	log := klog.FromContext(ctx)

	// Create cluster if requested
	if x.task.Isolation == IsolationModeCluster {
		kubeconfigPath := filepath.Join(x.taskDir, "kubeconfig.yaml")
		x.kubeConfig = kubeconfigPath

		clusterName := fmt.Sprintf("k8s-bench-%s", x.taskID)
		log.Info("creating kind cluster", "name", clusterName)

		args := []string{
			"kind",
			"create", "cluster",
			"--name", clusterName,
			"--wait", "5m",
			"--kubeconfig", kubeconfigPath,
		}
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		cmd.Dir = x.taskDir

		x.cleanupFunctions = append(x.cleanupFunctions, func() error {
			args := []string{
				"kind",
				"delete", "cluster",
				"--name", clusterName,
				"--kubeconfig", kubeconfigPath,
			}
			cmd := exec.CommandContext(ctx, args[0], args[1:]...)
			cmd.Dir = x.taskDir
			return x.runCommand(cmd)
		})

		if err := x.runCommand(cmd); err != nil {
			return err
		}
	}

	// Run setup if specified
	if x.task.Setup != "" {
		setupPath := filepath.Join(x.taskDir, x.task.Setup)
		cmd := exec.CommandContext(ctx, setupPath)
		cmd.Dir = x.taskDir
		cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", x.kubeConfig))

		if err := x.runCommand(cmd); err != nil {
			return err
		}
	}

	return nil
}

func (x *TaskExecution) runCleanup(ctx context.Context) error {
	var errs []error

	// Run cleanup if specified
	if x.task.Cleanup != "" {
		cleanupPath := filepath.Join(x.taskDir, x.task.Cleanup)
		cmd := exec.CommandContext(ctx, cleanupPath)
		cmd.Dir = x.taskDir
		cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", x.kubeConfig))

		if err := x.runCommand(cmd); err != nil {
			fmt.Printf("Warning: cleanup failed for task %s: %v\n", x.taskID, err)
		}
	}

	for _, cleanup := range x.cleanupFunctions {
		if err := cleanup(); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (x *TaskExecution) runAgent(ctx context.Context) (string, error) {
	tracePath := filepath.Join(x.taskOutputDir, "trace.yaml")

	args := []string{
		"--kubeconfig", x.kubeConfig,
		"--llm-provider", x.llmConfig.ProviderID,
		fmt.Sprintf("--enable-tool-use-shim=%t", x.llmConfig.EnableToolUseShim),
		fmt.Sprintf("--quiet=%t", x.llmConfig.Quiet),
		"--model", x.llmConfig.ModelID,
		"--trace-path", tracePath,
		"--skip-permissions",
		"--show-tool-output",
	}

	stdinReader, stdinWriter := io.Pipe()

	cmd := exec.CommandContext(ctx,
		x.AgentBin,
		args...,
	)
	cmd.Stdin = stdinReader
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	var stdoutBuffer bytes.Buffer
	if x.log != nil {
		cmd.Stdout = io.MultiWriter(cmd.Stdout, x.log, &stdoutBuffer)
		cmd.Stderr = io.MultiWriter(cmd.Stderr, x.log)
	}

	cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", x.kubeConfig))

	go func() {
		// TODO: Wait for idle between sending steps?
		for _, step := range x.task.Script {
			prompt, err := step.ResolvePrompt(x.taskDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error resolving prompt: %v\n", err)
				x.result.AddFailure("failed to resolve prompt: %v", err)
				stdinWriter.Close()
				return
			}
			fmt.Fprintf(stdinWriter, "%s\n", prompt)
		}
		stdinWriter.Close()
	}()

	if err := cmd.Run(); err != nil {
		return "", err
	}

	return stdoutBuffer.String(), nil
}

func (x *TaskExecution) runCommand(cmd *exec.Cmd) error {
	fmt.Printf("\nRunning command: %s\n", strings.Join(cmd.Args, " "))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if x.log != nil {
		cmd.Stdout = io.MultiWriter(cmd.Stdout, x.log)
		cmd.Stderr = io.MultiWriter(cmd.Stderr, x.log)
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running command %v: %w", strings.Join(cmd.Args, " "), err)
	}
	return nil
}

func printResults(allResults []model.TaskResult) {
	fmt.Println("\nEvaluation Results:")
	fmt.Println("==================")

	for _, result := range allResults {
		fmt.Printf("\nTask: %s\n", result.Task)
		fmt.Printf("  LLM Config: %+v\n", result.LLMConfig)
		fmt.Printf("    %v\n", result.Result)
		if result.Error != "" {
			fmt.Printf("    Error: %s\n", result.Error)
		}
	}
}

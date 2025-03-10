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
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/GoogleCloudPlatform/kubectl-ai/k8s-bench/pkg/model"
	"sigs.k8s.io/yaml"
)

func runEvaluation(ctx context.Context, config EvalConfig) error {
	tasks, err := loadTasks(config)
	if err != nil {
		return fmt.Errorf("failed to load tasks: %w", err)
	}

	var allResults []model.TaskResult

	for taskID, task := range tasks {
		fmt.Printf("Evaluating task: %s\n", taskID)

		for _, llmConfig := range config.LLMConfigs {
			taskOutputDir := ""
			if config.OutputDir != "" {
				taskOutputDir = filepath.Join(config.OutputDir, taskID, llmConfig.ID)
				if err := os.MkdirAll(taskOutputDir, 0755); err != nil {
					return fmt.Errorf("creating directory %q: %w", taskOutputDir, err)
				}
			}

			var log io.Writer
			if taskOutputDir != "" {
				logPath := filepath.Join(taskOutputDir, "log.txt")
				logFile, err := os.Create(logPath)
				if err != nil {
					return fmt.Errorf("creating log file %q: %w", logPath, err)
				}
				defer logFile.Close()
				log = logFile
			}

			result := evaluateTask(ctx, config, taskID, task, llmConfig, log)

			if taskOutputDir != "" {
				if err := writeToYAMLFile(filepath.Join(taskOutputDir, "results.yaml"), result); err != nil {
					return fmt.Errorf("writing results to file: %w", err)
				}
			}
			allResults = append(allResults, result)
		}
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

	entries, err := os.ReadDir(config.TasksDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		taskID := entry.Name()
		if config.TaskPattern != "" && !strings.Contains(taskID, config.TaskPattern) {
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

func evaluateTask(ctx context.Context, config EvalConfig, taskID string, task Task, llmConfig model.LLMConfig, log io.Writer) model.TaskResult {
	result := model.TaskResult{
		Task:      taskID,
		LLMConfig: llmConfig,
	}

	// Run setup if specified
	if task.Setup != "" {
		setupPath := filepath.Join(config.TasksDir, taskID, task.Setup)
		if err := runScript(ctx, setupPath, config.KubeConfig, log); err != nil {
			// Unexpected error
			result.Error = err.Error()
			return result
		}
	}

	// Run the agent
	{
		args := []string{
			"--kubeconfig", config.KubeConfig,
			"--llm-provider", llmConfig.ProviderID,
			"--strategy", llmConfig.Strategy,
			"--model", llmConfig.ModelID,
		}

		if config.OutputDir != "" {
			taskOutputDir := filepath.Join(config.OutputDir, taskID, llmConfig.ID)
			args = append(args, "--trace-path", filepath.Join(taskOutputDir, "trace.yaml"))
		}

		args = append(args, task.Goal)

		cmd := exec.CommandContext(ctx,
			config.AgentBin,
			args...,
		)

		cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", config.KubeConfig))
		fmt.Printf("\nRunning %s for task %s with %+v\n", config.AgentBin, taskID, llmConfig)

		if err := runCommand(cmd, log); err != nil {
			result.Result = "fail"
			result.Error = err.Error()
			return result
		}
	}

	// Run verifier if specified
	if task.Verifier != "" {
		verifierPath := filepath.Join(config.TasksDir, taskID, task.Verifier)
		cmd := exec.CommandContext(ctx, verifierPath)
		cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", config.KubeConfig))
		fmt.Printf("\nRunning verifier for task %s\n", taskID)

		err := runCommand(cmd, log)
		if err == nil {
			result.Result = "success"
		} else if _, ok := err.(*exec.ExitError); ok {
			// "Normal" script failure
			result.Result = "fail"
		} else {
			// Unexpected error
			result.Error = err.Error()
		}
	}

	// Run cleanup if specified
	if task.Cleanup != "" {
		cleanupPath := filepath.Join(config.TasksDir, taskID, task.Cleanup)
		if err := runScript(ctx, cleanupPath, config.KubeConfig, log); err != nil {
			fmt.Printf("Warning: cleanup failed for task %s: %v\n", taskID, err)
		}
	}

	return result
}

func runScript(ctx context.Context, path string, kubeconfig string, log io.Writer) error {
	cmd := exec.CommandContext(ctx, path)
	cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfig))
	fmt.Printf("\nRunning script: %s\n", path)
	return runCommand(cmd, log)
}

func runCommand(cmd *exec.Cmd, log io.Writer) error {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if log != nil {
		cmd.Stdout = io.MultiWriter(cmd.Stdout, log)
		cmd.Stderr = io.MultiWriter(cmd.Stderr, log)
	}
	return cmd.Run()
}

func printResults(allResults []model.TaskResult) {
	fmt.Println("\nEvaluation Results:")
	fmt.Println("==================")

	for _, result := range allResults {
		fmt.Printf("\nTask: %s\n", result.Task)
		fmt.Printf("  LLM Config: %+vv\n", result.LLMConfig)
		fmt.Printf("    %v\n", result.Result)
		if result.Error != "" {
			fmt.Printf("    Error: %s\n", result.Error)
		}
	}
}

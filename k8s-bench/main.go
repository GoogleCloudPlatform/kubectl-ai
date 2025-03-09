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
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GoogleCloudPlatform/kubectl-ai/k8s-bench/pkg/model"
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
	ctx := context.Background()
	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
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
	config := EvalConfig{
		TasksDir: "./tasks",
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
		return fmt.Errorf("Failed to expand kubeconfig path %q: %w", config.KubeConfig, err)
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

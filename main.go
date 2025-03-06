// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/journal"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/llmstrategy/agentic"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/llmstrategy/react"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/tools"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/ui"
	"k8s.io/klog/v2"
)

// models
var geminiModels = []string{
	"gemini-2.0-flash-thinking-exp-01-21",
}

func main() {
	ctx := context.Background()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		fmt.Fprintf(os.Stderr, "Received signal, shutting down... %s\n", sig)
		os.Exit(0)
	}()

	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	// Command line flags
	maxIterations := flag.Int("max-iterations", 20, "maximum number of iterations")
	kubeconfig := flag.String("kubeconfig", "", "path to the kubeconfig file")
	llmProvider := flag.String("llm-provider", "gemini", "language model provider")
	model := flag.String("model", geminiModels[0], "language model")
	templateFile := flag.String("prompt-template-path", "", "path to custom prompt template file")
	tracePath := flag.String("trace-path", "trace.log", "path to the trace file")
	removeWorkDir := flag.Bool("remove-workdir", false, "remove the temporary working directory after execution")

	// add commandline flags for logging
	klog.InitFlags(nil)

	flag.Set("logtostderr", "false") // disable logging to stderr
	flag.Set("log_file", "/tmp/kubectl-ai.log")

	flag.Parse()

	// Handle kubeconfig with priority: command-line arg > env var > default path
	kubeconfigPath := *kubeconfig
	if kubeconfigPath == "" {
		// Check environment variable
		kubeconfigPath = os.Getenv("KUBECONFIG")
		if kubeconfigPath == "" {
			// Use default path
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("error getting user home directory: %w", err)
			}
			kubeconfigPath = filepath.Join(homeDir, ".kube", "config")
		}
	}

	// Check for positional arguments (after all flags are parsed)
	args := flag.Args()
	var queryFromCmd string

	// Check if stdin has data (is not a terminal)
	stdinInfo, _ := os.Stdin.Stat()
	stdinHasData := (stdinInfo.Mode() & os.ModeCharDevice) == 0

	// Handle positional arguments and stdin
	if len(args) > 1 {
		return fmt.Errorf("only one positional argument (query) is allowed")
	} else if stdinHasData {
		// Read from stdin
		scanner := bufio.NewScanner(os.Stdin)
		var queryBuilder strings.Builder

		// If we have a positional argument, use it as a prefix
		if len(args) == 1 {
			queryBuilder.WriteString(args[0])
			queryBuilder.WriteString("\n")
		}

		// Read the rest from stdin
		for scanner.Scan() {
			queryBuilder.WriteString(scanner.Text())
			queryBuilder.WriteString("\n")
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("error reading from stdin: %w", err)
		}

		queryFromCmd = strings.TrimSpace(queryBuilder.String())
		if queryFromCmd == "" {
			return fmt.Errorf("no query provided from stdin")
		}
	} else if len(args) == 1 {
		// Just use the positional argument as the query
		queryFromCmd = args[0]
	}

	klog.Info("Application started", "pid", os.Getpid())

	var llmClient gollm.Client

	availableModels := []string{"unknown"}
	switch *llmProvider {
	case "gemini":
		geminiClient, err := gollm.NewGeminiClient(ctx)
		if err != nil {
			return fmt.Errorf("creating gemini client: %w", err)
		}
		defer geminiClient.Close()
		geminiClient.WithModel(*model)
		llmClient = geminiClient

		modelNames, err := geminiClient.ListModels(ctx)
		if err != nil {
			return fmt.Errorf("listing gemini models: %w", err)
		}
		availableModels = modelNames

	case "vertexai":
		vertexAIClient, err := gollm.NewVertexAIClient(ctx)
		if err != nil {
			return fmt.Errorf("creating vertexai client: %w", err)
		}
		defer vertexAIClient.Close()
		vertexAIClient.WithModel(*model)
		llmClient = vertexAIClient

	default:
		return fmt.Errorf("invalid language model provider: %s", *llmProvider)
	}

	u, err := ui.NewTerminalUI()
	if err != nil {
		return err
	}

	var recorder journal.Recorder
	if *tracePath != "" {
		fileRecorder, err := journal.NewFileRecorder(*tracePath)
		if err != nil {
			return fmt.Errorf("creating trace recorder: %w", err)
		}
		defer fileRecorder.Close()
		recorder = fileRecorder
	} else {
		// Ensure we always have a recorder, to avoid nil checks
		recorder = &journal.LogRecorder{}
		defer recorder.Close()
	}

	if queryFromCmd != "" {
		query := queryFromCmd

		strategy := &react.Strategy{
			Kubeconfig:       kubeconfigPath,
			ContentGenerator: llmClient,
			MaxIterations:    *maxIterations,
			TemplateFile:     *templateFile,
			Tools:            buildTools(),
			Recorder:         recorder,
			RemoveWorkDir:    *removeWorkDir,
		}
		agent := Agent{
			Model:    *model,
			Recorder: recorder,
			Strategy: strategy,
		}
		return agent.RunOnce(ctx, query, u)
	}

	chatSession := session{
		Queries: []string{},
		Model:   *model,
	}

	strategy := &agentic.Strategy{
		Recorder:  recorder,
		LLMClient: llmClient,
		Tools:     make(map[string]tools.Tool),
	}
	strategy.Tools["kubectl_get"] = &tools.KubectlGet{}
	if err := strategy.Init(ctx); err != nil {
		return err
	}

	u.RenderOutput(ctx, "Hey there, what can I help you with today?", ui.Foreground(ui.ColorRed))
	for {
		u.RenderOutput(ctx, "\n>> ")
		reader := bufio.NewReader(os.Stdin)
		query, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("reading input: %w", err)
		}
		query = strings.TrimSpace(query)

		if query == "" {
			continue
		}
		switch query {
		case "reset":
			chatSession.Queries = []string{}
			u.ClearScreen()
		case "clear":
			u.ClearScreen()
		case "exit", "quit":
			u.RenderOutput(ctx, "Allright...bye.")
			return nil
		case "models":
			u.RenderOutput(ctx, "Available models:")
			for _, modelName := range availableModels {
				u.RenderOutput(ctx, modelName)
			}
		default:
			if strings.HasPrefix(query, "model") {
				parts := strings.Split(query, " ")
				if len(parts) > 2 {
					u.RenderOutput(ctx, "Invalid model command. expected format: model <model-name>", ui.Foreground(ui.ColorRed))
					continue
				}
				if len(parts) == 1 {
					u.RenderOutput(ctx, fmt.Sprintf("Current model is `%s`\n", chatSession.Model), ui.RenderMarkdown())
					continue
				}
				chatSession.Model = parts[1]
				u.RenderOutput(ctx, fmt.Sprintf("Model set to `%s`\n", chatSession.Model), ui.RenderMarkdown())
				continue
			}

			// strategy := &react.Strategy{
			// 	ContentGenerator: llmClient,
			// 	MaxIterations:    *maxIterations,
			// 	TemplateFile:     *templateFile,
			// 	Tools:            buildTools(),
			// 	Recorder:         recorder,
			// 	RemoveWorkDir:    *removeWorkDir,
			// 	PastQueries:      chatSession.PreviousQueries(),
			// 	Kubeconfig:       kubeconfigPath,
			// }

			agent := Agent{
				Model:    chatSession.Model,
				Recorder: recorder,
				Strategy: strategy,
			}
			if err := agent.RunOnce(ctx, query, u); err != nil {
				return err
			}
			chatSession.Queries = append(chatSession.Queries, query)
		}
	}
}

// session represents each the chat session.
type session struct {
	Queries []string `json:"queries"`
	Model   string   `json:"model"`
}

func (s *session) PreviousQueries() string {
	return strings.Join(s.Queries, "\n")
}

func (a *Agent) RunOnce(ctx context.Context, query string, userInterface ui.UI) error {
	return a.Strategy.RunOnce(ctx, query, userInterface)
}

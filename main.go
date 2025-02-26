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
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/journal"
	"github.com/charmbracelet/glamour"
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
	// non interactive execution when query is specified on the command line.
	queryFromCmd := flag.String("query", "", "query for the agent")
	maxIterations := flag.Int("max-iterations", 20, "maximum number of iterations")
	kubeconfig := flag.String("kubeconfig", "", "path to the kubeconfig file")
	llmProvider := flag.String("llm-provider", "gemini", "language model provider")
	model := flag.String("model", geminiModels[0], "language model")
	templateFile := flag.String("prompt-template-path", "", "path to custom prompt template file")
	tracePath := flag.String("trace-path", "trace.log", "path to the trace file")
	removeWorkDir := flag.Bool("remove-workdir", false, "remove the temporary working directory after execution")
	flag.Parse()

	logFile, err := os.OpenFile("app.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("error opening log file: %w", err)
	}
	defer logFile.Close()

	textHandler := slog.NewTextHandler(logFile, nil)
	logger := slog.New(textHandler)

	mdRenderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithPreservedNewLines(),
		glamour.WithEmoji(),
	)
	if err != nil {
		return fmt.Errorf("error initializing the markdown renderer: %w", err)
	}

	logger.Info("Application started", "pid", os.Getpid())

	ctx = withLogger(ctx, logger)

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

	var recorder journal.Recorder
	if *tracePath != "" {
		recorder, err = journal.NewFileRecorder(*tracePath)
		if err != nil {
			return fmt.Errorf("creating trace recorder: %w", err)
		}
		defer recorder.Close()
	}

	if *queryFromCmd != "" {
		query := *queryFromCmd

		agent := Agent{
			Model:            *model,
			Query:            query,
			ContentGenerator: llmClient,
			MaxIterations:    *maxIterations,
			Kubeconfig:       *kubeconfig,
			RemoveWorkDir:    *removeWorkDir,
			templateFile:     *templateFile,
			markdownRenderer: mdRenderer,
			Recorder:         recorder,
		}
		agent.Execute(ctx)
		return nil
	}

	chatSession := session{
		Queries: []string{},
		Model:   *model,
	}

	fmt.Printf("\033[31mHey there, what can I help you with today?\033[0m")
	for {
		fmt.Printf("\n>> ")
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
			clearScreen()
		case "clear":
			clearScreen()
		case "exit", "quit":
			fmt.Println("Allright...bye.")
			return nil
		case "models":

			fmt.Println("Available models:")
			for _, modelName := range availableModels {
				fmt.Println(modelName)
			}
		default:
			if strings.HasPrefix(query, "model") {
				parts := strings.Split(query, " ")
				if len(parts) > 2 {
					fmt.Println("Invalid model command. expected format: model <model-name>")
					continue
				}
				if len(parts) == 1 {
					out := fmt.Sprintf("Current model is `%s`\n", chatSession.Model)
					rendered, _ := mdRenderer.Render(out)
					fmt.Println(rendered)
					continue
				}
				chatSession.Model = parts[1]
				fmt.Printf("Model set to `%s`\n", chatSession.Model)
				continue
			}
			agent := Agent{
				Model:            chatSession.Model,
				Query:            query,
				PastQueries:      chatSession.PreviousQueries(),
				ContentGenerator: llmClient,
				MaxIterations:    *maxIterations,
				Kubeconfig:       *kubeconfig,
				RemoveWorkDir:    *removeWorkDir,
				templateFile:     *templateFile,
				markdownRenderer: mdRenderer,
				Recorder:         recorder,
			}
			agent.Execute(ctx)
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

// Logger key for context
type loggerKey struct{}

// Function to create a new context with the logger.
func withLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

// Function to extract the logger from the context.
func loggerFromContext(ctx context.Context) *slog.Logger {
	logger, ok := ctx.Value(loggerKey{}).(*slog.Logger)
	if !ok {
		return slog.Default()
	}
	return logger
}

func clearScreen() {
	fmt.Print("\033[H\033[2J")
}

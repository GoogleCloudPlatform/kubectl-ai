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
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/agent"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/journal"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/tools"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/ui"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"
)

// Version will be set at build time
var Version = "0.1.0-dev"

// models
var geminiModels = []string{
	"gemini-2.0-flash-thinking-exp-01-21",
	"gemini-2.0-flash",
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

type Options struct {
	ProviderID string `json:"llmProvider,omitempty"`
	ModelID    string `json:"model,omitempty"`

	// AsksForConfirmation is a flag to ask for confirmation before executing kubectl commands
	// that modifies resources in the cluster.
	AsksForConfirmation bool `json:"askForConfirmation,omitempty"`

	// EnableToolUseShim is a flag to enable tool use shim.
	// TODO(droot): figure out a better way to discover if the model supports tool use
	// and set this automatically.
	EnableToolUseShim bool `json:"enableToolUseShim,omitempty"`

	MCPServer bool
}

func (o *Options) InitDefaults() {
	// default to react because default model doesn't support function calling.
	o.ProviderID = "gemini"
	o.ModelID = geminiModels[0]
	// default to false because our goal is to make the agent truly autonomous by default
	o.AsksForConfirmation = false
	o.MCPServer = false
	o.EnableToolUseShim = true
}

func (o *Options) LoadConfiguration(b []byte) error {
	if err := yaml.Unmarshal(b, &o); err != nil {
		return fmt.Errorf("parsing configuration: %w", err)
	}
	return nil
}

func (o *Options) LoadConfigurationFile() error {
	configPaths := []string{
		"{CONFIG}/kubectl-ai/config.yaml",
		"{HOME}/.config/kubectl-ai/config.yaml",
	}

	for _, configPath := range configPaths {
		// Try to load configuration
		tokens := strings.Split(configPath, "/")
		for i, token := range tokens {
			if token == "{CONFIG}" {
				configDir, err := os.UserConfigDir()
				if err != nil {
					return fmt.Errorf("getting user config directory: %w", err)
				}
				tokens[i] = configDir
			}
			if token == "{HOME}" {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("getting user home directory: %w", err)
				}
				tokens[i] = homeDir
			}
		}
		configPath = filepath.Join(tokens...)
		configBytes, err := os.ReadFile(configPath)
		if err != nil {
			if os.IsNotExist(err) {
				// ignore
			} else {
				fmt.Fprintf(os.Stderr, "warning: could not load defaults from %q: %v\n", configPath, err)
			}
		}
		if len(configBytes) > 0 {
			if err := o.LoadConfiguration(configBytes); err != nil {
				fmt.Fprintf(os.Stderr, "warning: error loading configuration from %q: %v\n", configPath, err)
			}
		}
	}

	return nil
}

func run(ctx context.Context) error {
	// Command line flags
	var opt Options
	opt.InitDefaults()

	if err := opt.LoadConfigurationFile(); err != nil {
		return fmt.Errorf("loading configuration file: %w", err)
	}

	maxIterations := flag.Int("max-iterations", 20, "maximum number of iterations agent will try before giving up")
	kubeconfig := flag.String("kubeconfig", "", "path to the kubeconfig file")
	promptTemplateFile := flag.String("prompt-template-file", "", "path to custom prompt template file")
	tracePath := flag.String("trace-path", "trace.log", "path to the trace file")
	removeWorkDir := flag.Bool("remove-workdir", false, "remove the temporary working directory after execution")

	flag.StringVar(&opt.ProviderID, "llm-provider", opt.ProviderID, "language model provider")
	flag.StringVar(&opt.ModelID, "model", opt.ModelID, "language model e.g. gemini-2.0-flash-thinking-exp-01-21, gemini-2.0-flash")
	flag.BoolVar(&opt.AsksForConfirmation, "ask-for-confirmation", opt.AsksForConfirmation, "ask for confirmation before executing kubectl commands that modify resources")
	flag.BoolVar(&opt.MCPServer, "mcp-server", opt.MCPServer, "run in MCP server mode")
	flag.BoolVar(&opt.EnableToolUseShim, "enable-tool-use-shim", opt.EnableToolUseShim, "enable tool use shim")
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
	if opt.MCPServer {
		workDir := "/tmp/kubectl-ai-mcp"
		if err := os.MkdirAll(workDir, 0755); err != nil {
			return fmt.Errorf("error creating work directory: %w", err)
		}
		mcpServer, err := newKubectlMCPServer(ctx, kubeconfigPath, tools.Default(), workDir)
		if err != nil {
			return fmt.Errorf("creating mcp server: %w", err)
		}
		return mcpServer.Serve(ctx)
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
	var err error

	availableModels := []string{"unknown"}
	switch opt.ProviderID {
	case "gemini":
		geminiClient, err := gollm.NewGeminiClient(ctx)
		if err != nil {
			return fmt.Errorf("creating gemini client: %w", err)
		}
		defer geminiClient.Close()
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
		llmClient = vertexAIClient

	case "ollama":
		ollamaClient, err := gollm.NewOllamaClient(ctx)
		if err != nil {
			return fmt.Errorf("creating ollama client: %w", err)
		}
		defer ollamaClient.Close()
		llmClient = ollamaClient

		modelNames, err := ollamaClient.ListModels(ctx)
		if err != nil {
			return fmt.Errorf("listing ollama models: %w", err)
		}
		availableModels = modelNames
	case "llamacpp":
		llamacppClient, err := gollm.NewLlamaCppClient(ctx)
		if err != nil {
			return fmt.Errorf("creating llama.cpp client: %w", err)
		}
		defer llamacppClient.Close()
		llmClient = llamacppClient

		// Does not support models
		availableModels = nil
	default:
		return fmt.Errorf("invalid language model provider: %s", opt.ProviderID)
	}

	err = llmClient.SetModel(opt.ModelID)
	if err != nil {
		return fmt.Errorf("setting model: %w", err)
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

	u, err := ui.NewTerminalUI(recorder)
	if err != nil {
		return err
	}

	conversation := &agent.Conversation{
		Kubeconfig:          kubeconfigPath,
		LLM:                 llmClient,
		MaxIterations:       *maxIterations,
		PromptTemplateFile:  *promptTemplateFile,
		Tools:               tools.Default(),
		Recorder:            recorder,
		RemoveWorkDir:       *removeWorkDir,
		AsksForConfirmation: opt.AsksForConfirmation,
		EnableToolUseShim:   opt.EnableToolUseShim,
	}

	err = conversation.Init(ctx, u)
	if err != nil {
		return fmt.Errorf("starting conversation: %w", err)
	}
	defer conversation.Close()

	if queryFromCmd != "" {
		query := queryFromCmd

		return conversation.RunOneRound(ctx, query)
	}

	chatSession := session{
		Model: opt.ModelID,
	}

	u.RenderOutput(ctx, "Hey there, what can I help you with today?\n", ui.Foreground(ui.ColorRed))
	for {
		u.RenderOutput(ctx, "\n>> ")
		reader := bufio.NewReader(os.Stdin)
		query, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				// Use hit control-D, or was piping and we reached the end of stdin.
				// Not a "big" problem
				return nil
			}
			return fmt.Errorf("reading input: %w", err)
		}
		query = strings.TrimSpace(query)

		switch {
		case query == "":
			continue
		case query == "reset":
			err = conversation.Init(ctx, u)
			if err != nil {
				return err
			}
			u.ClearScreen()
		case query == "clear":
			u.ClearScreen()
		case query == "exit" || query == "quit":
			u.RenderOutput(ctx, "Allright...bye.\n")
			return nil
		case query == "models":
			u.RenderOutput(ctx, "\n  Available models:\n", ui.Foreground(ui.ColorGreen))
			u.RenderOutput(ctx, strings.Join(availableModels, "\n"), ui.RenderMarkdown())
		case strings.HasPrefix(query, "model"):
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
			_ = llmClient.SetModel(chatSession.Model)
			u.RenderOutput(ctx, fmt.Sprintf("Model set to `%s`\n", chatSession.Model), ui.RenderMarkdown())
		case query == "version":
			u.RenderOutput(ctx, fmt.Sprintf("Client version: `%s`\n", Version), ui.RenderMarkdown())
		default:
			if err := conversation.RunOneRound(ctx, query); err != nil {
				return err
			}
		}
	}
}

// session represents each the chat session.
type session struct {
	Model string `json:"model"`
}

type kubectlMCPServer struct {
	kubectlConfig string
	server        *server.MCPServer
	tools         tools.Tools
	workDir       string
}

func newKubectlMCPServer(ctx context.Context, kubectlConfig string, tools tools.Tools, workDir string) (*kubectlMCPServer, error) {
	s := &kubectlMCPServer{
		kubectlConfig: kubectlConfig,
		workDir:       workDir,
		server: server.NewMCPServer(
			"kubectl-ai",
			"0.0.1",
			server.WithToolCapabilities(true),
		),
		tools: tools,
	}
	for _, tool := range s.tools.AllTools() {
		toolDefn := tool.FunctionDefinition()
		s.server.AddTool(mcp.NewTool(
			toolDefn.Name,
			mcp.WithDescription(toolDefn.Description),
			mcp.WithString("command", mcp.Description(toolDefn.Parameters.Properties["command"].Description)),
			mcp.WithString("modifies_resource", mcp.Description(toolDefn.Parameters.Properties["modifies_resource"].Description)),
		), s.handleToolCall)
	}
	return s, nil
}
func (s *kubectlMCPServer) Serve(ctx context.Context) error {
	return server.ServeStdio(s.server)
}

func (s *kubectlMCPServer) handleToolCall(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {

	log := klog.FromContext(ctx)

	name := request.Params.Name
	command := request.Params.Arguments["command"].(string)
	modifiesResource := request.Params.Arguments["modifies_resource"].(string)
	log.Info("Received tool call", "tool", name, "command", command, "modifies_resource", modifiesResource)

	tool := tools.Lookup(name)
	if tool == nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Error: Tool %s not found", name),
				},
			},
		}, nil
	}

	opt := &tools.ExecutionOptions{
		FunctionArguments: map[string]any{
			"command": command,
		},
		WorkDir:    s.workDir,
		Kubeconfig: s.kubectlConfig,
	}
	output, err := tool.Run(ctx, opt)
	if err != nil {
		log.Error(err, "Error running tool call")
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Error: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	log.Info("Tool call output", "tool", name, "output", output)

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: output.(string),
			},
		},
	}, nil
}

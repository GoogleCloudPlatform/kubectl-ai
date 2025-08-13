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
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/agent"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/journal"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/sessions"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/tools"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/ui"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/ui/html"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"
)

// Using the defaults from goreleaser as per https://goreleaser.com/cookbooks/using-main.version/
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func BuildRootCommand(opt *Options) (*cobra.Command, error) {
	rootCmd := &cobra.Command{
		Use:   "kubectl-ai",
		Short: "A CLI tool to interact with Kubernetes using natural language",
		Long:  "kubectl-ai is a command-line tool that allows you to interact with your Kubernetes cluster using natural language queries. It leverages large language models to understand your intent and translate it into kubectl",
		Args:  cobra.MaximumNArgs(1), // Only one positional arg is allowed.
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunRootCommand(cmd.Context(), *opt, args)
		},
	}

	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the version number of kubectl-ai",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("version: %s\ncommit: %s\ndate: %s\n", version, commit, date)
			os.Exit(0)
		},
	})

	if err := opt.bindCLIFlags(rootCmd.Flags()); err != nil {
		return nil, err
	}
	return rootCmd, nil
}

type Options struct {
	ProviderID string `json:"llmProvider,omitempty"`
	ModelID    string `json:"model,omitempty"`
	// SkipPermissions is a flag to skip asking for confirmation before executing kubectl commands
	// that modifies resources in the cluster.
	SkipPermissions bool `json:"skipPermissions,omitempty"`
	// EnableToolUseShim is a flag to enable tool use shim.
	// TODO(droot): figure out a better way to discover if the model supports tool use
	// and set this automatically.
	EnableToolUseShim bool `json:"enableToolUseShim,omitempty"`
	// Quiet flag indicates if the agent should run in non-interactive mode.
	// It requires a query to be provided as a positional argument.
	Quiet     bool `json:"quiet,omitempty"`
	MCPServer bool `json:"mcpServer,omitempty"`
	MCPClient bool `json:"mcpClient,omitempty"`
	// ExternalTools enables discovery and exposure of external MCP tools (only works with --mcp-server)
	ExternalTools bool `json:"externalTools,omitempty"`
	MaxIterations int  `json:"maxIterations,omitempty"`
	// MCPServerMode is the mode of the MCP server. only works with --mcp-server.
	MCPServerMode string `json:"mcpServerMode,omitempty"`
	// Set the SSEndpoint port for the MCP server. only works with --mcp-server and --mcp-server-mode=sse.
	SSEndpointPort int `json:"sseEndpointPort,omitempty"`
	// KubeConfigPath is the path to the kubeconfig file.
	// If not provided, the default kubeconfig path will be used.
	KubeConfigPath string `json:"kubeConfigPath,omitempty"`

	PromptTemplateFilePath string   `json:"promptTemplateFilePath,omitempty"`
	ExtraPromptPaths       []string `json:"extraPromptPaths,omitempty"`
	TracePath              string   `json:"tracePath,omitempty"`
	RemoveWorkDir          bool     `json:"removeWorkDir,omitempty"`
	ToolConfigPaths        []string `json:"toolConfigPaths,omitempty"`

	// UIType is the type of user interface to use.
	UIType ui.Type `json:"uiType,omitempty"`
	// UIListenAddress is the address to listen for the web UI.
	UIListenAddress string `json:"uiListenAddress,omitempty"`

	// SkipVerifySSL is a flag to skip verifying the SSL certificate of the LLM provider.
	SkipVerifySSL bool `json:"skipVerifySSL,omitempty"`

	// Session management options
	ResumeSession string `json:"resumeSession,omitempty"`
	NewSession    bool   `json:"newSession,omitempty"`
	ListSessions  bool   `json:"listSessions,omitempty"`
	DeleteSession string `json:"deleteSession,omitempty"`

	// ShowToolOutput is a flag to disable truncation of tool output in the terminal UI.
	ShowToolOutput bool `json:"showToolOutput,omitempty"`
}

var defaultToolConfigPaths = []string{
	filepath.Join("{CONFIG}", "kubectl-ai", "tools.yaml"),
	filepath.Join("{HOME}", ".config", "kubectl-ai", "tools.yaml"),
}

var defaultConfigPaths = []string{
	filepath.Join("{CONFIG}", "kubectl-ai", "config.yaml"),
	filepath.Join("{HOME}", ".config", "kubectl-ai", "config.yaml"),
}

func (o *Options) InitDefaults() {
	o.ProviderID = "gemini"
	o.ModelID = "gemini-2.5-pro"
	// by default, confirm before executing kubectl commands that modify resources in the cluster.
	o.SkipPermissions = false
	o.MCPServer = false
	o.MCPClient = false
	// by default, external tools are disabled (only works with --mcp-server)
	o.ExternalTools = false
	// We now default to our strongest model (gemini-2.5-pro-exp-03-25) which supports tool use natively.
	// so we don't need shim.
	o.EnableToolUseShim = false
	o.Quiet = false
	o.MCPServer = false
	o.MaxIterations = 20
	o.KubeConfigPath = ""
	o.PromptTemplateFilePath = ""
	o.ExtraPromptPaths = []string{}
	o.TracePath = filepath.Join(os.TempDir(), "kubectl-ai-trace.txt")
	o.RemoveWorkDir = false
	o.ToolConfigPaths = defaultToolConfigPaths
	// Default to terminal UI
	o.UIType = ui.UITypeTerminal
	// Default UI listen address for HTML UI
	o.UIListenAddress = "localhost:8888"
	// Default to not skipping SSL verification
	o.SkipVerifySSL = false
	// Default MCP server mode is stdio
	o.MCPServerMode = "stdio"
	// Default port for SSE endpoint
	o.SSEndpointPort = 9080

	// Session management options
	o.ResumeSession = ""
	o.NewSession = false
	o.ListSessions = false
	o.DeleteSession = ""

	// By default, hide tool outputs
	o.ShowToolOutput = false
}

func (o *Options) LoadConfiguration(b []byte) error {
	if err := yaml.Unmarshal(b, &o); err != nil {
		return fmt.Errorf("parsing configuration: %w", err)
	}
	return nil
}

func (o *Options) LoadConfigurationFile() error {
	configPaths := defaultConfigPaths
	for _, configPath := range configPaths {
		pathWithPlaceholdersExpanded := configPath

		if strings.Contains(pathWithPlaceholdersExpanded, "{CONFIG}") {
			configDir, err := os.UserConfigDir()
			if err != nil {
				return fmt.Errorf("getting user config directory (for config file path %q): %w", configPath, err)
			}
			pathWithPlaceholdersExpanded = strings.ReplaceAll(pathWithPlaceholdersExpanded, "{CONFIG}", configDir)
		}

		if strings.Contains(pathWithPlaceholdersExpanded, "{HOME}") {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("getting user home directory (for config file path %q): %w", configPath, err)
			}
			pathWithPlaceholdersExpanded = strings.ReplaceAll(pathWithPlaceholdersExpanded, "{HOME}", homeDir)
		}

		configPath = filepath.Clean(pathWithPlaceholdersExpanded)
		configBytes, err := os.ReadFile(configPath)
		if err != nil {
			if os.IsNotExist(err) {
				// ignore missing config files, they are optional
			} else {
				fmt.Fprintf(os.Stderr, "warning: could not load defaults from %q: %v\n", configPath, err)
			}
		} else if len(configBytes) > 0 {
			if err := o.LoadConfiguration(configBytes); err != nil {
				fmt.Fprintf(os.Stderr, "warning: error loading configuration from %q: %v\n", configPath, err)
			}
		}
	}
	return nil
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go func() {
		<-ctx.Done()
		// restore default behavior for a second signal
		signal.Stop(make(chan os.Signal))
		cancel()
		klog.Flush()
		fmt.Fprintf(os.Stderr, "\nReceived signal, shutting down gracefully... (press Ctrl+C again to force)\n")
	}()

	if err := run(ctx); err != nil {
		// Don't print error if it's a context cancellation
		if !errors.Is(err, context.Canceled) {
			fmt.Fprintln(os.Stderr, err)
		}
		// Exit with non-zero status code on error, unless it's a graceful shutdown.
		if errors.Is(err, context.Canceled) {
			os.Exit(0)
		}
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	// klog setup must happen before Cobra parses any flags

	// add commandline flags for logging
	klogFlags := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(klogFlags)

	klogFlags.Set("logtostderr", "false")
	klogFlags.Set("log_file", filepath.Join(os.TempDir(), "kubectl-ai.log"))

	defer klog.Flush()

	var opt Options

	opt.InitDefaults()

	// load YAML config values
	if err := opt.LoadConfigurationFile(); err != nil {
		return fmt.Errorf("failed to load config file: %w", err)
	}

	rootCmd, err := BuildRootCommand(&opt)
	if err != nil {
		return err
	}

	// cobra has to know that we pass pass flags with flag lib, otherwise it creates conflict with flags.parse() method
	// We add just the klog flags we want, not all the klog flags (there are a lot, most of them are very niche)
	rootCmd.PersistentFlags().AddGoFlag(klogFlags.Lookup("v"))
	rootCmd.PersistentFlags().AddGoFlag(klogFlags.Lookup("alsologtostderr"))

	// do this early, before the third-party code logs anything.
	redirectStdLogToKlog()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		return err
	}

	return nil
}

func (opt *Options) bindCLIFlags(f *pflag.FlagSet) error {
	f.IntVar(&opt.MaxIterations, "max-iterations", opt.MaxIterations, "maximum number of iterations agent will try before giving up")
	f.StringVar(&opt.KubeConfigPath, "kubeconfig", opt.KubeConfigPath, "path to kubeconfig file")
	f.StringVar(&opt.PromptTemplateFilePath, "prompt-template-file-path", opt.PromptTemplateFilePath, "path to custom prompt template file")
	f.StringArrayVar(&opt.ExtraPromptPaths, "extra-prompt-paths", opt.ExtraPromptPaths, "extra prompt template paths")
	f.StringVar(&opt.TracePath, "trace-path", opt.TracePath, "path to the trace file")
	f.BoolVar(&opt.RemoveWorkDir, "remove-workdir", opt.RemoveWorkDir, "remove the temporary working directory after execution")

	f.StringVar(&opt.ProviderID, "llm-provider", opt.ProviderID, "language model provider")
	f.StringVar(&opt.ModelID, "model", opt.ModelID, "language model e.g. gemini-2.0-flash-thinking-exp-01-21, gemini-2.0-flash")
	f.BoolVar(&opt.SkipPermissions, "skip-permissions", opt.SkipPermissions, "(dangerous) skip asking for confirmation before executing kubectl commands that modify resources")
	f.BoolVar(&opt.MCPServer, "mcp-server", opt.MCPServer, "run in MCP server mode")
	f.BoolVar(&opt.ExternalTools, "external-tools", opt.ExternalTools, "in MCP server mode, discover and expose external MCP tools")
	f.StringArrayVar(&opt.ToolConfigPaths, "custom-tools-config", opt.ToolConfigPaths, "path to custom tools config file or directory")
	f.BoolVar(&opt.MCPClient, "mcp-client", opt.MCPClient, "enable MCP client mode to connect to external MCP servers")
	f.StringVar(&opt.MCPServerMode, "mcp-server-mode", opt.MCPServerMode, "mode of the MCP server. Supported values: stdio, sse")
	f.IntVar(&opt.SSEndpointPort, "sse-endpoint-port", opt.SSEndpointPort, "port for the SSE endpoint in MCP server mode (only works with --mcp-server and --mcp-server-mode=sse)")
	f.BoolVar(&opt.EnableToolUseShim, "enable-tool-use-shim", opt.EnableToolUseShim, "enable tool use shim")
	f.BoolVar(&opt.Quiet, "quiet", opt.Quiet, "run in non-interactive mode, requires a query to be provided as a positional argument")

	f.Var(&opt.UIType, "ui-type", "user interface type to use. Supported values: terminal, web, tui.")
	f.StringVar(&opt.UIListenAddress, "ui-listen-address", opt.UIListenAddress, "address to listen for the HTML UI.")
	f.BoolVar(&opt.SkipVerifySSL, "skip-verify-ssl", opt.SkipVerifySSL, "skip verifying the SSL certificate of the LLM provider")
	f.BoolVar(&opt.ShowToolOutput, "show-tool-output", opt.ShowToolOutput, "show tool output in the terminal UI")

	f.StringVar(&opt.ResumeSession, "resume-session", opt.ResumeSession, "ID of session to resume (use 'latest' for the most recent session)")
	f.BoolVar(&opt.NewSession, "new-session", opt.NewSession, "create a new session")
	f.BoolVar(&opt.ListSessions, "list-sessions", opt.ListSessions, "list all available sessions")
	f.StringVar(&opt.DeleteSession, "delete-session", opt.DeleteSession, "delete a session by ID")

	return nil
}

func RunRootCommand(ctx context.Context, opt Options, args []string) error {
	var err error // Declare err once for the whole function

	// Validate flag combinations
	if opt.ExternalTools && !opt.MCPServer {
		return fmt.Errorf("--external-tools can only be used with --mcp-server")
	}

	// resolve kubeconfig path with priority: flag/env > KUBECONFIG > default path
	if err = resolveKubeConfigPath(&opt); err != nil {
		return fmt.Errorf("failed to resolve kubeconfig path: %w", err)
	}

	if opt.MCPServer {
		if err = startMCPServer(ctx, opt); err != nil {
			return fmt.Errorf("failed to start MCP server: %w", err)
		}
		return nil // MCP server mode blocks, so we return here
	}

	if opt.ListSessions {
		return handleListSessions()
	}

	if opt.DeleteSession != "" {
		return handleDeleteSession(opt.DeleteSession)
	}

	if err := handleCustomTools(opt.ToolConfigPaths); err != nil {
		return fmt.Errorf("failed to process custom tools: %w", err)
	}

	// After reading stdin, it is consumed
	var hasInputData bool
	hasInputData, err = hasStdInData()
	if err != nil {
		return fmt.Errorf("failed to check if stdin has data: %w", err)
	}

	// Handles positional args or stdin
	var queryFromCmd string
	queryFromCmd, err = resolveQueryInput(hasInputData, args)
	if err != nil {
		return fmt.Errorf("failed to resolve query input %w", err)
	}

	klog.Info("Application started", "pid", os.Getpid())

	var llmClient gollm.Client
	if opt.SkipVerifySSL {
		llmClient, err = gollm.NewClient(ctx, opt.ProviderID, gollm.WithSkipVerifySSL())
	} else {
		llmClient, err = gollm.NewClient(ctx, opt.ProviderID)
	}
	if err != nil {
		return fmt.Errorf("creating llm client: %w", err)
	}
	defer llmClient.Close()

	// Initialize session management
	var chatStore api.ChatMessageStore
	var sessionManager *sessions.SessionManager

	// TODO: Remove this when session persistence is default
	if opt.NewSession || opt.ResumeSession != "" {
		sessionManager, err = sessions.NewSessionManager()
		if err != nil {
			return fmt.Errorf("failed to create session manager: %w", err)
		}

		// Handle session creation or loading
		if opt.NewSession {
			// Create a new session
			meta := sessions.Metadata{
				ProviderID: opt.ProviderID,
				ModelID:    opt.ModelID,
			}
			chatStore, err = sessionManager.NewSession(meta)
			if err != nil {
				return fmt.Errorf("failed to create a new session: %w", err)
			}
			klog.Infof("Created new session: %s\n", chatStore.(*sessions.Session).ID)
		} else {
			// Load existing session
			var sessionID string
			if opt.ResumeSession == "" || opt.ResumeSession == "latest" {
				// Get the latest session
				chatStore, err = sessionManager.GetLatestSession()
				if err != nil {
					return fmt.Errorf("failed to get latest session: %w", err)
				}
				if chatStore == nil {
					// No sessions exist, create a new one
					meta := sessions.Metadata{
						ProviderID: opt.ProviderID,
						ModelID:    opt.ModelID,
					}
					chatStore, err = sessionManager.NewSession(meta)
					if err != nil {
						return fmt.Errorf("failed to create new session: %w", err)
					}
					klog.Infof("Created new session: %s\n", chatStore.(*sessions.Session).ID)
				}
			} else {
				sessionID = opt.ResumeSession
				chatStore, err = sessionManager.FindSessionByID(sessionID)
				if err != nil {
					return fmt.Errorf("session %s not found: %w", sessionID, err)
				}
			}

			if chatStore != nil {
				// Update last accessed time
				if err := chatStore.(*sessions.Session).UpdateLastAccessed(); err != nil {
					klog.Warningf("Failed to update session last accessed time: %v", err)
				}
			}
		}
	} else {
		chatStore = sessions.NewInMemoryChatStore()
	}

	var recorder journal.Recorder
	if opt.TracePath != "" {
		var fileRecorder journal.Recorder
		fileRecorder, err = journal.NewFileRecorder(opt.TracePath)
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

	k8sAgent := &agent.Agent{
		Model:              opt.ModelID,
		Kubeconfig:         opt.KubeConfigPath,
		LLM:                llmClient,
		MaxIterations:      opt.MaxIterations,
		PromptTemplateFile: opt.PromptTemplateFilePath,
		ExtraPromptPaths:   opt.ExtraPromptPaths,
		Tools:              tools.Default(),
		Recorder:           recorder,
		RemoveWorkDir:      opt.RemoveWorkDir,
		SkipPermissions:    opt.SkipPermissions,
		EnableToolUseShim:  opt.EnableToolUseShim,
		MCPClientEnabled:   opt.MCPClient,
		RunOnce:            opt.Quiet,
		InitialQuery:       queryFromCmd,
		ChatMessageStore:   chatStore,
	}

	err = k8sAgent.Init(ctx)
	if err != nil {
		return fmt.Errorf("starting k8s agent: %w", err)
	}
	defer k8sAgent.Close()

	var userInterface ui.UI
	switch opt.UIType {
	case ui.UITypeTerminal:
		// since stdin is already consumed, we use TTY for taking input from user
		useTTYForInput := hasInputData
		userInterface, err = ui.NewTerminalUI(k8sAgent, useTTYForInput, opt.ShowToolOutput, recorder)
		if err != nil {
			return fmt.Errorf("creating terminal UI: %w", err)
		}
	case ui.UITypeWeb:
		userInterface, err = html.NewHTMLUserInterface(k8sAgent, opt.UIListenAddress, recorder)
		if err != nil {
			return fmt.Errorf("creating web UI: %w", err)
		}
	case ui.UITypeTUI:
		userInterface = ui.NewTUI(k8sAgent)
	default:
		return fmt.Errorf("user-interface mode %q is not known", opt.UIType)
	}

	return repl(ctx, queryFromCmd, userInterface, k8sAgent)
}

func handleCustomTools(toolConfigPaths []string) error {
	// resolve tool config paths, and then load and register custom tools from config files and dirs
	for _, path := range toolConfigPaths {
		pathWithPlaceholdersExpanded := path

		if strings.Contains(pathWithPlaceholdersExpanded, "{CONFIG}") {
			configDir, err := os.UserConfigDir()
			if err != nil {
				klog.Warningf("Failed to get user config directory for tools path %q: %v", path, err)
				continue
			}
			pathWithPlaceholdersExpanded = strings.ReplaceAll(pathWithPlaceholdersExpanded, "{CONFIG}", configDir)
		}

		if strings.Contains(pathWithPlaceholdersExpanded, "{HOME}") {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				klog.Warningf("Failed to get user home directory for tools path %q: %v", path, err)
				continue
			}
			pathWithPlaceholdersExpanded = strings.ReplaceAll(pathWithPlaceholdersExpanded, "{HOME}", homeDir)
		}

		cleanedPath := filepath.Clean(pathWithPlaceholdersExpanded)

		klog.Infof("Attempting to load custom tools from processed path: %q (original value from config: %q)", cleanedPath, path)

		if err := tools.LoadAndRegisterCustomTools(cleanedPath); err != nil {
			if errors.Is(err, os.ErrNotExist) && !slices.Contains(defaultToolConfigPaths, path) {
				// user specified a directory that does not exist, we must error out
				return fmt.Errorf("custom tools directory not found (original value: %q, processed path: %q)", path, cleanedPath)
			} else {
				klog.Warningf("Failed to load or register custom tools (original value: %q, processed path: %q): %v", path, cleanedPath, err)
			}
		}
	}
	return nil
}

// repl is a read-eval-print loop for the chat session.
func repl(ctx context.Context, initialQuery string, ui ui.UI, agent *agent.Agent) error {
	query := initialQuery
	// Note: Initial greeting and MCP status are now handled by the agent itself
	// through the message-based system
	err := agent.Run(ctx, query)
	if err != nil {
		return fmt.Errorf("running agent: %w", err)
	}

	err = ui.Run(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("running UI: %w", err)
	}

	return nil
}

// Redirect standard log output to our custom klog writer
// This is primarily to suppress warning messages from
// genai library https://github.com/googleapis/go-genai/blob/6ac4afc0168762dc3b7a4d940fc463cc1854f366/types.go#L1633
func redirectStdLogToKlog() {
	log.SetOutput(klogWriter{})

	// Disable standard log's prefixes (date, time, file info)
	// because klog will add its own more detailed prefix.
	log.SetFlags(0)
}

// Define a custom writer that forwards messages to klog.Warning
type klogWriter struct{}

// Implement the io.Writer interface
func (writer klogWriter) Write(data []byte) (n int, err error) {
	// We trim the trailing newline because klog adds its own.
	message := string(bytes.TrimSuffix(data, []byte("\n")))
	klog.Warning(message)
	return len(data), nil
}

func hasStdInData() (bool, error) {
	hasData := false

	stat, err := os.Stdin.Stat()
	if err != nil {
		return hasData, fmt.Errorf("checking stdin: %w", err)
	}
	hasData = (stat.Mode() & os.ModeCharDevice) == 0

	return hasData, nil
}

// resolveQueryInput determines the query input from positional args and/or stdin.
// It supports:
// - 1 positional arg only -> kubectl-ai "get pods"
// - stdin only -> echo "get pods" | kubectl-ai
// - 1 positional arg + stdin (combined) -> kubectl-ai get <<< "pods" or kubectl-ai "get" <<< "pods"
// As default no positional arg nor stdin
func resolveQueryInput(hasStdInData bool, args []string) (string, error) {
	switch {
	case len(args) == 1 && !hasStdInData:
		// Use argument directly
		return args[0], nil

	case len(args) == 1 && hasStdInData:
		// Combine arg + stdin
		var b strings.Builder
		b.WriteString(args[0])
		b.WriteString("\n")

		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			b.WriteString(scanner.Text())
			b.WriteString("\n")
		}
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("reading stdin: %w", err)
		}
		query := strings.TrimSpace(b.String())
		if query == "" {
			return "", fmt.Errorf("no query provided from stdin")
		}
		return query, nil

	case len(args) == 0 && hasStdInData:
		// Read stdin only
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("reading stdin: %w", err)
		}
		query := strings.TrimSpace(string(b))
		if query == "" {
			return "", fmt.Errorf("no query provided from stdin")
		}
		return query, nil

	default:
		// Case: No input at all — return empty string, no error
		return "", nil
	}
}

func resolveKubeConfigPath(opt *Options) error {
	switch {
	case opt.KubeConfigPath != "":
		// Already set from flag or viper env
	case os.Getenv("KUBECONFIG") != "":
		opt.KubeConfigPath = os.Getenv("KUBECONFIG")
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get user home directory: %w", err)
		}
		opt.KubeConfigPath = filepath.Join(home, ".kube", "config")
	}

	// We resolve the kubeconfig path to an absolute path, so we can run kubectl from any working directory.
	if opt.KubeConfigPath != "" {
		p, err := filepath.Abs(opt.KubeConfigPath)
		if err != nil {
			return fmt.Errorf("failed to get absolute path for kubeconfig file %q: %w", opt.KubeConfigPath, err)
		}
		opt.KubeConfigPath = p
	}

	return nil
}

func startMCPServer(ctx context.Context, opt Options) error {
	workDir := filepath.Join(os.TempDir(), "kubectl-ai-mcp")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("error creating work directory: %w", err)
	}
	mcpServer, err := newKubectlMCPServer(ctx, opt.KubeConfigPath, tools.Default(), workDir, opt.ExternalTools, opt.MCPServerMode, opt.SSEndpointPort)
	if err != nil {
		return fmt.Errorf("creating mcp server: %w", err)
	}
	return mcpServer.Serve(ctx)
}

// handleListSessions lists all available sessions with their metadata.
func handleListSessions() error {
	manager, err := sessions.NewSessionManager()
	if err != nil {
		return fmt.Errorf("failed to create session manager: %w", err)
	}

	sessionList, err := manager.ListSessions()
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	if len(sessionList) == 0 {
		fmt.Println("No sessions found.")
		return nil
	}

	fmt.Println("Available sessions:")
	fmt.Println("ID\t\tCreated\t\t\tLast Accessed\t\tModel\t\tProvider")
	fmt.Println("--\t\t-------\t\t\t-------------\t\t-----\t\t--------")

	for _, session := range sessionList {
		metadata, err := session.LoadMetadata()
		if err != nil {
			fmt.Printf("%s\t\t<error loading metadata>\n", session.ID)
			continue
		}

		fmt.Printf("%s\t%s\t%s\t%s\t%s\n",
			session.ID,
			metadata.CreatedAt.Format("2006-01-02 15:04:05"),
			metadata.LastAccessed.Format("2006-01-02 15:04:05"),
			metadata.ModelID,
			metadata.ProviderID)
	}

	return nil
}

// handleDeleteSession deletes a session by ID.
func handleDeleteSession(sessionID string) error {
	manager, err := sessions.NewSessionManager()
	if err != nil {
		return fmt.Errorf("failed to create session manager: %w", err)
	}

	// Check if session exists
	session, err := manager.FindSessionByID(sessionID)
	if err != nil {
		return fmt.Errorf("session %s not found: %w", sessionID, err)
	}

	// Load metadata for confirmation
	metadata, err := session.LoadMetadata()
	if err != nil {
		return fmt.Errorf("failed to load session metadata: %w", err)
	}

	fmt.Printf("Deleting session %s:\n", sessionID)
	fmt.Printf("  Model: %s\n", metadata.ModelID)
	fmt.Printf("  Provider: %s\n", metadata.ProviderID)
	fmt.Printf("  Created: %s\n", metadata.CreatedAt.Format("2006-01-02 15:04:05"))

	fmt.Print("Are you sure you want to delete this session? (y/N): ")
	var response string
	fmt.Scanln(&response)

	if response != "y" && response != "Y" {
		fmt.Println("Deletion cancelled.")
		return nil
	}

	if err := manager.DeleteSession(sessionID); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	fmt.Printf("Session %s deleted successfully.\n", sessionID)
	return nil
}

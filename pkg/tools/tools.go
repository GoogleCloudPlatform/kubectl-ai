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

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/journal"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/sandbox"
	"github.com/google/uuid"
	"sigs.k8s.io/yaml"
)

type ContextKey string

const (
	KubeconfigKey ContextKey = "kubeconfig"
	WorkDirKey    ContextKey = "work_dir"
	ExecutorKey   ContextKey = "executor"
)

func Lookup(name string) Tool {
	return allTools.Lookup(name)
}

var allTools Tools = Tools{
	tools: make(map[string]Tool),
}

func Default() Tools {
	return allTools
}

// RegisterTool makes a tool available to the LLM.
func RegisterTool(tool Tool) {
	allTools.RegisterTool(tool)
}

type Tools struct {
	tools map[string]Tool
}

func (t *Tools) Init() {
	t.tools = make(map[string]Tool)
}

func (t *Tools) Lookup(name string) Tool {
	return t.tools[name]
}

func (t *Tools) AllTools() []Tool {
	return slices.Collect(maps.Values(t.tools))
}

func (t *Tools) Names() []string {
	names := make([]string, 0, len(t.tools))
	for name := range t.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (t *Tools) RegisterTool(tool Tool) {
	// if mcp tool use unique name
	name := tool.Name()
	// For MCP tools, we need to use a unique name to avoid conflicts
	// with built-in tools or tools from other MCP servers.
	if mcpTool, ok := tool.(*MCPTool); ok {
		name = mcpTool.UniqueToolName()
	}

	if _, exists := t.tools[name]; exists {
		panic("tool already registered: " + name)
	}
	t.tools[name] = tool
}

type ToolCall struct {
	tool      Tool
	name      string
	arguments map[string]any
}

// Description returns a description of the tool call.
// This is used to display the tool call in the UI.
// It should be human-readable,
// and should be concise enough that the user can read it quickly,
// but precise enough that the user can decide whether to invoke the tool.
func (t *ToolCall) Description() string {
	// Check if this is an MCP tool and format accordingly
	if mcpTool, ok := t.tool.(*MCPTool); ok {
		if command, ok := t.arguments["command"]; ok {
			return fmt.Sprintf("[MCP: %s] %s", mcpTool.serverName, command.(string))
		}
		var args []string
		for k, v := range t.arguments {
			args = append(args, fmt.Sprintf("%s=%v", k, v))
		}
		sort.Strings(args)
		return fmt.Sprintf("[MCP: %s] %s(%s)", mcpTool.serverName, t.name, strings.Join(args, ", "))
	}

	// Default formatting for non-MCP tools
	if command, ok := t.arguments["command"]; ok {
		return command.(string)
	}
	var args []string
	for k, v := range t.arguments {
		args = append(args, fmt.Sprintf("%s=%v", k, v))
	}
	sort.Strings(args)
	return fmt.Sprintf("%s(%s)", t.name, strings.Join(args, ", "))
}

// ParseToolInvocation parses a request from the LLM into a tool call.
func (t *Tools) ParseToolInvocation(ctx context.Context, name string, arguments map[string]any) (*ToolCall, error) {
	tool := t.Lookup(name)
	if tool == nil {
		return nil, fmt.Errorf("tool %q not recognized", name)
	}

	return &ToolCall{
		tool:      tool,
		name:      name,
		arguments: arguments,
	}, nil
}

type InvokeToolOptions struct {
	WorkDir string

	// Kubeconfig is the path to the kubeconfig file.
	Kubeconfig string

	// Executor is the executor for tool execution
	Executor sandbox.Executor
}

type ToolRequestEvent struct {
	CallID    string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

type ToolResponseEvent struct {
	CallID   string `json:"id,omitempty"`
	Response any    `json:"response,omitempty"`
	Error    string `json:"error,omitempty"`
}

// InvokeTool handles the execution of a single action
func (t *ToolCall) InvokeTool(ctx context.Context, opt InvokeToolOptions) (any, error) {
	recorder := journal.RecorderFromContext(ctx)

	callID := uuid.NewString()
	recorder.Write(ctx, &journal.Event{
		Timestamp: time.Now(),
		Action:    "tool-request",
		Payload: ToolRequestEvent{
			CallID:    callID,
			Name:      t.name,
			Arguments: t.arguments,
		},
	})

	ctx = context.WithValue(ctx, KubeconfigKey, opt.Kubeconfig)
	ctx = context.WithValue(ctx, WorkDirKey, opt.WorkDir)
	if opt.Executor != nil {
		ctx = context.WithValue(ctx, ExecutorKey, opt.Executor)
	}

	response, err := t.tool.Run(ctx, t.arguments)

	{
		ev := ToolResponseEvent{
			CallID:   callID,
			Response: response,
		}
		if err != nil {
			ev.Error = err.Error()
		}
		recorder.Write(ctx, &journal.Event{
			Timestamp: time.Now(),
			Action:    "tool-response",
			Payload:   ev,
		})
	}

	return response, err
}

// ToolResultToMap converts an arbitrary result to a map[string]any
func ToolResultToMap(result any) (map[string]any, error) {
	// Handle simple string results (common with MCP tools)
	if str, ok := result.(string); ok {
		return map[string]any{"content": str}, nil
	}

	// Handle nil results
	if result == nil {
		return map[string]any{"content": ""}, nil
	}

	// Try to convert to map via JSON for structured results
	b, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("converting result to json: %w", err)
	}

	m := make(map[string]any)
	if err := json.Unmarshal(b, &m); err != nil {
		// If JSON unmarshal fails, wrap the original result
		return map[string]any{"content": result}, nil
	}
	return m, nil
}

// LoadAndRegisterCustomTools loads tool configurations from a YAML file
// and registers them.
func LoadAndRegisterCustomTools(configPath string) error {
	pathInfo, err := os.Stat(configPath)
	if err != nil {
		return fmt.Errorf("failed to describe config file %s: %w", configPath, err)
	}

	if pathInfo.IsDir() {
		configPaths, err := os.ReadDir(configPath)
		if err != nil {
			return fmt.Errorf("failed to read config dir %s: %w", configPath, err)
		}

		for _, entry := range configPaths {
			if err := LoadAndRegisterCustomTools(filepath.Join(configPath, entry.Name())); err != nil {
				return err
			}
		}

		return nil
	}

	yamlFile, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	var configs []CustomToolConfig
	err = yaml.Unmarshal(yamlFile, &configs)
	if err != nil {
		return fmt.Errorf("failed to parse YAML config file %s: %w", configPath, err)
	}

	// Register each custom tool
	var registrationErrors []string
	for _, config := range configs {
		tool, err := NewCustomTool(config)
		if err != nil {
			registrationErrors = append(registrationErrors, fmt.Sprintf("failed to create tool %q: %v", config.Name, err))
			continue // Skip registration if creation failed
		}
		// Check for duplicate registration attempt
		if _, exists := allTools.tools[tool.Name()]; exists {
			registrationErrors = append(registrationErrors, fmt.Sprintf("tool %q already registered (possibly built-in), skipping custom definition", tool.Name()))
			continue
		}
		RegisterTool(tool)
	}

	if len(registrationErrors) > 0 {
		return fmt.Errorf("encountered errors during custom tool registration:\n - %s", strings.Join(registrationErrors, "\n - "))
	}

	return nil
}

// For CustomTool
func (t *CustomTool) IsInteractive(args map[string]any) (bool, error) {
	// Custom tools are not interactive by default
	return false, nil
}

// Add a method to access the tool
func (t *ToolCall) GetTool() Tool {
	return t.tool
}

// ExpandShellVar expands shell variables and syntax using bash
func ExpandShellVar(value string) (string, error) {
	if strings.Contains(value, "~") {
		if len(value) >= 2 && value[0] == '~' && os.IsPathSeparator(value[1]) {
			if runtime.GOOS == "windows" {
				value = filepath.Join(os.Getenv("USERPROFILE"), value[2:])
			} else {
				value = filepath.Join(os.Getenv("HOME"), value[2:])
			}
		}
	}
	return os.ExpandEnv(value), nil
}

func IsInteractiveCommand(command string) (bool, error) {
	// Inline isKubectlCommand logic
	words := strings.Fields(command)
	if len(words) == 0 {
		return false, nil
	}
	base := filepath.Base(words[0])
	if base != "kubectl" {
		return false, nil
	}

	isExec := strings.Contains(command, " exec ") && strings.Contains(command, " -it")
	isPortForward := strings.Contains(command, " port-forward ")
	isEdit := strings.Contains(command, " edit ")

	if isExec || isPortForward || isEdit {
		return true, fmt.Errorf("interactive mode not supported for kubectl, please use non-interactive commands")
	}
	return false, nil
}

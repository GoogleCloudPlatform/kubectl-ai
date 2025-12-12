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
	"fmt"
	"os"
	"strings"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/sandbox"
	"mvdan.cc/sh/v3/syntax"
)

// CustomToolConfig defines the structure for configuring a custom tool.
type CustomToolConfig struct {
	Name          string `yaml:"name"`
	Description   string `yaml:"description"`
	Command       string `yaml:"command"`
	CommandDesc   string `yaml:"command_desc"`
	IsInteractive bool   `yaml:"is_interactive"`
}

// CustomTool implements the Tool interface for external commands.
type CustomTool struct {
	config   CustomToolConfig
	executor sandbox.Executor
}

// NewCustomTool creates a new CustomTool instance.
func NewCustomTool(config CustomToolConfig) (*CustomTool, error) {
	if config.Name == "" {
		return nil, fmt.Errorf("custom tool name cannot be empty")
	}
	if len(config.Command) == 0 {
		return nil, fmt.Errorf("custom tool command cannot be empty for tool %q", config.Name)
	}

	return &CustomTool{config: config}, nil
}

// Name returns the tool's name.
func (t *CustomTool) Name() string {
	return t.config.Name
}

// Description returns the tool's description from its function definition.
func (t *CustomTool) Description() string {
	return t.config.Description
}

// FunctionDefinition returns the tool's function definition.
func (t *CustomTool) FunctionDefinition() *gollm.FunctionDefinition {
	return &gollm.FunctionDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: &gollm.Schema{
			Type: gollm.TypeObject,
			Properties: map[string]*gollm.Schema{
				"command": {
					Type:        gollm.TypeString,
					Description: t.config.CommandDesc,
				},
				"modifies_resource": {
					Type: gollm.TypeString,
					Description: `Whether the command modifies a resource.
Possible values:
- "yes" if the command modifies a resource
- "no" if the command does not modify a resource
- "unknown" if the command's effect on the resource is unknown
`,
				},
			},
		},
	}
}

// addCommandPrefix adds the tool's command prefix to the input command if needed.
// It only adds the prefix if the command is a simple command (no pipes, etc.)
// and doesn't already start with the prefix.
// TODO(droot): This will not be needed when models improve on following instructions
// and specify the complete command to execute.
func (t *CustomTool) addCommandPrefix(inputCmd string) (string, error) {
	// Parse the command to check if it's a simple command
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(inputCmd), "")
	if err != nil {
		return "", fmt.Errorf("failed to parse command: %w", err)
	}

	// Check if it's a simple command (no pipes, redirects, etc.)
	if len(prog.Stmts) != 1 {
		return inputCmd, nil
	}
	stmt := prog.Stmts[0]
	if stmt.Background || stmt.Coprocess || stmt.Negated || len(stmt.Redirs) > 0 {
		return inputCmd, nil
	}

	// Check if it's a simple call expression
	if _, ok := stmt.Cmd.(*syntax.CallExpr); !ok {
		return inputCmd, nil
	}

	// If we get here, it's a simple command without the prefix
	if strings.HasPrefix(inputCmd, t.config.Command) {
		return inputCmd, nil
	}

	return t.config.Command + " " + inputCmd, nil
}

// Run executes the external command defined for the custom tool.
func (t *CustomTool) Run(ctx context.Context, args map[string]any) (any, error) {
	var command string
	cmdVal, ok := args["command"]
	if !ok {
		return nil, fmt.Errorf("command not found in args")
	}
	command = cmdVal.(string)

	command, err := t.addCommandPrefix(command)
	if err != nil {
		return nil, fmt.Errorf("failed to process command: %w", err)
	}

	workDir := ctx.Value(WorkDirKey).(string)
	env := os.Environ()

	// Use the injected executor, or fallback to local if not set (e.g. for global instance)
	executor := t.executor
	if executor == nil {
		executor = sandbox.NewLocalExecutor()
	}

	// Execute the command
	return executor.Execute(ctx, command, env, workDir)
}

// CheckModifiesResource determines if the command modifies resources
// For custom tools, we'll conservatively assume they might modify resources
// unless we have specific knowledge otherwise
// Returns "yes", "no", or "unknown"
func (t *CustomTool) CheckModifiesResource(args map[string]any) string {
	// For custom tools, we'll conservatively use "unknown" since we can't
	return "unknown"
}

// CloneWithExecutor creates a copy of the CustomTool with the given executor.
// This is used to create a session-specific instance of the tool.
func (t *CustomTool) CloneWithExecutor(executor sandbox.Executor) *CustomTool {
	return &CustomTool{
		config:   t.config,
		executor: executor,
	}
}

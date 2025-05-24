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
	"os/exec"
	"strings"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
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
	config CustomToolConfig
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
					Description: `Whether the command modifies a kubernetes resource.
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

// Run executes the external command defined for the custom tool.
func (t *CustomTool) Run(ctx context.Context, args map[string]any) (any, error) {
	command := strings.Fields(t.config.Command)
	if len(command) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	cmdArgs := []string{}
	if len(command) > 1 {
		cmdArgs = command[1:]
	}
	if cmdVal, ok := args["command"]; ok {
		cmdArgs = append(cmdArgs, cmdVal.(string))
	}
	workDir := ctx.Value(WorkDirKey).(string)

	cmd := exec.CommandContext(ctx, command[0], cmdArgs...)
	cmd.Dir = workDir
	cmd.Env = os.Environ()

	return executeCommand(cmd)
}

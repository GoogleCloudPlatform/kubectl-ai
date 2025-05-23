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

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/mcp"
	"k8s.io/klog/v2"
)

// MCPTool wraps an MCP server tool to implement the Tool interface
type MCPTool struct {
	serverName  string
	toolName    string
	description string
	schema      *gollm.FunctionDefinition
	manager     *mcp.Manager
}

// NewMCPTool creates a new MCP tool wrapper
func NewMCPTool(serverName, toolName, description string, schema *gollm.FunctionDefinition, manager *mcp.Manager) *MCPTool {
	return &MCPTool{
		serverName:  serverName,
		toolName:    toolName,
		description: description,
		schema:      schema,
		manager:     manager,
	}
}

// Name returns the tool name
func (t *MCPTool) Name() string {
	return t.toolName
}

// Description returns the tool description
func (t *MCPTool) Description() string {
	return t.description
}

// FunctionDefinition returns the tool's function definition
func (t *MCPTool) FunctionDefinition() *gollm.FunctionDefinition {
	return t.schema
}

// Run executes the MCP tool
func (t *MCPTool) Run(ctx context.Context, args map[string]any) (any, error) {
	log := klog.FromContext(ctx)

	// Show MCP invocation message to user
	if err := ShowMCPInvocationMessage(ctx, t.toolName, t.serverName); err != nil {
		log.V(2).Info("Failed to show MCP invocation message", "error", err)
	}

	// Get MCP client for the server
	client, exists := t.manager.GetClient(t.serverName)
	if !exists {
		return nil, fmt.Errorf("MCP server %q not connected", t.serverName)
	}

	log.V(2).Info("Executing MCP tool", "tool", t.toolName, "server", t.serverName, "args", args)

	// Execute tool on MCP server
	result, err := client.CallTool(ctx, t.toolName, args)
	if err != nil {
		return nil, fmt.Errorf("calling MCP tool %q on server %q: %w", t.toolName, t.serverName, err)
	}

	log.V(2).Info("MCP tool executed successfully", "tool", t.toolName, "server", t.serverName)

	return result, nil
}

// ShowMCPInvocationMessage displays a message to the user when an MCP tool is invoked
func ShowMCPInvocationMessage(ctx context.Context, toolName, serverName string) error {
	msg := fmt.Sprintf("[MCP] Invoking tool %s via MCP server: %s", toolName, serverName)

	// Log the MCP invocation message
	// Note: UI message display would be handled at the agent level where document context is available
	klog.V(1).Info(msg)
	return nil
}

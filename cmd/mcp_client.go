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
	"context"
	"fmt"
	"strings"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/mcp"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/tools"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/ui"
	"k8s.io/klog/v2"
)

// InitializeMCPClient initializes MCP client functionality when --mcp-client flag is used.
// It connects to servers and registers discovered tools with the kubectl-ai tool system.
func InitializeMCPClient() (*mcp.Manager, error) {
	// Initialize the MCP manager
	manager, err := mcp.InitializeManager()
	if err != nil {
		return nil, err
	}

	// Connect to servers and register tools
	ctx := context.Background()
	err = manager.RegisterWithToolSystem(ctx, func(serverName string, toolInfo mcp.Tool) error {
		// Create schema for the tool
		schema, err := tools.ConvertToolToGollm(&mcp.Tool{
			Name:        toolInfo.Name,
			Description: toolInfo.Description,
		})
		if err != nil {
			return err
		}

		// Create and register MCP tool wrapper
		mcpTool := tools.NewMCPTool(serverName, toolInfo.Name, toolInfo.Description, schema, manager)
		tools.RegisterTool(mcpTool)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return manager, nil
}

// GetMCPServerStatusWithClientMode returns UI blocks showing MCP server status
func GetMCPServerStatusWithClientMode(mcpClientEnabled bool, mcpManager *mcp.Manager) ([]ui.Block, error) {
	ctx := context.Background()
	var status *mcp.MCPStatus
	var err error

	if mcpClientEnabled {
		// In client mode, use the provided manager
		if mcpManager != nil {
			status, err = mcpManager.GetStatus(ctx, mcpClientEnabled)
			if err != nil {
				klog.Errorf("Failed to get MCP server status: %v", err)
				return nil, err
			}
		} else {
			status = &mcp.MCPStatus{ClientEnabled: mcpClientEnabled}
		}
	}

	return formatMCPStatus(status), nil
}

// formatMCPStatus converts an MCP status into UI blocks for display
func formatMCPStatus(status *mcp.MCPStatus) []ui.Block {
	var blocks []ui.Block

	// Add summary block
	blocks = append(blocks, createSummaryBlock(status))

	// Add server details blocks
	if len(status.ServerInfoList) > 0 {
		for _, server := range status.ServerInfoList {
			blocks = append(blocks, createServerBlock(server))
		}
	}

	return blocks
}

// createSummaryBlock creates a summary block for MCP status
func createSummaryBlock(status *mcp.MCPStatus) ui.Block {
	var summaryText string

	if status.ClientEnabled && status.ConnectedCount > 0 {
		summaryText = fmt.Sprintf("\n  Successfully connected to %d MCP server(s) (%d tools discovered)\n",
			status.ConnectedCount, status.TotalTools)
	} else if status.ClientEnabled {
		summaryText = "\n  No MCP servers connected\n"
	} else if status.TotalServers > 0 {
		summaryText = fmt.Sprintf("\n  %d MCP servers configured (client mode disabled)\n",
			status.TotalServers)
	} else {
		summaryText = "\n  No MCP servers configured\n"
	}

	block := ui.NewAgentTextBlock()
	block.SetText(summaryText)
	return block
}

// createServerBlock creates a UI block for a single MCP server
func createServerBlock(server mcp.ServerConnectionInfo) ui.Block {
	// Get connection status
	connectionStatus := "Disconnected"
	if server.IsConnected {
		connectionStatus = "Connected"
	}

	// Get tool names if available
	var toolNames []string
	if server.IsConnected && len(server.AvailableTools) > 0 {
		for _, tool := range server.AvailableTools {
			toolNames = append(toolNames, tool.Name)
		}
	}

	// Format server details with clean spacing
	var details strings.Builder
	details.WriteString("\n\n") // Double newline for spacing between servers

	// Build server info line
	details.WriteString("    • ") // Bullet point with indentation
	details.WriteString(fmt.Sprintf("%s (%s) - %s",
		server.Name,
		extractCommandName(server.Command),
		connectionStatus))

	if len(toolNames) > 0 {
		details.WriteString(fmt.Sprintf(", Tools: %s", strings.Join(toolNames, ", ")))
	}

	details.WriteString("\n\n") // Add spacing after the server details

	block := ui.NewAgentTextBlock()
	block.SetText(details.String())
	return block
}

// extractCommandName gets the base command from a command string
func extractCommandName(command string) string {
	if command == "" {
		return "remote"  // Return 'remote' for HTTP-based servers
	}

	parts := strings.Fields(command)
	if len(parts) > 0 {
		return parts[0]
	}

	return command
}

// LoadMCPConfig loads and logs the MCP configuration
func LoadMCPConfig() {
	mcpConfigPath, err := mcp.DefaultConfigPath()
	if err != nil {
		klog.Warningf("Failed to get MCP config path: %v", err)
		return
	}

	// Create a temporary Manager instance to call LogConfig
	manager := &mcp.Manager{}
	if err := manager.LogConfig(mcpConfigPath); err != nil {
		klog.Warningf("Failed to load or log MCP config: %v", err)
	}
}

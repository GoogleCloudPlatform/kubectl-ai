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

package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/mcp"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/tools"
	"k8s.io/klog/v2"
)

// InitializeMCPClient initializes MCP client functionality for the agent.
// It connects to servers and registers discovered tools with the kubectl-ai tool system.
func (a *Agent) InitializeMCPClient(ctx context.Context) error {
	// Initialize the MCP manager
	manager, err := mcp.InitializeManager()
	if err != nil {
		return fmt.Errorf("failed to initialize MCP manager: %w", err)
	}

	// Connect to servers and register tools
	err = manager.RegisterWithToolSystem(ctx, func(serverName string, toolInfo mcp.Tool) error {
		// Create schema for the tool
		schema, err := tools.ConvertToolToGollm(&toolInfo)
		if err != nil {
			return err
		}

		// Create and register MCP tool wrapper
		mcpTool := tools.NewMCPTool(serverName, toolInfo.Name, toolInfo.Description, schema, manager)
		tools.RegisterTool(mcpTool)
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to register MCP tools: %w", err)
	}

	// Store the manager for later use
	a.mcpManager = manager

	// Register MCP resources with the agent
	var resourceTexts []string

	resources, err := manager.ListAvailableResources(ctx)
	if err != nil {
		klog.Warningf("Failed to list MCP resources: %v", err)
	} else {
		for serverName, resList := range resources {
			for _, res := range resList {
				content, err := manager.ReadResource(ctx, serverName, res.URI)
				if err != nil {
					klog.Warningf("Failed to read resource %s from server %s: %v", res.URI, serverName, err)
					continue
				}
				resourceTexts = append(resourceTexts, fmt.Sprintf("### %s (%s)\n%s", res.Name, serverName, content))
			}
		}
	}

	a.mcpResourceContext = strings.Join(resourceTexts, "\n\n")

	return nil
}

// UpdateMCPStatus updates the MCP status in the agent's session
func (a *Agent) UpdateMCPStatus(ctx context.Context, mcpClientEnabled bool) error {
	if a.mcpManager == nil && !mcpClientEnabled {
		// No MCP functionality requested
		return nil
	}

	status, err := a.getMCPStatus(ctx, mcpClientEnabled)
	if err != nil {
		klog.Errorf("Failed to get MCP server status: %v", err)
		return err
	}

	// Update the session with MCP status
	a.sessionMu.Lock()
	defer a.sessionMu.Unlock()
	a.session.MCPStatus = status

	return nil
}

// getMCPStatus retrieves the current MCP status
func (a *Agent) getMCPStatus(ctx context.Context, mcpClientEnabled bool) (*api.MCPStatus, error) {
	var mcpStatus *mcp.MCPStatus
	var err error

	if mcpClientEnabled && a.mcpManager != nil {
		// In client mode, use the provided manager
		mcpStatus, err = a.mcpManager.GetStatus(ctx, mcpClientEnabled)
		if err != nil {
			return nil, err
		}
	} else {
		// Create minimal status
		mcpStatus = &mcp.MCPStatus{
			ClientEnabled: mcpClientEnabled,
		}
	}

	// Convert from mcp.MCPStatus to api.MCPStatus
	return a.convertMCPStatus(mcpStatus), nil
}

// convertMCPStatus converts from mcp.MCPStatus to api.MCPStatus
func (a *Agent) convertMCPStatus(mcpStatus *mcp.MCPStatus) *api.MCPStatus {
	if mcpStatus == nil {
		return nil
	}

	apiStatus := &api.MCPStatus{
		TotalServers:   mcpStatus.TotalServers,
		ConnectedCount: mcpStatus.ConnectedCount,
		FailedCount:    mcpStatus.FailedCount,
		TotalTools:     mcpStatus.TotalTools,
		ClientEnabled:  mcpStatus.ClientEnabled,
	}

	// Convert server connection info
	for _, server := range mcpStatus.ServerInfoList {
		apiServerInfo := api.ServerConnectionInfo{
			Name:        server.Name,
			Command:     server.Command,
			IsLegacy:    server.IsLegacy,
			IsConnected: server.IsConnected,
		}

		// Convert tools
		for _, tool := range server.AvailableTools {
			apiTool := api.MCPTool{
				Name:        tool.Name,
				Description: tool.Description,
				Server:      tool.Server,
			}
			apiServerInfo.AvailableTools = append(apiServerInfo.AvailableTools, apiTool)
		}

		apiStatus.ServerInfoList = append(apiStatus.ServerInfoList, apiServerInfo)
	}

	return apiStatus
}

// GetMCPStatusText returns a formatted text representation of the MCP status
// This can be used by UIs that want to display the status as text
func (a *Agent) GetMCPStatusText() string {
	a.sessionMu.Lock()
	defer a.sessionMu.Unlock()
	if a.session.MCPStatus == nil {
		return ""
	}

	var statusText strings.Builder

	status := a.session.MCPStatus

	// Add summary text
	if status.ClientEnabled && status.ConnectedCount > 0 {
		statusText.WriteString(fmt.Sprintf("Successfully connected to %d MCP server(s) (%d tools discovered)\n\n",
			status.ConnectedCount, status.TotalTools))
	} else if status.ClientEnabled {
		statusText.WriteString("No MCP servers connected\n\n")
	} else if status.TotalServers > 0 {
		statusText.WriteString(fmt.Sprintf("%d MCP servers configured (client mode disabled)\n\n",
			status.TotalServers))
	} else {
		statusText.WriteString("No MCP servers configured\n\n")
	}

	// Add server details
	for _, server := range status.ServerInfoList {
		connectionStatus := "Disconnected"
		if server.IsConnected {
			connectionStatus = "Connected"
		}

		// Get tool names if available
		var toolNames []string
		for _, tool := range server.AvailableTools {
			toolNames = append(toolNames, tool.Name)
		}

		// Format server details
		statusText.WriteString("    • ") // Bullet point with indentation
		statusText.WriteString(fmt.Sprintf("%s (%s) - %s",
			server.Name,
			extractCommandName(server.Command),
			connectionStatus))

		if len(toolNames) > 0 {
			statusText.WriteString(fmt.Sprintf(", Tools: %s", strings.Join(toolNames, ", ")))
		}

		statusText.WriteString("\n")
	}

	return statusText.String()
}

// extractCommandName gets the base command from a command string
func extractCommandName(command string) string {
	if command == "" {
		return "remote" // Return 'remote' for HTTP-based servers
	}

	parts := strings.Fields(command)
	if len(parts) > 0 {
		return parts[0]
	}

	return command
}

// CloseMCPClient closes the MCP client connections
func (a *Agent) CloseMCPClient() error {
	if a.mcpManager != nil {
		err := a.mcpManager.Close()
		a.mcpManager = nil
		return err
	}
	return nil
}

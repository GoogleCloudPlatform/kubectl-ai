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
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/mcp"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/tools"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/ui"
	"k8s.io/klog/v2"
)

// GetMCPServerStatus returns a slice of text blocks with the current MCP server status
// This function is called to display MCP server information in the UI
func GetMCPServerStatus() ([]ui.Block, error) {
	var blocks []ui.Block

	// Try to get MCP config path
	mcpConfigPath, err := mcp.DefaultConfigPath()
	if err != nil {
		klog.Warningf("[DEBUG] Failed to get MCP config path: %v", err)
		return blocks, nil // Don't fail, just return empty blocks
	}

	// Load the config using the mcp package
	mcpConfig, err := mcp.LoadConfig(mcpConfigPath)
	if err != nil {
		return blocks, nil // Don't fail, just return empty blocks
	}

	totalServers := len(mcpConfig.Servers) + len(mcpConfig.MCPServers)

	if totalServers == 0 {
		blocks = append(blocks, ui.NewAgentTextBlock().WithText("No MCP servers configured."))
		return blocks, nil
	}

	// Get the MCP manager to access discovered tools
	mcpManager := tools.GetMCPManager()
	var serverTools map[string][]mcp.ToolInfo

	if mcpManager != nil {
		// Try to get available tools with a short timeout
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		serverTools, err = mcpManager.ListAvailableTools(ctx)
		if err != nil {
			klog.V(2).InfoS("Failed to get tools from MCP manager", "error", err)
			serverTools = make(map[string][]mcp.ToolInfo) // Empty map to avoid nil panics
		}
	} else {
		serverTools = make(map[string][]mcp.ToolInfo) // Empty map
	}

	// Build a user-friendly summary
	serverNames := []string{}
	for _, server := range mcpConfig.Servers {
		serverNames = append(serverNames, server.Name)
	}
	for name, server := range mcpConfig.MCPServers {
		if server.Name != "" {
			serverNames = append(serverNames, server.Name)
		} else {
			serverNames = append(serverNames, name)
		}
	}

	// Count total discovered tools
	totalTools := 0
	for _, toolList := range serverTools {
		totalTools += len(toolList)
	}

	summary := fmt.Sprintf("Loaded %d MCP server(s): %s", totalServers, strings.Join(serverNames, ", "))
	if totalTools > 0 {
		summary += fmt.Sprintf(" (%d tools discovered)", totalTools)
	}
	blocks = append(blocks, ui.NewAgentTextBlock().WithText(summary))

	// Show details for each server with their tools
	for _, server := range mcpConfig.Servers {
		serverBlock := ui.NewAgentTextBlock()
		serverText := fmt.Sprintf("    • %s (%s)", server.Name, server.Command)

		// Add tools information if available
		if tools, exists := serverTools[server.Name]; exists && len(tools) > 0 {
			toolNames := make([]string, len(tools))
			for i, tool := range tools {
				toolNames[i] = tool.Name
			}
			serverText += fmt.Sprintf(" - Tools: %s", strings.Join(toolNames, ", "))
		} else {
			serverText += " - No tools discovered"
		}

		serverBlock.SetText(serverText)
		blocks = append(blocks, serverBlock)
	}

	for name, server := range mcpConfig.MCPServers {
		serverName := name
		if server.Name != "" {
			serverName = server.Name
		}
		serverBlock := ui.NewAgentTextBlock()
		serverText := fmt.Sprintf("    • %s (%s) (legacy)", serverName, server.Command)

		// Add tools information if available
		if tools, exists := serverTools[serverName]; exists && len(tools) > 0 {
			toolNames := make([]string, len(tools))
			for i, tool := range tools {
				toolNames[i] = tool.Name
			}
			serverText += fmt.Sprintf(" - Tools: %s", strings.Join(toolNames, ", "))
		} else {
			serverText += " - No tools discovered"
		}

		serverBlock.SetText(serverText)
		blocks = append(blocks, serverBlock)
	}

	return blocks, nil
}

// StartMCPServer starts the MCP server with the given configuration
func StartMCPServer(ctx context.Context, opt Options) error {
	workDir := filepath.Join(os.TempDir(), "kubectl-ai-mcp")

	mcpServer, err := newKubectlMCPServer(ctx, opt.KubeConfigPath, tools.Default(), workDir)
	if err != nil {
		return fmt.Errorf("creating mcp server: %w", err)
	}
	return mcpServer.Serve(ctx)
}

// LoadMCPConfig loads and logs the MCP configuration
func LoadMCPConfig() {
	mcpConfigPath, err := mcp.DefaultConfigPath()
	if err == nil {
		if mcpConfig, err := mcp.LoadConfig(mcpConfigPath); err == nil {
			serverCount := len(mcpConfig.Servers)
			legacyServerCount := len(mcpConfig.MCPServers)
			totalServers := serverCount + legacyServerCount

			if totalServers > 0 {
				serverWord := "server"
				if totalServers > 1 {
					serverWord = "servers"
				}
				klog.Infof("Loaded %d MCP %s from %s", totalServers, serverWord, mcpConfigPath)

				// Log servers from the new format
				for _, server := range mcpConfig.Servers {
					klog.Infof("  - %s: %s", server.Name, server.Command)
				}

				// Log servers from the legacy format
				for name, server := range mcpConfig.MCPServers {
					serverName := name
					if server.Name != "" {
						serverName = server.Name
					}
					klog.Infof("  - %s: %s (legacy)", serverName, server.Command)
				}
			}
		} else {
			klog.Warningf("Failed to load MCP config from %s: %v", mcpConfigPath, err)
		}
	} else {
		klog.Warningf("Failed to get MCP config path: %v", err)
	}
}

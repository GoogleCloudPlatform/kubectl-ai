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

package main

import (
	"context"
	"fmt"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/mcp"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/tools"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"k8s.io/klog/v2"
)

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

	// Add built-in tools
	for _, tool := range s.tools.AllTools() {
		toolDefn := tool.FunctionDefinition()
		toolInputSchema, err := toolDefn.Parameters.ToRawSchema()
		if err != nil {
			return nil, fmt.Errorf("converting tool schema to json.RawMessage: %w", err)
		}
		s.server.AddTool(mcpgo.NewToolWithRawSchema(
			toolDefn.Name,
			toolDefn.Description,
			toolInputSchema,
		), s.handleToolCall)
	}

	// Initialize MCP manager to get client tools
	manager, err := mcp.InitializeManager()
	if err != nil {
		klog.Warningf("Failed to initialize MCP manager: %v", err)
		return s, nil // Return server with just built-in tools
	}

	// Connect to MCP servers and get their tools
	if err := manager.DiscoverAndConnectServers(ctx); err != nil {
		klog.Warningf("Failed to connect to MCP servers: %v", err)
		return s, nil // Return server with just built-in tools
	}

	// Get tools from all connected MCP servers
	serverTools, err := manager.ListAvailableTools(ctx)
	if err != nil {
		klog.Warningf("Failed to list tools from MCP servers: %v", err)
		return s, nil // Return server with just built-in tools
	}

	// Add tools from MCP servers
	for _, tools := range serverTools {
		for _, tool := range tools {
			// Create a schema for the tool
			schema := &gollm.FunctionDefinition{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters: &gollm.Schema{
					Type: gollm.TypeObject,
					Properties: map[string]*gollm.Schema{
						"args": {
							Type:        gollm.TypeObject,
							Description: "Tool arguments",
						},
					},
				},
			}

			toolInputSchema, err := schema.Parameters.ToRawSchema()
			if err != nil {
				klog.Warningf("Failed to convert tool schema for %s: %v", tool.Name, err)
				continue
			}

			// Add the tool to the server
			s.server.AddTool(mcpgo.NewToolWithRawSchema(
				tool.Name,
				tool.Description,
				toolInputSchema,
			), s.handleToolCall)
		}
	}

	return s, nil
}

func (s *kubectlMCPServer) Serve(ctx context.Context) error {
	return server.ServeStdio(s.server)
}

func (s *kubectlMCPServer) handleToolCall(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	// Find the tool in our tools collection
	tool := s.tools.Lookup(request.Params.Name)
	if tool == nil {
		return &mcpgo.CallToolResult{
			IsError: true,
			Content: []mcpgo.Content{
				mcpgo.TextContent{
					Type: "text",
					Text: fmt.Sprintf("tool %q not found", request.Params.Name),
				},
			},
		}, nil
	}

	// Convert arguments to the expected type
	args, ok := request.Params.Arguments.(map[string]any)
	if !ok {
		return &mcpgo.CallToolResult{
			IsError: true,
			Content: []mcpgo.Content{
				mcpgo.TextContent{
					Type: "text",
					Text: fmt.Sprintf("invalid arguments type: expected map[string]any, got %T", request.Params.Arguments),
				},
			},
		}, nil
	}

	// Execute the tool
	result, err := tool.Run(ctx, args)
	if err != nil {
		return &mcpgo.CallToolResult{
			IsError: true,
			Content: []mcpgo.Content{
				mcpgo.TextContent{
					Type: "text",
					Text: err.Error(),
				},
			},
		}, nil
	}

	// Convert result to string
	var resultStr string
	switch v := result.(type) {
	case string:
		resultStr = v
	default:
		resultStr = fmt.Sprintf("%v", v)
	}

	return &mcpgo.CallToolResult{
		Content: []mcpgo.Content{
			mcpgo.TextContent{
				Type: "text",
				Text: resultStr,
			},
		},
	}, nil
}

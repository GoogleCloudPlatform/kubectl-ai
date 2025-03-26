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

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/tools"
	"github.com/mark3labs/mcp-go/mcp"
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

	ctx = context.WithValue(ctx, "kubeconfig", s.kubectlConfig)
	ctx = context.WithValue(ctx, "work_dir", s.workDir)

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
	output, err := tool.Run(ctx, map[string]any{
		"command": command,
	})
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

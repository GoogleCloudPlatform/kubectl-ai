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
		toolInputSchema, err := toolDefn.Parameters.ToRawSchema()
		if err != nil {
			return nil, fmt.Errorf("converting tool schema to json.RawMessage: %w", err)
		}
		s.server.AddTool(mcp.NewToolWithRawSchema(
			toolDefn.Name,
			toolDefn.Description,
			toolInputSchema,
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

	// In v0.31.0, Arguments is an interface{} that needs type assertion to a map
	argMap, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return mcp.NewToolResultError("Invalid arguments format: expected a map"), nil
	}

	// Safely extract command parameter with type checking
	commandVal, ok := argMap["command"]
	if !ok {
		return mcp.NewToolResultError("Missing required parameter: command"), nil
	}
	command, ok := commandVal.(string)
	if !ok {
		return mcp.NewToolResultError("Parameter 'command' must be a string"), nil
	}

	// Safely extract modifies_resource parameter (optional)
	var modifiesResource string
	if modVal, ok := argMap["modifies_resource"]; ok {
		if modStr, ok := modVal.(string); ok {
			modifiesResource = modStr
		}
	}

	log.Info("Received tool call", "tool", name, "command", command, "modifies_resource", modifiesResource)

	ctx = context.WithValue(ctx, tools.KubeconfigKey, s.kubectlConfig)
	ctx = context.WithValue(ctx, tools.WorkDirKey, s.workDir)

	tool := tools.Lookup(name)
	if tool == nil {
		// Use utility method for error creation in v0.31.0
		return mcp.NewToolResultError(fmt.Sprintf("Tool %s not found", name)), nil
	}
	// Prepare arguments map with command and optional modifies_resource
	args := map[string]any{
		"command": command,
	}

	// Add modifies_resource if available
	if modifiesResource != "" {
		args["modifies_resource"] = modifiesResource
	}

	output, err := tool.Run(ctx, args)
	if err != nil {
		log.Error(err, "Error running tool call")
		// Use the NewToolResultError helper method in v0.31.0
		return mcp.NewToolResultError(fmt.Sprintf("Error running tool: %v", err)), nil
	}

	result, err := tools.ToolResultToMap(output)
	if err != nil {
		log.Error(err, "Error converting tool call output to result")
		// Use the NewToolResultError helper method in v0.31.0
		return mcp.NewToolResultError(fmt.Sprintf("Error processing result: %v", err)), nil
	}

	log.Info("Tool call output", "tool", name, "result", result)

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf("%v", result),
			},
		},
	}, nil
}

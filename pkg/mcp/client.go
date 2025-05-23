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

package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	mcp "github.com/mark3labs/mcp-go/mcp"
	"k8s.io/klog/v2"
)

// Client represents an MCP client that can connect to MCP servers
type Client struct {
	// Name is a friendly name for this MCP server connection
	Name string
	// Command is the command to execute for stdio-based MCP servers
	Command string
	// Args are the arguments to pass to the command
	Args []string
	// Env are the environment variables to set for the command
	Env []string
	// client is the underlying MCP client
	client *mcpclient.Client
	// initialized tracks if the client has been successfully initialized
	initialized bool
	// Note: cmd field removed since NewStdioMCPClient handles the server process automatically
}

// NewClient creates a new MCP client with the given configuration
func NewClient(name, command string, args []string, env map[string]string) *Client {
	// Convert env map to slice of KEY=value strings
	var envSlice []string
	for k, v := range env {
		envSlice = append(envSlice, fmt.Sprintf("%s=%s", k, v))
	}

	return &Client{
		Name:    name,
		Command: command,
		Args:    args,
		Env:     envSlice,
	}
}

// Connect establishes a connection to the MCP server
// It creates an MCP client that will start the server process automatically
func (c *Client) Connect(ctx context.Context) error {
	klog.V(2).InfoS("Connecting to MCP server", "name", c.Name, "command", c.Command, "args", c.Args)
	if c.client != nil && c.initialized {
		return nil // Already connected and initialized
	}

	// Expand the command path to handle ~ and environment variables
	expandedCmd, err := expandPath(c.Command)
	if err != nil {
		return fmt.Errorf("expanding command path: %w", err)
	}

	// Build environment slice properly (similar to reference implementation)
	processEnv := os.Environ()
	envMap := make(map[string]string)
	for _, e := range processEnv {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	// Add custom environment variables
	for _, envVar := range c.Env {
		parts := strings.SplitN(envVar, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	finalEnv := make([]string, 0, len(envMap))
	for k, v := range envMap {
		finalEnv = append(finalEnv, fmt.Sprintf("%s=%s", k, v))
	}

	// Create the MCP client - this will automatically start the server process
	// IMPORTANT: NewStdioMCPClient handles starting the server process internally
	client, err := mcpclient.NewStdioMCPClient(expandedCmd, finalEnv, c.Args...)
	if err != nil {
		return fmt.Errorf("creating MCP client: %w", err)
	}

	c.client = client

	// Initialize the client - this is required before making any requests
	// as per the mcp-go library documentation
	err = c.initialize(ctx)
	if err != nil {
		_ = c.client.Close() // Clean up on error
		c.client = nil
		return fmt.Errorf("initializing MCP client: %w", err)
	}

	klog.V(2).Info("Successfully connected to MCP server", "name", c.Name)
	return nil
}

// initialize performs the MCP initialization handshake
func (c *Client) initialize(ctx context.Context) error {
	if c.initialized {
		klog.V(2).InfoS("Client already initialized", "server", c.Name)
		return nil
	}

	klog.V(2).InfoS("Initializing MCP client", "server", c.Name)

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.Capabilities = mcp.ClientCapabilities{}
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "kubectl-ai-mcp-client",
		Version: "1.0.0",
	}

	_, err := c.client.Initialize(ctx, initReq)
	if err != nil {
		return fmt.Errorf("MCP initialize call failed: %w", err)
	}

	c.initialized = true
	klog.V(2).InfoS("MCP client initialized successfully", "server", c.Name)
	return nil
}

// Close closes the connection to the MCP server
func (c *Client) Close() error {
	var err error

	// Close the client if it exists
	if c.client != nil {
		err = c.client.Close()
		c.client = nil
	}

	// Note: cmd is no longer managed manually since NewStdioMCPClient
	// handles the server process lifecycle automatically

	return err
}

// ListTools lists all available tools from the MCP server
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	if c.client == nil {
		return nil, fmt.Errorf("not connected to MCP server")
	}

	// Ensure the client is initialized before making any tool calls
	if !c.initialized {
		klog.V(2).InfoS("Client not initialized, attempting to initialize", "server", c.Name)
		if err := c.initialize(ctx); err != nil {
			return nil, fmt.Errorf("MCP client not initialized before listing tools: %w", err)
		}
	}

	klog.V(2).InfoS("Listing tools from MCP server", "server", c.Name)

	req := mcp.ListToolsRequest{}
	result, err := c.client.ListTools(ctx, req)

	// Simple retry logic if first attempt fails (from reference implementation)
	if err != nil {
		klog.V(2).InfoS("First ListTools attempt failed, trying ping and retry", "server", c.Name, "error", err)

		// Try ping with a shorter timeout
		pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
		if pingErr := c.client.Ping(pingCtx); pingErr != nil {
			pingCancel()
			klog.V(2).InfoS("Ping also failed", "server", c.Name, "error", pingErr)
			return nil, fmt.Errorf("listing tools failed and ping failed: %w", err)
		} else {
			pingCancel()
			klog.V(2).InfoS("Ping succeeded, retrying ListTools", "server", c.Name)
			result, err = c.client.ListTools(ctx, req) // Retry the call
		}
	}

	if err != nil {
		return nil, fmt.Errorf("listing tools failed even after retry: %w", err)
	}

	if result == nil {
		klog.V(2).InfoS("ListTools returned nil result", "server", c.Name)
		return []Tool{}, nil
	}

	// Convert the result to our simplified Tool type
	var tools []Tool
	for _, tool := range result.Tools {
		tools = append(tools, Tool{
			Name:        tool.Name,
			Description: tool.Description,
		})
		klog.V(2).InfoS("Discovered tool", "name", tool.Name, "description", tool.Description, "server", c.Name)
	}

	klog.V(2).InfoS("Tool discovery completed", "server", c.Name, "tools", len(tools))
	return tools, nil
}

// CallTool calls a tool on the MCP server and returns the result as a string
// The arguments should be a map of parameter names to values that will be passed to the tool
func (c *Client) CallTool(ctx context.Context, toolName string, arguments map[string]interface{}) (string, error) {
	klog.V(2).InfoS("Calling MCP tool", "server", c.Name, "tool", toolName, "args", arguments)
	if c.client == nil {
		return "", fmt.Errorf("not connected to MCP server")
	}

	// Ensure we have a valid context
	if ctx == nil {
		ctx = context.Background()
	}

	// Convert arguments to the format expected by the MCP server
	args := make(map[string]interface{})
	for k, v := range arguments {
		args[k] = v
	}

	// Add the command as an argument if not already present
	if _, ok := args["command"]; !ok {
		args["command"] = toolName
	}

	// Call the tool on the MCP server
	result, err := c.client.CallTool(ctx, mcp.CallToolRequest{
		Params: struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments,omitempty"`
			Meta      *struct {
				ProgressToken mcp.ProgressToken `json:"progressToken,omitempty"`
			} `json:"_meta,omitempty"`
		}{
			Name:      toolName,
			Arguments: args,
		},
	})

	if err != nil {
		return "", fmt.Errorf("calling tool %q: %w", toolName, err)
	}

	// Handle error response
	if result.IsError {
		if len(result.Content) > 0 {
			if textContent, ok := result.Content[0].(mcp.TextContent); ok {
				return "", fmt.Errorf("tool error: %s", textContent.Text)
			}
		}
		return "", fmt.Errorf("tool returned an error")
	}

	// Convert the result to a string
	if len(result.Content) > 0 {
		if textContent, ok := result.Content[0].(mcp.TextContent); ok {
			return textContent.Text, nil
		}
	}

	// If we couldn't extract text content, return a generic message
	return "Tool executed successfully, but no text content was returned", nil
}

// Tool represents an MCP tool
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// expandPath expands the command path, handling ~ and environment variables
// If the path is just a binary name (no path separators), it looks in $PATH
func expandPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path cannot be empty")
	}

	// Expand environment variables first
	expanded := os.ExpandEnv(path)

	// If the command contains no path separators, look it up in $PATH first
	if !strings.Contains(expanded, string(filepath.Separator)) && !strings.HasPrefix(expanded, "~") {
		klog.V(2).InfoS("Attempting PATH lookup for command", "command", expanded)
		// Try to find the command in $PATH
		if pathResolved, err := exec.LookPath(expanded); err == nil {
			klog.V(2).InfoS("Found command in PATH", "command", expanded, "resolved", pathResolved)
			return pathResolved, nil
		} else {
			klog.V(2).InfoS("Command not found in PATH", "command", expanded, "error", err)
		}
		// If not found in PATH, continue with the original logic below
		klog.V(2).InfoS("Command not found in PATH, trying relative to current directory", "command", expanded)
	} else {
		klog.V(2).InfoS("Skipping PATH lookup", "command", expanded, "hasPathSeparator", strings.Contains(expanded, string(filepath.Separator)), "hasTilde", strings.HasPrefix(expanded, "~"))
	}

	// Handle ~ for home directory
	if strings.HasPrefix(expanded, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("getting home directory: %w", err)
		}
		expanded = filepath.Join(home, expanded[1:])
	}

	// Clean the path to remove any . or .. elements
	expanded = filepath.Clean(expanded)

	// Make the path absolute if it's not already
	if !filepath.IsAbs(expanded) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("getting current working directory: %w", err)
		}
		expanded = filepath.Clean(filepath.Join(cwd, expanded))
	}

	// Verify the file exists and is executable
	info, err := os.Stat(expanded)
	if err != nil {
		return "", fmt.Errorf("checking path %q: %w", expanded, err)
	}

	// Check if it's a regular file and executable
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("path %q is not a regular file", expanded)
	}

	// Check if the file is executable by the current user
	if info.Mode().Perm()&0111 == 0 {
		return "", fmt.Errorf("file %q is not executable", expanded)
	}

	return expanded, nil
}

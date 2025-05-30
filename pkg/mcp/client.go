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
	// Note: cmd field removed since NewStdioMCPClient handles the server process automatically
}

// NewClient creates a new MCP client with the given configuration
// TODO(tuannvm): add support for HTTP streamable MCP servers
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
	if c.client != nil {
		return nil // Already connected
	}

	// Step 1: Prepare environment and command
	expandedCmd, finalEnv, err := c.prepareEnvironment()
	if err != nil {
		return fmt.Errorf("preparing environment: %w", err)
	}

	// Step 2: Create the MCP client
	if err := c.createMCPClient(expandedCmd, finalEnv); err != nil {
		return fmt.Errorf("creating MCP client: %w", err)
	}

	// Step 3: Initialize the connection
	if err := c.initializeConnection(ctx); err != nil {
		c.cleanup()
		return fmt.Errorf("initializing connection: %w", err)
	}

	// Step 4: Verify the connection
	if err := c.verifyConnection(ctx); err != nil {
		c.cleanup()
		return fmt.Errorf("verifying connection: %w", err)
	}

	klog.V(2).Info("Successfully connected to MCP server", "name", c.Name)
	return nil
}

// prepareEnvironment expands the command path and merges environment variables
func (c *Client) prepareEnvironment() (string, []string, error) {
	// Expand the command path to handle ~ and environment variables
	expandedCmd, err := expandPath(c.Command)
	if err != nil {
		return "", nil, fmt.Errorf("expanding command path: %w", err)
	}

	// Build proper environment slice by merging process environment with custom env
	finalEnv := mergeEnvironmentVariables(os.Environ(), c.Env)

	return expandedCmd, finalEnv, nil
}

// createMCPClient creates the underlying MCP client
func (c *Client) createMCPClient(command string, env []string) error {
	client, err := mcpclient.NewStdioMCPClient(command, env, c.Args...)
	if err != nil {
		return fmt.Errorf("creating stdio MCP client: %w", err)
	}
	c.client = client
	return nil
}

// initializeConnection initializes the MCP connection with proper handshake
func (c *Client) initializeConnection(ctx context.Context) error {
	initCtx, initCancel := withTimeout(ctx, DefaultConnectionTimeout)
	defer initCancel()

	initReq := c.buildInitializeRequest()
	_, err := c.client.Initialize(initCtx, initReq)
	if err != nil {
		return fmt.Errorf("initializing MCP client: %w", err)
	}

	return nil
}

// verifyConnection verifies the connection works by testing tool listing with retry
func (c *Client) verifyConnection(ctx context.Context) error {
	verifyCtx, verifyCancel := withTimeout(ctx, DefaultVerificationTimeout)
	defer verifyCancel()

	_, err := c.ListTools(verifyCtx)
	if err != nil {
		klog.V(2).InfoS("First ListTools attempt failed, trying ping and retry", "server", c.Name, "error", err)
		return c.retryConnectionWithPing(ctx)
	}

	return nil
}

// retryConnectionWithPing attempts to ping the server and retry ListTools
func (c *Client) retryConnectionWithPing(ctx context.Context) error {
	// Try ping to check if server is responsive
	pingCtx, pingCancel := withTimeout(ctx, DefaultPingTimeout)
	defer pingCancel()

	if err := c.client.Ping(pingCtx); err != nil {
		klog.V(2).InfoS("Ping also failed", "server", c.Name, "error", err)
		return fmt.Errorf("server ping failed: %w", err)
	}

	klog.V(2).InfoS("Ping succeeded, retrying ListTools", "server", c.Name)

	// Retry ListTools after successful ping
	retryCtx, retryCancel := withTimeout(ctx, DefaultVerificationTimeout)
	defer retryCancel()

	_, err := c.ListTools(retryCtx)
	if err != nil {
		return fmt.Errorf("ListTools retry failed: %w", err)
	}

	return nil
}

// buildInitializeRequest creates the MCP initialize request
func (c *Client) buildInitializeRequest() mcp.InitializeRequest {
	return mcp.InitializeRequest{
		// In v0.31.0, InitializeRequest uses the InitializeParams struct directly
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			Capabilities:    mcp.ClientCapabilities{},
			ClientInfo: mcp.Implementation{
				Name:    ClientName,
				Version: ClientVersion,
			},
		},
	}
}

// cleanup closes the client connection and resets the client state
func (c *Client) cleanup() {
	if c.client != nil {
		_ = c.client.Close()
		c.client = nil
	}
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
	if err := c.ensureConnected(); err != nil {
		return nil, err
	}

	// Call the ListTools method on the MCP server
	result, err := c.client.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("listing tools: %w", err)
	}

	// Convert the result using the helper function
	tools := convertMCPToolsToTools(result.Tools)

	klog.V(2).InfoS("Listed tools from MCP server", "count", len(tools), "server", c.Name)
	return tools, nil
}

// CallTool calls a tool on the MCP server and returns the result as a string
// The arguments should be a map of parameter names to values that will be passed to the tool
func (c *Client) CallTool(ctx context.Context, toolName string, arguments map[string]interface{}) (string, error) {
	klog.V(2).InfoS("Calling MCP tool", "server", c.Name, "tool", toolName, "args", arguments)

	if err := c.ensureConnected(); err != nil {
		return "", err
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

	// In v0.31.0, CallToolRequest has a simplified structure with Params field
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      toolName,
			Arguments: args,
		},
	}

	// Call the tool on the MCP server
	result, err := c.client.CallTool(ctx, request)

	if err != nil {
		return "", fmt.Errorf(ErrToolCallFmt, toolName, err)
	}

	// Handle error response
	if result.IsError {
		if len(result.Content) > 0 {
			if textContent, ok := mcp.AsTextContent(result.Content[0]); ok {
				return "", fmt.Errorf("tool error: %s", textContent.Text)
			}
		}
		return "", fmt.Errorf("tool returned an error")
	}

	// Convert the result to a string using utility method in v0.31.0
	if len(result.Content) > 0 {
		if textContent, ok := mcp.AsTextContent(result.Content[0]); ok {
			return textContent.Text, nil
		}
	}

	// If we couldn't extract text content, return a generic message
	return "Tool executed successfully, but no text content was returned", nil
}

// Tool represents an MCP tool with optional server information
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Server      string `json:"server,omitempty"`
}

// NewTool creates a new tool with basic information
func NewTool(name, description string) Tool {
	return Tool{
		Name:        name,
		Description: description,
	}
}

// NewToolWithServer creates a new tool with server information
func NewToolWithServer(name, description, server string) Tool {
	return Tool{
		Name:        name,
		Description: description,
		Server:      server,
	}
}

// WithServer returns a copy of the tool with server information added
func (t Tool) WithServer(server string) Tool {
	return Tool{
		Name:        t.Name,
		Description: t.Description,
		Server:      server,
	}
}

// ID returns a unique identifier for the tool
func (t Tool) ID() string {
	if t.Server != "" {
		return fmt.Sprintf("%s/%s", t.Server, t.Name)
	}
	return t.Name
}

// String returns a human-readable representation of the tool
func (t Tool) String() string {
	if t.Server != "" {
		return fmt.Sprintf("%s [%s]: %s", t.Name, t.Server, t.Description)
	}
	return fmt.Sprintf("%s: %s", t.Name, t.Description)
}

// AsBasicTool returns the tool without server information (for client.ListTools compatibility)
func (t Tool) AsBasicTool() Tool {
	return Tool{
		Name:        t.Name,
		Description: t.Description,
	}
}

// IsFromServer checks if the tool belongs to a specific server
func (t Tool) IsFromServer(server string) bool {
	return t.Server == server
}

// convertMCPToolsToTools converts MCP library tools to our Tool type
func convertMCPToolsToTools(mcpTools []mcp.Tool) []Tool {
	tools := make([]Tool, 0, len(mcpTools))
	for _, tool := range mcpTools {
		tools = append(tools, Tool{
			Name:        tool.Name,
			Description: tool.Description,
		})
	}
	return tools
}

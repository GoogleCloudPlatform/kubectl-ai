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

	mcpclient "github.com/mark3labs/mcp-go/client"
	mcp "github.com/mark3labs/mcp-go/mcp"
	"k8s.io/klog/v2"
)

// ===================================================================
// Client Types and Factory Functions
// ===================================================================

// Client represents an MCP client that can connect to MCP servers.
// It is a wrapper around the MCPClient interface for backward compatibility.
type Client struct {
	// Name is a friendly name for this MCP server connection
	Name string
	// The actual client implementation (stdio or HTTP)
	impl MCPClient
	// client is the underlying MCP library client
	client *mcpclient.Client
}

// Tool represents an MCP tool with optional server information.
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Server      string `json:"server,omitempty"`
}

// NewClient creates a new MCP client with the given configuration.
// This function supports both stdio and HTTP-based MCP servers.
func NewClient(config ClientConfig) *Client {
	// Create the appropriate implementation based on configuration
	var impl MCPClient
	if config.URL != "" {
		// HTTP-based client
		impl = NewHTTPClient(config)
	} else {
		// Stdio-based client
		impl = NewStdioClient(config)
	}

	return &Client{
		Name: config.Name,
		impl: impl,
	}
}

// CreateStdioClient creates a new stdio-based MCP client (for backward compatibility).
func CreateStdioClient(name, command string, args []string, env map[string]string) *Client {
	// Convert env map to slice of KEY=value strings
	var envSlice []string
	for k, v := range env {
		envSlice = append(envSlice, fmt.Sprintf("%s=%s", k, v))
	}

	config := ClientConfig{
		Name:    name,
		Command: command,
		Args:    args,
		Env:     envSlice,
	}

	return NewClient(config)
}

// ===================================================================
// Main Client Interface Methods
// ===================================================================

// Connect establishes a connection to the MCP server.
// This delegates to the appropriate implementation (stdio or HTTP).
func (c *Client) Connect(ctx context.Context) error {
	klog.V(2).InfoS("Connecting to MCP server", "name", c.Name)

	// Delegate to the implementation
	if err := c.impl.Connect(ctx); err != nil {
		return err
	}

	// Store the underlying client for backward compatibility
	c.client = c.impl.getUnderlyingClient()

	klog.V(2).InfoS("Successfully connected to MCP server", "name", c.Name)
	return nil
}

// Close closes the connection to the MCP server.
func (c *Client) Close() error {
	if c.impl == nil {
		return nil // Not initialized
	}

	klog.V(2).InfoS("Closing connection to MCP server", "name", c.Name)

	// Delegate to implementation
	err := c.impl.Close()
	c.client = nil // Clear reference to underlying client

	if err != nil {
		return fmt.Errorf("closing MCP client: %w", err)
	}

	return nil
}

// ListTools lists all available tools from the MCP server.
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	if err := c.ensureConnected(); err != nil {
		return nil, err
	}

	// Delegate to implementation
	tools, err := c.impl.ListTools(ctx)
	if err != nil {
		return nil, err
	}

	klog.V(2).InfoS("Listed tools from MCP server", "count", len(tools), "server", c.Name)
	return tools, nil
}

// CallTool calls a tool on the MCP server and returns the result as a string.
// The arguments should be a map of parameter names to values that will be passed to the tool.
func (c *Client) CallTool(ctx context.Context, toolName string, arguments map[string]interface{}) (string, error) {
	klog.V(2).InfoS("Calling MCP tool", "server", c.Name, "tool", toolName, "args", arguments)

	if err := c.ensureConnected(); err != nil {
		return "", err
	}

	// Ensure we have a valid context
	if ctx == nil {
		ctx = context.Background()
	}

	// Delegate to implementation
	return c.impl.CallTool(ctx, toolName, arguments)
}

// ===================================================================
// Tool Factory Functions and Methods
// ===================================================================

// NewTool creates a new tool with basic information.
func NewTool(name, description string) Tool {
	return Tool{
		Name:        name,
		Description: description,
	}
}

// NewToolWithServer creates a new tool with server information.
func NewToolWithServer(name, description, server string) Tool {
	return Tool{
		Name:        name,
		Description: description,
		Server:      server,
	}
}

// WithServer returns a copy of the tool with server information added.
func (t Tool) WithServer(server string) Tool {
	copy := t
	copy.Server = server
	return copy
}

// ID returns a unique identifier for the tool.
func (t Tool) ID() string {
	if t.Server != "" {
		return fmt.Sprintf("%s@%s", t.Name, t.Server)
	}
	return t.Name
}

// String returns a human-readable representation of the tool.
func (t Tool) String() string {
	if t.Server != "" {
		return fmt.Sprintf("%s (from %s)", t.Name, t.Server)
	}
	return t.Name
}

// AsBasicTool returns the tool without server information (for client.ListTools compatibility).
func (t Tool) AsBasicTool() Tool {
	copy := t
	copy.Server = ""
	return copy
}

// IsFromServer checks if the tool belongs to a specific server.
func (t Tool) IsFromServer(server string) bool {
	return t.Server == server
}

// convertMCPToolsToTools converts MCP library tools to our Tool type.
func convertMCPToolsToTools(mcpTools []mcp.Tool) []Tool {
	tools := make([]Tool, 0, len(mcpTools))
	for _, mcpTool := range mcpTools {
		tools = append(tools, Tool{
			Name:        mcpTool.Name,
			Description: mcpTool.Description,
		})
	}
	return tools
}

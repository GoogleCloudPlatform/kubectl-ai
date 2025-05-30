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
	"reflect"

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

// ===================================================================
// Common Functions
// ===================================================================

// ensureClientConnected checks if the client is connected.
func ensureClientConnected(client *mcpclient.Client) error {
	if client == nil {
		return fmt.Errorf("client not connected")
	}
	return nil
}

// initializeClientConnection initializes the MCP connection with proper handshake.
func initializeClientConnection(ctx context.Context, client *mcpclient.Client) error {
	initCtx, cancel := context.WithTimeout(ctx, DefaultConnectionTimeout)
	defer cancel()

	// Create initialize request with the structure expected by v0.31.0
	initReq := mcp.InitializeRequest{
		// The structure might differ in v0.31.0 - adapt as needed
		// This is a placeholder that will be updated when the actual API is known
	}

	_, err := client.Initialize(initCtx, initReq)
	if err != nil {
		return fmt.Errorf("initializing MCP client: %w", err)
	}

	return nil
}

// verifyClientConnection verifies the connection works by testing tool listing.
func verifyClientConnection(ctx context.Context, client *mcpclient.Client) error {
	verifyCtx, cancel := context.WithTimeout(ctx, DefaultConnectionTimeout)
	defer cancel()

	// Try to list tools as a basic connectivity test
	_, err := client.ListTools(verifyCtx, mcp.ListToolsRequest{})
	if err != nil {
		return fmt.Errorf("listing tools: %w", err)
	}

	return nil
}

// cleanupClient closes the client connection safely.
func cleanupClient(client **mcpclient.Client) {
	if *client != nil {
		_ = (*client).Close() // Ignore errors on cleanup
		*client = nil
	}
}

// processToolResponse processes a tool call response and extracts the text result.
// This function works with any MCP response object that has the expected fields.
func processToolResponse(result any) (string, error) {
	// Use reflection to safely access fields
	rv := reflect.ValueOf(result)

	// Handle pointer to struct
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}

	if rv.Kind() != reflect.Struct {
		return "", fmt.Errorf("unexpected response type: %T", result)
	}

	// Check for IsError field
	isErrorField := rv.FieldByName("IsError")
	if isErrorField.IsValid() && isErrorField.Kind() == reflect.Bool {
		isError := isErrorField.Bool()

		// Handle error response
		if isError {
			// Try to extract error message if possible
			return "", fmt.Errorf("tool returned an error")
		}
	}

	// Check for Content field
	contentField := rv.FieldByName("Content")
	if contentField.IsValid() && contentField.Len() > 0 {
		// Let's rely on the AsTextContent method from MCP package
		// which handles the specific response format
		content := contentField.Index(0).Interface()
		if textContent, ok := mcp.AsTextContent(content); ok {
			return textContent.Text, nil
		}
	}

	// If we couldn't extract text content, return a generic message
	return "Tool executed successfully, but no text content was returned", nil
}

// listClientTools implements the common ListTools functionality shared by both client types.
func listClientTools(ctx context.Context, client *mcpclient.Client, serverName string) ([]Tool, error) {
	if err := ensureClientConnected(client); err != nil {
		return nil, err
	}

	// Call the ListTools method on the MCP server
	result, err := client.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("listing tools: %w", err)
	}

	// Convert the result using the helper function
	tools := convertMCPToolsToTools(result.Tools)

	// Add the server name to each tool
	for i := range tools {
		tools[i].Server = serverName
	}

	return tools, nil
}

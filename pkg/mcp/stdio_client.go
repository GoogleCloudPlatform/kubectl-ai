package mcp

import (
	"context"
	"fmt"

	mcpclient "github.com/mark3labs/mcp-go/client"
	mcp "github.com/mark3labs/mcp-go/mcp"
	"k8s.io/klog/v2"
)

// ===================================================================
// Stdio Client Implementation
// ===================================================================

// stdioClient is an MCP client that communicates via standard I/O
type stdioClient struct {
	name    string
	command string
	args    []string
	env     []string
	client  *mcpclient.Client
}

// NewStdioClient creates a new stdio-based MCP client
func NewStdioClient(config ClientConfig) MCPClient {
	return &stdioClient{
		name:    config.Name,
		command: config.Command,
		args:    config.Args,
		env:     config.Env,
	}
}

// getUnderlyingClient returns the underlying MCP client.
func (c *stdioClient) getUnderlyingClient() *mcpclient.Client {
	return c.client
}

// ensureConnected makes sure the client is connected.
func (c *stdioClient) ensureConnected() error {
	if c.client == nil {
		return fmt.Errorf("client not connected")
	}
	return nil
}

// Name returns the name of this client.
func (c *stdioClient) Name() string {
	return c.name
}

// Connect establishes a connection to the stdio MCP server.
func (c *stdioClient) Connect(ctx context.Context) error {
	klog.V(2).InfoS("Connecting to stdio MCP server", "name", c.name, "command", c.command)
	if c.client != nil {
		return nil // Already connected
	}

	// Expand the command path and prepare the environment
	expandedCmd, err := expandPath(c.command)
	if err != nil {
		return fmt.Errorf("expanding command path: %w", err)
	}

	// Create the stdio MCP client
	client, err := mcpclient.NewStdioMCPClient(expandedCmd, c.env, c.args...)
	if err != nil {
		return fmt.Errorf("creating stdio MCP client: %w", err)
	}

	c.client = client

	// Initialize the connection
	if err := c.initializeConnection(ctx); err != nil {
		c.cleanup()
		return fmt.Errorf("initializing connection: %w", err)
	}

	// Verify the connection
	if err := c.verifyConnection(ctx); err != nil {
		c.cleanup()
		return fmt.Errorf("verifying connection: %w", err)
	}

	klog.V(2).InfoS("Successfully connected to stdio MCP server", "name", c.name)
	return nil
}

// initializeConnection initializes the MCP connection with proper handshake
func (c *stdioClient) initializeConnection(ctx context.Context) error {
	initCtx, cancel := context.WithTimeout(ctx, DefaultConnectionTimeout)
	defer cancel()

	// Create initialize request with the structure expected by v0.31.0
	initReq := mcp.InitializeRequest{
		// The structure might differ in v0.31.0 - adapt as needed
		// This is a placeholder that will be updated when the actual API is known
	}

	_, err := c.client.Initialize(initCtx, initReq)
	if err != nil {
		return fmt.Errorf("initializing MCP client: %w", err)
	}

	return nil
}

// verifyConnection verifies the connection works by testing tool listing
func (c *stdioClient) verifyConnection(ctx context.Context) error {
	verifyCtx, cancel := context.WithTimeout(ctx, DefaultConnectionTimeout)
	defer cancel()

	// Try to list tools as a basic connectivity test
	_, err := c.client.ListTools(verifyCtx, mcp.ListToolsRequest{})
	if err != nil {
		return fmt.Errorf("listing tools: %w", err)
	}

	return nil
}

// cleanup closes the client connection and resets the client state
func (c *stdioClient) cleanup() {
	if c.client != nil {
		_ = c.client.Close() // Ignore errors on cleanup
		c.client = nil
	}
}

// Close closes the connection to the MCP server
func (c *stdioClient) Close() error {
	if c.client == nil {
		return nil // Already closed
	}

	klog.V(2).InfoS("Closing connection to stdio MCP server", "name", c.name)
	err := c.client.Close()
	c.client = nil

	if err != nil {
		return fmt.Errorf("closing MCP client: %w", err)
	}

	return nil
}

// ListTools lists all available tools from the MCP server
func (c *stdioClient) ListTools(ctx context.Context) ([]Tool, error) {
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

	klog.V(2).InfoS("Listed tools from stdio MCP server", "count", len(tools), "server", c.name)
	return tools, nil
}

// CallTool calls a tool on the MCP server and returns the result as a string
func (c *stdioClient) CallTool(ctx context.Context, toolName string, arguments map[string]interface{}) (string, error) {
	klog.V(2).InfoS("Calling MCP tool via stdio", "server", c.name, "tool", toolName)

	if err := c.ensureConnected(); err != nil {
		return "", err
	}

	// Create v0.31.0 compatible request
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      toolName,
			Arguments: arguments,
		},
	}

	// Call the tool on the MCP server
	result, err := c.client.CallTool(ctx, request)
	if err != nil {
		return "", fmt.Errorf("error calling tool %s: %w", toolName, err)
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

	// Extract result using v0.31.0 helper methods
	if len(result.Content) > 0 {
		if textContent, ok := mcp.AsTextContent(result.Content[0]); ok {
			return textContent.Text, nil
		}
	}

	// If we couldn't extract text content, return a generic message
	return "Tool executed successfully, but no text content was returned", nil
}

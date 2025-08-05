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
	return ensureClientConnected(c.client)
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
	return initializeClientConnection(ctx, c.client)
}

// verifyConnection verifies the connection works by testing tool listing
func (c *stdioClient) verifyConnection(ctx context.Context) error {
	return verifyClientConnection(ctx, c.client)
}

// cleanup closes the client connection and resets the client state
func (c *stdioClient) cleanup() {
	cleanupClient(&c.client)
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
	tools, err := listClientTools(ctx, c.client, c.name)
	if err != nil {
		return nil, err
	}

	klog.V(2).InfoS("Listed tools from stdio MCP server", "count", len(tools), "server", c.name)
	return tools, nil
}

// ListResources lists all available resources from the MCP server
func (c *stdioClient) ListResources(ctx context.Context) ([]Resource, error) {
	resources, err := listClientResources(ctx, c.client, c.name)
	if err != nil {
		return nil, err
	}

	klog.V(2).InfoS("Listed resources from stdio MCP server", "count", len(resources), "server", c.name)
	return resources, nil
}

// ReadResource reads a resource from the MCP server and returns its text content
func (c *stdioClient) ReadResource(ctx context.Context, uri string) (string, error) {
	klog.V(2).InfoS("Reading MCP resource via stdio", "server", c.name, "uri", uri)

	if err := c.ensureConnected(); err != nil {
		return "", err
	}

	return readClientResource(ctx, c.client, uri)
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

	return processToolResponse(result)
}

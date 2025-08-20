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

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
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

	InputSchema *gollm.Schema `json:"inputSchema,omitempty"`
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

	// Delegate to implementation
	return c.impl.CallTool(ctx, toolName, arguments)
}

// ===================================================================
// Tool Factory Functions and Methods
// ===================================================================

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
func convertMCPToolsToTools(mcpTools []mcp.Tool) ([]Tool, error) {
	tools := make([]Tool, 0, len(mcpTools))
	for _, mcpTool := range mcpTools {
		tool := Tool{
			Name:        mcpTool.Name,
			Description: mcpTool.Description,
		}
		// TODO: Annotations (give hints about e.g. read-only, destructive, idempotent, open-world)

		if mcpTool.InputSchema.Type != "" {
			schema, err := convertMCPInputSchema(&mcpTool.InputSchema)
			if err != nil {
				return nil, fmt.Errorf("converting MCP input schema to tool input schema: %w", err)
			}
			tool.InputSchema = schema
		} else {
			// TODO: Use RawInputSchema if available
			// klog.Warningf("no input schema for tool %s", mcpTool.Name)
			return nil, fmt.Errorf("no input schema for tool %s", mcpTool.Name)
		}

		tools = append(tools, tool)
	}
	return tools, nil
}

func convertMCPInputSchema(mcpInputSchema *mcp.ToolInputSchema) (*gollm.Schema, error) {
	gollmSchema := &gollm.Schema{}
	switch mcpInputSchema.Type {
	case "string":
		gollmSchema.Type = gollm.TypeString
	// case "number":
	// 	gollmSchema.Type = gollm.TypeNumber
	case "boolean":
		gollmSchema.Type = gollm.TypeBoolean
	case "object":
		gollmSchema.Type = gollm.TypeObject
	default:
		return nil, fmt.Errorf("unexpected MCP input schema type: %s", mcpInputSchema.Type)
	}
	if mcpInputSchema.Properties != nil {
		gollmSchema.Properties = make(map[string]*gollm.Schema)
		for key, value := range mcpInputSchema.Properties {
			if valueSchema, ok := value.(mcp.ToolInputSchema); ok {
				gollmValue, err := convertMCPInputSchema(&valueSchema)
				if err != nil {
					return nil, fmt.Errorf("converting MCP input schema to tool input schema: %w", err)
				}
				gollmSchema.Properties[key] = gollmValue
			} else if valueMap, ok := value.(map[string]interface{}); ok {
				gollmValue, err := convertMCPMapSchema(key, valueMap)
				if err != nil {
					return nil, fmt.Errorf("converting MCP input schema to tool input schema: %w", err)
				}
				gollmSchema.Properties[key] = gollmValue
			} else {
				return nil, fmt.Errorf("unexpected input schema type for %q: %T %+v", key, value, value)
			}
		}
	}
	gollmSchema.Required = mcpInputSchema.Required
	return gollmSchema, nil
}

func convertMCPMapSchema(key string, schemaMap map[string]interface{}) (*gollm.Schema, error) {
	gollmSchema := &gollm.Schema{}

	if descriptionObj, ok := schemaMap["description"]; ok {
		description, ok := descriptionObj.(string)
		if !ok {
			return nil, fmt.Errorf("unexpected description for key %q: %+v", key, schemaMap)
		}
		gollmSchema.Description = description
	}

	mcpType, ok := schemaMap["type"].(string)
	if !ok {
		// Fallback: treat any unrecognized schema as generic object
		klog.V(2).InfoS("Unrecognized schema format, treating as object", "key", key)
		gollmSchema.Type = gollm.TypeObject
		return gollmSchema, nil
	}
	switch mcpType {
	case "string":
		gollmSchema.Type = gollm.TypeString
	case "number":
		gollmSchema.Type = gollm.TypeNumber
	case "integer":
		gollmSchema.Type = gollm.TypeNumber
	case "boolean":
		gollmSchema.Type = gollm.TypeBoolean
	case "array":
		items, ok := schemaMap["items"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("did not find items for array: key %q: %+v", key, schemaMap)
		}
		itemsSchema, err := convertMCPMapSchema(key+".items", items)
		if err != nil {
			return nil, fmt.Errorf("converting MCP input schema to tool input schema: %w", err)
		}
		gollmSchema.Type = gollm.TypeArray
		gollmSchema.Items = itemsSchema

	case "object":
		gollmSchema.Type = gollm.TypeObject
		gollmSchema.Properties = make(map[string]*gollm.Schema)
		for key, value := range schemaMap["properties"].(map[string]interface{}) {
			propertySchema, err := convertMCPMapSchema(key, value.(map[string]interface{}))
			if err != nil {
				return nil, fmt.Errorf("converting MCP input schema to tool input schema: %w", err)
			}
			gollmSchema.Properties[key] = propertySchema
		}
	default:
		return nil, fmt.Errorf("unexpected input schema type %q for key %q: %+v", mcpType, key, schemaMap)
	}

	return gollmSchema, nil
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
			// Extract error message
			errorMsg := fmt.Sprintf("%+v", result)
			
			// Try to get message from Content field
			contentField := rv.FieldByName("Content")
			if contentField.IsValid() && contentField.Len() > 0 {
				if content := contentField.Index(0).Interface(); content != nil {
					if textContent, ok := mcp.AsTextContent(content); ok {
						errorMsg = textContent.Text
					}
				}
			}
			
			// Return JSON error data instead of Go error
			return fmt.Sprintf(`{"error": true, "message": %q, "status": "failed"}`, errorMsg), nil
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
	tools, err := convertMCPToolsToTools(result.Tools)
	if err != nil {
		return nil, fmt.Errorf("parsing tools from MCP server: %w", err)
	}

	// Add the server name to each tool
	for i := range tools {
		tools[i].Server = serverName
	}

	return tools, nil
}

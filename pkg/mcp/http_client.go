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
	"encoding/base64"
	"fmt"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	mcp "github.com/mark3labs/mcp-go/mcp"
	"k8s.io/klog/v2"
)

// ===================================================================
// HTTP Client Implementation
// ===================================================================

// httpClient is an MCP client that communicates with HTTP-based MCP servers
type httpClient struct {
	name         string
	url          string
	auth         *AuthConfig
	oauthConfig  *OAuthConfig
	timeout      int
	useStreaming bool
	client       *mcpclient.Client
}

// NewHTTPClient creates a new HTTP-based MCP client
func NewHTTPClient(config ClientConfig) MCPClient {
	return &httpClient{
		name:         config.Name,
		url:          config.URL,
		auth:         config.Auth,
		oauthConfig:  config.OAuthConfig,
		timeout:      config.Timeout,
		useStreaming: config.UseStreaming,
	}
}

// getUnderlyingClient returns the underlying MCP client.
func (c *httpClient) getUnderlyingClient() *mcpclient.Client {
	return c.client
}

// ensureConnected makes sure the client is connected.
func (c *httpClient) ensureConnected() error {
	return ensureClientConnected(c.client)
}

// Name returns the name of this client.
func (c *httpClient) Name() string {
	return c.name
}

// Connect establishes a connection to the HTTP MCP server.
func (c *httpClient) Connect(ctx context.Context) error {
	klog.V(2).InfoS("Connecting to HTTP MCP server", "name", c.name, "url", c.url)
	if c.client != nil {
		return nil // Already connected
	}

	var client *mcpclient.Client
	var err error

	// Create the appropriate client based on configuration
	if c.oauthConfig != nil {
		client, err = c.createOAuthClient(ctx)
	} else if c.useStreaming {
		client, err = c.createStreamingClient()
	} else {
		client, err = c.createStandardClient()
	}

	if err != nil {
		return fmt.Errorf("creating HTTP MCP client: %w", err)
	}

	c.client = client

	// Initialize the connection
	if err := c.initializeConnection(ctx); err != nil {
		c.cleanup()
		return fmt.Errorf("initializing connection: %w", err)
	}

	// Verify connection
	if err := c.verifyConnection(ctx); err != nil {
		c.cleanup()
		return fmt.Errorf("verifying connection: %w", err)
	}

	klog.V(2).InfoS("Successfully connected to HTTP MCP server", "name", c.name)
	return nil
}

// createStreamingClient creates a streamable HTTP client for better performance
func (c *httpClient) createStreamingClient() (*mcpclient.Client, error) {
	// Set up options for the HTTP client
	var options []transport.StreamableHTTPCOption

	// Add timeout if specified
	if c.timeout > 0 {
		options = append(options, transport.WithHTTPTimeout(time.Duration(c.timeout)*time.Second))
	}

	// Add authentication if specified
	if c.auth != nil {
		// Prepare headers map for authentication
		headers := make(map[string]string)

		switch c.auth.Type {
		case "basic":
			auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(c.auth.Username+":"+c.auth.Password))
			headers["Authorization"] = auth
			klog.V(3).InfoS("Using basic auth for HTTP client", "server", c.name)
		case "bearer":
			headers["Authorization"] = "Bearer " + c.auth.Token
			klog.V(3).InfoS("Using bearer auth for HTTP client", "server", c.name)
		case "api-key":
			headerName := "X-Api-Key"
			if c.auth.HeaderName != "" {
				headerName = c.auth.HeaderName
			}
			headers[headerName] = c.auth.ApiKey
			klog.V(3).InfoS("Using API key auth for HTTP client", "server", c.name)
		}

		// Add headers if any were set
		if len(headers) > 0 {
			options = append(options, transport.WithHTTPHeaders(headers))
		}
	}

	klog.V(4).InfoS("Creating streamable HTTP client", "server", c.name, "url", c.url)
	client, err := mcpclient.NewStreamableHttpClient(c.url, options...)
	if err != nil {
		return nil, fmt.Errorf("creating streamable HTTP client: %w", err)
	}

	return client, nil
}

// createStandardClient creates a standard HTTP client
func (c *httpClient) createStandardClient() (*mcpclient.Client, error) {
	// Standard client delegates to streaming client implementation for now
	// In the future, they might have different configurations
	return c.createStreamingClient()
}

// createOAuthClient creates an HTTP client with OAuth authentication
func (c *httpClient) createOAuthClient(ctx context.Context) (*mcpclient.Client, error) {
	if c.oauthConfig == nil {
		return nil, fmt.Errorf("OAuth config required but not provided")
	}

	klog.V(3).InfoS("Creating OAuth HTTP client", "server", c.name, "client_id", c.oauthConfig.ClientID)

	// Set up options for the HTTP client
	var options []transport.StreamableHTTPCOption

	// Create OAuth configuration for the transport
	oauthCfg := transport.OAuthConfig{
		ClientID:     c.oauthConfig.ClientID,
		ClientSecret: c.oauthConfig.ClientSecret,
		Scopes:       c.oauthConfig.Scopes,
		RedirectURI:  c.oauthConfig.RedirectURL,
		// Use the token URL as the auth server metadata URL if available
		AuthServerMetadataURL: c.oauthConfig.TokenURL,
	}

	// Add OAuth configuration
	options = append(options, transport.WithOAuth(oauthCfg))

	// Add timeout if specified
	if c.timeout > 0 {
		options = append(options, transport.WithHTTPTimeout(time.Duration(c.timeout)*time.Second))
	}

	klog.V(4).InfoS("Creating OAuth streamable HTTP client", "server", c.name, "url", c.url)
	client, err := mcpclient.NewStreamableHttpClient(c.url, options...)
	if err != nil {
		return nil, fmt.Errorf("creating OAuth HTTP client: %w", err)
	}

	return client, nil
}

// initializeConnection initializes the MCP connection with proper handshake
func (c *httpClient) initializeConnection(ctx context.Context) error {
	return initializeClientConnection(ctx, c.client)
}

// verifyConnection verifies the connection works by testing tool listing
func (c *httpClient) verifyConnection(ctx context.Context) error {
	return verifyClientConnection(ctx, c.client)
}

// cleanup closes the client connection and resets the client state
func (c *httpClient) cleanup() {
	cleanupClient(&c.client)
}

// Close closes the connection to the MCP server
func (c *httpClient) Close() error {
	if c.client == nil {
		return nil // Already closed
	}

	klog.V(2).InfoS("Closing connection to HTTP MCP server", "name", c.name)
	err := c.client.Close()
	c.client = nil

	if err != nil {
		return fmt.Errorf("closing MCP client: %w", err)
	}

	return nil
}

// ListTools lists all available tools from the MCP server
func (c *httpClient) ListTools(ctx context.Context) ([]Tool, error) {
	tools, err := listClientTools(ctx, c.client, c.name)
	if err != nil {
		return nil, err
	}

	klog.V(2).InfoS("Listed tools from HTTP MCP server", "count", len(tools), "server", c.name)
	return tools, nil
}

// ListResources lists all available resources from the MCP server
func (c *httpClient) ListResources(ctx context.Context) ([]Resource, error) {
	resources, err := listClientResources(ctx, c.client, c.name)
	if err != nil {
		return nil, err
	}

	klog.V(2).InfoS("Listed resources from HTTP MCP server", "count", len(resources), "server", c.name)
	return resources, nil
}

// ReadResource reads a resource from the MCP server and returns its text content
func (c *httpClient) ReadResource(ctx context.Context, uri string) (string, error) {
	klog.V(2).InfoS("Reading MCP resource via HTTP", "server", c.name, "uri", uri)

	if err := c.ensureConnected(); err != nil {
		return "", err
	}

	return readClientResource(ctx, c.client, uri)
}

// CallTool calls a tool on the MCP server and returns the result as a string
func (c *httpClient) CallTool(ctx context.Context, toolName string, arguments map[string]interface{}) (string, error) {
	klog.V(2).InfoS("Calling MCP tool via HTTP", "server", c.name, "tool", toolName)

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

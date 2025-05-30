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
	if c.client == nil {
		return fmt.Errorf("client not connected")
	}
	return nil
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
	initCtx, cancel := context.WithTimeout(ctx, DefaultConnectionTimeout)
	defer cancel()

	// Create initialize request with the structure expected by v0.31.0
	initReq := mcp.InitializeRequest{
		// In v0.31.0, the structure might differ - adapt as needed
		// This is a placeholder that will be updated when the actual API is known
	}

	_, err := c.client.Initialize(initCtx, initReq)
	if err != nil {
		return fmt.Errorf("initializing MCP client: %w", err)
	}

	return nil
}

// verifyConnection verifies the connection works by testing tool listing
func (c *httpClient) verifyConnection(ctx context.Context) error {
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
func (c *httpClient) cleanup() {
	if c.client != nil {
		_ = c.client.Close() // Ignore errors on cleanup
		c.client = nil
	}
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

	klog.V(2).InfoS("Listed tools from HTTP MCP server", "count", len(tools), "server", c.name)
	return tools, nil
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

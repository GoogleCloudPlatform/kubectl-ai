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
	"sync"
	"time"

	"k8s.io/klog/v2"
)

// =============================================================================
// Status Types
// =============================================================================

// ServerConnectionInfo holds connection status for a single MCP server
type ServerConnectionInfo struct {
	Name           string
	Command        string
	IsLegacy       bool
	IsConnected    bool
	AvailableTools []Tool
}

// MCPStatus represents the overall status of MCP servers and tools
type MCPStatus struct {
	ServerInfoList []ServerConnectionInfo
	TotalServers   int
	ConnectedCount int
	FailedCount    int
	TotalTools     int
	ClientEnabled  bool
}

// =============================================================================
// Manager Core
// =============================================================================

// Manager handles MCP client connections and tool discovery
type Manager struct {
	config  *Config
	clients map[string]*Client
	mu      sync.RWMutex
}

// NewManager creates a new MCP manager with the given configuration
func NewManager(config *Config) *Manager {
	return &Manager{
		config:  config,
		clients: make(map[string]*Client),
	}
}

// InitializeManager creates and initializes the MCP manager
// with configuration loaded from default paths
func InitializeManager() (*Manager, error) {
	klog.V(1).Info("Initializing MCP client functionality")

	config, err := LoadConfig("")
	if err != nil {
		klog.V(2).Info("Failed to load MCP config", "error", err)
		return nil, err
	}

	return NewManager(config), nil
}

// =============================================================================
// Connection Management
// =============================================================================

// ConnectAll connects to all configured MCP servers
func (m *Manager) ConnectAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error

	for _, serverCfg := range m.config.Servers {
		if _, exists := m.clients[serverCfg.Name]; exists {
			klog.V(2).Info("MCP client already connected", "name", serverCfg.Name)
			continue
		}

		// Convert environment map to slice
		var envSlice []string
		for k, v := range serverCfg.Env {
			envSlice = append(envSlice, fmt.Sprintf("%s=%s", k, v))
		}

		// Create client config with environment map
		config := ClientConfig{
			Name:    serverCfg.Name,
			Command: serverCfg.Command,
			Args:    serverCfg.Args,
			Env:     envSlice,
			URL:     serverCfg.URL,
		}

		client := NewClient(config)
		if err := client.Connect(ctx); err != nil {
			err := fmt.Errorf(ErrServerConnectionFmt, serverCfg.Name, err)
			errs = append(errs, err)
			klog.Error(err)
			continue
		}

		m.clients[serverCfg.Name] = client
		klog.V(2).Info("Connected to MCP server", "name", serverCfg.Name)
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to connect to some MCP servers: %v", errs)
	}

	return nil
}

// Close closes all MCP client connections
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error

	for name, client := range m.clients {
		if err := client.Close(); err != nil {
			errs = append(errs, fmt.Errorf(ErrServerCloseFmt, name, err))
		}
		delete(m.clients, name)
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors while closing MCP clients: %v", errs)
	}

	return nil
}

// GetClient returns a connected MCP client by name
func (m *Manager) GetClient(name string) (*Client, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	client, exists := m.clients[name]
	return client, exists
}

// ListClients returns a list of all connected MCP clients
func (m *Manager) ListClients() []*Client {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var clients []*Client
	for _, client := range m.clients {
		clients = append(clients, client)
	}

	return clients
}

// =============================================================================
// Server and Tool Discovery
// =============================================================================

// DiscoverAndConnectServers connects to all configured servers
// with a timeout and stabilization delay
func (m *Manager) DiscoverAndConnectServers(ctx context.Context) error {
	klog.V(1).Info("Connecting to MCP servers")

	connectCtx, connectCancel := context.WithTimeout(ctx, DefaultConnectionTimeout)
	defer connectCancel()

	if err := m.ConnectAll(connectCtx); err != nil {
		klog.V(2).Info("Failed to connect to some MCP servers during auto-discovery", "error", err)
		// Continue with partial connections
	}

	// Allow connections to stabilize before tool discovery
	klog.V(3).Info("Waiting for server connections to stabilize", "delay", DefaultStabilizationDelay)
	time.Sleep(DefaultStabilizationDelay)

	return nil
}

// ListAvailableTools returns tools from all connected servers
// For retries and more robust handling, use RefreshToolDiscovery
func (m *Manager) ListAvailableTools(ctx context.Context) (map[string][]Tool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tools := make(map[string][]Tool)

	for name, client := range m.clients {
		toolList, err := client.ListTools(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing tools from MCP server %q: %w", name, err)
		}

		var serverTools []Tool
		for _, tool := range toolList {
			serverTools = append(serverTools, tool.WithServer(name))
		}

		tools[name] = serverTools
	}

	return tools, nil
}

// ListAvailableResources returns resources from all connected servers
func (m *Manager) ListAvailableResources(ctx context.Context) (map[string][]Resource, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	resources := make(map[string][]Resource)

	for name, client := range m.clients {
		resList, err := client.ListResources(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing resources from MCP server %q: %w", name, err)
		}

		resources[name] = resList
	}

	return resources, nil
}

// ReadResource reads a resource's content from a specific server
func (m *Manager) ReadResource(ctx context.Context, serverName, uri string) (string, error) {
	m.mu.RLock()
	client, exists := m.clients[serverName]
	m.mu.RUnlock()
	if !exists {
		return "", fmt.Errorf("MCP server %q not found", serverName)
	}

	return client.ReadResource(ctx, uri)
}

// RefreshToolDiscovery discovers tools from all servers with retries
func (m *Manager) RefreshToolDiscovery(ctx context.Context) (map[string][]Tool, error) {
	klog.V(1).Info("Starting tool discovery from MCP servers with retries")

	var serverTools map[string][]Tool

	retryConfig := DefaultRetryConfig("tool discovery from MCP servers")
	err := RetryOperation(ctx, retryConfig, func() error {
		var err error
		serverTools, err = m.ListAvailableTools(ctx)
		return err
	})

	if err != nil {
		klog.Warningf("Failed to discover tools after retries: %v", err)
		return nil, err
	}

	// Log discovery results
	toolCount := 0
	for serverName, tools := range serverTools {
		klog.V(1).Info("Discovered tools from MCP server", "server", serverName, "toolCount", len(tools))
		toolCount += len(tools)
	}

	if toolCount > 0 {
		klog.InfoS("Successfully discovered MCP tools", "totalTools", toolCount)
	} else {
		klog.V(1).Info("No MCP tools were discovered from connected servers")
	}

	return serverTools, nil
}

// RegisterTools discovers and registers tools from all MCP servers using the provided callback
// The callback function is responsible for creating and registering tool wrappers
func (m *Manager) RegisterTools(ctx context.Context, registerCallback func(serverName string, tool Tool) error) error {
	// Discover tools from connected servers
	serverTools, err := m.RefreshToolDiscovery(ctx)
	if err != nil {
		return err
	}

	toolCount := 0
	for serverName, tools := range serverTools {
		for _, toolInfo := range tools {
			// Use the callback to register each tool
			if err := registerCallback(serverName, toolInfo); err != nil {
				klog.Warningf("Failed to register tool %s from server %s: %v", toolInfo.Name, serverName, err)
				continue
			}
			toolCount++
		}
	}

	if toolCount > 0 {
		klog.InfoS("Registered MCP tools", "totalTools", toolCount)
	}

	return nil
}

// =============================================================================
// Status Reporting
// =============================================================================

// GetStatus returns status of all MCP servers and their tools
func (m *Manager) GetStatus(ctx context.Context, mcpClientEnabled bool) (*MCPStatus, error) {
	status := &MCPStatus{
		ClientEnabled: mcpClientEnabled,
	}

	mcpConfigPath, err := DefaultConfigPath()
	if err != nil {
		klog.V(2).Infof("Failed to get MCP config path: %v", err)
		return status, nil // Return empty status
	}

	mcpConfig, err := LoadConfig(mcpConfigPath)
	if err != nil {
		return status, nil // Return empty status
	}

	status.TotalServers = len(mcpConfig.Servers)

	if status.TotalServers == 0 {
		return status, nil
	}

	var serverTools map[string][]Tool
	var connectedClients []*Client

	if mcpClientEnabled && m != nil {
		connectedClients = m.ListClients()
		status.ConnectedCount = len(connectedClients)
		status.FailedCount = status.TotalServers - status.ConnectedCount

		toolsCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		serverTools, err = m.ListAvailableTools(toolsCtx)
		if err != nil {
			klog.V(2).InfoS("Failed to get tools from MCP manager", "error", err)
			serverTools = make(map[string][]Tool)
		}

		for _, toolList := range serverTools {
			status.TotalTools += len(toolList)
		}
	} else {
		serverTools = make(map[string][]Tool)
	}

	connectedServerNames := make(map[string]bool)
	if mcpClientEnabled {
		for _, client := range connectedClients {
			connectedServerNames[client.Name] = true
		}
	}

	// Process all servers
	for _, server := range mcpConfig.Servers {
		serverInfo := ServerConnectionInfo{
			Name:        server.Name,
			Command:     server.Command,
			IsLegacy:    false,
			IsConnected: connectedServerNames[server.Name],
		}

		if tools, exists := serverTools[server.Name]; exists {
			serverInfo.AvailableTools = tools
		}

		status.ServerInfoList = append(status.ServerInfoList, serverInfo)
	}

	return status, nil
}

// LogConfig logs the MCP configuration summary
// If mcpConfigPath is empty, uses the Manager's existing config
func (m *Manager) LogConfig(mcpConfigPath string) error {
	var mcpConfig *Config
	var err error

	if mcpConfigPath == "" && m.config != nil {
		mcpConfig = m.config
	} else {
		mcpConfig, err = LoadConfig(mcpConfigPath)
		if err != nil {
			return fmt.Errorf("failed to load MCP config from %s: %w", mcpConfigPath, err)
		}
	}

	serverCount := len(mcpConfig.Servers)
	totalServers := serverCount

	if totalServers > 0 {
		serverWord := "server"
		if totalServers > 1 {
			serverWord = "servers"
		}

		if mcpConfigPath != "" {
			klog.V(2).Infof("Loaded %d MCP %s from %s", totalServers, serverWord, mcpConfigPath)
		} else {
			klog.V(2).Infof("Found %d MCP %s in configuration", totalServers, serverWord)
		}

		for _, server := range mcpConfig.Servers {
			klog.V(2).Infof("  - %s: %s", server.Name, server.Command)
		}
	}

	return nil
}

// =============================================================================
// Integration Methods
// =============================================================================

// RegisterWithToolSystem connects to MCP servers and registers discovered tools with an external tool system
// using the provided callback function. This simplifies integration with kubectl-ai's tool system.
func (m *Manager) RegisterWithToolSystem(ctx context.Context, registerCallback func(serverName string, tool Tool) error) error {
	klog.V(1).Info("Initializing MCP client functionality and registering tools")

	// Connect to all configured servers
	if err := m.DiscoverAndConnectServers(ctx); err != nil {
		return fmt.Errorf("MCP server connection failed: %w", err)
	}

	// Register all discovered tools using the callback
	if err := m.RegisterTools(ctx, registerCallback); err != nil {
		return fmt.Errorf("MCP tool registration failed: %w", err)
	}

	return nil
}

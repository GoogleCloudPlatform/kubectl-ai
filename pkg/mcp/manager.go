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

	"k8s.io/klog/v2"
)

// Manager manages multiple MCP client connections
type Manager struct {
	config *Config
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

		// Create client with environment variables
		client := NewClient(serverCfg.Name, serverCfg.Command, serverCfg.Args, serverCfg.Env)
		if err := client.Connect(ctx); err != nil {
			err := fmt.Errorf("connecting to MCP server %q: %w", serverCfg.Name, err)
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
			errs = append(errs, fmt.Errorf("closing MCP client %q: %w", name, err))
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

// ListAvailableTools returns a map of all available tools from all connected MCP servers
func (m *Manager) ListAvailableTools(ctx context.Context) (map[string][]ToolInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tools := make(map[string][]ToolInfo)

	for name, client := range m.clients {
		toolList, err := client.ListTools(ctx)
		if err != nil {
			klog.Errorf("Failed to list tools from MCP server %q: %v", name, err)
			continue
		}

		var serverTools []ToolInfo
		for _, tool := range toolList {
			serverTools = append(serverTools, ToolInfo{
				Name:        tool.Name,
				Description: tool.Description,
				Server:      name,
			})
		}

		tools[name] = serverTools
	}

	return tools, nil
}

// ToolInfo represents information about an available tool
// This is a simplified version of the MCP Tool type for display purposes
type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Server      string `json:"server"`
}

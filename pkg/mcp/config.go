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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"k8s.io/klog/v2"
)

// ServerConfig represents the configuration for an MCP server
type ServerConfig struct {
	// Name is a friendly name for this MCP server
	Name string `json:"name"`
	// Command is the command to execute for stdio-based MCP servers
	Command string `json:"command"`
	// Args are the arguments to pass to the command
	Args []string `json:"args,omitempty"`
	// Env are the environment variables to set for the command
	Env map[string]string `json:"env,omitempty"`
}

// Config represents the MCP client configuration file
type Config struct {
	// Servers is a list of MCP server configurations
	Servers []ServerConfig `json:"servers,omitempty"`
	// Legacy field for backward compatibility with mcpServers format
	MCPServers map[string]ServerConfig `json:"mcpServers,omitempty"`
}

// loadDefaultConfig loads the default configuration from the embedded file
func loadDefaultConfig() (*Config, error) {
	// This path is relative to the module root
	defaultConfigPath := filepath.Join("pkg", "mcp", "default_config.json")

	// Read the file
	data, err := os.ReadFile(defaultConfigPath)
	if err != nil {
		return nil, fmt.Errorf("reading default config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing default config: %w", err)
	}

	return &config, nil
}

// DefaultConfigPath returns the default path to the MCP config file
func DefaultConfigPath() (string, error) {
	// Get the home directory first
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting user home directory: %w", err)
	}

	var configPath string
	var oldConfigPath string

	// Handle different operating systems
	switch runtime.GOOS {
	case "windows":
		// On Windows, use %APPDATA%\kubectl-ai\mcp.json
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		configPath = filepath.Join(appData, "kubectl-ai", "mcp.json")
		oldConfigPath = filepath.Join(home, ".kube", "mcp-config.json")
	default:
		// On Unix-like systems, use XDG_CONFIG_HOME/kubectl-ai/mcp.json
		configDir := os.Getenv("XDG_CONFIG_HOME")
		if configDir == "" {
			configDir = filepath.Join(home, ".config")
		}
		configPath = filepath.Join(configDir, "kubectl-ai", "mcp.json")
		oldConfigPath = filepath.Join(home, ".kube", "mcp-config.json")
	}

	// For backward compatibility, check if the old config exists
	if _, err := os.Stat(oldConfigPath); err == nil {
		// If the old config exists, move it to the new location
		if err := os.MkdirAll(filepath.Dir(configPath), 0755); err == nil {
			if err := os.Rename(oldConfigPath, configPath); err == nil {
				klog.V(2).Info("Migrated MCP config to new location", "oldPath", oldConfigPath, "newPath", configPath)
			}
		}
	}

	return configPath, nil
}

// LoadConfig loads the MCP configuration from the given path
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		var err error
		path, err = DefaultConfigPath()
		if err != nil {
			return nil, err
		}
	}

	// If the file doesn't exist, create it with default configuration
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// Create the directory if it doesn't exist
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("creating config directory: %w", err)
		}

		// Read the default config from the embedded file
		defaultConfig, err := loadDefaultConfig()
		if err != nil {
			return nil, fmt.Errorf("loading default config: %w", err)
		}

		// Save the default configuration to the user's config path
		if err := defaultConfig.Save(path); err != nil {
			return nil, fmt.Errorf("saving default config: %w", err)
		}

		klog.V(2).Info("Created default MCP configuration", "path", path)
		return defaultConfig, nil
	}

	// Load existing configuration
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	// Convert mcpServers map to Servers slice if needed (legacy support)
	if len(config.MCPServers) > 0 && len(config.Servers) == 0 {
		for name, serverCfg := range config.MCPServers {
			// Set the name from the map key if not already set
			if serverCfg.Name == "" {
				serverCfg.Name = name
			}
			config.Servers = append(config.Servers, serverCfg)
		}
		
		// Save the converted configuration
		if err := config.Save(path); err != nil {
			klog.Warningf("Failed to save converted MCP config: %v", err)
		}
	}

	return &config, nil
}

// Save saves the configuration to the given path
// Note: This will save in the new format with the 'servers' array
func (c *Config) Save(path string) error {
	if path == "" {
		var err error
		path, err = DefaultConfigPath()
		if err != nil {
			return err
		}
	}

	// Create the directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	// Create a temporary file in the same directory to ensure atomic write
	tmpFile, err := os.CreateTemp(dir, ".mcp-config-*")
	if err != nil {
		return fmt.Errorf("creating temporary file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath) // Clean up the temp file if we fail

	// Create a copy of the config with only the Servers field for saving
	saveConfig := struct {
		Servers []ServerConfig `json:"servers"`
	}{
		Servers: c.Servers,
	}

	// Write to the temporary file
	data, err := json.MarshalIndent(saveConfig, "", "  ")
	if err != nil {
		tmpFile.Close()
		return fmt.Errorf("marshaling config: %w", err)
	}

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("writing to temporary file: %w", err)
	}

	// Ensure the data is written to disk
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return fmt.Errorf("syncing temporary file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("closing temporary file: %w", err)
	}

	// Set the correct permissions before renaming
	if err := os.Chmod(tmpPath, 0600); err != nil {
		return fmt.Errorf("setting file permissions: %w", err)
	}

	// Rename the temporary file to the target file (atomic on Unix-like systems)
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("renaming temporary file: %w", err)
	}

	klog.V(2).Info("Saved MCP configuration", "path", path)
	return nil
}

// AddServer adds a new server configuration
func (c *Config) AddServer(name, command string, args []string, env map[string]string) {
	c.Servers = append(c.Servers, ServerConfig{
		Name:    name,
		Command: command,
		Args:    args,
		Env:     env,
	})
}

// RemoveServer removes a server configuration by name
func (c *Config) RemoveServer(name string) bool {
	for i, server := range c.Servers {
		if server.Name == name {
			c.Servers = append(c.Servers[:i], c.Servers[i+1:]...)
			return true
		}
	}
	return false
}

// GetServer returns a server configuration by name
func (c *Config) GetServer(name string) (*ServerConfig, bool) {
	for _, server := range c.Servers {
		if server.Name == name {
			return &server, true
		}
	}
	return nil, false
}

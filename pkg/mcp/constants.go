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

import "time"

// Timeout constants for MCP operations
const (
	// DefaultConnectionTimeout is the timeout for establishing connections to MCP servers
	DefaultConnectionTimeout = 30 * time.Second

	// DefaultVerificationTimeout is the timeout for verifying server connections
	DefaultVerificationTimeout = 10 * time.Second

	// DefaultPingTimeout is the timeout for ping operations
	DefaultPingTimeout = 5 * time.Second

	// DefaultStabilizationDelay is the delay to allow servers to stabilize after connection
	DefaultStabilizationDelay = 2 * time.Second
)

// Error message templates
const (
	ErrServerConnectionFmt = "connecting to MCP server %q: %w"
	ErrServerCloseFmt      = "closing MCP client %q: %w"
	ErrToolCallFmt         = "calling tool %q: %w"
	ErrPathCheckFmt        = "checking path %q: %w"
)

// Client constants
const (
	ClientName    = "kubectl-ai-mcp-client"
	ClientVersion = "1.0.0"
)

// File permissions
const (
	ConfigFilePermissions = 0600
	ConfigDirPermissions  = 0755
)

// Constants for environment variables
const (
	// EnvMCPServerPrefix is the prefix for MCP server environment variables
	EnvMCPServerPrefix = "MCP_"
)

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
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

// RetryConfig defines retry behavior for MCP operations
type RetryConfig struct {
	MaxRetries  int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	Multiplier  float64
	Description string
}

// DefaultRetryConfig returns a sensible default retry configuration
func DefaultRetryConfig(description string) RetryConfig {
	return RetryConfig{
		MaxRetries:  3,
		BaseDelay:   1 * time.Second,
		MaxDelay:    10 * time.Second,
		Multiplier:  2.0,
		Description: description,
	}
}

// RetryOperation executes an operation with exponential backoff retry
func RetryOperation(ctx context.Context, config RetryConfig, operation func() error) error {
	var lastErr error

	for attempt := 1; attempt <= config.MaxRetries; attempt++ {
		klog.V(3).InfoS("Attempting operation",
			"operation", config.Description,
			"attempt", attempt,
			"maxRetries", config.MaxRetries)

		if err := operation(); err == nil {
			if attempt > 1 {
				klog.V(2).InfoS("Operation succeeded after retry",
					"operation", config.Description,
					"attempt", attempt)
			}
			return nil
		} else {
			lastErr = err

			if attempt < config.MaxRetries {
				delay := calculateBackoffDelay(attempt, config)
				klog.V(3).InfoS("Operation failed, retrying",
					"operation", config.Description,
					"attempt", attempt,
					"error", err,
					"nextRetryIn", delay)

				select {
				case <-ctx.Done():
					return fmt.Errorf("operation cancelled: %w", ctx.Err())
				case <-time.After(delay):
					// Continue to next attempt
				}
			}
		}
	}

	return fmt.Errorf("operation failed after %d attempts: %w", config.MaxRetries, lastErr)
}

// calculateBackoffDelay calculates exponential backoff delay with jitter
func calculateBackoffDelay(attempt int, config RetryConfig) time.Duration {
	delay := float64(config.BaseDelay) * math.Pow(config.Multiplier, float64(attempt-1))

	if time.Duration(delay) > config.MaxDelay {
		return config.MaxDelay
	}

	return time.Duration(delay)
}

// SanitizeServerName ensures server names are valid identifiers
func SanitizeServerName(name string) string {
	// Simple sanitization - replace invalid characters
	result := ""
	for _, char := range name {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' || char == '_' {
			result += string(char)
		} else {
			result += "_"
		}
	}

	if result == "" {
		result = "unnamed"
	}

	return result
}

// GroupToolsByServer groups tools by their server name for easier display
func GroupToolsByServer(tools map[string][]Tool) map[string]int {
	summary := make(map[string]int)

	for serverName, serverTools := range tools {
		summary[serverName] = len(serverTools)
	}

	return summary
}

// mergeEnvironmentVariables merges process environment with custom environment variables
func mergeEnvironmentVariables(processEnv, customEnv []string) []string {
	envMap := make(map[string]string)

	// Parse process environment
	for _, e := range processEnv {
		if parts := strings.SplitN(e, "=", 2); len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	// Override with custom environment variables
	for _, env := range customEnv {
		if parts := strings.SplitN(env, "=", 2); len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	// Convert back to slice
	finalEnv := make([]string, 0, len(envMap))
	for k, v := range envMap {
		finalEnv = append(finalEnv, fmt.Sprintf("%s=%s", k, v))
	}

	return finalEnv
}

// expandPath expands the command path, handling ~ and environment variables
// If the path is just a binary name (no path separators), it looks in $PATH
func expandPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path cannot be empty")
	}

	// Expand environment variables first
	expanded := os.ExpandEnv(path)

	// If the command contains no path separators, look it up in $PATH first
	if !strings.Contains(expanded, string(filepath.Separator)) && !strings.HasPrefix(expanded, "~") {
		klog.V(2).InfoS("Attempting PATH lookup for command", "command", expanded)
		// Try to find the command in $PATH
		if pathResolved, err := exec.LookPath(expanded); err == nil {
			klog.V(2).InfoS("Found command in PATH", "command", expanded, "resolved", pathResolved)
			return pathResolved, nil
		} else {
			klog.V(2).InfoS("Command not found in PATH", "command", expanded, "error", err)
		}
		// If not found in PATH, continue with the original logic below
		klog.V(2).InfoS("Command not found in PATH, trying relative to current directory", "command", expanded)
	} else {
		klog.V(2).InfoS("Skipping PATH lookup", "command", expanded, "hasPathSeparator", strings.Contains(expanded, string(filepath.Separator)), "hasTilde", strings.HasPrefix(expanded, "~"))
	}

	// Handle ~ for home directory
	if strings.HasPrefix(expanded, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("getting home directory: %w", err)
		}
		expanded = filepath.Join(home, expanded[1:])
	}

	// Clean the path to remove any . or .. elements
	expanded = filepath.Clean(expanded)

	// Make the path absolute if it's not already
	if !filepath.IsAbs(expanded) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("getting current working directory: %w", err)
		}
		expanded = filepath.Clean(filepath.Join(cwd, expanded))
	}

	// Verify the file exists and is executable
	info, err := os.Stat(expanded)
	if err != nil {
		return "", fmt.Errorf(ErrPathCheckFmt, expanded, err)
	}

	// Check if it's a regular file and executable
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("path %q is not a regular file", expanded)
	}

	// Check if the file is executable by the current user
	if info.Mode().Perm()&0111 == 0 {
		return "", fmt.Errorf("file %q is not executable", expanded)
	}

	return expanded, nil
}

// =============================================================================
// Helper Functions to Reduce Redundancy
// =============================================================================

// withTimeout creates a context with the specified timeout and returns the context and cancel function
func withTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, timeout)
}

// ensureConnected checks if the client is connected and returns an error if not
func (c *Client) ensureConnected() error {
	if c.client == nil {
		return fmt.Errorf("not connected to MCP server")
	}
	return nil
}

// =============================================================================
// MCP Tool Helper Functions
// =============================================================================

// FunctionDefinition is an interface representing generic function schema definitions
// This allows the MCP package to create schemas without directly depending on gollm
type FunctionDefinition interface {
	// Schema returns a representation of the function schema
	Schema() any
}

// SchemaProperty is an interface representing generic schema properties
type SchemaProperty interface {
	// Property returns a representation of the schema property
	Property() any
}

// SchemaBuilder is a function that builds a function definition from a tool
type SchemaBuilder func(tool *Tool) (FunctionDefinition, error)

// ConvertArgs handles all argument conversions for MCP tools.
// It transforms keys from snake_case to camelCase and converts values to appropriate types.
func ConvertArgs(args map[string]any) map[string]any {
	if len(args) == 0 {
		return args
	}

	result := make(map[string]any, len(args))

	for key, value := range args {
		// Convert key from snake_case to camelCase
		camelKey := SnakeToCamel(key)

		// Convert value based on key name patterns
		result[camelKey] = ConvertValue(camelKey, value)
	}

	return result
}

// SnakeToCamel converts a snake_case string to camelCase.
func SnakeToCamel(s string) string {
	if !strings.Contains(s, "_") {
		return s
	}

	parts := strings.Split(s, "_")
	result := parts[0]

	for _, part := range parts[1:] {
		if len(part) > 0 {
			result += strings.ToUpper(part[:1]) + part[1:]
		}
	}

	return result
}

// ConvertValue infers and converts a value to an appropriate type based on the parameter name.
func ConvertValue(paramName string, value any) any {
	// Already primitive types that don't need conversion
	switch value.(type) {
	case bool, int, int32, int64, float32, float64:
		return value
	}

	name := strings.ToLower(paramName)

	// Number parameter detection
	if IsNumberParam(name) {
		if str, ok := value.(string); ok {
			// Try integer conversion first
			if num, err := strconv.Atoi(str); err == nil {
				return num
			}
			// Then try float conversion
			if num, err := strconv.ParseFloat(str, 64); err == nil {
				return num
			}
		} else if f, ok := value.(float64); ok && f == float64(int(f)) {
			// Convert whole number floats to int
			return int(f)
		}
	}

	// Boolean parameter detection
	if IsBoolParam(name) {
		if str, ok := value.(string); ok {
			if b, err := strconv.ParseBool(str); err == nil {
				return b
			}
		} else if n, ok := value.(int); ok {
			return n != 0
		}
	}

	return value
}

// IsNumberParam checks if a parameter name suggests a numeric value.
func IsNumberParam(name string) bool {
	numberPatterns := []string{"number", "count", "total", "max", "min", "limit"}
	for _, pattern := range numberPatterns {
		if strings.Contains(name, pattern) {
			return true
		}
	}
	return false
}

// IsBoolParam checks if a parameter name suggests a boolean value.
func IsBoolParam(name string) bool {
	// Prefix checks
	boolPrefixes := []string{"is", "has", "needs", "enable"}
	for _, prefix := range boolPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}

	// Contains checks
	boolPatterns := []string{"required", "enabled"}
	for _, pattern := range boolPatterns {
		if strings.Contains(name, pattern) {
			return true
		}
	}

	return false
}

// SerializeArgsForMCP ensures arguments are properly serialized for MCP servers.
func SerializeArgsForMCP(args map[string]any) (map[string]any, error) {
	if len(args) == 0 {
		return args, nil
	}

	// Create a new map to store serialized arguments
	serialized := make(map[string]any, len(args))

	for key, value := range args {
		// Convert each value to its JSON-serializable form
		serializedValue, err := serializeValue(value)
		if err != nil {
			return nil, fmt.Errorf("serializing argument %q: %w", key, err)
		}
		serialized[key] = serializedValue
	}

	return serialized, nil
}

// serializeValue converts a value to its JSON-serializable representation
func serializeValue(value any) (any, error) {
	if value == nil {
		return nil, nil
	}

	switch v := value.(type) {
	case string, bool, int, int32, int64, float32, float64:
		// Primitive types are already JSON-serializable
		return v, nil
	case map[string]any:
		// Recursively serialize nested maps
		serialized := make(map[string]any, len(v))
		for k, val := range v {
			serializedVal, err := serializeValue(val)
			if err != nil {
				return nil, fmt.Errorf("serializing nested value %q: %w", k, err)
			}
			serialized[k] = serializedVal
		}
		return serialized, nil
	case []any:
		// Recursively serialize slices
		serialized := make([]any, len(v))
		for i, val := range v {
			serializedVal, err := serializeValue(val)
			if err != nil {
				return nil, fmt.Errorf("serializing slice element %d: %w", i, err)
			}
			serialized[i] = serializedVal
		}
		return serialized, nil
	default:
		// For any other type, try to convert through JSON marshal/unmarshal
		// This handles complex types and ensures they're properly serializable
		jsonBytes, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("marshaling value to JSON: %w", err)
		}

		var result any
		if err := json.Unmarshal(jsonBytes, &result); err != nil {
			return nil, fmt.Errorf("unmarshaling JSON back to value: %w", err)
		}

		return result, nil
	}
}

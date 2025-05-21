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

package tools

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/journal"
)

type mockTool struct {
	name        string
	description string
	runFunc     func(ctx context.Context, args map[string]any) (any, error)
}

func (m *mockTool) Name() string {
	return m.name
}

func (m *mockTool) Description() string {
	return m.description
}

func (m *mockTool) FunctionDefinition() *gollm.FunctionDefinition {
	return &gollm.FunctionDefinition{
		Name:        m.name,
		Description: m.description,
		Parameters: &gollm.Schema{
			Type: gollm.TypeObject,
			Properties: map[string]*gollm.Schema{
				"test_param": {
					Type:        gollm.TypeString,
					Description: "A test parameter",
				},
			},
		},
	}
}

func (m *mockTool) Run(ctx context.Context, args map[string]any) (any, error) {
	if m.runFunc != nil {
		return m.runFunc(ctx, args)
	}
	return nil, nil
}

func TestToolRegistration(t *testing.T) {
	// Save original tools and restore after test
	originalTools := allTools
	defer func() {
		allTools = originalTools
	}()

	// Create a fresh tools instance for testing
	allTools = Tools{
		tools: make(map[string]Tool),
	}

	tests := []struct {
		name        string
		tool        Tool
		shouldPanic bool
	}{
		{
			name: "register new tool",
			tool: &mockTool{
				name:        "test_tool",
				description: "A test tool",
			},
			shouldPanic: false,
		},
		{
			name: "register duplicate tool",
			tool: &mockTool{
				name:        "test_tool", // Same name as first tool
				description: "A test tool",
			},
			shouldPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if tt.shouldPanic {
					if r == nil {
						t.Errorf("expected panic, but did not panic")
					}
				} else {
					if r != nil {
						t.Errorf("did not expect panic, but panicked: %v", r)
					}
				}
			}()
			allTools.RegisterTool(tt.tool)
			if !tt.shouldPanic {
				if got := allTools.Lookup(tt.tool.Name()); got != tt.tool {
					t.Errorf("Lookup() = %v, want %v", got, tt.tool)
				}
			}
		})
	}
}

func TestParseToolInvocation(t *testing.T) {
	tests := []struct {
		name      string
		toolName  string
		arguments map[string]any
		wantErr   bool
	}{
		{
			name:     "valid kubectl invocation",
			toolName: "kubectl",
			arguments: map[string]any{
				"command":           "get pods",
				"modifies_resource": "no",
			},
			wantErr: false,
		},
		{
			name:     "non-existent tool",
			toolName: "non_existent_tool",
			arguments: map[string]any{
				"command": "test",
			},
			wantErr: true,
		},
		{
			name:      "empty arguments",
			toolName:  "kubectl",
			arguments: map[string]any{},
			wantErr:   false,
		},
	}

	tools := Default()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			call, err := tools.ParseToolInvocation(ctx, tt.toolName, tt.arguments)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				if call != nil {
					t.Errorf("expected nil call, got %v", call)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if call == nil {
					t.Fatalf("expected call, got nil")
				}
				if call.name != tt.toolName {
					t.Errorf("call.name = %v, want %v", call.name, tt.toolName)
				}
				if len(call.arguments) != len(tt.arguments) {
					t.Errorf("call.arguments = %v, want %v", call.arguments, tt.arguments)
				}
			}
		})
	}
}

func TestInvokeTool(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "kubectl-ai-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// // Create a mock recorder for testing
	mockRecorder := &journal.LogRecorder{}

	// Create a custom error for testing
	testErr := fmt.Errorf("mock tool error")

	tests := []struct {
		name      string
		tool      Tool
		arguments map[string]any
		options   InvokeToolOptions
		wantErr   bool
		wantError string
	}{
		{
			name: "successful tool invocation",
			tool: &mockTool{
				name:        "test_tool",
				description: "A test tool",
				runFunc: func(ctx context.Context, args map[string]any) (any, error) {
					return "success", nil
				},
			},
			arguments: map[string]any{
				"test_param": "value",
			},
			options: InvokeToolOptions{
				WorkDir:    tmpDir,
				Kubeconfig: "/path/to/kubeconfig",
			},
			wantErr: false,
		},
		{
			name: "tool execution error",
			tool: &mockTool{
				name:        "error_tool",
				description: "A tool that returns an error",
				runFunc: func(ctx context.Context, args map[string]any) (any, error) {
					return nil, testErr
				},
			},
			arguments: map[string]any{
				"test_param": "value",
			},
			options: InvokeToolOptions{
				WorkDir:    tmpDir,
				Kubeconfig: "/path/to/kubeconfig",
			},
			wantErr:   true,
			wantError: testErr.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new context for each test to avoid context cancellation
			testCtx := context.Background()
			testCtx = journal.ContextWithRecorder(testCtx, mockRecorder)
			testCtx = context.WithValue(testCtx, KubeconfigKey, tt.options.Kubeconfig)
			testCtx = context.WithValue(testCtx, WorkDirKey, tt.options.WorkDir)

			call := &ToolCall{
				tool:      tt.tool,
				name:      tt.tool.Name(),
				arguments: tt.arguments,
			}

			result, err := call.InvokeTool(testCtx, tt.options)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				if err != nil && err.Error() != tt.wantError {
					t.Errorf("error = %v, want %v", err, tt.wantError)
				}
				if result != nil {
					t.Errorf("expected nil result, got %v", result)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result == nil {
					t.Errorf("expected result, got nil")
				}
				if result != "success" {
					t.Errorf("result = %v, want 'success'", result)
				}
			}
		})
	}
}

func TestToolResultToMap(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    map[string]any
		wantErr bool
	}{
		{
			name: "simple struct",
			input: struct {
				Name  string `json:"name"`
				Value int    `json:"value"`
			}{
				Name:  "test",
				Value: 42,
			},
			want: map[string]any{
				"name":  "test",
				"value": float64(42),
			},
			wantErr: false,
		},
		{
			name:    "nil input",
			input:   nil,
			want:    nil,
			wantErr: false,
		},
		{
			name: "complex struct",
			input: struct {
				Nested struct {
					Field string `json:"field"`
				} `json:"nested"`
				Array []int `json:"array"`
			}{
				Nested: struct {
					Field string `json:"field"`
				}{
					Field: "value",
				},
				Array: []int{1, 2, 3},
			},
			want: map[string]any{
				"nested": map[string]any{
					"field": "value",
				},
				"array": []any{float64(1), float64(2), float64(3)},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToolResultToMap(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if len(got) != len(tt.want) {
					t.Errorf("got = %v, want %v", got, tt.want)
				}
				for k, v := range tt.want {
					if fmt.Sprintf("%v", got[k]) != fmt.Sprintf("%v", v) {
						t.Errorf("got[%q] = %v, want %v", k, got[k], v)
					}
				}
			}
		})
	}
}

func TestKubectlTool(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "kubectl-ai-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name      string
		arguments map[string]any
		wantErr   bool
		checkFunc func(t *testing.T, result *ExecResult)
	}{
		{
			name: "valid kubectl command",
			arguments: map[string]any{
				"command":           "kubectl get pods",
				"modifies_resource": "no",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result *ExecResult) {
				if result == nil {
					t.Fatalf("result is nil")
				}
				// Note: The actual command will fail because kubectl isn't available in test environment
				// but we're testing the tool's behavior, not the actual kubectl execution
			},
		},
		{
			name: "interactive command not allowed",
			arguments: map[string]any{
				"command":           "kubectl edit pod test-pod",
				"modifies_resource": "yes",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result *ExecResult) {
				if result == nil {
					t.Errorf("expected result, got nil")
					return
				}
				if !strings.Contains(result.Error, "interactive mode not supported") {
					t.Errorf("expected error to contain 'interactive mode not supported', got %q", result.Error)
				}
			},
		},
		{
			name: "port-forward not allowed",
			arguments: map[string]any{
				"command":           "kubectl port-forward pod/test-pod 8080:80",
				"modifies_resource": "no",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result *ExecResult) {
				if result == nil {
					t.Errorf("expected result, got nil")
					return
				}
				if !strings.Contains(result.Error, "port-forwarding is not allowed") {
					t.Errorf("expected error to contain 'port-forwarding is not allowed', got %q", result.Error)
				}
			},
		},
		{
			name: "missing command",
			arguments: map[string]any{
				"modifies_resource": "no",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result *ExecResult) {
				if result == nil {
					t.Errorf("expected result, got nil")
					return
				}
				if !strings.Contains(result.Error, "kubectl command not provided") {
					t.Errorf("expected error to contain 'kubectl command not provided', got %q", result.Error)
				}
			},
		},
	}

	kubectl := &Kubectl{}
	ctx := context.Background()
	ctx = context.WithValue(ctx, KubeconfigKey, "/path/to/kubeconfig")
	ctx = context.WithValue(ctx, WorkDirKey, tmpDir)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := kubectl.Run(ctx, tt.arguments)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				if result != nil {
					t.Errorf("expected nil result, got %v", result)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result == nil {
					t.Errorf("expected result, got nil")
				}
				execResult, ok := result.(*ExecResult)
				if !ok {
					t.Fatalf("result should be *ExecResult, got %T", result)
				}
				if tt.checkFunc != nil {
					tt.checkFunc(t, execResult)
				}
			}
		})
	}
}

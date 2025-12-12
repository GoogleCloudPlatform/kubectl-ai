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
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/sandbox"
)

func TestCustomTool_AddCommandPrefix(t *testing.T) {
	tests := []struct {
		name           string
		configCommand  string
		inputCommand   string
		expectedOutput string
		expectError    bool
	}{
		{
			name:           "simple command without prefix",
			configCommand:  "gcloud",
			inputCommand:   "compute instances list",
			expectedOutput: "gcloud compute instances list",
			expectError:    false,
		},
		{
			name:           "simple command with prefix",
			configCommand:  "gcloud",
			inputCommand:   "gcloud compute instances list",
			expectedOutput: "gcloud compute instances list",
			expectError:    false,
		},
		{
			name:           "command with pipe",
			configCommand:  "gcloud",
			inputCommand:   "compute instances list | grep test",
			expectedOutput: "compute instances list | grep test",
			expectError:    false,
		},
		{
			name:           "command with redirect",
			configCommand:  "gcloud",
			inputCommand:   "compute instances list > instances.txt",
			expectedOutput: "compute instances list > instances.txt",
			expectError:    false,
		},
		{
			name:           "command with background",
			configCommand:  "gcloud",
			inputCommand:   "compute instances list &",
			expectedOutput: "compute instances list &",
			expectError:    false,
		},
		{
			name:           "command with subshell",
			configCommand:  "gcloud",
			inputCommand:   "(compute instances list)",
			expectedOutput: "(compute instances list)",
			expectError:    false,
		},
		{
			name:           "command with multiple statements",
			configCommand:  "gcloud",
			inputCommand:   "compute instances list; compute disks list",
			expectedOutput: "compute instances list; compute disks list",
			expectError:    false,
		},
		{
			name:           "invalid shell syntax",
			configCommand:  "gcloud",
			inputCommand:   "compute instances list |",
			expectedOutput: "",
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := &CustomTool{
				config: CustomToolConfig{
					Command: tt.configCommand,
				},
			}

			output, err := tool.addCommandPrefix(tt.inputCommand)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if output != tt.expectedOutput {
				t.Errorf("expected %q, got %q", tt.expectedOutput, output)
			}
		})
	}
}

// MockExecutor implements sandbox.Executor for testing
type MockExecutor struct {
	CapturedCommand string
	CapturedEnv     []string
	CapturedWorkDir string
}

func (m *MockExecutor) Execute(ctx context.Context, command string, env []string, workDir string) (*sandbox.ExecResult, error) {
	m.CapturedCommand = command
	m.CapturedEnv = env
	m.CapturedWorkDir = workDir
	return &sandbox.ExecResult{Stdout: "executed"}, nil
}

func (m *MockExecutor) Close(ctx context.Context) error {
	return nil
}

func TestCustomTool_CloneWithExecutor(t *testing.T) {
	config := CustomToolConfig{
		Name:    "test-tool",
		Command: "echo",
	}

	tool, err := NewCustomTool(config)
	if err != nil {
		t.Fatalf("failed to create tool: %v", err)
	}

	mockExec := &MockExecutor{}
	clonedTool := tool.CloneWithExecutor(mockExec)

	ctx := context.WithValue(context.Background(), WorkDirKey, "/tmp")
	args := map[string]any{
		"command": "hello",
	}

	result, err := clonedTool.Run(ctx, args)
	if err != nil {
		t.Fatalf("tool run failed: %v", err)
	}

	execResult, ok := result.(*sandbox.ExecResult)
	if !ok {
		t.Fatalf("expected *sandbox.ExecResult, got %T", result)
	}
	if execResult.Stdout != "executed" {
		t.Errorf("expected Stdout 'executed', got %q", execResult.Stdout)
	}

	if mockExec.CapturedCommand != "echo hello" {
		t.Errorf("expected command 'echo hello', got %q", mockExec.CapturedCommand)
	}
	if !strings.Contains(strings.Join(mockExec.CapturedEnv, "\n"), "PATH") {
		t.Errorf("expected captured environment to contain PATH")
	}
	if mockExec.CapturedWorkDir != "/tmp" {
		t.Errorf("expected workdir '/tmp', got %q", mockExec.CapturedWorkDir)
	}
}

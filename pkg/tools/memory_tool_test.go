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

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/memory"
)

func TestMemoryTool_Enabled(t *testing.T) {
	tempDir := t.TempDir()
	mgr, err := memory.NewDefaultManager(memory.Config{
		Enabled:     true,
		Environment: "test-env",
		StorageDir:  tempDir,
	}, "")
	if err != nil {
		t.Fatalf("failed to create memory manager: %v", err)
	}

	tool := NewMemoryTool(mgr)
	if tool.Name() != "remember_environment_context" {
		t.Fatalf("unexpected tool name: %s", tool.Name())
	}
	if tool.CheckModifiesResource(nil) != "no" {
		t.Fatalf("expected modifies resource to be 'no'")
	}
	interactive, err := tool.IsInteractive(nil)
	if interactive || err != nil {
		t.Fatalf("expected tool to not be interactive")
	}

	fd := tool.FunctionDefinition()
	if fd == nil || fd.Name != "remember_environment_context" {
		t.Fatalf("unexpected function definition: %+v", fd)
	}

	// Missing parameters
	_, err = tool.Run(context.Background(), map[string]any{"key": "test"})
	if err == nil {
		t.Fatalf("expected error for missing content")
	}

	_, err = tool.Run(context.Background(), map[string]any{"content": "test"})
	if err == nil {
		t.Fatalf("expected error for missing key")
	}

	// Successful run
	resp, err := tool.Run(context.Background(), map[string]any{
		"key":      "default-ns",
		"content":  "production",
		"category": "guideline",
	})
	if err != nil {
		t.Fatalf("unexpected error running memory tool: %v", err)
	}
	respStr, ok := resp.(string)
	if !ok || !strings.Contains(respStr, "Successfully saved") {
		t.Fatalf("unexpected response: %v", resp)
	}

	entry, found := mgr.Get("default-ns")
	if !found || entry.Content != "production" || entry.Category != "guideline" {
		t.Fatalf("expected entry to be saved in manager, got: %+v (found=%v)", entry, found)
	}
}

func TestMemoryTool_Disabled(t *testing.T) {
	mgr, err := memory.NewDefaultManager(memory.Config{
		Enabled:     false,
		Environment: "test-env",
	}, "")
	if err != nil {
		t.Fatalf("failed to create memory manager: %v", err)
	}

	tool := NewMemoryTool(mgr)
	resp, err := tool.Run(context.Background(), map[string]any{
		"key":     "k",
		"content": "v",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	respStr, ok := resp.(string)
	if !ok || !strings.Contains(respStr, "Environment memory is disabled") {
		t.Fatalf("expected disabled message, got: %v", resp)
	}
}

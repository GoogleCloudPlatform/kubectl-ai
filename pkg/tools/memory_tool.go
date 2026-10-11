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

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/memory"
)

// MemoryTool allows the LLM to store operational preferences and runbook notes in environment-scoped memory.
type MemoryTool struct {
	manager *memory.Manager
}

// NewMemoryTool constructs a MemoryTool backed by the specified memory manager.
func NewMemoryTool(manager *memory.Manager) *MemoryTool {
	return &MemoryTool{manager: manager}
}

func (t *MemoryTool) Name() string {
	return "remember_environment_context"
}

func (t *MemoryTool) Description() string {
	return `Stores an operational preference, runbook guideline, or cluster setting in the environment-scoped memory. Use this tool only when the user explicitly instructs to remember or persist a preference, rule, or setting for this environment.`
}

func (t *MemoryTool) FunctionDefinition() *gollm.FunctionDefinition {
	return &gollm.FunctionDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: &gollm.Schema{
			Type: gollm.TypeObject,
			Properties: map[string]*gollm.Schema{
				"key": {
					Type:        gollm.TypeString,
					Description: "A concise identifier for the memory entry (e.g., 'default-storageclass', 'primary-namespace', 'runbook-dns-troubleshooting').",
				},
				"content": {
					Type:        gollm.TypeString,
					Description: "The operational guideline, preference, or runbook instructions to retain.",
				},
				"category": {
					Type:        gollm.TypeString,
					Description: "Optional category: 'preference', 'runbook', 'guideline', or 'note'. Defaults to 'preference'.",
				},
			},
			Required: []string{"key", "content"},
		},
	}
}

func (t *MemoryTool) Run(_ context.Context, args map[string]any) (any, error) {
	if t.manager == nil || !t.manager.IsEnabled() {
		return "Environment memory is disabled. Set --enable-memory to enable retention.", nil
	}

	key, ok := args["key"].(string)
	if !ok || key == "" {
		return nil, fmt.Errorf("missing required parameter 'key'")
	}

	content, ok := args["content"].(string)
	if !ok || content == "" {
		return nil, fmt.Errorf("missing required parameter 'content'")
	}

	category, _ := args["category"].(string)
	if category == "" {
		category = "preference"
	}

	if err := t.manager.Set(key, content, category); err != nil {
		return nil, fmt.Errorf("saving environment memory: %w", err)
	}

	return fmt.Sprintf("Successfully saved environment memory for %q: [%s] %s: %s", t.manager.Environment(), category, key, content), nil
}

func (t *MemoryTool) IsInteractive(_ map[string]any) (bool, error) {
	return false, nil
}

func (t *MemoryTool) CheckModifiesResource(_ map[string]any) string {
	return "no"
}

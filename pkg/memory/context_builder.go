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

package memory

import (
	"fmt"
	"sort"
	"strings"
)

// ContextBuilder renders formatted context suitable for injection into LLM prompts.
type ContextBuilder interface {
	BuildPromptContext(mem *EnvironmentMemory) string
}

// DefaultContextBuilder formats environment memory into structured text.
type DefaultContextBuilder struct {
	MaxEntries int
}

// NewContextBuilder creates a DefaultContextBuilder.
func NewContextBuilder() *DefaultContextBuilder {
	return &DefaultContextBuilder{
		MaxEntries: 50,
	}
}

// BuildPromptContext returns a formatted block of environment memory notes.
// Entries are ordered deterministically by key to maximize prompt cache reuse.
func (b *DefaultContextBuilder) BuildPromptContext(mem *EnvironmentMemory) string {
	if mem == nil || len(mem.Entries) == 0 {
		return ""
	}

	keys := make([]string, 0, len(mem.Entries))
	for k := range mem.Entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Environment Scope: %s\n", mem.Environment))
	sb.WriteString("Retained operational notes, preferences, and guidelines:\n")

	count := 0
	for _, k := range keys {
		if b.MaxEntries > 0 && count >= b.MaxEntries {
			sb.WriteString(fmt.Sprintf("... (%d additional entries omitted)\n", len(keys)-count))
			break
		}
		entry := mem.Entries[k]
		if entry.Category != "" {
			sb.WriteString(fmt.Sprintf("- [%s] %s: %s\n", entry.Category, entry.Key, entry.Content))
		} else {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", entry.Key, entry.Content))
		}
		count++
	}

	return strings.TrimSpace(sb.String())
}

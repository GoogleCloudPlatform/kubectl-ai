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
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Entry represents a single memorized item such as an operational preference,
// runbook note, or environment guideline.
type Entry struct {
	Key       string    `json:"key"`
	Content   string    `json:"content"`
	Category  string    `json:"category,omitempty"`
	Source    string    `json:"source,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// EnvironmentMemory holds all memory entries scoped to a specific cluster or environment.
type EnvironmentMemory struct {
	Environment string           `json:"environment"`
	Entries     map[string]Entry `json:"entries"`
	LastUpdated time.Time        `json:"lastUpdated"`
}

// Store defines the storage abstraction for persisting environment memory.
type Store interface {
	Load(environment string) (*EnvironmentMemory, error)
	Save(memory *EnvironmentMemory) error
	Delete(environment string) error
	ListEnvironments() ([]string, error)
}

// NoopStore is an implementation of Store used when memory is disabled.
// It performs zero disk I/O and retains zero state across sessions.
type NoopStore struct{}

func NewNoopStore() *NoopStore {
	return &NoopStore{}
}

func (n *NoopStore) Load(environment string) (*EnvironmentMemory, error) {
	return &EnvironmentMemory{
		Environment: environment,
		Entries:     make(map[string]Entry),
		LastUpdated: time.Time{},
	}, nil
}

func (n *NoopStore) Save(_ *EnvironmentMemory) error {
	return nil
}

func (n *NoopStore) Delete(_ string) error {
	return nil
}

func (n *NoopStore) ListEnvironments() ([]string, error) {
	return []string{}, nil
}

// FileStore persists environment-scoped memory as JSON files in a directory.
type FileStore struct {
	baseDir string
	mu      sync.RWMutex
}

// NewFileStore constructs a FileStore rooted at baseDir.
func NewFileStore(baseDir string) *FileStore {
	return &FileStore{
		baseDir: baseDir,
	}
}

func (f *FileStore) environmentFilePath(environment string) string {
	escaped := url.QueryEscape(environment)
	escaped = strings.ReplaceAll(escaped, "/", "%2F")
	escaped = strings.ReplaceAll(escaped, "\\", "%5C")
	return filepath.Join(f.baseDir, fmt.Sprintf("%s.json", escaped))
}

func (f *FileStore) Load(environment string) (*EnvironmentMemory, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	filePath := f.environmentFilePath(environment)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &EnvironmentMemory{
				Environment: environment,
				Entries:     make(map[string]Entry),
				LastUpdated: time.Now(),
			}, nil
		}
		return nil, fmt.Errorf("reading memory file for environment %q: %w", environment, err)
	}

	var mem EnvironmentMemory
	if err := json.Unmarshal(data, &mem); err != nil {
		return nil, fmt.Errorf("decoding memory file for environment %q: %w", environment, err)
	}
	if mem.Entries == nil {
		mem.Entries = make(map[string]Entry)
	}
	if mem.Environment == "" {
		mem.Environment = environment
	}
	return &mem, nil
}

func (f *FileStore) Save(mem *EnvironmentMemory) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := os.MkdirAll(f.baseDir, 0o700); err != nil {
		return fmt.Errorf("creating memory storage directory %q: %w", f.baseDir, err)
	}

	filePath := f.environmentFilePath(mem.Environment)
	tmpPath := fmt.Sprintf("%s.tmp.%d", filePath, time.Now().UnixNano())

	data, err := json.MarshalIndent(mem, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling memory for environment %q: %w", mem.Environment, err)
	}

	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("writing temporary memory file %q: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("renaming temporary memory file to %q: %w", filePath, err)
	}

	return nil
}

func (f *FileStore) Delete(environment string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	filePath := f.environmentFilePath(environment)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing memory file for environment %q: %w", environment, err)
	}
	return nil
}

func (f *FileStore) ListEnvironments() ([]string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	entries, err := os.ReadDir(f.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("listing memory directory %q: %w", f.baseDir, err)
	}

	var envs []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		trimmed := strings.TrimSuffix(entry.Name(), ".json")
		unescaped, err := url.QueryUnescape(trimmed)
		if err != nil {
			envs = append(envs, trimmed)
		} else {
			envs = append(envs, unescaped)
		}
	}
	sort.Strings(envs)
	return envs, nil
}

// MemoryStore is an in-memory Store implementation primarily for testing.
type MemoryStore struct {
	mu   sync.RWMutex
	data map[string]*EnvironmentMemory
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data: make(map[string]*EnvironmentMemory),
	}
}

func (m *MemoryStore) Load(environment string) (*EnvironmentMemory, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	existing, ok := m.data[environment]
	if !ok {
		return &EnvironmentMemory{
			Environment: environment,
			Entries:     make(map[string]Entry),
			LastUpdated: time.Now(),
		}, nil
	}

	copied := &EnvironmentMemory{
		Environment: existing.Environment,
		Entries:     make(map[string]Entry, len(existing.Entries)),
		LastUpdated: existing.LastUpdated,
	}
	for k, v := range existing.Entries {
		copied.Entries[k] = v
	}
	return copied, nil
}

func (m *MemoryStore) Save(mem *EnvironmentMemory) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	copied := &EnvironmentMemory{
		Environment: mem.Environment,
		Entries:     make(map[string]Entry, len(mem.Entries)),
		LastUpdated: mem.LastUpdated,
	}
	for k, v := range mem.Entries {
		copied.Entries[k] = v
	}
	m.data[mem.Environment] = copied
	return nil
}

func (m *MemoryStore) Delete(environment string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.data, environment)
	return nil
}

func (m *MemoryStore) ListEnvironments() ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	envs := make([]string, 0, len(m.data))
	for env := range m.data {
		envs = append(envs, env)
	}
	sort.Strings(envs)
	return envs, nil
}

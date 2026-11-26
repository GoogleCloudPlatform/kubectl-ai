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
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"k8s.io/client-go/tools/clientcmd"
)

// Config configures environment-scoped memory behavior.
type Config struct {
	Enabled     bool   `json:"enabled"`
	Environment string `json:"environment,omitempty"`
	StorageDir  string `json:"storageDir,omitempty"`
}

// Manager orchestrates environment-scoped memory storage, retrieval, and prompt context building.
type Manager struct {
	config         Config
	store          Store
	contextBuilder ContextBuilder
	environment    string
	current        *EnvironmentMemory
	mu             sync.RWMutex
}

// DefaultStorageDir returns the default directory for storing environment memory files.
func DefaultStorageDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("retrieving user home directory: %w", err)
	}
	return filepath.Join(home, ".kubectl-ai", "memory"), nil
}

// ResolveEnvironment resolves the environment identifier from kubeconfig.
// Returns the active context name, or "default" if unresolvable.
func ResolveEnvironment(kubeconfigPath string) string {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfigPath != "" {
		rules.ExplicitPath = kubeconfigPath
	}
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{})
	rawConfig, err := clientConfig.RawConfig()
	if err != nil || rawConfig.CurrentContext == "" {
		return "default"
	}
	return rawConfig.CurrentContext
}

// NewManager creates a Manager with the specified configuration, store, and context builder.
func NewManager(cfg Config, store Store, builder ContextBuilder) (*Manager, error) {
	if builder == nil {
		builder = NewContextBuilder()
	}

	env := cfg.Environment
	if env == "" {
		env = "default"
	}

	if !cfg.Enabled {
		store = NewNoopStore()
	}

	mem, err := store.Load(env)
	if err != nil {
		return nil, fmt.Errorf("loading memory for environment %q: %w", env, err)
	}

	return &Manager{
		config:         cfg,
		store:          store,
		contextBuilder: builder,
		environment:    env,
		current:        mem,
	}, nil
}

// NewDefaultManager creates a Manager using the standard file-based store and kubeconfig environment resolver.
func NewDefaultManager(cfg Config, kubeconfigPath string) (*Manager, error) {
	if !cfg.Enabled {
		return NewManager(cfg, NewNoopStore(), nil)
	}

	if cfg.Environment == "" {
		cfg.Environment = ResolveEnvironment(kubeconfigPath)
	}

	if cfg.StorageDir == "" {
		dir, err := DefaultStorageDir()
		if err != nil {
			return nil, err
		}
		cfg.StorageDir = dir
	}

	store := NewFileStore(cfg.StorageDir)
	return NewManager(cfg, store, nil)
}

// IsEnabled reports whether environment memory is enabled.
func (m *Manager) IsEnabled() bool {
	return m.config.Enabled
}

// Environment returns the active environment identifier.
func (m *Manager) Environment() string {
	return m.environment
}

// Get retrieves an entry by key.
func (m *Manager) Get(key string) (Entry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.current.Entries[key]
	return entry, ok
}

// Set stores or updates an entry for the active environment.
func (m *Manager) Set(key, content string, category ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	cat := ""
	if len(category) > 0 {
		cat = category[0]
	}

	existing, exists := m.current.Entries[key]
	created := now
	if exists {
		created = existing.CreatedAt
		if cat == "" {
			cat = existing.Category
		}
	}

	m.current.Entries[key] = Entry{
		Key:       key,
		Content:   content,
		Category:  cat,
		Source:    "user",
		CreatedAt: created,
		UpdatedAt: now,
	}
	m.current.LastUpdated = now

	return m.store.Save(m.current)
}

// Delete removes an entry from the active environment.
func (m *Manager) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.current.Entries[key]; !exists {
		return nil
	}

	delete(m.current.Entries, key)
	m.current.LastUpdated = time.Now()

	return m.store.Save(m.current)
}

// Clear removes all entries for the active environment.
func (m *Manager) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.current.Entries = make(map[string]Entry)
	m.current.LastUpdated = time.Now()

	if !m.config.Enabled {
		return nil
	}

	return m.store.Delete(m.environment)
}

// List returns all entries sorted alphabetically by key.
func (m *Manager) List() []Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entries := make([]Entry, 0, len(m.current.Entries))
	for _, entry := range m.current.Entries {
		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Key < entries[j].Key
	})

	return entries
}

// BuildPromptContext builds the formatted context block for LLM prompts.
// Returns an empty string when memory is disabled.
func (m *Manager) BuildPromptContext() string {
	if !m.config.Enabled {
		return ""
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.contextBuilder.BuildPromptContext(m.current)
}

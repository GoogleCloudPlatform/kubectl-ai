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

package sessions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"sigs.k8s.io/yaml"
)

// RetentionPolicy defines session and memory retention parameters for an environment.
type RetentionPolicy struct {
	MaxSessions int           `json:"maxSessions,omitempty"` // Maximum sessions retained per environment (0 = unlimited)
	TTL         time.Duration `json:"ttl,omitempty"`         // Maximum age of sessions before eviction (0 = no expiry)
	MaxMemories int           `json:"maxMemories,omitempty"` // Maximum operational notes retained per environment (0 = unlimited)
}

// MemoryEntry represents a scoped operational note, preference, or runbook snippet.
type MemoryEntry struct {
	ID           string    `json:"id"`
	Environment  string    `json:"environment"`
	Key          string    `json:"key"`
	Content      string    `json:"content"`
	Category     string    `json:"category,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	LastAccessed time.Time `json:"lastAccessed"`
}

var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(bearer\s+[a-zA-Z0-9_\-\.]+)|(eyJh[a-zA-Z0-9_\-\.]+)`),
	regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[:=]\s*[^\s]+`),
	regexp.MustCompile(`(?i)(client_secret|clientsecret)\s*[:=]\s*[^\s]+`),
	regexp.MustCompile(`(?i)-----BEGIN[A-Z\s]+PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)(aws_secret_access_key|aws_session_token)\s*[:=]\s*`),
}

// ValidateMemoryEntry checks memory fields and rejects raw secrets or credentials.
func ValidateMemoryEntry(entry *MemoryEntry) error {
	if entry == nil {
		return errors.New("memory entry cannot be nil")
	}
	if strings.TrimSpace(entry.Key) == "" {
		return errors.New("memory key cannot be empty")
	}
	if strings.TrimSpace(entry.Content) == "" {
		return errors.New("memory content cannot be empty")
	}
	if strings.TrimSpace(entry.Environment) == "" {
		return errors.New("memory environment cannot be empty")
	}

	for _, pattern := range sensitivePatterns {
		if pattern.MatchString(entry.Content) || pattern.MatchString(entry.Key) {
			return errors.New("memory entry contains sensitive credentials or secrets; storing credentials is not permitted")
		}
	}
	return nil
}

// MemoryStore manages environment-scoped operational memories.
type MemoryStore interface {
	SaveMemory(entry *MemoryEntry) error
	GetMemory(env, id string) (*MemoryEntry, error)
	ListMemories(env string) ([]*MemoryEntry, error)
	DeleteMemory(env, id string) error
	ClearMemories(env string) error
}

type inMemoryMemoryStore struct {
	mu       sync.RWMutex
	memories map[string]map[string]*MemoryEntry // env -> id -> entry
}

// NewInMemoryMemoryStore creates an in-memory MemoryStore.
func NewInMemoryMemoryStore() MemoryStore {
	return &inMemoryMemoryStore{
		memories: make(map[string]map[string]*MemoryEntry),
	}
}

func (s *inMemoryMemoryStore) SaveMemory(entry *MemoryEntry) error {
	if err := ValidateMemoryEntry(entry); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	env := SanitizeEnvironment(entry.Environment)
	entry.Environment = env
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	now := time.Now()
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	entry.LastAccessed = now

	if _, exists := s.memories[env]; !exists {
		s.memories[env] = make(map[string]*MemoryEntry)
	}

	s.memories[env][entry.ID] = entry
	return nil
}

func (s *inMemoryMemoryStore) GetMemory(env, id string) (*MemoryEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	env = SanitizeEnvironment(env)
	envMemories, exists := s.memories[env]
	if !exists {
		return nil, errors.New("memory not found")
	}

	entry, ok := envMemories[id]
	if !ok {
		return nil, errors.New("memory not found")
	}

	return entry, nil
}

func (s *inMemoryMemoryStore) ListMemories(env string) ([]*MemoryEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	env = SanitizeEnvironment(env)
	envMemories, exists := s.memories[env]
	if !exists {
		return []*MemoryEntry{}, nil
	}

	result := make([]*MemoryEntry, 0, len(envMemories))
	for _, entry := range envMemories {
		result = append(result, entry)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})

	return result, nil
}

func (s *inMemoryMemoryStore) DeleteMemory(env, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	env = SanitizeEnvironment(env)
	envMemories, exists := s.memories[env]
	if !exists {
		return errors.New("memory not found")
	}

	if _, ok := envMemories[id]; !ok {
		return errors.New("memory not found")
	}

	delete(envMemories, id)
	return nil
}

func (s *inMemoryMemoryStore) ClearMemories(env string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	env = SanitizeEnvironment(env)
	delete(s.memories, env)
	return nil
}

type fileMemoryStore struct {
	basePath string
	mu       sync.RWMutex
}

// NewFileMemoryStore creates a filesystem-backed MemoryStore.
func NewFileMemoryStore(basePath string) MemoryStore {
	return &fileMemoryStore{
		basePath: basePath,
	}
}

func (s *fileMemoryStore) envDir(env string) string {
	return filepath.Join(s.basePath, "memory", SanitizeEnvironment(env))
}

func (s *fileMemoryStore) envFile(env string) string {
	return filepath.Join(s.envDir(env), "memories.yaml")
}

func (s *fileMemoryStore) loadEntries(env string) (map[string]*MemoryEntry, error) {
	filePath := s.envFile(env)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return make(map[string]*MemoryEntry), nil
		}
		return nil, err
	}

	var entries []*MemoryEntry
	if err := yaml.Unmarshal(data, &entries); err != nil {
		return nil, err
	}

	result := make(map[string]*MemoryEntry, len(entries))
	for _, e := range entries {
		result[e.ID] = e
	}
	return result, nil
}

func (s *fileMemoryStore) writeEntries(env string, entries map[string]*MemoryEntry) error {
	dir := s.envDir(env)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	slice := make([]*MemoryEntry, 0, len(entries))
	for _, e := range entries {
		slice = append(slice, e)
	}

	sort.Slice(slice, func(i, j int) bool {
		return slice[i].CreatedAt.Before(slice[j].CreatedAt)
	})

	data, err := yaml.Marshal(slice)
	if err != nil {
		return err
	}

	return os.WriteFile(s.envFile(env), data, 0o644)
}

func (s *fileMemoryStore) SaveMemory(entry *MemoryEntry) error {
	if err := ValidateMemoryEntry(entry); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	env := SanitizeEnvironment(entry.Environment)
	entry.Environment = env
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	now := time.Now()
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	entry.LastAccessed = now

	entries, err := s.loadEntries(env)
	if err != nil {
		return err
	}

	entries[entry.ID] = entry
	return s.writeEntries(env, entries)
}

func (s *fileMemoryStore) GetMemory(env, id string) (*MemoryEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := s.loadEntries(env)
	if err != nil {
		return nil, err
	}

	entry, ok := entries[id]
	if !ok {
		return nil, errors.New("memory not found")
	}

	return entry, nil
}

func (s *fileMemoryStore) ListMemories(env string) ([]*MemoryEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := s.loadEntries(env)
	if err != nil {
		return nil, err
	}

	result := make([]*MemoryEntry, 0, len(entries))
	for _, e := range entries {
		result = append(result, e)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})

	return result, nil
}

func (s *fileMemoryStore) DeleteMemory(env, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.loadEntries(env)
	if err != nil {
		return err
	}

	if _, ok := entries[id]; !ok {
		return errors.New("memory not found")
	}

	delete(entries, id)
	return s.writeEntries(env, entries)
}

func (s *fileMemoryStore) ClearMemories(env string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filePath := s.envFile(env)
	if err := os.Remove(filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("clearing memories for environment %q: %w", env, err)
	}
	return nil
}

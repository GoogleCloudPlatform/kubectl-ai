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
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
)

type SessionManager struct {
	store           Store
	memoryStore     MemoryStore
	environment     string
	retentionPolicy RetentionPolicy
}

// ManagerOption configures a SessionManager instance.
type ManagerOption func(*SessionManager)

// WithEnvironment scopes the SessionManager to a specific environment.
func WithEnvironment(env string) ManagerOption {
	return func(sm *SessionManager) {
		sm.environment = SanitizeEnvironment(env)
	}
}

// WithRetentionPolicy configures session and memory retention bounds.
func WithRetentionPolicy(policy RetentionPolicy) ManagerOption {
	return func(sm *SessionManager) {
		sm.retentionPolicy = policy
	}
}

// WithStore overrides the session store implementation.
func WithStore(store Store) ManagerOption {
	return func(sm *SessionManager) {
		sm.store = store
	}
}

// WithMemoryStore overrides the operational memory store implementation.
func WithMemoryStore(memoryStore MemoryStore) ManagerOption {
	return func(sm *SessionManager) {
		sm.memoryStore = memoryStore
	}
}

// NewSessionManager creates a SessionManager with optional configuration options.
func NewSessionManager(backend string, opts ...ManagerOption) (*SessionManager, error) {
	var store Store
	var memStore MemoryStore
	var err error

	if backend == "" {
		// Try filesystem first
		store, err = NewStore("filesystem")
		if err != nil {
			// Fallback to memory
			store, err = NewStore("memory")
			memStore = NewInMemoryMemoryStore()
		} else {
			basePath, _ := defaultFilesystemBasePath()
			memStore = NewFileMemoryStore(basePath)
		}
	} else if backend == "filesystem" {
		store, err = NewStore("filesystem")
		if err == nil {
			basePath, _ := defaultFilesystemBasePath()
			memStore = NewFileMemoryStore(basePath)
		}
	} else {
		store, err = NewStore(backend)
		memStore = NewInMemoryMemoryStore()
	}

	if err != nil {
		return nil, err
	}

	sm := &SessionManager{
		store:       store,
		memoryStore: memStore,
	}

	for _, opt := range opts {
		opt(sm)
	}

	return sm, nil
}

// Environment returns the active environment scope.
func (sm *SessionManager) Environment() string {
	return sm.environment
}

// SetEnvironment updates the environment scope.
func (sm *SessionManager) SetEnvironment(env string) {
	sm.environment = SanitizeEnvironment(env)
}

// RetentionPolicy returns the configured retention policy.
func (sm *SessionManager) RetentionPolicy() RetentionPolicy {
	return sm.retentionPolicy
}

// SetRetentionPolicy sets the retention policy.
func (sm *SessionManager) SetRetentionPolicy(policy RetentionPolicy) {
	sm.retentionPolicy = policy
}

// NewSession creates and registers a new session, stamping it with the environment scope.
func (sm *SessionManager) NewSession(meta Metadata) (*api.Session, error) {
	suffix := fmt.Sprintf("%04d", rand.Intn(10000))
	sessionID := time.Now().Format("20060102") + "-" + suffix

	env := meta.Environment
	if env == "" && sm.environment != "" {
		env = sm.environment
	}
	if env != "" {
		env = SanitizeEnvironment(env)
	}

	now := time.Now()
	session := &api.Session{
		ID:           sessionID,
		Name:         "Session " + sessionID,
		ProviderID:   meta.ProviderID,
		ModelID:      meta.ModelID,
		Environment:  env,
		AgentState:   api.AgentStateIdle,
		CreatedAt:    now,
		LastModified: now,
	}

	if err := sm.store.CreateSession(session); err != nil {
		return nil, err
	}

	if env != "" {
		_, _ = sm.EnforceRetention(env)
	}

	return session, nil
}

// ListSessions lists sessions, filtering by the active environment if one is configured.
func (sm *SessionManager) ListSessions() ([]*api.Session, error) {
	if sm.environment != "" {
		return sm.ListSessionsForEnvironment(sm.environment)
	}
	return sm.store.ListSessions()
}

// ListSessionsForEnvironment lists sessions strictly belonging to the specified environment.
func (sm *SessionManager) ListSessionsForEnvironment(env string) ([]*api.Session, error) {
	sanitized := SanitizeEnvironment(env)
	return sm.store.ListSessionsByEnvironment(sanitized)
}

// ListAllSessions returns all sessions regardless of environment scope.
func (sm *SessionManager) ListAllSessions() ([]*api.Session, error) {
	return sm.store.ListSessions()
}

// FindSessionByID retrieves a session, enforcing environment isolation when scoped.
func (sm *SessionManager) FindSessionByID(id string) (*api.Session, error) {
	session, err := sm.store.GetSession(id)
	if err != nil {
		return nil, err
	}

	if sm.environment != "" && session.Environment != "" && session.Environment != sm.environment {
		return nil, fmt.Errorf("session %s belongs to environment %q (isolated from %q)", id, session.Environment, sm.environment)
	}

	return session, nil
}

// DeleteSession removes a session, verifying isolation before removal.
func (sm *SessionManager) DeleteSession(id string) error {
	if sm.environment != "" {
		session, err := sm.store.GetSession(id)
		if err != nil {
			return err
		}
		if session.Environment != "" && session.Environment != sm.environment {
			return fmt.Errorf("session %s belongs to environment %q (isolated from %q)", id, session.Environment, sm.environment)
		}
	}
	return sm.store.DeleteSession(id)
}

// GetLatestSession returns the newest session within the active environment scope.
func (sm *SessionManager) GetLatestSession() (*api.Session, error) {
	var sessions []*api.Session
	var err error

	if sm.environment != "" {
		sessions, err = sm.ListSessionsForEnvironment(sm.environment)
	} else {
		sessions, err = sm.store.ListSessions()
	}

	if err != nil {
		return nil, err
	}

	if len(sessions) == 0 {
		return nil, nil
	}

	latest := sessions[0]
	for _, session := range sessions[1:] {
		if session.LastModified.After(latest.LastModified) {
			latest = session
		}
	}

	return latest, nil
}

// UpdateLastAccessed updates the last modified timestamp for a session.
func (sm *SessionManager) UpdateLastAccessed(session *api.Session) error {
	session.LastModified = time.Now()
	return sm.store.UpdateSession(session)
}

// EnforceRetention deletes expired sessions or excess sessions exceeding MaxSessions in an environment.
func (sm *SessionManager) EnforceRetention(env string) ([]string, error) {
	sanitized := SanitizeEnvironment(env)
	if sm.retentionPolicy.MaxSessions <= 0 && sm.retentionPolicy.TTL <= 0 {
		return nil, nil
	}

	sessions, err := sm.ListSessionsForEnvironment(sanitized)
	if err != nil {
		return nil, err
	}

	var evictedIDs []string
	now := time.Now()

	// 1. Evict by TTL
	var activeSessions []*api.Session
	for _, sess := range sessions {
		if sm.retentionPolicy.TTL > 0 && now.Sub(sess.LastModified) > sm.retentionPolicy.TTL {
			if err := sm.store.DeleteSession(sess.ID); err == nil {
				evictedIDs = append(evictedIDs, sess.ID)
			}
		} else {
			activeSessions = append(activeSessions, sess)
		}
	}

	// 2. Evict by MaxSessions
	if sm.retentionPolicy.MaxSessions > 0 && len(activeSessions) > sm.retentionPolicy.MaxSessions {
		// activeSessions is sorted newest first. Evict from the tail.
		excess := len(activeSessions) - sm.retentionPolicy.MaxSessions
		for i := len(activeSessions) - excess; i < len(activeSessions); i++ {
			toEvict := activeSessions[i]
			if err := sm.store.DeleteSession(toEvict.ID); err == nil {
				evictedIDs = append(evictedIDs, toEvict.ID)
			}
		}
	}

	return evictedIDs, nil
}

// SaveMemory validates, persists, and enforces retention on operational notes for an environment.
func (sm *SessionManager) SaveMemory(entry *MemoryEntry) error {
	if sm.memoryStore == nil {
		return fmt.Errorf("memory store is not configured")
	}

	if entry.Environment == "" && sm.environment != "" {
		entry.Environment = sm.environment
	}

	if err := sm.memoryStore.SaveMemory(entry); err != nil {
		return err
	}

	_, _ = sm.EnforceMemoryRetention(entry.Environment)
	return nil
}

// GetMemory retrieves a specific operational memory entry.
func (sm *SessionManager) GetMemory(env, id string) (*MemoryEntry, error) {
	if sm.memoryStore == nil {
		return nil, fmt.Errorf("memory store is not configured")
	}
	return sm.memoryStore.GetMemory(env, id)
}

// ListMemories lists all operational memory entries for an environment.
func (sm *SessionManager) ListMemories(env string) ([]*MemoryEntry, error) {
	if sm.memoryStore == nil {
		return []*MemoryEntry{}, nil
	}
	return sm.memoryStore.ListMemories(env)
}

// DeleteMemory deletes an operational memory entry.
func (sm *SessionManager) DeleteMemory(env, id string) error {
	if sm.memoryStore == nil {
		return fmt.Errorf("memory store is not configured")
	}
	return sm.memoryStore.DeleteMemory(env, id)
}

// ClearMemories deletes all operational memories for an environment.
func (sm *SessionManager) ClearMemories(env string) error {
	if sm.memoryStore == nil {
		return fmt.Errorf("memory store is not configured")
	}
	return sm.memoryStore.ClearMemories(env)
}

// EnforceMemoryRetention drops excess operational notes beyond MaxMemories in an environment.
func (sm *SessionManager) EnforceMemoryRetention(env string) ([]string, error) {
	if sm.memoryStore == nil || sm.retentionPolicy.MaxMemories <= 0 {
		return nil, nil
	}

	memories, err := sm.memoryStore.ListMemories(env)
	if err != nil {
		return nil, err
	}

	if len(memories) <= sm.retentionPolicy.MaxMemories {
		return nil, nil
	}

	sort.Slice(memories, func(i, j int) bool {
		return memories[i].LastAccessed.After(memories[j].LastAccessed)
	})

	excess := len(memories) - sm.retentionPolicy.MaxMemories
	var evictedIDs []string
	for i := len(memories) - excess; i < len(memories); i++ {
		toEvict := memories[i]
		if err := sm.memoryStore.DeleteMemory(env, toEvict.ID); err == nil {
			evictedIDs = append(evictedIDs, toEvict.ID)
		}
	}

	return evictedIDs, nil
}

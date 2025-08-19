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
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
)

// SessionManager manages chat sessions.
type SessionManager struct {
	store Store
}

// NewSessionManager creates a new SessionManager with the specified backend.
func NewSessionManager(backend string) (*SessionManager, error) {
	store, err := NewStore(backend)
	if err != nil {
		return nil, err
	}
	return &SessionManager{store: store}, nil
}

// NewSession creates a new session.
func (sm *SessionManager) NewSession(meta Metadata) (*api.Session, error) {
	// Generate a unique session ID with date prefix and random suffix
	suffix := fmt.Sprintf("%04d", rand.Intn(1000))
	sessionID := time.Now().Format("20060102") + "-" + suffix
	session := &api.Session{
		ID:           sessionID,
		ProviderID:   meta.ProviderID,
		ModelID:      meta.ModelID,
		AgentState:   api.AgentStateIdle,
		CreatedAt:    time.Now(),
		LastModified: time.Now(),
	}

	if err := sm.store.CreateSession(session); err != nil {
		return nil, err
	}
	return session, nil
}

// ListSessions lists all available sessions.
func (sm *SessionManager) ListSessions() ([]*api.Session, error) {
	return sm.store.ListSessions()
}

// GetLatestSession returns the latest session.
func (sm *SessionManager) GetLatestSession() (*api.Session, error) {
	sessions, err := sm.store.ListSessions()
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, nil // No sessions found
	}
	return sessions[0], nil
}

// FindSessionByID finds a session by its ID.
func (sm *SessionManager) FindSessionByID(id string) (*api.Session, error) {
	return sm.store.GetSession(id)
}

// DeleteSession deletes a session by its ID.
func (sm *SessionManager) DeleteSession(id string) error {
	return sm.store.DeleteSession(id)
}

// UpdateLastAccessed updates the last accessed timestamp for a session.
func (sm *SessionManager) UpdateLastAccessed(session *api.Session) error {
	session.LastModified = time.Now()
	return sm.store.UpdateSession(session)
}

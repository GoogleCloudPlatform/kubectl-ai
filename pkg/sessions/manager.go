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
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	sessionsDirName = "sessions"
	timeFormat      = "20060102"
)

// SessionManager manages the chat sessions.
type SessionManager struct {
	BasePath string
}

// NewSessionManager creates a new SessionManager.
// It will create the base directory for sessions if it does not exist.
func NewSessionManager() (*SessionManager, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	basePath := filepath.Join(homeDir, ".kubectl-ai", sessionsDirName)
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, err
	}
	return &SessionManager{
		BasePath: basePath,
	}, nil
}

// NewSession creates a new session.
func (m *SessionManager) NewSession(meta Metadata) (*Session, error) {
	// Generate a unique session ID with date prefix and random suffix
	suffix := fmt.Sprintf("%04d", rand.Intn(10000))
	sessionID := time.Now().Format(timeFormat) + "-" + suffix
	sessionPath := filepath.Join(m.BasePath, sessionID)

	if err := os.MkdirAll(sessionPath, 0755); err != nil {
		return nil, err
	}

	s := &Session{
		ID:   sessionID,
		Path: sessionPath,
	}

	// Set creation and last accessed times
	meta.CreatedAt = time.Now()
	meta.LastAccessed = time.Now()

	if err := s.SaveMetadata(&meta); err != nil {
		return nil, err
	}

	return s, nil
}

// ListSessions lists all the sessions.
func (m *SessionManager) ListSessions() ([]*Session, error) {
	entries, err := os.ReadDir(m.BasePath)
	if err != nil {
		return nil, err
	}

	var sessions []*Session
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sessions = append(sessions, &Session{
			ID:   entry.Name(),
			Path: filepath.Join(m.BasePath, entry.Name()),
		})
	}

	// Sort sessions by name, which will sort by date (newest first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].ID > sessions[j].ID
	})

	return sessions, nil
}

// GetLatestSession returns the latest session.
func (m *SessionManager) GetLatestSession() (*Session, error) {
	sessions, err := m.ListSessions()
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, nil // No sessions found
	}
	return sessions[0], nil
}

// FindSessionByID finds a session by its ID.
func (m *SessionManager) FindSessionByID(id string) (*Session, error) {
	sessions, err := m.ListSessions()
	if err != nil {
		return nil, err
	}
	for _, s := range sessions {
		if s.ID == id {
			return s, nil
		}
	}
	return nil, fmt.Errorf("session with ID %q not found", id)
}

// DeleteSession deletes a session and all its data.
func (m *SessionManager) DeleteSession(id string) error {
	session, err := m.FindSessionByID(id)
	if err != nil {
		return err
	}

	return os.RemoveAll(session.Path)
}

// GetSessionInfo returns detailed information about a session including metadata.
func (m *SessionManager) GetSessionInfo(id string) (*Session, *Metadata, error) {
	session, err := m.FindSessionByID(id)
	if err != nil {
		return nil, nil, err
	}

	metadata, err := session.LoadMetadata()
	if err != nil {
		return nil, nil, err
	}

	return session, metadata, nil
}

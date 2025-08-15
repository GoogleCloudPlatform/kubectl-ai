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
	"sort"
	"sync"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
)

// memoryStore is an in-memory implementation of the Store interface.
type memoryStore struct {
	mu       sync.RWMutex
	sessions map[string]*api.Session
}

// newMemoryStore creates a new memoryStore.
func newMemoryStore() Store {
	return &memoryStore{
		sessions: make(map[string]*api.Session),
	}
}

// GetSession retrieves a session by its ID.
func (s *memoryStore) GetSession(id string) (*api.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session with ID %q not found", id)
	}
	return session, nil
}

// CreateSession creates a new session.
func (s *memoryStore) CreateSession(session *api.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[session.ID]; ok {
		return fmt.Errorf("session with ID %q already exists", session.ID)
	}
	session.ChatMessageStore = NewInMemoryChatStore()
	s.sessions[session.ID] = session
	return nil
}

// UpdateSession updates an existing session.
func (s *memoryStore) UpdateSession(session *api.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[session.ID]; !ok {
		return fmt.Errorf("session with ID %q not found", session.ID)
	}
	s.sessions[session.ID] = session
	return nil
}

// ListSessions lists all available sessions.
func (s *memoryStore) ListSessions() ([]*api.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var sessionList []*api.Session
	for _, session := range s.sessions {
		sessionList = append(sessionList, session)
	}

	// Sort sessions by last modified date
	sort.Slice(sessionList, func(i, j int) bool {
		return sessionList[i].LastModified.After(sessionList[j].LastModified)
	})

	return sessionList, nil
}

// DeleteSession deletes a session by its ID.
func (s *memoryStore) DeleteSession(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[id]; !ok {
		return fmt.Errorf("session with ID %q not found", id)
	}
	delete(s.sessions, id)
	return nil
}

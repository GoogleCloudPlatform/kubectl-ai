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
	"sort"
	"sync"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
)

type memoryStore struct {
	mu       sync.RWMutex
	sessions map[string]*api.Session
}

func newMemoryStore() Store {
	return &memoryStore{sessions: make(map[string]*api.Session)}
}

func (m *memoryStore) GetSession(id string) (*api.Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.sessions[id]
	if !ok {
		return nil, errors.New("session not found")
	}
	return session, nil
}

func (m *memoryStore) CreateSession(session *api.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.sessions[session.ID]; exists {
		return errors.New("session already exists")
	}

	if session.ChatMessageStore == nil {
		session.ChatMessageStore = NewInMemoryChatStore()
	}

	m.sessions[session.ID] = session
	return nil
}

func (m *memoryStore) UpdateSession(session *api.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.sessions[session.ID]; !exists {
		return errors.New("session not found")
	}

	m.sessions[session.ID] = session
	return nil
}

func (m *memoryStore) ListSessions() ([]*api.Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessions := make([]*api.Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastModified.After(sessions[j].LastModified)
	})

	return sessions, nil
}

func (m *memoryStore) ListSessionsByEnvironment(env string) ([]*api.Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessions := make([]*api.Session, 0)
	for _, session := range m.sessions {
		if session.Environment == env {
			sessions = append(sessions, session)
		}
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastModified.After(sessions[j].LastModified)
	})

	return sessions, nil
}

func (m *memoryStore) DeleteSession(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.sessions[id]; !exists {
		return errors.New("session not found")
	}

	delete(m.sessions, id)
	return nil
}

// InMemoryChatStore is an in-memory implementation of the api.ChatMessageStore interface.
// It stores chat messages in a slice and is safe for concurrent use.
type InMemoryChatStore struct {
	mu       sync.RWMutex
	messages []*api.Message
}

// NewInMemoryChatStore creates a new InMemoryChatStore.
func NewInMemoryChatStore() *InMemoryChatStore {
	return &InMemoryChatStore{
		messages: make([]*api.Message, 0),
	}
}

// AddChatMessage adds a message to the store.
func (s *InMemoryChatStore) AddChatMessage(record *api.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, record)
	return nil
}

// SetChatMessages replaces the entire chat history with a new one.
func (s *InMemoryChatStore) SetChatMessages(newHistory []*api.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = newHistory
	return nil
}

// ChatMessages returns all chat messages from the store.
func (s *InMemoryChatStore) ChatMessages() []*api.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	messageCopy := make([]*api.Message, len(s.messages))
	copy(messageCopy, s.messages)
	return messageCopy
}

// ClearChatMessages removes all messages from the store.
func (s *InMemoryChatStore) ClearChatMessages() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = make([]*api.Message, 0)
	return nil
}

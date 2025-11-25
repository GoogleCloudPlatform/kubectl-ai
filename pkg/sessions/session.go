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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
	"sigs.k8s.io/yaml"
)

const (
	metadataFileName = "metadata.yaml"
	historyFileName  = "history.json"
)

// Metadata contains metadata about a session
type Metadata struct {
	ProviderID     string    `json:"providerID"`
	ModelID        string    `json:"modelID"`
	CreatedAt      time.Time `json:"createdAt"`
	LastAccessed   time.Time `json:"lastAccessed"`
	TokensConsumed int64     `json:"tokensConsumed"`
}

// Session represents a single chat session.
type Session struct {
	ID   string
	Path string
	mu   sync.Mutex
}

// HistoryPath returns the path to the history file for the session.
func (s *Session) HistoryPath() string {
	return filepath.Join(s.Path, historyFileName)
}

// MetadataPath returns the path to the metadata file for the session.
func (s *Session) MetadataPath() string {
	return filepath.Join(s.Path, metadataFileName)
}

// LoadMetadata loads the metadata for the session.
func (s *Session) LoadMetadata() (*Metadata, error) {
	b, err := os.ReadFile(s.MetadataPath())
	if err != nil {
		return nil, err
	}
	var m Metadata
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// SaveMetadata saves the metadata for the session.
func (s *Session) SaveMetadata(m *Metadata) error {
	b, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(s.MetadataPath(), b, 0644)
}

// UpdateLastAccessed updates the last accessed timestamp in the metadata.
func (s *Session) UpdateLastAccessed() error {
	m, err := s.LoadMetadata()
	if err != nil {
		return err
	}
	m.LastAccessed = time.Now()
	return s.SaveMetadata(m)
}

// AddChatMessage appends a new message to the history and persists it to the sessions's history file.
func (s *Session) AddChatMessage(msg *api.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(s.HistoryPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

// SetChatMessages replaces the current messages with a new set of messages and overwrites the session's history file.
func (s *Session) SetChatMessages(newMessages []*api.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(s.HistoryPath(), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, msg := range newMessages {
		b, err := json.Marshal(msg)
		if err != nil {
			return err
		}
		if _, err := f.Write(append(b, '\n')); err != nil {
			return err
		}
	}
	return nil
}

// ChatMessages returns all messages from the session's history file.
func (s *Session) ChatMessages() []*api.Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	var messages []*api.Message

	f, err := os.Open(s.HistoryPath())
	if err != nil {
		return nil
	}
	defer f.Close()

	scanner := json.NewDecoder(f)
	for scanner.More() {
		var message api.Message
		if err := scanner.Decode(&message); err != nil {
			continue // skip malformed messages
		}
		messages = append(messages, &message)
	}

	return messages
}

// ClearChatMessages removes all records from the history and truncates the session's history file.
func (s *Session) ClearChatMessages() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Truncate the file by opening it with O_TRUNC
	f, err := os.OpenFile(s.HistoryPath(), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	return f.Close()
}

func (s *Session) String() (string, error) {
	metadata, err := s.LoadMetadata()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Current session:\n\nID: %s\nCreated: %s\nLast Accessed: %s\nModel: %s\nProvider: %s\n\n",
		s.ID,
		metadata.CreatedAt.Format("2006-01-02 15:04:05"),
		metadata.LastAccessed.Format("2006-01-02 15:04:05"),
		metadata.ModelID,
		metadata.ProviderID), nil
}

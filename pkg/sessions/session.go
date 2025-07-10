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
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	"sigs.k8s.io/yaml"
)

const (
	metadataFileName = "metadata.yaml"
	historyFileName  = "history.jsonl"
)

// Metadata contains metadata about a session.
type Metadata struct {
	ProviderID   string    `json:"providerID"`
	ModelID      string    `json:"modelID"`
	CreatedAt    time.Time `json:"createdAt"`
	LastAccessed time.Time `json:"lastAccessed"`
	TotalTokens  int64     `json:"totalTokens,omitempty"`
	TotalCost    float64   `json:"totalCost,omitempty"`
	MessageCount int       `json:"messageCount,omitempty"`
}

// Session represents a single chat session.
type Session struct {
	ID   string
	Path string
	mu   sync.Mutex
}

// Session implements the ChatMessageStore interface.
var _ ChatMessageStore = &Session{}

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
	meta, err := s.LoadMetadata()
	if err != nil {
		return err
	}
	meta.LastAccessed = time.Now()
	return s.SaveMetadata(meta)
}

// UpdateUsage updates the usage statistics in the metadata.
func (s *Session) UpdateUsage(tokens int64, cost float64, messageCount int) error {
	meta, err := s.LoadMetadata()
	if err != nil {
		return err
	}
	meta.TotalTokens += tokens
	meta.TotalCost += cost
	meta.MessageCount += messageCount
	meta.LastAccessed = time.Now()
	return s.SaveMetadata(meta)
}

// AddChatMessage appends a new message to the history and persists it to the session's history file.
func (s *Session) AddChatMessage(message *gollm.ChatMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(s.HistoryPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	b, err := json.Marshal(message)
	if err != nil {
		return err
	}

	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

// SetChatMessages replaces the current messages with a new set of messages and overwrites the session's history file.
func (s *Session) SetChatMessages(newMessages []*gollm.ChatMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(s.HistoryPath(), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, message := range newMessages {
		b, err := json.Marshal(message)
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
func (s *Session) ChatMessages() []*gollm.ChatMessage {
	s.mu.Lock()
	defer s.mu.Unlock()

	var messages []*gollm.ChatMessage

	f, err := os.Open(s.HistoryPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet, return empty messages
		}
		return nil // Return empty messages on other errors
	}
	defer f.Close()

	scanner := json.NewDecoder(f)
	for scanner.More() {
		var message gollm.ChatMessage
		if err := scanner.Decode(&message); err != nil {
			continue // Skip malformed messages
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

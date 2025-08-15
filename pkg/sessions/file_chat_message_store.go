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

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
)

const (
	historyFileName = "history.json"
)

// FileChatMessageStore is a file-based implementation of the api.ChatMessageStore interface.
type FileChatMessageStore struct {
	Path string
	mu   sync.Mutex
}

// NewFileChatMessageStore creates a new FileChatMessageStore.
func NewFileChatMessageStore(path string) *FileChatMessageStore {
	return &FileChatMessageStore{
		Path: path,
	}
}

// HistoryPath returns the path to the history file for the session.
func (s *FileChatMessageStore) HistoryPath() string {
	return filepath.Join(s.Path, historyFileName)
}

// AddChatMessage appends a new message to the history and persists it to the sessions's history file.
func (s *FileChatMessageStore) AddChatMessage(msg *api.Message) error {
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
func (s *FileChatMessageStore) SetChatMessages(newMessages []*api.Message) error {
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
func (s *FileChatMessageStore) ChatMessages() []*api.Message {
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
func (s *FileChatMessageStore) ClearChatMessages() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Truncate the file by opening it with O_TRUNC
	f, err := os.OpenFile(s.HistoryPath(), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	return f.Close()
}

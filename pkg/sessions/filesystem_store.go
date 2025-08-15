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
	"os"
	"path/filepath"
	"sort"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
	"sigs.k8s.io/yaml"
)

const (
	metadataFileName = "metadata.yaml"
)

// filesystemStore is a file-based implementation of the Store interface.
type filesystemStore struct {
	basePath string
}

// newFilesystemStore creates a new filesystemStore.
func newFilesystemStore(basePath string) Store {
	return &filesystemStore{basePath: basePath}
}

// GetSession retrieves a session by its ID.
func (s *filesystemStore) GetSession(id string) (*api.Session, error) {
	sessionPath := filepath.Join(s.basePath, id)
	metadata, err := s.loadMetadata(sessionPath)
	if err != nil {
		return nil, err
	}

	chatStore := NewFileChatMessageStore(sessionPath)
	return &api.Session{
		ID:               id,
		ProviderID:       metadata.ProviderID,
		ModelID:          metadata.ModelID,
		ChatMessageStore: chatStore,
		CreatedAt:        metadata.CreatedAt,
		LastModified:     metadata.LastAccessed,
	}, nil
}

// CreateSession creates a new session.
func (s *filesystemStore) CreateSession(session *api.Session) error {
	sessionPath := filepath.Join(s.basePath, session.ID)
	if err := os.MkdirAll(sessionPath, 0755); err != nil {
		return err
	}
	session.ChatMessageStore = NewFileChatMessageStore(sessionPath)
	metadata := &Metadata{
		ProviderID:   session.ProviderID,
		ModelID:      session.ModelID,
		CreatedAt:    session.CreatedAt,
		LastAccessed: session.LastModified,
	}

	return s.saveMetadata(sessionPath, metadata)
}

// UpdateSession updates an existing session's metadata.
func (s *filesystemStore) UpdateSession(session *api.Session) error {
	sessionPath := filepath.Join(s.basePath, session.ID)
	metadata, err := s.loadMetadata(sessionPath)
	if err != nil {
		return err
	}

	metadata.LastAccessed = session.LastModified
	metadata.ProviderID = session.ProviderID
	metadata.ModelID = session.ModelID
	return s.saveMetadata(sessionPath, metadata)
}

// ListSessions lists all available sessions.
func (s *filesystemStore) ListSessions() ([]*api.Session, error) {
	entries, err := os.ReadDir(s.basePath)
	if err != nil {
		return nil, err
	}

	var sessions []*api.Session
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		session, err := s.GetSession(entry.Name())
		if err != nil {
			// Log the error but continue, so one corrupted session doesn't break the whole list
			fmt.Printf("warning: could not load session %q: %v\n", entry.Name(), err)
			continue
		}
		sessions = append(sessions, session)
	}

	// Sort sessions by last modified date
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastModified.After(sessions[j].LastModified)
	})

	return sessions, nil
}

// DeleteSession deletes a session by its ID.
func (s *filesystemStore) DeleteSession(id string) error {
	sessionPath := filepath.Join(s.basePath, id)
	if _, err := os.Stat(sessionPath); os.IsNotExist(err) {
		return fmt.Errorf("session with ID %q not found", id)
	}
	return os.RemoveAll(sessionPath)
}

func (s *filesystemStore) loadMetadata(sessionPath string) (*Metadata, error) {
	b, err := os.ReadFile(filepath.Join(sessionPath, metadataFileName))
	if err != nil {
		return nil, err
	}
	var m Metadata
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *filesystemStore) saveMetadata(sessionPath string, m *Metadata) error {
	b, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(sessionPath, metadataFileName), b, 0644)
}

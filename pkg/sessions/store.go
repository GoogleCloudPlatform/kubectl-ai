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

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
)

const (
	sessionsDirName = "sessions"
)

// Store defines the interface for session persistence.
type Store interface {
	// GetSession retrieves a session by its ID.
	GetSession(id string) (*api.Session, error)
	// CreateSession creates a new session.
	CreateSession(session *api.Session) error
	// UpdateSession updates an existing session's metadata.
	UpdateSession(session *api.Session) error
	// ListSessions lists all available sessions.
	ListSessions() ([]*api.Session, error)
	// DeleteSession deletes a session by its ID.
	DeleteSession(id string) error
}

// NewStore creates a new store based on the specified backend type.
func NewStore(backend string) (Store, error) {
	switch backend {
	case "memory":
		return newMemoryStore(), nil
	case "filesystem":
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		basePath := filepath.Join(homeDir, ".kubectl-ai", sessionsDirName)
		if err := os.MkdirAll(basePath, 0755); err != nil {
			return nil, err
		}
		return newFilesystemStore(basePath), nil
	default:
		return nil, fmt.Errorf("unknown session backend: %s", backend)
	}
}

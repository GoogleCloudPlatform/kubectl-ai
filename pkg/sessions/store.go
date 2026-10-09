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

	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
)

const sessionsDirName = "sessions"

type Metadata struct {
	ProviderID   string    `json:"providerID"`
	ModelID      string    `json:"modelID"`
	CreatedAt    time.Time `json:"createdAt"`
	LastAccessed time.Time `json:"lastAccessed"`
	Environment  string    `json:"environment,omitempty"`
}

var defaultMemoryStore Store = newMemoryStore()

type Store interface {
	GetSession(id string) (*api.Session, error)
	CreateSession(session *api.Session) error
	UpdateSession(session *api.Session) error
	ListSessions() ([]*api.Session, error)
	DeleteSession(id string) error
	ListSessionsByEnvironment(env string) ([]*api.Session, error)
}

func NewStore(backend string) (Store, error) {
	switch backend {
	case "memory":
		return defaultMemoryStore, nil
	case "filesystem":
		basePath, err := defaultFilesystemBasePath()
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(basePath, 0o755); err != nil {
			return nil, err
		}
		return newFilesystemStore(basePath), nil
	default:
		return nil, fmt.Errorf("unsupported sessions backend: %s", backend)
	}
}

func defaultFilesystemBasePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".kubectl-ai", sessionsDirName), nil
}

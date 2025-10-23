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

package journal

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"sigs.k8s.io/yaml"
)

// Recorder is an interface for recording a structured log of the agent's actions and observations.
type Recorder interface {
	io.Closer

	// Write will add an event to the recorder.
	Write(ctx context.Context, event *Event) error
}

// FileRecorder writes a structured log of the agent's actions and observations to a file.
type FileRecorder struct {
	f *os.File
}

// NewFileRecorder creates a new FileRecorder that writes to the given file.
func NewFileRecorder(path string) (*FileRecorder, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	return &FileRecorder{
		f: file,
	}, nil
}

// Close closes the file.
func (r *FileRecorder) Close() error {
	return r.f.Close()
}

func (r *FileRecorder) Write(ctx context.Context, event *Event) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	yamlBytes, err := yaml.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshalling event: %w", err)
	}
	var b bytes.Buffer
	b.Write(yamlBytes)
	b.Write([]byte("\n\n---\n\n"))
	_, err = r.f.Write(b.Bytes())
	return err
}

type Event struct {
	Timestamp time.Time `json:"timestamp"`
	Action    string    `json:"action"`
	Payload   any       `json:"payload,omitempty"`
}

const (
	ActionHTTPRequest  = "http.request"
	ActionHTTPResponse = "http.response"
	ActionHTTPError    = "http.error"
)

// ActionUIRender is for an event that indicates we wrote output to the UI
const ActionUIRender = "ui.render"

// GetString is a helper to get a string value from the Payload
func (e *Event) GetString(key string) (string, bool) {
	if e.Payload == nil {
		return "", false
	}
	m, ok := e.Payload.(map[string]any)
	if !ok {
		return "", false
	}
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	return s, true
}

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

package store

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
)

// DataStore manages the persistence of conversation history.
type DataStore struct {
	dataFile string
	history  []*gollm.RecordMessage
	mu       sync.Mutex
}

// New creates a new DataStore, loading any existing history from the dataFile.
func New(dataFile string) (*DataStore, error) {
	s := &DataStore{
		dataFile: dataFile,
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// load reads the data file and populates the in-memory history.
func (s *DataStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.Open(s.dataFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File not existing is not an error, history is just empty.
		}
		return err
	}
	defer f.Close()

	var history []*gollm.RecordMessage
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var record gollm.RecordMessage
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			// TODO: Be more robust against corrupted lines.
			return err
		}
		history = append(history, &record)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	s.history = history
	return nil
}

// Add appends a new record to the history and persists it to the data file.
func (s *DataStore) Add(record *gollm.RecordMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.history = append(s.history, record)

	f, err := os.OpenFile(s.dataFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	b, err := json.Marshal(record)
	if err != nil {
		return err
	}

	_, err = f.Write(append(b, '\n'))
	return err
}

// History returns a copy of the current conversation history.
func (s *DataStore) History() []*gollm.RecordMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Return a copy to prevent modification of the internal slice.
	historyCopy := make([]*gollm.RecordMessage, len(s.history))
	copy(historyCopy, s.history)
	return historyCopy
}

// Clear removes all records from the history and truncates the data file.
func (s *DataStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.history = nil

	// Truncate the file by opening it with O_TRUNC
	f, err := os.OpenFile(s.dataFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	return f.Close()
}

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

package gollm

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"time"
)

// We define some standard structs to allow for persistence of the LLM requests and responses.
// This lets us store the history of the conversation for later analysis.

// LoadHistoryFromFile loads the chat history from a file.
func LoadHistoryFromFile(filePath string) ([]*RecordMessage, error) {
	f, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Return empty history if file doesn't exist
		}
		return nil, err
	}
	defer f.Close()

	var history []*RecordMessage
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var record RecordMessage
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, err
		}
		history = append(history, &record)
	}
	return history, scanner.Err()
}

// PersistentChat is a wrapper around a Chat that persists the conversation.
type PersistentChat struct {
	chat     Chat
	filePath string
}

// NewPersistentChat creates a new PersistentChat.
func NewPersistentChat(chat Chat, filePath string) (*PersistentChat, error) {
	history, err := LoadHistoryFromFile(filePath)
	if err != nil {
		return nil, err
	}
	if len(history) > 0 {
		if err := chat.LoadHistory(history); err != nil {
			return nil, err
		}
	}

	return &PersistentChat{
		chat:     chat,
		filePath: filePath,
	}, nil
}

func (c *PersistentChat) writeRecord(record *RecordMessage) error {
	f, err := os.OpenFile(c.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	b, err := json.Marshal(record)
	if err != nil {
		return err
	}

	writer := bufio.NewWriter(f)
	_, err = writer.WriteString(string(b) + "\n")
	if err != nil {
		return err
	}
	return writer.Flush()
}

// Send implements Chat.
func (c *PersistentChat) Send(ctx context.Context, contents ...any) (ChatResponse, error) {
	if err := c.writeRecord(&RecordMessage{
		Timestamp: time.Now(),
		Role:      "user",
		Content:   contents,
	}); err != nil {
		return nil, err
	}

	resp, err := c.chat.Send(ctx, contents...)
	if err != nil {
		return nil, err
	}

	if err := c.writeRecord(&RecordMessage{
		Timestamp: time.Now(),
		Role:      "model",
		Response:  resp,
	}); err != nil {
		return nil, err
	}

	return resp, nil
}

// SendStreaming implements Chat.
func (c *PersistentChat) SendStreaming(ctx context.Context, contents ...any) (ChatResponseIterator, error) {
	if err := c.writeRecord(&RecordMessage{
		Timestamp: time.Now(),
		Role:      "user",
		Content:   contents,
	}); err != nil {
		return nil, err
	}

	iter, err := c.chat.SendStreaming(ctx, contents...)
	if err != nil {
		return nil, err
	}

	// Wrap the iterator to persist the responses.
	return func(yield func(ChatResponse, error) bool) {
		iter(func(resp ChatResponse, err error) bool {
			if err != nil {
				return yield(nil, err)
			}
			if err := c.writeRecord(&RecordMessage{
				Timestamp: time.Now(),
				Role:      "model",
				Response:  resp,
			}); err != nil {
				return yield(nil, err)
			}
			return yield(resp, nil)
		})
	}, nil
}

// SetFunctionDefinitions implements Chat.
func (c *PersistentChat) SetFunctionDefinitions(functionDefinitions []*FunctionDefinition) error {
	return c.chat.SetFunctionDefinitions(functionDefinitions)
}

// IsRetryableError implements Chat.
func (c *PersistentChat) IsRetryableError(err error) bool {
	return c.chat.IsRetryableError(err)
}

// LoadHistory implements Chat.
func (c *PersistentChat) LoadHistory(history []*RecordMessage) error {
	return c.chat.LoadHistory(history)
}

var _ Chat = &PersistentChat{}

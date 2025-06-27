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

package agent

import (
	"context"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/sessions"
)

// ChatLogger is a wrapper around a gollm.Chat that logs interactions to a DataStore.
type ChatLogger struct {
	underlying gollm.Chat
	store      sessions.ChatMessageStore
}

var _ gollm.Chat = &ChatLogger{}

// NewChatLogger creates a new ChatLogger.
func NewChatLogger(underlying gollm.Chat, store sessions.ChatMessageStore) *ChatLogger {
	return &ChatLogger{
		underlying: underlying,
		store:      store,
	}
}

func (c *ChatLogger) writeRecord(record *gollm.ChatMessage) {
	if c.store == nil {
		return
	}
	// Errors are logged and ignored, as logging should not block the chat flow.
	if err := c.store.AddChatMessage(record); err != nil {
		// TODO: Log this error.
	}
}

// Send implements gollm.Chat.
func (c *ChatLogger) Send(ctx context.Context, contents ...any) (gollm.ChatResponse, error) {
	c.writeRecord(&gollm.ChatMessage{
		Timestamp: time.Now(),
		Role:      "user",
		Content:   contents,
	})

	resp, err := c.underlying.Send(ctx, contents...)
	if err != nil {
		return nil, err
	}

	c.writeRecord(&gollm.ChatMessage{
		Timestamp: time.Now(),
		Role:      "model",
		Response:  resp,
	})

	return resp, nil
}

// SendStreaming implements gollm.Chat.
func (c *ChatLogger) SendStreaming(ctx context.Context, contents ...any) (gollm.ChatResponseIterator, error) {
	c.writeRecord(&gollm.ChatMessage{
		Timestamp: time.Now(),
		Role:      "user",
		Content:   contents,
	})

	iter, err := c.underlying.SendStreaming(ctx, contents...)
	if err != nil {
		return nil, err
	}

	// Wrap the iterator to persist the responses.
	return func(yield func(gollm.ChatResponse, error) bool) {
		iter(func(resp gollm.ChatResponse, err error) bool {
			if err != nil {
				// We don't log the error response.
				return yield(nil, err)
			}
			c.writeRecord(&gollm.ChatMessage{
				Timestamp: time.Now(),
				Role:      "model",
				Response:  resp,
			})
			return yield(resp, nil)
		})
	}, nil
}

// SetFunctionDefinitions implements gollm.Chat.
func (c *ChatLogger) SetFunctionDefinitions(functionDefinitions []*gollm.FunctionDefinition) error {
	return c.underlying.SetFunctionDefinitions(functionDefinitions)
}

// IsRetryableError implements gollm.Chat.
func (c *ChatLogger) IsRetryableError(err error) bool {
	return c.underlying.IsRetryableError(err)
}

// Initialize implements gollm.Chat.
func (c *ChatLogger) Initialize(messages []*gollm.ChatMessage) error {
	return c.underlying.Initialize(messages)
}

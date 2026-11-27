// Copyright 2026 Google LLC
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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
)

// TestLlamaCppProviderRegistration verifies that "llamacpp" is registered in the global registry.
func TestLlamaCppProviderRegistration(t *testing.T) {
	providers := globalRegistry.listProviders()
	found := false
	for _, p := range providers {
		if p == "llamacpp" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'llamacpp' to be registered, got providers: %v", providers)
	}
}

// TestLlamaCppInitialize verifies that Initialize correctly converts api.Message history
// into llamacppChatMessage history while preserving the initial system prompt.
func TestLlamaCppInitialize(t *testing.T) {
	t.Run("converts user and model messages and preserves system prompt", func(t *testing.T) {
		chat := &LlamaCppChat{
			history: []llamacppChatMessage{
				{
					Role:    "system",
					Content: ptrTo("You are a helpful assistant"),
				},
			},
		}

		messages := []*api.Message{
			{
				Source:    api.MessageSourceUser,
				Type:      api.MessageTypeText,
				Payload:   "hello llama",
				Timestamp: time.Now(),
			},
			{
				Source:    api.MessageSourceModel,
				Type:      api.MessageTypeText,
				Payload:   "hello user",
				Timestamp: time.Now(),
			},
			{
				Source:    api.MessageSourceAgent,
				Type:      api.MessageTypeText,
				Payload:   "agent text response",
				Timestamp: time.Now(),
			},
		}

		if err := chat.Initialize(messages); err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		if len(chat.history) != 4 {
			t.Fatalf("expected 4 messages in history, got %d", len(chat.history))
		}

		// Verify system prompt
		if chat.history[0].Role != "system" || *chat.history[0].Content != "You are a helpful assistant" {
			t.Errorf("unexpected message at index 0: %+v", chat.history[0])
		}

		// Verify user message
		if chat.history[1].Role != "user" || *chat.history[1].Content != "hello llama" {
			t.Errorf("unexpected message at index 1: %+v", chat.history[1])
		}

		// Verify model message
		if chat.history[2].Role != "assistant" || *chat.history[2].Content != "hello user" {
			t.Errorf("unexpected message at index 2: %+v", chat.history[2])
		}

		// Verify agent message
		if chat.history[3].Role != "assistant" || *chat.history[3].Content != "agent text response" {
			t.Errorf("unexpected message at index 3: %+v", chat.history[3])
		}
	})

	t.Run("empty messages clears history while preserving system prompt", func(t *testing.T) {
		chat := &LlamaCppChat{
			history: []llamacppChatMessage{
				{
					Role:    "system",
					Content: ptrTo("system prompt"),
				},
				{
					Role:    "user",
					Content: ptrTo("previous user message"),
				},
				{
					Role:    "assistant",
					Content: ptrTo("previous assistant message"),
				},
			},
		}

		if err := chat.Initialize([]*api.Message{}); err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		if len(chat.history) != 1 {
			t.Fatalf("expected 1 message in history (system prompt only), got %d", len(chat.history))
		}
		if chat.history[0].Role != "system" || *chat.history[0].Content != "system prompt" {
			t.Errorf("unexpected system prompt: %+v", chat.history[0])
		}
	})

	t.Run("skips non-text and empty messages", func(t *testing.T) {
		chat := &LlamaCppChat{
			history: []llamacppChatMessage{
				{
					Role:    "system",
					Content: ptrTo("system prompt"),
				},
			},
		}

		messages := []*api.Message{
			{
				Source:  api.MessageSourceUser,
				Type:    api.MessageTypeToolCallRequest,
				Payload: "tool request",
			},
			{
				Source:  api.MessageSourceAgent,
				Type:    api.MessageTypeToolCallResponse,
				Payload: "tool response",
			},
			{
				Source:  api.MessageSourceUser,
				Type:    api.MessageTypeText,
				Payload: "",
			},
			{
				Source:  api.MessageSourceUser,
				Type:    api.MessageTypeText,
				Payload: nil,
			},
			{
				Source:  api.MessageSource("unknown"),
				Type:    api.MessageTypeText,
				Payload: "unknown source",
			},
			{
				Source:  api.MessageSourceUser,
				Type:    api.MessageTypeText,
				Payload: "valid user message",
			},
		}

		if err := chat.Initialize(messages); err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		if len(chat.history) != 2 {
			t.Fatalf("expected 2 messages in history, got %d", len(chat.history))
		}
		if chat.history[1].Role != "user" || *chat.history[1].Content != "valid user message" {
			t.Errorf("unexpected message at index 1: %+v", chat.history[1])
		}
	})

	t.Run("converts non-string payload to string", func(t *testing.T) {
		chat := &LlamaCppChat{}

		messages := []*api.Message{
			{
				Source:  api.MessageSourceUser,
				Type:    api.MessageTypeText,
				Payload: 12345,
			},
		}

		if err := chat.Initialize(messages); err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		if len(chat.history) != 1 {
			t.Fatalf("expected 1 message in history, got %d", len(chat.history))
		}
		if *chat.history[0].Content != "12345" {
			t.Errorf("expected content '12345', got %q", *chat.history[0].Content)
		}
	})
}

// TestLlamaCppSend_WithInitializedHistory tests that initialized history is included in LLM requests.
func TestLlamaCppSend_WithInitializedHistory(t *testing.T) {
	var capturedReq llamacppChatRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}

		if err := json.NewDecoder(r.Body).Decode(&capturedReq); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		resp := llamacppChatResponse{
			Id:    "test-id",
			Model: "test-model",
			Choices: []llamacppChoice{
				{
					Index: 0,
					Message: &llamacppChatMessage{
						Role:    "assistant",
						Content: ptrTo("pong"),
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("failed to parse test server url: %v", err)
	}

	client := &LlamaCppClient{
		baseURL:    u,
		httpClient: server.Client(),
	}

	chat := client.StartChat("system prompt", "test-model").(*LlamaCppChat)

	// Initialize with past conversation
	err = chat.Initialize([]*api.Message{
		{
			Source:  api.MessageSourceUser,
			Type:    api.MessageTypeText,
			Payload: "past question",
		},
		{
			Source:  api.MessageSourceModel,
			Type:    api.MessageTypeText,
			Payload: "past answer",
		},
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Send new message
	res, err := chat.Send(context.Background(), "ping")
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if len(res.Candidates()) == 0 {
		t.Fatalf("expected at least one candidate in response")
	}
	text, ok := res.Candidates()[0].Parts()[0].AsText()
	if !ok || text != "pong" {
		t.Errorf("expected response 'pong', got %q", text)
	}

	// Verify messages sent to server
	if len(capturedReq.Messages) != 4 {
		t.Fatalf("expected 4 messages sent to server, got %d", len(capturedReq.Messages))
	}

	if capturedReq.Messages[0].Role != "system" || *capturedReq.Messages[0].Content != "system prompt" {
		t.Errorf("unexpected message 0: %+v", capturedReq.Messages[0])
	}
	if capturedReq.Messages[1].Role != "user" || *capturedReq.Messages[1].Content != "past question" {
		t.Errorf("unexpected message 1: %+v", capturedReq.Messages[1])
	}
	if capturedReq.Messages[2].Role != "assistant" || *capturedReq.Messages[2].Content != "past answer" {
		t.Errorf("unexpected message 2: %+v", capturedReq.Messages[2])
	}
	if capturedReq.Messages[3].Role != "user" || *capturedReq.Messages[3].Content != "ping" {
		t.Errorf("unexpected message 3: %+v", capturedReq.Messages[3])
	}

	// Verify that chat history now contains the assistant's new response as well
	if len(chat.history) != 5 {
		t.Fatalf("expected 5 messages in chat history after Send, got %d", len(chat.history))
	}
	if chat.history[4].Role != "assistant" || *chat.history[4].Content != "pong" {
		t.Errorf("unexpected message 4 in history: %+v", chat.history[4])
	}
}

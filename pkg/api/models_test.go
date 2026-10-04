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

package api

import (
	"testing"
)

type dummyChatStore struct {
	messages []*Message
}

func (d *dummyChatStore) AddChatMessage(record *Message) error {
	d.messages = append(d.messages, record)
	return nil
}

func (d *dummyChatStore) SetChatMessages(newHistory []*Message) error {
	d.messages = newHistory
	return nil
}

func (d *dummyChatStore) ChatMessages() []*Message {
	return d.messages
}

func (d *dummyChatStore) ClearChatMessages() error {
	d.messages = nil
	return nil
}

func TestMessage_IsMeta(t *testing.T) {
	tests := []struct {
		name     string
		msg      *Message
		wantMeta bool
	}{
		{
			name:     "nil message",
			msg:      nil,
			wantMeta: false,
		},
		{
			name: "nil metadata",
			msg: &Message{
				Payload: "hello",
			},
			wantMeta: false,
		},
		{
			name: "empty metadata",
			msg: &Message{
				Payload:  "hello",
				Metadata: map[string]string{},
			},
			wantMeta: false,
		},
		{
			name: "unrelated metadata",
			msg: &Message{
				Payload:  "hello",
				Metadata: map[string]string{"key": "value"},
			},
			wantMeta: false,
		},
		{
			name: "type meta",
			msg: &Message{
				Payload:  "models",
				Metadata: map[string]string{MessageMetadataKeyType: MessageMetadataValueMeta},
			},
			wantMeta: true,
		},
		{
			name: "meta_query true",
			msg: &Message{
				Payload:  "tools",
				Metadata: map[string]string{MessageMetadataKeyMetaQuery: "true"},
			},
			wantMeta: true,
		},
		{
			name: "is_meta true",
			msg: &Message{
				Payload:  "session",
				Metadata: map[string]string{"is_meta": "true"},
			},
			wantMeta: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.msg.IsMeta(); got != tt.wantMeta {
				t.Errorf("IsMeta() = %v, want %v", got, tt.wantMeta)
			}
		})
	}
}

func TestMessage_SetMeta(t *testing.T) {
	msg := &Message{Payload: "test"}
	if msg.IsMeta() {
		t.Fatalf("expected initial message not to be meta")
	}

	msg.SetMeta(true)
	if !msg.IsMeta() {
		t.Fatalf("expected message to be meta after SetMeta(true)")
	}
	if msg.Metadata[MessageMetadataKeyType] != MessageMetadataValueMeta {
		t.Errorf("expected Metadata[%q] == %q, got %q", MessageMetadataKeyType, MessageMetadataValueMeta, msg.Metadata[MessageMetadataKeyType])
	}
	if msg.Metadata[MessageMetadataKeyMetaQuery] != "true" {
		t.Errorf("expected Metadata[%q] == %q, got %q", MessageMetadataKeyMetaQuery, "true", msg.Metadata[MessageMetadataKeyMetaQuery])
	}

	msg.SetMeta(false)
	if msg.IsMeta() {
		t.Fatalf("expected message not to be meta after SetMeta(false)")
	}

	// nil message safety
	var nilMsg *Message
	nilMsg.SetMeta(true) // should not panic
}

func TestFilterNonMetaMessages(t *testing.T) {
	userMsg := &Message{Source: MessageSourceUser, Payload: "deploy pod"}
	metaReq := &Message{
		Source:   MessageSourceUser,
		Payload:  "models",
		Metadata: map[string]string{MessageMetadataKeyType: MessageMetadataValueMeta},
	}
	metaResp := &Message{
		Source:   MessageSourceAgent,
		Payload:  "Available models: ...",
		Metadata: map[string]string{MessageMetadataKeyType: MessageMetadataValueMeta},
	}
	agentMsg := &Message{Source: MessageSourceAgent, Payload: "pod deployed"}

	all := []*Message{userMsg, metaReq, metaResp, agentMsg}
	filtered := FilterNonMetaMessages(all)

	if len(filtered) != 2 {
		t.Fatalf("expected 2 filtered messages, got %d", len(filtered))
	}
	if filtered[0] != userMsg {
		t.Errorf("expected first message to be userMsg, got %v", filtered[0])
	}
	if filtered[1] != agentMsg {
		t.Errorf("expected second message to be agentMsg, got %v", filtered[1])
	}

	// Nil / empty checks
	if got := FilterNonMetaMessages(nil); len(got) != 0 {
		t.Errorf("expected empty result for nil slice, got %v", got)
	}
	if got := FilterNonMetaMessages([]*Message{metaReq, metaResp}); len(got) != 0 {
		t.Errorf("expected empty result when all messages are meta, got %v", got)
	}
}

func TestSession_LLMMessages(t *testing.T) {
	var nilSession *Session
	if got := nilSession.LLMMessages(); got != nil {
		t.Errorf("expected nil for nilSession.LLMMessages(), got %v", got)
	}

	sess := &Session{}
	if got := sess.LLMMessages(); got != nil {
		t.Errorf("expected nil for session with nil store, got %v", got)
	}

	store := &dummyChatStore{
		messages: []*Message{
			{Source: MessageSourceUser, Payload: "help with ingress"},
			{Source: MessageSourceUser, Payload: "tools", Metadata: map[string]string{MessageMetadataKeyType: MessageMetadataValueMeta}},
			{Source: MessageSourceAgent, Payload: "tools list", Metadata: map[string]string{MessageMetadataKeyType: MessageMetadataValueMeta}},
			{Source: MessageSourceModel, Payload: "here is an ingress"},
		},
	}
	sess.ChatMessageStore = store

	all := sess.AllMessages()
	if len(all) != 4 {
		t.Fatalf("expected AllMessages() to return 4 messages, got %d", len(all))
	}

	llmMsgs := sess.LLMMessages()
	if len(llmMsgs) != 2 {
		t.Fatalf("expected LLMMessages() to return 2 messages, got %d", len(llmMsgs))
	}
	if llmMsgs[0].Payload != "help with ingress" || llmMsgs[1].Payload != "here is an ingress" {
		t.Errorf("unexpected LLMMessages: %v", llmMsgs)
	}
}

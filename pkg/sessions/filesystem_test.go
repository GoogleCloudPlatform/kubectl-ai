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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
)

func TestFileChatMessageStore_MetadataPersistence(t *testing.T) {
	tempDir := t.TempDir()
	store := NewFileChatMessageStore(tempDir)

	msg1 := &api.Message{
		ID:        "msg-1",
		Source:    api.MessageSourceUser,
		Type:      api.MessageTypeText,
		Payload:   "regular user message",
		Timestamp: time.Now(),
	}

	msg2 := &api.Message{
		ID:        "msg-2",
		Source:    api.MessageSourceUser,
		Type:      api.MessageTypeText,
		Payload:   "models",
		Timestamp: time.Now(),
		Metadata: map[string]string{
			api.MessageMetadataKeyType:      api.MessageMetadataValueMeta,
			api.MessageMetadataKeyMetaQuery: "true",
		},
	}

	msg3 := &api.Message{
		ID:        "msg-3",
		Source:    api.MessageSourceAgent,
		Type:      api.MessageTypeText,
		Payload:   "Available models: gpt-4, gemini",
		Timestamp: time.Now(),
		Metadata: map[string]string{
			api.MessageMetadataKeyType: api.MessageMetadataValueMeta,
		},
	}

	// Add messages one by one (JSONL append)
	if err := store.AddChatMessage(msg1); err != nil {
		t.Fatalf("AddChatMessage(msg1) failed: %v", err)
	}
	if err := store.AddChatMessage(msg2); err != nil {
		t.Fatalf("AddChatMessage(msg2) failed: %v", err)
	}
	if err := store.AddChatMessage(msg3); err != nil {
		t.Fatalf("AddChatMessage(msg3) failed: %v", err)
	}

	// Read messages back from disk
	readMsgs := store.ChatMessages()
	if len(readMsgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(readMsgs))
	}

	if readMsgs[0].IsMeta() {
		t.Errorf("msg1 should not be meta")
	}
	if !readMsgs[1].IsMeta() {
		t.Errorf("msg2 should be meta")
	}
	if !readMsgs[2].IsMeta() {
		t.Errorf("msg3 should be meta")
	}

	if readMsgs[1].Metadata[api.MessageMetadataKeyType] != api.MessageMetadataValueMeta {
		t.Errorf("expected msg2 metadata type %q, got %q", api.MessageMetadataValueMeta, readMsgs[1].Metadata[api.MessageMetadataKeyType])
	}

	// Test SetChatMessages overwrite
	if err := store.SetChatMessages([]*api.Message{msg2, msg3}); err != nil {
		t.Fatalf("SetChatMessages failed: %v", err)
	}

	readAgain := store.ChatMessages()
	if len(readAgain) != 2 {
		t.Fatalf("expected 2 messages after overwrite, got %d", len(readAgain))
	}
	if !readAgain[0].IsMeta() || !readAgain[1].IsMeta() {
		t.Errorf("both messages should be meta after overwrite")
	}

	// Verify FilterNonMetaMessages works with persisted messages
	filtered := api.FilterNonMetaMessages(readAgain)
	if len(filtered) != 0 {
		t.Errorf("expected 0 filtered messages, got %d", len(filtered))
	}
}

func TestFileChatMessageStore_LegacyFormatWithMetadata(t *testing.T) {
	tempDir := t.TempDir()
	historyPath := filepath.Join(tempDir, "history.json")

	// Write legacy JSON array format
	legacyData := `[
		{"ID":"1","Source":"user","Type":"text","Payload":"hello"},
		{"ID":"2","Source":"user","Type":"text","Payload":"tools","Metadata":{"type":"meta"}}
	]`
	if err := os.WriteFile(historyPath, []byte(legacyData), 0o644); err != nil {
		t.Fatalf("failed to write legacy history: %v", err)
	}

	store := NewFileChatMessageStore(tempDir)
	msgs := store.ChatMessages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	if msgs[0].IsMeta() {
		t.Errorf("expected message 1 not to be meta")
	}
	if !msgs[1].IsMeta() {
		t.Errorf("expected message 2 to be meta")
	}
}

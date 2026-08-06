// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gollm

import (
	"reflect"
	"testing"

	kctlApi "github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
	"github.com/ollama/ollama/api"
)

func TestOllamaInitializeRestoresSupportedMessages(t *testing.T) {
	chat := &OllamaChat{
		history: []api.Message{
			{Role: "system", Content: "system prompt"},
			{Role: "user", Content: "stale message"},
		},
	}

	messages := []*kctlApi.Message{
		{Source: kctlApi.MessageSourceUser, Type: kctlApi.MessageTypeText, Payload: "list pods"},
		{Source: kctlApi.MessageSourceModel, Type: kctlApi.MessageTypeText, Payload: "Here are the pods."},
		{Source: kctlApi.MessageSourceAgent, Type: kctlApi.MessageTypeText, Payload: "Tool execution finished."},
		{Source: kctlApi.MessageSourceUser, Type: kctlApi.MessageTypeText, Payload: 42},
		{Source: kctlApi.MessageSourceUser, Type: kctlApi.MessageTypeError, Payload: "ignored error"},
		{Source: kctlApi.MessageSource("unknown"), Type: kctlApi.MessageTypeText, Payload: "ignored source"},
		{Source: kctlApi.MessageSourceUser, Type: kctlApi.MessageTypeText, Payload: ""},
		{Source: kctlApi.MessageSourceUser, Type: kctlApi.MessageTypeText},
		nil,
	}

	if err := chat.Initialize(messages); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	want := []api.Message{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "list pods"},
		{Role: "assistant", Content: "Here are the pods."},
		{Role: "assistant", Content: "Tool execution finished."},
		{Role: "user", Content: "42"},
	}
	if !reflect.DeepEqual(chat.history, want) {
		t.Fatalf("Initialize() history = %#v, want %#v", chat.history, want)
	}
}

func TestOllamaInitializeReplacesConversationAndPreservesSystemPrompt(t *testing.T) {
	chat := &OllamaChat{
		history: []api.Message{
			{Role: "system", Content: "system prompt"},
			{Role: "user", Content: "old user message"},
			{Role: "assistant", Content: "old assistant message"},
		},
	}

	first := []*kctlApi.Message{
		{Source: kctlApi.MessageSourceUser, Type: kctlApi.MessageTypeText, Payload: "first restored message"},
	}
	if err := chat.Initialize(first); err != nil {
		t.Fatalf("first Initialize() error = %v", err)
	}

	second := []*kctlApi.Message{
		{Source: kctlApi.MessageSourceUser, Type: kctlApi.MessageTypeText, Payload: "replacement message"},
	}
	if err := chat.Initialize(second); err != nil {
		t.Fatalf("second Initialize() error = %v", err)
	}

	want := []api.Message{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "replacement message"},
	}
	if !reflect.DeepEqual(chat.history, want) {
		t.Fatalf("Initialize() history = %#v, want %#v", chat.history, want)
	}

	if err := chat.Initialize(nil); err != nil {
		t.Fatalf("Initialize(nil) error = %v", err)
	}
	want = []api.Message{{Role: "system", Content: "system prompt"}}
	if !reflect.DeepEqual(chat.history, want) {
		t.Fatalf("Initialize(nil) history = %#v, want %#v", chat.history, want)
	}
}

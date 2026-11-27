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
	"os"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	"github.com/GoogleCloudPlatform/kubectl-ai/internal/mocks"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/sessions"
	"go.uber.org/mock/gomock"
)

func TestHandleMetaQuery(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name         string
		query        string
		expectations func(t *testing.T) *Agent
		verify       func(t *testing.T, a *Agent, answer string)
		expect       string
	}{
		{
			name:   "clear (shows store before/after with mocked model + tool outputs)",
			query:  "clear",
			expect: "Cleared the conversation.",
			expectations: func(t *testing.T) *Agent {
				ctrl := gomock.NewController(t)
				t.Cleanup(ctrl.Finish)

				store := sessions.NewInMemoryChatStore()

				chat := mocks.NewMockChat(ctrl)
				chat.EXPECT().Initialize([]*api.Message{}).Times(1)

				mt := mocks.NewMockTool(ctrl)
				mt.EXPECT().Name().Return("mock namespace tool").AnyTimes()
				mt.EXPECT().FunctionDefinition().Return(&gollm.FunctionDefinition{
					Name:        "mock namespace tool",
					Description: "Inspect current Kubernetes namespace",
				}).AnyTimes()

				const toolResult = `{"namespace":"test-namespace"}`

				mt.EXPECT().Run(gomock.Any(), gomock.Any()).
					Return(toolResult, nil).Times(1)

				const modelText = "The current namespace is test-namespace."

				// user message
				_ = store.AddChatMessage(&api.Message{
					ID:      "u1",
					Source:  api.MessageSourceUser,
					Type:    api.MessageTypeText,
					Payload: "What's my current namespace?",
				})

				// model response
				_ = store.AddChatMessage(&api.Message{
					ID:      "a1",
					Source:  api.MessageSourceAgent,
					Type:    api.MessageTypeText,
					Payload: modelText,
				})

				// tool call result
				if out, err := mt.Run(ctx, map[string]any{}); err == nil {
					_ = store.AddChatMessage(&api.Message{
						ID:      "t1",
						Source:  api.MessageSourceAgent,
						Type:    api.MessageTypeText,
						Payload: out,
					})
				} else {
					t.Fatalf("mock tool run failed: %v", err)
				}

				if got := len(store.ChatMessages()); got != 3 {
					t.Fatalf("precondition: expected 3 messages before clear, got %d", got)
				}

				a := &Agent{llmChat: chat}
				a.Session = &api.Session{ChatMessageStore: store}

				return a
			},
			verify: func(t *testing.T, a *Agent, _ string) {
				if got := len(a.Session.ChatMessageStore.ChatMessages()); got != 0 {
					t.Fatalf("expected store to be empty after clear, got %d", got)
				}
			},
		},
		{
			name:   "exit",
			query:  "exit",
			expect: "It has been a pleasure assisting you. Have a great day!",
			expectations: func(t *testing.T) *Agent {
				a := &Agent{}
				a.Session = &api.Session{}
				return a
			},
			verify: func(t *testing.T, a *Agent, _ string) {
				if a.AgentState() != api.AgentStateExited {
					t.Fatalf("expected agent to exit")
				}
			},
		},
		{
			name:   "model",
			query:  "model",
			expect: "Current model is `test-model`",
			expectations: func(t *testing.T) *Agent {
				a := &Agent{Model: "test-model"}
				a.Session = &api.Session{}
				return a
			},
		},
		{
			name:   "models",
			query:  "models",
			expect: "Available models:\n\n  - a\n  - b\n\n",
			expectations: func(t *testing.T) *Agent {
				ctrl := gomock.NewController(t)
				t.Cleanup(ctrl.Finish)
				llm := mocks.NewMockClient(ctrl)
				llm.EXPECT().ListModels(ctx).Return([]string{"a", "b"}, nil)

				a := &Agent{LLM: llm}
				a.Session = &api.Session{}
				return a
			},
		},
		{
			name:   "tools",
			query:  "tools",
			expect: "Available tools:",
			expectations: func(t *testing.T) *Agent {
				ctrl := gomock.NewController(t)
				t.Cleanup(ctrl.Finish)

				mt := mocks.NewMockTool(ctrl)
				mt.EXPECT().Name().Return("mocktool").AnyTimes()
				mt.EXPECT().FunctionDefinition().Return(&gollm.FunctionDefinition{
					Name:        "mocktool",
					Description: "Mocked tool for tests",
				}).AnyTimes()

				a := &Agent{}

				a.Tools.Init()
				a.Tools.RegisterTool(mt)
				a.Session = &api.Session{}
				return a
			},
			verify: func(t *testing.T, _ *Agent, answer string) {
				if !strings.Contains(answer, "mocktool") {
					t.Fatalf("expected kubectl tool in output: %q", answer)
				}
			},
		},
		{
			name:   "session",
			query:  "session",
			expect: "Session ID:",
			expectations: func(t *testing.T) *Agent {
				oldHome := os.Getenv("HOME")
				t.Cleanup(func() { os.Setenv("HOME", oldHome) })
				home := t.TempDir()
				os.Setenv("HOME", home)

				manager, err := sessions.NewSessionManager("memory")
				if err != nil {
					t.Fatalf("creating session manager: %v", err)
				}
				sess, err := manager.NewSession(sessions.Metadata{ProviderID: "p", ModelID: "m"})
				if err != nil {
					t.Fatalf("creating session: %v", err)
				}
				a := &Agent{ChatMessageStore: sess.ChatMessageStore, SessionBackend: "filesystem"}
				a.Session = sess
				return a
			},
			verify: func(t *testing.T, _ *Agent, answer string) {
				if !strings.Contains(answer, "ID:") {
					t.Fatalf("expected session info, got %q", answer)
				}
			},
		},
		{
			name:   "sessions",
			query:  "sessions",
			expect: "Available sessions:",
			expectations: func(t *testing.T) *Agent {
				oldHome := os.Getenv("HOME")
				t.Cleanup(func() { os.Setenv("HOME", oldHome) })
				home := t.TempDir()
				os.Setenv("HOME", home)

				manager, err := sessions.NewSessionManager("memory")
				if err != nil {
					t.Fatalf("creating session manager: %v", err)
				}
				if _, err := manager.NewSession(sessions.Metadata{ProviderID: "p1", ModelID: "m1"}); err != nil {
					t.Fatalf("creating session: %v", err)
				}

				a := &Agent{SessionBackend: "memory"}
				a.Session = &api.Session{ChatMessageStore: sessions.NewInMemoryChatStore()}
				return a
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := tt.expectations(t)
			ans, handled, err := a.handleMetaQuery(ctx, tt.query)
			if err != nil {
				t.Fatalf("handleMetaQuery returned error: %v", err)
			}
			if !handled {
				t.Fatalf("expected query %q to be handled", tt.query)
			}
			if tt.expect != "" && !strings.Contains(ans, tt.expect) {
				t.Fatalf("expected %q to contain %q", ans, tt.expect)
			}
			if tt.verify != nil {
				tt.verify(t, a, ans)
			}
		})
	}
}

func TestAgent_NewSession(t *testing.T) {
	// Setup
	manager, err := sessions.NewSessionManager("memory")
	if err != nil {
		t.Fatalf("creating session manager: %v", err)
	}

	// Create initial session
	sess1, err := manager.NewSession(sessions.Metadata{})
	if err != nil {
		t.Fatalf("creating session 1: %v", err)
	}

	a := &Agent{
		SessionBackend: "memory",
	}
	a.Session = sess1

	// Call NewSession
	newID, err := a.NewSession()
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	if newID == sess1.ID {
		t.Fatalf("expected new session ID to be different from old one")
	}

	if a.Session.ID != newID {
		t.Fatalf("agent session ID mismatch: got %s, want %s", a.Session.ID, newID)
	}
}

func TestAgent_LoadSession_ResetsState(t *testing.T) {
	// Setup
	manager, err := sessions.NewSessionManager("memory")
	if err != nil {
		t.Fatalf("creating session manager: %v", err)
	}

	// Create a session in "running" state
	sess1, err := manager.NewSession(sessions.Metadata{})
	if err != nil {
		t.Fatalf("creating session 1: %v", err)
	}
	sess1.AgentState = api.AgentStateRunning
	if err := manager.UpdateLastAccessed(sess1); err != nil {
		t.Fatalf("updating session: %v", err)
	}

	a := &Agent{
		SessionBackend: "memory",
	}

	// Load the session
	if err := a.LoadSession(sess1.ID); err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}

	// Verify state is reset to idle
	if a.Session.AgentState != api.AgentStateIdle {
		t.Errorf("expected agent state to be idle, got %s", a.Session.AgentState)
	}
}

func TestAgent_Init_CreatesSessionInStore(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockClient(ctrl)
	mockChat := mocks.NewMockChat(ctrl)

	// Expect StartChat to be called
	mockClient.EXPECT().StartChat(gomock.Any(), gomock.Any()).Return(mockChat)
	// Expect Initialize to be called
	mockChat.EXPECT().Initialize(gomock.Any()).Return(nil)
	// Expect SetFunctionDefinitions to be called
	mockChat.EXPECT().SetFunctionDefinitions(gomock.Any()).Return(nil)

	// Setup
	session := &api.Session{
		ID:               "test-session",
		AgentState:       api.AgentStateIdle,
		ChatMessageStore: sessions.NewInMemoryChatStore(),
	}

	a := &Agent{
		SessionBackend: "memory",
		// Init requires these
		Input:   make(chan any),
		Output:  make(chan any),
		LLM:     mockClient,
		Session: session,
	}

	if err := a.Init(context.Background()); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if a.Session != session {
		t.Errorf("expected agent to use provided session")
	}
}

func TestAgent_NewSession_NoDeadlock(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockClient(ctrl)
	mockChat := mocks.NewMockChat(ctrl)

	// Expect StartChat to be called for initial session only
	mockClient.EXPECT().StartChat(gomock.Any(), gomock.Any()).Return(mockChat).Times(1)
	// Expect Initialize to be called for initial session AND new session (and maybe more?)
	mockChat.EXPECT().Initialize(gomock.Any()).Return(nil).AnyTimes()
	// Expect SetFunctionDefinitions to be called for initial session only
	mockChat.EXPECT().SetFunctionDefinitions(gomock.Any()).Return(nil).Times(1)

	// Setup
	session := &api.Session{
		ID:               "initial-session",
		AgentState:       api.AgentStateIdle,
		ChatMessageStore: sessions.NewInMemoryChatStore(),
	}

	a := &Agent{
		SessionBackend: "memory",
		Input:          make(chan any),
		Output:         make(chan any),
		LLM:            mockClient,
		Session:        session,
	}

	// Init
	if err := a.Init(context.Background()); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Create new session
	// This should not deadlock
	done := make(chan struct{})
	go func() {
		if _, err := a.NewSession(); err != nil {
			t.Errorf("NewSession failed: %v", err)
		}
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("NewSession timed out (potential deadlock)")
	}
}

func TestParseReActResponse_JSONBlock(t *testing.T) {
	input := "```json\n{\"thought\":\"check pods\",\"action\":{\"name\":\"kubectl\",\"reason\":\"list pods\",\"command\":\"kubectl get pods -A\",\"modifies_resource\":\"no\"}}\n```"

	got, err := parseReActResponse(input)
	if err != nil {
		t.Fatalf("parseReActResponse returned error: %v", err)
	}
	if got.Action == nil {
		t.Fatalf("expected action to be parsed")
	}
	if got.Action.Name != "kubectl" {
		t.Fatalf("expected action name kubectl, got %q", got.Action.Name)
	}
}

func TestParseReActResponse_PlainTextFallback(t *testing.T) {
	input := "The command was executed successfully. Pods are healthy."

	got, err := parseReActResponse(input)
	if err != nil {
		t.Fatalf("parseReActResponse returned error: %v", err)
	}
	if got.Answer != input {
		t.Fatalf("expected answer %q, got %q", input, got.Answer)
	}
	if got.Action != nil {
		t.Fatalf("expected nil action for plain text fallback")
	}
}

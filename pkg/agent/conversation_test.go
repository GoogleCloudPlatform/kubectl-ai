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
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/tools"
)

func TestRunHandlesMetaQueries(t *testing.T) {
	testCases := []struct {
		name                    string
		query                   string
		expectedResponseContain string
		checkSideEffect         func(t *testing.T, a *Agent)
	}{
		{
			name:                    "model",
			query:                   "model",
			expectedResponseContain: "Current model is `test-model`",
		},
		{
			name:                    "models",
			query:                   "models",
			expectedResponseContain: "mock-model",
		},
		{
			name:                    "tools",
			query:                   "tools",
			expectedResponseContain: "kubectl", // Check for a default tool
		},
		{
			name:                    "clear",
			query:                   "clear",
			expectedResponseContain: "Cleared the conversation.",
			checkSideEffect: func(t *testing.T, a *Agent) {
				time.Sleep(100 * time.Millisecond)
				if len(a.session.ChatMessageStore.ChatMessages()) != 1 {
					t.Errorf("expected 1 chat message after clear, but got %d messages", len(a.session.ChatMessageStore.ChatMessages()))
				}
			},
		},
		{
			name:                    "reset",
			query:                   "reset",
			expectedResponseContain: "Cleared the conversation.",
			checkSideEffect: func(t *testing.T, a *Agent) {
				time.Sleep(100 * time.Millisecond)
				if len(a.session.ChatMessageStore.ChatMessages()) != 1 {
					t.Errorf("expected 1 chat message after reset, but got %d messages", len(a.session.ChatMessageStore.ChatMessages()))
				}
			},
		},
		{
			name:  "exit",
			query: "exit",
			checkSideEffect: func(t *testing.T, a *Agent) {
				if a.AgentState() != api.AgentStateExited {
					t.Errorf("expected agent state to be Exited, but got %s", a.AgentState())
				}
			},
			expectedResponseContain: "It has been a pleasure assisting you",
		},
		{
			name:  "quit",
			query: "quit",
			checkSideEffect: func(t *testing.T, a *Agent) {
				if a.AgentState() != api.AgentStateExited {
					t.Errorf("expected agent state to be Exited, but got %s", a.AgentState())
				}
			},
			expectedResponseContain: "It has been a pleasure assisting you",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			defaultTools := tools.Default()
			a := newTestAgent(&mockChat{}, (&defaultTools).AllTools()...)

			if err := a.Run(ctx, ""); err != nil {
				t.Fatalf("Run returned error: %v", err)
			}

			// Wait for the agent to prompt for input
			timeout := time.After(time.Second)
		WaitForPrompt:
			for {
				select {
				case <-timeout:
					t.Fatalf("did not receive prompt for %q", tc.query)
				case msg := <-a.Output:
					m := msg.(*api.Message)
					if m.Type == api.MessageTypeUserInputRequest && m.Payload.(string) == ">>>" {
						a.Input <- &api.UserInputResponse{Query: tc.query}
						break WaitForPrompt
					}
				}
			}

			responseTimeout := time.After(time.Second)
			var foundResponse bool
		WaitForResponse:
			for {
				select {
				case <-responseTimeout:
					if tc.expectedResponseContain != "" && !foundResponse {
						t.Fatalf("timed out waiting for response for query %q", tc.query)
					}
					break WaitForResponse
				case msg, ok := <-a.Output:
					if !ok { // channel closed
						break WaitForResponse
					}
					m := msg.(*api.Message)
					if m.Source == api.MessageSourceAgent && m.Type == api.MessageTypeText {
						payload, ok := m.Payload.(string)
						if !ok {
							t.Fatalf("unexpected payload type: %T", m.Payload)
						}
						if strings.Contains(payload, tc.expectedResponseContain) {
							foundResponse = true
							break WaitForResponse
						}
					}
				}
			}

			if !foundResponse && tc.expectedResponseContain != "" {
				t.Errorf("did not find expected response for query %q", tc.query)
			}

			if tc.checkSideEffect != nil {
				// Give a moment for state to update for async operations
				if tc.name == "exit" || tc.name == "quit" {
					time.Sleep(50 * time.Millisecond)
				}
				tc.checkSideEffect(t, a)
			}
		})
	}
}

func TestRunHandlesEmptyQuery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defaultTools := tools.Default()
	a := newTestAgent(&mockChat{}, (&defaultTools).AllTools()...)

	if err := a.Run(ctx, ""); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	// Wait for the agent to prompt for input
	timeout := time.After(time.Second)
WaitForPrompt:
	for {
		select {
		case <-timeout:
			t.Fatal("did not receive prompt for input")
		case msg := <-a.Output:
			m := msg.(*api.Message)
			if m.Type == api.MessageTypeUserInputRequest && m.Payload.(string) == ">>>" {
				a.Input <- &api.UserInputResponse{Query: ""} // Send empty query
				break WaitForPrompt
			}
		}
	}

	// Expect another prompt for input
	responseTimeout := time.After(time.Second)
	var gotNextPrompt bool
WaitForNextPrompt:
	for {
		select {
		case <-responseTimeout:
			t.Fatal("timed out waiting for next prompt")
		case msg, ok := <-a.Output:
			if !ok {
				break WaitForNextPrompt
			}
			m := msg.(*api.Message)
			if m.Type == api.MessageTypeUserInputRequest && m.Payload.(string) == ">>>" {
				gotNextPrompt = true
				break WaitForNextPrompt
			}
		}
	}

	if !gotNextPrompt {
		t.Error("did not receive next prompt after empty query")
	}
}

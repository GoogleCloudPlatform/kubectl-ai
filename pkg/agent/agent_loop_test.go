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
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/tools"
)

type mockChatResponse struct {
	gollm.ChatResponse
	candidates []gollm.Candidate
}

func (r *mockChatResponse) Candidates() []gollm.Candidate {
	return r.candidates
}

func (r *mockChatResponse) UsageMetadata() any {
	return nil
}

type mockCandidate struct {
	gollm.Candidate
	parts []gollm.Part
}

func (c *mockCandidate) Parts() []gollm.Part {
	return c.parts
}

func (c *mockCandidate) String() string {
	var result string
	for _, part := range c.parts {
		if text, ok := part.AsText(); ok {
			result += text
		}
	}
	return result
}

type mockTextPart struct {
	gollm.Part
	text string
}

func (p *mockTextPart) AsText() (string, bool) {
	return p.text, true
}

func (p *mockTextPart) AsFunctionCalls() ([]gollm.FunctionCall, bool) {
	return nil, false
}

type mockFunctionCallPart struct {
	gollm.Part
	calls []gollm.FunctionCall
}

func (p *mockFunctionCallPart) AsText() (string, bool) {
	return "", false
}

func (p *mockFunctionCallPart) AsFunctionCalls() ([]gollm.FunctionCall, bool) {
	return p.calls, true
}

// newMockChatResponse creates a gollm.ChatResponseIterator from a list of parts.
func newMockChatResponse(parts ...gollm.Part) gollm.ChatResponseIterator {
	return func(yield func(gollm.ChatResponse, error) bool) {
		response := &mockChatResponse{
			candidates: []gollm.Candidate{
				&mockCandidate{
					parts: parts,
				},
			},
		}
		yield(response, nil)
	}
}

func TestAgentRun_LLMTextResponse(t *testing.T) {
	t.Parallel()
	mockChat := &mockChat{
		sendStreamingFunc: func(ctx context.Context, contents ...any) (gollm.ChatResponseIterator, error) {
			return newMockChatResponse(&mockTextPart{text: "hello from the mock"}), nil
		},
	}
	a := newTestAgent(mockChat)
	a.RunOnce = true

	err := a.Run(context.Background(), "test query")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Expect a text response from the model
	timeout := time.After(1 * time.Second)
	var receivedResponse bool
	for !receivedResponse {
		select {
		case msg := <-a.Output:
			m := msg.(*api.Message)
			if m.Type == api.MessageTypeText && m.Source == api.MessageSourceModel {
				if strings.Contains(m.Payload.(string), "hello from the mock") {
					receivedResponse = true
				}
			}
		case <-timeout:
			t.Fatal("timed out waiting for model response")
		}
	}

	// Agent should be exited after one run
	time.Sleep(100 * time.Millisecond)
	if a.AgentState() != api.AgentStateExited {
		t.Errorf("Expected agent state to be Exited, but got %s", a.AgentState())
	}
}

func TestAgentRun_LLMErrorResponse(t *testing.T) {
	t.Parallel()
	expectedErr := errors.New("mock LLM error")
	mockChat := &mockChat{
		sendStreamingFunc: func(ctx context.Context, contents ...any) (gollm.ChatResponseIterator, error) {
			return nil, expectedErr
		},
	}
	a := newTestAgent(mockChat)
	a.RunOnce = true

	err := a.Run(context.Background(), "test query")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Expect an error message from the agent
	timeout := time.After(2 * time.Second)
	var receivedResponse bool
	for !receivedResponse {
		select {
		case msg := <-a.Output:
			m := msg.(*api.Message)
			if m.Type == api.MessageTypeError && m.Source == api.MessageSourceAgent {
				if strings.Contains(m.Payload.(string), expectedErr.Error()) {
					receivedResponse = true
				}
			}
		case <-timeout:
			t.Fatal("timed out waiting for error message")
		}
	}

	time.Sleep(100 * time.Millisecond)
	if a.AgentState() != api.AgentStateExited {
		t.Errorf("Expected agent state to be Exited, but got %s", a.AgentState())
	}
}

func TestAgentRun_ToolCall_RequiresPermission(t *testing.T) {
	t.Parallel()
	var wg sync.WaitGroup
	wg.Add(1)

	mockChat := &mockChat{
		sendStreamingFunc: func(ctx context.Context, contents ...any) (gollm.ChatResponseIterator, error) {
			toolCall := gollm.FunctionCall{Name: "test-tool", Arguments: map[string]any{"arg1": "val1"}}
			return newMockChatResponse(&mockFunctionCallPart{calls: []gollm.FunctionCall{toolCall}}), nil
		},
	}
	testTool := &mockTool{
		name:                  "test-tool",
		checkModifiesResource: "yes",
		functionDefinition:    &gollm.FunctionDefinition{Name: "test-tool"},
	}
	a := newTestAgent(mockChat, testTool)
	a.RunOnce = false // Needs to be interactive

	err := a.Run(context.Background(), "run the test tool")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Expect a user choice request
	go func() {
		defer wg.Done()
		timeout := time.After(2 * time.Second)
		var receivedResponse bool
		for !receivedResponse {
			select {
			case msg := <-a.Output:
				m := msg.(*api.Message)
				if m.Type == api.MessageTypeUserChoiceRequest {
					receivedResponse = true
					// Simulate user declining
					a.Input <- &api.UserChoiceResponse{Choice: 3} // 3 is "No"
				}
			case <-timeout:
				t.Error("timed out waiting for user choice request")
				return
			}
		}
	}()

	wg.Wait()

	// Agent should be waiting for input after asking for permission
	if a.AgentState() != api.AgentStateWaitingForInput {
		t.Errorf("Expected agent state to be WaitingForInput, but got %s", a.AgentState())
	}
}

func TestAgentRun_ToolCall_SkipPermission(t *testing.T) {
	t.Parallel()
	var wg sync.WaitGroup
	wg.Add(1)

	var once sync.Once
	toolExecuted := false
	mockChat := &mockChat{
		sendStreamingFunc: func(ctx context.Context, contents ...any) (gollm.ChatResponseIterator, error) {
			toolCall := gollm.FunctionCall{Name: "test-tool", Arguments: map[string]any{"arg1": "val1"}}
			return newMockChatResponse(&mockFunctionCallPart{calls: []gollm.FunctionCall{toolCall}}), nil
		},
	}
	testTool := &mockTool{
		name:                  "test-tool",
		checkModifiesResource: "yes",
		functionDefinition:    &gollm.FunctionDefinition{Name: "test-tool"},
		invokeFunc: func(ctx context.Context, options tools.InvokeToolOptions) (any, error) {
			toolExecuted = true
			once.Do(func() {
				wg.Done()
			})
			return "tool executed", nil
		},
	}
	a := newTestAgent(mockChat, testTool)
	a.RunOnce = true
	a.SkipPermissions = true // Key part of the test

	err := a.Run(context.Background(), "run the test tool")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wg.Wait() // Wait for the tool to be invoked

	if !toolExecuted {
		t.Error("Expected tool to be executed, but it was not")
	}
}

func TestEmptyUserInput_DoesNotCallLLM(t *testing.T) {
	t.Parallel()

	var llmCalls int32
	mock := &mockChat{
		sendStreamingFunc: func(ctx context.Context, contents ...any) (gollm.ChatResponseIterator, error) {
			atomic.AddInt32(&llmCalls, 1)
			return newMockChatResponse(&mockTextPart{text: "ok"}), nil
		},
	}

	a := newTestAgent(mock)
	a.RunOnce = false // interactive loop

	if err := a.Run(context.Background(), "seed"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&llmCalls) < 1 {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for first LLM call")
		}
		time.Sleep(20 * time.Millisecond)
	}

	a.Input <- &api.UserInputResponse{Query: ""}

	time.Sleep(150 * time.Millisecond)

	if got := atomic.LoadInt32(&llmCalls); got != 1 {
		t.Fatalf("expected no extra LLM call for empty input; got %d total calls", got)
	}

	if st := a.AgentState(); st != api.AgentStateDone {
		t.Fatalf("expected AgentState to be Done, got %v", st)
	}
}

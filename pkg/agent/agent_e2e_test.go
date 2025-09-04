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
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	"github.com/GoogleCloudPlatform/kubectl-ai/internal/mocks"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/sessions"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/tools"
	"go.uber.org/mock/gomock"
)

func recvMsg(t *testing.T, ctx context.Context, ch <-chan any) *api.Message {
	t.Helper()
	select {
	case v := <-ch:
		m, ok := v.(*api.Message)
		if !ok {
			t.Fatalf("recvMsg: expected *api.Message, got %T", v)
			return nil
		}
		return m
	case <-ctx.Done():
		t.Fatalf("timed out waiting for message: %v", ctx.Err())
		return nil
	}
}

func recvUntil(t *testing.T, ctx context.Context, ch <-chan any, pred func(*api.Message) bool) *api.Message {
	t.Helper()
	for {
		select {
		case v := <-ch:
			m, ok := v.(*api.Message)
			if !ok {
				t.Fatalf("recvUntil: expected *api.Message, got %T", v)
				return nil
			}
			if pred(m) {
				return m
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for matching message: %v", ctx.Err())
		}
	}
}

type fakePart struct {
	text  string
	calls []gollm.FunctionCall
}

func (p fakePart) AsText() (string, bool) {
	if p.text != "" {
		return p.text, true
	}
	return "", false
}

func (p fakePart) AsFunctionCalls() ([]gollm.FunctionCall, bool) {
	if p.calls != nil {
		return p.calls, true
	}
	return nil, false
}

type fakeCandidate struct{ parts []gollm.Part }

func (c fakeCandidate) String() string      { return "" }
func (c fakeCandidate) Parts() []gollm.Part { return c.parts }

type fakeChatResponse struct{ candidate gollm.Candidate }

func (r fakeChatResponse) UsageMetadata() any            { return nil }
func (r fakeChatResponse) Candidates() []gollm.Candidate { return []gollm.Candidate{r.candidate} }

func fCalls(name string, args map[string]any) gollm.Part {
	return fakePart{calls: []gollm.FunctionCall{{ID: "1", Name: name, Arguments: args}}}
}

func fText(s string) gollm.Part { return fakePart{text: s} }

func chatWith(parts ...gollm.Part) gollm.ChatResponse {
	return fakeChatResponse{candidate: fakeCandidate{parts: parts}}
}

func TestAgentEndToEndToolExecution(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	store := sessions.NewInMemoryChatStore()

	client := mocks.NewMockClient(ctrl)
	chat := mocks.NewMockChat(ctrl)

	client.EXPECT().StartChat(gomock.Any(), "test-model").Return(chat)
	chat.EXPECT().Initialize(gomock.Any()).Return(nil)
	chat.EXPECT().SetFunctionDefinitions(gomock.Any()).Return(nil)

	firstResp := chatWith(fCalls("mocktool", map[string]any{"command": "do"}))
	secondResp := chatWith(fText("all done"))

	firstIter := gollm.ChatResponseIterator(func(yield func(gollm.ChatResponse, error) bool) {
		yield(firstResp, nil)
	})
	secondIter := gollm.ChatResponseIterator(func(yield func(gollm.ChatResponse, error) bool) {
		yield(secondResp, nil)
	})

	gomock.InOrder(
		chat.EXPECT().SendStreaming(gomock.Any(), gomock.Any()).Return(firstIter, nil),
		chat.EXPECT().SendStreaming(gomock.Any(), gomock.Any()).Return(secondIter, nil),
	)

	tool := mocks.NewMockTool(ctrl)
	tool.EXPECT().Name().Return("mocktool").AnyTimes()
	tool.EXPECT().Description().Return("mock tool").AnyTimes()
	tool.EXPECT().FunctionDefinition().Return(&gollm.FunctionDefinition{Name: "mocktool"}).AnyTimes()
	tool.EXPECT().IsInteractive(gomock.Any()).Return(false, nil).AnyTimes()
	tool.EXPECT().CheckModifiesResource(gomock.Any()).Return("yes").AnyTimes()
	tool.EXPECT().Run(gomock.Any(), gomock.Any()).Return(map[string]any{"result": "ok"}, nil)

	var toolset tools.Tools
	toolset.Init()
	toolset.RegisterTool(tool)

	a := &Agent{
		ChatMessageStore: store,
		LLM:              client,
		Model:            "test-model",
		Tools:            toolset,
		MaxIterations:    4,
	}

	if err := a.Init(ctx); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := a.Run(ctx, ""); err != nil {
		t.Fatalf("run: %v", err)
	}

	// Expect greeting and prompt inline (UI-driven startup)
	m1 := recvMsg(t, ctx, a.Output)
	if m1.Type != api.MessageTypeText || m1.Source != api.MessageSourceAgent {
		t.Fatalf("expected greeting text from agent, got type=%v source=%v", m1.Type, m1.Source)
	}
	m2 := recvMsg(t, ctx, a.Output)
	if m2.Type != api.MessageTypeUserInputRequest {
		t.Fatalf("expected user-input-request, got %v", m2.Type)
	}

	// Send a query (UI -> Agent)
	a.Input <- &api.UserInputResponse{Query: "test"}

	// Wait for choice request indicating state waiting for input.
	choiceMsg := recvUntil(t, ctx, a.Output, func(m *api.Message) bool {
		return m.Type == api.MessageTypeUserChoiceRequest
	})
	if choiceMsg == nil {
		t.Fatalf("did not receive choice request")
	}
	if st := a.AgentState(); st != api.AgentStateWaitingForInput {
		t.Fatalf("expected waiting-for-input state, got %s", st)
	}

	// Approve tool execution (UI -> Agent)
	a.Input <- &api.UserChoiceResponse{Choice: 1}

	// Expect tool invocation messages and final response.
	sawToolReq, sawToolResp, sawFinal := false, false, false
	for !(sawToolReq && sawToolResp && sawFinal) {
		select {
		case v := <-a.Output:
			m, ok := v.(*api.Message)
			if !ok {
				t.Fatalf("expected *api.Message on output, got %T", v)
				break
			}
			switch m.Type {
			case api.MessageTypeToolCallRequest:
				sawToolReq = true
			case api.MessageTypeToolCallResponse:
				sawToolResp = true
			case api.MessageTypeText:
				if m.Source == api.MessageSourceModel {
					sawFinal = true
				}
			}
		case <-ctx.Done():
			t.Fatalf("timeout before complete tool execution flow: req=%v resp=%v final=%v", sawToolReq, sawToolResp, sawFinal)
		}
	}

	// After final model text, the agent may either prompt for more input (UI loop)
	// or declare Done depending on configuration. Accept either behavior.
	select {
	case v := <-a.Output:
		if m, ok := v.(*api.Message); ok {
			if m.Type != api.MessageTypeUserInputRequest && m.Type != api.MessageTypeText {
				t.Fatalf("unexpected message after final model text: type=%v", m.Type)
			}
		}
	default:
		if st := a.AgentState(); st != api.AgentStateDone && st != api.AgentStateWaitingForInput {
			t.Fatalf("unexpected state after tool run: %s (want Done or WaitingForInput)", st)
		}
	}
}

func TestAgentEndToEndMetaClear(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	store := sessions.NewInMemoryChatStore()
	store.AddChatMessage(&api.Message{ID: "u1", Source: api.MessageSourceUser, Type: api.MessageTypeText, Payload: "hi"})
	store.AddChatMessage(&api.Message{ID: "a1", Source: api.MessageSourceAgent, Type: api.MessageTypeText, Payload: "hello"})

	client := mocks.NewMockClient(ctrl)
	chat := mocks.NewMockChat(ctrl)

	client.EXPECT().StartChat(gomock.Any(), "test-model").Return(chat)
	chat.EXPECT().Initialize(gomock.Any()).Return(nil).Times(2) // second init after clear
	chat.EXPECT().SetFunctionDefinitions(gomock.Any()).Return(nil)

	var toolset tools.Tools
	toolset.Init()

	a := &Agent{
		ChatMessageStore: store,
		LLM:              client,
		Model:            "test-model",
		Tools:            toolset,
	}

	if err := a.Init(ctx); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := a.Run(ctx, ""); err != nil {
		t.Fatalf("run: %v", err)
	}

	// Expect startup greeting + prompt
	m1 := recvMsg(t, ctx, a.Output)
	if m1.Type != api.MessageTypeText || m1.Source != api.MessageSourceAgent {
		t.Fatalf("expected greeting text from agent, got type=%v source=%v", m1.Type, m1.Source)
	}

	m2 := recvMsg(t, ctx, a.Output)
	if m2.Type != api.MessageTypeUserInputRequest {
		t.Fatalf("expected user-input-request, got %v", m2.Type)
	}

	if got := len(store.ChatMessages()); got != 4 {
		t.Fatalf("precondition: expected 4 messages before clear, got %d", got)
	}

	// UI sends the meta command
	a.Input <- &api.UserInputResponse{Query: "clear"}

	sawClear, sawPrompt := false, false
	for !(sawClear && sawPrompt) {
		select {
		case v := <-a.Output:
			m, ok := v.(*api.Message)
			if !ok {
				t.Fatalf("expected *api.Message on output, got %T", v)
				break
			}
			if m.Type == api.MessageTypeText && m.Payload == "Cleared the conversation." {
				sawClear = true
			}
			if sawClear && m.Type == api.MessageTypeUserInputRequest {
				sawPrompt = true
			}
		case <-ctx.Done():
			t.Fatalf("timeout waiting for clear confirmation and prompt: %v", ctx.Err())
		}
	}

	msgs := store.ChatMessages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages after clear, got %d", len(msgs))
	}
	if msgs[0].Payload != "Cleared the conversation." {
		t.Fatalf("first message after clear = %q, want %q", msgs[0].Payload, "Cleared the conversation.")
	}
	if msgs[1].Type != api.MessageTypeUserInputRequest {
		t.Fatalf("second message type = %v, want user input request", msgs[1].Type)
	}
}

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
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	"github.com/GoogleCloudPlatform/kubectl-ai/internal/mocks"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/sessions"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/tools"
	"go.uber.org/mock/gomock"
)

// fake implementations

type fakeLLMClient struct{ chat *fakeChat }

func (f *fakeLLMClient) Close() error                                    { return nil }
func (f *fakeLLMClient) StartChat(systemPrompt, model string) gollm.Chat { return f.chat }
func (f *fakeLLMClient) GenerateCompletion(ctx context.Context, req *gollm.CompletionRequest) (gollm.CompletionResponse, error) {
	return nil, nil
}
func (f *fakeLLMClient) SetResponseSchema(schema *gollm.Schema) error { return nil }
func (f *fakeLLMClient) ListModels(ctx context.Context) ([]string, error) {
	return []string{"fake-model"}, nil
}

type fakeChat struct{ tokenCh chan struct{} }

func (f *fakeChat) Send(ctx context.Context, contents ...any) (gollm.ChatResponse, error) {
	return nil, nil
}
func (f *fakeChat) SendStreaming(ctx context.Context, contents ...any) (gollm.ChatResponseIterator, error) {
	return func(yield func(gollm.ChatResponse, error) bool) {
		resp := fakeChatResponse{candidate: fakeCandidate{parts: []gollm.Part{fakeTextPart("partial")}}}
		f.tokenCh <- struct{}{}
		if !yield(resp, nil) {
			return
		}
		<-ctx.Done()
		yield(nil, ctx.Err())
	}, nil
}
func (f *fakeChat) SetFunctionDefinitions([]*gollm.FunctionDefinition) error { return nil }
func (f *fakeChat) IsRetryableError(err error) bool                          { return false }
func (f *fakeChat) Initialize(messages []*api.Message) error                 { return nil }

// fake parts and responses

type fakeTextPart string

func (p fakeTextPart) AsText() (string, bool)                        { return string(p), true }
func (p fakeTextPart) AsFunctionCalls() ([]gollm.FunctionCall, bool) { return nil, false }

func TestCancelRequestAddsCancellationMessage(t *testing.T) {
	ctx := context.Background()

	fc := &fakeChat{tokenCh: make(chan struct{}, 1)}
	llm := &fakeLLMClient{chat: fc}

	var ts tools.Tools
	ts.Init()

	store := sessions.NewInMemoryChatStore()

	ag := &Agent{
		LLM:              llm,
		Tools:            ts,
		ChatMessageStore: store,
	}
	if err := ag.Init(ctx); err != nil {
		t.Fatalf("init agent: %v", err)
	}
	defer ag.Close()

	ag.startRequest(ctx)

	done := make(chan struct{})
	go func() {
		stream, err := ag.llmChat.SendStreaming(ag.currentRequest.Context(), "hi")
		if err != nil {
			t.Errorf("SendStreaming: %v", err)
			close(done)
			return
		}
		var streamedText string
		var llmErr error
		for resp, err := range stream {
			if err != nil {
				llmErr = err
				break
			}
			if resp == nil {
				break
			}
			if len(resp.Candidates()) > 0 {
				for _, part := range resp.Candidates()[0].Parts() {
					if text, ok := part.AsText(); ok {
						streamedText += text
					}
				}
			}
		}
		if llmErr == nil && streamedText != "" {
			ag.addMessage(api.MessageSourceModel, api.MessageTypeText, streamedText)
		}
		close(done)
	}()

	// wait until first token emitted
	select {
	case <-fc.tokenCh:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for token")
	}

	// cancel request
	ag.CancelRequest(ag.session.CurrentRequestID)
	<-done

	msgs := ag.session.ChatMessageStore.ChatMessages()
	if len(msgs) == 0 {
		t.Fatalf("no messages recorded")
	}
	last := msgs[len(msgs)-1]
	if last.Source != api.MessageSourceAgent || last.Payload != "Request cancelled." {
		t.Fatalf("expected final cancellation message, got %+v", last)
	}
	for _, m := range msgs[:len(msgs)-1] {
		if m.Source == api.MessageSourceModel {
			t.Fatalf("unexpected model output before cancellation: %v", m.Payload)
		}
	}
}

func TestAgentCancelsLongRunningTool(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store := sessions.NewInMemoryChatStore()

	client := mocks.NewMockClient(ctrl)
	chat := mocks.NewMockChat(ctrl)

	client.EXPECT().StartChat(gomock.Any(), "test-model").Return(chat)
	chat.EXPECT().Initialize(gomock.Any()).Return(nil)
	chat.EXPECT().SetFunctionDefinitions(gomock.Any()).Return(nil)

	response := chatWith(fCalls("mocktool", map[string]any{}))
	iter := gollm.ChatResponseIterator(func(yield func(gollm.ChatResponse, error) bool) {
		yield(response, nil)
	})
	chat.EXPECT().SendStreaming(gomock.Any(), gomock.Any()).Return(iter, nil)

	tool := mocks.NewMockTool(ctrl)
	tool.EXPECT().Name().Return("mocktool").AnyTimes()
	tool.EXPECT().Description().Return("mock tool").AnyTimes()
	tool.EXPECT().FunctionDefinition().Return(&gollm.FunctionDefinition{Name: "mocktool"}).AnyTimes()
	tool.EXPECT().IsInteractive(gomock.Any()).Return(false, nil).AnyTimes()
	tool.EXPECT().CheckModifiesResource(gomock.Any()).Return("no").AnyTimes()

	runStarted := make(chan struct{})
	runCanceled := make(chan error, 1)

	tool.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, args map[string]any) (any, error) {
		close(runStarted)
		select {
		case <-ctx.Done():
			err := ctx.Err()
			runCanceled <- err
			return nil, err
		case <-time.After(3 * time.Second):
			err := errors.New("tool was not cancelled")
			runCanceled <- err
			return nil, err
		}
	})

	var toolset tools.Tools
	toolset.Init()
	toolset.RegisterTool(tool)

	a := &Agent{
		ChatMessageStore: store,
		LLM:              client,
		Model:            "test-model",
		Tools:            toolset,
		MaxIterations:    2,
		RunOnce:          true,
		InitialQuery:     "run tool",
		RemoveWorkDir:    true,
	}

	if err := a.Init(ctx); err != nil {
		t.Fatalf("init: %v", err)
	}
	defer func() {
		if err := a.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
	}()

	if err := a.Run(ctx, "run tool"); err != nil {
		t.Fatalf("run: %v", err)
	}

	var requestID string
	for requestID == "" {
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for request ID: %v", ctx.Err())
		case <-time.After(10 * time.Millisecond):
			requestID = a.Session().CurrentRequestID
		}
	}

	select {
	case <-ctx.Done():
		t.Fatalf("timed out waiting for tool execution to start: %v", ctx.Err())
	case <-runStarted:
	}

	a.CancelRequest(requestID)

	var toolErr error
	select {
	case toolErr = <-runCanceled:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for tool cancellation: %v", ctx.Err())
	}
	if !errors.Is(toolErr, context.Canceled) {
		t.Fatalf("expected tool to be cancelled, got %v", toolErr)
	}

	sawCancel := false
	for !sawCancel {
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for cancellation message: %v", ctx.Err())
		case v := <-a.Output:
			m, ok := v.(*api.Message)
			if !ok {
				t.Fatalf("expected *api.Message on output, got %T", v)
			}
			if m.Source == api.MessageSourceAgent && m.Type == api.MessageTypeText && m.Payload == "Request cancelled." {
				sawCancel = true
			}
		}
	}
}

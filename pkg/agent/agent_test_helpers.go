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

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/sessions"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/tools"
)

// mockLLM is a mock implementation of the gollm.Client interface for testing.
type mockLLM struct {
	gollm.Client
	startChatFunc  func(systemPrompt, model string) gollm.Chat
	listModelsFunc func(ctx context.Context) ([]string, error)
}

func (m *mockLLM) StartChat(systemPrompt, model string) gollm.Chat {
	if m.startChatFunc != nil {
		return m.startChatFunc(systemPrompt, model)
	}
	return &mockChat{}
}

func (m *mockLLM) ListModels(ctx context.Context) ([]string, error) {
	if m.listModelsFunc != nil {
		return m.listModelsFunc(ctx)
	}
	return []string{"mock-model"}, nil
}

func (m *mockLLM) Close() error { return nil }

// mockChat is a mock implementation of the gollm.Chat interface for testing.
type mockChat struct {
	gollm.Chat
	sendStreamingFunc          func(ctx context.Context, contents ...any) (gollm.ChatResponseIterator, error)
	initializeFunc             func(messages []*api.Message) error
	setFunctionDefinitionsFunc func(functionDefinitions []*gollm.FunctionDefinition) error
}

func (m *mockChat) Initialize(messages []*api.Message) error {
	if m.initializeFunc != nil {
		return m.initializeFunc(messages)
	}
	return nil
}

func (m *mockChat) SendStreaming(ctx context.Context, contents ...any) (gollm.ChatResponseIterator, error) {
	if m.sendStreamingFunc != nil {
		return m.sendStreamingFunc(ctx, contents...)
	}
	// Default behavior: return an empty iterator
	return func(yield func(gollm.ChatResponse, error) bool) {}, nil
}

func (m *mockChat) SetFunctionDefinitions(functionDefinitions []*gollm.FunctionDefinition) error {
	if m.setFunctionDefinitionsFunc != nil {
		return m.setFunctionDefinitionsFunc(functionDefinitions)
	}
	return nil
}

// mockTool is a mock implementation of the tools.Tool interface for testing.
type mockTool struct {
	tools.Tool
	name                  string
	description           string
	functionDefinition    *gollm.FunctionDefinition
	checkModifiesResource string
	invokeFunc            func(ctx context.Context, options tools.InvokeToolOptions) (any, error)
}

func (m *mockTool) Name() string {
	return m.name
}

func (m *mockTool) Description() string {
	return m.description
}

func (m *mockTool) FunctionDefinition() *gollm.FunctionDefinition {
	return m.functionDefinition
}

func (m *mockTool) CheckModifiesResource(args map[string]any) string {
	return m.checkModifiesResource
}

func (m *mockTool) Run(ctx context.Context, args map[string]any) (any, error) {
	if m.invokeFunc != nil {
		return m.invokeFunc(ctx, tools.InvokeToolOptions{})
	}
	return "tool executed successfully", nil
}

func (m *mockTool) Parse(ctx context.Context, name string, args map[string]any) (*tools.ToolCall, error) {
	toolCollection := tools.Tools{}
	toolCollection.RegisterTool(m)
	return toolCollection.ParseToolInvocation(ctx, name, args)
}

func (m *mockTool) IsInteractive(args map[string]any) (bool, error) {
	return false, nil
}

// newTestAgent creates a new agent for testing with mock dependencies.
func newTestAgent(mockChat gollm.Chat, testTools ...tools.Tool) *Agent {
	chatStore := sessions.NewInMemoryChatStore()
	agentTools := tools.Tools{}
	agentTools.Init()
	for _, t := range testTools {
		agentTools.RegisterTool(t)
	}

	agent := &Agent{
		Input:            make(chan any, 1),
		Output:           make(chan any, 10),
		LLM:              &mockLLM{startChatFunc: func(sp, m string) gollm.Chat { return mockChat }},
		llmChat:          mockChat,
		Model:            "test-model",
		Tools:            agentTools,
		ChatMessageStore: chatStore,
		MaxIterations:    5,
	}
	agent.session = &api.Session{AgentState: api.AgentStateIdle, ChatMessageStore: agent.ChatMessageStore}
	return agent
}

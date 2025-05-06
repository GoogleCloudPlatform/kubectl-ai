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

package gollm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"k8s.io/klog/v2"
)

// Register the Grok provider factory on package initialization.
func init() {
	if err := RegisterProvider("grok", newGrokClientFactory); err != nil {
		klog.Fatalf("Failed to register Grok provider: %v", err)
	}
}

// newGrokClientFactory is the factory function for creating Grok clients.
func newGrokClientFactory(ctx context.Context, _ *url.URL) (Client, error) {
	// The URL is not currently used for Grok config, relies on env vars.
	return NewGrokClient(ctx)
}

// GrokClient implements the gollm.Client interface for X.AI's Grok model.
type GrokClient struct {
	client openai.Client
}

// Ensure GrokClient implements the Client interface.
var _ Client = &GrokClient{}

// NewGrokClient creates a new client for interacting with X.AI's Grok model.
// It reads the API key and optional endpoint from environment variables
// GROK_API_KEY and GROK_ENDPOINT.
func NewGrokClient(ctx context.Context) (*GrokClient, error) {
	apiKey := os.Getenv("GROK_API_KEY")
	if apiKey == "" {
		// The NewClient might handle this, but explicit check is safer
		return nil, errors.New("GROK_API_KEY environment variable not set")
	}

	// Default API endpoint for X.AI
	endpoint := "https://api.x.ai/v1"

	// Allow endpoint override
	customEndpoint := os.Getenv("GROK_ENDPOINT")
	if customEndpoint != "" {
		endpoint = customEndpoint
		klog.Infof("Using custom Grok endpoint: %s", endpoint)
	}

	// Use the OpenAI client with custom base URL
	return &GrokClient{
		client: openai.NewClient(
			option.WithAPIKey(apiKey),
			option.WithBaseURL(endpoint),
		),
	}, nil
}

// Close cleans up any resources used by the client.
func (c *GrokClient) Close() error {
	// No specific cleanup needed for the Grok client currently.
	return nil
}

// StartChat starts a new chat session.
func (c *GrokClient) StartChat(systemPrompt, model string) Chat {
	// Default to Grok-3-beta if no model is specified
	if model == "" {
		model = "grok-3-beta"
		klog.V(1).Info("No model specified, defaulting to grok-3-beta")
	}
	klog.V(1).Infof("Starting new Grok chat session with model: %s", model)

	// Initialize history with system prompt if provided
	history := []openai.ChatCompletionMessageParamUnion{}
	if systemPrompt != "" {
		history = append(history, openai.SystemMessage(systemPrompt))
	}

	return &grokChatSession{
		client:  c.client,
		history: history,
		model:   model,
		// functionDefinitions and tools will be set later via SetFunctionDefinitions
	}
}

// simpleGrokCompletionResponse is a basic implementation of CompletionResponse.
type simpleGrokCompletionResponse struct {
	content string
}

// Response returns the completion content.
func (r *simpleGrokCompletionResponse) Response() string {
	return r.content
}

// UsageMetadata returns nil for now.
func (r *simpleGrokCompletionResponse) UsageMetadata() any {
	return nil
}

// GenerateCompletion sends a completion request to the Grok API.
func (c *GrokClient) GenerateCompletion(ctx context.Context, req *CompletionRequest) (CompletionResponse, error) {
	klog.Infof("Grok GenerateCompletion called with model: %s", req.Model)
	klog.V(1).Infof("Prompt:\n%s", req.Prompt)

	// Use the Chat Completions API as shown in examples
	chatReq := openai.ChatCompletionNewParams{
		Model: openai.ChatModel(req.Model), // Use the model specified in the request
		Messages: []openai.ChatCompletionMessageParamUnion{
			// Assuming a simple user message structure for now
			openai.UserMessage(req.Prompt),
		},
	}

	completion, err := c.client.Chat.Completions.New(ctx, chatReq)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Grok completion: %w", err)
	}

	// Check if there are choices and a message
	if len(completion.Choices) == 0 || completion.Choices[0].Message.Content == "" {
		return nil, errors.New("received an empty response from Grok")
	}

	// Return the content of the first choice
	resp := &simpleGrokCompletionResponse{
		content: completion.Choices[0].Message.Content,
	}

	return resp, nil
}

// SetResponseSchema is not implemented yet for Grok.
func (c *GrokClient) SetResponseSchema(schema *Schema) error {
	klog.Warning("GrokClient.SetResponseSchema is not implemented yet")
	return nil
}

// ListModels returns a list of available Grok models.
func (c *GrokClient) ListModels(ctx context.Context) ([]string, error) {
	// Currently, Grok only has a fixed set of models
	// This could be updated to call a models endpoint if X.AI provides one in the future
	return []string{"grok-3-beta"}, nil
}

// --- Chat Session Implementation ---

type grokChatSession struct {
	client              openai.Client
	history             []openai.ChatCompletionMessageParamUnion
	model               string
	functionDefinitions []*FunctionDefinition            // Stored in gollm format
	tools               []openai.ChatCompletionToolParam // Stored in OpenAI format
}

// Ensure grokChatSession implements the Chat interface.
var _ Chat = (*grokChatSession)(nil)

// SetFunctionDefinitions stores the function definitions and converts them to Grok format.
func (cs *grokChatSession) SetFunctionDefinitions(defs []*FunctionDefinition) error {
	cs.functionDefinitions = defs
	cs.tools = nil // Clear previous tools
	if len(defs) > 0 {
		cs.tools = make([]openai.ChatCompletionToolParam, len(defs))
		for i, gollmDef := range defs {
			// Basic conversion, assuming schema is compatible or nil
			var params openai.FunctionParameters
			if gollmDef.Parameters != nil {
				// NOTE: This assumes gollmDef.Parameters is directly marshalable to JSON
				// that fits openai.FunctionParameters. May need refinement.
				bytes, err := gollmDef.Parameters.ToRawSchema()
				if err != nil {
					return fmt.Errorf("failed to convert schema for function %s: %w", gollmDef.Name, err)
				}
				if err := json.Unmarshal(bytes, &params); err != nil {
					return fmt.Errorf("failed to unmarshal schema for function %s: %w", gollmDef.Name, err)
				}
			}
			cs.tools[i] = openai.ChatCompletionToolParam{
				Function: openai.FunctionDefinitionParam{
					Name:        gollmDef.Name,
					Description: openai.String(gollmDef.Description),
					Parameters:  params,
				},
			}
		}
	}
	klog.V(1).Infof("Set %d function definitions for Grok chat session", len(cs.functionDefinitions))
	return nil
}

// Send sends the user message(s), appends to history, and gets the LLM response.
func (cs *grokChatSession) Send(ctx context.Context, contents ...any) (ChatResponse, error) {
	klog.V(1).InfoS("grokChatSession.Send called", "model", cs.model, "history_len", len(cs.history))

	// 1. Append user message(s) to history
	for _, content := range contents {
		switch c := content.(type) {
		case string:
			klog.V(2).Infof("Adding user message to history: %s", c)
			cs.history = append(cs.history, openai.UserMessage(c))
		case FunctionCallResult:
			klog.V(2).Infof("Adding tool call result to history: Name=%s, ID=%s", c.Name, c.ID)
			// Marshal the result map into a JSON string for the message content
			resultJSON, err := json.Marshal(c.Result)
			if err != nil {
				klog.Errorf("Failed to marshal function call result: %v", err)
				return nil, fmt.Errorf("failed to marshal function call result %q: %w", c.Name, err)
			}
			cs.history = append(cs.history, openai.ToolMessage(string(resultJSON), c.ID))
		default:
			// TODO: Handle other content types if necessary?
			klog.Warningf("Unhandled content type in Send: %T", content)
			return nil, fmt.Errorf("unhandled content type: %T", content)
		}
	}

	// 2. Prepare the API request
	chatReq := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(cs.model),
		Messages: cs.history,
	}
	if len(cs.tools) > 0 {
		chatReq.Tools = cs.tools
		// chatReq.ToolChoice = openai.ToolChoiceAuto // Or specify if needed
	}

	// 3. Call the Grok API
	klog.V(1).InfoS("Sending request to Grok Chat API", "model", cs.model, "messages", len(chatReq.Messages), "tools", len(chatReq.Tools))
	completion, err := cs.client.Chat.Completions.New(ctx, chatReq)
	if err != nil {
		klog.Errorf("Grok ChatCompletion API error: %v", err)
		return nil, fmt.Errorf("Grok chat completion failed: %w", err)
	}
	klog.V(1).InfoS("Received response from Grok Chat API", "id", completion.ID, "choices", len(completion.Choices))

	// 4. Process the response
	if len(completion.Choices) == 0 {
		klog.Warning("Received response with no choices from Grok")
		return nil, errors.New("received empty response from Grok (no choices)")
	}

	// Add assistant's response (first choice) to history
	assistantMsg := completion.Choices[0].Message
	// Convert to param type before appending to history
	cs.history = append(cs.history, assistantMsg.ToParam())
	klog.V(2).InfoS("Added assistant message to history", "content_present", assistantMsg.Content != "", "tool_calls", len(assistantMsg.ToolCalls))

	// Wrap the response
	resp := &grokChatResponse{
		grokCompletion: completion,
	}

	return resp, nil
}

// SendStreaming sends the user message(s) and returns an iterator for the LLM response stream.
// NOTE: This function simulates streaming by making a single non-streaming call and returning an iterator
// that yields the single response. This satisfies the agent's interface requirement.
func (cs *grokChatSession) SendStreaming(ctx context.Context, contents ...any) (ChatResponseIterator, error) {
	klog.V(1).InfoS("grokChatSession.SendStreaming called (simulated)", "model", cs.model)

	// Call the non-streaming Send method we implemented earlier
	singleResponse, err := cs.Send(ctx, contents...)
	if err != nil {
		// Send already logs errors, just wrap it
		return nil, fmt.Errorf("simulated streaming failed during non-streaming call: %w", err)
	}

	// Return an iterator function that yields the single response once.
	return singletonChatResponseIterator(singleResponse), nil
}

// IsRetryableError returns false for now.
func (cs *grokChatSession) IsRetryableError(err error) bool {
	// TODO: Implement actual retry logic if needed
	return false
}

// --- Helper structs for ChatResponse interface ---

type grokChatResponse struct {
	grokCompletion *openai.ChatCompletion
}

var _ ChatResponse = (*grokChatResponse)(nil)

func (r *grokChatResponse) UsageMetadata() any {
	// Check if the main completion object and Usage exist
	if r.grokCompletion != nil && r.grokCompletion.Usage.TotalTokens > 0 { // Check a field within Usage
		return r.grokCompletion.Usage
	}
	return nil
}

func (r *grokChatResponse) Candidates() []Candidate {
	if r.grokCompletion == nil {
		return nil
	}
	candidates := make([]Candidate, len(r.grokCompletion.Choices))
	for i, choice := range r.grokCompletion.Choices {
		candidates[i] = &grokCandidate{grokChoice: &choice}
	}
	return candidates
}

type grokCandidate struct {
	grokChoice *openai.ChatCompletionChoice
}

var _ Candidate = (*grokCandidate)(nil)

func (c *grokCandidate) Parts() []Part {
	// Check if the choice exists before accessing Message
	if c.grokChoice == nil {
		return nil
	}

	// Grok message can have Content AND ToolCalls
	var parts []Part
	if c.grokChoice.Message.Content != "" {
		parts = append(parts, &grokPart{content: c.grokChoice.Message.Content})
	}
	if len(c.grokChoice.Message.ToolCalls) > 0 {
		parts = append(parts, &grokPart{toolCalls: c.grokChoice.Message.ToolCalls})
	}
	return parts
}

// String provides a simple string representation for logging/debugging.
func (c *grokCandidate) String() string {
	if c.grokChoice == nil {
		return "<nil candidate>"
	}
	content := "<no content>"
	if c.grokChoice.Message.Content != "" {
		content = c.grokChoice.Message.Content
	}
	toolCalls := len(c.grokChoice.Message.ToolCalls)
	finishReason := string(c.grokChoice.FinishReason)
	return fmt.Sprintf("Candidate(FinishReason: %s, ToolCalls: %d, Content: %q)", finishReason, toolCalls, content)
}

type grokPart struct {
	content   string
	toolCalls []openai.ChatCompletionMessageToolCall
}

var _ Part = (*grokPart)(nil)

func (p *grokPart) AsText() (string, bool) {
	return p.content, p.content != ""
}

func (p *grokPart) AsFunctionCalls() ([]FunctionCall, bool) {
	if len(p.toolCalls) == 0 {
		return nil, false
	}

	gollmCalls := make([]FunctionCall, len(p.toolCalls))
	for i, tc := range p.toolCalls {
		// Check if it's a function call by seeing if Function Name is populated
		if tc.Function.Name == "" {
			klog.V(2).Infof("Skipping non-function tool call ID: %s", tc.ID)
			continue
		}
		var args map[string]any
		// Attempt to unmarshal arguments, ignore error for now if it fails
		_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)

		gollmCalls[i] = FunctionCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: args,
		}
	}
	return gollmCalls, true
}

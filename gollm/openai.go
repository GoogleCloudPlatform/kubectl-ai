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
	"os"
	"strings"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"k8s.io/klog/v2"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
)

// Package-level env var storage (OpenAI env)
var (
	openAIAPIKey   string
	openAIEndpoint string
	openAIAPIBase  string
	openAIModel    string
)

// init reads and caches OpenAI environment variables:
//   - OPENAI_API_KEY, OPENAI_ENDPOINT, OPENAI_API_BASE, OPENAI_MODEL
//
// These serve as defaults; the model can be overridden by the Cobra --model flag.
// After loading env values, it registers the OpenAI provider factory.
func init() {
	// Load environment variables
	openAIAPIKey = os.Getenv("OPENAI_API_KEY")
	openAIEndpoint = os.Getenv("OPENAI_ENDPOINT")
	openAIAPIBase = os.Getenv("OPENAI_API_BASE")
	openAIModel = os.Getenv("OPENAI_MODEL")

	// Register "openai" as the provider ID
	if err := RegisterProvider("openai", newOpenAIClientFactory); err != nil {
		klog.Fatalf("Failed to register openai provider: %v", err)
	}

	// Also register with any aliases defined in config
	aliases := []string{"openai-compatible"}
	for _, alias := range aliases {
		if err := RegisterProvider(alias, newOpenAIClientFactory); err != nil {
			klog.Warningf("Failed to register openai provider alias %q: %v", alias, err)
		}
	}
}

// OpenAIClient implements the gollm.Client interface for OpenAI models.
type OpenAIClient struct {
	client openai.Client
}

// Ensure OpenAIClient implements the Client interface.
var _ Client = &OpenAIClient{}

// NewOpenAIClient creates a new client for interacting with OpenAI.
// Supports custom HTTP client (e.g., for skipping SSL verification).
func NewOpenAIClient(ctx context.Context, opts ClientOptions) (*OpenAIClient, error) {
	// Get API key from loaded env var
	apiKey := openAIAPIKey
	if apiKey == "" {
		return nil, errors.New("OpenAI API key not found. Set via OPENAI_API_KEY env var")
	}

	// Set options for client creation
	options := []option.RequestOption{option.WithAPIKey(apiKey)}

	// Check for custom endpoint or API base URL
	baseURL := openAIEndpoint
	if baseURL == "" {
		baseURL = openAIAPIBase
	}

	if baseURL != "" {
		klog.Infof("Using custom OpenAI base URL: %s", baseURL)
		options = append(options, option.WithBaseURL(baseURL))
	}

	// Support custom HTTP client (e.g., skip SSL verification)
	httpClient := createCustomHTTPClient(opts.SkipVerifySSL)
	options = append(options, option.WithHTTPClient(httpClient))

	return &OpenAIClient{
		client: openai.NewClient(options...),
	}, nil
}

// Close cleans up any resources used by the client.
func (c *OpenAIClient) Close() error {
	// No specific cleanup needed for the OpenAI client currently.
	return nil
}

// StartChat starts a new chat session.
func (c *OpenAIClient) StartChat(systemPrompt, model string) Chat {
	// Get the model to use for this chat
	selectedModel := getOpenAIModel(model)

	klog.V(1).Infof("Starting new OpenAI chat session with model: %s", selectedModel)

	// Initialize history with system prompt if provided
	history := []openai.ChatCompletionMessageParamUnion{}
	if systemPrompt != "" {
		history = append(history, openai.SystemMessage(systemPrompt))
	}

	return &openAIChatSession{
		client:  c.client,
		history: history,
		model:   selectedModel,
		// functionDefinitions and tools will be set later via SetFunctionDefinitions
	}
}

// simpleCompletionResponse is a basic implementation of CompletionResponse.
type simpleCompletionResponse struct {
	content string
}

// Response returns the completion content.
func (r *simpleCompletionResponse) Response() string {
	return r.content
}

// UsageMetadata returns nil for now.
func (r *simpleCompletionResponse) UsageMetadata() any {
	return nil
}

// GenerateCompletion sends a completion request to the OpenAI API.
func (c *OpenAIClient) GenerateCompletion(ctx context.Context, req *CompletionRequest) (CompletionResponse, error) {
	klog.Infof("OpenAI GenerateCompletion called with model: %s", req.Model)
	klog.V(1).Infof("Prompt:\n%s", req.Prompt)

	// Use the Chat Completions API with the new v1.0.0 API
	completion, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: openai.ChatModel(req.Model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(req.Prompt),
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to generate OpenAI completion: %w", err)
	}

	// Check if there are choices and a message
	if len(completion.Choices) == 0 || completion.Choices[0].Message.Content == "" {
		return nil, errors.New("received an empty response from OpenAI")
	}

	// Return the content of the first choice
	resp := &simpleCompletionResponse{
		content: completion.Choices[0].Message.Content,
	}

	return resp, nil
}

// SetResponseSchema is not implemented yet.
func (c *OpenAIClient) SetResponseSchema(schema *Schema) error {
	klog.Warning("OpenAIClient.SetResponseSchema is not implemented yet")
	return nil
}

// ListModels returns a slice of strings with model IDs.
// Note: This may not work with all OpenAI-compatible providers if they don't fully implement
// the Models.List endpoint or return data in a different format.
func (c *OpenAIClient) ListModels(ctx context.Context) ([]string, error) {
	res, err := c.client.Models.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("error listing models from OpenAI: %w", err)
	}

	modelIDs := make([]string, 0, len(res.Data))
	for _, model := range res.Data {
		modelIDs = append(modelIDs, model.ID)
	}

	return modelIDs, nil
}

// Chat Session Implementation

type openAIChatSession struct {
	client              openai.Client
	history             []openai.ChatCompletionMessageParamUnion
	model               string
	functionDefinitions []*FunctionDefinition            // Stored in gollm format
	tools               []openai.ChatCompletionToolParam // Stored in OpenAI format
}

// Ensure openAIChatSession implements the Chat interface.
var _ Chat = (*openAIChatSession)(nil)

// SetFunctionDefinitions stores the function definitions and converts them to OpenAI format.
func (cs *openAIChatSession) SetFunctionDefinitions(defs []*FunctionDefinition) error {
	cs.functionDefinitions = defs
	cs.tools = nil // Clear previous tools
	if len(defs) > 0 {
		cs.tools = make([]openai.ChatCompletionToolParam, len(defs))
		for i, gollmDef := range defs {
			klog.Infof("Processing function definition: %s", gollmDef.Name)

			// Process function parameters
			params, err := cs.convertFunctionParameters(gollmDef)
			if err != nil {
				return fmt.Errorf("failed to process parameters for function %s: %w", gollmDef.Name, err)
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
	klog.V(1).Infof("Set %d function definitions for OpenAI chat session", len(cs.functionDefinitions))
	return nil
}

// Send sends the user message(s), appends to history, and gets the LLM response.
func (cs *openAIChatSession) Send(ctx context.Context, contents ...any) (ChatResponse, error) {
	klog.V(1).InfoS("openAIChatSession.Send called", "model", cs.model, "history_len", len(cs.history))

	// Process and append messages to history
	if err := cs.addContentsToHistory(contents); err != nil {
		return nil, err
	}

	// Prepare and send API request
	chatReq := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(cs.model),
		Messages: cs.history,
	}
	if len(cs.tools) > 0 {
		chatReq.Tools = cs.tools
	}

	// Call the OpenAI API
	klog.V(1).InfoS("Sending request to OpenAI Chat API", "model", cs.model, "messages", len(chatReq.Messages), "tools", len(chatReq.Tools))
	completion, err := cs.client.Chat.Completions.New(ctx, chatReq)
	if err != nil {
		// TODO: Check if error is retryable using cs.IsRetryableError
		klog.Errorf("OpenAI ChatCompletion API error: %v", err)
		return nil, fmt.Errorf("OpenAI chat completion failed: %w", err)
	}
	klog.V(1).InfoS("Received response from OpenAI Chat API", "id", completion.ID, "choices", len(completion.Choices))

	// Process the response
	if len(completion.Choices) == 0 {
		klog.Warning("Received response with no choices from OpenAI")
		return nil, errors.New("received empty response from OpenAI (no choices)")
	}

	// Add assistant's response (first choice) to history
	assistantMsg := completion.Choices[0].Message
	// Convert to param type before appending to history
	cs.history = append(cs.history, assistantMsg.ToParam())
	klog.V(2).InfoS("Added assistant message to history", "content_present", assistantMsg.Content != "", "tool_calls", len(assistantMsg.ToolCalls))

	// Wrap the response
	resp := &openAIChatResponse{
		openaiCompletion: completion,
	}

	return resp, nil
}

// SendStreaming sends the user message(s) and returns an iterator for the LLM response stream.
func (cs *openAIChatSession) SendStreaming(ctx context.Context, contents ...any) (ChatResponseIterator, error) {
	klog.V(1).InfoS("Starting OpenAI streaming request", "model", cs.model)

	// Process and append messages to history
	if err := cs.addContentsToHistory(contents); err != nil {
		return nil, err
	}

	// Prepare and send API request
	chatReq := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(cs.model),
		Messages: cs.history,
	}
	if len(cs.tools) > 0 {
		chatReq.Tools = cs.tools
	}

	// Start the OpenAI streaming request
	klog.V(1).InfoS("Sending streaming request to OpenAI API",
		"model", cs.model,
		"messageCount", len(chatReq.Messages),
		"toolCount", len(chatReq.Tools))

	stream := cs.client.Chat.Completions.NewStreaming(ctx, chatReq)

	// Create an accumulator to track the full response
	acc := openai.ChatCompletionAccumulator{}

	// Create and return the stream iterator
	return func(yield func(ChatResponse, error) bool) {
		defer stream.Close()

		var lastResponseChunk *openAIChatStreamResponse
		var currentContent strings.Builder
		var currentToolCalls []openai.ChatCompletionMessageToolCall

		// Process stream chunks
		for stream.Next() {
			chunk := stream.Current()

			// Update the accumulator with the new chunk, guarding against
			// panics from the upstream client when tool call indices are
			// missing. See https://github.com/openai/openai-go/issues/464 for more details.
			if ok, err := addChunkSafely(&acc, chunk); err != nil {
				klog.ErrorS(err, "Recovered from OpenAI accumulator panic while processing chunk")
				yield(nil, fmt.Errorf("OpenAI streaming error while accumulating chunk: %w", err))
				return
			} else if !ok {
				err := errors.New("OpenAI accumulator rejected stream chunk")
				klog.Error(err)
				yield(nil, fmt.Errorf("OpenAI streaming error while accumulating chunk: %w", err))
				return
			}

			// Handle content completion
			if _, ok := acc.JustFinishedContent(); ok {
				klog.V(2).Info("Content stream finished")
			}

			// Handle refusal completion
			if refusal, ok := acc.JustFinishedRefusal(); ok {
				klog.V(2).Infof("Refusal stream finished: %v", refusal)
				yield(nil, fmt.Errorf("model refused to respond: %v", refusal))
				return
			}

			// Handle tool call completion
			var toolCallsForThisChunk []openai.ChatCompletionMessageToolCall
			if tool, ok := acc.JustFinishedToolCall(); ok {
				klog.V(2).Infof("Tool call finished: %s %s", tool.Name, tool.Arguments)
				newToolCall := openai.ChatCompletionMessageToolCall{
					ID: tool.ID,
					Function: openai.ChatCompletionMessageToolCallFunction{
						Name:      tool.Name,
						Arguments: tool.Arguments,
					},
				}
				currentToolCalls = append(currentToolCalls, newToolCall)
				// Only include the newly finished tool call in this chunk
				toolCallsForThisChunk = []openai.ChatCompletionMessageToolCall{newToolCall}
			}

			streamResponse := &openAIChatStreamResponse{
				streamChunk: chunk,
				accumulator: acc,
				content:     "", // Default to empty content
				toolCalls:   toolCallsForThisChunk,
			}

			// Only process content if there are choices and a delta
			if len(chunk.Choices) > 0 {
				delta := chunk.Choices[0].Delta
				if delta.Content != "" {
					currentContent.WriteString(delta.Content)
					streamResponse.content = delta.Content // Only set content if there's new content
				}
			}

			// Keep track of the last response for history
			lastResponseChunk = &openAIChatStreamResponse{
				streamChunk: chunk,
				accumulator: acc,
				content:     currentContent.String(), // Full accumulated content for history
				toolCalls:   currentToolCalls,
			}

			// Only yield if there's actual content or tool calls to report
			if streamResponse.content != "" || len(streamResponse.toolCalls) > 0 {
				if !yield(streamResponse, nil) {
					return
				}
			}
		}

		// Check for errors after streaming completes
		if err := stream.Err(); err != nil {
			klog.Errorf("Error in OpenAI streaming: %v", err)
			yield(nil, fmt.Errorf("OpenAI streaming error: %w", err))
			return
		}

		// Update conversation history with the complete message
		if lastResponseChunk != nil {
			completeMessage := openai.ChatCompletionMessage{
				Content:   currentContent.String(),
				Role:      "assistant",
				ToolCalls: currentToolCalls,
			}

			// Append the full assistant response to history
			cs.history = append(cs.history, completeMessage.ToParam())
			klog.V(2).InfoS("Added complete assistant message to history",
				"content_present", completeMessage.Content != "",
				"tool_calls", len(completeMessage.ToolCalls))
		}
	}, nil
}

// addChunkSafely wraps the accumulator's AddChunk method to protect against
// panics triggered by malformed tool call indices returned by the upstream
// openai-go client. Returning an error allows the caller to surface a regular
// failure instead of crashing the entire process while the upstream fix is in
// flight.
func addChunkSafely(acc *openai.ChatCompletionAccumulator, chunk openai.ChatCompletionChunk) (added bool, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("openai accumulator panic: %v", r)
			added = false
		}
	}()

	added = acc.AddChunk(chunk)
	return
}

// IsRetryableError determines if an error from the OpenAI API should be retried.
func (cs *openAIChatSession) IsRetryableError(err error) bool {
	if err == nil {
		return false
	}
	return DefaultIsRetryableError(err)
}

func (cs *openAIChatSession) Initialize(messages []*api.Message) error {
	klog.Warning("chat history persistence is not supported for provider 'openai', using in-memory chat history")
	return nil
}

// Helper structs for ChatResponse interface

type openAIChatResponse struct {
	openaiCompletion *openai.ChatCompletion
}

var _ ChatResponse = (*openAIChatResponse)(nil)

func (r *openAIChatResponse) UsageMetadata() any {
	// Check if the main completion object and Usage exist
	if r.openaiCompletion != nil && r.openaiCompletion.Usage.TotalTokens > 0 { // Check a field within Usage
		return r.openaiCompletion.Usage
	}
	return nil
}

func (r *openAIChatResponse) Candidates() []Candidate {
	if r.openaiCompletion == nil {
		return nil
	}
	candidates := make([]Candidate, len(r.openaiCompletion.Choices))
	for i, choice := range r.openaiCompletion.Choices {
		candidates[i] = &openAICandidate{openaiChoice: &choice}
	}
	return candidates
}

type openAICandidate struct {
	openaiChoice *openai.ChatCompletionChoice
}

var _ Candidate = (*openAICandidate)(nil)

func (c *openAICandidate) Parts() []Part {
	// Check if the choice exists before accessing Message
	if c.openaiChoice == nil {
		return nil
	}

	// OpenAI message can have Content AND ToolCalls
	var parts []Part
	if c.openaiChoice.Message.Content != "" {
		parts = append(parts, &openAIPart{content: c.openaiChoice.Message.Content})
	}
	if len(c.openaiChoice.Message.ToolCalls) > 0 {
		parts = append(parts, &openAIPart{toolCalls: c.openaiChoice.Message.ToolCalls})
	}
	return parts
}

// String provides a simple string representation for logging/debugging.
func (c *openAICandidate) String() string {
	if c.openaiChoice == nil {
		return "<nil candidate>"
	}
	content := "<no content>"
	if c.openaiChoice.Message.Content != "" {
		content = c.openaiChoice.Message.Content
	}
	toolCalls := len(c.openaiChoice.Message.ToolCalls)
	finishReason := string(c.openaiChoice.FinishReason)
	return fmt.Sprintf("Candidate(FinishReason: %s, ToolCalls: %d, Content: %q)", finishReason, toolCalls, content)
}

type openAIPart struct {
	content   string
	toolCalls []openai.ChatCompletionMessageToolCall // Correct type
}

var _ Part = (*openAIPart)(nil)

func (p *openAIPart) AsText() (string, bool) {
	return p.content, p.content != ""
}

func (p *openAIPart) AsFunctionCalls() ([]FunctionCall, bool) {
	return convertToolCallsToFunctionCalls(p.toolCalls)
}

// Update openAIChatStreamResponse to include accumulated content
type openAIChatStreamResponse struct {
	streamChunk openai.ChatCompletionChunk
	accumulator openai.ChatCompletionAccumulator
	content     string
	toolCalls   []openai.ChatCompletionMessageToolCall
}

// Update Candidates() to use accumulated content
func (r *openAIChatStreamResponse) Candidates() []Candidate {
	if len(r.streamChunk.Choices) == 0 {
		return nil
	}

	candidates := make([]Candidate, len(r.streamChunk.Choices))
	for i, choice := range r.streamChunk.Choices {
		candidates[i] = &openAIStreamCandidate{
			streamChoice: choice,
			content:      r.content,
			toolCalls:    r.toolCalls,
		}
	}
	return candidates
}

// Update openAIStreamCandidate to handle delta content
type openAIStreamCandidate struct {
	streamChoice openai.ChatCompletionChunkChoice
	content      string // This will now be just the delta content
	toolCalls    []openai.ChatCompletionMessageToolCall
}

// Update Parts() to handle delta content
func (c *openAIStreamCandidate) Parts() []Part {
	var parts []Part

	// Only include the delta content
	if c.content != "" {
		parts = append(parts, &openAIStreamPart{
			content: c.content,
		})
	}

	// Include accumulated tool calls
	if len(c.toolCalls) > 0 {
		parts = append(parts, &openAIStreamPart{
			toolCalls: c.toolCalls,
		})
	}

	return parts
}

// Add UsageMetadata implementation
func (r *openAIChatStreamResponse) UsageMetadata() any {
	if r.accumulator.Usage.TotalTokens > 0 {
		return r.accumulator.Usage
	}
	return nil
}

// Add String implementation
func (c *openAIStreamCandidate) String() string {
	return fmt.Sprintf("StreamingCandidate(Content: %q, ToolCalls: %d)",
		c.content, len(c.toolCalls))
}

// Define openAIStreamPart
type openAIStreamPart struct {
	content   string
	toolCalls []openai.ChatCompletionMessageToolCall
}

// Ensure openAIStreamPart implements Part interface
var _ Part = (*openAIStreamPart)(nil)

func (p *openAIStreamPart) AsText() (string, bool) {
	return p.content, p.content != ""
}

func (p *openAIStreamPart) AsFunctionCalls() ([]FunctionCall, bool) {
	return convertToolCallsToFunctionCalls(p.toolCalls)
}

// convertSchemaForOpenAI converts and transforms a schema for OpenAI compatibility
// This function handles both gollm Schema objects and ensures the final JSON meets OpenAI requirements
func convertSchemaForOpenAI(schema *Schema) (*Schema, error) {
	if schema == nil {
		// Return a minimal valid object schema for OpenAI
		return &Schema{
			Type:       TypeObject,
			Properties: make(map[string]*Schema),
		}, nil
	}

	// Create a deep copy to avoid modifying the original
	validated := &Schema{
		Description: schema.Description,
		Required:    make([]string, len(schema.Required)),
	}
	copy(validated.Required, schema.Required)

	// Handle type validation and normalization based on OpenAI requirements
	switch schema.Type {
	case TypeObject:
		validated.Type = TypeObject
		// Objects MUST have properties for OpenAI (even if empty)
		validated.Properties = make(map[string]*Schema)
		if schema.Properties != nil {
			for key, prop := range schema.Properties {
				validatedProp, err := convertSchemaForOpenAI(prop)
				if err != nil {
					return nil, fmt.Errorf("validating property %q: %w", key, err)
				}
				validated.Properties[key] = validatedProp
			}
		}

	case TypeArray:
		validated.Type = TypeArray
		// Arrays MUST have items schema for OpenAI
		if schema.Items != nil {
			validatedItems, err := convertSchemaForOpenAI(schema.Items)
			if err != nil {
				return nil, fmt.Errorf("validating array items: %w", err)
			}
			validated.Items = validatedItems
		} else {
			// Default to string items if not specified
			validated.Items = &Schema{Type: TypeString}
		}

	case TypeString:
		validated.Type = TypeString

	case TypeNumber:
		validated.Type = TypeNumber

	case TypeInteger:
		// OpenAI prefers "number" for integers
		validated.Type = TypeNumber

	case TypeBoolean:
		validated.Type = TypeBoolean

	case "":
		// If no type specified, default to object with empty properties
		klog.Warningf("Schema has no type, defaulting to object")
		validated.Type = TypeObject
		validated.Properties = make(map[string]*Schema)

	default:
		// For unknown types, log a warning and default to object
		klog.Warningf("Unknown schema type '%s', defaulting to object", schema.Type)
		validated.Type = TypeObject
		validated.Properties = make(map[string]*Schema)
	}

	// Final validation: Ensure object types always have properties
	// This handles edge cases where malformed schemas might slip through
	if validated.Type == TypeObject && validated.Properties == nil {
		klog.Warningf("Object schema missing properties, initializing empty properties map")
		validated.Properties = make(map[string]*Schema)
	}

	return validated, nil
}

// convertFunctionParameters handles the conversion of gollm parameters to OpenAI format
func (cs *openAIChatSession) convertFunctionParameters(gollmDef *FunctionDefinition) (openai.FunctionParameters, error) {
	var params openai.FunctionParameters

	if gollmDef.Parameters == nil {
		return params, nil
	}

	// Convert the schema for OpenAI compatibility
	klog.V(2).Infof("Original schema for function %s: %+v", gollmDef.Name, gollmDef.Parameters)
	validatedSchema, err := convertSchemaForOpenAI(gollmDef.Parameters)
	if err != nil {
		return params, fmt.Errorf("schema conversion failed: %w", err)
	}
	klog.V(2).Infof("Converted schema for function %s: %+v", gollmDef.Name, validatedSchema)

	// Convert to raw schema bytes
	schemaBytes, err := cs.convertSchemaToBytes(validatedSchema, gollmDef.Name)
	if err != nil {
		return params, err
	}

	// Unmarshal into OpenAI parameters format
	if err := json.Unmarshal(schemaBytes, &params); err != nil {
		return params, fmt.Errorf("failed to unmarshal schema: %w", err)
	}

	return params, nil
}

// openAISchema wraps a gollm Schema with OpenAI-specific marshaling behavior
type openAISchema struct {
	*Schema
}

// MarshalJSON provides OpenAI-specific JSON marshaling that ensures object schemas have properties
func (s openAISchema) MarshalJSON() ([]byte, error) {
	// Create a map to build the JSON representation
	result := make(map[string]interface{})

	if s.Type != "" {
		result["type"] = s.Type
	}

	if s.Description != "" {
		result["description"] = s.Description
	}

	if len(s.Required) > 0 {
		result["required"] = s.Required
	}

	// For object types, always include properties (even if empty) to satisfy OpenAI
	if s.Type == TypeObject {
		if s.Properties != nil {
			result["properties"] = s.Properties
		} else {
			result["properties"] = make(map[string]*Schema)
		}
	} else if s.Properties != nil && len(s.Properties) > 0 {
		// For non-object types, only include properties if they exist and are non-empty
		result["properties"] = s.Properties
	}

	if s.Items != nil {
		result["items"] = s.Items
	}

	return json.Marshal(result)
}

// convertSchemaToBytes converts a validated schema to JSON bytes using OpenAI-specific marshaling
func (cs *openAIChatSession) convertSchemaToBytes(schema *Schema, functionName string) ([]byte, error) {
	// Wrap the schema with OpenAI-specific marshaling behavior
	openAIWrapper := openAISchema{Schema: schema}

	bytes, err := json.Marshal(openAIWrapper)
	if err != nil {
		return nil, fmt.Errorf("failed to convert schema: %w", err)
	}

	klog.Infof("OpenAI schema for function %s: %s", functionName, string(bytes))

	return bytes, nil
}

// newOpenAIClientFactory is the factory function for creating OpenAI clients.
func newOpenAIClientFactory(ctx context.Context, opts ClientOptions) (Client, error) {
	return NewOpenAIClient(ctx, opts)
}

// addContentsToHistory processes and appends user messages to chat history
func (cs *openAIChatSession) addContentsToHistory(contents []any) error {
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
				return fmt.Errorf("failed to marshal function call result %q: %w", c.Name, err)
			}
			cs.history = append(cs.history, openai.ToolMessage(string(resultJSON), c.ID))
		default:
			klog.Warningf("Unhandled content type: %T", content)
			return fmt.Errorf("unhandled content type: %T", content)
		}
	}
	return nil
}

// convertToolCallsToFunctionCalls converts OpenAI tool calls to gollm function calls
func convertToolCallsToFunctionCalls(toolCalls []openai.ChatCompletionMessageToolCall) ([]FunctionCall, bool) {
	if len(toolCalls) == 0 {
		return nil, false
	}

	calls := make([]FunctionCall, 0, len(toolCalls))
	for _, tc := range toolCalls {
		// Skip non-function tool calls
		if tc.Function.Name == "" {
			klog.V(2).Infof("Skipping non-function tool call ID: %s", tc.ID)
			continue
		}

		// Parse function arguments with error handling
		var args map[string]any
		if tc.Function.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				klog.V(2).Infof("Error unmarshalling function arguments for %s: %v", tc.Function.Name, err)
				args = make(map[string]any)
			}
		} else {
			args = make(map[string]any)
		}

		calls = append(calls, FunctionCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}
	return calls, len(calls) > 0
}

// getOpenAIModel returns the appropriate model based on configuration and explicitly provided model name
func getOpenAIModel(model string) string {
	// If explicit model is provided, use it
	if model != "" {
		klog.V(2).Infof("Using explicitly provided model: %s", model)
		return model
	}

	// Check configuration
	configModel := openAIModel
	if configModel != "" {
		klog.V(1).Infof("Using model from config: %s", configModel)
		return configModel
	}

	// Default model as fallback
	klog.V(2).Info("No model specified, defaulting to gpt-4.1")
	return "gpt-4.1"
}

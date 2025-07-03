// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
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
	"fmt"
	"io"
	"iter"
	"time"
)

// Client is a client for a language model.
type Client interface {
	io.Closer

	// StartChat starts a new multi-turn chat with a language model.
	StartChat(systemPrompt, model string) Chat

	// GenerateCompletion generates a single completion for a given prompt.
	GenerateCompletion(ctx context.Context, req *CompletionRequest) (CompletionResponse, error)

	// SetResponseSchema constrains LLM responses to match the provided schema.
	// Calling with nil will clear the current schema.
	SetResponseSchema(schema *Schema) error

	// ListModels lists the models available in the LLM.
	ListModels(ctx context.Context) ([]string, error)
}

// Chat is an active conversation with a language model.
// Messages are sent and received, and add to a conversation history.
type Chat interface {
	// Send adds a user message to the chat, and gets the response from the LLM.
	// Note that this method automatically updates the state of the Chat,
	// you do not need to "replay" any messages from the LLM.
	Send(ctx context.Context, contents ...any) (ChatResponse, error)

	// SendStreaming is the streaming version of Send.
	SendStreaming(ctx context.Context, contents ...any) (ChatResponseIterator, error)

	// SetFunctionDefinitions configures the set of tools (functions) available to the LLM
	// for function calling.
	SetFunctionDefinitions(functionDefinitions []*FunctionDefinition) error

	// IsRetryableError returns true if the error is retryable.
	IsRetryableError(error) bool
}

// CompletionRequest is a request to generate a completion for a given prompt.
type CompletionRequest struct {
	Model  string `json:"model,omitempty"`
	Prompt string `json:"prompt,omitempty"`
}

// CompletionResponse is a response from the GenerateCompletion method.
type CompletionResponse interface {
	Response() string
	UsageMetadata() any
}

// FunctionCall is a function call to a language model.
// The LLM will reply with a FunctionCall to a user-defined function, and we will send the results back.
type FunctionCall struct {
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// FunctionDefinition is a user-defined function that can be called by the LLM.
// If the LLM determines the function should be called, it will reply with a FunctionCall object;
// we will invoke the function and the results back.
type FunctionDefinition struct {
	Name        string  `json:"name,omitempty"`
	Description string  `json:"description,omitempty"`
	Parameters  *Schema `json:"parameters,omitempty"`
}

// Schema is a schema for a function definition.
type Schema struct {
	Type        SchemaType         `json:"type,omitempty"`
	Properties  map[string]*Schema `json:"properties,omitempty"`
	Items       *Schema            `json:"items,omitempty"`
	Description string             `json:"description,omitempty"`
	Required    []string           `json:"required,omitempty"`
}

// ToRawSchema converts a Schema to a json.RawMessage.
func (s *Schema) ToRawSchema() (json.RawMessage, error) {
	jsonSchema, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("converting tool schema to json: %w", err)
	}
	var rawSchema json.RawMessage
	if err := json.Unmarshal(jsonSchema, &rawSchema); err != nil {
		return nil, fmt.Errorf("converting tool schema to json.RawMessage: %w", err)
	}
	return rawSchema, nil
}

// SchemaType is the type of a field in a Schema.
type SchemaType string

const (
	TypeObject SchemaType = "object"
	TypeArray  SchemaType = "array"

	TypeString  SchemaType = "string"
	TypeBoolean SchemaType = "boolean"
	TypeNumber  SchemaType = "number"
	TypeInteger SchemaType = "integer"
)

// FunctionCallResult is the result of a function call.
// We use this to send the results back to the LLM.
type FunctionCallResult struct {
	ID     string         `json:"id,omitempty"`
	Name   string         `json:"name,omitempty"`
	Result map[string]any `json:"result,omitempty"`
}

// ChatResponse is a generic chat response from the LLM.
type ChatResponse interface {
	UsageMetadata() any

	// Candidates are a set of candidate responses from the LLM.
	// The LLM may return multiple candidates, and we can choose the best one.
	Candidates() []Candidate
}

// ChatResponseIterator is a streaming chat response from the LLM.
type ChatResponseIterator iter.Seq2[ChatResponse, error]

// Candidate is one of a set of candidate response from the LLM.
type Candidate interface {
	// String returns a string representation of the candidate.
	fmt.Stringer

	// Parts returns the parts of the candidate.
	Parts() []Part
}

// Part is a part of a candidate response from the LLM.
// It can be a text response, or a function call.
// A response may comprise multiple parts,
// for example a text response and a function call
// where the text response is "I need to do the necessary"
// and then the function call is "do_necessary".
type Part interface {
	// AsText returns the text of the part.
	// if the part is not text, it returns ("", false)
	AsText() (string, bool)

	// AsFunctionCalls returns the function calls of the part.
	// if the part is not a function call, it returns (nil, false)
	AsFunctionCalls() ([]FunctionCall, bool)
}

// Usage represents standardized token usage and cost information across providers.
// This provides a structured format for usage metrics while maintaining backwards
// compatibility with the existing UsageMetadata() any interface for raw provider data.
type Usage struct {
	// Token usage information
	InputTokens  int `json:"inputTokens,omitempty"`
	OutputTokens int `json:"outputTokens,omitempty"`
	TotalTokens  int `json:"totalTokens,omitempty"`

	// Cost information (in USD)
	InputCost  float64 `json:"inputCost,omitempty"`
	OutputCost float64 `json:"outputCost,omitempty"`
	TotalCost  float64 `json:"totalCost,omitempty"`

	// Metadata
	Model     string    `json:"model,omitempty"`
	Provider  string    `json:"provider,omitempty"`
	Timestamp time.Time `json:"timestamp,omitempty"`
}

// MarshalJSON implements json.Marshaler interface for Usage.
func (u Usage) MarshalJSON() ([]byte, error) {
	// Create a map to handle omitempty behavior properly
	result := make(map[string]interface{})

	if u.InputTokens != 0 {
		result["inputTokens"] = u.InputTokens
	}
	if u.OutputTokens != 0 {
		result["outputTokens"] = u.OutputTokens
	}
	if u.TotalTokens != 0 {
		result["totalTokens"] = u.TotalTokens
	}
	if u.InputCost != 0 {
		result["inputCost"] = u.InputCost
	}
	if u.OutputCost != 0 {
		result["outputCost"] = u.OutputCost
	}
	if u.TotalCost != 0 {
		result["totalCost"] = u.TotalCost
	}
	if u.Model != "" {
		result["model"] = u.Model
	}
	if u.Provider != "" {
		result["provider"] = u.Provider
	}
	if !u.Timestamp.IsZero() {
		result["timestamp"] = u.Timestamp
	}

	return json.Marshal(result)
}

// UnmarshalJSON implements json.Unmarshaler interface for Usage.
func (u *Usage) UnmarshalJSON(data []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if v, ok := raw["inputTokens"]; ok {
		if tokens, ok := v.(float64); ok {
			u.InputTokens = int(tokens)
		}
	}
	if v, ok := raw["outputTokens"]; ok {
		if tokens, ok := v.(float64); ok {
			u.OutputTokens = int(tokens)
		}
	}
	if v, ok := raw["totalTokens"]; ok {
		if tokens, ok := v.(float64); ok {
			u.TotalTokens = int(tokens)
		}
	}
	if v, ok := raw["inputCost"]; ok {
		if cost, ok := v.(float64); ok {
			u.InputCost = cost
		}
	}
	if v, ok := raw["outputCost"]; ok {
		if cost, ok := v.(float64); ok {
			u.OutputCost = cost
		}
	}
	if v, ok := raw["totalCost"]; ok {
		if cost, ok := v.(float64); ok {
			u.TotalCost = cost
		}
	}
	if v, ok := raw["model"]; ok {
		if model, ok := v.(string); ok {
			u.Model = model
		}
	}
	if v, ok := raw["provider"]; ok {
		if provider, ok := v.(string); ok {
			u.Provider = provider
		}
	}
	if v, ok := raw["timestamp"]; ok {
		if timestamp, ok := v.(string); ok {
			if t, err := time.Parse(time.RFC3339, timestamp); err == nil {
				u.Timestamp = t
			}
		}
	}

	return nil
}

// IsValid validates that Usage has minimum required fields.
func (u Usage) IsValid() bool {
	// Provider is required for proper usage tracking
	return u.Provider != ""
}

// InferenceConfig provides standardized inference parameters across providers.
// This allows passing configuration to providers in a consistent way while
// each provider can extract relevant parameters for their specific implementation.
type InferenceConfig struct {
	// Model configuration
	Model  string `json:"model,omitempty" yaml:"model,omitempty"`
	Region string `json:"region,omitempty" yaml:"region,omitempty"`

	// Generation parameters
	Temperature float32 `json:"temperature,omitempty" yaml:"temperature,omitempty"`
	MaxTokens   int32   `json:"maxTokens,omitempty" yaml:"maxTokens,omitempty"`
	TopP        float32 `json:"topP,omitempty" yaml:"topP,omitempty"`
	TopK        int32   `json:"topK,omitempty" yaml:"topK,omitempty"`

	// Retry configuration
	MaxRetries int `json:"maxRetries,omitempty" yaml:"maxRetries,omitempty"`
}

// MarshalYAML implements yaml.Marshaler interface for InferenceConfig.
func (c InferenceConfig) MarshalYAML() (interface{}, error) {
	// Create a map to handle omitempty behavior
	result := make(map[string]interface{})

	if c.Model != "" {
		result["model"] = c.Model
	}
	if c.Region != "" {
		result["region"] = c.Region
	}
	if c.Temperature != 0 {
		result["temperature"] = c.Temperature
	}
	if c.MaxTokens != 0 {
		result["maxTokens"] = c.MaxTokens
	}
	if c.TopP != 0 {
		result["topP"] = c.TopP
	}
	if c.TopK != 0 {
		result["topK"] = c.TopK
	}
	if c.MaxRetries != 0 {
		result["maxRetries"] = c.MaxRetries
	}

	return result, nil
}

// IsValid validates that InferenceConfig has reasonable parameter values.
func (c InferenceConfig) IsValid() bool {
	// Check parameter ranges
	if c.Temperature < 0 || c.Temperature > 2.0 {
		return false
	}
	if c.MaxTokens < 0 {
		return false
	}
	if c.TopP < 0 || c.TopP > 1.0 {
		return false
	}
	if c.TopK < 0 {
		return false
	}
	if c.MaxRetries < 0 {
		return false
	}

	return true
}

// UsageCallback is called when structured usage data is available.
// This allows upstream applications to collect usage metrics, calculate costs,
// and aggregate statistics across multiple model calls.
type UsageCallback func(providerName string, model string, usage Usage)

// UsageExtractor provides a way to convert raw provider-specific usage data
// into standardized Usage structs. Each provider can implement their own
// extractor to handle their specific usage data format.
type UsageExtractor interface {
	// ExtractUsage converts raw provider usage data to standardized Usage.
	// Returns nil if the raw usage data cannot be processed.
	ExtractUsage(rawUsage any, model string, provider string) *Usage
}

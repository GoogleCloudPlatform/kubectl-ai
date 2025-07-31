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

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
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

	// Initialize initializes the chat with a previous conversation history.
	Initialize(messages []*api.Message) error
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

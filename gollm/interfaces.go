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
	"fmt"
	"io"
)

// Client is a client for a language model.
type Client interface {
	io.Closer

	// StartChat starts a new multi-turn chat with a language model.
	StartChat(systemPrompt string) Chat

	// GenerateCompletion generates a single completion for a given prompt.
	GenerateCompletion(ctx context.Context, req *CompletionRequest) (CompletionResponse, error)
}

// Chat is an active conversation with a language model.
// Messages are sent and received, and add to a conversation history.
type Chat interface {
	SendMessage(ctx context.Context, userParts ...string) (ChatResponse, error)
	SetFunctionDefinitions(functionDefinitions []*FunctionDefinition) error
	SendFunctionResults(ctx context.Context, functionResults []FunctionCallResult) (ChatResponse, error)
}

// CompletionRequest is a request to generate a completion for a given prompt.
type CompletionRequest struct {
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
	Description string             `json:"description,omitempty"`
	Required    []string           `json:"required,omitempty"`
}

// SchemaType is the type of a field in a Schema.
type SchemaType string

const (
	TypeObject SchemaType = "object"
	TypeString SchemaType = "string"
	TypeBoolean SchemaType = "boolean"
)

// FunctionCallResult is the result of a function call.
// We use this to send the results back to the LLM.
type FunctionCallResult struct {
	Name   string         `json:"name,omitempty"`
	Result map[string]any `json:"result,omitempty"`
}

// Response is a generic chat response from the LLM.
type ChatResponse interface {
	UsageMetadata() any

	// Candidates are a set of candidate responses from the LLM.
	// The LLM may return multiple candidates, and we can choose the best one.
	Candidates() []Candidate
}

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

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
	"encoding/json"
	"testing"

	"github.com/openai/openai-go"
)

func TestConvertSchemaForOpenAI(t *testing.T) {
	tests := []struct {
		name           string
		inputSchema    *Schema
		expectedType   SchemaType
		expectedError  bool
		validateResult func(t *testing.T, result *Schema)
	}{
		// Core logic tests
		{
			name:          "nil schema",
			inputSchema:   nil,
			expectedType:  TypeObject,
			expectedError: false,
			validateResult: func(t *testing.T, result *Schema) {
				if result.Properties == nil {
					t.Error("expected properties map to be initialized")
				}
				if len(result.Properties) != 0 {
					t.Error("expected empty properties map")
				}
			},
		},
		{
			name: "simple string schema",
			inputSchema: &Schema{
				Type:        TypeString,
				Description: "A simple string",
			},
			expectedType:  TypeString,
			expectedError: false,
			validateResult: func(t *testing.T, result *Schema) {
				if result.Description != "A simple string" {
					t.Errorf("expected description 'A simple string', got %q", result.Description)
				}
			},
		},
		{
			name: "simple number schema",
			inputSchema: &Schema{
				Type: TypeNumber,
			},
			expectedType:  TypeNumber,
			expectedError: false,
		},
		{
			name: "integer schema converted to number",
			inputSchema: &Schema{
				Type:        TypeInteger,
				Description: "An integer value",
			},
			expectedType:  TypeNumber, // OpenAI prefers number for integers
			expectedError: false,
			validateResult: func(t *testing.T, result *Schema) {
				if result.Description != "An integer value" {
					t.Errorf("expected description preserved")
				}
			},
		},
		{
			name: "boolean schema",
			inputSchema: &Schema{
				Type: TypeBoolean,
			},
			expectedType:  TypeBoolean,
			expectedError: false,
		},
		{
			name: "empty type defaults to object",
			inputSchema: &Schema{
				Description: "No type specified",
			},
			expectedType:  TypeObject,
			expectedError: false,
			validateResult: func(t *testing.T, result *Schema) {
				if result.Properties == nil {
					t.Error("expected properties map to be initialized")
				}
			},
		},
		{
			name: "unknown type defaults to object",
			inputSchema: &Schema{
				Type: "unknown",
			},
			expectedType:  TypeObject,
			expectedError: false,
			validateResult: func(t *testing.T, result *Schema) {
				if result.Properties == nil {
					t.Error("expected properties map to be initialized")
				}
			},
		},
		{
			name: "object schema with properties",
			inputSchema: &Schema{
				Type: TypeObject,
				Properties: map[string]*Schema{
					"name": {Type: TypeString, Description: "User name"},
					"age":  {Type: TypeInteger, Description: "User age"},
				},
				Required: []string{"name"},
			},
			expectedType:  TypeObject,
			expectedError: false,
			validateResult: func(t *testing.T, result *Schema) {
				if len(result.Properties) != 2 {
					t.Errorf("expected 2 properties, got %d", len(result.Properties))
				}
				if result.Properties["name"].Type != TypeString {
					t.Error("expected name property to be string")
				}
				// Age should be converted from integer to number
				if result.Properties["age"].Type != TypeNumber {
					t.Error("expected age property to be converted to number")
				}
				if len(result.Required) != 1 || result.Required[0] != "name" {
					t.Error("expected required fields to be preserved")
				}
			},
		},
		{
			name: "object schema without properties",
			inputSchema: &Schema{
				Type: TypeObject,
			},
			expectedType:  TypeObject,
			expectedError: false,
			validateResult: func(t *testing.T, result *Schema) {
				if result.Properties == nil {
					t.Error("expected properties map to be initialized")
				}
				if len(result.Properties) != 0 {
					t.Error("expected empty properties map")
				}
			},
		},
		{
			name: "array schema with string items",
			inputSchema: &Schema{
				Type:  TypeArray,
				Items: &Schema{Type: TypeString},
			},
			expectedType:  TypeArray,
			expectedError: false,
			validateResult: func(t *testing.T, result *Schema) {
				if result.Items == nil {
					t.Error("expected items schema to be present")
				}
				if result.Items.Type != TypeString {
					t.Error("expected items to be string type")
				}
			},
		},
		{
			name: "array schema with integer items (converted to number)",
			inputSchema: &Schema{
				Type:  TypeArray,
				Items: &Schema{Type: TypeInteger},
			},
			expectedType:  TypeArray,
			expectedError: false,
			validateResult: func(t *testing.T, result *Schema) {
				if result.Items == nil {
					t.Error("expected items schema to be present")
				}
				if result.Items.Type != TypeNumber {
					t.Error("expected items to be converted to number type")
				}
			},
		},
		{
			name: "array schema without items (defaults to string)",
			inputSchema: &Schema{
				Type: TypeArray,
			},
			expectedType:  TypeArray,
			expectedError: false,
			validateResult: func(t *testing.T, result *Schema) {
				if result.Items == nil {
					t.Error("expected items schema to be defaulted")
				}
				if result.Items.Type != TypeString {
					t.Error("expected default items to be string type")
				}
			},
		},
		{
			name: "nested object in array",
			inputSchema: &Schema{
				Type: TypeArray,
				Items: &Schema{
					Type: TypeObject,
					Properties: map[string]*Schema{
						"id":   {Type: TypeInteger},
						"name": {Type: TypeString},
					},
				},
			},
			expectedType:  TypeArray,
			expectedError: false,
			validateResult: func(t *testing.T, result *Schema) {
				if result.Items == nil {
					t.Error("expected items schema to be present")
				}
				if result.Items.Type != TypeObject {
					t.Error("expected items to be object type")
				}
				if result.Items.Properties["id"].Type != TypeNumber {
					t.Error("expected nested integer to be converted to number")
				}
				if result.Items.Properties["name"].Type != TypeString {
					t.Error("expected nested string to remain string")
				}
			},
		},

		// Built-in tool schema tests
		{
			name: "kubectl tool schema",
			inputSchema: &Schema{
				Type: TypeObject,
				Properties: map[string]*Schema{
					"command": {
						Type:        TypeString,
						Description: "The complete kubectl command to execute",
					},
					"modifies_resource": {
						Type:        TypeString,
						Description: "Whether the command modifies a kubernetes resource",
					},
				},
			},
			expectedType:  TypeObject,
			expectedError: false,
			validateResult: func(t *testing.T, result *Schema) {
				if len(result.Properties) != 2 {
					t.Errorf("expected 2 properties, got %d", len(result.Properties))
				}
				if result.Properties["command"].Type != TypeString {
					t.Error("expected command property to be string")
				}
				if result.Properties["modifies_resource"].Type != TypeString {
					t.Error("expected modifies_resource property to be string")
				}
				// Properties should be initialized
				if result.Properties == nil {
					t.Error("expected properties to be initialized")
				}
			},
		},
		{
			name: "bash tool schema",
			inputSchema: &Schema{
				Type: TypeObject,
				Properties: map[string]*Schema{
					"command": {
						Type:        TypeString,
						Description: "The bash command to execute",
					},
					"modifies_resource": {
						Type:        TypeString,
						Description: "Whether the command modifies a kubernetes resource",
					},
				},
			},
			expectedType:  TypeObject,
			expectedError: false,
			validateResult: func(t *testing.T, result *Schema) {
				if len(result.Properties) != 2 {
					t.Errorf("expected 2 properties, got %d", len(result.Properties))
				}
				// All string properties should remain strings
				if result.Properties["command"].Type != TypeString {
					t.Error("expected command property to remain string")
				}
				if result.Properties["modifies_resource"].Type != TypeString {
					t.Error("expected modifies_resource property to remain string")
				}
			},
		},
		{
			name: "mcp tool schema with complex nested structure",
			inputSchema: &Schema{
				Type: TypeObject,
				Properties: map[string]*Schema{
					"server_name": {
						Type:        TypeString,
						Description: "Name of the MCP server",
					},
					"method": {
						Type:        TypeString,
						Description: "MCP method name",
					},
					"params": {
						Type: TypeObject,
						Properties: map[string]*Schema{
							"query": {Type: TypeString},
							"limit": {Type: TypeInteger}, // Should convert to number
						},
					},
				},
				Required: []string{"server_name", "method"},
			},
			expectedType:  TypeObject,
			expectedError: false,
			validateResult: func(t *testing.T, result *Schema) {
				// Check top-level properties
				if len(result.Properties) != 3 {
					t.Errorf("expected 3 properties, got %d", len(result.Properties))
				}
				// Check nested object conversion
				params := result.Properties["params"]
				if params.Type != TypeObject {
					t.Error("expected params to be object type")
				}
				if params.Properties == nil {
					t.Error("expected params properties to be initialized")
				}
				// Check nested integer conversion
				if params.Properties["limit"].Type != TypeNumber {
					t.Error("expected nested limit property to be converted to number")
				}
				// Check required fields preservation
				if len(result.Required) != 2 {
					t.Error("expected required fields to be preserved")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := convertSchemaForOpenAI(tt.inputSchema)

			if tt.expectedError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Error("expected non-nil result")
				return
			}

			if result.Type != tt.expectedType {
				t.Errorf("expected type %q, got %q", tt.expectedType, result.Type)
			}

			// Run custom validation if provided
			if tt.validateResult != nil {
				tt.validateResult(t, result)
			}
		})
	}
}

// TestConvertSchemaToBytes tests the JSON-level fix for the omitempty issue
func TestConvertSchemaToBytes(t *testing.T) {
	session := &openAIChatSession{}

	// Test case: Object schema with empty properties map (which gets omitted by omitempty)
	schema := &Schema{
		Type:       TypeObject,
		Properties: make(map[string]*Schema), // Empty map gets omitted by omitempty
	}

	bytes, err := session.convertSchemaToBytes(schema, "test_function")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
		return
	}

	// Parse the JSON to verify it has properties field
	var schemaMap map[string]any
	if err := json.Unmarshal(bytes, &schemaMap); err != nil {
		t.Errorf("failed to unmarshal schema: %v", err)
		return
	}

	// Verify the schema has type: object
	if schemaType, ok := schemaMap["type"].(string); !ok || schemaType != "object" {
		t.Errorf("expected type 'object', got %v", schemaMap["type"])
	}

	// Verify the schema has properties field (even if empty)
	if _, hasProperties := schemaMap["properties"]; !hasProperties {
		t.Error("expected properties field to be present in JSON, but it was missing")
	}

	// Verify properties is an empty object
	if props, ok := schemaMap["properties"].(map[string]any); !ok {
		t.Error("expected properties to be an object")
	} else if len(props) != 0 {
		t.Errorf("expected empty properties object, got %v", props)
	}
}

// TestConvertToolCallsToFunctionCalls tests the tool call conversion logic
func TestConvertToolCallsToFunctionCalls(t *testing.T) {
	tests := []struct {
		name           string
		toolCalls      []openai.ChatCompletionMessageToolCall
		expectedCount  int
		expectedResult bool
		validateCalls  func(t *testing.T, calls []FunctionCall)
	}{
		{
			name:           "empty tool calls",
			toolCalls:      []openai.ChatCompletionMessageToolCall{},
			expectedCount:  0,
			expectedResult: false,
		},
		{
			name:           "nil tool calls",
			toolCalls:      nil,
			expectedCount:  0,
			expectedResult: false,
		},
		{
			name: "single valid tool call",
			toolCalls: []openai.ChatCompletionMessageToolCall{
				{
					ID: "call_123",
					Function: openai.ChatCompletionMessageToolCallFunction{
						Name:      "kubectl",
						Arguments: `{"command":"kubectl get pods --namespace=app-dev01","modifies_resource":"no"}`,
					},
				},
			},
			expectedCount:  1,
			expectedResult: true,
			validateCalls: func(t *testing.T, calls []FunctionCall) {
				if calls[0].ID != "call_123" {
					t.Errorf("expected ID 'call_123', got %s", calls[0].ID)
				}
				if calls[0].Name != "kubectl" {
					t.Errorf("expected Name 'kubectl', got %s", calls[0].Name)
				}
				if calls[0].Arguments["command"] != "kubectl get pods --namespace=app-dev01" {
					t.Errorf("expected command argument, got %v", calls[0].Arguments["command"])
				}
				if calls[0].Arguments["modifies_resource"] != "no" {
					t.Errorf("expected modifies_resource argument, got %v", calls[0].Arguments["modifies_resource"])
				}
			},
		},
		{
			name: "tool call with empty function name",
			toolCalls: []openai.ChatCompletionMessageToolCall{
				{
					ID: "call_456",
					Function: openai.ChatCompletionMessageToolCallFunction{
						Name:      "",
						Arguments: `{"command":"kubectl get pods"}`,
					},
				},
			},
			expectedCount:  0,
			expectedResult: false,
		},
		{
			name: "tool call with invalid JSON arguments",
			toolCalls: []openai.ChatCompletionMessageToolCall{
				{
					ID: "call_789",
					Function: openai.ChatCompletionMessageToolCallFunction{
						Name:      "kubectl",
						Arguments: `{"command":"kubectl get pods", invalid json}`,
					},
				},
			},
			expectedCount:  1,
			expectedResult: true,
			validateCalls: func(t *testing.T, calls []FunctionCall) {
				if calls[0].ID != "call_789" {
					t.Errorf("expected ID 'call_789', got %s", calls[0].ID)
				}
				if calls[0].Name != "kubectl" {
					t.Errorf("expected Name 'kubectl', got %s", calls[0].Name)
				}
				// Arguments should be empty due to parsing error
				if len(calls[0].Arguments) != 0 {
					t.Errorf("expected empty arguments due to parse error, got %v", calls[0].Arguments)
				}
			},
		},
		{
			name: "tool call with empty arguments",
			toolCalls: []openai.ChatCompletionMessageToolCall{
				{
					ID: "call_empty",
					Function: openai.ChatCompletionMessageToolCallFunction{
						Name:      "kubectl",
						Arguments: "",
					},
				},
			},
			expectedCount:  1,
			expectedResult: true,
			validateCalls: func(t *testing.T, calls []FunctionCall) {
				if calls[0].ID != "call_empty" {
					t.Errorf("expected ID 'call_empty', got %s", calls[0].ID)
				}
				if calls[0].Name != "kubectl" {
					t.Errorf("expected Name 'kubectl', got %s", calls[0].Name)
				}
				// Arguments should be empty but not nil
				if calls[0].Arguments == nil {
					t.Error("expected non-nil arguments map")
				}
				if len(calls[0].Arguments) != 0 {
					t.Errorf("expected empty arguments, got %v", calls[0].Arguments)
				}
			},
		},
		{
			name: "multiple tool calls with reasoning model pattern",
			toolCalls: []openai.ChatCompletionMessageToolCall{
				{
					ID: "call_1",
					Function: openai.ChatCompletionMessageToolCallFunction{
						Name:      "kubectl",
						Arguments: `{"command":"kubectl get pods --namespace=app-dev01\nkubectl get pods --namespace=app-dev02","modifies_resource":"no"}`,
					},
				},
				{
					ID: "call_2",
					Function: openai.ChatCompletionMessageToolCallFunction{
						Name:      "bash",
						Arguments: `{"command":"echo 'test'","modifies_resource":"no"}`,
					},
				},
			},
			expectedCount:  2,
			expectedResult: true,
			validateCalls: func(t *testing.T, calls []FunctionCall) {
				if len(calls) != 2 {
					t.Errorf("expected 2 calls, got %d", len(calls))
				}
				// Check first call
				if calls[0].Name != "kubectl" {
					t.Errorf("expected first call to be 'kubectl', got %s", calls[0].Name)
				}
				if calls[0].Arguments["command"] != "kubectl get pods --namespace=app-dev01\nkubectl get pods --namespace=app-dev02" {
					t.Errorf("expected multi-line command, got %v", calls[0].Arguments["command"])
				}
				// Check second call
				if calls[1].Name != "bash" {
					t.Errorf("expected second call to be 'bash', got %s", calls[1].Name)
				}
				if calls[1].Arguments["command"] != "echo 'test'" {
					t.Errorf("expected echo command, got %v", calls[1].Arguments["command"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls, ok := convertToolCallsToFunctionCalls(tt.toolCalls)

			if ok != tt.expectedResult {
				t.Errorf("expected result %v, got %v", tt.expectedResult, ok)
			}

			if len(calls) != tt.expectedCount {
				t.Errorf("expected %d calls, got %d", tt.expectedCount, len(calls))
			}

			if tt.validateCalls != nil && len(calls) > 0 {
				tt.validateCalls(t, calls)
			}
		})
	}
}

func TestAddChunkSafely(t *testing.T) {
	tests := []struct {
		name             string
		chunk            openai.ChatCompletionChunk
		expectError      bool
		expectAdded      bool
		setupAccumulator func() *openai.ChatCompletionAccumulator
		unmarshalJSON    bool
		jsonString       string
	}{
		{
			name: "valid chunk with valid accumulator",
			chunk: openai.ChatCompletionChunk{
				Choices: []openai.ChatCompletionChunkChoice{
					{
						Delta: openai.ChatCompletionChunkChoiceDelta{
							Content: "hello",
						},
					},
				},
			},
			expectError: false,
			expectAdded: true,
			setupAccumulator: func() *openai.ChatCompletionAccumulator {
				return &openai.ChatCompletionAccumulator{}
			},
		},
		{
			name:        "chunk with negative tool call index should panic and be recovered",
			expectError: true,
			expectAdded: false,
			setupAccumulator: func() *openai.ChatCompletionAccumulator {
				return &openai.ChatCompletionAccumulator{}
			},
			unmarshalJSON: true,
			jsonString:    "{\"id\":\"gen-1753952129163910909\",\"object\":\"chat.completion.chunk\",\"created\":1753952129,\"model\":\"model\",\"choices\":[{\"index\":0,\"delta\":{\"role\":null,\"content\":null,\"reasoning_content\":null,\"tool_calls\":[{\"id\":\"call_goVJu3KUQtuahQuqW6wgzQ\",\"index\":-1,\"type\":\"function\",\"function\":{\"name\":\"doc_search\",\"arguments\":\"\"}}]},\"logprobs\":null,\"finish_reason\":null,\"matched_stop\":null}],\"usage\":null}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			acc := tt.setupAccumulator()
			chunk := tt.chunk

			if tt.unmarshalJSON {
				err := json.Unmarshal([]byte(tt.jsonString), &chunk)
				if err != nil {
					t.Fatalf("Failed to unmarshal test JSON: %v", err)
				}
			}

			added, err := addChunkSafely(acc, chunk)

			if tt.expectError {
				if err == nil {
					t.Error("expected an error, but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}

			if added != tt.expectAdded {
				t.Errorf("expected added to be %v, but got %v", tt.expectAdded, added)
			}
		})
	}
}

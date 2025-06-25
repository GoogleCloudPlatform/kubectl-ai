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
	"testing"
)

func TestConvertSchemaForOpenAI(t *testing.T) {
	tests := []struct {
		name           string
		inputSchema    *Schema
		expectedType   SchemaType
		expectedError  bool
		validateResult func(t *testing.T, result *Schema)
	}{
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

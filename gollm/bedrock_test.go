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

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// TestBedrockSetFunctionDefinitions_ToolChoiceAuto verifies that
// SetFunctionDefinitions sets ToolChoice to Auto, not Any.
// Any forces a tool call on every turn, preventing natural loop termination.
func TestBedrockSetFunctionDefinitions_ToolChoiceAuto(t *testing.T) {
	tests := []struct {
		name      string
		functions []*FunctionDefinition
	}{
		{
			name: "single function",
			functions: []*FunctionDefinition{
				{
					Name:        "kubectl",
					Description: "Run a kubectl command",
					Parameters: &Schema{
						Type: TypeObject,
						Properties: map[string]*Schema{
							"command": {Type: TypeString, Description: "The kubectl command"},
						},
					},
				},
			},
		},
		{
			name: "multiple functions",
			functions: []*FunctionDefinition{
				{
					Name:        "kubectl",
					Description: "Run a kubectl command",
					Parameters: &Schema{
						Type: TypeObject,
						Properties: map[string]*Schema{
							"command": {Type: TypeString, Description: "The kubectl command"},
						},
					},
				},
				{
					Name:        "bash",
					Description: "Run a bash command",
					Parameters: &Schema{
						Type: TypeObject,
						Properties: map[string]*Schema{
							"command": {Type: TypeString, Description: "The bash command"},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chat := &bedrockChat{}

			if err := chat.SetFunctionDefinitions(tt.functions); err != nil {
				t.Fatalf("SetFunctionDefinitions returned error: %v", err)
			}

			if chat.toolConfig == nil {
				t.Fatal("expected toolConfig to be set")
			}

			if _, ok := chat.toolConfig.ToolChoice.(*types.ToolChoiceMemberAuto); !ok {
				t.Errorf("expected ToolChoice to be *types.ToolChoiceMemberAuto, got %T", chat.toolConfig.ToolChoice)
			}

			if _, ok := chat.toolConfig.ToolChoice.(*types.ToolChoiceMemberAny); ok {
				t.Error("ToolChoice must not be *types.ToolChoiceMemberAny (forces tool call on every turn)")
			}

			if len(chat.toolConfig.Tools) != len(tt.functions) {
				t.Errorf("expected %d tools, got %d", len(tt.functions), len(chat.toolConfig.Tools))
			}
		})
	}
}

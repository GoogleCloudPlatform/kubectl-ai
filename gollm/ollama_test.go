// Copyright 2026 Google LLC
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

	"github.com/ollama/ollama/api"
)

func TestFnDefToOllamaTool(t *testing.T) {
	fnDef := &FunctionDefinition{
		Name:        "kubectl",
		Description: "Run a kubectl command",
		Parameters: &Schema{
			Type:     TypeObject,
			Required: []string{"command"},
			Properties: map[string]*Schema{
				"command": {Type: TypeString, Description: "The kubectl command to run"},
			},
		},
	}

	tool := fnDefToOllamaTool(fnDef)

	if tool.Type != "function" {
		t.Errorf("Type = %q, want %q", tool.Type, "function")
	}
	if tool.Function.Name != "kubectl" {
		t.Errorf("Function.Name = %q, want %q", tool.Function.Name, "kubectl")
	}
	if tool.Function.Parameters.Type != "object" {
		t.Errorf("Parameters.Type = %q, want %q", tool.Function.Parameters.Type, "object")
	}
	if len(tool.Function.Parameters.Required) != 1 || tool.Function.Parameters.Required[0] != "command" {
		t.Errorf("Parameters.Required = %v, want [command]", tool.Function.Parameters.Required)
	}

	prop, ok := tool.Function.Parameters.Properties.Get("command")
	if !ok {
		t.Fatalf("Properties missing %q", "command")
	}
	if len(prop.Type) != 1 || prop.Type[0] != string(TypeString) {
		t.Errorf("command property Type = %v, want [%s]", prop.Type, TypeString)
	}
	if prop.Description != "The kubectl command to run" {
		t.Errorf("command property Description = %q, want %q", prop.Description, "The kubectl command to run")
	}
}

func TestOllamaPartAsFunctionCalls(t *testing.T) {
	args := api.NewToolCallFunctionArguments()
	args.Set("namespace", "default")
	args.Set("replicas", float64(3))

	part := &OllamaPart{
		toolCalls: []api.ToolCall{
			{
				Function: api.ToolCallFunction{
					Name:      "scale_deployment",
					Arguments: args,
				},
			},
		},
	}

	calls, ok := part.AsFunctionCalls()
	if !ok {
		t.Fatalf("AsFunctionCalls returned ok=false")
	}
	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(calls))
	}
	if calls[0].Name != "scale_deployment" {
		t.Errorf("Name = %q, want %q", calls[0].Name, "scale_deployment")
	}
	if calls[0].Arguments["namespace"] != "default" {
		t.Errorf("Arguments[namespace] = %v, want %q", calls[0].Arguments["namespace"], "default")
	}
	if calls[0].Arguments["replicas"] != float64(3) {
		t.Errorf("Arguments[replicas] = %v, want 3", calls[0].Arguments["replicas"])
	}
}

func TestOllamaPartAsFunctionCallsEmpty(t *testing.T) {
	part := &OllamaPart{}

	if _, ok := part.AsFunctionCalls(); ok {
		t.Errorf("AsFunctionCalls returned ok=true for a part with no tool calls")
	}
}

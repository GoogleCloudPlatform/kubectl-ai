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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/ollama/ollama/api"
)

func TestFnDefToOllamaTool(t *testing.T) {
	t.Run("nil parameters", func(t *testing.T) {
		fn := &FunctionDefinition{
			Name:        "ping",
			Description: "Health check",
		}
		tool := fnDefToOllamaTool(fn)
		data, err := json.Marshal(tool)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}
		want := `{"type":"function","function":{"name":"ping","description":"Health check","parameters":{"type":"object","properties":{}}}}`
		if string(data) != want {
			t.Errorf("Marshal mismatch:\ngot:  %s\nwant: %s", string(data), want)
		}
	})

	t.Run("nested object and array parameters", func(t *testing.T) {
		fn := &FunctionDefinition{
			Name:        "kubectl",
			Description: "Run kubectl command",
			Parameters: &Schema{
				Type:     TypeObject,
				Required: []string{"command"},
				Properties: map[string]*Schema{
					"command": {
						Type:        TypeString,
						Description: "kubectl command",
					},
					"flags": {
						Type:        TypeArray,
						Description: "extra flags",
						Items: &Schema{
							Type: TypeString,
						},
					},
					"metadata": {
						Type:     TypeObject,
						Required: []string{"namespace"},
						Properties: map[string]*Schema{
							"namespace": {
								Type:        TypeString,
								Description: "target namespace",
							},
						},
					},
				},
			},
		}

		tool := fnDefToOllamaTool(fn)
		data, err := json.Marshal(tool)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		var got map[string]any
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}

		fnMap := got["function"].(map[string]any)
		params := fnMap["parameters"].(map[string]any)
		props := params["properties"].(map[string]any)

		cmdProp := props["command"].(map[string]any)
		if cmdProp["type"] != "string" || cmdProp["description"] != "kubectl command" {
			t.Errorf("unexpected command property: %#v", cmdProp)
		}

		flagsProp := props["flags"].(map[string]any)
		if flagsProp["type"] != "array" {
			t.Errorf("expected flags type array, got %#v", flagsProp["type"])
		}
		itemsProp := flagsProp["items"].(map[string]any)
		if itemsProp["type"] != "string" {
			t.Errorf("expected items type string, got %#v", itemsProp["type"])
		}

		metaProp := props["metadata"].(map[string]any)
		metaProps := metaProp["properties"].(map[string]any)
		nsProp := metaProps["namespace"].(map[string]any)
		if nsProp["type"] != "string" || nsProp["description"] != "target namespace" {
			t.Errorf("unexpected nested namespace property: %#v", nsProp)
		}
	})
}

func TestOllamaPartAsFunctionCalls(t *testing.T) {
	t.Run("no tool calls", func(t *testing.T) {
		part := &OllamaPart{text: "hello"}
		if calls, ok := part.AsFunctionCalls(); ok || calls != nil {
			t.Fatalf("expected (nil, false), got (%#v, %v)", calls, ok)
		}
	})

	t.Run("unmarshaled tool call arguments", func(t *testing.T) {
		raw := `{"id":"call_123","function":{"name":"kubectl","arguments":{"namespace":"default","replicas":3}}}`
		var tc api.ToolCall
		if err := json.Unmarshal([]byte(raw), &tc); err != nil {
			t.Fatalf("Unmarshal ToolCall failed: %v", err)
		}

		part := &OllamaPart{toolCalls: []api.ToolCall{tc}}
		calls, ok := part.AsFunctionCalls()
		if !ok {
			t.Fatalf("expected ok=true")
		}
		if len(calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(calls))
		}
		if calls[0].ID != "call_123" {
			t.Errorf("expected ID call_123, got %q", calls[0].ID)
		}
		if calls[0].Name != "kubectl" {
			t.Errorf("expected Name kubectl, got %q", calls[0].Name)
		}
		wantArgs := map[string]any{
			"namespace": "default",
			"replicas":  float64(3),
		}
		if !reflect.DeepEqual(calls[0].Arguments, wantArgs) {
			t.Errorf("Arguments mismatch:\ngot:  %#v\nwant: %#v", calls[0].Arguments, wantArgs)
		}
	})
}

func TestOllamaChatToolRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var req api.ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode ChatRequest: %v", err)
		}
		if len(req.Tools) != 1 || req.Tools[0].Function.Name != "get_pods" {
			t.Errorf("expected tool get_pods in request, got %+v", req.Tools)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"gemma3:latest","message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","function":{"name":"get_pods","arguments":{"namespace":"kube-system"}}}]},"done":true}`))
	}))
	defer srv.Close()

	t.Setenv("OLLAMA_HOST", srv.URL)
	client, err := NewOllamaClient(context.Background(), ClientOptions{})
	if err != nil {
		t.Fatalf("NewOllamaClient failed: %v", err)
	}

	chat := client.StartChat("You are helpful", "gemma3:latest")
	err = chat.SetFunctionDefinitions([]*FunctionDefinition{
		{
			Name:        "get_pods",
			Description: "List pods in namespace",
			Parameters: &Schema{
				Type:     TypeObject,
				Required: []string{"namespace"},
				Properties: map[string]*Schema{
					"namespace": {Type: TypeString, Description: "Namespace"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("SetFunctionDefinitions failed: %v", err)
	}

	resp, err := chat.Send(context.Background(), "list kube-system pods")
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	candidates := resp.Candidates()
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	parts := candidates[0].Parts()
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}
	calls, ok := parts[0].AsFunctionCalls()
	if !ok || len(calls) != 1 {
		t.Fatalf("expected 1 function call, got (%+v, %v)", calls, ok)
	}
	if calls[0].Name != "get_pods" || calls[0].Arguments["namespace"] != "kube-system" {
		t.Errorf("unexpected function call: %+v", calls[0])
	}
}

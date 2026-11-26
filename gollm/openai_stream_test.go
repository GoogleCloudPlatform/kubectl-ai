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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const toolCallArgs = `{\"command\":\"kubectl get nodes\"}`

// serveChunks starts a server that answers every chat completion request with the
// given SSE data payloads.
func serveChunks(t *testing.T, chunks ...string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, chunk := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)
	return server
}

// streamFunctionCalls sends one streaming request and returns the text and function
// calls the iterator yields, plus the tool calls recorded in the chat history.
func streamFunctionCalls(t *testing.T, server *httptest.Server) (string, []FunctionCall, int) {
	t.Helper()
	client := &OpenAIClient{client: openai.NewClient(
		option.WithBaseURL(server.URL),
		option.WithAPIKey("test-key"),
	)}
	chat := client.StartChat("", "test-model")
	iterator, err := chat.SendStreaming(context.Background(), "How many nodes are in the cluster?")
	if err != nil {
		t.Fatalf("SendStreaming: %v", err)
	}

	var text strings.Builder
	var calls []FunctionCall
	for response, err := range iterator {
		if err != nil {
			t.Fatalf("stream error: %v", err)
		}
		for _, candidate := range response.Candidates() {
			for _, part := range candidate.Parts() {
				if s, ok := part.AsText(); ok {
					text.WriteString(s)
				}
				if c, ok := part.AsFunctionCalls(); ok {
					calls = append(calls, c...)
				}
			}
		}
	}

	session := chat.(*openAIChatSession)
	last := session.history[len(session.history)-1]
	return text.String(), calls, len(last.OfAssistant.ToolCalls)
}

func TestSendStreamingToolCalls(t *testing.T) {
	finish := `{"id":"c","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`
	tests := []struct {
		name          string
		chunks        []string
		wantCalls     int
		wantText      string
		wantHistCalls int
	}{
		{
			name: "complete tool call with empty content in one chunk",
			chunks: []string{
				`{"id":"c","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{"role":"assistant","content":"","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"kubectl","arguments":"` + toolCallArgs + `"}}]}}]}`,
				finish,
			},
			wantCalls:     1,
			wantHistCalls: 1,
		},
		{
			name: "tool call streamed in fragments",
			chunks: []string{
				`{"id":"c","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"kubectl","arguments":""}}]}}]}`,
				`{"id":"c","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"command\":"}}]}}]}`,
				`{"id":"c","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"kubectl get nodes\"}"}}]}}]}`,
				finish,
			},
			wantCalls:     1,
			wantHistCalls: 1,
		},
		{
			name: "text only",
			chunks: []string{
				`{"id":"c","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{"role":"assistant","content":"There is 1 node."}}]}`,
				`{"id":"c","object":"chat.completion.chunk","model":"m","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			},
			wantText: "There is 1 node.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, calls, histCalls := streamFunctionCalls(t, serveChunks(t, tt.chunks...))
			if text != tt.wantText {
				t.Errorf("text = %q, want %q", text, tt.wantText)
			}
			if len(calls) != tt.wantCalls {
				t.Fatalf("got %d function calls, want %d: %+v", len(calls), tt.wantCalls, calls)
			}
			if histCalls != tt.wantHistCalls {
				t.Errorf("history has %d tool calls, want %d", histCalls, tt.wantHistCalls)
			}
			for _, call := range calls {
				if call.ID != "call_1" || call.Name != "kubectl" || call.Arguments["command"] != "kubectl get nodes" {
					t.Errorf("unexpected function call: %+v", call)
				}
			}
		})
	}
}

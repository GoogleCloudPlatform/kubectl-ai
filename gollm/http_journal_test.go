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
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/journal"
)

// fakeRoundTripper returns a canned response without making a real network call.
type fakeRoundTripper struct{}

func (f *fakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: 200,
		Status:     "200 OK",
		Header: http.Header{
			"Content-Type": []string{"application/json"},
			"Set-Cookie":   []string{"session=super-secret-session-cookie"},
			"X-Request-Id": []string{"abc-123"},
		},
		Body: io.NopCloser(strings.NewReader(`{"ok":true}`)),
	}, nil
}

// capturingRecorder collects every event written to it, for assertions.
type capturingRecorder struct {
	events []*journal.Event
}

func (c *capturingRecorder) Write(_ context.Context, event *journal.Event) error {
	c.events = append(c.events, event)
	return nil
}

func (c *capturingRecorder) Close() error { return nil }

func TestJournalingRoundTripper_RedactsSensitiveRequestHeaders(t *testing.T) {
	recorder := &capturingRecorder{}
	ctx := journal.ContextWithRecorder(context.Background(), recorder)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://generativelanguage.googleapis.com/v1/models", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Header.Set("X-Goog-Api-Key", "AIzaSyTHIS-IS-A-SECRET-GEMINI-KEY")
	req.Header.Set("Authorization", "Bearer sk-THIS-IS-A-SECRET-OPENAI-KEY")
	req.Header.Set("X-Api-Key", "sk-ant-THIS-IS-A-SECRET-ANTHROPIC-KEY")
	req.Header.Set("Api-Key", "THIS-IS-A-SECRET-AZURE-KEY")
	req.Header.Set("Content-Type", "application/json")

	rt := &journalingRoundTripper{next: &fakeRoundTripper{}}
	if _, err := rt.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip returned error: %v", err)
	}

	var requestDump string
	for _, e := range recorder.events {
		if e.Action == journal.ActionHTTPRequest {
			requestDump, _ = e.GetString("request")
		}
	}
	if requestDump == "" {
		t.Fatal("no http.request event captured")
	}

	for _, secret := range []string{
		"AIzaSyTHIS-IS-A-SECRET-GEMINI-KEY",
		"sk-THIS-IS-A-SECRET-OPENAI-KEY",
		"sk-ant-THIS-IS-A-SECRET-ANTHROPIC-KEY",
		"THIS-IS-A-SECRET-AZURE-KEY",
	} {
		if strings.Contains(requestDump, secret) {
			t.Errorf("recorded request dump leaked secret %q:\n%s", secret, requestDump)
		}
	}

	// Non-sensitive headers must still be visible, so the trace stays useful.
	if !strings.Contains(requestDump, "application/json") {
		t.Errorf("recorded request dump lost a non-sensitive header:\n%s", requestDump)
	}
}

func TestJournalingRoundTripper_RedactsSensitiveResponseHeaders(t *testing.T) {
	recorder := &capturingRecorder{}
	ctx := journal.ContextWithRecorder(context.Background(), recorder)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://example.com", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("building request: %v", err)
	}

	rt := &journalingRoundTripper{next: &fakeRoundTripper{}}
	if _, err := rt.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip returned error: %v", err)
	}

	var payload map[string]any
	for _, e := range recorder.events {
		if e.Action == journal.ActionHTTPResponse {
			p, ok := e.Payload.(map[string]any)
			if !ok {
				t.Fatalf("unexpected payload type: %T", e.Payload)
			}
			payload = p
		}
	}
	if payload == nil {
		t.Fatal("no http.response event captured")
	}

	headers, ok := payload["headers"].(http.Header)
	if !ok {
		t.Fatalf("unexpected headers type: %T", payload["headers"])
	}
	if got := headers.Get("Set-Cookie"); strings.Contains(got, "super-secret-session-cookie") {
		t.Errorf("recorded response headers leaked Set-Cookie value: %q", got)
	}
	if got := headers.Get("X-Request-Id"); got != "abc-123" {
		t.Errorf("recorded response headers lost a non-sensitive header, got %q", got)
	}
}

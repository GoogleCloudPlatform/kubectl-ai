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
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/journal"
)

type captureRecorder struct {
	events []*journal.Event
}

func (r *captureRecorder) Close() error { return nil }

func (r *captureRecorder) Write(_ context.Context, event *journal.Event) error {
	r.events = append(r.events, event)
	return nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestJournalingRoundTripperRedactsSensitiveHeaders(t *testing.T) {
	recorder := &captureRecorder{}
	rt := &journalingRoundTripper{next: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("Authorization"); got != "Bearer request-token" {
			t.Fatalf("request header was modified: %q", got)
		}
		return &http.Response{
			Status:     "200 OK",
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Set-Cookie": []string{"session=response-secret"},
				"X-Trace-Id": []string{"trace-1"},
			},
			Body: io.NopCloser(strings.NewReader("ok")),
		}, nil
	})}

	req, err := http.NewRequestWithContext(
		journal.ContextWithRecorder(context.Background(), recorder),
		http.MethodPost,
		"https://example.com",
		strings.NewReader("request body"),
	)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer request-token")
	req.Header.Set("X-Api-Key", "request-key")
	req.Header.Set("X-Trace-ID", "trace-1")

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if len(recorder.events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(recorder.events))
	}
	request, _ := recorder.events[0].GetString("request")
	if strings.Contains(request, "request-token") || strings.Contains(request, "request-key") {
		t.Fatalf("request trace leaked sensitive header: %s", request)
	}
	if !strings.Contains(request, "Authorization: [REDACTED]") ||
		!strings.Contains(request, "X-Api-Key: [REDACTED]") ||
		!strings.Contains(request, "X-Trace-Id: trace-1") {
		t.Fatalf("request trace did not redact headers as expected: %s", request)
	}

	payload := recorder.events[1].Payload.(map[string]any)
	headers := payload["headers"].(http.Header)
	if got := headers.Get("Set-Cookie"); got != "[REDACTED]" {
		t.Fatalf("Set-Cookie was not redacted: %q", got)
	}
	if got := headers.Get("X-Trace-Id"); got != "trace-1" {
		t.Fatalf("non-sensitive response header was changed: %q", got)
	}
}

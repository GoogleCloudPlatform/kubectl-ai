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
	"strings"
	"testing"
)

func TestNewClient(t *testing.T) {
	_, err := NewClient(context.Background(), "gemini")
	if err == nil || err.Error() != "GEMINI_API_KEY environment variable not set" {
		t.Fatalf("Unexpected error: %v", err)
	}

	_, err = NewClient(context.Background(), "invalid")
	if err == nil || !strings.Contains(err.Error(), "provider \"invalid\" not registered") {
		t.Fatalf("Unexpected error: %v", err)
	}
}

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

package tools

import "testing"

func TestAddDefaultTailForLogs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "kubectl logs without tail",
			input:    "kubectl logs my-pod",
			expected: "kubectl logs my-pod --tail=500",
		},
		{
			name:     "kubectl logs already has tail",
			input:    "kubectl logs my-pod --tail=100",
			expected: "kubectl logs my-pod --tail=100",
		},
		{
			name:     "kubectl logs with --since",
			input:    "kubectl logs my-pod --since=1h",
			expected: "kubectl logs my-pod --since=1h",
		},
		{
			name:     "kubectl logs -f (streaming)",
			input:    "kubectl logs my-pod -f",
			expected: "kubectl logs my-pod -f --tail=500",
		},
		{
			name:     "not a logs command",
			input:    "kubectl get pods",
			expected: "kubectl get pods",
		},
		{
			name:     "logs with namespace",
			input:    "kubectl logs my-pod -n production",
			expected: "kubectl logs my-pod -n production --tail=500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := addDefaultTailForLogs(tt.input)
			if result != tt.expected {
				t.Errorf("addDefaultTailForLogs(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

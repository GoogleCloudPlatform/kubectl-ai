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

func TestEnsureKubectlPrefix(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "already has kubectl prefix",
			input:    "kubectl get pods",
			expected: "kubectl get pods",
		},
		{
			name:     "missing kubectl prefix",
			input:    "get pods",
			expected: "kubectl get pods",
		},
		{
			name:     "missing prefix with flags",
			input:    "get pods -n default",
			expected: "kubectl get pods -n default",
		},
		{
			name:     "missing prefix describe",
			input:    "describe pod my-pod",
			expected: "kubectl describe pod my-pod",
		},
		{
			name:     "already has prefix with namespace",
			input:    "kubectl get pods -n kube-system",
			expected: "kubectl get pods -n kube-system",
		},
		{
			name:     "pipe command left unchanged",
			input:    "get pods | grep Running",
			expected: "get pods | grep Running",
		},
		{
			name:     "kubectl with pipe left unchanged",
			input:    "kubectl get pods | grep Running",
			expected: "kubectl get pods | grep Running",
		},
		{
			name:     "just kubectl",
			input:    "kubectl",
			expected: "kubectl",
		},
		{
			name:     "apply with file",
			input:    "apply -f deployment.yaml",
			expected: "kubectl apply -f deployment.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ensureKubectlPrefix(tt.input)
			if result != tt.expected {
				t.Errorf("ensureKubectlPrefix(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

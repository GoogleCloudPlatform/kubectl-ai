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

package sessions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeEnvironment(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "default"},
		{"   ", "default"},
		{"prod", "prod"},
		{"PROD-CLUSTER", "prod-cluster"},
		{"gke_proj_us-central1_cluster-1", "gke_proj_us-central1_cluster-1"},
		{"kind/my-cluster:dev", "kind-my-cluster-dev"},
		{"---special@@characters---", "special-characters"},
	}

	for _, tt := range tests {
		got := SanitizeEnvironment(tt.input)
		if got != tt.expected {
			t.Errorf("SanitizeEnvironment(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}

func TestResolveEnvironment(t *testing.T) {
	t.Run("explicit value takes priority", func(t *testing.T) {
		got := ResolveEnvironment("staging-east", "/nonexistent/kubeconfig")
		if got != "staging-east" {
			t.Errorf("ResolveEnvironment with explicit value = %q; want %q", got, "staging-east")
		}
	})

	t.Run("detects context from valid kubeconfig", func(t *testing.T) {
		tmpDir := t.TempDir()
		kcPath := filepath.Join(tmpDir, "kubeconfig")
		content := `apiVersion: v1
clusters:
- cluster:
    server: https://127.0.0.1:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: gke_my-project_us-east1_prod-cluster
current-context: gke_my-project_us-east1_prod-cluster
kind: Config
preferences: {}
users:
- name: test-user
  user:
    token: fake-token
`
		if err := os.WriteFile(kcPath, []byte(content), 0o600); err != nil {
			t.Fatalf("failed to write mock kubeconfig: %v", err)
		}

		got := ResolveEnvironment("", kcPath)
		want := "gke_my-project_us-east1_prod-cluster"
		if got != want {
			t.Errorf("ResolveEnvironment from kubeconfig = %q; want %q", got, want)
		}
	})

	t.Run("fallback to default when no kubeconfig or context", func(t *testing.T) {
		got := ResolveEnvironment("", "/nonexistent/kubeconfig")
		if got != "default" {
			t.Errorf("ResolveEnvironment fallback = %q; want %q", got, "default")
		}
	})
}

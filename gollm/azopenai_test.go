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

import "testing"

func TestAzureOpenAIDeploymentName(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  string
	}{
		{
			name:  "plain deployment",
			model: "gpt-4.1",
			want:  "gpt-4.1",
		},
		{
			name:  "azure prefixed deployment",
			model: "azure/gpt-4.1",
			want:  "gpt-4.1",
		},
		{
			name:  "azopenai prefixed deployment",
			model: "azopenai/gpt-4.1",
			want:  "gpt-4.1",
		},
		{
			name:  "empty model",
			model: "",
			want:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := azureOpenAIDeploymentName(tc.model); got != tc.want {
				t.Fatalf("azureOpenAIDeploymentName(%q) = %q, want %q", tc.model, got, tc.want)
			}
		})
	}
}

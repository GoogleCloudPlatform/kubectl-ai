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

package html

import (
	"bytes"
	"context"
	"testing"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/ui"
)

// TestLoadTemplate tests the LoadTemplate function to ensure it correctly handles
// various template paths.
func TestLoadTemplate(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{
			name:    "valid template",
			key:     "templates/index.html",
			wantErr: false,
		},
		{
			name:    "non-existent template",
			key:     "templates/nonexistent.html",
			wantErr: true,
		},
		{
			name:    "component template",
			key:     "templates/components/test.html",
			wantErr: false,
		},
		{
			name:    "invalid path",
			key:     "../invalid/path.html",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := loadTemplate(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tmpl == nil {
				t.Errorf("LoadTemplate() returned nil template but no error")
			}
		})
	}
}

func TestRenderTemplat(t *testing.T) {

	tests := []struct {
		name    string
		key     string
		data    any
		wantErr bool
	}{
		{
			name:    "agent text block with test template",
			key:     "templates/components/test.html",
			data:    ui.NewAgentTextBlock().WithText("Test1"),
			wantErr: false,
		},
		{
			name:    "agent text block with agent template",
			key:     "templates/components/agent_text_block.html",
			data:    ui.NewAgentTextBlock().WithText("Test1"),
			wantErr: false,
		},
		{
			name:    "error block with test template",
			key:     "templates/components/test.html",
			data:    ui.NewErrorBlock().WithText("Errors!"),
			wantErr: false,
		},
		{
			name:    "error text block with error template",
			key:     "templates/components/error_block.html",
			data:    ui.NewAgentTextBlock().WithText("Test1"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			var buf bytes.Buffer
			err := renderTemplate(ctx, &buf, tt.key, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("renderTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && buf.Len() == 0 {
				t.Errorf("renderTemplate() produced no output")
			}
		})
	}
}

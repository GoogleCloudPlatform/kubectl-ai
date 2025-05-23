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
	"context"
	"embed"
	"fmt"
	"html/template"
	"io"

	"k8s.io/klog/v2"
)

//go:embed templates/*.html templates/components/*.html
var htmlFiles embed.FS

// loadTemplate loads and parses an HTML template from embedded filesystem.
// Ensure that the corresponding template existing in the templates directory
// or in the components subdirectory.
//
// Parameters:
//   - key: The file path of the template to load from the embedded filesystem.
//
// Returns:
//   - *template.Template: The parsed template if successful.
//   - error: An error if the template cannot be read or parsed.
func loadTemplate(key string) (*template.Template, error) {
	// TODO: Caching
	b, err := htmlFiles.ReadFile(key)
	if err != nil {
		return nil, fmt.Errorf("reading %q: %w", key, err)
	}

	tmpl, err := template.New(key).Parse(string(b))
	if err != nil {
		return nil, fmt.Errorf("parsing %q: %w", key, err)
	}
	return tmpl, nil
}

func renderTemplate(ctx context.Context, w io.Writer, key string, data any) error {
	log := klog.FromContext(ctx)
	tmpl, err := loadTemplate(key)
	if err != nil {
		return fmt.Errorf("loading template %q: %w", key, err)
	}
	if err := tmpl.Execute(w, data); err != nil {
		return fmt.Errorf("executing %q: %w", key, err)
	}

	log.Info("rendered page", "key", key)
	return nil
}

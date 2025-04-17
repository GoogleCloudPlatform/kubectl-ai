package templates

import (
	"embed"
	"fmt"
	"html/template"
)

//go:embed *.html
var htmlFiles embed.FS

func LoadTemplate(key string) (*template.Template, error) {
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

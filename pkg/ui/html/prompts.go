package html

import (
	_ "embed"

	"gopkg.in/yaml.v3"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
)

//go:embed prompts.yaml
var defaultPromptsYAML []byte

// loadPromptLibrary parses the embedded YAML. Later we can extend to merge
// user-supplied files.
func loadPromptLibrary() ([]api.PromptGroup, error) {
	var groups []api.PromptGroup
	if err := yaml.Unmarshal(defaultPromptsYAML, &groups); err != nil {
		return nil, err
	}
	return groups, nil
}

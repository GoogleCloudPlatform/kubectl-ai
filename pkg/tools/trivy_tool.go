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

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
)

func init() {
	RegisterTool(&ScanImageWithTrivy{})
}

type ScanImageWithTrivy struct {
	// Image is the image to scan
	Image string `json:"image,omitempty"`
}

func (t *ScanImageWithTrivy) Name() string {
	return "scan_image_with_trivy"
}

func (t *ScanImageWithTrivy) Description() string {
	return "Scans a container image for vulnerabilities, using the trivy tool."
}

func (t *ScanImageWithTrivy) FunctionDefinition() *gollm.FunctionDefinition {
	return &gollm.FunctionDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: &gollm.Schema{
			Type: gollm.TypeObject,
			Properties: map[string]*gollm.Schema{
				"image": {
					Type:        gollm.TypeString,
					Description: `The name of the container image to scan.`,
				},
			},
			Required: []string{"image"},
		},
	}
}

func (t *ScanImageWithTrivy) Run(ctx context.Context, functionArgs map[string]any) (any, error) {
	workDir := ctx.Value(WorkDirKey).(string)

	if err := parseFunctionArgs(functionArgs, t); err != nil {
		return nil, err
	}

	if t.Image == "" {
		return nil, fmt.Errorf("image is required")
	}

	args := []string{"trivy", "image", t.Image}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = workDir
	cmd.Env = os.Environ()

	return executeCommand(cmd)
}

func parseFunctionArgs(functionArgs map[string]any, task any) error {
	j, err := json.Marshal(functionArgs)
	if err != nil {
		return fmt.Errorf("converting function parameters to json: %w", err)
	}
	if err := json.Unmarshal(j, task); err != nil {
		return fmt.Errorf("parsing function parameters into %T: %w", task, err)
	}
	return nil
}

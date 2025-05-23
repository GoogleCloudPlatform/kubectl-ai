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
	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/mcp"
)

// convertMCPSchemaToGollm converts an MCP tool schema to gollm.FunctionDefinition
func convertMCPSchemaToGollm(mcpTool *mcp.Tool) (*gollm.FunctionDefinition, error) {
	// Since MCP Tool doesn't have schema info in the current implementation,
	// we create a simple schema that accepts any parameters
	return &gollm.FunctionDefinition{
		Name:        mcpTool.Name,
		Description: mcpTool.Description,
		Parameters: &gollm.Schema{
			Type:       gollm.TypeObject,
			Properties: make(map[string]*gollm.Schema),
		},
	}, nil
}

// Note: The current MCP Tool struct only has Name and Description fields.
// Schema conversion could be enhanced in the future when MCP tools include schema information.

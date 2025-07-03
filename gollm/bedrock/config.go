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

package bedrock

import (
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
)

const (
	Name = "bedrock"

	ErrMsgConfigLoad       = "failed to load AWS configuration"
	ErrMsgModelInvoke      = "failed to invoke Bedrock model"
	ErrMsgResponseParse    = "failed to parse Bedrock response"
	ErrMsgRequestBuild     = "failed to build request"
	ErrMsgStreamingFailed  = "Bedrock streaming failed"
	ErrMsgUnsupportedModel = "unsupported model - only Claude and Nova models are supported"
)

type BedrockOptions struct {
	Region              string
	CredentialsProvider aws.CredentialsProvider
	Model               string
	MaxTokens           int32
	Temperature         float32
	TopP                float32
	Timeout             time.Duration
	MaxRetries          int
}

var DefaultOptions = &BedrockOptions{
	Region:      "us-west-2",
	Model:       "us.anthropic.claude-sonnet-4-20250514-v1:0",
	MaxTokens:   64000,
	Temperature: 0.1,
	TopP:        0.9,
	Timeout:     30 * time.Second,
	MaxRetries:  10,
}

// supportedModelsByRegion defines the available models for each AWS region
// This allows for region-specific model availability and easier maintenance
var supportedModelsByRegion = map[string][]string{
	"us-east-1": {
		"us.anthropic.claude-sonnet-4-20250514-v1:0",
		"us.anthropic.claude-3-7-sonnet-20250219-v1:0",
		"us.amazon.nova-pro-v1:0",
		"us.amazon.nova-lite-v1:0",
		"us.amazon.nova-micro-v1:0",
		"anthropic.claude-v2:1",
		"anthropic.claude-instant-v1",
		"amazon.nova-pro-v1:0",
		"mistral.mistral-large-2402-v1:0",
	},
	"us-west-2": {
		"us.anthropic.claude-sonnet-4-20250514-v1:0",
		"us.anthropic.claude-3-7-sonnet-20250219-v1:0",
		"us.amazon.nova-pro-v1:0",
		"us.amazon.nova-lite-v1:0",
		"us.amazon.nova-micro-v1:0",
		"anthropic.claude-v2:1",
		"amazon.nova-pro-v1:0",
		"stability.sd3-large-v1:0",
	},
	"eu-west-1": {
		"anthropic.claude-v2:1",
		"anthropic.claude-instant-v1",
		"amazon.nova-pro-v1:0",
		"amazon.nova-lite-v1:0",
		"amazon.nova-micro-v1:0",
	},
	"eu-central-1": {
		"anthropic.claude-v2:1",
		"anthropic.claude-instant-v1",
		"amazon.nova-pro-v1:0",
		"amazon.nova-lite-v1:0",
		"amazon.nova-micro-v1:0",
	},
	"ap-southeast-1": {
		"anthropic.claude-v2:1",
		"anthropic.claude-instant-v1",
		"amazon.nova-pro-v1:0",
		"amazon.nova-lite-v1:0",
		"amazon.nova-micro-v1:0",
	},
	"ap-northeast-1": {
		"anthropic.claude-v2:1",
		"anthropic.claude-instant-v1",
		"amazon.nova-pro-v1:0",
		"amazon.nova-lite-v1:0",
		"amazon.nova-micro-v1:0",
	},
}

// isModelSupported checks if the given model is supported in the specified region
func isModelSupported(model string) bool {
	return isModelSupportedInRegion(model, "")
}

// isModelSupportedInRegion checks if the given model is supported in the specified region
func isModelSupportedInRegion(model, region string) bool {
	if model == "" {
		return false
	}

	modelLower := strings.ToLower(model)

	// If region is specified, check region-specific models first
	if region != "" {
		if models, exists := supportedModelsByRegion[region]; exists {
			for _, supported := range models {
				if modelLower == strings.ToLower(supported) {
					return true
				}
			}
		}
	}

	// Fallback: check all regions if no region specified or model not found in specified region
	for _, models := range supportedModelsByRegion {
		for _, supported := range models {
			if modelLower == strings.ToLower(supported) {
				return true
			}
		}
	}

	// Handle special cases (ARNs and inference profiles)
	if strings.Contains(modelLower, "arn:aws:bedrock") {
		if strings.Contains(modelLower, "inference-profile") {
			if strings.Contains(modelLower, "anthropic") || strings.Contains(modelLower, "claude") {
				return true
			}

			if strings.Contains(modelLower, "amazon") || strings.Contains(modelLower, "nova") {
				return true
			}

			return true
		}

		if strings.Contains(modelLower, "foundation-model") {
			parts := strings.Split(model, "/")
			if len(parts) > 0 {
				extractedModel := parts[len(parts)-1]
				return isModelSupportedInRegion(extractedModel, region)
			}
		}
	}

	return false
}

// getSupportedModels returns all supported models across all regions
func getSupportedModels() []string {
	return getSupportedModelsForRegion("")
}

// getSupportedModelsForRegion returns supported models for the specified region
// If region is empty, returns all models across all regions
func getSupportedModelsForRegion(region string) []string {
	if region != "" {
		if models, exists := supportedModelsByRegion[region]; exists {
			// Return a copy to avoid external modification
			result := make([]string, len(models))
			copy(result, models)
			return result
		}
		return []string{} // Return empty slice if region not found
	}

	// Return all models across all regions (with deduplication)
	modelSet := make(map[string]bool)
	for _, models := range supportedModelsByRegion {
		for _, model := range models {
			modelSet[model] = true
		}
	}

	var result []string
	for model := range modelSet {
		result = append(result, model)
	}

	return result
}

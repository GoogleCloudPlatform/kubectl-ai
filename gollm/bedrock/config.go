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

// isModelSupported checks if the given model is supported
func isModelSupported(model string) bool {
	if model == "" {
		return false
	}

	modelLower := strings.ToLower(model)

	supportedModels := []string{
		"us.anthropic.claude-sonnet-4-20250514-v1:0",
		"us.anthropic.claude-3-7-sonnet-20250219-v1:0",
		"us.amazon.nova-pro-v1:0",
		"us.amazon.nova-lite-v1:0",
		"us.amazon.nova-micro-v1:0",
	}

	for _, supported := range supportedModels {
		if modelLower == strings.ToLower(supported) {
			return true
		}
	}

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
				return isModelSupported(extractedModel)
			}
		}
	}

	return false
}

func getSupportedModels() []string {
	return []string{
		"us.anthropic.claude-sonnet-4-20250514-v1:0",
		"us.anthropic.claude-3-7-sonnet-20250219-v1:0",
		"us.amazon.nova-pro-v1:0",
		"us.amazon.nova-lite-v1:0",
		"us.amazon.nova-micro-v1:0",
	}
}

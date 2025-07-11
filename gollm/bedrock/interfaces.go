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
	"errors"
	"time"
)

// ConfigurableClient provides advanced Bedrock-specific configuration
// This interface extends the basic gollm.Client with Bedrock-specific features
// that can be accessed via type assertion without polluting global interfaces.
type ConfigurableClient interface {
	// SetInferenceConfig configures Bedrock-specific inference parameters
	SetInferenceConfig(config *InferenceConfig) error

	// GetInferenceConfig returns the current inference configuration
	GetInferenceConfig() *InferenceConfig

	// SetUsageCallback configures a callback for usage tracking
	SetUsageCallback(callback UsageCallback)
}

// InferenceConfig contains Bedrock-specific inference parameters
// This replaces the global InferenceConfig that was polluting the interface
type InferenceConfig struct {
	// Model configuration
	Model  string `json:"model,omitempty" yaml:"model,omitempty"`
	Region string `json:"region,omitempty" yaml:"region,omitempty"`

	// Generation parameters
	Temperature float32 `json:"temperature,omitempty" yaml:"temperature,omitempty"`
	MaxTokens   int32   `json:"maxTokens,omitempty" yaml:"maxTokens,omitempty"`
	TopP        float32 `json:"topP,omitempty" yaml:"topP,omitempty"`
	TopK        int32   `json:"topK,omitempty" yaml:"topK,omitempty"`

	// Retry configuration
	MaxRetries int `json:"maxRetries,omitempty" yaml:"maxRetries,omitempty"`
}

// Validate checks if inference config has valid parameters
func (c *InferenceConfig) Validate() error {
	// Check parameter ranges for Bedrock-specific constraints
	if c.Temperature < 0 || c.Temperature > 2.0 {
		return errors.New("temperature must be between 0.0 and 2.0")
	}
	if c.MaxTokens < 0 {
		return errors.New("maxTokens must be non-negative")
	}
	if c.TopP < 0 || c.TopP > 1.0 {
		return errors.New("topP must be between 0.0 and 1.0")
	}
	if c.TopK < 0 {
		return errors.New("topK must be non-negative")
	}
	if c.MaxRetries < 0 {
		return errors.New("maxRetries must be non-negative")
	}
	return nil
}

// IsValid validates that InferenceConfig has reasonable parameter values
// Returns true if all parameters are within valid ranges
func (c *InferenceConfig) IsValid() bool {
	return c.Validate() == nil
}

// UsageCallback is called when usage data is available for Bedrock
// This provides structured usage tracking specific to Bedrock's capabilities
type UsageCallback func(provider, model string, usage Usage)

// Usage represents Bedrock-specific token usage information
// This extends the basic gollm.Usage with Bedrock-specific fields
type Usage struct {
	// Token usage information
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
	TotalTokens  int `json:"totalTokens"`

	// Cost information (in USD) - Bedrock-specific pricing
	InputCost  float64 `json:"inputCost,omitempty"`
	OutputCost float64 `json:"outputCost,omitempty"`
	TotalCost  float64 `json:"totalCost,omitempty"`

	// Metadata
	Model     string    `json:"model"`
	Provider  string    `json:"provider"`
	Timestamp time.Time `json:"timestamp"`
}

// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gollm

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhase1IntegrationDemo(t *testing.T) {
	// Test demonstrates the complete Phase 1 implementation:
	// 1. Provider-agnostic types (Usage, InferenceConfig)
	// 2. Enhanced ClientOptions with functional options
	// 3. Usage callbacks and extractors
	// 4. Full backwards compatibility

	t.Run("BackwardsCompatibility", func(t *testing.T) {
		// Existing usage patterns continue to work unchanged
		opts := ClientOptions{
			SkipVerifySSL: true,
		}

		assert.True(t, opts.SkipVerifySSL)
		assert.Nil(t, opts.InferenceConfig)
		assert.Nil(t, opts.UsageCallback)
		assert.Nil(t, opts.UsageExtractor)
		assert.False(t, opts.Debug)
	})

	t.Run("EnhancedClientOptions", func(t *testing.T) {
		// New enhanced usage with all capabilities
		var capturedUsage []Usage
		var capturedCosts []float64

		callback := func(provider, model string, usage Usage) {
			capturedUsage = append(capturedUsage, usage)
			capturedCosts = append(capturedCosts, usage.TotalCost)
		}

		config := &InferenceConfig{
			Model:       "claude-3-sonnet",
			Region:      "us-west-2",
			Temperature: 0.7,
			MaxTokens:   4000,
			TopP:        0.9,
			MaxRetries:  3,
		}

		extractor := &mockUsageExtractor{}

		// Build options using new functional pattern
		opts := ClientOptions{}
		WithInferenceConfig(config)(&opts)
		WithUsageCallback(callback)(&opts)
		WithUsageExtractor(extractor)(&opts)
		WithDebug(true)(&opts)

		// Verify all options are set correctly
		assert.Equal(t, config, opts.InferenceConfig)
		assert.NotNil(t, opts.UsageCallback)
		assert.Equal(t, extractor, opts.UsageExtractor)
		assert.True(t, opts.Debug)

		// Test the callback functionality
		testUsage := Usage{
			InputTokens:  100,
			OutputTokens: 50,
			TotalTokens:  150,
			TotalCost:    0.005,
			Model:        "claude-3-sonnet",
			Provider:     "bedrock",
			Timestamp:    time.Now(),
		}

		opts.UsageCallback("bedrock", "claude-3-sonnet", testUsage)

		require.Len(t, capturedUsage, 1)
		assert.Equal(t, testUsage, capturedUsage[0])
		assert.Equal(t, 0.005, capturedCosts[0])
	})

	t.Run("UsageAggregationScenario", func(t *testing.T) {
		// Demonstrate real-world usage aggregation scenario
		var totalCost float64
		var totalInputTokens, totalOutputTokens int
		var callCount int
		var modelUsage = make(map[string]Usage)

		// Aggregator callback that tracks totals and per-model usage
		aggregator := func(provider, model string, usage Usage) {
			callCount++
			totalCost += usage.TotalCost
			totalInputTokens += usage.InputTokens
			totalOutputTokens += usage.OutputTokens

			// Track per-model usage
			key := provider + ":" + model
			existing := modelUsage[key]
			modelUsage[key] = Usage{
				InputTokens:  existing.InputTokens + usage.InputTokens,
				OutputTokens: existing.OutputTokens + usage.OutputTokens,
				TotalTokens:  existing.TotalTokens + usage.TotalTokens,
				TotalCost:    existing.TotalCost + usage.TotalCost,
				Model:        model,
				Provider:     provider,
			}
		}

		// Simulate multiple model calls across different providers and models
		calls := []struct {
			provider string
			model    string
			usage    Usage
		}{
			{
				provider: "bedrock",
				model:    "claude-3-sonnet",
				usage:    Usage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150, TotalCost: 0.003},
			},
			{
				provider: "bedrock",
				model:    "claude-3-sonnet",
				usage:    Usage{InputTokens: 200, OutputTokens: 100, TotalTokens: 300, TotalCost: 0.006},
			},
			{
				provider: "bedrock",
				model:    "nova-pro",
				usage:    Usage{InputTokens: 150, OutputTokens: 75, TotalTokens: 225, TotalCost: 0.002},
			},
			{
				provider: "openai",
				model:    "gpt-4",
				usage:    Usage{InputTokens: 80, OutputTokens: 40, TotalTokens: 120, TotalCost: 0.004},
			},
		}

		for _, call := range calls {
			aggregator(call.provider, call.model, call.usage)
		}

		// Verify aggregation results
		assert.Equal(t, 4, callCount)
		assert.InDelta(t, 0.015, totalCost, 0.0001) // Use approximate equality for floats
		assert.Equal(t, 530, totalInputTokens)      // 100+200+150+80
		assert.Equal(t, 265, totalOutputTokens)     // 50+100+75+40

		// Verify per-model aggregation
		claudeUsage := modelUsage["bedrock:claude-3-sonnet"]
		assert.Equal(t, 300, claudeUsage.InputTokens)           // 100+200
		assert.Equal(t, 150, claudeUsage.OutputTokens)          // 50+100
		assert.InDelta(t, 0.009, claudeUsage.TotalCost, 0.0001) // 0.003+0.006

		novaUsage := modelUsage["bedrock:nova-pro"]
		assert.Equal(t, 150, novaUsage.InputTokens)
		assert.Equal(t, 0.002, novaUsage.TotalCost)

		gptUsage := modelUsage["openai:gpt-4"]
		assert.Equal(t, 80, gptUsage.InputTokens)
		assert.Equal(t, 0.004, gptUsage.TotalCost)
	})

	t.Run("InferenceConfigValidationIntegration", func(t *testing.T) {
		// Test inference config validation in realistic scenarios

		validConfigs := []*InferenceConfig{
			{
				Model:       "claude-3-sonnet",
				Temperature: 0.7,
				MaxTokens:   4000,
				TopP:        0.9,
			},
			{
				Model:     "gpt-4",
				MaxTokens: 2000,
			},
			{
				// Minimal config - just model
				Model: "nova-pro",
			},
		}

		for i, config := range validConfigs {
			t.Run(fmt.Sprintf("ValidConfig%d", i+1), func(t *testing.T) {
				assert.True(t, config.IsValid(), "Config should be valid: %+v", config)

				// Test using with ClientOptions
				opts := ClientOptions{}
				WithInferenceConfig(config)(&opts)
				assert.Equal(t, config, opts.InferenceConfig)
			})
		}

		invalidConfigs := []*InferenceConfig{
			{
				Temperature: 3.0, // Too high
			},
			{
				MaxTokens: -100, // Negative
			},
			{
				TopP: 1.5, // > 1.0
			},
		}

		for i, config := range invalidConfigs {
			t.Run(fmt.Sprintf("InvalidConfig%d", i+1), func(t *testing.T) {
				assert.False(t, config.IsValid(), "Config should be invalid: %+v", config)
			})
		}
	})

	t.Run("UsageExtractorIntegration", func(t *testing.T) {
		// Test usage extractor with different provider data formats
		extractor := &mockUsageExtractor{}

		// Test different raw usage data formats
		testCases := []struct {
			name     string
			rawUsage any
			model    string
			provider string
			expected *Usage
		}{
			{
				name: "BedrockStyleUsage",
				rawUsage: map[string]interface{}{
					"input_tokens":  100,
					"output_tokens": 50,
				},
				model:    "claude-3-sonnet",
				provider: "bedrock",
				expected: &Usage{
					InputTokens:  100,
					OutputTokens: 50,
					TotalTokens:  150,
					Model:        "claude-3-sonnet",
					Provider:     "bedrock",
				},
			},
			{
				name:     "NilUsage",
				rawUsage: nil,
				model:    "test-model",
				provider: "test-provider",
				expected: nil,
			},
			{
				name:     "UnsupportedFormat",
				rawUsage: "unsupported string format",
				model:    "test-model",
				provider: "test-provider",
				expected: nil,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := extractor.ExtractUsage(tc.rawUsage, tc.model, tc.provider)

				if tc.expected == nil {
					assert.Nil(t, result)
				} else {
					require.NotNil(t, result)
					assert.Equal(t, tc.expected.InputTokens, result.InputTokens)
					assert.Equal(t, tc.expected.OutputTokens, result.OutputTokens)
					assert.Equal(t, tc.expected.TotalTokens, result.TotalTokens)
					assert.Equal(t, tc.expected.Model, result.Model)
					assert.Equal(t, tc.expected.Provider, result.Provider)
					assert.False(t, result.Timestamp.IsZero()) // Should be set
				}
			})
		}
	})
}

// Example of how a real provider might use the new capabilities
func Example_phase1Usage() {
	// This demonstrates how the enhanced ClientOptions would be used
	// in a real application with proper usage tracking

	ctx := context.Background()

	// Set up cost tracking
	var totalCost float64
	usageTracker := func(provider, model string, usage Usage) {
		totalCost += usage.TotalCost
		// In real app: log to metrics system, database, etc.
	}

	// Set up inference configuration
	config := &InferenceConfig{
		Model:       "claude-3-sonnet",
		Region:      "us-west-2",
		Temperature: 0.7,
		MaxTokens:   4000,
	}

	// Note: This would work once providers are updated to use the enhanced options
	_ = ctx    // Avoid unused variable in example
	_ = config // These would be used in: NewClient(ctx, "bedrock", WithInferenceConfig(config), WithUsageCallback(usageTracker))
	_ = usageTracker

	fmt.Println("Example demonstrates Phase 1 provider-agnostic foundation")
	// Output: Example demonstrates Phase 1 provider-agnostic foundation
}

// TestBedrockClientTimeoutFix tests that bedrock client creation doesn't hang indefinitely
func TestBedrockClientTimeoutFix(t *testing.T) {
	ctx := context.Background()

	t.Run("bedrock_client_creation_with_timeout", func(t *testing.T) {
		// This is the exact call that was hanging indefinitely in the issue
		start := time.Now()

		client, err := NewClient(ctx, "bedrock",
			WithInferenceConfig(&InferenceConfig{
				Model:       "anthropic.claude-3-sonnet-20240229-v1:0",
				Region:      "us-west-2",
				Temperature: 0.3,
				MaxTokens:   300,
				TopP:        0.1,
				TopK:        1,
				MaxRetries:  10,
			}),
			WithUsageCallback(func(provider, model string, usage Usage) {
				// Test callback - not critical for this test
			}),
			WithDebug(true),
		)

		elapsed := time.Since(start)

		// The key test: should complete quickly, not hang indefinitely
		assert.Less(t, elapsed, 30*time.Second, "Client creation should complete within 30 seconds, not hang indefinitely")

		if err != nil {
			// Error is expected in various scenarios, but should not hang
			t.Logf("✅ Client creation failed quickly after %v: %v", elapsed, err)

			// Could be because bedrock isn't registered or because of credential issues
			errorContainsBedrock := strings.Contains(err.Error(), "bedrock")
			errorContainsAWS := strings.Contains(err.Error(), "AWS")

			// At least one of these should be true
			assert.True(t, errorContainsBedrock || errorContainsAWS, "Error should be related to bedrock or AWS, got: %v", err)
		} else {
			// Success is also fine if credentials are valid
			t.Logf("✅ Client creation succeeded after %v", elapsed)
			assert.NotNil(t, client, "Client should not be nil on success")
			defer client.Close()
		}

		// Most importantly: it should not take more than a few seconds
		assert.Less(t, elapsed, 5*time.Second, "Should complete very quickly in most cases")
	})

	t.Run("openai_client_still_works", func(t *testing.T) {
		// Verify that our fix didn't break other providers
		start := time.Now()

		client, err := NewClient(ctx, "openai",
			WithInferenceConfig(&InferenceConfig{
				Model:       "gpt-4",
				Temperature: 0.3,
				MaxTokens:   100,
			}),
		)

		elapsed := time.Since(start)

		// Should complete quickly
		assert.Less(t, elapsed, 5*time.Second, "OpenAI client creation should be fast")

		if err != nil {
			// Error expected if no API key, but should be quick
			t.Logf("✅ OpenAI client creation failed quickly after %v (expected without API key): %v", elapsed, err)
		} else {
			t.Logf("✅ OpenAI client creation succeeded after %v", elapsed)
			defer client.Close()
		}
	})
}

// Mock implementation for testing - shared across integration tests
type mockUsageExtractor struct{}

func (e *mockUsageExtractor) ExtractUsage(rawUsage any, model string, provider string) *Usage {
	if rawUsage == nil {
		return nil
	}

	// Mock extraction logic
	if data, ok := rawUsage.(map[string]interface{}); ok {
		usage := &Usage{
			Model:     model,
			Provider:  provider,
			Timestamp: time.Now(),
		}

		if inputTokens, exists := data["input_tokens"]; exists {
			if tokens, ok := inputTokens.(int); ok {
				usage.InputTokens = tokens
			}
		}

		if outputTokens, exists := data["output_tokens"]; exists {
			if tokens, ok := outputTokens.(int); ok {
				usage.OutputTokens = tokens
			}
		}

		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
		return usage
	}

	return nil
}

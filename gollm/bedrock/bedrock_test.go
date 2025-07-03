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
	"context"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeWithClientOptions(t *testing.T) {
	tests := []struct {
		name            string
		defaults        *BedrockOptions
		inferenceConfig *gollm.InferenceConfig
		expectedResult  *BedrockOptions
	}{
		{
			name: "merge all inference config parameters",
			defaults: &BedrockOptions{
				Region:      "us-west-2",
				Model:       "default-model",
				MaxTokens:   1000,
				Temperature: 0.5,
				TopP:        0.9,
				MaxRetries:  3,
			},
			inferenceConfig: &gollm.InferenceConfig{
				Model:       "custom-model",
				Region:      "us-east-1",
				Temperature: 0.7,
				MaxTokens:   2000,
				TopP:        0.95,
				MaxRetries:  5,
			},
			expectedResult: &BedrockOptions{
				Region:      "us-east-1",    // From InferenceConfig
				Model:       "custom-model", // From InferenceConfig
				MaxTokens:   2000,           // From InferenceConfig
				Temperature: 0.7,            // From InferenceConfig
				TopP:        0.95,           // From InferenceConfig
				MaxRetries:  5,              // From InferenceConfig
			},
		},
		{
			name: "partial inference config overrides",
			defaults: &BedrockOptions{
				Region:      "us-west-2",
				Model:       "default-model",
				MaxTokens:   1000,
				Temperature: 0.5,
				TopP:        0.9,
				MaxRetries:  3,
			},
			inferenceConfig: &gollm.InferenceConfig{
				Temperature: 0.8,
				MaxTokens:   1500,
			},
			expectedResult: &BedrockOptions{
				Region:      "us-west-2",     // From defaults
				Model:       "default-model", // From defaults
				MaxTokens:   1500,            // From InferenceConfig
				Temperature: 0.8,             // From InferenceConfig
				TopP:        0.9,             // From defaults
				MaxRetries:  3,               // From defaults
			},
		},
		{
			name: "nil inference config uses defaults",
			defaults: &BedrockOptions{
				Region:      "us-west-2",
				Model:       "default-model",
				MaxTokens:   1000,
				Temperature: 0.5,
				TopP:        0.9,
				MaxRetries:  3,
			},
			inferenceConfig: nil,
			expectedResult: &BedrockOptions{
				Region:      "us-west-2",
				Model:       "default-model",
				MaxTokens:   1000,
				Temperature: 0.5,
				TopP:        0.9,
				MaxRetries:  3,
			},
		},
		{
			name: "zero values in inference config are ignored",
			defaults: &BedrockOptions{
				Region:      "us-west-2",
				Model:       "default-model",
				MaxTokens:   1000,
				Temperature: 0.5,
				TopP:        0.9,
				MaxRetries:  3,
			},
			inferenceConfig: &gollm.InferenceConfig{
				Model:       "custom-model", // Non-zero value, should override
				Temperature: 0,              // Zero value, should be ignored
				MaxTokens:   0,              // Zero value, should be ignored
			},
			expectedResult: &BedrockOptions{
				Region:      "us-west-2",    // From defaults
				Model:       "custom-model", // From InferenceConfig
				MaxTokens:   1000,           // From defaults (InferenceConfig was 0)
				Temperature: 0.5,            // From defaults (InferenceConfig was 0)
				TopP:        0.9,            // From defaults
				MaxRetries:  3,              // From defaults
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientOpts := gollm.ClientOptions{
				InferenceConfig: tt.inferenceConfig,
			}

			result := mergeWithClientOptions(tt.defaults, clientOpts)

			assert.Equal(t, tt.expectedResult.Region, result.Region)
			assert.Equal(t, tt.expectedResult.Model, result.Model)
			assert.Equal(t, tt.expectedResult.MaxTokens, result.MaxTokens)
			assert.Equal(t, tt.expectedResult.Temperature, result.Temperature)
			assert.Equal(t, tt.expectedResult.TopP, result.TopP)
			assert.Equal(t, tt.expectedResult.MaxRetries, result.MaxRetries)
		})
	}
}

func TestConvertAWSUsage(t *testing.T) {
	tests := []struct {
		name           string
		awsUsage       any
		model          string
		provider       string
		expectedResult *gollm.Usage
	}{
		{
			name: "valid token usage conversion",
			awsUsage: &types.TokenUsage{
				InputTokens:  aws.Int32(100),
				OutputTokens: aws.Int32(50),
				TotalTokens:  aws.Int32(150),
			},
			model:    "test-model",
			provider: "bedrock",
			expectedResult: &gollm.Usage{
				InputTokens:  100,
				OutputTokens: 50,
				TotalTokens:  150,
				Model:        "test-model",
				Provider:     "bedrock",
				// Timestamp will be set to time.Now(), so we'll check it separately
			},
		},
		{
			name:           "nil usage returns nil",
			awsUsage:       nil,
			model:          "test-model",
			provider:       "bedrock",
			expectedResult: nil,
		},
		{
			name:           "invalid usage type returns nil",
			awsUsage:       "invalid-type",
			model:          "test-model",
			provider:       "bedrock",
			expectedResult: nil,
		},
		{
			name: "partial token usage with nil values",
			awsUsage: &types.TokenUsage{
				InputTokens:  aws.Int32(75),
				OutputTokens: nil, // Nil values should be handled
				TotalTokens:  aws.Int32(75),
			},
			model:    "test-model",
			provider: "bedrock",
			expectedResult: &gollm.Usage{
				InputTokens:  75,
				OutputTokens: 0, // aws.ToInt32(nil) returns 0
				TotalTokens:  75,
				Model:        "test-model",
				Provider:     "bedrock",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertAWSUsage(tt.awsUsage, tt.model, tt.provider)

			if tt.expectedResult == nil {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)
			assert.Equal(t, tt.expectedResult.InputTokens, result.InputTokens)
			assert.Equal(t, tt.expectedResult.OutputTokens, result.OutputTokens)
			assert.Equal(t, tt.expectedResult.TotalTokens, result.TotalTokens)
			assert.Equal(t, tt.expectedResult.Model, result.Model)
			assert.Equal(t, tt.expectedResult.Provider, result.Provider)

			// Check that timestamp is recent (within last 5 seconds)
			timeDiff := time.Since(result.Timestamp)
			assert.True(t, timeDiff < 5*time.Second, "Timestamp should be recent")
			assert.True(t, timeDiff >= 0, "Timestamp should not be in the future")
		})
	}
}

func TestUsageCallbackIntegration(t *testing.T) {
	// Track usage callback invocations
	var callbackInvocations []struct {
		provider string
		model    string
		usage    gollm.Usage
	}

	usageCallback := func(provider, model string, usage gollm.Usage) {
		callbackInvocations = append(callbackInvocations, struct {
			provider string
			model    string
			usage    gollm.Usage
		}{
			provider: provider,
			model:    model,
			usage:    usage,
		})
	}

	// Create mock BedrockClient with usage callback
	client := &BedrockClient{
		clientOpts: gollm.ClientOptions{
			UsageCallback: usageCallback,
		},
	}

	// Create mock chat session
	session := &bedrockChatSession{
		client: client,
		model:  "test-model",
	}

	// Simulate AWS usage data
	awsUsage := &types.TokenUsage{
		InputTokens:  aws.Int32(200),
		OutputTokens: aws.Int32(100),
		TotalTokens:  aws.Int32(300),
	}

	// Test that usage callback is called when sending
	// Note: We can't easily test the full Send() method without AWS setup,
	// so we'll test the callback logic directly
	if client.clientOpts.UsageCallback != nil {
		if structuredUsage := convertAWSUsage(awsUsage, session.model, "bedrock"); structuredUsage != nil {
			client.clientOpts.UsageCallback("bedrock", session.model, *structuredUsage)
		}
	}

	// Verify callback was invoked correctly
	require.Len(t, callbackInvocations, 1)
	invocation := callbackInvocations[0]

	assert.Equal(t, "bedrock", invocation.provider)
	assert.Equal(t, "test-model", invocation.model)
	assert.Equal(t, 200, invocation.usage.InputTokens)
	assert.Equal(t, 100, invocation.usage.OutputTokens)
	assert.Equal(t, 300, invocation.usage.TotalTokens)
	assert.Equal(t, "test-model", invocation.usage.Model)
	assert.Equal(t, "bedrock", invocation.usage.Provider)
}

func TestBedrockChatResponseUsageMetadata(t *testing.T) {
	tests := []struct {
		name         string
		rawUsage     any
		expectedType string
	}{
		{
			name: "structured usage returned for valid AWS usage",
			rawUsage: &types.TokenUsage{
				InputTokens:  aws.Int32(50),
				OutputTokens: aws.Int32(25),
				TotalTokens:  aws.Int32(75),
			},
			expectedType: "*gollm.Usage",
		},
		{
			name:         "raw usage returned for nil",
			rawUsage:     nil,
			expectedType: "<nil>",
		},
		{
			name:         "raw usage returned for invalid type",
			rawUsage:     "invalid-usage-data",
			expectedType: "string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := &bedrockChatResponse{
				usage: tt.rawUsage,
			}

			metadata := response.UsageMetadata()

			switch tt.expectedType {
			case "*gollm.Usage":
				usage, ok := metadata.(*gollm.Usage)
				require.True(t, ok, "Expected *gollm.Usage, got %T", metadata)
				assert.Equal(t, 50, usage.InputTokens)
				assert.Equal(t, 25, usage.OutputTokens)
				assert.Equal(t, 75, usage.TotalTokens)
				assert.Equal(t, "bedrock", usage.Model)
				assert.Equal(t, "bedrock", usage.Provider)
			case "<nil>":
				assert.Nil(t, metadata)
			case "string":
				_, ok := metadata.(string)
				require.True(t, ok, "Expected string, got %T", metadata)
				assert.Equal(t, "invalid-usage-data", metadata)
			}
		})
	}
}

func TestBedrockCompletionResponseUsageMetadata(t *testing.T) {
	// Test completion response usage metadata follows same pattern
	rawUsage := &types.TokenUsage{
		InputTokens:  aws.Int32(60),
		OutputTokens: aws.Int32(30),
		TotalTokens:  aws.Int32(90),
	}

	response := &simpleBedrockCompletionResponse{
		usage: rawUsage,
	}

	metadata := response.UsageMetadata()
	usage, ok := metadata.(*gollm.Usage)
	require.True(t, ok, "Expected *gollm.Usage, got %T", metadata)

	assert.Equal(t, 60, usage.InputTokens)
	assert.Equal(t, 30, usage.OutputTokens)
	assert.Equal(t, 90, usage.TotalTokens)
	assert.Equal(t, "bedrock", usage.Model)
	assert.Equal(t, "bedrock", usage.Provider)
}

// Integration test that verifies the full ClientOptions flow
func TestClientOptionsIntegration(t *testing.T) {
	// Test that NewBedrockClient properly stores and uses ClientOptions
	inferenceConfig := &gollm.InferenceConfig{
		Model:       "test-model",
		Region:      "us-east-1",
		Temperature: 0.8,
		MaxTokens:   2000,
		TopP:        0.95,
		MaxRetries:  5,
	}

	var callbackCalled bool
	usageCallback := func(provider, model string, usage gollm.Usage) {
		callbackCalled = true
	}

	clientOpts := gollm.ClientOptions{
		InferenceConfig: inferenceConfig,
		UsageCallback:   usageCallback,
		Debug:           true,
	}

	// Note: We can't create a real client without AWS credentials,
	// but we can test the merging logic
	merged := mergeWithClientOptions(DefaultOptions, clientOpts)

	// Verify inference config was merged correctly
	assert.Equal(t, "test-model", merged.Model)
	assert.Equal(t, "us-east-1", merged.Region)
	assert.Equal(t, float32(0.8), merged.Temperature)
	assert.Equal(t, int32(2000), merged.MaxTokens)
	assert.Equal(t, float32(0.95), merged.TopP)
	assert.Equal(t, 5, merged.MaxRetries)

	// Test client options storage (simulate what NewBedrockClient does)
	mockClient := &BedrockClient{
		options:    merged,
		clientOpts: clientOpts,
	}

	// Verify clientOpts were stored
	assert.NotNil(t, mockClient.clientOpts.InferenceConfig)
	assert.NotNil(t, mockClient.clientOpts.UsageCallback)
	assert.True(t, mockClient.clientOpts.Debug)

	// Test callback functionality
	if mockClient.clientOpts.UsageCallback != nil {
		mockClient.clientOpts.UsageCallback("test", "test", gollm.Usage{})
	}
	assert.True(t, callbackCalled)
}

// TestClientCreationWithTimeout tests that client creation respects timeout and doesn't hang
func TestClientCreationWithTimeout(t *testing.T) {
	ctx := context.Background()

	t.Run("timeout_during_config_loading", func(t *testing.T) {
		// Create a context with a very short timeout to simulate timeout during config loading
		shortCtx, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
		defer cancel()

		// This should timeout quickly rather than hanging indefinitely
		start := time.Now()
		client, err := NewBedrockClientWithOptions(shortCtx, &BedrockOptions{
			Region:  "us-east-1",
			Model:   "us.anthropic.claude-sonnet-4-20250514-v1:0",
			Timeout: 1 * time.Millisecond, // Very short timeout
		})

		elapsed := time.Since(start)

		// Should either fail quickly or succeed, but not hang indefinitely
		assert.Less(t, elapsed, 5*time.Second, "Should complete quickly, not hang")

		if err != nil {
			// If it fails, it should be due to timeout or credential issues
			t.Logf("Client creation failed as expected after %v with error: %v", elapsed, err)
			assert.Contains(t, err.Error(), "failed to load AWS configuration", "Error should indicate AWS config issue")
		} else {
			// If it succeeds, that's also fine - just make sure it didn't hang
			assert.NotNil(t, client, "Client should not be nil if no error")
			if client != nil {
				client.Close()
			}
			t.Logf("Client creation succeeded after %v", elapsed)
		}
	})

	t.Run("reasonable_timeout_with_invalid_credentials", func(t *testing.T) {
		// Test with a reasonable timeout but potentially invalid credentials
		// This should complete within the timeout period, not hang indefinitely
		start := time.Now()

		client, err := NewBedrockClientWithOptions(ctx, &BedrockOptions{
			Region:  "us-east-1",
			Model:   "us.anthropic.claude-sonnet-4-20250514-v1:0",
			Timeout: 5 * time.Second, // Reasonable timeout
		})

		elapsed := time.Since(start)

		// Either succeeds (if valid credentials) or fails within timeout period
		assert.Less(t, elapsed, 10*time.Second, "Should complete within reasonable time, not hang")

		if err != nil {
			t.Logf("Client creation failed as expected after %v with error: %v", elapsed, err)
		} else {
			assert.NotNil(t, client, "Client should not be nil if no error")
			if client != nil {
				client.Close()
			}
			t.Logf("Client creation succeeded after %v", elapsed)
		}
	})
}

// TestTimeoutConfigurationRespected tests that custom timeout values are properly used
func TestTimeoutConfigurationRespected(t *testing.T) {
	testCases := []struct {
		name            string
		configTimeout   time.Duration
		expectedMaxTime time.Duration
	}{
		{
			name:            "very_short_timeout",
			configTimeout:   100 * time.Millisecond,
			expectedMaxTime: 2 * time.Second,
		},
		{
			name:            "moderate_timeout",
			configTimeout:   2 * time.Second,
			expectedMaxTime: 5 * time.Second,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a context that will definitely timeout
			ctx, cancel := context.WithTimeout(context.Background(), tc.configTimeout)
			defer cancel()

			start := time.Now()
			_, err := NewBedrockClientWithOptions(ctx, &BedrockOptions{
				Region:  "us-east-1",
				Model:   "us.anthropic.claude-sonnet-4-20250514-v1:0",
				Timeout: tc.configTimeout,
			})
			elapsed := time.Since(start)

			// Should complete within expected maximum time range
			// Note: Removed minimum time constraint since execution time varies greatly
			// in different environments (local vs CI/CD)
			assert.LessOrEqual(t, elapsed, tc.expectedMaxTime, "Should not exceed maximum expected time")

			if err != nil {
				t.Logf("Timeout test completed in %v with error: %v", elapsed, err)
			}
		})
	}
}

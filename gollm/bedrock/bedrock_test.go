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

// TestConfigurableClientInterface verifies that BedrockClient implements ConfigurableClient
func TestConfigurableClientInterface(t *testing.T) {
	// Create a mock client for testing (without AWS credentials)
	client := &BedrockClient{
		options: DefaultOptions,
	}

	// Test type assertion works
	configurable, ok := (interface{})(client).(ConfigurableClient)
	require.True(t, ok, "BedrockClient should implement ConfigurableClient")

	// Test configuration setting and retrieval
	config := &InferenceConfig{
		Model:       "anthropic.claude-3-sonnet-20240229-v1:0",
		Region:      "us-west-2",
		Temperature: 0.7,
		MaxTokens:   2000,
		TopP:        0.9,
		MaxRetries:  3,
	}

	err := configurable.SetInferenceConfig(config)
	require.NoError(t, err)

	retrieved := configurable.GetInferenceConfig()
	assert.Equal(t, config.Model, retrieved.Model)
	assert.Equal(t, config.Region, retrieved.Region)
	assert.Equal(t, config.Temperature, retrieved.Temperature)
	assert.Equal(t, config.MaxTokens, retrieved.MaxTokens)
	assert.Equal(t, config.TopP, retrieved.TopP)
	assert.Equal(t, config.MaxRetries, retrieved.MaxRetries)

	// Verify that internal options were updated
	assert.Equal(t, config.Model, client.options.Model)
	assert.Equal(t, config.Region, client.options.Region)
	assert.Equal(t, config.Temperature, client.options.Temperature)
	assert.Equal(t, config.MaxTokens, client.options.MaxTokens)
	assert.Equal(t, config.TopP, client.options.TopP)
	assert.Equal(t, config.MaxRetries, client.options.MaxRetries)
}

// TestInferenceConfigValidation tests the Validate method
func TestInferenceConfigValidation(t *testing.T) {
	tests := []struct {
		name      string
		config    *InferenceConfig
		expectErr bool
	}{
		{
			name: "valid config",
			config: &InferenceConfig{
				Temperature: 0.7,
				MaxTokens:   2000,
				TopP:        0.9,
				TopK:        40,
				MaxRetries:  3,
			},
			expectErr: false,
		},
		{
			name: "invalid temperature too low",
			config: &InferenceConfig{
				Temperature: -0.1,
			},
			expectErr: true,
		},
		{
			name: "invalid temperature too high",
			config: &InferenceConfig{
				Temperature: 2.1,
			},
			expectErr: true,
		},
		{
			name: "invalid maxTokens negative",
			config: &InferenceConfig{
				MaxTokens: -1,
			},
			expectErr: true,
		},
		{
			name: "invalid topP too low",
			config: &InferenceConfig{
				TopP: -0.1,
			},
			expectErr: true,
		},
		{
			name: "invalid topP too high",
			config: &InferenceConfig{
				TopP: 1.1,
			},
			expectErr: true,
		},
		{
			name: "invalid topK negative",
			config: &InferenceConfig{
				TopK: -1,
			},
			expectErr: true,
		},
		{
			name: "invalid maxRetries negative",
			config: &InferenceConfig{
				MaxRetries: -1,
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestUsageCallbackWithExtensionInterface tests the new usage callback pattern
func TestUsageCallbackWithExtensionInterface(t *testing.T) {
	// Track usage callback invocations
	var callbackInvocations []struct {
		provider string
		model    string
		usage    Usage
	}

	usageCallback := func(provider, model string, usage Usage) {
		callbackInvocations = append(callbackInvocations, struct {
			provider string
			model    string
			usage    Usage
		}{
			provider: provider,
			model:    model,
			usage:    usage,
		})
	}

	// Create mock BedrockClient and configure it via extension interface
	client := &BedrockClient{
		options: DefaultOptions,
	}

	configurable := (interface{})(client).(ConfigurableClient)
	configurable.SetUsageCallback(usageCallback)

	// Create mock chat session
	session := &bedrockChatSession{
		client: client,
		model:  "test-model",
	}

	// Simulate AWS usage data and test the callback
	awsUsage := &types.TokenUsage{
		InputTokens:  aws.Int32(200),
		OutputTokens: aws.Int32(100),
		TotalTokens:  aws.Int32(300),
	}

	// Test the new callback pattern
	if client.usageCallback != nil {
		usage := convertAWSUsageToBedrock(awsUsage, session.model)
		client.usageCallback("bedrock", session.model, usage)
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

// TestConvertAWSUsageToBedrock tests the Bedrock-specific usage conversion
func TestConvertAWSUsageToBedrock(t *testing.T) {
	tests := []struct {
		name     string
		awsUsage *types.TokenUsage
		model    string
		expected Usage
	}{
		{
			name: "complete token usage conversion",
			awsUsage: &types.TokenUsage{
				InputTokens:  aws.Int32(150),
				OutputTokens: aws.Int32(75),
				TotalTokens:  aws.Int32(225),
			},
			model: "anthropic.claude-3-sonnet",
			expected: Usage{
				InputTokens:  150,
				OutputTokens: 75,
				TotalTokens:  225,
				Model:        "anthropic.claude-3-sonnet",
				Provider:     "bedrock",
				// Timestamp will be checked separately
			},
		},
		{
			name: "token usage with nil values",
			awsUsage: &types.TokenUsage{
				InputTokens:  aws.Int32(100),
				OutputTokens: nil, // Nil should become 0
				TotalTokens:  aws.Int32(100),
			},
			model: "test-model",
			expected: Usage{
				InputTokens:  100,
				OutputTokens: 0,
				TotalTokens:  100,
				Model:        "test-model",
				Provider:     "bedrock",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertAWSUsageToBedrock(tt.awsUsage, tt.model)

			assert.Equal(t, tt.expected.InputTokens, result.InputTokens)
			assert.Equal(t, tt.expected.OutputTokens, result.OutputTokens)
			assert.Equal(t, tt.expected.TotalTokens, result.TotalTokens)
			assert.Equal(t, tt.expected.Model, result.Model)
			assert.Equal(t, tt.expected.Provider, result.Provider)

			// Check that timestamp is recent (within last 5 seconds)
			timeDiff := time.Since(result.Timestamp)
			assert.True(t, timeDiff < 5*time.Second, "Timestamp should be recent")
			assert.True(t, timeDiff >= 0, "Timestamp should not be in the future")
		})
	}
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

// TestExtensionPatternIntegration demonstrates the complete extension pattern usage
func TestExtensionPatternIntegration(t *testing.T) {
	// Create mock client for testing (without AWS credentials)
	client := &BedrockClient{
		options: DefaultOptions,
	}

	// Use type assertion to access Bedrock-specific features
	if bedrockClient, ok := (interface{})(client).(ConfigurableClient); ok {
		// Configure inference parameters
		config := &InferenceConfig{
			Model:       "anthropic.claude-3-5-sonnet-20241022-v2:0",
			Region:      "us-west-2",
			Temperature: 0.7,
			MaxTokens:   4000,
			TopP:        0.9,
			MaxRetries:  3,
		}
		err := bedrockClient.SetInferenceConfig(config)
		assert.NoError(t, err)

		// Set usage tracking
		var capturedUsage []Usage
		bedrockClient.SetUsageCallback(func(provider, model string, usage Usage) {
			capturedUsage = append(capturedUsage, usage)
		})

		// Verify configuration was applied
		retrievedConfig := bedrockClient.GetInferenceConfig()
		assert.Equal(t, config.Model, retrievedConfig.Model)
		assert.Equal(t, config.Temperature, retrievedConfig.Temperature)

		// Test usage callback
		testUsage := Usage{
			InputTokens:  100,
			OutputTokens: 50,
			TotalTokens:  150,
			Model:        "test-model",
			Provider:     "bedrock",
			Timestamp:    time.Now(),
		}

		// Simulate usage callback invocation
		client.usageCallback("bedrock", "test-model", testUsage)

		require.Len(t, capturedUsage, 1)
		assert.Equal(t, testUsage.TotalTokens, capturedUsage[0].TotalTokens)
	} else {
		t.Error("BedrockClient should implement ConfigurableClient interface")
	}
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
		expectedMinTime time.Duration
		expectedMaxTime time.Duration
	}{
		{
			name:            "very_short_timeout",
			configTimeout:   100 * time.Millisecond,
			expectedMinTime: 50 * time.Millisecond,
			expectedMaxTime: 2 * time.Second,
		},
		{
			name:            "moderate_timeout",
			configTimeout:   2 * time.Second,
			expectedMinTime: 100 * time.Millisecond,
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

			// Should timeout within expected range
			assert.GreaterOrEqual(t, elapsed, tc.expectedMinTime, "Should take at least minimum expected time")
			assert.LessOrEqual(t, elapsed, tc.expectedMaxTime, "Should not exceed maximum expected time")

			if err != nil {
				t.Logf("Timeout test completed in %v with error: %v", elapsed, err)
			}
		})
	}
}

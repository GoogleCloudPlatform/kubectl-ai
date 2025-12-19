package gollm

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestBedrockClient is a single entry point that validates Client Options
// for the Bedrock client across several named scenarios. It does not perform
// any live network calls.
func TestBedrockClient(t *testing.T) {
	t.Run("LLM_CLIENT not set shows error message", func(t *testing.T) {
		// Ensure LLM_CLIENT is not set
		os.Unsetenv("LLM_CLIENT")

		ctx := context.Background()

		// Call NewClient with empty providerID - should fail
		client, err := NewClient(ctx, "")

		// Expect an error to be returned
		if err == nil {
			t.Fatal("Expected error when LLM_CLIENT is not set, but got nil error")
		}

		// Verify the error message mentions LLM_CLIENT
		if !strings.Contains(err.Error(), "LLM_CLIENT is not set") {
			t.Errorf("Expected error to contain 'LLM_CLIENT is not set', got: %v", err)
		}

		// Verify client is nil when error occurs
		// TODO (nisranjan) Do we need to close the client?
		if client != nil {
			client.Close()
			t.Errorf("Expected nil client when LLM_CLIENT is not set, got: %T", client)
		}

		t.Logf("✓ Error message when LLM_CLIENT not set: %v", err)
	})

	t.Run("LLM_CLIENT set ProviderId set Validate Schema", func(t *testing.T) {
		// Test various valid URL formats
		testCases := []struct {
			name        string
			url         string
			expectError bool
		}{
			{
				name:        "Valid URL with us-east-1",
				url:         "bedrock://bedrock.us-east-1.amazonaws.com",
				expectError: false,
			},
			{
				name:        "Valid URL with eu-west-1",
				url:         "bedrock://bedrock.eu-west-1.amazonaws.com",
				expectError: false,
			},
			{
				name:        "Invalid URL without region",
				url:         "bedrock://bedrock.amazonaws.com",
				expectError: true,
			},
			{
				name:        "Invalid URL with malformed region",
				url:         "bedrock://bedrock.invalid.amazonaws.com",
				expectError: true,
			},
			//TODO (nisranjan) Add test for default region
			//TODO (nisranjan) Add test for error message when neither region not set in URL schema or environment variable
			{
				name:        "Simple bedrock scheme (should default)",
				url:         "bedrock://",
				expectError: false, // Should use default region
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Setenv("LLM_CLIENT", tc.url)
				//t.Setenv("AWS_ACCESS_KEY_ID", "test")
				//t.Setenv("AWS_SECRET_ACCESS_KEY", "test")

				ctx := context.Background()
				client, err := NewClient(ctx, "")

				if tc.expectError {
					if err == nil {
						if client != nil {
							client.Close()
						}
						t.Errorf("Expected error for URL %q but got none", tc.url)
					} else {
						t.Logf("✓ Got expected error for %q: %v", tc.url, err)
					}
				} else {
					if err != nil {
						t.Logf("Client creation failed for %q (validation may not be implemented): %v", tc.url, err)
					} else {
						defer client.Close()
						t.Logf("✓ Successfully created client for %q", tc.url)
					}
				}
			})
		}
	})

	t.Run("AWS_REGION env validation with various scenarios", func(t *testing.T) {
		testCases := []struct {
			name          string
			llmClientURL  string
			awsRegionEnv  string
			expectError   bool
			errorContains string
			description   string
		}{
			{
				name:         "Matching regions URL & ENV us-east-1",
				llmClientURL: "bedrock://bedrock.us-east-1.amazonaws.com",
				awsRegionEnv: "us-east-1",
				expectError:  false,
				description:  "Both URL and Env have us-east-1",
			},
			{
				name:         "Matching regions URL & ENV eu-west-1",
				llmClientURL: "bedrock://bedrock.eu-west-1.amazonaws.com",
				awsRegionEnv: "eu-west-1",
				expectError:  false,
				description:  "Both URL and Env have eu-west-1",
			},
			{
				name:          "Mismatch URL us-east-1 ENV eu-west-1",
				llmClientURL:  "bedrock://bedrock.us-east-1.amazonaws.com",
				awsRegionEnv:  "eu-west-1",
				expectError:   true,
				errorContains: "mismatch",
				description:   "URL has us-east-1, Env has eu-west-1",
			},
			{
				name:          "Mismatch URL eu-west-1 ENV us-east-1",
				llmClientURL:  "bedrock://bedrock.eu-west-1.amazonaws.com",
				awsRegionEnv:  "us-east-1",
				expectError:   true,
				errorContains: "mismatch",
				description:   "URL has eu-west-1, Env has us-east-1",
			},
			{
				name:          "Mismatch URL ap-south-1 ENV us-west-2",
				llmClientURL:  "bedrock://bedrock.ap-south-1.amazonaws.com",
				awsRegionEnv:  "us-west-2",
				expectError:   true,
				errorContains: "mismatch",
				description:   "URL has ap-south-1, Env has us-west-2",
			},
			{
				name:         "URL has region, Env not set (should use URL region)",
				llmClientURL: "bedrock://bedrock.us-east-1.amazonaws.com",
				awsRegionEnv: "",
				expectError:  false,
				description:  "Only URL has region, Env is empty",
			},
			{
				name:         "URL without region, ENV has region (should use env region)",
				llmClientURL: "bedrock://",
				awsRegionEnv: "us-east-1",
				expectError:  false,
				description:  "Only Env has region, URL is simple",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Setenv("LLM_CLIENT", tc.llmClientURL)
				t.Setenv("AWS_BEARER_TOKEN_BEDROCK", "test-bearer-token")
				if tc.awsRegionEnv != "" {
					t.Setenv("AWS_REGION", tc.awsRegionEnv)
				} else {
					os.Unsetenv("AWS_REGION")
				}

				ctx := context.Background()
				client, err := NewClient(ctx, tc.llmClientURL)

				if tc.expectError {
					if err == nil {
						if client != nil {
							client.Close()
						}
						t.Errorf("Expected error for %s, but got none", tc.description)
					} else {
						// Check if error contains expected message
						if tc.errorContains != "" && !strings.Contains(err.Error(), tc.errorContains) {
							t.Errorf("Expected error to contain %q for %s, got: %v",
								tc.errorContains, tc.description, err)
						}
						t.Logf("✓ Got expected error for %s: %v", tc.description, err)
					}

					// Verify client is nil on error
					if client != nil {
						client.Close()
						t.Errorf("Expected nil client on error for %s, got: %T", tc.description, client)
					}
				} else {
					if err != nil {
						t.Errorf("Expected success for %s, got error: %v", tc.description, err)
					} else {
						defer client.Close()
						if _, ok := client.(*BedrockClient); !ok {
							t.Errorf("Expected *BedrockClient for %s, got %T", tc.description, client)
						}
						t.Logf("✓ Successfully validated %s", tc.description)
					}
				}
			})
		}
	})

	t.Run("Returns bearer when AWS_BEARER_TOKEN_BEDROCK is set", func(t *testing.T) {
		t.Setenv("AWS_BEARER_TOKEN_BEDROCK", "test-bearer-token")
		// Clear other credentials to ensure bearer token is used
		os.Unsetenv("AWS_ACCESS_KEY_ID")
		os.Unsetenv("AWS_SECRET_ACCESS_KEY")

		authMethod, err := getAWSAuthMethod()

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if authMethod != AWSAuthBearerToken {
			t.Errorf("Expected auth method %q, got %q", AWSAuthBearerToken, authMethod)
		}

		t.Logf("✓ Correctly detected bearer token authentication")
	})

	t.Run("Returns AWS SigV4 when credentials are set", func(t *testing.T) {
		os.Unsetenv("AWS_BEARER_TOKEN_BEDROCK")
		t.Setenv("AWS_ACCESS_KEY_ID", "test-key")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")

		authMethod, err := getAWSAuthMethod()

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if authMethod != AWSAuthAWSSigV4 {
			t.Errorf("Expected auth method %q, got %q", AWSAuthAWSSigV4, authMethod)
		}

		t.Logf("✓ Correctly detected AWS SigV4 authentication")
	})

	t.Run("Bearer token takes precedence over SigV4", func(t *testing.T) {
		t.Setenv("AWS_BEARER_TOKEN_BEDROCK", "test-bearer")
		t.Setenv("AWS_ACCESS_KEY_ID", "test-key")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")

		authMethod, err := getAWSAuthMethod()

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if authMethod != AWSAuthBearerToken {
			t.Errorf("Expected bearer token to take precedence, got %q", authMethod)
		}

		t.Logf("✓ Bearer token correctly took precedence")
	})

	t.Run("Returns error when no credentials are set", func(t *testing.T) {
		os.Unsetenv("AWS_BEARER_TOKEN_BEDROCK")
		os.Unsetenv("AWS_ACCESS_KEY_ID")
		os.Unsetenv("AWS_SECRET_ACCESS_KEY")

		_, err := getAWSAuthMethod()

		if err == nil {
			t.Fatal("Expected error when no credentials are set")
		}

		if !strings.Contains(err.Error(), "AWS_SECRET_ACCESS_KEY") &&
			!strings.Contains(err.Error(), "AWS_ACCESS_KEY_ID") {
			t.Errorf("Expected error about missing credentials, got: %v", err)
		}

		t.Logf("✓ Correctly returned error for missing credentials: %v", err)
	})

}

func TestListModels(t *testing.T) {
	t.Run("LIVE: Network connectivity check with real Bedrock API call", func(t *testing.T) {
		// TODO (nisranjan) Remove the short mode check
		if testing.Short() {
			t.Skip("Skipping live network test in short mode")
		}

		// Force the test to use Bedrock so we create the right client.
		t.Setenv("LLM_CLIENT", "bedrock://bedrock.ap-south-1.amazonaws.com")
		//t.Setenv("BEDROCK_MODEL", "us.anthropic.claude-sonnet-4-20250514-v1:0")

		if !hasBedrockCredentials() {
			t.Skip("Skipping Bedrock GenerateCompletion test because AWS credentials are not configured")
		}

		ctx := context.Background()

		// Create the Bedrock client
		client, err := NewClient(ctx, "")
		if err != nil {
			t.Fatalf("Failed to create Bedrock client: %v", err)
		}
		defer client.Close()

		// Verify it's a BedrockClient
		bedrockClient, ok := client.(*BedrockClient)
		if !ok {
			t.Fatalf("Expected *BedrockClient, got %T", client)
		}

		t.Logf("✓ Successfully created BedrockClient")

		// Make a live API call to check network connectivity
		// Using ListModels to test the connection
		models, err := bedrockClient.ListModels(ctx)

		// Check for network/connectivity errors
		if err != nil {
			if strings.Contains(err.Error(), "network") ||
				strings.Contains(err.Error(), "connection") ||
				strings.Contains(err.Error(), "timeout") ||
				strings.Contains(err.Error(), "dial") {
				t.Fatalf("Network connectivity error: %v", err)
			}
			// Other errors might be related to auth, model availability, etc.
			t.Logf("Warning: API call failed (may not be network issue): %v", err)
		} else {
			t.Logf("✓ Network connectivity successful")
			t.Logf("✓ Successfully listed %d foundation models from Bedrock API", len(models))
			if len(models) > 0 {
				// Show first 3 models as samples
				sampleCount := len(models)
				if sampleCount > 3 {
					sampleCount = 3
				}
				t.Logf("Sample models: %v", models[:sampleCount])
			}
		}
	})
}

func TestGenerateCompletion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Bedrock GenerateCompletion live test in short mode")
	}

	// Force the test to use Bedrock so we create the right client.
	t.Setenv("LLM_CLIENT", "bedrock://bedrock.ap-south-1.amazonaws.com")
	t.Setenv("BEDROCK_MODEL", "amazon.titan-text-express-v1")

	if !hasBedrockCredentials() {
		t.Skip("Skipping Bedrock GenerateCompletion test because AWS credentials are not configured")
	}

	ctx := context.Background()
	client, err := NewClient(ctx, "")
	if err != nil {
		t.Fatalf("failed to create Bedrock client: %v", err)
	}
	defer client.Close()

	validModel := getBedrockModel("")

	testCases := []struct {
		name           string
		req            *CompletionRequest
		expectErr      bool
		errContains    string
		expectResponse bool
	}{
		{
			name: "model does not support messages",
			req: &CompletionRequest{
				Model:  "amazon.titan-text-express-v1",
				Prompt: "Check if the model can handle simple messages.",
			},
			expectErr: true,
		},
		{
			name: "model specified but no prompt",
			req: &CompletionRequest{
				Model: validModel,
			},
			expectErr:   true,
			errContains: "prompt must be provided",
		},
		{
			name: "prompt provided but no model",
			req: &CompletionRequest{
				Prompt: "This prompt is missing a model ID.",
			},
			expectErr:   true,
			errContains: "model must be specified",
		},
		{
			name: "unsupported models",
			req: &CompletionRequest{
				Model:  "openai.gpt-oss-20b-1:0",
				Prompt: "This prompt is using an unsupported model.",
			},
			expectErr:   true,
			errContains: "model not supported",
		},
		{
			name: "valid completion request",
			req: &CompletionRequest{
				Model:  validModel,
				Prompt: "List a couple of kubectl commands that describe namespaces.",
			},
			expectErr:      false,
			expectResponse: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			reqCtx, cancel := context.WithTimeout(ctx, time.Minute)
			defer cancel()

			resp, err := client.GenerateCompletion(reqCtx, tc.req)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error for %q, got nil", tc.name)
				}
				if tc.errContains != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.errContains)) {
					t.Fatalf("expected error to contain %q, got %v", tc.errContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.name, err)
			}

			if resp == nil {
				t.Fatalf("expected non-nil response for %q, got nil", tc.name)
			}

			if tc.expectResponse {
				if strings.TrimSpace(resp.Response()) == "" {
					t.Fatalf("expected response text for %q, got empty string", tc.name)
				}
			}
		})
	}
}

func hasBedrockCredentials() bool {
	if os.Getenv("AWS_BEARER_TOKEN_BEDROCK") != "" {
		return true
	}
	if os.Getenv("AWS_ACCESS_KEY_ID") != "" && os.Getenv("AWS_SECRET_ACCESS_KEY") != "" {
		return true
	}
	return false
}

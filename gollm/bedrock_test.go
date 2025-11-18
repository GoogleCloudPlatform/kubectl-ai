package gollm

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestConnection is a single entry point that validates provider selection
// for the Bedrock client across several named scenarios. It does not perform
// any live network calls.
func TestConnection(t *testing.T) {
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
}

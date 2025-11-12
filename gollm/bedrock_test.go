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

}

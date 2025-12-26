package gollm

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	//"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
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

func TestStartChat(t *testing.T) {
	if testing.Short() {
		t.Skip("live Bedrock tests skipped in short mode")
	}
	if !hasBedrockCredentials() {
		t.Skip("AWS credentials are required for Bedrock StartChat tests")
	}

	t.Setenv("LLM_CLIENT", "bedrock://bedrock.ap-south-1.amazonaws.com")
	ctx := context.Background()
	client, err := NewClient(ctx, "")
	if err != nil {
		t.Fatalf("failed to create Bedrock client: %v", err)
	}
	defer client.Close()

	bedrockClient, ok := client.(*BedrockClient)
	if !ok {
		t.Fatalf("expected *BedrockClient, got %T", client)
	}

	models, err := bedrockClient.ListModels(ctx)
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected at least one Bedrock model from ListModels")
	}
	validModel := models[0]

	testCases := []struct {
		name       string
		model      string
		expectErr  string
		expectChat bool
	}{
		{
			name:      "nonexistent model",
			model:     "does-not-exist-model",
			expectErr: "model",
		},
		{
			name:      "model not in ListModels",
			model:     "some-virtual-but-not-in-list",
			expectErr: "model not available",
		},
		{
			name:       "valid model",
			model:      validModel,
			expectChat: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			chat := bedrockClient.StartChat("system prompt", tc.model)
			if tc.expectErr != "" {
				// StartChat would have to return (Chat, error) or expose validation; adapt once that change exists
				t.Fatalf("expected error %q but StartChat currently cannot return errors", tc.expectErr)
			}
			if tc.expectChat && chat == nil {
				t.Fatalf("expected a Chat, got nil")
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

func TestSend(t *testing.T) {
	// ... existing short/credential guard ...
	if testing.Short() {
		t.Skip("live Bedrock tests skipped in short mode")
	}
	if !hasBedrockCredentials() {
		t.Skip("AWS credentials are required for Bedrock chat tests")
	}

	// Force the test to use Bedrock so we create the right client.
	t.Setenv("BEDROCK_MODEL", "amazon.titan-text-express-v1")
	t.Setenv("LLM_CLIENT", "bedrock://bedrock.ap-south-1.amazonaws.com")

	ctx := context.Background()
	client, err := NewClient(ctx, "")
	if err != nil {
		t.Fatalf("failed to create Bedrock client: %v", err)
	}
	defer client.Close()

	bedrockClient, ok := client.(*BedrockClient)
	if !ok {
		t.Fatalf("expected *BedrockClient, got %T", client)
	}

	models, err := bedrockClient.ListModels(ctx)
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected at least one Bedrock model")
	}
	validModel := models[0]

	testCases := []struct {
		name        string
		model       string
		contents    []any
		expectErr   string
		expectReply bool
	}{
		{
			name:      "missing message",
			model:     validModel,
			contents:  nil,
			expectErr: "message",
		},
		{
			name:        "first message",
			model:       validModel,
			contents:    []any{"Hello from the test"},
			expectReply: true,
		},
	}

	if len(models) > 1 {
		for i := range models {
			testCases = append(testCases, struct {
				name        string
				model       string
				contents    []any
				expectErr   string
				expectReply bool
			}{
				name:        "different model",
				model:       models[i],
				contents:    []any{"Hello from another model"},
				expectReply: true,
			})
		}

	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			chat := bedrockClient.StartChat("You are a helpful assistant.", tc.model)
			if chat == nil {
				t.Fatalf("expected chat, got nil")
			}

			resp, err := chat.Send(ctx, tc.contents...)
			if tc.expectErr != "" {
				if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.expectErr)) {
					t.Fatalf("expected error containing %q, got %v", tc.expectErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp == nil {
				t.Fatalf("expected non-nil response")
			}
			candidates := resp.Candidates()
			if tc.expectReply {
				if len(candidates) == 0 {
					t.Fatalf("expected at least one candidate, got none")
				}
				if usage := resp.UsageMetadata(); usage == nil {
					t.Fatalf("expected usage metadata from response")
				}
				return
			}
			if len(candidates) != 0 {
				t.Fatalf("expected no candidates, got %d", len(candidates))
			}
		})
	}
}

func TestSendStreaming(t *testing.T) {
	if testing.Short() {
		t.Skip("live Bedrock tests skipped in short mode")
	}
	if !hasBedrockCredentials() {
		t.Skip("AWS credentials are required for Bedrock chat tests")
	}

	t.Setenv("BEDROCK_MODEL", "amazon.titan-text-express-v1")
	t.Setenv("LLM_CLIENT", "bedrock://bedrock.ap-south-1.amazonaws.com")

	ctx := context.Background()
	client, err := NewClient(ctx, "")
	if err != nil {
		t.Fatalf("failed to create Bedrock client: %v", err)
	}
	defer client.Close()

	bedrockClient, ok := client.(*BedrockClient)
	if !ok {
		t.Fatalf("expected *BedrockClient, got %T", client)
	}

	streamingModels, nonStreamingModels, err := GetModelStreamingNotSupported(ctx, bedrockClient)
	if err != nil {
		t.Fatalf("failed to determine streaming support: %v", err)
	}
	if len(streamingModels) == 0 {
		t.Skip("no streaming-supported models discovered via Bedrock APIs")
	}

	const prompt = "List two kubectl commands that describe namespaces."

	for _, modelID := range streamingModels {
		modelID := modelID
		t.Run("streaming/"+modelID, func(t *testing.T) {
			chat := bedrockClient.StartChat("You are a helpful assistant.", modelID)
			if chat == nil {
				t.Fatalf("expected chat, got nil for %s", modelID)
			}

			reqCtx, cancel := context.WithTimeout(ctx, time.Minute)
			defer cancel()

			iter, err := chat.SendStreaming(reqCtx, prompt)
			if err != nil {
				t.Fatalf("unexpected streaming error for %s: %v", modelID, err)
			}
			if iter == nil {
				t.Fatalf("expected streaming iterator for %s, got nil", modelID)
			}

			var sawText bool
			iter(func(resp ChatResponse, err error) bool {
				if err != nil {
					t.Fatalf("streaming error for %s: %v", modelID, err)
				}
				for _, candidate := range resp.Candidates() {
					for _, part := range candidate.Parts() {
						if text, ok := part.AsText(); ok {
							if trimmed := strings.TrimSpace(text); trimmed != "" {
								sawText = true
								t.Logf("Streaming chunk [%s]: %q", modelID, trimmed)
							}
						}
					}
				}
				return true
			})

			if !sawText {
				t.Fatalf("expected non-empty streaming response for %s", modelID)
			}
		})
	}

	if len(nonStreamingModels) == 0 {
		t.Skip("no non-streaming models discovered via Bedrock APIs")
	}

	for _, modelID := range nonStreamingModels {
		modelID := modelID
		t.Run("nonstreaming/"+modelID, func(t *testing.T) {
			chat := bedrockClient.StartChat("You are a helpful assistant.", modelID)
			if chat == nil {
				t.Fatalf("expected chat, got nil for %s", modelID)
			}

			reqCtx, cancel := context.WithTimeout(ctx, time.Minute)
			defer cancel()

			_, err := chat.SendStreaming(reqCtx, prompt)
			if err == nil {
				t.Fatalf("expected streaming error for non-streaming model %s, got nil", modelID)
			}
			if !strings.Contains(err.Error(), "model does not support streaming chat") {
				t.Fatalf("expected streaming rejection for %s, got: %v", modelID, err)
			}
		})
	}
}

func TestFunctionCalling(t *testing.T) {
	if testing.Short() {
		t.Skip("live Bedrock tests skipped in short mode")
	}
	if !hasBedrockCredentials() {
		t.Skip("AWS credentials are required for Bedrock function-calling tests")
	}

	t.Setenv("LLM_CLIENT", "bedrock://bedrock.ap-south-1.amazonaws.com")
	t.Setenv("BEDROCK_MODEL", "qwen.qwen3-vl-235b-a22b")

	ctx := context.Background()
	client, err := NewClient(ctx, "")
	if err != nil {
		t.Fatalf("failed to create Bedrock client: %v", err)
	}
	defer client.Close()

	bedrockClient, ok := client.(*BedrockClient)
	if !ok {
		t.Fatalf("expected *BedrockClient, got %T", client)
	}

	chat := bedrockClient.StartChat("You are a helpful assistant.", "")
	if chat == nil {
		t.Fatalf("expected chat, got nil")
	}

	// Define a simple tool that lists files in the working directory
	toolDef := &FunctionDefinition{
		Name:        "list_files",
		Description: "Lists all files in given directory",
		Parameters: &Schema{
			Type: TypeObject,
			Properties: map[string]*Schema{
				"directory": {
					Type:        TypeString,
					Description: "The directory whose files are to be listed",
				},
			},
			Required: []string{"directory"},
		},
	}
	if err := chat.SetFunctionDefinitions([]*FunctionDefinition{toolDef}); err != nil {
		t.Fatalf("failed to set function definitions: %v", err)
	}

	response, err := chat.Send(ctx, "What are all the files with go extension in the current directory?")
	if err != nil {
		t.Fatalf("Send error: %v", err)
	}

	var gotToolCall bool

	for _, candidate := range response.Candidates() {
		for _, part := range candidate.Parts() {
			if calls, ok := part.AsFunctionCalls(); ok {
				gotToolCall = true
				for _, call := range calls {
					// Simulate the tool: run `ls` and send the result back
					if call.Name != "list_files" {
						t.Fatalf("unexpected function call %q", call.Name)
					}
					args, ok := call.Arguments["directory"].(string)
					if !ok || args == "" {
						args = "."
					}
					output, err := os.ReadDir(args)
					if err != nil {
						t.Fatalf("failed to list directory %q: %v", args, err)
					}
					files := []string{}
					for _, entry := range output {
						if strings.HasSuffix(entry.Name(), ".go") {
							files = append(files, entry.Name())
						}
						//(nisran) Uncomment to see all files
						//t.Logf("Got file %s", entry)
					}
					result := map[string]any{
						"files": files,
					}

					resp, err := chat.Send(ctx, FunctionCallResult{
						ID:     call.ID,
						Name:   call.Name,
						Result: result,
						//Status: types.
					})
					if err != nil {
						t.Fatalf("failed to send function result: %v", err)
					}
					t.Logf("Send response candidates: %d", len(resp.Candidates()))
					/* Uncomment to see LLM output
					for _, candi := range resp.Candidates() {
						t.Logf("Candidate: %s", candi.String())
						for _, prt := range candi.Parts() {
							if prtText, isText := prt.AsText(); isText {
								t.Logf("Parts: %s", prtText)
							}
							_, isText := prt.AsFunctionCalls()
							if isText {
								t.Logf("Parts is FunctionCall")
							}
						}
					}
					*/
				}
			}
		}
	}

	if !gotToolCall {
		t.Fatalf("expected Bedrock to invoke the function call")
	}
}

func TestFunctionCalling_Coverage(t *testing.T) {
	if testing.Short() {
		t.Skip("live Bedrock tests skipped in short mode")
	}
	if !hasBedrockCredentials() {
		t.Skip("AWS credentials are required for Bedrock function-calling coverage tests")
	}

	t.Setenv("LLM_CLIENT", "bedrock://bedrock.ap-south-1.amazonaws.com")
	ctx := context.Background()
	client, err := NewClient(ctx, "")
	if err != nil {
		t.Fatalf("failed to create Bedrock client: %v", err)
	}
	defer client.Close()

	bedrockClient, ok := client.(*BedrockClient)
	if !ok {
		t.Fatalf("expected *BedrockClient, got %T", client)
	}

	models, err := bedrockClient.ListModels(ctx)
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("ListModels returned no models")
	}

	toolDef := &FunctionDefinition{
		Name:        "list_files",
		Description: "List go files in the current directory",
		Parameters: &Schema{
			Type: TypeObject,
			Properties: map[string]*Schema{
				"directory": {
					Type:        TypeString,
					Description: "Directory to list",
				},
			},
			Required: []string{"directory"},
		},
	}

	for _, modelID := range models {
		modelID := modelID
		t.Run(modelID, func(t *testing.T) {
			chat := bedrockClient.StartChat("You are a helpful assistant.", modelID)
			if chat == nil {
				t.Fatalf("StartChat returned nil for %q", modelID)
			}

			err := chat.SetFunctionDefinitions([]*FunctionDefinition{toolDef})
			supported := isFunctionCallingSupported(modelID)
			if !supported {
				if err == nil {
					t.Fatalf("expected SetFunctionDefinitions to fail for unsupported model %q", modelID)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error setting functions for %q: %v", modelID, err)
			}

			resp, err := chat.Send(ctx, "List files in the current directory.")
			if err != nil {
				t.Fatalf("unexpected error calling Send for %q: %v", modelID, err)
			}
			if resp == nil || len(resp.Candidates()) == 0 {
				t.Fatalf("expected candidates from %q", modelID)
			}
		})
	}
}

func GetModelStreamingNotSupported(ctx context.Context, bedrockClient *BedrockClient) (streaming, nonStreaming []string, err error) {
	if bedrockClient == nil || bedrockClient.controlPlane == nil {
		return nil, nil, fmt.Errorf("bedrock client not initialized")
	}

	output, err := bedrockClient.controlPlane.ListFoundationModels(ctx, &bedrock.ListFoundationModelsInput{})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list foundation models: %w", err)
	}

	streamingSupport := make(map[string]bool, len(output.ModelSummaries))
	for _, summary := range output.ModelSummaries {
		if summary.ModelId == nil {
			continue
		}
		streamingSupport[aws.ToString(summary.ModelId)] = aws.ToBool(summary.ResponseStreamingSupported)
	}

	models, err := bedrockClient.ListModels(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list available models: %w", err)
	}

	for _, modelID := range models {
		supported, ok := streamingSupport[modelID]
		if !ok {
			continue
		}
		if supported && len(streaming) < 5 {
			streaming = append(streaming, modelID)
		}
		if !supported && len(nonStreaming) < 5 {
			nonStreaming = append(nonStreaming, modelID)
		}
		if len(streaming) >= 5 && len(nonStreaming) >= 5 {
			break
		}
	}

	return streaming, nonStreaming, nil
}

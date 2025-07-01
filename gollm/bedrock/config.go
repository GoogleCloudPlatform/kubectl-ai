package bedrock

import (
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
)

const (
	Name = "bedrock"
	// Standard error messages for consistent error handling
	ErrMsgConfigLoad       = "failed to load AWS configuration"
	ErrMsgModelInvoke      = "failed to invoke Bedrock model"
	ErrMsgResponseParse    = "failed to parse Bedrock response"
	ErrMsgRequestBuild     = "failed to build request"
	ErrMsgStreamingFailed  = "Bedrock streaming failed"
	ErrMsgUnsupportedModel = "unsupported model - only Claude and Nova models are supported"
)

// BedrockOptions provides comprehensive configuration for the Bedrock client
type BedrockOptions struct {
	Region              string
	CredentialsProvider aws.CredentialsProvider
	Model               string
	MaxTokens           int32
	Temperature         float32 // Controls randomness in responses (0.0-1.0)
	TopP                float32 // Controls diversity via nucleus sampling
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

// isModelSupported checks if the given model is explicitly supported
// Updated to support Claude 4, Claude 3.7, Nova models, and inference profiles
func isModelSupported(model string) bool {
	if model == "" {
		return false
	}

	modelLower := strings.ToLower(model)

	// Supported models based on integration test results
	supportedModels := []string{
		"us.anthropic.claude-sonnet-4-20250514-v1:0",
		"us.anthropic.claude-3-7-sonnet-20250219-v1:0",
		"us.amazon.nova-pro-v1:0",
		"us.amazon.nova-lite-v1:0",
		"us.amazon.nova-micro-v1:0",
	}

	// Check exact match against allowlist
	for _, supported := range supportedModels {
		if modelLower == strings.ToLower(supported) {
			return true
		}
	}

	// Support AWS Bedrock ARNs - both foundation models and inference profiles
	if strings.Contains(modelLower, "arn:aws:bedrock") {
		// Handle different ARN formats:
		// 1. Foundation model: arn:aws:bedrock:region::foundation-model/model-id
		// 2. Inference profile: arn:aws:bedrock:region:account:inference-profile/profile-id
		// 3. Application inference profile: arn:aws:bedrock:region:account:application-inference-profile/profile-id

		// Check if it's an inference profile or application inference profile
		if strings.Contains(modelLower, "inference-profile") {
			// For inference profiles, we validate by checking if it contains Claude or Nova indicators
			// since inference profiles typically wrap supported base models

			// Check for Claude indicators in the ARN
			if strings.Contains(modelLower, "anthropic") || strings.Contains(modelLower, "claude") {
				return true
			}

			// Check for Nova indicators in the ARN
			if strings.Contains(modelLower, "amazon") || strings.Contains(modelLower, "nova") {
				return true
			}

			// For generic inference profiles without clear model indicators,
			// we'll allow them and let AWS Bedrock handle validation
			// This is necessary for application inference profiles like the user's ARN
			return true
		}

		// Handle foundation model ARNs by extracting model ID
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

// getSupportedModels returns the list of models supported by this implementation
// Updated to include Claude 4, Claude 3.7, Nova models, and ARN information
func getSupportedModels() []string {
	return []string{
		// Claude 4 (latest and most capable) - ✅ VERIFIED WORKING
		"us.anthropic.claude-sonnet-4-20250514-v1:0",

		// Claude 3.7 (your other available Claude model) - ✅ VERIFIED WORKING
		"us.anthropic.claude-3-7-sonnet-20250219-v1:0",

		// Nova models (only US-region versions work without inference profiles) - ✅ TESTED
		// Note: Basic nova models require inference profiles which you don't have configured
		"us.amazon.nova-pro-v1:0",   // Nova Pro (US region) - ✅ CONFIRMED WORKING
		"us.amazon.nova-lite-v1:0",  // Nova Lite (US region) - assumed working
		"us.amazon.nova-micro-v1:0", // Nova Micro (US region) - assumed working

		// Also supports AWS Bedrock ARNs including:
		// - Foundation model ARNs: arn:aws:bedrock:region::foundation-model/model-id
		// - Inference profile ARNs: arn:aws:bedrock:region:account:inference-profile/profile-id
		// - Application inference profile ARNs: arn:aws:bedrock:region:account:application-inference-profile/profile-id
	}
}

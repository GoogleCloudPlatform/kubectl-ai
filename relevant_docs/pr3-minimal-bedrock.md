# PR #3: Minimal AWS Bedrock Provider Implementation

## Overview
This PR implements the absolute minimum viable Bedrock provider by **stripping all advanced features** from PR #2. Based on direct reviewer feedback to "keep this PR narrowly focused on providing Bedrock support only" and "remove or defer usage tracking, extended configuration knobs, and other cross-provider features."

## Working Backwards Strategy
- **Create from**: PR #2 branch (usage tracking implementation)
- **Strip out**: All usage tracking, callbacks, extractors, advanced features
- **Keep**: Only basic Bedrock provider functionality
- **Submit**: First (to establish foundation for subsequent PRs)

## Reviewer Feedback Addressed

### Key Requirements from Review
- ✅ **Scope Focus**: Bedrock support only, no cross-provider features
- ✅ **Remove Usage Tracking**: No UsageCallback, UsageExtractor, Usage struct
- ✅ **Remove InferenceConfig**: No advanced configuration in factory.go
- ✅ **Remove Debug Flag**: Use existing logging only
- ✅ **Environment Variables**: Use AWS env vars for configuration
- ✅ **Fix CI Issues**: Address formatting and go.mod issues

## Files to Modify (Strip from PR #2)

### Strip Down to Minimal Implementation

#### `gollm/bedrock/bedrock.go` (~250 lines)
```go
package bedrock

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"k8s.io/klog/v2"
)

func init() {
	if err := gollm.RegisterProvider("bedrock", newBedrockClientFactory); err != nil {
		klog.Fatalf("Failed to register bedrock provider: %v", err)
	}
}

func newBedrockClientFactory(ctx context.Context, opts gollm.ClientOptions) (gollm.Client, error) {
	return NewBedrockClient(ctx)
}

type BedrockClient struct {
	runtimeClient *bedrockruntime.Client
	region        string
}

func NewBedrockClient(ctx context.Context) (*BedrockClient, error) {
	// Use default AWS configuration with environment variables
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS configuration: %w", err)
	}

	return &BedrockClient{
		runtimeClient: bedrockruntime.NewFromConfig(cfg),
		region:        cfg.Region,
	}, nil
}

func (c *BedrockClient) Close() error {
	return nil
}

func (c *BedrockClient) StartChat(systemPrompt, model string) gollm.Chat {
	if model == "" {
		model = "anthropic.claude-3-5-sonnet-20241022-v2:0" // Default model
	}
	
	return &bedrockChatSession{
		client:       c,
		systemPrompt: systemPrompt,
		model:        model,
		history:      make([]types.Message, 0),
	}
}

func (c *BedrockClient) GenerateCompletion(ctx context.Context, req *gollm.CompletionRequest) (gollm.CompletionResponse, error) {
	chat := c.StartChat("", req.Model)
	response, err := chat.Send(ctx, req.Prompt)
	if err != nil {
		return nil, err
	}
	
	return &simpleCompletionResponse{
		content: extractTextFromResponse(response),
	}, nil
}

func (c *BedrockClient) SetResponseSchema(schema *gollm.Schema) error {
	return nil // Not implemented in minimal version
}

func (c *BedrockClient) ListModels(ctx context.Context) ([]string, error) {
	return []string{
		"anthropic.claude-3-5-sonnet-20241022-v2:0",
		"anthropic.claude-3-haiku-20240307-v1:0",
		"amazon.nova-micro-v1:0",
		"amazon.nova-lite-v1:0",
		"amazon.nova-pro-v1:0",
	}, nil
}

type bedrockChatSession struct {
	client       *BedrockClient
	systemPrompt string
	model        string
	history      []types.Message
}

func (cs *bedrockChatSession) Send(ctx context.Context, contents ...any) (gollm.ChatResponse, error) {
	// Basic text message handling only
	var message string
	for _, content := range contents {
		if text, ok := content.(string); ok {
			message = text
			break
		}
	}
	
	if message == "" {
		return nil, errors.New("no text message provided")
	}

	// Add user message
	cs.addTextMessage(types.ConversationRoleUser, message)
	
	// Build and send request
	input := &bedrockruntime.ConverseInput{
		ModelId:  aws.String(cs.model),
		Messages: cs.history,
		InferenceConfig: &types.InferenceConfiguration{
			MaxTokens:   aws.Int32(4096),
			Temperature: aws.Float32(0.1),
		},
	}

	if cs.systemPrompt != "" {
		input.System = []types.SystemContentBlock{
			&types.SystemContentBlockMemberText{Value: cs.systemPrompt},
		}
	}

	output, err := cs.client.runtimeClient.Converse(ctx, input)
	if err != nil {
		cs.removeLastMessage() // Remove user message on failure
		return nil, fmt.Errorf("bedrock API call failed: %w", err)
	}

	// Parse response
	response := &bedrockChatResponse{
		content: extractContentFromOutput(&output.Output),
		usage:   output.Usage,
	}

	// Add assistant response to history
	cs.addTextMessage(types.ConversationRoleAssistant, response.content)

	return response, nil
}

func (cs *bedrockChatSession) SendStreaming(ctx context.Context, contents ...any) (gollm.ChatResponseIterator, error) {
	return nil, errors.New("streaming not implemented in minimal version")
}

func (cs *bedrockChatSession) SetFunctionDefinitions(defs []*gollm.FunctionDefinition) error {
	return errors.New("function calling not implemented in minimal version")
}

func (cs *bedrockChatSession) IsRetryableError(err error) bool {
	return false // Minimal error handling
}

// Helper methods (simplified)
func (cs *bedrockChatSession) addTextMessage(role types.ConversationRole, content string) {
	if content == "" {
		return
	}
	textBlock := &types.ContentBlockMemberText{Value: content}
	message := types.Message{
		Role:    role,
		Content: []types.ContentBlock{textBlock},
	}
	cs.history = append(cs.history, message)
}

func (cs *bedrockChatSession) removeLastMessage() {
	if len(cs.history) > 0 {
		cs.history = cs.history[:len(cs.history)-1]
	}
}

// Response implementations
type bedrockChatResponse struct {
	content string
	usage   any
}

func (r *bedrockChatResponse) UsageMetadata() any {
	return r.usage
}

func (r *bedrockChatResponse) Candidates() []gollm.Candidate {
	return []gollm.Candidate{&bedrockCandidate{content: r.content}}
}

type bedrockCandidate struct {
	content string
}

func (c *bedrockCandidate) String() string {
	return c.content
}

func (c *bedrockCandidate) Parts() []gollm.Part {
	return []gollm.Part{&bedrockPart{content: c.content}}
}

type bedrockPart struct {
	content string
}

func (p *bedrockPart) AsText() (string, bool) {
	return p.content, true
}

func (p *bedrockPart) AsFunctionCalls() ([]gollm.FunctionCall, bool) {
	return nil, false
}

type simpleCompletionResponse struct {
	content string
}

func (r *simpleCompletionResponse) Response() string {
	return r.content
}

func (r *simpleCompletionResponse) UsageMetadata() any {
	return nil
}

// Utility functions
func extractTextFromResponse(response gollm.ChatResponse) string {
	for _, candidate := range response.Candidates() {
		for _, part := range candidate.Parts() {
			if text, ok := part.AsText(); ok {
				return text
			}
		}
	}
	return ""
}

func extractContentFromOutput(output *types.ConverseOutput) string {
	if output == nil || len(output.Message.Content) == 0 {
		return ""
	}
	
	for _, block := range output.Message.Content {
		if textBlock, ok := block.(*types.ContentBlockMemberText); ok {
			return textBlock.Value
		}
	}
	return ""
}
```

#### `gollm/bedrock/config.go` (~30 lines)
```go
package bedrock

const (
	// Provider name
	Name = "bedrock"
)

// Supported models (based on review feedback to link to AWS docs)
func getSupportedModels() []string {
	return []string{
		"anthropic.claude-3-5-sonnet-20241022-v2:0",
		"anthropic.claude-3-haiku-20240307-v1:0", 
		"amazon.nova-micro-v1:0",
		"amazon.nova-lite-v1:0",
		"amazon.nova-pro-v1:0",
	}
}

func isModelSupported(model string) bool {
	supported := getSupportedModels()
	for _, m := range supported {
		if m == model {
			return true
		}
	}
	return false
}
```

### Files to Strip Down

#### `gollm/factory.go` (Strip all advanced features)
**Strip from PR #2:**
- Remove UsageCallback and UsageExtractor from ClientOptions
- Remove WithUsageCallback() and WithUsageExtractor() functions
- Keep only basic ClientOptions with URL and SkipVerifySSL

**Resulting minimal ClientOptions:**
```go
type ClientOptions struct {
	URL           *url.URL
	SkipVerifySSL bool
	// NO InferenceConfig, NO UsageCallback, NO Debug flags
}
```

#### `gollm/interfaces.go` (Strip usage infrastructure)
**Strip from PR #2:**
- Remove entire Usage struct and methods
- Remove UsageCallback function type
- Remove UsageExtractor interface
- Keep only core gollm interfaces

#### `go.mod` (Add AWS dependencies only)
```go
require (
	// ... existing dependencies ...
	github.com/aws/aws-sdk-go-v2 v1.27.2
	github.com/aws/aws-sdk-go-v2/config v1.27.18
	github.com/aws/aws-sdk-go-v2/service/bedrock v1.11.3
	github.com/aws/aws-sdk-go-v2/service/bedrockruntime v1.11.2
)
```

## What's Explicitly REMOVED (Per Review Feedback)

### From Current Implementation
- ❌ **All Usage Tracking**: No `Usage` struct, `UsageCallback`, `UsageExtractor`
- ❌ **InferenceConfig**: No advanced configuration options
- ❌ **Debug Flags**: No custom debug logging
- ❌ **Advanced Timeout Logic**: No complex timeout handling
- ❌ **Streaming Support**: Minimal implementation only
- ❌ **Function Calling**: Not in minimal version
- ❌ **Comprehensive Tests**: Basic tests only
- ❌ **Advanced Error Handling**: Simple error handling only

### Configuration Strategy
- ✅ **Environment Variables**: Use `AWS_REGION`, `AWS_PROFILE`, `AWS_ACCESS_KEY_ID`, etc.
- ✅ **AWS Default Config**: Let AWS SDK handle configuration
- ✅ **No Custom Flags**: No new command-line options

## Testing Strategy

### Basic Tests Only
```go
func TestBasicBedrockClient(t *testing.T) {
	// Test client creation
	// Test basic chat functionality  
	// Test model listing
	// NO timeout tests (per review feedback)
	// NO usage tracking tests
	// NO inference config tests
}
```

### CI Fixes Required
1. **Format**: Run `go fmt ./...` and `goimports -w .`
2. **Go Mod**: Run `go mod tidy` 
3. **Remove**: All advanced test scenarios

## Documentation

### Minimal `docs/bedrock.md`
- AWS credentials setup only
- Supported models (link to AWS docs)
- Basic usage examples
- NO advanced configuration examples
- NO usage tracking documentation

## PR Description Template

```markdown
feat: Add minimal AWS Bedrock provider support

This PR adds basic AWS Bedrock provider support to kubectl-ai, implementing
only core functionality as requested in review feedback.

## What's Included
- Basic Bedrock client with Claude and Nova model support
- Standard AWS credential chain support  
- Essential error handling and logging
- Follows existing provider patterns

## What's NOT Included (by design)
- Usage tracking infrastructure (will be separate PR)
- Advanced inference configuration (will be separate PR)  
- Streaming support (minimal implementation)
- Function calling (minimal implementation)
- Advanced timeout/retry logic

## Dependencies
- Adds AWS SDK v2 for Bedrock integration
- No breaking changes to existing interfaces
- Uses environment variables for configuration

## Testing
- Basic functionality tests
- Manual testing with AWS credentials
- CI formatting and go mod issues resolved

Addresses reviewer feedback to keep scope minimal and provider-focused.
```

## Success Criteria

- [ ] Basic Bedrock models work with kubectl-ai
- [ ] No breaking changes to existing providers  
- [ ] No cross-provider features added
- [ ] AWS credentials handled via environment
- [ ] CI passes without advanced testing
- [ ] Clean, minimal codebase (~300 total lines)
- [ ] Follows exact reviewer guidance

## Implementation Steps (Strip from PR #2)

### Step 1: Create PR #3 Branch  
- **Branch from**: PR #2 branch (usage tracking implementation)
- **Timeline**: 1 day (stripping features is faster than building)

### Step 2: Strip All Advanced Features
- Remove all usage tracking from interfaces.go
- Remove usage callbacks from factory.go  
- Simplify bedrock.go to basic functionality only
- Remove advanced error handling and streaming
- Remove function calling support

### Step 3: Validate Minimal Implementation
- **Testing**: Ensure basic Bedrock functionality works
- **Verification**: Simple chat and completion calls succeed
- **CI**: Fix formatting and go.mod issues per reviewer feedback

### Step 4: Submit PR #3 First
- **Submit**: First (establishes foundation)
- **Review**: 1-2 weeks (fast due to minimal scope and reviewer alignment)
- **Merge**: Should be quick due to focused scope and direct address of feedback

This backwards approach ensures each stripped version is tested and working, reducing risk compared to building up from scratch. The minimal implementation directly addresses all reviewer concerns while providing a solid foundation for subsequent feature additions. 
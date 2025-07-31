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

package gollm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"k8s.io/klog/v2"
)

// Register the Bedrock provider factory on package initialization
func init() {
	if err := RegisterProvider("bedrock", newBedrockClientFactory); err != nil {
		klog.Fatalf("Failed to register bedrock provider: %v", err)
	}
}

// newBedrockClientFactory creates a new Bedrock client with the given options
func newBedrockClientFactory(ctx context.Context, opts ClientOptions) (Client, error) {
	return NewBedrockClient(ctx, opts)
}

// BedrockClient implements the gollm.Client interface for AWS Bedrock models
type BedrockClient struct {
	client *bedrockruntime.Client
}

// Ensure BedrockClient implements the Client interface
var _ Client = &BedrockClient{}

// NewBedrockClient creates a new client for interacting with AWS Bedrock models
func NewBedrockClient(ctx context.Context, opts ClientOptions) (*BedrockClient, error) {
	// Load AWS config with timeout protection
	configCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cfg, err := config.LoadDefaultConfig(configCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Default to us-east-1 for Bedrock if no region is set
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}

	return &BedrockClient{
		client: bedrockruntime.NewFromConfig(cfg),
	}, nil
}

// Close cleans up any resources used by the client
func (c *BedrockClient) Close() error {
	return nil
}

// StartChat starts a new chat session with the specified system prompt and model
func (c *BedrockClient) StartChat(systemPrompt, model string) Chat {
	selectedModel := getBedrockModel(model)

	klog.V(1).Infof("Starting new Bedrock chat session with model: %s", selectedModel)

	return &bedrockChat{
		client:       c,
		systemPrompt: systemPrompt,
		model:        selectedModel,
		messages:     []types.Message{},
	}
}

// GenerateCompletion generates a single completion for the given request
func (c *BedrockClient) GenerateCompletion(ctx context.Context, req *CompletionRequest) (CompletionResponse, error) {
	chat := c.StartChat("", req.Model)
	chatResponse, err := chat.Send(ctx, req.Prompt)
	if err != nil {
		return nil, err
	}

	// Wrap ChatResponse in a CompletionResponse
	return &bedrockCompletionResponse{
		chatResponse: chatResponse,
	}, nil
}

// SetResponseSchema sets the response schema for the client (not supported by Bedrock)
func (c *BedrockClient) SetResponseSchema(schema *Schema) error {
	return fmt.Errorf("response schema not supported by Bedrock")
}

// ListModels returns the list of supported Bedrock models
func (c *BedrockClient) ListModels(ctx context.Context) ([]string, error) {
	return []string{
		"us.anthropic.claude-sonnet-4-20250514-v1:0",   // Claude Sonnet 4 (default)
		"us.anthropic.claude-3-7-sonnet-20250219-v1:0", // Claude 3.7 Sonnet
	}, nil
}

// bedrockChat implements the Chat interface for Bedrock conversations
type bedrockChat struct {
	client       *BedrockClient
	systemPrompt string
	model        string
	messages     []types.Message
	toolConfig   *types.ToolConfiguration
	functionDefs []*FunctionDefinition
}

// Send sends a message to the chat and returns the response
func (c *bedrockChat) Send(ctx context.Context, contents ...any) (ChatResponse, error) {
	if len(contents) == 0 {
		return nil, errors.New("no content provided")
	}

	// Convert content to string
	message := fmt.Sprintf("%v", contents[0])

	// Add user message to conversation history
	c.messages = append(c.messages, types.Message{
		Role: types.ConversationRoleUser,
		Content: []types.ContentBlock{
			&types.ContentBlockMemberText{Value: message},
		},
	})

	// Prepare the request
	input := &bedrockruntime.ConverseInput{
		ModelId:  aws.String(c.model),
		Messages: c.messages,
		InferenceConfig: &types.InferenceConfiguration{
			MaxTokens: aws.Int32(4096),
		},
	}

	// Add system prompt if provided
	if c.systemPrompt != "" {
		input.System = []types.SystemContentBlock{
			&types.SystemContentBlockMemberText{Value: c.systemPrompt},
		}
	}

	// Add tool configuration if functions are defined
	if c.toolConfig != nil {
		input.ToolConfig = c.toolConfig
	}

	// Call the Bedrock Converse API
	output, err := c.client.client.Converse(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("bedrock converse error: %w", err)
	}

	// Extract response content and update conversation history
	response := &bedrockResponse{
		output: output,
		model:  c.model,
	}

	// Update conversation history with assistant's response
	if output.Output != nil {
		if msg, ok := output.Output.(*types.ConverseOutputMemberMessage); ok {
			c.messages = append(c.messages, msg.Value)
		}
	}

	return response, nil
}

// SendStreaming sends a message and returns a streaming response
func (c *bedrockChat) SendStreaming(ctx context.Context, contents ...any) (ChatResponseIterator, error) {
	if len(contents) == 0 {
		return nil, errors.New("no content provided")
	}

	// Convert content to string
	message := fmt.Sprintf("%v", contents[0])

	// Add user message to conversation history
	c.messages = append(c.messages, types.Message{
		Role: types.ConversationRoleUser,
		Content: []types.ContentBlock{
			&types.ContentBlockMemberText{Value: message},
		},
	})

	// Prepare the streaming request
	input := &bedrockruntime.ConverseStreamInput{
		ModelId:  aws.String(c.model),
		Messages: c.messages,
		InferenceConfig: &types.InferenceConfiguration{
			MaxTokens: aws.Int32(4096),
		},
	}

	// Add system prompt if provided
	if c.systemPrompt != "" {
		input.System = []types.SystemContentBlock{
			&types.SystemContentBlockMemberText{Value: c.systemPrompt},
		}
	}

	// Add tool configuration if functions are defined
	if c.toolConfig != nil {
		input.ToolConfig = c.toolConfig
	}

	// Start the streaming request
	output, err := c.client.client.ConverseStream(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("bedrock stream error: %w", err)
	}

	// Return streaming iterator
	return func(yield func(ChatResponse, error) bool) {
		defer func() {
			if stream := output.GetStream(); stream != nil {
				stream.Close()
			}
		}()

		var assistantMessage types.Message
		assistantMessage.Role = types.ConversationRoleAssistant
		var fullContent strings.Builder

		// Process streaming events
		stream := output.GetStream()
		for event := range stream.Events() {
			switch v := event.(type) {
			case *types.ConverseStreamOutputMemberContentBlockDelta:
				// Handle text deltas
				if textDelta, ok := v.Value.Delta.(*types.ContentBlockDeltaMemberText); ok {
					fullContent.WriteString(textDelta.Value)

					response := &bedrockStreamResponse{
						content: textDelta.Value,
						model:   c.model,
						done:    false,
					}

					if !yield(response, nil) {
						return
					}
				}

			case *types.ConverseStreamOutputMemberContentBlockStart:
				// Handle content block start (for tool calls)
				if v.Value.Start != nil {
					klog.V(3).Infof("Content block started at index: %v", aws.ToInt32(v.Value.ContentBlockIndex))
				}

			case *types.ConverseStreamOutputMemberMetadata:
				// Handle final usage metadata
				if v.Value.Usage != nil {
					finalResponse := &bedrockStreamResponse{
						content: "",
						usage:   v.Value.Usage,
						model:   c.model,
						done:    true,
					}
					yield(finalResponse, nil)
				}
			}
		}

		// Update conversation history with the full response
		if fullContent.Len() > 0 {
			assistantMessage.Content = append(assistantMessage.Content,
				&types.ContentBlockMemberText{Value: fullContent.String()})
			c.messages = append(c.messages, assistantMessage)
		}

		// Check for stream errors
		if err := stream.Err(); err != nil {
			yield(nil, fmt.Errorf("stream error: %w", err))
		}
	}, nil
}

// SetFunctionDefinitions configures the available functions for tool use
func (c *bedrockChat) SetFunctionDefinitions(functions []*FunctionDefinition) error {
	c.functionDefs = functions

	if len(functions) == 0 {
		c.toolConfig = nil
		return nil
	}

	var tools []types.Tool
	for _, fn := range functions {
		// Convert gollm function definition to AWS tool specification
		inputSchema := make(map[string]interface{})
		if fn.Parameters != nil {
			// Convert Schema to map[string]interface{}
			jsonData, err := json.Marshal(fn.Parameters)
			if err != nil {
				return fmt.Errorf("failed to marshal function parameters: %w", err)
			}
			if err := json.Unmarshal(jsonData, &inputSchema); err != nil {
				return fmt.Errorf("failed to unmarshal function parameters: %w", err)
			}
		}

		toolSpec := types.ToolSpecification{
			Name:        aws.String(fn.Name),
			Description: aws.String(fn.Description),
			InputSchema: &types.ToolInputSchemaMemberJson{
				Value: document.NewLazyDocument(inputSchema),
			},
		}

		tools = append(tools, &types.ToolMemberToolSpec{Value: toolSpec})
	}

	c.toolConfig = &types.ToolConfiguration{
		Tools: tools,
	}

	return nil
}

// IsRetryableError determines if an error is retryable
func (c *bedrockChat) IsRetryableError(err error) bool {
	return DefaultIsRetryableError(err)
}

// bedrockResponse implements ChatResponse for regular (non-streaming) responses
type bedrockResponse struct {
	output *bedrockruntime.ConverseOutput
	model  string
}

// UsageMetadata returns the usage metadata from the response
func (r *bedrockResponse) UsageMetadata() any {
	if r.output != nil && r.output.Usage != nil {
		return r.output.Usage
	}
	return nil
}

// Candidates returns the candidate responses
func (r *bedrockResponse) Candidates() []Candidate {
	if r.output == nil || r.output.Output == nil {
		return []Candidate{}
	}

	if msg, ok := r.output.Output.(*types.ConverseOutputMemberMessage); ok {
		candidate := &bedrockCandidate{
			message: &msg.Value,
			model:   r.model,
		}
		return []Candidate{candidate}
	}

	return []Candidate{}
}

// bedrockStreamResponse implements ChatResponse for streaming responses
type bedrockStreamResponse struct {
	content string
	usage   *types.TokenUsage
	model   string
	done    bool
}

// UsageMetadata returns the usage metadata from the streaming response
func (r *bedrockStreamResponse) UsageMetadata() any {
	return r.usage
}

// Candidates returns the candidate responses for streaming
func (r *bedrockStreamResponse) Candidates() []Candidate {
	if r.content == "" && r.usage == nil {
		return []Candidate{}
	}

	candidate := &bedrockStreamCandidate{
		content: r.content,
		model:   r.model,
	}
	return []Candidate{candidate}
}

// bedrockCandidate implements Candidate for regular responses
type bedrockCandidate struct {
	message *types.Message
	model   string
}

// String returns a string representation of the candidate
func (c *bedrockCandidate) String() string {
	if c.message == nil {
		return ""
	}

	var content strings.Builder
	for _, block := range c.message.Content {
		if textBlock, ok := block.(*types.ContentBlockMemberText); ok {
			content.WriteString(textBlock.Value)
		}
	}
	return content.String()
}

// Parts returns the parts of the candidate
func (c *bedrockCandidate) Parts() []Part {
	if c.message == nil {
		return []Part{}
	}

	var parts []Part
	for _, block := range c.message.Content {
		switch v := block.(type) {
		case *types.ContentBlockMemberText:
			parts = append(parts, &bedrockTextPart{text: v.Value})
		case *types.ContentBlockMemberToolUse:
			parts = append(parts, &bedrockToolPart{toolUse: &v.Value})
		}
	}
	return parts
}

// bedrockStreamCandidate implements Candidate for streaming responses
type bedrockStreamCandidate struct {
	content string
	model   string
}

// String returns a string representation of the streaming candidate
func (c *bedrockStreamCandidate) String() string {
	return c.content
}

// Parts returns the parts of the streaming candidate
func (c *bedrockStreamCandidate) Parts() []Part {
	if c.content == "" {
		return []Part{}
	}
	return []Part{&bedrockTextPart{text: c.content}}
}

// bedrockTextPart implements Part for text content
type bedrockTextPart struct {
	text string
}

// AsText returns the text content
func (p *bedrockTextPart) AsText() (string, bool) {
	return p.text, true
}

// AsFunctionCalls returns nil since this is a text part
func (p *bedrockTextPart) AsFunctionCalls() ([]FunctionCall, bool) {
	return nil, false
}

// bedrockToolPart implements Part for tool/function calls
type bedrockToolPart struct {
	toolUse *types.ToolUseBlock
}

// AsText returns empty string since this is a tool part
func (p *bedrockToolPart) AsText() (string, bool) {
	return "", false
}

// AsFunctionCalls returns the function calls
func (p *bedrockToolPart) AsFunctionCalls() ([]FunctionCall, bool) {
	if p.toolUse == nil {
		return nil, false
	}

	// Convert AWS tool use to gollm function call
	var args map[string]any
	if p.toolUse.Input != nil {
		if err := p.toolUse.Input.UnmarshalSmithyDocument(&args); err != nil {
			klog.Errorf("Failed to unmarshal tool input: %v", err)
			args = make(map[string]any)
		}
	}

	funcCall := FunctionCall{
		ID:        aws.ToString(p.toolUse.ToolUseId),
		Name:      aws.ToString(p.toolUse.Name),
		Arguments: args,
	}

	return []FunctionCall{funcCall}, true
}

// Helper functions

// getBedrockModel returns the model to use, checking in order:
// 1. Explicitly provided model
// 2. Environment variable BEDROCK_MODEL
// 3. Default model (Claude Sonnet 4)
func getBedrockModel(model string) string {
	if model != "" {
		klog.V(2).Infof("Using explicitly provided model: %s", model)
		return model
	}

	if envModel := os.Getenv("BEDROCK_MODEL"); envModel != "" {
		klog.V(1).Infof("Using model from environment variable: %s", envModel)
		return envModel
	}

	defaultModel := "us.anthropic.claude-sonnet-4-20250514-v1:0"
	klog.V(1).Infof("Using default model: %s", defaultModel)
	return defaultModel
}

// bedrockCompletionResponse wraps a ChatResponse to implement CompletionResponse
type bedrockCompletionResponse struct {
	chatResponse ChatResponse
}

var _ CompletionResponse = (*bedrockCompletionResponse)(nil)

func (r *bedrockCompletionResponse) Response() string {
	if r.chatResponse == nil {
		return ""
	}
	candidates := r.chatResponse.Candidates()
	if len(candidates) == 0 {
		return ""
	}
	parts := candidates[0].Parts()
	for _, part := range parts {
		if text, ok := part.AsText(); ok {
			return text
		}
	}
	return ""
}

func (r *bedrockCompletionResponse) UsageMetadata() any {
	if r.chatResponse == nil {
		return nil
	}
	return r.chatResponse.UsageMetadata()
}

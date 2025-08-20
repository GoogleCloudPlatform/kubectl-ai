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

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
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

	// Enhance system prompt for tool-use shim compatibility
	// Detect if tool-use shim is enabled by looking for JSON formatting instructions
	enhancedPrompt := systemPrompt
	if strings.Contains(systemPrompt, "```json") && strings.Contains(systemPrompt, "\"action\"") {
		// Tool-use shim is enabled - add stronger JSON formatting instructions for all Bedrock models
		enhancedPrompt += "\n\nCRITICAL JSON FORMATTING REQUIREMENTS:\n"
		enhancedPrompt += "1. You MUST ALWAYS wrap your JSON responses in ```json code blocks exactly as shown in the examples above.\n"
		enhancedPrompt += "2. NEVER respond with raw JSON without the markdown ```json formatting.\n"
		enhancedPrompt += "3. Ensure your JSON is syntactically correct with proper commas between fields.\n"
		enhancedPrompt += "4. This is critical for proper parsing. Example format:\n"
		enhancedPrompt += "```json\n{\"thought\": \"your reasoning\", \"action\": {\"name\": \"tool_name\", \"command\": \"command\"}}\n```\n"
		enhancedPrompt += "Note the comma after the \"thought\" field! Malformed JSON will cause failures."
	}

	return &bedrockChat{
		client:       c,
		systemPrompt: enhancedPrompt,
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

func (cs *bedrockChat) Initialize(history []*api.Message) error {
	cs.messages = make([]types.Message, 0, len(history))

	for _, msg := range history {
		// Convert api.Message to types.Message
		var role types.ConversationRole
		switch msg.Source {
		case api.MessageSourceUser:
			role = types.ConversationRoleUser
		case api.MessageSourceModel, api.MessageSourceAgent:
			role = types.ConversationRoleAssistant
		default:
			// Skip unknown message sources
			continue
		}

		// Convert payload to string content
		var content string
		if msg.Type == api.MessageTypeText && msg.Payload != nil {
			if textPayload, ok := msg.Payload.(string); ok {
				content = textPayload
			} else {
				// Try to convert other types to string
				content = fmt.Sprintf("%v", msg.Payload)
			}
		} else {
			// Skip non-text messages for now
			continue
		}

		if content == "" {
			continue
		}

		bedrockMsg := types.Message{
			Role: role,
			Content: []types.ContentBlock{
				&types.ContentBlockMemberText{Value: content},
			},
		}

		cs.messages = append(cs.messages, bedrockMsg)
	}

	return nil
}

// Send sends a message to the chat and returns the response
func (c *bedrockChat) Send(ctx context.Context, contents ...any) (ChatResponse, error) {
	if len(contents) == 0 {
		return nil, errors.New("no content provided")
	}

	// Process and append contents to conversation history
	if err := c.addContentsToHistory(contents); err != nil {
		return nil, err
	}

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

	// Process and append contents to conversation history
	if err := c.addContentsToHistory(contents); err != nil {
		return nil, err
	}

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

		// Tool state tracking for streaming
		type partialTool struct {
			id    string
			name  string
			input strings.Builder
		}
		partialTools := make(map[int32]*partialTool)
		var completedTools []types.ToolUseBlock

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

				// Handle tool input deltas
				if toolDelta, ok := v.Value.Delta.(*types.ContentBlockDeltaMemberToolUse); ok {
					idx := aws.ToInt32(v.Value.ContentBlockIndex)
					if partial, exists := partialTools[idx]; exists {
						deltaInput := aws.ToString(toolDelta.Value.Input)
						partial.input.WriteString(deltaInput)
					}
				}

			case *types.ConverseStreamOutputMemberContentBlockStart:
				// Handle content block start (for tool calls)
				if v.Value.Start != nil {
					if toolStart, ok := v.Value.Start.(*types.ContentBlockStartMemberToolUse); ok {
						// Store partial tool for input accumulation
						idx := aws.ToInt32(v.Value.ContentBlockIndex)
						partialTools[idx] = &partialTool{
							id:   aws.ToString(toolStart.Value.ToolUseId),
							name: aws.ToString(toolStart.Value.Name),
						}
					}
				}

			case *types.ConverseStreamOutputMemberContentBlockStop:
				// Handle content block stop (tool completion)
				idx := aws.ToInt32(v.Value.ContentBlockIndex)
				if partial, exists := partialTools[idx]; exists {
					// Parse the JSON to extract arguments for function call
					inputJSON := partial.input.String()
					
					var args map[string]any
					if inputJSON != "" {
						if err := json.Unmarshal([]byte(inputJSON), &args); err != nil {
							args = make(map[string]any)
						}
					} else {
						args = make(map[string]any)
					}
					
					// Create ToolUseBlock for conversation history
					// Use the accumulated JSON string to create proper Input document
					toolUse := types.ToolUseBlock{
						ToolUseId: aws.String(partial.id),
						Name:      aws.String(partial.name),
						Input:     document.NewLazyDocument(args),
					}
					completedTools = append(completedTools, toolUse)
					
					// Yield tool immediately with parsed arguments
					response := &bedrockStreamResponse{
						content:       "",
						model:         c.model,
						done:          false,
						toolUses:      []types.ToolUseBlock{toolUse},
						streamingArgs: map[int]map[string]any{0: args},
					}
					if !yield(response, nil) {
						return
					}
					
					delete(partialTools, idx)
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
		}
		
		// Include completed tools in conversation history
		for _, tool := range completedTools {
			assistantMessage.Content = append(assistantMessage.Content,
				&types.ContentBlockMemberToolUse{Value: tool})
		}
		
		// Only add to history if there's content or tools
		if len(assistantMessage.Content) > 0 {
			c.messages = append(c.messages, assistantMessage)
		}

		// Check for stream errors
		if err := stream.Err(); err != nil {
			yield(nil, fmt.Errorf("stream error: %w", err))
		}
	}, nil
}

// addContentsToHistory processes and appends user messages to chat history
// following AWS Bedrock Converse API patterns
func (c *bedrockChat) addContentsToHistory(contents []any) error {
	var contentBlocks []types.ContentBlock
	
	for _, content := range contents {
		switch c := content.(type) {
		case string:
			// Add text content block
			contentBlocks = append(contentBlocks, &types.ContentBlockMemberText{Value: c})
		case FunctionCallResult:
			// Determine status based on Result content
			status := types.ToolResultStatusSuccess
			if c.Result != nil {
				// Check for error field
				if errorVal, hasError := c.Result["error"]; hasError {
					if errorBool, isBool := errorVal.(bool); isBool && errorBool {
						status = types.ToolResultStatusError
					}
				}
				// Check for status field
				if statusVal, hasStatus := c.Result["status"]; hasStatus {
					if statusStr, isString := statusVal.(string); isString && 
					   (statusStr == "failed" || statusStr == "error") {
						status = types.ToolResultStatusError
					}
				}
			}
			
			// Convert to AWS Bedrock ToolResultBlock format per official docs
			toolResult := types.ToolResultBlock{
				ToolUseId: aws.String(c.ID),
				Content: []types.ToolResultContentBlock{
					&types.ToolResultContentBlockMemberJson{
						Value: document.NewLazyDocument(c.Result),
					},
				},
				Status: status,
			}
			contentBlocks = append(contentBlocks, &types.ContentBlockMemberToolResult{Value: toolResult})
		default:
			return fmt.Errorf("unhandled content type: %T", content)
		}
	}
	
	if len(contentBlocks) > 0 {
		// Add user message with all content blocks to conversation history
		c.messages = append(c.messages, types.Message{
			Role:    types.ConversationRoleUser,
			Content: contentBlocks,
		})
	}
	
	return nil
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
		ToolChoice: &types.ToolChoiceMemberAny{
			Value: types.AnyToolChoice{},
		},
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
	content       string
	usage         *types.TokenUsage
	model         string
	done          bool
	toolUses      []types.ToolUseBlock
	streamingArgs map[int]map[string]any
}

// UsageMetadata returns the usage metadata from the streaming response
func (r *bedrockStreamResponse) UsageMetadata() any {
	return r.usage
}

// Candidates returns the candidate responses for streaming
func (r *bedrockStreamResponse) Candidates() []Candidate {
	if r.content == "" && r.usage == nil && len(r.toolUses) == 0 {
		return []Candidate{}
	}

	candidate := &bedrockStreamCandidate{
		content:       r.content,
		model:         r.model,
		toolUses:      r.toolUses,
		streamingArgs: r.streamingArgs,
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
	content       string
	model         string
	toolUses      []types.ToolUseBlock
	streamingArgs map[int]map[string]any
}

// String returns a string representation of the streaming candidate
func (c *bedrockStreamCandidate) String() string {
	return c.content
}

// Parts returns the parts of the streaming candidate
func (c *bedrockStreamCandidate) Parts() []Part {
	var parts []Part
	
	// Handle text content
	if c.content != "" {
		parts = append(parts, &bedrockTextPart{text: c.content})
	}
	
	// Handle tool calls with streaming args
	for i, toolUse := range c.toolUses {
		var args map[string]any
		if c.streamingArgs != nil {
			args = c.streamingArgs[i]
		}
		parts = append(parts, &bedrockToolPart{
			toolUse: &toolUse,
			args:    args,
		})
	}
	
	return parts
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
	args    map[string]any // For streaming case when Input can't be unmarshaled
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

	// Get arguments - prefer pre-parsed args (streaming), fall back to unmarshaling
	var args map[string]any
	if p.args != nil {
		// Streaming case - use pre-parsed arguments
		args = p.args
	} else if p.toolUse.Input != nil {
		// Non-streaming case - unmarshal from Input
		if err := p.toolUse.Input.UnmarshalSmithyDocument(&args); err != nil {
			klog.V(2).Infof("Failed to unmarshal tool input: %v", err)
			args = make(map[string]any)
		}
	} else {
		args = make(map[string]any)
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

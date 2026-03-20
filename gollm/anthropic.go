// Copyright 2026 Google LLC
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
	"strconv"
	"strings"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/vertex"
	"k8s.io/klog/v2"
)

// Package-level env var storage (Anthropic env)
var (
	anthropicAPIKey           string
	anthropicDefaultModel     string
	anthropicPromptCaching    bool
	anthropicExtendedThinking bool
	anthropicMaxTokens        int64
	anthropicVertexProjectID  string
	anthropicVertexLocation   string
)

func init() {
	anthropicAPIKey = os.Getenv("ANTHROPIC_API_KEY")
	anthropicDefaultModel = os.Getenv("ANTHROPIC_MODEL")

	// Vertex AI environment variables (following GCP conventions)
	anthropicVertexProjectID = os.Getenv("ANTHROPIC_VERTEX_PROJECT_ID")
	if anthropicVertexProjectID == "" {
		anthropicVertexProjectID = os.Getenv("GOOGLE_CLOUD_PROJECT")
	}

	anthropicVertexLocation = os.Getenv("ANTHROPIC_VERTEX_LOCATION")
	if anthropicVertexLocation == "" {
		anthropicVertexLocation = os.Getenv("GOOGLE_CLOUD_LOCATION")
	}
	if anthropicVertexLocation == "" {
		anthropicVertexLocation = os.Getenv("GOOGLE_CLOUD_REGION")
	}

	// Prompt caching defaults to true; set ANTHROPIC_PROMPT_CACHING=false to disable
	if v := os.Getenv("ANTHROPIC_PROMPT_CACHING"); v == "false" {
		anthropicPromptCaching = false
	} else {
		anthropicPromptCaching = true
	}

	if v := os.Getenv("ANTHROPIC_EXTENDED_THINKING"); strings.ToLower(v) == "true" {
		anthropicExtendedThinking = true
	}

	anthropicMaxTokens = 4096 // default
	if v := os.Getenv("ANTHROPIC_MAX_TOKENS"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			anthropicMaxTokens = n
		}
	}

	if err := RegisterProvider("anthropic", newAnthropicClientFactory); err != nil {
		klog.Fatalf("Failed to register anthropic provider: %v", err)
	}
}

// newAnthropicClientFactory creates a new Anthropic client with the given options.
func newAnthropicClientFactory(ctx context.Context, opts ClientOptions) (Client, error) {
	return NewAnthropicClient(ctx, opts)
}

// AnthropicClient implements the gollm.Client interface for Anthropic models.
type AnthropicClient struct {
	client *anthropic.Client
}

// Ensure AnthropicClient implements the Client interface.
var _ Client = &AnthropicClient{}

// NewAnthropicClient creates a new client for interacting with Anthropic models.
// It supports both direct Anthropic API and Google Vertex AI.
func NewAnthropicClient(ctx context.Context, opts ClientOptions) (*AnthropicClient, error) {
	var clientOpts []option.RequestOption

	// Check if Vertex AI configuration is present
	if anthropicVertexProjectID != "" && anthropicVertexLocation != "" {
		klog.V(2).Infof("Using Anthropic via Google Vertex AI (project=%s, location=%s)", anthropicVertexProjectID, anthropicVertexLocation)

		// Use Vertex AI with Google Application Default Credentials
		// Note: vertex.WithGoogleAuth creates its own authenticated HTTP client,
		// so we should not override it with a custom client
		vertexOpt := vertex.WithGoogleAuth(ctx, anthropicVertexLocation, anthropicVertexProjectID)
		clientOpts = []option.RequestOption{
			vertexOpt,
		}
	} else {
		// Use direct Anthropic API
		apiKey := anthropicAPIKey
		if apiKey == "" {
			return nil, errors.New("Anthropic API key not found. Set via ANTHROPIC_API_KEY env var, or configure Vertex AI with ANTHROPIC_VERTEX_PROJECT_ID and ANTHROPIC_VERTEX_LOCATION (or GOOGLE_CLOUD_PROJECT and GOOGLE_CLOUD_LOCATION)")
		}

		klog.V(2).Infof("Using Anthropic via direct API")

		httpClient := createCustomHTTPClient(opts.SkipVerifySSL)
		httpClient = withJournaling(httpClient)

		clientOpts = []option.RequestOption{
			option.WithAPIKey(apiKey),
			option.WithHTTPClient(httpClient),
		}
	}

	client := anthropic.NewClient(clientOpts...)
	return &AnthropicClient{client: &client}, nil
}

// Close cleans up any resources used by the client.
func (c *AnthropicClient) Close() error {
	return nil
}

// StartChat starts a new chat session with the specified system prompt and model.
func (c *AnthropicClient) StartChat(systemPrompt, model string) Chat {
	selectedModel := getAnthropicModel(model)
	klog.V(2).Infof("Starting new Anthropic chat session with model: %s", selectedModel)

	return &anthropicChatSession{
		client:           c.client,
		model:            selectedModel,
		systemPrompt:     systemPrompt,
		messages:         []anthropic.MessageParam{},
		promptCaching:    anthropicPromptCaching,
		extendedThinking: anthropicExtendedThinking,
	}
}

// GenerateCompletion generates a single completion for the given request.
func (c *AnthropicClient) GenerateCompletion(ctx context.Context, req *CompletionRequest) (CompletionResponse, error) {
	chat := c.StartChat("", req.Model)
	chatResponse, err := chat.Send(ctx, req.Prompt)
	if err != nil {
		return nil, err
	}
	return &anthropicCompletionResponse{chatResponse: chatResponse}, nil
}

// SetResponseSchema is not supported by the native Anthropic provider.
func (c *AnthropicClient) SetResponseSchema(schema *Schema) error {
	klog.Warning("AnthropicClient.SetResponseSchema is not supported by the native Anthropic provider")
	return nil
}

// ListModels returns the list of supported Anthropic Claude models.
func (c *AnthropicClient) ListModels(ctx context.Context) ([]string, error) {
	return []string{
		// Claude 4.6 (latest)
		"claude-opus-4-6",
		"claude-sonnet-4-6",
		// Claude 4.5
		"claude-opus-4-5",
		"claude-opus-4-5-20251101",
		"claude-sonnet-4-5",
		"claude-sonnet-4-5-20250929",
		"claude-haiku-4-5",
		"claude-haiku-4-5-20251001",
		// Claude 3.7
		"claude-3-7-sonnet-latest",
		"claude-3-7-sonnet-20250219",
		// Claude 3.5
		"claude-3-5-haiku-latest",
		"claude-3-5-haiku-20241022",
		// Claude 3
		"claude-3-opus-latest",
		"claude-3-opus-20240229",
		"claude-3-haiku-20240307",
	}, nil
}

// anthropicChatSession implements the Chat interface for Anthropic conversations.
type anthropicChatSession struct {
	client           *anthropic.Client
	model            string
	systemPrompt     string
	messages         []anthropic.MessageParam
	tools            []anthropic.ToolUnionParam
	promptCaching    bool
	extendedThinking bool
}

// Ensure anthropicChatSession implements the Chat interface.
var _ Chat = (*anthropicChatSession)(nil)

// Initialize initializes the chat with a previous conversation history.
func (c *anthropicChatSession) Initialize(history []*api.Message) error {
	c.messages = make([]anthropic.MessageParam, 0, len(history))

	for _, msg := range history {
		var role anthropic.MessageParamRole
		switch msg.Source {
		case api.MessageSourceUser:
			role = anthropic.MessageParamRoleUser
		case api.MessageSourceModel, api.MessageSourceAgent:
			role = anthropic.MessageParamRoleAssistant
		default:
			continue
		}

		if msg.Type != api.MessageTypeText || msg.Payload == nil {
			continue
		}

		var content string
		if textPayload, ok := msg.Payload.(string); ok {
			content = textPayload
		} else {
			content = fmt.Sprintf("%v", msg.Payload)
		}

		if content == "" {
			continue
		}

		param := anthropic.MessageParam{
			Role:    role,
			Content: []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock(content)},
		}
		c.messages = append(c.messages, param)
	}

	return nil
}

// SetFunctionDefinitions configures the available functions for tool use.
func (c *anthropicChatSession) SetFunctionDefinitions(functions []*FunctionDefinition) error {
	c.tools = nil

	if len(functions) == 0 {
		return nil
	}

	c.tools = make([]anthropic.ToolUnionParam, len(functions))
	for i, fn := range functions {
		// Build input schema properties from gollm Schema
		var properties any
		var required []string
		if fn.Parameters != nil {
			schemaBytes, err := json.Marshal(fn.Parameters)
			if err != nil {
				return fmt.Errorf("failed to marshal parameters for function %s: %w", fn.Name, err)
			}
			var schemaMap map[string]any
			if err := json.Unmarshal(schemaBytes, &schemaMap); err != nil {
				return fmt.Errorf("failed to unmarshal parameters for function %s: %w", fn.Name, err)
			}
			properties = schemaMap["properties"]
			if reqSlice, ok := schemaMap["required"].([]any); ok {
				for _, r := range reqSlice {
					if s, ok := r.(string); ok {
						required = append(required, s)
					}
				}
			}
		}

		tool := anthropic.ToolParam{
			Name:        fn.Name,
			Description: anthropic.String(fn.Description),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: properties,
				Required:   required,
			},
		}

		// Apply prompt cache breakpoint to the last tool definition
		if c.promptCaching && i == len(functions)-1 {
			tool.CacheControl = anthropic.NewCacheControlEphemeralParam()
		}

		c.tools[i] = anthropic.ToolUnionParam{OfTool: &tool}
	}

	return nil
}

// buildSystemBlocks constructs the system prompt blocks, optionally with cache_control.
func (c *anthropicChatSession) buildSystemBlocks() []anthropic.TextBlockParam {
	if c.systemPrompt == "" {
		return nil
	}
	block := anthropic.TextBlockParam{
		Text: c.systemPrompt,
	}
	if c.promptCaching {
		block.CacheControl = anthropic.NewCacheControlEphemeralParam()
	}
	return []anthropic.TextBlockParam{block}
}

// addContentsToHistory processes and appends user messages to chat history.
func (c *anthropicChatSession) addContentsToHistory(contents []any) error {
	var blocks []anthropic.ContentBlockParamUnion

	for _, content := range contents {
		switch v := content.(type) {
		case string:
			blocks = append(blocks, anthropic.NewTextBlock(v))
		case FunctionCallResult:
			resultJSON, err := json.Marshal(v.Result)
			if err != nil {
				return fmt.Errorf("failed to marshal function call result %q: %w", v.Name, err)
			}
			// Detect error from result map
			isError := false
			if v.Result != nil {
				if errVal, ok := v.Result["error"]; ok {
					if errBool, isBool := errVal.(bool); isBool && errBool {
						isError = true
					}
				}
				if statusVal, ok := v.Result["status"]; ok {
					if statusStr, isStr := statusVal.(string); isStr &&
						(statusStr == "failed" || statusStr == "error") {
						isError = true
					}
				}
			}
			blocks = append(blocks, anthropic.NewToolResultBlock(v.ID, string(resultJSON), isError))
		default:
			return fmt.Errorf("unhandled content type: %T", content)
		}
	}

	if len(blocks) > 0 {
		c.messages = append(c.messages, anthropic.NewUserMessage(blocks...))
	}
	return nil
}

// Send sends a message and returns a non-streaming response.
func (c *anthropicChatSession) Send(ctx context.Context, contents ...any) (ChatResponse, error) {
	if len(contents) == 0 {
		return nil, errors.New("no content provided")
	}

	if err := c.addContentsToHistory(contents); err != nil {
		return nil, err
	}

	const thinkingBudget = 8000
	maxTokens := anthropicMaxTokens
	if c.extendedThinking {
		// max_tokens must exceed budget_tokens
		maxTokens = thinkingBudget + anthropicMaxTokens
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: maxTokens,
		Messages:  c.messages,
		System:    c.buildSystemBlocks(),
	}

	if len(c.tools) > 0 {
		params.Tools = c.tools
	}

	if c.extendedThinking {
		params.Thinking = anthropic.ThinkingConfigParamOfEnabled(thinkingBudget)
	}

	msg, err := c.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("anthropic message error: %w", err)
	}

	// Append assistant message to history
	c.messages = append(c.messages, msg.ToParam())

	return &anthropicResponse{message: msg}, nil
}

// SendStreaming sends a message and returns a streaming response iterator.
func (c *anthropicChatSession) SendStreaming(ctx context.Context, contents ...any) (ChatResponseIterator, error) {
	if len(contents) == 0 {
		return nil, errors.New("no content provided")
	}

	if err := c.addContentsToHistory(contents); err != nil {
		return nil, err
	}

	const thinkingBudget = 8000
	maxTokens := anthropicMaxTokens
	if c.extendedThinking {
		// max_tokens must exceed budget_tokens
		maxTokens = thinkingBudget + anthropicMaxTokens
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: maxTokens,
		Messages:  c.messages,
		System:    c.buildSystemBlocks(),
	}

	if len(c.tools) > 0 {
		params.Tools = c.tools
	}

	if c.extendedThinking {
		params.Thinking = anthropic.ThinkingConfigParamOfEnabled(thinkingBudget)
	}

	stream := c.client.Messages.NewStreaming(ctx, params)

	return func(yield func(ChatResponse, error) bool) {
		defer stream.Close()

		// Accumulated message for history
		acc := anthropic.Message{}

		// Tool accumulation state
		type partialTool struct {
			id    string
			name  string
			input strings.Builder
		}
		toolsByIndex := make(map[int64]*partialTool)

		for stream.Next() {
			event := stream.Current()

			// Accumulate for history
			if err := acc.Accumulate(event); err != nil {
				klog.V(2).Infof("Anthropic accumulate error: %v", err)
			}

			switch ev := event.AsAny().(type) {
			case anthropic.ContentBlockStartEvent:
				// Register new tool_use block
				if ev.ContentBlock.Type == "tool_use" {
					toolsByIndex[ev.Index] = &partialTool{
						id:   ev.ContentBlock.ID,
						name: ev.ContentBlock.Name,
					}
				}

			case anthropic.ContentBlockDeltaEvent:
				switch delta := ev.Delta.AsAny().(type) {
				case anthropic.TextDelta:
					if !yield(&anthropicStreamResponse{text: delta.Text}, nil) {
						return
					}
				case anthropic.InputJSONDelta:
					// Accumulate tool input JSON
					if pt, ok := toolsByIndex[ev.Index]; ok {
						pt.input.WriteString(delta.PartialJSON)
					}
				case anthropic.ThinkingDelta:
					// thinking content is kept in history via accumulator, not yielded to UI
				}

			case anthropic.ContentBlockStopEvent:
				// Check if a tool_use block completed
				if pt, ok := toolsByIndex[ev.Index]; ok {
					inputJSON := pt.input.String()
					var args map[string]any
					if inputJSON != "" {
						if err := json.Unmarshal([]byte(inputJSON), &args); err != nil {
							klog.V(2).Infof("Failed to unmarshal tool input: %v", err)
							args = make(map[string]any)
						}
					} else {
						args = make(map[string]any)
					}

					fc := FunctionCall{
						ID:        pt.id,
						Name:      pt.name,
						Arguments: args,
					}
					if !yield(&anthropicStreamResponse{functionCall: &fc}, nil) {
						return
					}

					delete(toolsByIndex, ev.Index)
				}
			}
		}

		if err := stream.Err(); err != nil {
			yield(nil, fmt.Errorf("anthropic stream error: %w", err))
			return
		}

		// Append accumulated assistant message to history
		if len(acc.Content) > 0 {
			c.messages = append(c.messages, acc.ToParam())
		}
		// Note: We don't yield usage-only responses because kubectl-ai expects
		// all responses to have candidates. Usage metadata is available in
		// the accumulated message's Usage field if needed.
	}, nil
}

// IsRetryableError determines if an error from the Anthropic API should be retried.
func (c *anthropicChatSession) IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		sc := apiErr.StatusCode
		// 429 = rate limit, 529 = overloaded, 5xx = server errors
		return sc == 429 || sc == 529 || (sc >= 500 && sc < 600)
	}

	return DefaultIsRetryableError(err)
}

// anthropicResponse implements ChatResponse for non-streaming responses.
type anthropicResponse struct {
	message *anthropic.Message
}

var _ ChatResponse = (*anthropicResponse)(nil)

func (r *anthropicResponse) UsageMetadata() any {
	if r.message != nil {
		return r.message.Usage
	}
	return nil
}

func (r *anthropicResponse) Candidates() []Candidate {
	if r.message == nil {
		return nil
	}
	return []Candidate{&anthropicCandidate{content: r.message.Content}}
}

// anthropicStreamResponse implements ChatResponse for streaming responses.
type anthropicStreamResponse struct {
	text         string
	functionCall *FunctionCall
	usage        *anthropic.Usage
}

var _ ChatResponse = (*anthropicStreamResponse)(nil)

func (r *anthropicStreamResponse) UsageMetadata() any {
	if r.usage != nil {
		return r.usage
	}
	return nil
}

func (r *anthropicStreamResponse) Candidates() []Candidate {
	if r.text == "" && r.functionCall == nil {
		return nil
	}
	return []Candidate{&anthropicStreamCandidate{
		text:         r.text,
		functionCall: r.functionCall,
	}}
}

// anthropicCandidate implements Candidate for non-streaming responses.
type anthropicCandidate struct {
	content []anthropic.ContentBlockUnion
}

var _ Candidate = (*anthropicCandidate)(nil)

func (c *anthropicCandidate) String() string {
	var sb strings.Builder
	for _, block := range c.content {
		switch block.Type {
		case "text":
			tb := block.AsText()
			sb.WriteString(tb.Text)
		}
	}
	return sb.String()
}

func (c *anthropicCandidate) Parts() []Part {
	var parts []Part
	for _, block := range c.content {
		switch block.Type {
		case "text":
			tb := block.AsText()
			if tb.Text != "" {
				parts = append(parts, &anthropicTextPart{text: tb.Text})
			}
		case "tool_use":
			tu := block.AsToolUse()
			var args map[string]any
			if len(tu.Input) > 0 {
				if err := json.Unmarshal(tu.Input, &args); err != nil {
					klog.V(2).Infof("Failed to unmarshal tool input: %v", err)
					args = make(map[string]any)
				}
			} else {
				args = make(map[string]any)
			}
			parts = append(parts, &anthropicToolPart{
				functionCall: FunctionCall{
					ID:        tu.ID,
					Name:      tu.Name,
					Arguments: args,
				},
			})
		case "thinking":
			// ThinkingBlock — do not yield to UI, skip
		}
	}
	return parts
}

// anthropicStreamCandidate implements Candidate for streaming responses.
type anthropicStreamCandidate struct {
	text         string
	functionCall *FunctionCall
}

var _ Candidate = (*anthropicStreamCandidate)(nil)

func (c *anthropicStreamCandidate) String() string {
	if c.text != "" {
		return c.text
	}
	if c.functionCall != nil {
		return fmt.Sprintf("FunctionCall(%s)", c.functionCall.Name)
	}
	return ""
}

func (c *anthropicStreamCandidate) Parts() []Part {
	var parts []Part
	if c.text != "" {
		parts = append(parts, &anthropicTextPart{text: c.text})
	}
	if c.functionCall != nil {
		parts = append(parts, &anthropicToolPart{functionCall: *c.functionCall})
	}
	return parts
}

// anthropicTextPart implements Part for text content.
type anthropicTextPart struct {
	text string
}

var _ Part = (*anthropicTextPart)(nil)

func (p *anthropicTextPart) AsText() (string, bool) {
	return p.text, p.text != ""
}

func (p *anthropicTextPart) AsFunctionCalls() ([]FunctionCall, bool) {
	return nil, false
}

// anthropicToolPart implements Part for tool/function calls.
type anthropicToolPart struct {
	functionCall FunctionCall
}

var _ Part = (*anthropicToolPart)(nil)

func (p *anthropicToolPart) AsText() (string, bool) {
	return "", false
}

func (p *anthropicToolPart) AsFunctionCalls() ([]FunctionCall, bool) {
	return []FunctionCall{p.functionCall}, true
}

// anthropicCompletionResponse wraps a ChatResponse to implement CompletionResponse.
type anthropicCompletionResponse struct {
	chatResponse ChatResponse
}

var _ CompletionResponse = (*anthropicCompletionResponse)(nil)

func (r *anthropicCompletionResponse) Response() string {
	if r.chatResponse == nil {
		return ""
	}
	candidates := r.chatResponse.Candidates()
	if len(candidates) == 0 {
		return ""
	}
	for _, part := range candidates[0].Parts() {
		if text, ok := part.AsText(); ok {
			return text
		}
	}
	return ""
}

func (r *anthropicCompletionResponse) UsageMetadata() any {
	if r.chatResponse == nil {
		return nil
	}
	return r.chatResponse.UsageMetadata()
}

func getAnthropicModel(model string) string {
	if model != "" && strings.HasPrefix(model, "claude") {
		klog.V(4).Infof("Using explicitly provided Anthropic model: %s", model)
		return model
	}
	if model != "" {
		klog.V(2).Infof("Ignoring non-Claude model %q, falling back to default", model)
	}
	if anthropicDefaultModel != "" {
		klog.V(2).Infof("Using Anthropic model from environment: %s", anthropicDefaultModel)
		return anthropicDefaultModel
	}
	defaultModel := "claude-sonnet-4-6"
	klog.V(2).Infof("Using default Anthropic model: %s", defaultModel)
	return defaultModel
}

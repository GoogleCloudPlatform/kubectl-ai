package bedrock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"k8s.io/klog/v2"
)

func init() {
	if err := gollm.RegisterProvider("bedrock", newBedrockClientFactory); err != nil {
		klog.Fatalf("Failed to register bedrock provider: %v", err)
	}
	klog.V(1).Info("Successfully registered Bedrock provider with Claude and Nova support")
}

func newBedrockClientFactory(ctx context.Context, opts gollm.ClientOptions) (gollm.Client, error) {
	return NewBedrockClient(ctx, opts)
}

type BedrockClient struct {
	runtimeClient *bedrockruntime.Client
	bedrockClient *bedrock.Client
	options       *BedrockOptions
	clientOpts    gollm.ClientOptions
}

var _ gollm.Client = &BedrockClient{}

func NewBedrockClient(ctx context.Context, opts gollm.ClientOptions) (*BedrockClient, error) {
	options := mergeWithClientOptions(DefaultOptions, opts)
	client, err := NewBedrockClientWithOptions(ctx, options)
	if err != nil {
		return nil, err
	}

	client.clientOpts = opts
	return client, nil
}

func mergeWithClientOptions(defaults *BedrockOptions, opts gollm.ClientOptions) *BedrockOptions {
	merged := *defaults

	if opts.InferenceConfig != nil {
		config := opts.InferenceConfig
		if config.Model != "" {
			merged.Model = config.Model
		}
		if config.Region != "" {
			merged.Region = config.Region
		}
		if config.Temperature != 0 {
			merged.Temperature = config.Temperature
		}
		if config.MaxTokens != 0 {
			merged.MaxTokens = config.MaxTokens
		}
		if config.TopP != 0 {
			merged.TopP = config.TopP
		}
		if config.MaxRetries != 0 {
			merged.MaxRetries = config.MaxRetries
		}
	}

	return &merged
}

func convertAWSUsage(awsUsage any, model, provider string) *gollm.Usage {
	if awsUsage == nil {
		return nil
	}

	if usage, ok := awsUsage.(*types.TokenUsage); ok {
		return &gollm.Usage{
			InputTokens:  int(aws.ToInt32(usage.InputTokens)),
			OutputTokens: int(aws.ToInt32(usage.OutputTokens)),
			TotalTokens:  int(aws.ToInt32(usage.TotalTokens)),
			Model:        model,
			Provider:     provider,
			Timestamp:    time.Now(),
		}
	}

	return nil
}

func NewBedrockClientWithOptions(ctx context.Context, options *BedrockOptions) (*BedrockClient, error) {
	klog.V(1).Infof("Initializing Bedrock client - Region: %s, Model: %s", options.Region, options.Model)
	configOptions := []func(*config.LoadOptions) error{
		config.WithRegion(options.Region),
	}

	if options.CredentialsProvider != nil {
		configOptions = append(configOptions, config.WithCredentialsProvider(options.CredentialsProvider))
	}
	if options.MaxRetries > 0 {
		configOptions = append(configOptions, config.WithRetryMaxAttempts(options.MaxRetries))
	}

	// Create a timeout context for AWS config loading to prevent indefinite hangs
	// This is crucial because config.LoadDefaultConfig can hang during credential resolution
	configTimeout := 30 * time.Second
	if options.Timeout > 0 {
		configTimeout = options.Timeout
	}

	configCtx, cancel := context.WithTimeout(ctx, configTimeout)
	defer cancel()

	klog.V(2).Infof("Loading AWS configuration with timeout: %v", configTimeout)

	cfg, err := config.LoadDefaultConfig(configCtx, configOptions...)
	if err != nil {
		// Check if the error was due to context timeout
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("%s: AWS configuration loading timed out after %v - this usually indicates credential or network issues. Please check your AWS credentials and network connectivity: %w", ErrMsgConfigLoad, configTimeout, err)
		}
		return nil, fmt.Errorf("%s: %w", ErrMsgConfigLoad, err)
	}

	klog.V(2).Info("AWS configuration loaded successfully")

	return &BedrockClient{
		runtimeClient: bedrockruntime.NewFromConfig(cfg),
		bedrockClient: bedrock.NewFromConfig(cfg),
		options:       options,
	}, nil
}

func (c *BedrockClient) Close() error {
	klog.V(2).Info("Bedrock client closed")
	return nil
}

func (c *BedrockClient) StartChat(systemPrompt, model string) gollm.Chat {
	selectedModel := model
	if selectedModel == "" {
		selectedModel = c.options.Model
	}

	if !isModelSupported(selectedModel) {
		klog.Errorf("Unsupported model requested: %s, falling back to default: %s", selectedModel, c.options.Model)
		return &bedrockChatSession{
			client:       c,
			systemPrompt: systemPrompt,
			model:        c.options.Model,
			history:      make([]types.Message, 0),
			functionDefs: make([]*gollm.FunctionDefinition, 0),
		}
	}

	klog.V(1).Infof("Starting chat session with model: %s", selectedModel)

	return &bedrockChatSession{
		client:       c,
		systemPrompt: systemPrompt,
		model:        selectedModel,
		history:      make([]types.Message, 0),
		functionDefs: make([]*gollm.FunctionDefinition, 0),
	}
}

func (c *BedrockClient) GenerateCompletion(ctx context.Context, req *gollm.CompletionRequest) (gollm.CompletionResponse, error) {
	klog.V(1).Infof("GenerateCompletion called with model: %s", req.Model)

	model := req.Model
	if model == "" {
		model = c.options.Model
	}

	if !isModelSupported(model) {
		return nil, fmt.Errorf("%s: %s", ErrMsgUnsupportedModel, model)
	}

	chat := c.StartChat("", model)

	response, err := chat.Send(ctx, req.Prompt)
	if err != nil {
		return nil, fmt.Errorf("completion failed: %w", err)
	}

	return &simpleBedrockCompletionResponse{
		content:  extractTextFromResponse(response),
		usage:    response.UsageMetadata(),
		model:    model,
		provider: "bedrock",
	}, nil
}

func (c *BedrockClient) SetResponseSchema(schema *gollm.Schema) error {
	klog.V(1).Info("Response schema set for Bedrock client")
	return nil
}

func (c *BedrockClient) ListModels(ctx context.Context) ([]string, error) {
	return getSupportedModels(), nil
}

type bedrockChatSession struct {
	client       *BedrockClient
	systemPrompt string
	model        string
	history      []types.Message
	functionDefs []*gollm.FunctionDefinition
}

var _ gollm.Chat = (*bedrockChatSession)(nil)

func (cs *bedrockChatSession) SetFunctionDefinitions(defs []*gollm.FunctionDefinition) error {
	cs.functionDefs = defs
	klog.V(1).Infof("SetFunctionDefinitions called with %d definitions", len(defs))
	return nil
}

func (cs *bedrockChatSession) Send(ctx context.Context, contents ...any) (gollm.ChatResponse, error) {
	if !isModelSupported(cs.model) {
		return nil, fmt.Errorf("%s: %s. Supported models: %v",
			ErrMsgUnsupportedModel, cs.model, getSupportedModels())
	}

	userMessage, err := cs.processContents(contents...)
	if err != nil {
		return nil, err
	}

	if userMessage != "" {
		cs.addTextMessage(types.ConversationRoleUser, userMessage)
	}
	input := cs.buildConverseInput()

	klog.V(2).Infof("Sending Converse request for model: %s", cs.model)

	output, err := cs.client.runtimeClient.Converse(ctx, input)
	if err != nil {
		cs.removeLastMessage()
		return nil, fmt.Errorf("Converse API failed: %w", err)
	}

	response := cs.parseConverseOutput(&output.Output)
	response.usage = output.Usage

	if cs.client.clientOpts.UsageCallback != nil {
		if structuredUsage := convertAWSUsage(output.Usage, cs.model, "bedrock"); structuredUsage != nil {
			cs.client.clientOpts.UsageCallback("bedrock", cs.model, *structuredUsage)
		}
	}

	cs.addAssistantResponse(response)

	return response, nil
}

func (cs *bedrockChatSession) SendStreaming(ctx context.Context, contents ...any) (gollm.ChatResponseIterator, error) {
	if !isModelSupported(cs.model) {
		return nil, fmt.Errorf("%s: %s. Supported models: %v",
			ErrMsgUnsupportedModel, cs.model, getSupportedModels())
	}

	userMessage, err := cs.processContents(contents...)
	if err != nil {
		return nil, err
	}

	cs.addTextMessage(types.ConversationRoleUser, userMessage)
	input := cs.buildConverseStreamInput()

	klog.V(2).Infof("Starting ConverseStream for model: %s", cs.model)

	output, err := cs.client.runtimeClient.ConverseStream(ctx, input)
	if err != nil {
		cs.removeLastMessage()
		return nil, fmt.Errorf("ConverseStream failed: %w", err)
	}

	return cs.createStreamingIterator(output), nil
}

func (cs *bedrockChatSession) processContents(contents ...any) (string, error) {
	var messages []string
	var toolResults []types.ContentBlock

	for _, content := range contents {
		switch c := content.(type) {
		case string:
			messages = append(messages, c)
		case gollm.FunctionCallResult:
			toolResult := &types.ContentBlockMemberToolResult{
				Value: types.ToolResultBlock{
					ToolUseId: aws.String(c.ID),
					Content: []types.ToolResultContentBlock{
						&types.ToolResultContentBlockMemberText{
							Value: cs.formatToolResult(c),
						},
					},
				},
			}
			toolResults = append(toolResults, toolResult)
		default:
			return "", fmt.Errorf("unsupported content type: %T", content)
		}
	}

	if len(messages) > 0 && len(toolResults) > 0 {
		return "", fmt.Errorf("cannot mix text messages and tool results in the same call")
	}

	if len(toolResults) > 0 {
		cs.addToolResults(toolResults)
		return "", nil
	}

	if len(messages) == 0 {
		return "", errors.New("no valid messages provided")
	}

	return strings.Join(messages, "\n"), nil
}

func (cs *bedrockChatSession) formatToolResult(result gollm.FunctionCallResult) string {
	if result.Result == nil {
		return fmt.Sprintf("Tool %s completed successfully", result.Name)
	}

	resultJSON, err := json.Marshal(result.Result)
	if err != nil {
		return fmt.Sprintf("Tool %s completed with result: %v", result.Name, result.Result)
	}

	return string(resultJSON)
}

func (cs *bedrockChatSession) addMessage(role types.ConversationRole, contentBlocks ...types.ContentBlock) {
	if len(contentBlocks) == 0 {
		klog.V(3).Infof("Skipping empty message for role: %s", role)
		return
	}

	message := types.Message{
		Role:    role,
		Content: contentBlocks,
	}

	cs.history = append(cs.history, message)
}

func (cs *bedrockChatSession) addTextMessage(role types.ConversationRole, content string) {
	if content == "" {
		return
	}
	textBlock := &types.ContentBlockMemberText{Value: content}
	cs.addMessage(role, textBlock)
}

func (cs *bedrockChatSession) addToolResults(toolResults []types.ContentBlock) {
	if len(toolResults) > 0 {
		cs.addMessage(types.ConversationRoleUser, toolResults...)
	}
}

func (cs *bedrockChatSession) addAssistantResponse(response *bedrockChatResponse) {
	var contentBlocks []types.ContentBlock

	if response.content != "" {
		contentBlocks = append(contentBlocks, &types.ContentBlockMemberText{
			Value: response.content,
		})
	}
	for _, toolCall := range response.toolCalls {
		toolUseBlock := cs.createToolUseBlock(toolCall)
		contentBlocks = append(contentBlocks, toolUseBlock)
	}

	if len(contentBlocks) > 0 {
		cs.addMessage(types.ConversationRoleAssistant, contentBlocks...)
	}
}

func (cs *bedrockChatSession) createToolUseBlock(toolCall gollm.FunctionCall) *types.ContentBlockMemberToolUse {
	toolUseBlock := &types.ContentBlockMemberToolUse{
		Value: types.ToolUseBlock{
			ToolUseId: aws.String(toolCall.ID),
			Name:      aws.String(toolCall.Name),
		},
	}

	var inputDoc document.Interface
	if len(toolCall.Arguments) > 0 {
		inputDoc = document.NewLazyDocument(toolCall.Arguments)
	} else {
		inputDoc = document.NewLazyDocument(map[string]any{})
	}
	toolUseBlock.Value.Input = inputDoc

	return toolUseBlock
}

func (cs *bedrockChatSession) addMessageToHistory(role types.ConversationRole, content string) {
	cs.addTextMessage(role, content)
}

func (cs *bedrockChatSession) removeLastMessage() {
	if len(cs.history) > 0 {
		cs.history = cs.history[:len(cs.history)-1]
		klog.V(3).Info("Removed last message from history")
	}
}

func (cs *bedrockChatSession) buildConverseInput() *bedrockruntime.ConverseInput {
	input := &bedrockruntime.ConverseInput{
		ModelId:  aws.String(cs.model),
		Messages: cs.history,
		InferenceConfig: &types.InferenceConfiguration{
			MaxTokens:   aws.Int32(cs.client.options.MaxTokens),
			Temperature: aws.Float32(cs.client.options.Temperature),
			TopP:        aws.Float32(cs.client.options.TopP),
		},
	}

	if cs.systemPrompt != "" {
		input.System = []types.SystemContentBlock{
			&types.SystemContentBlockMemberText{Value: cs.systemPrompt},
		}
	}
	if len(cs.functionDefs) > 0 {
		tools := cs.buildTools()
		if len(tools) > 0 {
			input.ToolConfig = &types.ToolConfiguration{
				Tools: tools,
				ToolChoice: &types.ToolChoiceMemberAny{
					Value: types.AnyToolChoice{},
				},
			}
		}
	}

	return input
}

func (cs *bedrockChatSession) buildConverseStreamInput() *bedrockruntime.ConverseStreamInput {
	input := &bedrockruntime.ConverseStreamInput{
		ModelId:  aws.String(cs.model),
		Messages: cs.history,
		InferenceConfig: &types.InferenceConfiguration{
			MaxTokens:   aws.Int32(cs.client.options.MaxTokens),
			Temperature: aws.Float32(cs.client.options.Temperature),
			TopP:        aws.Float32(cs.client.options.TopP),
		},
	}

	if cs.systemPrompt != "" {
		input.System = []types.SystemContentBlock{
			&types.SystemContentBlockMemberText{Value: cs.systemPrompt},
		}
	}

	if len(cs.functionDefs) > 0 {
		tools := cs.buildTools()
		if len(tools) > 0 {
			input.ToolConfig = &types.ToolConfiguration{
				Tools: tools,
				ToolChoice: &types.ToolChoiceMemberAny{
					Value: types.AnyToolChoice{},
				},
			}
		}
	}

	return input
}

func (cs *bedrockChatSession) buildTools() []types.Tool {
	if len(cs.functionDefs) == 0 {
		return []types.Tool{}
	}

	tools := make([]types.Tool, 0, len(cs.functionDefs))

	for _, funcDef := range cs.functionDefs {
		if funcDef == nil {
			continue
		}

		toolSpec := &types.ToolSpecification{
			Name:        aws.String(funcDef.Name),
			Description: aws.String(funcDef.Description),
		}

		if funcDef.Parameters != nil {
			schemaMap := convertSchemaToMap(funcDef.Parameters)
			if schemaMap != nil {
				schemaDoc := document.NewLazyDocument(schemaMap)
				toolSpec.InputSchema = &types.ToolInputSchemaMemberJson{
					Value: schemaDoc,
				}
			}
		}
		tool := &types.ToolMemberToolSpec{
			Value: *toolSpec,
		}

		tools = append(tools, tool)
	}

	return tools
}

func convertSchemaToMap(schema *gollm.Schema) map[string]any {
	if schema == nil {
		return nil
	}

	schemaMap := make(map[string]any)

	if schema.Type != "" {
		schemaMap["type"] = string(schema.Type)
	}
	if schema.Description != "" {
		schemaMap["description"] = schema.Description
	}
	if schema.Type == "object" && len(schema.Properties) > 0 {
		properties := make(map[string]any)
		for propName, prop := range schema.Properties {
			properties[propName] = convertSchemaToMap(prop)
		}
		schemaMap["properties"] = properties

		if len(schema.Required) > 0 {
			schemaMap["required"] = schema.Required
		}
	}
	if schema.Type == "array" && schema.Items != nil {
		schemaMap["items"] = convertSchemaToMap(schema.Items)
	}

	return schemaMap
}

func (cs *bedrockChatSession) parseConverseOutput(output *types.ConverseOutput) *bedrockChatResponse {
	response := &bedrockChatResponse{
		usage:     nil,
		toolCalls: []gollm.FunctionCall{},
		model:     cs.model,
		provider:  "bedrock",
	}

	if messageOutput, ok := (*output).(*types.ConverseOutputMemberMessage); ok {
		message := messageOutput.Value
		if len(message.Content) > 0 {
			var contentParts []string
			for _, content := range message.Content {
				switch c := content.(type) {
				case *types.ContentBlockMemberText:
					contentParts = append(contentParts, c.Value)
				case *types.ContentBlockMemberToolUse:
					toolCall := gollm.FunctionCall{}

					if c.Value.ToolUseId != nil {
						toolCall.ID = *c.Value.ToolUseId
					}
					if c.Value.Name != nil {
						toolCall.Name = *c.Value.Name
					}

					if c.Value.Input != nil {
						var inputValue any
						if err := c.Value.Input.UnmarshalSmithyDocument(&inputValue); err != nil {
							klog.Errorf("Failed to unmarshal document interface: %v", err)
							toolCall.Arguments = map[string]any{}
						} else {
							if argMap, ok := inputValue.(map[string]any); ok {
								toolCall.Arguments = argMap
							} else {
								klog.Errorf("Document value is not a map[string]any, got %T", inputValue)
								toolCall.Arguments = map[string]any{}
							}
						}
					} else {
						toolCall.Arguments = map[string]any{}
					}

					response.toolCalls = append(response.toolCalls, toolCall)
				}
			}
			response.content = strings.Join(contentParts, "\n")
		}
	} else {
		klog.Errorf("Unexpected ConverseOutput type: %T", *output)
		response.content = "Error: Unable to parse response"
	}

	return response
}

func (cs *bedrockChatSession) createStreamingIterator(output *bedrockruntime.ConverseStreamOutput) gollm.ChatResponseIterator {
	return func(yield func(gollm.ChatResponse, error) bool) {
		if output == nil || output.GetStream() == nil {
			yield(nil, fmt.Errorf("streaming output or stream is nil"))
			return
		}

		defer output.GetStream().Close()

		var fullContent strings.Builder
		var usage any

		for event := range output.GetStream().Events() {
			switch e := event.(type) {
			case *types.ConverseStreamOutputMemberMessageStart:
				klog.V(3).Info("Stream: Message started")

			case *types.ConverseStreamOutputMemberContentBlockStart:
				klog.V(3).Info("Stream: Content block started")

			case *types.ConverseStreamOutputMemberContentBlockDelta:
				if delta := e.Value.Delta; delta != nil {
					if textDelta, ok := delta.(*types.ContentBlockDeltaMemberText); ok {
						text := textDelta.Value
						fullContent.WriteString(text)

						response := &bedrockChatResponse{
							content:   text,
							usage:     nil,
							toolCalls: []gollm.FunctionCall{},
							model:     cs.model,
							provider:  "bedrock",
						}

						if !yield(response, nil) {
							return
						}
					}
				}

			case *types.ConverseStreamOutputMemberContentBlockStop:
				klog.V(3).Info("Stream: Content block stopped")

			case *types.ConverseStreamOutputMemberMessageStop:
				klog.V(2).Info("Stream: Message completed")
				if fullContent.Len() > 0 {
					cs.addTextMessage(types.ConversationRoleAssistant, fullContent.String())
				}

			case *types.ConverseStreamOutputMemberMetadata:
				if e.Value.Usage != nil {
					usage = e.Value.Usage

					if cs.client.clientOpts.UsageCallback != nil {
						if structuredUsage := convertAWSUsage(usage, cs.model, "bedrock"); structuredUsage != nil {
							cs.client.clientOpts.UsageCallback("bedrock", cs.model, *structuredUsage)
							klog.V(2).Infof("Usage callback invoked for streaming: %d tokens", structuredUsage.TotalTokens)
						}
					}

					finalResponse := &bedrockChatResponse{
						content:   "",
						usage:     usage,
						toolCalls: []gollm.FunctionCall{},
						model:     cs.model,
						provider:  "bedrock",
					}

					if !yield(finalResponse, nil) {
						return
					}
				}

			default:
				klog.V(3).Infof("Stream: Unknown event type: %T", e)
			}
		}

		if err := output.GetStream().Err(); err != nil {
			yield(nil, fmt.Errorf("stream error: %w", err))
		}
	}
}

func (cs *bedrockChatSession) IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	retryableErrors := []string{
		"throttling",
		"serviceunavailable",
		"internalservererror",
		"requesttimeout",
	}

	for _, retryableErr := range retryableErrors {
		if strings.Contains(errStr, retryableErr) {
			return true
		}
	}

	return false
}

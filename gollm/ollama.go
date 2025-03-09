package gollm

import (
	"context"
	"fmt"

	"github.com/ollama/ollama/api"
	"k8s.io/klog/v2"
)

const (
	defaultOllamaModel = "gemma2:latest"
)

type OllamaClient struct {
	client *api.Client
	model  string
}

type OllamaChat struct {
	client  *api.Client
	model   string
	history []api.Message
	tools   []api.Tool
}

var _ Client = &OllamaClient{}

func NewOllamaClient(ctx context.Context) (*OllamaClient, error) {
	client, err := api.ClientFromEnvironment()
	if err != nil {
		return nil, err
	}

	return &OllamaClient{
		client: client,
		model:  defaultOllamaModel,
	}, nil
}

func (c *OllamaClient) Close() error {
	return nil
}

func (c *OllamaClient) SetModel(model string) error {
	c.model = model
	return nil
}

func (c *OllamaClient) GenerateCompletion(ctx context.Context, request *CompletionRequest) (CompletionResponse, error) {
	req := &api.GenerateRequest{
		Model:  c.model,
		Prompt: request.Prompt,

		// set streaming to false
		Stream: new(bool),
	}

	var ollamaResponse *OllamaCompletionResponse

	respFunc := func(resp api.GenerateResponse) error {
		ollamaResponse = &OllamaCompletionResponse{response: resp.Response}
		return nil
	}

	err := c.client.Generate(ctx, req, respFunc)
	if err != nil {
		return nil, err
	}

	return ollamaResponse, nil
}

func (c *OllamaClient) ListModels(ctx context.Context) ([]string, error) {
	modelResponse, err := c.client.List(ctx)
	if err != nil {
		return nil, err
	}

	var models []string
	for _, model := range modelResponse.Models {
		models = append(models, model.Name)
	}

	return models, nil
}

func (c *OllamaClient) SetResponseSchema(schema *Schema) error {
	return nil
}

func (c *OllamaClient) StartChat(systemPrompt string) Chat {
	return &OllamaChat{
		client: c.client,
		model:  c.model,
		history: []api.Message{
			{
				Role:    "system",
				Content: systemPrompt,
			},
		},
	}
}

type OllamaCompletionResponse struct {
	response string
}

func (r *OllamaCompletionResponse) Response() string {
	return r.response
}

func (r *OllamaCompletionResponse) UsageMetadata() any {
	return nil
}

func (c *OllamaChat) Send(ctx context.Context, contents ...any) (ChatResponse, error) {
	log := klog.FromContext(ctx)
	for _, content := range contents {
		switch v := content.(type) {
		case string:
			message := api.Message{
				Role:    "user",
				Content: v,
			}
			c.history = append(c.history, message)
		case FunctionCallResult:
			message := api.Message{
				Role:    "user",
				Content: fmt.Sprintf("Function call result: %s", v.Result),
			}
			c.history = append(c.history, message)
		default:
			return nil, fmt.Errorf("unsupported content type: %T", v)
		}
	}

	req := &api.ChatRequest{
		Model:    c.model,
		Messages: c.history,
		// set streaming to false
		Stream: new(bool),
		Tools:  c.tools,
	}

	var ollamaResponse *OllamaChatResponse

	respFunc := func(resp api.ChatResponse) error {
		log.Info("recieved response from ollama", "resp", resp)
		ollamaResponse = &OllamaChatResponse{
			candidates: []*OllamaCandidate{
				{
					parts: []OllamaPart{
						{
							text:      resp.Message.Content,
							toolCalls: resp.Message.ToolCalls,
						},
					},
				},
			},
		}
		c.history = append(c.history, resp.Message)
		return nil
	}

	err := c.client.Chat(ctx, req, respFunc)
	if err != nil {
		return nil, err
	}

	log.Info("ollama response", "parsed_response", ollamaResponse)
	return ollamaResponse, nil
}

type OllamaChatResponse struct {
	candidates []*OllamaCandidate
}

func (r *OllamaChatResponse) String() string {
	return fmt.Sprintf("OllamaChatResponse{candidates=%v}", r.candidates)
}

func (r *OllamaChatResponse) UsageMetadata() any {
	return nil
}

func (r *OllamaChatResponse) Candidates() []Candidate {
	var cads []Candidate
	for _, candidate := range r.candidates {
		cads = append(cads, candidate)
	}
	return cads
}

type OllamaCandidate struct {
	parts []OllamaPart
}

func (r *OllamaCandidate) String() string {
	return r.parts[0].text
}

func (r *OllamaCandidate) Parts() []Part {
	var parts []Part
	for _, part := range r.parts {
		parts = append(parts, &OllamaPart{
			text:      part.text,
			toolCalls: part.toolCalls,
		})
	}
	return parts
}

type OllamaPart struct {
	text      string
	toolCalls []api.ToolCall
}

func (p *OllamaPart) AsText() (string, bool) {
	if len(p.text) > 0 {
		return p.text, true
	}
	return "", false
}

func (p *OllamaPart) AsFunctionCalls() ([]FunctionCall, bool) {
	if len(p.toolCalls) > 0 {
		var functionCalls []FunctionCall
		for _, toolCall := range p.toolCalls {
			functionCalls = append(functionCalls, FunctionCall{
				Name:      toolCall.Function.Name,
				Arguments: toolCall.Function.Arguments,
			})
		}
		return functionCalls, true
	}
	return nil, false
}

func (c *OllamaChat) SetFunctionDefinitions(functionDefinitions []*FunctionDefinition) error {
	var tools []api.Tool
	for _, functionDefinition := range functionDefinitions {
		tools = append(tools, fnDefToOllamaTool(functionDefinition))
	}
	c.tools = tools
	return nil
}

func fnDefToOllamaTool(fnDef *FunctionDefinition) api.Tool {
	tool := api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        fnDef.Name,
			Description: fnDef.Description,
			Parameters: struct {
				Type       string   `json:"type"`
				Required   []string `json:"required"`
				Properties map[string]struct {
					Type        string   `json:"type"`
					Description string   `json:"description"`
					Enum        []string `json:"enum,omitempty"`
				} `json:"properties"`
			}{
				Type:     "object",
				Required: fnDef.Parameters.Required,
				Properties: map[string]struct {
					Type        string   `json:"type"`
					Description string   `json:"description"`
					Enum        []string `json:"enum,omitempty"`
				}{},
			},
		},
	}

	for paramName, param := range fnDef.Parameters.Properties {
		tool.Function.Parameters.Properties[paramName] = struct {
			Type        string   `json:"type"`
			Description string   `json:"description"`
			Enum        []string `json:"enum,omitempty"`
		}{
			Type:        string(param.Type),
			Description: param.Description,
		}
	}

	return tool
}

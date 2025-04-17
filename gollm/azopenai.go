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
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/ai/azopenai"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

func init() {
	RegisterProvider("azopenai", azureOpenAIFactory)
}

func azureOpenAIFactory(ctx context.Context, u *url.URL) (Client, error) {
	return NewAzureOpenAIClient(ctx)
}

type AzureOpenAIClient struct {
	client *azopenai.Client
}

var _ Client = &AzureOpenAIClient{}

func NewAzureOpenAIClient(ctx context.Context) (*AzureOpenAIClient, error) {
	azureOpenAIEndpoint := os.Getenv("AZURE_OPENAI_ENDPOINT")
	if azureOpenAIEndpoint == "" {
		return nil, fmt.Errorf("AZURE_OPENAI_ENDPOINT environment variable not set")
	}

	azureOpenAIClient := AzureOpenAIClient{}
	azureOpenAIKey := os.Getenv("AZURE_OPENAI_API_KEY")
	if azureOpenAIKey != "" {
		keyCredential := azcore.NewKeyCredential(azureOpenAIKey)
		client, err := azopenai.NewClientWithKeyCredential(azureOpenAIEndpoint, keyCredential, nil)
		if err != nil {
			return nil, err
		}
		azureOpenAIClient.client = client
	} else {
		credential, err := azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			return nil, err
		}
		client, err := azopenai.NewClient(azureOpenAIEndpoint, credential, nil)
		if err != nil {
			return nil, err
		}
		azureOpenAIClient.client = client
	}

	return &azureOpenAIClient, nil

}

func (c *AzureOpenAIClient) Close() error {
	return nil
}

func (c *AzureOpenAIClient) GenerateCompletion(ctx context.Context, request *CompletionRequest) (CompletionResponse, error) {
	req := azopenai.ChatCompletionsOptions{
		Messages: []azopenai.ChatRequestMessageClassification{
			&azopenai.ChatRequestUserMessage{Content: azopenai.NewChatRequestUserMessageContent(request.Prompt)},
		},
		DeploymentName: &request.Model,
	}

	resp, err := c.client.GetChatCompletions(ctx, req, nil)
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) > 0 || resp.Choices[0].Message == nil || resp.Choices[0].Message.Content == nil {
		return nil, fmt.Errorf("invalid completion response: %v", resp)
	}

	return &AzureOpenAICompletionResponse{response: *resp.Choices[0].Message.Content}, nil
}

func (c *AzureOpenAIClient) ListModels(ctx context.Context) ([]string, error) {
	return nil, fmt.Errorf("listing models not supported yet for Azure OpenAI")
}

func (c *AzureOpenAIClient) SetResponseSchema(schema *Schema) error {
	return nil
}

func (c *AzureOpenAIClient) StartChat(systemPrompt string, model string) Chat {
	return &AzureOpenAIChat{
		client: c.client,
		model:  model,
		history: []azopenai.ChatRequestMessageClassification{
			&azopenai.ChatRequestSystemMessage{Content: azopenai.NewChatRequestSystemMessageContent(systemPrompt)},
		},
	}
}

type AzureOpenAICompletionResponse struct {
	response string
}

func (r *AzureOpenAICompletionResponse) Response() string {
	return r.response
}

func (r *AzureOpenAICompletionResponse) UsageMetadata() any {
	return nil
}

type AzureOpenAIChat struct {
	client  *azopenai.Client
	model   string
	history []azopenai.ChatRequestMessageClassification
	tools   []azopenai.ChatCompletionsToolDefinitionClassification
}

func (c *AzureOpenAIChat) Send(ctx context.Context, contents ...any) (ChatResponse, error) {
	for _, content := range contents {
		switch v := content.(type) {
		case string:
			message := azopenai.ChatRequestUserMessage{
				Content: azopenai.NewChatRequestUserMessageContent(v),
			}
			c.history = append(c.history, &message)
		case FunctionCallResult:
			message := azopenai.ChatRequestUserMessage{
				Content: azopenai.NewChatRequestUserMessageContent(fmt.Sprintf("Function call result: %s", v.Result)),
			}
			c.history = append(c.history, &message)
		default:
			return nil, fmt.Errorf("unsupported content type: %T", v)
		}
	}

	resp, err := c.client.GetChatCompletions(ctx, azopenai.ChatCompletionsOptions{
		DeploymentName: &c.model,
		Messages:       c.history,
		Tools:          c.tools,
	}, nil)
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from Azure OpenAI: %v", resp)
	}

	return &AzureOpenAIChatResponse{azureOpenAIResponse: resp}, nil
}

func (c *AzureOpenAIChat) IsRetryableError(err error) bool {
	// TODO: Implement this
	return false
}

func (c *AzureOpenAIChat) SendStreaming(ctx context.Context, contents ...any) (ChatResponseIterator, error) {
	// TODO: Implement streaming
	response, err := c.Send(ctx, contents...)
	if err != nil {
		return nil, err
	}
	return singletonChatResponseIterator(response), nil
}

type AzureOpenAIChatResponse struct {
	azureOpenAIResponse azopenai.GetChatCompletionsResponse
}

var _ ChatResponse = &AzureOpenAIChatResponse{}

func (r *AzureOpenAIChatResponse) MarshalJSON() ([]byte, error) {
	formatted := RecordChatResponse{
		Raw: r.azureOpenAIResponse,
	}
	return json.Marshal(&formatted)
}

func (r *AzureOpenAIChatResponse) String() string {
	return fmt.Sprintf("AzureOpenAIChatResponse{candidates=%v}", r.azureOpenAIResponse.Choices)
}

func (r *AzureOpenAIChatResponse) UsageMetadata() any {
	return r.azureOpenAIResponse.Usage
}

func (r *AzureOpenAIChatResponse) Candidates() []Candidate {
	var candidates []Candidate
	for _, candidate := range r.azureOpenAIResponse.Choices {
		candidates = append(candidates, &AzureOpenAICandidate{candidate: candidate})
	}
	return candidates
}

type AzureOpenAICandidate struct {
	candidate azopenai.ChatChoice
}

func (r *AzureOpenAICandidate) String() string {
	var response strings.Builder
	response.WriteString("[")
	for i, parts := range r.Parts() {
		if i > 0 {
			response.WriteString(", ")
		}
		text, ok := parts.AsText()
		if ok {
			response.WriteString(text)
		}
		functionCalls, ok := parts.AsFunctionCalls()
		if ok {
			response.WriteString("functionCalls=[")
			for _, functionCall := range functionCalls {
				response.WriteString(fmt.Sprintf("%q(args=%v)", functionCall.Name, functionCall.Arguments))
			}
			response.WriteString("]}")
		}
	}
	response.WriteString("]}")
	return response.String()
}

func (r *AzureOpenAICandidate) Parts() []Part {
	var parts []Part

	if r.candidate.Message != nil {
		parts = append(parts, &AzureOpenAIPart{
			text: r.candidate.Message.Content,
		})
	}

	for _, tool := range r.candidate.Message.ToolCalls {
		if tool == nil {
			continue
		}
		parts = append(parts, &AzureOpenAIPart{
			functionCall: tool.(*azopenai.ChatCompletionsFunctionToolCall).Function,
		})
	}

	return parts
}

type AzureOpenAIPart struct {
	text         *string
	functionCall *azopenai.FunctionCall
}

func (p *AzureOpenAIPart) AsText() (string, bool) {
	if p.text != nil && len(*p.text) > 0 {
		return *p.text, true
	}
	return "", false
}

func (p *AzureOpenAIPart) AsFunctionCalls() ([]FunctionCall, bool) {
	if p.functionCall != nil {
		argumentsObj := map[string]any{}
		err := json.Unmarshal([]byte(*p.functionCall.Arguments), &argumentsObj)
		if err != nil {
			return nil, false
		}
		functionCalls := []FunctionCall{
			{
				Name:      *p.functionCall.Name,
				Arguments: argumentsObj,
			},
		}
		return functionCalls, true
	}
	return nil, false
}

func (c *AzureOpenAIChat) SetFunctionDefinitions(functionDefinitions []*FunctionDefinition) error {
	var tools []azopenai.ChatCompletionsToolDefinitionClassification
	for _, functionDefinition := range functionDefinitions {
		tools = append(tools, &azopenai.ChatCompletionsFunctionToolDefinition{Function: fnDefToAzureOpenAITool(functionDefinition)})
	}
	c.tools = tools
	return nil
}

func fnDefToAzureOpenAITool(fnDef *FunctionDefinition) *azopenai.ChatCompletionsFunctionToolDefinitionFunction {
	properties := make(map[string]any)
	for paramName, param := range fnDef.Parameters.Properties {
		properties[paramName] = map[string]any{
			"type":        string(param.Type),
			"description": param.Description,
		}
	}
	parameters := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(fnDef.Parameters.Required) > 0 {
		parameters["required"] = fnDef.Parameters.Required
	}
	jsonBytes, _ := json.Marshal(parameters)

	tool := azopenai.ChatCompletionsFunctionToolDefinitionFunction{
		Name:        &fnDef.Name,
		Description: &fnDef.Description,
		Parameters:  jsonBytes,
	}

	return &tool
}

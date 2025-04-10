// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
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
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"google.golang.org/genai"

	"k8s.io/klog/v2"
)

func init() {
	RegisterProvider("gemini", geminiFactory)
	RegisterProvider("vertexai", vertexaiViaGeminiFactory)
}

func geminiFactory(ctx context.Context, u *url.URL) (Client, error) {
	opt := GeminiClientOptions{}
	opt.Backend = genai.BackendGeminiAPI

	return NewGeminiClient(ctx, opt)
}

func vertexaiViaGeminiFactory(ctx context.Context, u *url.URL) (Client, error) {
	opt := GeminiClientOptions{}
	opt.Backend = genai.BackendVertexAI

	return NewGeminiClient(ctx, opt)
}

// GeminiClientOptions are the options for the Gemini API client.
type GeminiClientOptions struct {
	// API Key for GenAI. Required for BackendGeminiAPI.
	APIKey string
	// Backend for GenAI. See Backend constants. Defaults to BackendGeminiAPI unless explicitly set to BackendVertexAI, or the environment variable GOOGLE_GENAI_USE_VERTEXAI is set to "1" or "true".
	Backend genai.Backend
	// GCP Project ID for Vertex AI. Required for BackendVertexAI.
	Project string
	// GCP Location/Region for Vertex AI. Required for BackendVertexAI. See https://cloud.google.com/vertex-ai/docs/general/locations
	Location string
}

// NewGeminiClient builds a client for the Gemini API.
func NewGeminiClient(ctx context.Context, opt GeminiClientOptions) (*GeminiClient, error) {
	var cc *genai.ClientConfig

	switch opt.Backend {
	case genai.BackendGeminiAPI:
		apiKey := opt.APIKey
		if apiKey == "" {
			apiKey = os.Getenv("GEMINI_API_KEY")
		}
		if apiKey == "" {
			return nil, fmt.Errorf("GEMINI_API_KEY environment variable not set")
		}
		cc = &genai.ClientConfig{
			APIKey:  apiKey,
			Backend: genai.BackendGeminiAPI,
		}
	case genai.BackendVertexAI:
		cc = &genai.ClientConfig{
			// Project ID is loaded from the GOOGLE_CLOUD_PROJECT environment variable
			// Location/Region is loaded from either GOOGLE_CLOUD_LOCATION or GOOGLE_CLOUD_REGION environment variable
			Backend:  genai.BackendVertexAI,
			Project:  opt.Project,
			Location: opt.Location,
		}
	default:
		return nil, fmt.Errorf("unknown backend %q", opt.Backend)
	}

	client, err := genai.NewClient(ctx, cc)
	if err != nil {
		return nil, fmt.Errorf("building gemini client: %w", err)
	}

	return &GeminiClient{
		client: client,
	}, nil
}

// GeminiClient is a client for the Gemini API.
// It implements the Client interface.
type GeminiClient struct {
	client *genai.Client

	// responseSchema will constrain the output to match the given schema
	responseSchema *genai.Schema
}

var _ Client = &GeminiClient{}

// ListModels lists the models available in the Gemini API.
func (c *GeminiClient) ListModels(ctx context.Context) (modelNames []string, err error) {
	for model, err := range c.client.Models.All(ctx) {
		if err != nil {
			return nil, fmt.Errorf("error listing models: %w", err)
		}
		modelNames = append(modelNames, strings.TrimPrefix(model.Name, "models/"))
	}
	return modelNames, nil
}

// Close frees the resources used by the client.
func (c *GeminiClient) Close() error {
	return nil
}

// SetResponseSchema constrains LLM responses to match the provided schema.
// Calling with nil will clear the current schema.
func (c *GeminiClient) SetResponseSchema(responseSchema *Schema) error {
	if responseSchema == nil {
		c.responseSchema = nil
		return nil
	}

	geminiSchema, err := toGeminiSchema(responseSchema)
	if err != nil {
		return err
	}

	c.responseSchema = geminiSchema
	return nil
}

func (c *GeminiClient) GenerateCompletion(ctx context.Context, request *CompletionRequest) (CompletionResponse, error) {
	log := klog.FromContext(ctx)

	var config *genai.GenerateContentConfig

	if c.responseSchema != nil {
		config = &genai.GenerateContentConfig{
			ResponseSchema:   c.responseSchema,
			ResponseMIMEType: "application/json",
		}
	}

	content := []*genai.Content{
		{Role: "user", Parts: []*genai.Part{{Text: request.Prompt}}},
	}

	log.Info("sending GenerateContent request to gemini", "content", content)
	result, err := c.client.Models.GenerateContent(ctx, request.Model, content, config)
	if err != nil {
		return nil, err
	}

	return &GeminiCompletionResponse{geminiResponse: result, text: result.Text()}, nil
}

// StartChat starts a new chat with the model.
func (c *GeminiClient) StartChat(systemPrompt string, model string) Chat {
	// Some values that are recommended by aistudio
	temperature := float32(1.0)
	topK := float32(40)
	topP := float32(0.95)
	maxOutputTokens := int32(8192)

	chat := &GeminiChat{
		model:  model,
		client: c.client,
		genConfig: &genai.GenerateContentConfig{
			SystemInstruction: &genai.Content{
				Parts: []*genai.Part{
					{Text: systemPrompt},
				},
			},
			Temperature:      &temperature,
			TopK:             &topK,
			TopP:             &topP,
			MaxOutputTokens:  maxOutputTokens,
			ResponseMIMEType: "text/plain",
		},
		history: []*genai.Content{},
	}

	if chat.model == "gemma-3-27b-it" {
		// Note: gemma-3-27b-it does not allow system prompt
		// xref: https://discuss.ai.google.dev/t/gemma-3-missing-features-despite-announcement/71692
		// TODO: remove this hack once gemma-3-27b-it supports system prompt
		chat.genConfig.SystemInstruction = nil
		chat.history = []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: systemPrompt}}},
		}
	}

	if c.responseSchema != nil {
		chat.genConfig.ResponseSchema = c.responseSchema
		chat.genConfig.ResponseMIMEType = "application/json"
	}
	return chat
}

// GeminiChat is a chat with the model.
// It implements the Chat interface.
type GeminiChat struct {
	model     string
	client    *genai.Client
	history   []*genai.Content
	genConfig *genai.GenerateContentConfig
}

// SetFunctionDefinitions sets the function definitions for the chat.
// This allows the LLM to call user-defined functions.
func (c *GeminiChat) SetFunctionDefinitions(functionDefinitions []*FunctionDefinition) error {
	var genaiFunctionDeclarations []*genai.FunctionDeclaration
	for _, functionDefinition := range functionDefinitions {
		parameters, err := toGeminiSchema(functionDefinition.Parameters)
		if err != nil {
			return err
		}
		genaiFunctionDeclarations = append(genaiFunctionDeclarations, &genai.FunctionDeclaration{
			Name:        functionDefinition.Name,
			Description: functionDefinition.Description,
			Parameters:  parameters,
		})
	}
	c.genConfig.Tools = []*genai.Tool{
		{
			FunctionDeclarations: genaiFunctionDeclarations,
		},
	}
	return nil
}

// toGeminiSchema converts our generic Schema to a genai.Schema
func toGeminiSchema(schema *Schema) (*genai.Schema, error) {
	ret := &genai.Schema{
		Description: schema.Description,
		Required:    schema.Required,
	}

	switch schema.Type {
	case TypeObject:
		ret.Type = genai.TypeObject
	case TypeString:
		ret.Type = genai.TypeString
	case TypeBoolean:
		ret.Type = genai.TypeBoolean
	case TypeInteger:
		ret.Type = genai.TypeInteger
	case TypeArray:
		ret.Type = genai.TypeArray
	default:
		return nil, fmt.Errorf("type %q not handled by genai.Schema", schema.Type)
	}
	if schema.Properties != nil {
		ret.Properties = make(map[string]*genai.Schema)
		for k, v := range schema.Properties {
			geminiValue, err := toGeminiSchema(v)
			if err != nil {
				return nil, err
			}
			ret.Properties[k] = geminiValue
		}
	}
	if schema.Items != nil {
		geminiValue, err := toGeminiSchema(schema.Items)
		if err != nil {
			return nil, err
		}
		ret.Items = geminiValue
	}
	return ret, nil
}

// SendMessage sends a message to the model.
// It returns a ChatResponse object containing the response from the model.
func (c *GeminiChat) Send(ctx context.Context, contents ...any) (ChatResponse, error) {
	log := klog.FromContext(ctx)
	log.V(1).Info("sending LLM request", "user", contents)

	genaiContent := &genai.Content{Role: "user"}
	for _, content := range contents {
		switch v := content.(type) {
		case string:
			genaiContent.Parts = append(genaiContent.Parts, genai.NewPartFromText(v))
		case FunctionCallResult:
			genaiContent.Parts = append(genaiContent.Parts, &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					ID:       v.ID,
					Name:     v.Name,
					Response: v.Result,
				},
			})
		default:
			return nil, fmt.Errorf("unexpected type of content: %T", content)
		}
	}
	c.history = append(c.history, genaiContent)
	result, err := c.client.Models.GenerateContent(ctx, c.model, c.history, c.genConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to generate content: %w", err)
	}
	if result == nil || len(result.Candidates) == 0 {
		return nil, fmt.Errorf("no response from Gemini")
	}
	c.history = append(c.history, result.Candidates[0].Content)
	geminiResponse := result
	log.V(1).Info("got LLM response", "response", geminiResponse)
	return &GeminiChatResponse{geminiResponse: geminiResponse}, nil
}

// GeminiChatResponse is a response from the Gemini API.
// It implements the ChatResponse interface.
type GeminiChatResponse struct {
	geminiResponse *genai.GenerateContentResponse
}

var _ ChatResponse = &GeminiChatResponse{}

func (r *GeminiChatResponse) MarshalJSON() ([]byte, error) {
	formatted := RecordChatResponse{
		Raw: r.geminiResponse,
	}
	return json.Marshal(&formatted)
}

// String returns a string representation of the response.
func (r *GeminiChatResponse) String() string {
	return r.geminiResponse.Text()
}

// UsageMetadata returns the usage metadata for the response.
func (r *GeminiChatResponse) UsageMetadata() any {
	return r.geminiResponse.UsageMetadata
}

// Candidates returns the candidates for the response.
func (r *GeminiChatResponse) Candidates() []Candidate {
	var candidates []Candidate
	for _, candidate := range r.geminiResponse.Candidates {
		candidates = append(candidates, &GeminiCandidate{candidate: candidate})
	}
	return candidates
}

// GeminiCandidate is a candidate for the response.
// It implements the Candidate interface.
type GeminiCandidate struct {
	candidate *genai.Candidate
}

// String returns a string representation of the response.
func (r *GeminiCandidate) String() string {
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

// Parts returns the parts of the candidate.
func (r *GeminiCandidate) Parts() []Part {
	var parts []Part
	if r.candidate.Content != nil {
		for _, part := range r.candidate.Content.Parts {
			parts = append(parts, &GeminiPart{part: *part})
		}
	}
	return parts
}

// GeminiPart is a part of a candidate.
// It implements the Part interface.
type GeminiPart struct {
	part genai.Part
}

// AsText returns the text of the part.
func (p *GeminiPart) AsText() (string, bool) {
	if p.part.Text != "" {
		return p.part.Text, true
	}
	return "", false
}

// AsFunctionCalls returns the function calls of the part.
func (p *GeminiPart) AsFunctionCalls() ([]FunctionCall, bool) {
	if p.part.FunctionCall != nil {
		return []FunctionCall{
			{
				ID:        p.part.FunctionCall.ID,
				Name:      p.part.FunctionCall.Name,
				Arguments: p.part.FunctionCall.Args,
			},
		}, true
	}
	return nil, false
}

type GeminiCompletionResponse struct {
	geminiResponse *genai.GenerateContentResponse
	text           string
}

var _ CompletionResponse = &GeminiCompletionResponse{}

func (r *GeminiCompletionResponse) MarshalJSON() ([]byte, error) {
	formatted := RecordCompletionResponse{
		Text: r.text,
		Raw:  r.geminiResponse,
	}
	return json.Marshal(&formatted)
}

func (r *GeminiCompletionResponse) Response() string {
	return r.text
}

func (r *GeminiCompletionResponse) UsageMetadata() any {
	return r.geminiResponse.UsageMetadata
}

func (r *GeminiCompletionResponse) String() string {
	return fmt.Sprintf("{text=%q}", r.text)
}

func (c *GeminiChat) IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	var apiErr genai.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case http.StatusConflict, http.StatusTooManyRequests,
			http.StatusInternalServerError, http.StatusBadGateway,
			http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		default:
			return false
		}
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	// Add other error checks specific to LLM clients if needed
	// e.g., if errors.Is(err, specificLLMRateLimitError) { return true }

	return false
}

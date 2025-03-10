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
	"fmt"
	"os"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"k8s.io/klog/v2"
)

const (
	geminiDefaultModel = "gemini-2.0-pro-exp-02-05"
)

// NewGeminiClient builds a client for the Gemini API.
func NewGeminiClient(ctx context.Context) (*GeminiClient, error) {
	var opts []option.ClientOption

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable not set")
	}

	opts = append(opts, option.WithAPIKey(apiKey))

	client, err := genai.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("building gemini client: %w", err)
	}
	model := geminiDefaultModel
	return &GeminiClient{
		client: client,
		model:  model,
	}, nil
}

// GeminiClient is a client for the Gemini API.
// It implements the Client interface.
type GeminiClient struct {
	client *genai.Client
	model  string

	// responseSchema will constrain the output to match the given schema
	responseSchema *genai.Schema
}

var _ Client = &GeminiClient{}

// ListModels lists the models available in the Gemini API.
func (c *GeminiClient) ListModels(ctx context.Context) (modelNames []string, err error) {
	models := c.client.ListModels(ctx)

	for {
		m, err := models.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			return nil, err
		}
		modelNames = append(modelNames, strings.TrimPrefix(m.Name, "models/"))
	}
	return modelNames, nil
}

// Close frees the resources used by the client.
func (c *GeminiClient) Close() error {
	return c.client.Close()
}

// SetModel sets the model to use for the client.
func (c *GeminiClient) SetModel(model string) error {
	c.model = model
	// TODO: validate model
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

	model := c.client.GenerativeModel(c.model)

	if c.responseSchema != nil {
		model.ResponseSchema = c.responseSchema
		model.ResponseMIMEType = "application/json"
	}

	var geminiParts []genai.Part

	geminiParts = append(geminiParts, genai.Text(request.Prompt))

	log.Info("sending GenerateContent request to gemini", "parts", geminiParts)
	geminiResponse, err := model.GenerateContent(ctx, geminiParts...)
	if err != nil {
		return nil, err
	}

	if len(geminiResponse.Candidates) == 0 {
		return nil, fmt.Errorf("got no responses from gemini")
	}

	if len(geminiResponse.Candidates) > 1 {
		log.Info("only considering first candidate")
		for i := 1; i < len(geminiResponse.Candidates); i++ {
			candidate := geminiResponse.Candidates[i]
			log.Info("ignoring candidate: %q", candidate.Content)
		}
	}
	var response strings.Builder
	candidate := geminiResponse.Candidates[0]
	for _, part := range candidate.Content.Parts {
		switch part := part.(type) {
		case genai.Text:
			if response.Len() != 0 {
				response.WriteString("\n")
			}
			response.WriteString(string(part))
		default:
			return nil, fmt.Errorf("unexpected type of content part: %T", part)
		}
	}

	return &GeminiCompletionResponse{geminiResponse: geminiResponse, text: response.String()}, nil
}

// StartChat starts a new chat with the model.
func (c *GeminiClient) StartChat(systemPrompt string) Chat {
	model := c.client.GenerativeModel(c.model)

	// Some values that are recommended by aistudio
	model.SetTemperature(1)
	model.SetTopK(40)
	model.SetTopP(0.95)
	model.SetMaxOutputTokens(8192)
	model.ResponseMIMEType = "text/plain"

	if c.responseSchema != nil {
		model.ResponseSchema = c.responseSchema
		model.ResponseMIMEType = "application/json"
	}

	if systemPrompt != "" {
		model.SystemInstruction = &genai.Content{
			Parts: []genai.Part{
				genai.Text(systemPrompt),
			},
		}
	} else {
		klog.Warningf("systemPrompt not provided")
	}

	chat := model.StartChat()

	return &GeminiChat{
		model: model,
		chat:  chat,
	}
}

// GeminiChat is a chat with the model.
// It implements the Chat interface.
type GeminiChat struct {
	model *genai.GenerativeModel
	chat  *genai.ChatSession
}

// SetFunctionDefinitions sets the function definitions for the chat.
// This allows the LLM to call user-defined functions.
func (c *GeminiChat) SetFunctionDefinitions(functionDefinitions []*FunctionDefinition) error {
	var geminiFunctionDefinitions []*genai.FunctionDeclaration
	for _, functionDefinition := range functionDefinitions {
		parameters, err := toGeminiSchema(functionDefinition.Parameters)
		if err != nil {
			return err
		}
		geminiFunctionDefinitions = append(geminiFunctionDefinitions, &genai.FunctionDeclaration{
			Name:        functionDefinition.Name,
			Description: functionDefinition.Description,
			Parameters:  parameters,
		})
	}

	c.model.Tools = append(c.model.Tools, &genai.Tool{
		FunctionDeclarations: geminiFunctionDefinitions,
	})
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
	log.Info("sending LLM request", "user", contents)

	var geminiParts []genai.Part
	for _, content := range contents {
		switch v := content.(type) {
		case string:
			geminiParts = append(geminiParts, genai.Text(v))
		case FunctionCallResult:
			geminiParts = append(geminiParts, genai.FunctionResponse{
				Name:     v.Name,
				Response: v.Result,
			})
		default:
			return nil, fmt.Errorf("unexpected type of content: %T", content)
		}
	}
	geminiResponse, err := c.chat.SendMessage(ctx, geminiParts...)
	if err != nil {
		return nil, err
	}
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
	var response strings.Builder
	response.WriteString("{candidates=[")
	for i, candidate := range r.Candidates() {
		if i > 0 {
			response.WriteString(", ")
		}
		response.WriteString(candidate.String())
	}
	response.WriteString("]}")
	return response.String()
}

// UsageMetadata returns the usage metadata for the response.
func (r *GeminiChatResponse) UsageMetadata() any {
	return r.geminiResponse.UsageMetadata
}

// Candidates returns the candidates for the response.
func (r *GeminiChatResponse) Candidates() []Candidate {
	var candidates []Candidate
	for _, candidate := range r.geminiResponse.Candidates {
		// klog.Infof("candidate: %+v", candidate)
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
			parts = append(parts, &GeminiPart{part: part})
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
	if text, ok := p.part.(genai.Text); ok {
		return string(text), true
	}
	return "", false
}

// AsFunctionCalls returns the function calls of the part.
func (p *GeminiPart) AsFunctionCalls() ([]FunctionCall, bool) {
	if functionCall, ok := p.part.(genai.FunctionCall); ok {
		var ret []FunctionCall
		ret = append(ret, FunctionCall{
			Name:      functionCall.Name,
			Arguments: functionCall.Args,
		})
		return ret, true
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

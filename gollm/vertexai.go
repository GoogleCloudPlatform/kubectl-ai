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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"cloud.google.com/go/vertexai/genai"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"k8s.io/klog/v2"
)

const (
	vertexaiDefaultModel = "gemini-2.0-pro-exp-02-05"
)

// NewVertexAIClient builds a client for the VertexAI API.
func NewVertexAIClient(ctx context.Context) (*VertexAIClient, error) {
	log := klog.FromContext(ctx)

	var opts []option.ClientOption

	creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/generative-language", "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, fmt.Errorf("finding default credentials: %w", err)
	}
	opts = append(opts, option.WithCredentials(creds))

	projectID := ""
	location := "us-central1"

	if projectID == "" {
		cmd := exec.CommandContext(ctx, "gcloud", "config", "get", "project")
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("cannot get project (using gcloud config get project): %w", err)
		}
		projectID = strings.TrimSpace(stdout.String())
		if projectID == "" {
			return nil, fmt.Errorf("project was not set in gcloud config")
		}
		log.Info("got project from gcloud config", "project", projectID)
	}

	// TODO: Detect and/or auto-enable "gcloud services enable aiplatform.googleapis.com" if not enabled?

	client, err := genai.NewClient(ctx, projectID, location, opts...)
	if err != nil {
		return nil, fmt.Errorf("building vertexai client: %w", err)
	}
	model := vertexaiDefaultModel
	return &VertexAIClient{
		client: client,
		model:  model,
	}, nil
}

// VertexAIClient is a client for the VertexAI API.
// It implements the Client interface.
type VertexAIClient struct {
	client *genai.Client
	model  string

	// responseSchema will constrain the output to match the given schema
	responseSchema *genai.Schema
}

var _ Client = &VertexAIClient{}

// Close frees the resources used by the client.
func (c *VertexAIClient) Close() error {
	return c.client.Close()
}

// SetModel sets the model to use for the client.
func (c *VertexAIClient) SetModel(model string) error {
	c.model = model
	// TODO: validate model
	return nil
}

// SetResponseSchema constrains LLM responses to match the provided schema.
// Calling with nil will clear the current schema.
func (c *VertexAIClient) SetResponseSchema(responseSchema *Schema) error {
	if responseSchema == nil {
		c.responseSchema = nil
		return nil
	}

	vertexAISchema, err := toVertexAISchema(responseSchema)
	if err != nil {
		return err
	}

	c.responseSchema = vertexAISchema
	return nil
}

func (c *VertexAIClient) GenerateCompletion(ctx context.Context, request *CompletionRequest) (CompletionResponse, error) {
	log := klog.FromContext(ctx)

	model := c.client.GenerativeModel(c.model)

	if c.responseSchema != nil {
		model.ResponseSchema = c.responseSchema
		model.ResponseMIMEType = "application/json"
	}

	var vertexaiParts []genai.Part

	vertexaiParts = append(vertexaiParts, genai.Text(request.Prompt))

	log.Info("sending GenerateContent request to vertexai", "parts", vertexaiParts)
	vertexaiResponse, err := model.GenerateContent(ctx, vertexaiParts...)
	if err != nil {
		return nil, err
	}

	if len(vertexaiResponse.Candidates) > 1 {
		klog.Infof("only considering first candidate")
		for i := 1; i < len(vertexaiResponse.Candidates); i++ {
			candidate := vertexaiResponse.Candidates[i]
			klog.Infof("ignoring candidate: %q", candidate.Content)
		}
	}
	var response strings.Builder
	candidate := vertexaiResponse.Candidates[0]
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

	return &VertexAICompletionResponse{vertexaiResponse: vertexaiResponse, text: response.String()}, nil
}

// StartChat starts a new chat with the model.
func (c *VertexAIClient) StartChat(systemPrompt string) Chat {
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

	return &VertexAIChat{
		model: model,
		chat:  chat,
	}
}

// VertexAIChat is a chat with the model.
// It implements the Chat interface.
type VertexAIChat struct {
	model *genai.GenerativeModel
	chat  *genai.ChatSession
}

// SetFunctionDefinitions sets the function definitions for the chat.
// This allows the LLM to call user-defined functions.
func (c *VertexAIChat) SetFunctionDefinitions(functionDefinitions []*FunctionDefinition) error {
	var vertexaiFunctionDefinitions []*genai.FunctionDeclaration
	for _, functionDefinition := range functionDefinitions {
		parameters, err := toVertexAISchema(functionDefinition.Parameters)
		if err != nil {
			return err
		}
		vertexaiFunctionDefinitions = append(vertexaiFunctionDefinitions, &genai.FunctionDeclaration{
			Name:        functionDefinition.Name,
			Description: functionDefinition.Description,
			Parameters:  parameters,
		})
	}

	c.model.Tools = append(c.model.Tools, &genai.Tool{
		FunctionDeclarations: vertexaiFunctionDefinitions,
	})
	return nil
}

// toVertexAISchema converts our generic Schema to a genai.Schema
func toVertexAISchema(schema *Schema) (*genai.Schema, error) {
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
			vertexaiValue, err := toVertexAISchema(v)
			if err != nil {
				return nil, err
			}
			ret.Properties[k] = vertexaiValue
		}
	}
	if schema.Items != nil {
		vertexItems, err := toVertexAISchema(schema.Items)
		if err != nil {
			return nil, err
		}
		ret.Items = vertexItems
	}
	return ret, nil
}

func (c *VertexAIChat) Send(ctx context.Context, contents ...any) (ChatResponse, error) {
	log := klog.FromContext(ctx)
	log.Info("sending LLM request", "user", contents)

	var vertexaiParts []genai.Part
	for _, content := range contents {
		switch v := content.(type) {
		case string:
			vertexaiParts = append(vertexaiParts, genai.Text(v))
		case FunctionCallResult:
			vertexaiParts = append(vertexaiParts, genai.FunctionResponse{
				Name:     v.Name,
				Response: v.Result,
			})
		default:
			return nil, fmt.Errorf("unexpected type of content: %T", content)
		}
	}
	vertexaiResponse, err := c.chat.SendMessage(ctx, vertexaiParts...)
	if err != nil {
		return nil, err
	}
	return &VertexAIChatResponse{vertexaiResponse: vertexaiResponse}, nil
}

// VertexAIChatResponse is a response from the VertexAI API.
// It implements the ChatResponse interface.
type VertexAIChatResponse struct {
	vertexaiResponse *genai.GenerateContentResponse
}

var _ ChatResponse = &VertexAIChatResponse{}

func (r *VertexAIChatResponse) MarshalJSON() ([]byte, error) {
	formatted := RecordChatResponse{
		Raw: r.vertexaiResponse,
	}
	return json.Marshal(&formatted)
}

// String returns a string representation of the response.
func (r *VertexAIChatResponse) String() string {
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
func (r *VertexAIChatResponse) UsageMetadata() any {
	return r.vertexaiResponse.UsageMetadata
}

// Candidates returns the candidates for the response.
func (r *VertexAIChatResponse) Candidates() []Candidate {
	var candidates []Candidate
	for _, candidate := range r.vertexaiResponse.Candidates {
		// klog.Infof("candidate: %+v", candidate)
		candidates = append(candidates, &VertexAICandidate{candidate: candidate})
	}
	return candidates
}

// VertexAICandidate is a candidate for the response.
// It implements the Candidate interface.
type VertexAICandidate struct {
	candidate *genai.Candidate
}

// String returns a string representation of the response.
func (r *VertexAICandidate) String() string {
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
func (r *VertexAICandidate) Parts() []Part {
	var parts []Part
	if r.candidate.Content != nil {
		for _, part := range r.candidate.Content.Parts {
			parts = append(parts, &VertexAIPart{part: part})
		}
	}
	return parts
}

// VertexAIPart is a part of a candidate.
// It implements the Part interface.
type VertexAIPart struct {
	part genai.Part
}

// AsText returns the text of the part.
func (p *VertexAIPart) AsText() (string, bool) {
	if text, ok := p.part.(genai.Text); ok {
		return string(text), true
	}
	return "", false
}

// AsFunctionCalls returns the function calls of the part.
func (p *VertexAIPart) AsFunctionCalls() ([]FunctionCall, bool) {
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

type VertexAICompletionResponse struct {
	vertexaiResponse *genai.GenerateContentResponse
	text             string
}

var _ CompletionResponse = &VertexAICompletionResponse{}

func (r *VertexAICompletionResponse) MarshalJSON() ([]byte, error) {
	formatted := RecordCompletionResponse{
		Text: r.text,
		Raw:  r.vertexaiResponse,
	}
	return json.Marshal(&formatted)
}

func (r *VertexAICompletionResponse) Response() string {
	return r.text
}

func (r *VertexAICompletionResponse) UsageMetadata() any {
	return r.vertexaiResponse.UsageMetadata
}

func (r *VertexAICompletionResponse) String() string {
	return fmt.Sprintf("{text=%q}", r.text)
}

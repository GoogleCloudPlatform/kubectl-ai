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
	"log"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/responses"
	"k8s.io/klog/v2"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
)

// Chat Session Implementation
type openAIResponseChatSession struct {
	client              openai.Client
	history             responses.ResponseInputParam
	model               string
	functionDefinitions []*FunctionDefinition      // Stored in gollm format
	tools               []responses.ToolUnionParam // Stored in OpenAI format

	// params to be intialized at the beginning of the session
	params responses.ResponseNewParams
}

// Ensure openAIChatSession implements the Chat interface.
var _ Chat = (*openAIChatSession)(nil)

// SetFunctionDefinitions stores the function definitions and converts them to OpenAI format.
func (cs *openAIResponseChatSession) SetFunctionDefinitions(defs []*FunctionDefinition) error {
	cs.functionDefinitions = defs
	cs.tools = nil // Clear previous tools
	if len(defs) > 0 {
		cs.tools = make([]responses.ToolUnionParam, len(defs))
		for i, gollmDef := range defs {
			klog.Infof("Processing function definition: %s", gollmDef.Name)
			// Process function parameters
			params, err := cs.convertFunctionParameters(gollmDef)
			if err != nil {
				return fmt.Errorf("failed to process parameters for function %s: %w", gollmDef.Name, err)
			}
			cs.tools[i] = responses.ToolUnionParam{
				OfFunction: &responses.FunctionToolParam{
					Name:        gollmDef.Name,
					Description: openai.String(gollmDef.Description),
					Parameters:  params,
				},
			}
		}
	}
	klog.V(1).Infof("Set %d function definitions for OpenAI chat session", len(cs.functionDefinitions))
	return nil
}

// Send sends the user message(s), appends to history, and gets the LLM response.
func (cs *openAIResponseChatSession) Send(ctx context.Context, contents ...any) (ChatResponse, error) {
	klog.V(1).InfoS("openAIChatSession.Send called", "model", cs.model, "history_len", len(cs.history))

	// TODO(droot): kubectl-ai agent uses SendStreaming instead of Send so deferred the implementation for now.
	return &openAIChatResponse{}, errors.ErrUnsupported
}

// SendStreaming sends the user message(s) and returns an iterator for the LLM response stream.
func (cs *openAIResponseChatSession) SendStreaming(ctx context.Context, contents ...any) (ChatResponseIterator, error) {
	klog.V(1).InfoS("Starting OpenAI streaming request", "model", cs.model)

	// Process and append messages to history
	if err := cs.addContentsToHistory(contents); err != nil {
		return nil, err
	}
	// Prepare and send API request
	cs.params.Input = responses.ResponseNewParamsInputUnion{
		OfInputItemList: cs.history,
	}
	cs.params.Tools = cs.tools

	klog.V(1).InfoS("Sending streaming request to OpenAI API",
		"model", cs.model,
		"messageCount", len(cs.params.Input.OfInputItemList),
		"toolCount", len(cs.params.Tools))

	resp, err := cs.client.Responses.New(ctx, cs.params)
	if err == nil {
		for _, output := range resp.Output {
			switch output.AsAny().(type) {
			case responses.ResponseFunctionToolCall:
				fc := output.AsFunctionCall()
				log.Printf("Inspected function call item: %+v", fc)
				fpP := fc.ToParam()
				cs.history = append(cs.history, responses.ResponseInputItemUnionParam{
					OfFunctionCall: &fpP,
				})
			case responses.ResponseReasoningItem:
				reason := output.AsReasoning()
				log.Printf("Inspected Reasoning item: %+v", reason)
				reasonParam := reason.ToParam()
				cs.history = append(cs.history, responses.ResponseInputItemUnionParam{
					OfReasoning: &reasonParam,
				})
			case responses.ResponseOutputMessage:
				msg := output.AsMessage()
				log.Printf("Inspected Output Message: %+v", msg.Content[0].Text)
				cs.history = append(cs.history,
					responses.ResponseInputItemParamOfOutputMessage([]responses.ResponseOutputMessageContentUnionParam{
						{
							OfOutputText: &responses.ResponseOutputTextParam{
								Annotations: []responses.ResponseOutputTextAnnotationUnionParam{},
								Text:        msg.Content[0].Text,
								Type:        "output_text",
							},
						},
					}, msg.ID, msg.Status),
				)
			default:
				log.Println("no variant present", output)
			}
		}
	}
	return singletonChatResponseIterator(&openAIResponseChatResponse{resp: resp}), err
}

// IsRetryableError determines if an error from the OpenAI API should be retried.
func (cs *openAIResponseChatSession) IsRetryableError(err error) bool {
	if err == nil {
		return false
	}
	return DefaultIsRetryableError(err)
}

func (cs *openAIResponseChatSession) Initialize(messages []*api.Message) error {
	klog.Warning("chat history persistence is not supported for provider 'openai-responses', using in-memory chat history")
	return nil
}

func (cs *openAIResponseChatSession) SaveMessages(path string) error {
	return fmt.Errorf("SaveMessages is not implemented for OpenAI Response API")
}

// Helper structs for ChatResponse interface
type openAIResponseChatResponse struct {
	resp *responses.Response
}

var _ ChatResponse = (*openAIResponseChatResponse)(nil)

func (r *openAIResponseChatResponse) UsageMetadata() any {
	return nil
}

func (r *openAIResponseChatResponse) Candidates() []Candidate {
	if r.resp == nil {
		return nil
	}
	var candidates []Candidate
	for _, output := range r.resp.Output {
		switch output.AsAny().(type) {
		case responses.ResponseFunctionToolCall, responses.ResponseOutputMessage:
			candidates = append(candidates, &openAIResponseCandidate{
				candidate: &output,
			})
		default:
			// skip reasoning messages because agentic loop doesn't know
			// how to handle them yet.
		}
	}
	return candidates
}

type openAIResponseCandidate struct {
	candidate *responses.ResponseOutputItemUnion
}

var _ Candidate = (*openAIResponseCandidate)(nil)

func (c *openAIResponseCandidate) Parts() []Part {
	if c.candidate == nil {
		return nil
	}
	// OpenAI message can have Content AND ToolCalls
	var parts []Part

	output := c.candidate
	switch output.AsAny().(type) {
	case responses.ResponseFunctionToolCall:
		fc := output.AsFunctionCall()
		toolCall, err := convertResponseToolCallToFunctionCall(fc)
		if err != nil {
			//
		}
		parts = append(parts, &openAIResponsePart{
			toolCall: toolCall,
		})
	case responses.ResponseReasoningItem:
		reason := output.AsReasoning()
		log.Printf("Inspected Reasoning item: %+v", reason)
	case responses.ResponseOutputMessage:
		msg := output.AsMessage()
		parts = append(parts, &openAIResponsePart{
			content: msg.Content[0].AsOutputText().Text,
		})
	default:
		log.Println("no variant present", output)
	}
	return parts
}

// String provides a simple string representation for logging/debugging.
func (c *openAIResponseCandidate) String() string {
	return fmt.Sprintf("%+v", c.candidate)
}

type openAIResponsePart struct {
	content  string
	toolCall FunctionCall
}

var _ Part = (*openAIResponsePart)(nil)

func (p *openAIResponsePart) AsText() (string, bool) {
	return p.content, p.content != ""
}

func (p *openAIResponsePart) AsFunctionCalls() ([]FunctionCall, bool) {
	return []FunctionCall{p.toolCall}, p.content == ""
}

// convertFunctionParameters handles the conversion of gollm parameters to OpenAI format
func (cs *openAIResponseChatSession) convertFunctionParameters(gollmDef *FunctionDefinition) (openai.FunctionParameters, error) {
	var params openai.FunctionParameters

	if gollmDef.Parameters == nil {
		return params, nil
	}

	// Convert the schema for OpenAI compatibility
	klog.V(2).Infof("Original schema for function %s: %+v", gollmDef.Name, gollmDef.Parameters)
	validatedSchema, err := convertSchemaForOpenAI(gollmDef.Parameters)
	if err != nil {
		return params, fmt.Errorf("schema conversion failed: %w", err)
	}
	klog.V(2).Infof("Converted schema for function %s: %+v", gollmDef.Name, validatedSchema)

	// Convert to raw schema bytes
	schemaBytes, err := cs.convertSchemaToBytes(validatedSchema, gollmDef.Name)
	if err != nil {
		return params, err
	}

	// Unmarshal into OpenAI parameters format
	if err := json.Unmarshal(schemaBytes, &params); err != nil {
		return params, fmt.Errorf("failed to unmarshal schema: %w", err)
	}

	return params, nil
}

// convertSchemaToBytes converts a validated schema to JSON bytes using OpenAI-specific marshaling
func (cs *openAIResponseChatSession) convertSchemaToBytes(schema *Schema, functionName string) ([]byte, error) {
	// Wrap the schema with OpenAI-specific marshaling behavior
	openAIWrapper := openAISchema{Schema: schema}

	bytes, err := json.Marshal(openAIWrapper)
	if err != nil {
		return nil, fmt.Errorf("failed to convert schema: %w", err)
	}

	klog.Infof("OpenAI schema for function %s: %s", functionName, string(bytes))

	return bytes, nil
}

// addContentsToHistory processes and appends user messages to chat history
func (cs *openAIResponseChatSession) addContentsToHistory(contents []any) error {
	for _, content := range contents {
		switch c := content.(type) {
		case string:
			klog.V(2).Infof("Adding user message to history: %s", c)
			cs.history = append(cs.history, responses.ResponseInputItemUnionParam{
				OfMessage: &responses.EasyInputMessageParam{
					Content: responses.EasyInputMessageContentUnionParam{
						OfString: openai.String(c),
					},
					Role: responses.EasyInputMessageRoleUser,
				},
			})
		case FunctionCallResult:
			klog.V(2).Infof("Adding tool call result to history: Name=%s, ID=%s", c.Name, c.ID)
			// Marshal the result map into a JSON string for the message content
			resultJSON, err := json.Marshal(c.Result)
			if err != nil {
				klog.Errorf("Failed to marshal function call result: %v", err)
				return fmt.Errorf("failed to marshal function call result %q: %w", c.Name, err)
			}
			// cs.history = append(cs.history, openai.ToolMessage(string(resultJSON), c.ID))
			cs.history = append(cs.history, responses.ResponseInputItemParamOfFunctionCallOutput(c.ID, string(resultJSON)))
		default:
			klog.Warningf("Unhandled content type: %T", content)
			return fmt.Errorf("unhandled content type: %T", content)
		}
	}
	return nil
}

// convertToolCallsToFunctionCalls converts OpenAI tool calls to gollm function calls
func convertResponseToolCallToFunctionCall(responseToolCall responses.ResponseFunctionToolCall) (FunctionCall, error) {
	fc := FunctionCall{}

	// Skip non-function tool calls
	if responseToolCall.Name == "" {
		klog.V(2).Infof("Skipping non-function tool call ID: %s", responseToolCall.ID)
		return fc, fmt.Errorf("missing name %v", responseToolCall)
	}
	fc.Name = responseToolCall.Name
	// Parse function arguments with error handling
	var args map[string]any
	if responseToolCall.Arguments != "" {
		if err := json.Unmarshal([]byte(responseToolCall.Arguments), &args); err != nil {
			klog.V(2).Infof("Error unmarshalling function arguments for %s: %v", fc.Name, err)
			args = make(map[string]any)
		}
	} else {
		args = make(map[string]any)
	}

	return FunctionCall{
		ID:        responseToolCall.CallID,
		Name:      responseToolCall.Name,
		Arguments: args,
	}, nil
}

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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"k8s.io/klog/v2"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
)

func init() {
	if err := RegisterProvider("llamacpp", llamacppFactory); err != nil {
		klog.Fatalf("Failed to register llamacpp provider: %v", err)
	}
}

// llamacppFactory is the provider factory function for llama.cpp.
// Supports ClientOptions for custom configuration, including skipVerifySSL.
func llamacppFactory(ctx context.Context, opts ClientOptions) (Client, error) {
	return NewLlamaCppClient(ctx, opts)
}

type LlamaCppClient struct {
	baseURL        *url.URL
	httpClient     *http.Client
	responseSchema *llamacppSchema
}

type LlamaCppChat struct {
	client  *LlamaCppClient
	model   string
	history []llamacppChatMessage
	tools   []llamacppTool
}

var _ Client = &LlamaCppClient{}

// NewLlamaCppClient creates a new client for llama.cpp.
// Supports custom HTTP client and skipVerifySSL via ClientOptions.
func NewLlamaCppClient(ctx context.Context, opts ClientOptions) (*LlamaCppClient, error) {
	host := os.Getenv("LLAMACPP_HOST")
	if host == "" {
		host = "http://127.0.0.1:8080/"
	}

	baseURL, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("parsing host %q: %w", host, err)
	}
	klog.Infof("using llama.cpp with base url %v", baseURL.String())

	httpClient := createCustomHTTPClient(opts.SkipVerifySSL)

	return &LlamaCppClient{
		baseURL:    baseURL,
		httpClient: httpClient,
	}, nil
}

func (c *LlamaCppClient) Close() error {
	return nil
}

func (c *LlamaCppClient) GenerateCompletion(ctx context.Context, request *CompletionRequest) (CompletionResponse, error) {
	llamacppRequest := &llamacppCompletionRequest{
		Prompt:     request.Prompt,
		JSONSchema: c.responseSchema,
	}

	llamacppResponse, err := c.doCompletion(ctx, llamacppRequest)
	if err != nil {
		return nil, err
	}

	if llamacppResponse.Content == "" {
		return nil, fmt.Errorf("no response returned from llamacpp")
	}

	response := &LlamaCppCompletionResponse{llamacppResponse: llamacppResponse}
	return response, nil
}

func (c *LlamaCppClient) doRequest(ctx context.Context, httpMethod, relativePath string, req any, response any) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("building json body: %w", err)
	}
	u := c.baseURL.JoinPath(relativePath)
	klog.V(2).Infof("sending %s request to %v: %v", httpMethod, u.String(), string(body))
	httpRequest, err := http.NewRequestWithContext(ctx, httpMethod, u.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building http request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")

	httpResponse, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return fmt.Errorf("performing http request: %w", err)
	}
	defer httpResponse.Body.Close()

	b, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}

	if httpResponse.StatusCode != 200 {
		return fmt.Errorf("unexpected http status: %q with response %q", httpResponse.Status, string(b))
	}

	if err := json.Unmarshal(b, response); err != nil {
		return fmt.Errorf("unmarshalling json response: %w", err)
	}

	return nil
}

func (c *LlamaCppClient) doCompletion(ctx context.Context, req *llamacppCompletionRequest) (*llamacppCompletionResponse, error) {
	completionResponse := &llamacppCompletionResponse{}
	if err := c.doRequest(ctx, "POST", "completion", req, completionResponse); err != nil {
		return nil, err
	}
	return completionResponse, nil
}

func (c *LlamaCppClient) doChat(ctx context.Context, req *llamacppChatRequest) (*llamacppChatResponse, error) {
	chatResponse := &llamacppChatResponse{}
	if err := c.doRequest(ctx, "POST", "v1/chat/completions", req, chatResponse); err != nil {
		return nil, err
	}
	return chatResponse, nil
}

func (c *LlamaCppClient) ListModels(ctx context.Context) ([]string, error) {
	return nil, fmt.Errorf("model switching not supported by llama.cpp")
}

func (c *LlamaCppClient) SetResponseSchema(responseSchema *Schema) error {
	llamaSchema := toLlamacppSchema(responseSchema)
	c.responseSchema = llamaSchema
	return nil
}

func (c *LlamaCppClient) StartChat(systemPrompt, model string) Chat {
	return &LlamaCppChat{
		client: c,
		model:  model,
		history: []llamacppChatMessage{
			{
				Role:    "system",
				Content: ptrTo(systemPrompt),
			},
		},
	}
}

type LlamaCppCompletionResponse struct {
	llamacppResponse *llamacppCompletionResponse
}

func (r *LlamaCppCompletionResponse) Response() string {
	return r.llamacppResponse.Content
}

func (r *LlamaCppCompletionResponse) UsageMetadata() any {
	return nil
}

func (c *LlamaCppChat) Send(ctx context.Context, contents ...any) (ChatResponse, error) {
	log := klog.FromContext(ctx)
	for _, content := range contents {
		switch v := content.(type) {
		case string:
			message := llamacppChatMessage{
				Role:    "user",
				Content: ptrTo(v),
			}
			c.history = append(c.history, message)
		case FunctionCallResult:
			resultJSON, err := json.Marshal(v.Result)
			if err != nil {
				return nil, fmt.Errorf("marshalling function call result: %w", err)
			}

			message := llamacppChatMessage{
				Role: "tool",
				// TODO: Do we need ToolCallID?  ToolCallID: toolCallId,
				Content: ptrTo(string(resultJSON)),
			}
			c.history = append(c.history, message)
		default:
			return nil, fmt.Errorf("unsupported content type: %T", v)
		}
	}

	req := &llamacppChatRequest{
		Model:    c.model,
		Messages: c.history,
		// Stream:   ptrTo(false),
		Tools: c.tools,
	}

	var llmacppResponse *LlamaCppChatResponse

	resp, err := c.client.doChat(ctx, req)
	if err != nil {
		return nil, err
	}

	log.V(2).Info("received response from llama.cpp", "resp", resp)
	llmacppResponse = &LlamaCppChatResponse{
		LlamaCppResponse: *resp,
	}
	for i, choice := range resp.Choices {
		candidate := &LlamaCppCandidate{}

		if choice.Message != nil && choice.Message.Content != nil {
			parts := &LlamaCppPart{
				text: *choice.Message.Content,
			}
			candidate.parts = append(candidate.parts, parts)
		}
		if choice.Message != nil && len(choice.Message.ToolCalls) != 0 {
			var functionCalls []FunctionCall
			for _, toolCall := range choice.Message.ToolCalls {
				functionCall := FunctionCall{
					Name: toolCall.Function.Name,
				}

				if toolCall.Function.Arguments != "" {
					arguments := make(map[string]any)
					if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &arguments); err != nil {
						return nil, fmt.Errorf("parsing function call arguments: %w", err)
					}
					functionCall.Arguments = arguments
				}
				functionCalls = append(functionCalls, functionCall)
			}

			parts := &LlamaCppPart{
				functionCalls: functionCalls,
			}
			candidate.parts = append(candidate.parts, parts)
		}
		llmacppResponse.candidates = append(llmacppResponse.candidates, candidate)

		if i == 0 {
			if choice.Message != nil {
				msg := llamacppChatMessage{
					Role:       "assistant",
					Content:    choice.Message.Content,
					ToolCalls:  choice.Message.ToolCalls,
					ToolCallID: choice.Message.ToolCallID,
				}
				c.history = append(c.history, msg)
			}
		}
	}

	return llmacppResponse, nil
}

func (c *LlamaCppChat) SendStreaming(ctx context.Context, contents ...any) (ChatResponseIterator, error) {
	// TODO: Implement streaming
	response, err := c.Send(ctx, contents...)
	if err != nil {
		return nil, err
	}
	return singletonChatResponseIterator(response), nil
}

func (c *LlamaCppChat) IsRetryableError(err error) bool {
	// TODO(droot): Implement this
	return false
}

func (c *LlamaCppChat) Initialize(messages []*api.Message) error {
	klog.Warning("chat history persistence is not supported for provider 'llamacpp', using in-memory chat history")
	return nil
}

func (c *LlamaCppChat) SaveMessages(path string) error {
	return fmt.Errorf("SaveMessages is not implemented for LlamaCpp")
}

func ptrTo[T any](t T) *T {
	return &t
}

type LlamaCppChatResponse struct {
	candidates       []*LlamaCppCandidate
	LlamaCppResponse llamacppChatResponse
}

var _ ChatResponse = &LlamaCppChatResponse{}

func (r *LlamaCppChatResponse) MarshalJSON() ([]byte, error) {
	formatted := RecordChatResponse{
		Raw: r.LlamaCppResponse,
	}
	return json.Marshal(&formatted)
}

func (r *LlamaCppChatResponse) String() string {
	return fmt.Sprintf("LlamaCppChatResponse{candidates=%v}", r.candidates)
}

// func (r *LlamaCppChatResponse) String() string {
// 	var sb strings.Builder

// 	fmt.Fprintf(&sb, "LlamaCppChatResponse{candidates=[")
// 	for _, candidate := range r.candidates {
// 		fmt.Fprintf(&sb, "%v", candidate)
// 	}
// 	fmt.Fprintf(&sb, "]}")
// 	return sb.String()
// }

func (r *LlamaCppChatResponse) UsageMetadata() any {
	return nil
}

func (r *LlamaCppChatResponse) Candidates() []Candidate {
	var cads []Candidate
	for _, candidate := range r.candidates {
		cads = append(cads, candidate)
	}
	return cads
}

type LlamaCppCandidate struct {
	parts []*LlamaCppPart
}

func (r *LlamaCppCandidate) String() string {
	return r.parts[0].text
}

func (r *LlamaCppCandidate) Parts() []Part {
	var out []Part
	for _, part := range r.parts {
		out = append(out, part)
	}
	return out
}

type LlamaCppPart struct {
	text          string
	functionCalls []FunctionCall
}

func (p *LlamaCppPart) AsText() (string, bool) {
	if len(p.text) > 0 {
		return p.text, true
	}
	return "", false
}

func (p *LlamaCppPart) AsFunctionCalls() ([]FunctionCall, bool) {
	if len(p.functionCalls) > 0 {
		return p.functionCalls, true
	}
	return nil, false
}

func (c *LlamaCppChat) SetFunctionDefinitions(functionDefinitions []*FunctionDefinition) error {
	var tools []llamacppTool
	for _, functionDefinition := range functionDefinitions {
		tools = append(tools, toLlamacppTool(functionDefinition))
	}
	c.tools = tools
	return nil
}

func toLlamacppTool(fnDef *FunctionDefinition) llamacppTool {
	function := &llamacppFunction{
		Description: fnDef.Description,
		Name:        fnDef.Name,
	}

	if fnDef.Parameters != nil {
		function.Parameters = toLlamacppSchema(fnDef.Parameters)
	}

	tool := llamacppTool{
		Type:     "function",
		Function: function,
	}

	return tool
}

func toLlamacppSchema(in *Schema) *llamacppSchema {
	if in == nil {
		return nil
	}

	out := &llamacppSchema{
		Type:        string(in.Type),
		Items:       toLlamacppSchema(in.Items),
		Description: in.Description,
		Required:    in.Required,
	}

	if in.Properties != nil {
		out.Properties = make(map[string]llamacppSchema, len(in.Properties))
		for k, v := range in.Properties {
			out.Properties[k] = *toLlamacppSchema(v)
		}
	}

	return out
}

type llamacppCompletionRequest struct {
	// See https://github.com/ggerganov/llama.cpp/blob/master/examples/server/README.md#post-completion-given-a-prompt-it-returns-the-predicted-completion

	Prompt string `json:"prompt,omitempty"`

	JSONSchema *llamacppSchema `json:"json_schema,omitempty"`
}

type llamacppCompletionResponse struct {
	Content string `json:"content,omitempty"`

	Index int32 `json:"index,omitempty"`

	IDSlot int32 `json:"id_slot,omitempty"`

	Stop bool `json:"stop,omitempty"`

	Model string `json:"model,omitempty"`

	TokensPredicted int32 `json:"tokens_predicted,omitempty"`

	TokensEvaluated int32 `json:"tokens_evaluated,omitempty"`

	// "generation_settings":{"n_predict":-1,"seed":4294967295,"temperature":0.800000011920929,"dynatemp_range":0.0,"dynatemp_exponent":1.0,"top_k":40,"top_p":0.949999988079071,"min_p":0.05000000074505806,"xtc_probability":0.0,"xtc_threshold":0.10000000149011612,"typical_p":1.0,"repeat_last_n":64,"repeat_penalty":1.0,"presence_penalty":0.0,"frequency_penalty":0.0,"dry_multiplier":0.0,"dry_base":1.75,"dry_allowed_length":2,"dry_penalty_last_n":16384,"dry_sequence_breakers":["\n",":","\"","*"],"mirostat":0,"mirostat_tau":5.0,"mirostat_eta":0.10000000149011612,"stop":[],"max_tokens":-1,"n_keep":0,"n_discard":0,"ignore_eos":false,"stream":false,"logit_bias":[],"n_probs":0,"min_keep":0,"grammar":"","grammar_lazy":false,"grammar_triggers":[],"preserved_tokens":[],"chat_format":"Content-only","samplers":["penalties","dry","top_k","typ_p","top_p","min_p","xtc","temperature"],"speculative.n_max":16,"speculative.n_min":0,"speculative.p_min":0.75,"timings_per_token":false,"post_sampling_probs":false,"lora":[]},
	// GenerationSettings llamacppGenerationSettings `json:"generation_settings,omitempty"`

	Prompt string `json:"prompt,omitempty"`

	HasNewLine bool `json:"has_new_line,omitempty"`

	Truncated bool `json:"truncated,omitempty"`

	StopType string `json:"stop_type,omitempty"`

	StoppingWord string `json:"stopping_word,omitempty"`

	TokensCached int32 `json:"tokens_cached,omitempty"`

	Timings llamacppTimings `json:"timings,omitempty"`
}

type llamacppTimings struct {
	PromptN             int32   `json:"prompt_n,omitempty"`
	PromptMs            float64 `json:"prompt_ms,omitempty"`
	PromptPerTokenMs    float64 `json:"prompt_per_token_ms,omitempty"`
	PromptPerSecond     float64 `json:"prompt_per_second,omitempty"`
	PredictedN          int32   `json:"predicted_n,omitempty"`
	PredictedMs         float64 `json:"predicted_ms,omitempty"`
	PredictedPerTokenMs float64 `json:"predicted_per_token_ms,omitempty"`
	PredictedPerSecond  float64 `json:"predicted_per_second,omitempty"`
}

type llamacppChatRequest struct {
	Model    string                `json:"model,omitempty"`
	Messages []llamacppChatMessage `json:"messages,omitempty"`
	Tools    []llamacppTool        `json:"tools,omitempty"`
}

type llamacppChatResponse struct {
	Choices           []llamacppChoice `json:"choices,omitempty"`
	Created           int64            `json:"created,omitempty"`
	Model             string           `json:"model,omitempty"`
	SystemFingerprint string           `json:"system_fingerprint,omitempty"`
	Object            string           `json:"object,omitempty"`
	Usage             *llamacppUsage   `json:"usage,omitempty"`
	Id                string           `json:"id,omitempty"`
	Timings           *llamacppTimings `json:"timings,omitempty"`
}

type llamacppChoice struct {
	FinishReason string               `json:"finish_reason,omitempty"`
	Index        int32                `json:"index,omitempty"`
	Message      *llamacppChatMessage `json:"message,omitempty"`
}

type llamacppUsage struct {
	CompletionTokens int32 `json:"completion_tokens,omitempty"`
	PromptTokens     int32 `json:"prompt_tokens,omitempty"`
	TotalTokens      int32 `json:"total_tokens,omitempty"`
}

type llamacppChatMessage struct {
	Role       string             `json:"role,omitempty"`
	Content    *string            `json:"content,omitempty"`
	ToolCalls  []llamacppToolCall `json:"tool_calls,omitempty"`
	ToolCallID string             `json:"tool_call_id,omitempty"`
}

type llamacppToolCall struct {
	Type     string               `json:"type,omitempty"`
	Function llamacppFunctionCall `json:"function,omitempty"`
}

type llamacppFunctionCall struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	ID        string `json:"id,omitempty"`
}

type llamacppTool struct {
	Type     string            `json:"type,omitempty"`
	Function *llamacppFunction `json:"function,omitempty"`
}

type llamacppFunction struct {
	Description string          `json:"description,omitempty"`
	Name        string          `json:"name,omitempty"`
	Parameters  *llamacppSchema `json:"parameters,omitempty"`
}

type llamacppSchema struct {
	Type        string                    `json:"type,omitempty"`
	Required    []string                  `json:"required,omitempty"`
	Items       *llamacppSchema           `json:"items,omitempty"`
	Properties  map[string]llamacppSchema `json:"properties,omitempty"`
	Description string                    `json:"description,omitempty"`
	Enum        []string                  `json:"enum,omitempty"`
}

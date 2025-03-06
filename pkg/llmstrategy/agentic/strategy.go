package agentic

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/journal"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/tools"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/ui"
	"k8s.io/klog/v2"
)

type Strategy struct {

	// Recorder captures events for diagnostics
	Recorder journal.Recorder

	LLMClient gollm.Client

	Tools map[string]tools.Tool

	chat gollm.Chat
}

func (s *Strategy) Init(ctx context.Context) error {
	systemPrompt := "" //"You are an expert in kubernetes"
	chat := s.LLMClient.StartChat(systemPrompt)

	var functionDefinitions []*gollm.FunctionDefinition
	for _, tool := range s.Tools {
		functionDefinitions = append(functionDefinitions, tool.BuildFunctionDefinition())
	}

	chat.SetFunctionDefinitions(functionDefinitions)
	s.chat = chat
	return nil
}

// type Message struct {
// 	Role    string `json:"role"`
// 	Content string `json:"content"`
// }

// type ReActResponse struct {
// 	Thought string  `json:"thought"`
// 	Answer  string  `json:"answer,omitempty"`
// 	Action  *Action `json:"action,omitempty"`
// }

// type Action struct {
// 	Name   string `json:"name"`
// 	Reason string `json:"reason"`
// 	Input  string `json:"input"`
// }

// // Data represents the structure of the data to be filled into the template.
// type Data struct {
// 	Query       string
// 	PastQueries string
// 	History     string
// 	Tools       string
// }

// // AskLLM asks the LLM for the next action, sending a prompt including the .History
// func (a *Strategy) AskLLM(ctx context.Context) (*ReActResponse, error) {
// 	log := klog.FromContext(ctx)
// 	log.Info("Asking LLM...")

// 	// data := Data{
// 	// 	Query:       a.Query,
// 	// 	PastQueries: a.PastQueries,
// 	// 	History:     a.History(),
// 	// 	Tools:       "kubectl, gcrane, bash",
// 	// }

// 	// log.Info("Thinking...", "prompt", prompt)

// 	response, err := a.chat.SendMessage(ctx, a.Query)
// 	if err != nil {
// 		return nil, fmt.Errorf("generating LLM completion: %w", err)
// 	}

// 	a.Recorder.Write(ctx, &journal.Event{
// 		Timestamp: time.Now(),
// 		Action:    "llm-response",
// 		Payload:   response,
// 	})

// 	reActResp, err := parseReActResponse(response.Response())
// 	if err != nil {
// 		return nil, fmt.Errorf("parsing ReAct response: %w", err)
// 	}
// 	return reActResp, nil
// }

// func (a *Strategy) History() string {
// 	var history strings.Builder
// 	for _, msg := range a.Messages {
// 		history.WriteString(fmt.Sprintf("%s: %s\n", msg.Role, msg.Content))
// 	}
// 	return history.String()
// }

func (a *Strategy) RunOnce(ctx context.Context, query string, u ui.UI) error {
	log := klog.FromContext(ctx)
	log.Info("Executing query:", "query", query)

	response, err := a.chat.SendMessage(ctx, query)
	if err != nil {
		return fmt.Errorf("conversing with LLM: %w", err)
	}

	for {
		a.Recorder.Write(ctx, &journal.Event{
			Timestamp: time.Now(),
			Action:    "llm-response",
			Payload:   response,
		})

		candidates := response.Candidates()
		if len(candidates) == 0 {
			return fmt.Errorf("no candidates in LLM response")
		}

		var functionCallResults []gollm.FunctionCallResult

		for _, part := range candidates[0].Parts() {
			text, ok := part.AsText()
			if ok {
				u.RenderOutput(ctx, text, ui.RenderMarkdown())
			}
			calls, ok := part.AsFunctionCalls()
			if ok {
				for _, call := range calls {
					u.RenderOutput(ctx, fmt.Sprintf("function call %+v\n", call))
					functionCallResult, err := a.doFunctionCall(ctx, call)
					if err != nil {
						return fmt.Errorf("executing function call: %w", err)
					}
					functionCallResults = append(functionCallResults, functionCallResult)
					u.RenderOutput(ctx, fmt.Sprintf("function call response %+v\n", functionCallResult))
				}
			}
		}

		if len(functionCallResults) == 0 {
			break

		}

		r, err := a.chat.SendFunctionResults(ctx, functionCallResults)
		if err != nil {
			return fmt.Errorf("sending function call results to LLM: %w", err)
		}
		response = r
	}

	return nil
}

func (a *Strategy) recordError(ctx context.Context, err error) error {
	return a.Recorder.Write(ctx, &journal.Event{
		Timestamp: time.Now(),
		Action:    "error",
		Payload:   err.Error(),
	})
}

// executeAction handles the execution of a single action
func (a *Strategy) doFunctionCall(ctx context.Context, functionCall gollm.FunctionCall) (gollm.FunctionCallResult, error) {
	// log := klog.FromContext(ctx)

	var result gollm.FunctionCallResult
	result.Name = functionCall.Name

	tool, ok := a.Tools[functionCall.Name]
	if !ok {
		return result, fmt.Errorf("unknown function %q", functionCall.Name)
	}

	invocation := reflect.New(reflect.TypeOf(tool).Elem()).Interface().(tools.Tool)
	if err := parseParameters(functionCall.Arguments, invocation); err != nil {
		return result, fmt.Errorf("parsing tool arguments: %w", err)
	}

	out, err := invocation.Execute(ctx)
	if err != nil {
		return result, err
	}

	{
		j, err := json.Marshal(out)
		if err != nil {
			return result, err
		}

		m := make(map[string]any)
		if err := json.Unmarshal(j, &m); err != nil {
			return result, fmt.Errorf("converting function results: %w", err)
		}
		result.Result = m
	}

	return result, nil

	// tool := a.Tools[action.Name]
	// if tool == nil {
	// 	a.addMessage(ctx, "system", fmt.Sprintf("Error: Tool %s not found", action.Name))
	// 	log.Info("Unknown action: ", "action", action.Name)
	// 	return "", fmt.Errorf("unknown action: %s", action.Name)
	// }

	// output, err := tool(action.Input, a.Kubeconfig, workDir)
	// if err != nil {
	// 	return fmt.Sprintf("Error executing %q command: %v", action.Name, err), err
	// }
	// return output, nil
}

// parseParameters parses the LLM parameters into the given struct.
func parseParameters(args map[string]any, out any) error {
	j, err := json.Marshal(args)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(j, out); err != nil {
		return err
	}
	return nil
}

// func (a *Strategy) addMessage(ctx context.Context, role, content string) error {
// 	log := klog.FromContext(ctx)
// 	log.Info("Tracing...")

// 	msg := Message{
// 		Role:    role,
// 		Content: content,
// 	}
// 	a.Messages = append(a.Messages, msg)
// 	a.Recorder.Write(ctx, &journal.Event{
// 		Timestamp: time.Now(),
// 		Action:    "trace",
// 		Payload:   msg,
// 	})

// 	return nil
// }

// // parseReActResponse parses the LLM response into a ReActResponse struct
// // This function assumes the input contains exactly one JSON code block
// // formatted with ```json and ``` markers. The JSON block is expected to
// // contain a valid ReActResponse object.
// func parseReActResponse(input string) (*ReActResponse, error) {
// 	cleaned := strings.TrimSpace(input)

// 	const jsonBlockMarker = "```json"
// 	first := strings.Index(cleaned, jsonBlockMarker)
// 	last := strings.LastIndex(cleaned, "```")
// 	if first == -1 || last == -1 {
// 		return nil, fmt.Errorf("no JSON code block found in %q", cleaned)
// 	}
// 	cleaned = cleaned[first+len(jsonBlockMarker) : last]

// 	cleaned = strings.ReplaceAll(cleaned, "\n", "")
// 	cleaned = strings.TrimSpace(cleaned)

// 	var reActResp ReActResponse
// 	if err := json.Unmarshal([]byte(cleaned), &reActResp); err != nil {
// 		return nil, fmt.Errorf("error parsing response %q: %w", cleaned, err)
// 	}
// 	return &reActResp, nil
// }

// func sanitizeToolInput(input string) string {
// 	return strings.TrimSpace(input)
// }

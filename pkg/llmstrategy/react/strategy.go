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

package react

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/journal"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/ui"
	"k8s.io/klog/v2"
)

type Strategy struct {
	LLM gollm.Client

	// PromptTemplateFile allows specifying a custom template file
	PromptTemplateFile string

	// Recorder captures events for diagnostics
	Recorder journal.Recorder

	RemoveWorkDir bool

	Messages         []Message
	MaxIterations    int
	CurrentIteration int

	PastQueries string

	Kubeconfig string

	Tools map[string]func(input string, kubeconfig string, workDir string) (string, error)
}

func (a *Strategy) RunOnce(ctx context.Context, query string, u ui.UI) error {
	log := klog.FromContext(ctx)
	log.Info("Executing query:", "query", query)

	// Create a temporary working directory
	workDir, err := os.MkdirTemp("", "agent-workdir-*")
	if err != nil {
		log.Error(err, "Failed to create temporary working directory")
		return err
	}
	if a.RemoveWorkDir {
		defer os.RemoveAll(workDir)
	}
	log.Info("Created temporary working directory", "workDir", workDir)

	// Main execution loop
	for a.CurrentIteration < a.MaxIterations {
		log.Info("Starting iteration", "iteration", a.CurrentIteration)

		// Get next action from LLM
		reActResp, err := a.AskLLM(ctx, query)
		if err != nil {
			log.Error(err, "Error asking LLM")
			u.RenderOutput(ctx, fmt.Sprintf("\nSorry, Couldn't complete the task. LLM error %v\n", err), ui.Foreground(ui.ColorRed))
			return err
		}

		// Log the thought process
		log.Info("Thinking...", "thought", reActResp.Thought)
		a.addMessage(ctx, "assistant", fmt.Sprintf("Thought: %s", reActResp.Thought))

		// Handle final answer
		if reActResp.Answer != "" {
			log.Info("Final answer received", "answer", reActResp.Answer)
			a.addMessage(ctx, "assistant", fmt.Sprintf("Final Answer: %s", reActResp.Answer))
			u.RenderOutput(ctx, reActResp.Answer, ui.RenderMarkdown())
			return nil
		}

		// Handle action
		if reActResp.Action != nil {
			log.Info("Executing action",
				"name", reActResp.Action.Name,
				"reason", reActResp.Action.Reason,
				"input", reActResp.Action.Input,
			)

			// Sanitize and prepare action
			reActResp.Action.Input = sanitizeToolInput(reActResp.Action.Input)
			a.addMessage(ctx, "assistant", fmt.Sprintf("Action: Using %s tool", reActResp.Action.Name))

			// Display action details
			u.RenderOutput(ctx, fmt.Sprintf("  Running: %s", reActResp.Action.Input), ui.Foreground(ui.ColorGreen))
			u.RenderOutput(ctx, reActResp.Action.Reason, ui.RenderMarkdown())

			// Execute action
			output, err := a.executeAction(ctx, reActResp.Action.Name, reActResp.Action.Input, workDir)
			if err != nil {
				log.Error(err, "Error executing action")
				return err
			}

			// Record observation
			observation := fmt.Sprintf("Observation from %s:\n%s", reActResp.Action.Name, output)
			a.addMessage(ctx, "system", observation)
		}

		a.CurrentIteration++
	}

	// Handle max iterations reached
	log.Info("Max iterations reached", "iterations", a.CurrentIteration)
	u.RenderOutput(ctx, fmt.Sprintf("\nSorry, Couldn't complete the task after %d attempts.\n", a.MaxIterations), ui.Foreground(ui.ColorRed))
	return a.recordError(ctx, fmt.Errorf("max iterations reached"))
}

// executeAction handles the execution of a single action
func (a *Strategy) executeAction(ctx context.Context, actionName string, actionInput string, workDir string) (string, error) {
	log := klog.FromContext(ctx)

	tool := a.Tools[actionName]
	if tool == nil {
		a.addMessage(ctx, "system", fmt.Sprintf("Error: Tool %s not found", actionName))
		log.Info("Unknown action: ", "action", actionName)
		return "", fmt.Errorf("unknown action: %s", actionName)
	}

	output, err := tool(actionInput, a.Kubeconfig, workDir)
	if err != nil {
		return fmt.Sprintf("Error executing %q command: %v", actionName, err), err
	}
	return output, nil
}

// AskLLM asks the LLM for the next action, sending a prompt including the .History
func (a *Strategy) AskLLM(ctx context.Context, query string) (*ReActResponse, error) {
	log := klog.FromContext(ctx)
	log.Info("Asking LLM...")

	data := Data{
		Query:       query,
		PastQueries: a.PastQueries,
		History:     a.History(),
		Tools:       "kubectl, gcrane, bash",
	}

	prompt, err := a.generatePrompt(ctx, defaultReActPromptTemplate, data)
	if err != nil {
		log.Error(err, "generating from template")
		return nil, err
	}

	log.Info("Thinking...", "prompt", prompt)

	response, err := a.LLM.GenerateCompletion(ctx, &gollm.CompletionRequest{
		Prompt: prompt,
	})
	if err != nil {
		return nil, fmt.Errorf("generating LLM completion: %w", err)
	}

	a.Recorder.Write(ctx, &journal.Event{
		Timestamp: time.Now(),
		Action:    "llm-response",
		Payload:   response,
	})

	reActResp, err := parseReActResponse(response.Response())
	if err != nil {
		return nil, fmt.Errorf("parsing ReAct response: %w", err)
	}
	return reActResp, nil
}

func sanitizeToolInput(input string) string {
	return strings.TrimSpace(input)
}

func (a *Strategy) addMessage(ctx context.Context, role, content string) error {
	log := klog.FromContext(ctx)
	log.Info("Tracing...")

	msg := Message{
		Role:    role,
		Content: content,
	}
	a.Messages = append(a.Messages, msg)
	a.Recorder.Write(ctx, &journal.Event{
		Timestamp: time.Now(),
		Action:    "trace",
		Payload:   msg,
	})

	return nil
}

func (a *Strategy) recordError(ctx context.Context, err error) error {
	return a.Recorder.Write(ctx, &journal.Event{
		Timestamp: time.Now(),
		Action:    "error",
		Payload:   err.Error(),
	})
}

func (a *Strategy) History() string {
	var history strings.Builder
	for _, msg := range a.Messages {
		history.WriteString(fmt.Sprintf("%s: %s\n", msg.Role, msg.Content))
	}
	return history.String()
}

type ReActResponse struct {
	Thought string  `json:"thought"`
	Answer  string  `json:"answer,omitempty"`
	Action  *Action `json:"action,omitempty"`
}

type Action struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
	Input  string `json:"input"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Data represents the structure of the data to be filled into the template.
type Data struct {
	Query       string
	PastQueries string
	History     string
	Tools       string
}

// generateFromTemplate generates a prompt for LLM. It uses the prompt from the provides template file or default.
func (a *Strategy) generatePrompt(_ context.Context, defaultPromptTemplate string, data Data) (string, error) {
	var tmpl *template.Template
	var err error

	if a.PromptTemplateFile != "" {
		// Read custom template file
		content, err := os.ReadFile(a.PromptTemplateFile)
		if err != nil {
			return "", fmt.Errorf("error reading template file: %v", err)
		}
		tmpl, err = template.New("customTemplate").Parse(string(content))
		if err != nil {
			return "", fmt.Errorf("error parsing custom template: %v", err)
		}
	} else {
		// Use default template
		tmpl, err = template.New("promptTemplate").Parse(defaultPromptTemplate)
		if err != nil {
			return "", err
		}
	}

	// Use a strings.Builder for efficient string concatenation
	var result strings.Builder
	// Execute the template, writing the output to the strings.Builder
	err = tmpl.Execute(&result, data)
	if err != nil {
		return "", err
	}

	return result.String(), nil
}

// parseReActResponse parses the LLM response into a ReActResponse struct
// This function assumes the input contains exactly one JSON code block
// formatted with ```json and ``` markers. The JSON block is expected to
// contain a valid ReActResponse object.
func parseReActResponse(input string) (*ReActResponse, error) {
	cleaned := strings.TrimSpace(input)

	const jsonBlockMarker = "```json"
	first := strings.Index(cleaned, jsonBlockMarker)
	last := strings.LastIndex(cleaned, "```")
	if first == -1 || last == -1 {
		return nil, fmt.Errorf("no JSON code block found in %q", cleaned)
	}
	cleaned = cleaned[first+len(jsonBlockMarker) : last]

	cleaned = strings.ReplaceAll(cleaned, "\n", "")
	cleaned = strings.TrimSpace(cleaned)

	var reActResp ReActResponse
	if err := json.Unmarshal([]byte(cleaned), &reActResp); err != nil {
		return nil, fmt.Errorf("parsing JSON %q: %w", cleaned, err)
	}
	return &reActResp, nil
}

// defaultReActPromptTemplate is the default prompt template for the ReAct agent.
const defaultReActPromptTemplate = `You are a Kubernetes Assistant tasked with answering the following query:

<query> {{.Query}} </query>

Your goal is to reason about the query and decide on the best course of action to answer it accurately.

Previous reasoning steps and observations (if any):
<previous-steps>
{{.History}}
</previous-steps>

Available tools: {{.Tools}}

Instructions:
1. Analyze the query, previous reasoning steps, and observations.
2. Decide on the next action: use a tool or provide a final answer.
3. Respond in the following JSON format:

If you need to use a tool:
{
    "thought": "Your detailed reasoning about what to do next",
    "action": {
        "name": "Tool name (kubectl, gcrane, cat, echo)",
        "reason": "Explanation of why you chose this tool (not more than 100 words)",
        "input": "complete command to be executed."
    }
}

If you have enough information to answer the query:
{
    "thought": "Your final reasoning process",
    "answer": "Your comprehensive answer to the query"
}

Remember:
- Be thorough in your reasoning.
- For creating new resources, try to create the resource using the tools available. DO NOT ask the user to create the resource.
- Prefer the tool usage that does not require any interactive input.
- Use tools when you need more information. Do not respond with the instructions on how to use the tools or what commands to run, instead just use the tool.
- Always base your reasoning on the actual observations from tool use.
- If a tool returns no results or fails, acknowledge this and consider using a different tool or approach.
- Provide a final answer only when you're confident you have sufficient information.
- If you cannot find the necessary information after using available tools, admit that you don't have enough information to answer the query confidently.
- Feel free to respond with emjois where appropriate.

Additional information from the previous queries (if any):
<previous-queries>
{{.PastQueries}}
</previous-queries>
`

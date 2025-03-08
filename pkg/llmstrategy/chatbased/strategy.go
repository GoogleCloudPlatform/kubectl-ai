package chatbased

import (
	"context"
	"fmt"
	"html/template"
	"os"
	"strings"

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

	MaxIterations    int
	CurrentIteration int

	Kubeconfig string

	Tools map[string]func(input string, kubeconfig string, workDir string) (string, error)
}

// ExecuteChatBased executes a chat-based agentic loop with the LLM using function calling.
func (a *Strategy) RunOnce(ctx context.Context, query string, u ui.UI) error {
	log := klog.FromContext(ctx)
	log.Info("Starting chat loop for query:", "query", query)
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

	systemPrompt, err := a.generatePrompt(ctx, defaultSystemPromptChatAgent)
	if err != nil {
		log.Error(err, "Failed to generate system prompt")
		return err
	}

	// Start a new chat session
	chat := a.LLM.StartChat(systemPrompt)

	// Define the kubectl function
	kubectlFunction := &gollm.FunctionDefinition{
		Name:        "kubectl",
		Description: "Executes a kubectl command against user's Kubernetes cluster. Use this tool only when you need to query or modify the state of user's Kubernetes cluster.",
		Parameters: &gollm.Schema{
			Type: gollm.TypeObject,
			Properties: map[string]*gollm.Schema{
				"command": {
					Type: gollm.TypeString,
					Description: `The complete kubectl command to execute. Please including the kubectl prefix as well.

Example:
user: what pods are running in the cluster?
assistant: kubectl get pods

user: what is the status of the pod my-pod?
assistant: kubectl get pod my-pod -o jsonpath='{.status.phase}'
`,
				},
			},
		},
	}

	// make the tools available to the LLM
	if err := chat.SetFunctionDefinitions([]*gollm.FunctionDefinition{kubectlFunction}); err != nil {
		log.Error(err, "Failed to set function definitions")
		return err
	}

	// currChatContent tracks chat content that needs to be sent
	// to the LLM in each iteration of  the agentic loop below
	var currChatContent []any

	// Set the initial message to start the conversation
	currChatContent = []any{fmt.Sprintf("can you help me with query: %q", query)}

	for a.CurrentIteration < a.MaxIterations {
		log.Info("Starting iteration", "iteration", a.CurrentIteration)

		response, err := chat.Send(ctx, currChatContent...)
		if err != nil {
			log.Error(err, "Error sending initial message")
			return err
		}
		currChatContent = nil

		if len(response.Candidates()) == 0 {
			log.Error(nil, "No candidates in response")
			return fmt.Errorf("no candidates in LLM response")
		}

		candidate := response.Candidates()[0]

		// Process each part of the response
		var functionCalls []gollm.FunctionCall

		for _, part := range candidate.Parts() {
			// Check if it's a text response
			if text, ok := part.AsText(); ok {
				log.Info("text response", "text", text)
				textResponse := text
				// If we have a text response, render it
				if textResponse != "" {
					u.RenderOutput(ctx, textResponse, ui.RenderMarkdown())
					currChatContent = append(currChatContent, textResponse)
				}
			}

			// Check if it's a function call
			if calls, ok := part.AsFunctionCalls(); ok && len(calls) > 0 {
				log.Info("function calls", "calls", calls)
				functionCalls = append(functionCalls, calls...)

				// TODO(droot): Run all function calls in parallel
				// (may have to specify in the prompt to make these function calls independent)
				for _, call := range calls {
					functionName := call.Name
					command, _ := call.Arguments["command"].(string)
					log.Info("function call", "functionName", functionName, "command", command)
					u.RenderOutput(ctx, fmt.Sprintf("  Running: %s\n", command), ui.Foreground(ui.ColorGreen))

					output, err := a.executeAction(ctx, functionName, command, workDir)
					if err != nil {
						log.Error(err, "Error executing action")
						return err
					}

					currChatContent = append(currChatContent, gollm.FunctionCallResult{
						Name: functionName,
						Result: map[string]any{
							"command": command,
							"output":  output,
						},
					})

				}
			}
		}

		// If no function calls were made, we're done
		if len(functionCalls) == 0 {
			return nil
		}

		a.CurrentIteration++
	}

	// If we've reached the maximum number of iterations
	log.Info("Max iterations reached", "iterations", a.MaxIterations)
	u.RenderOutput(ctx, fmt.Sprintf("\nSorry, couldn't complete the task after %d iterations.\n", a.MaxIterations), ui.Foreground(ui.ColorRed))
	return fmt.Errorf("max iterations reached")
}

// executeAction handles the execution of a single action
func (a *Strategy) executeAction(ctx context.Context, actionName string, actionInput string, workDir string) (string, error) {
	log := klog.FromContext(ctx)

	tool := a.Tools[actionName]
	if tool == nil {
		log.Info("Unknown action: ", "action", actionName)
		return "", fmt.Errorf("tool %q not found", actionName)
	}

	output, err := tool(actionInput, a.Kubeconfig, workDir)
	if err != nil {
		return fmt.Sprintf("Error executing %q command: %v", actionName, err), err
	}
	return output, nil
}

// generateFromTemplate generates a prompt for LLM. It uses the prompt from the provides template file or default.
func (a *Strategy) generatePrompt(_ context.Context, defaultPromptTemplate string) (string, error) {
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

	data := map[string]string{}

	// Use a strings.Builder for efficient string concatenation
	var result strings.Builder
	// Execute the template, writing the output to the strings.Builder
	err = tmpl.Execute(&result, data)
	if err != nil {
		return "", err
	}

	return result.String(), nil
}

// system prompt template for the chat based agent.
const defaultSystemPromptChatAgent = `You are a Kubernetes Assistant and your role is assist the user with their kubernetes related queries and tasks.
Your goal is to reason about the query and decide on the best course of action to answer it accurately.

Instructions:
- Be thorough in your reasoning.
- Don't just reason about the query, but also take actions to answer the query.
- For creating new resources, try to create the resource using the tools available. DO NOT ask the user to create the resource.
- Prefer the tool usage that does not require any interactive input.
- Use tools when you need more information.
- Always base your reasoning on the actual observations from tool use.
- If a tool returns no results or fails, acknowledge this and consider using a different tool or approach.
- Provide a final answer only when you're confident you have sufficient information.
- If you cannot find the necessary information after using available tools, admit that you don't have enough information to answer the query confidently.
- Feel free to respond with emojis where appropriate.
`

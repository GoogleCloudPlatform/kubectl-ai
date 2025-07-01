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

package agent

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/journal"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/tools"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/ui"
	"k8s.io/klog/v2"
)

//go:embed systemprompt_template_default.txt
var defaultSystemPromptTemplate string

type AgentState string

const (
	AgentStateIdle    AgentState = "idle"
	AgentStateRunning AgentState = "running"
	AgentStateWaiting AgentState = "waiting"
	AgentStateDone    AgentState = "done"
)

type Agent struct {
	Input  chan any
	Output chan any

	// tool calls that are pending execution
	// These will typically be all the tool calls suggested by the LLM in the
	// previous iteration of the agentic loop.
	pendingFunctionCalls []ToolCallAnalysis

	// currChatContent tracks chat content that needs to be sent
	// to the LLM in the current iteration of the agentic loop.
	currChatContent []any

	// currIteration tracks the current iteration of the agentic loop.
	currIteration int

	// state tracks the current state of the agent.
	state AgentState

	LLM gollm.Client

	// PromptTemplateFile allows specifying a custom template file
	PromptTemplateFile string
	// ExtraPromptPaths allows specifying additional prompt templates
	// to be combined with PromptTemplateFile
	ExtraPromptPaths []string
	Model            string

	RemoveWorkDir bool

	MaxIterations int

	// Kubeconfig is the path to the kubeconfig file.
	Kubeconfig string

	SkipPermissions bool

	Tools tools.Tools

	EnableToolUseShim bool

	// MCPClientEnabled indicates whether MCP client mode is enabled
	MCPClientEnabled bool

	// Recorder captures events for diagnostics
	Recorder journal.Recorder

	// doc is the document which renders the conversation
	doc *ui.Document

	llmChat gollm.Chat

	workDir string
}

func (s *Agent) Init(ctx context.Context, doc *ui.Document) error {
	log := klog.FromContext(ctx)

	s.Input = make(chan any, 10)
	// TODO: this need to be probably a buffered channel because
	// there is out multiple messages that can be sent to the output channel
	// and we don't want to block the main loop.
	s.Output = make(chan any, 10)
	s.currIteration = 0
	// when we support session, we will need to initialize this with the
	// current history of the conversation.
	s.currChatContent = []any{}

	// today, the agent always starts in idle state.
	// in the future, we will support session, and the agent will start in
	// the state of the previous session.
	s.state = AgentStateIdle

	// Create a temporary working directory
	workDir, err := os.MkdirTemp("", "agent-workdir-*")
	if err != nil {
		log.Error(err, "Failed to create temporary working directory")
		return err
	}

	log.Info("Created temporary working directory", "workDir", workDir)

	systemPrompt, err := s.generatePrompt(ctx, defaultSystemPromptTemplate, PromptData{
		Tools:             s.Tools,
		EnableToolUseShim: s.EnableToolUseShim,
	})
	if err != nil {
		return fmt.Errorf("generating system prompt: %w", err)
	}

	// Start a new chat session
	s.llmChat = gollm.NewRetryChat(
		s.LLM.StartChat(systemPrompt, s.Model),
		gollm.RetryConfig{
			MaxAttempts:    3,
			InitialBackoff: 10 * time.Second,
			MaxBackoff:     60 * time.Second,
			BackoffFactor:  2,
			Jitter:         true,
		},
	)

	if !s.EnableToolUseShim {
		var functionDefinitions []*gollm.FunctionDefinition
		for _, tool := range s.Tools.AllTools() {
			functionDefinitions = append(functionDefinitions, tool.FunctionDefinition())
		}
		// Sort function definitions to help KV cache reuse
		sort.Slice(functionDefinitions, func(i, j int) bool {
			return functionDefinitions[i].Name < functionDefinitions[j].Name
		})
		if err := s.llmChat.SetFunctionDefinitions(functionDefinitions); err != nil {
			return fmt.Errorf("setting function definitions: %w", err)
		}
	}
	s.workDir = workDir
	s.doc = doc

	return nil
}

func (c *Agent) Close() error {
	if c.workDir != "" {
		if c.RemoveWorkDir {
			if err := os.RemoveAll(c.workDir); err != nil {
				klog.Warningf("error cleaning up directory %q: %v", c.workDir, err)
			}
		}
	}
	return nil
}

func (c *Agent) Run(ctx context.Context) error {
	log := klog.FromContext(ctx)

	log.Info("Starting agent loop")

	if c.currIteration == 0 && c.state == AgentStateIdle {
		c.Output <- ui.NewAgentTextBlock().WithText("Hey there, what can I help you with today?")
		// c.OutputCh <- ui.NewInputTextBlock().SetEditable(true)
	}

	// main agent loop
	go func() {
		for {
			var userInput any
			switch c.state {
			case AgentStateIdle, AgentStateDone:
				c.Output <- ui.NewInputTextBlock()
				select {
				case <-ctx.Done():
					log.Info("Agent loop done")
					return
				case userInput = <-c.Input:
				}
			case AgentStateWaiting:
				select {
				case <-ctx.Done():
					log.Info("Agent loop done")
					return
				case userInput = <-c.Input:
				}
			}

			if userInput == io.EOF {
				log.Info("Agent loop done")
				return
			}

			if userInput != nil {
				log.Info("User input", "userInput", userInput)
				switch u := userInput.(type) {
				case string:
					log.Info("Text input", "text", u)

					if c.state == AgentStateIdle || c.state == AgentStateDone {
						c.state = AgentStateRunning
						c.currIteration = 0
						c.currChatContent = []any{u}
						c.pendingFunctionCalls = []ToolCallAnalysis{}
					} else {
						klog.Errorf("invalid state: %v", c.state)
						continue
					}
				case int32:
					if c.state != AgentStateWaiting {
						klog.Errorf("invalid state for choice: %v", c.state)
						continue
					}
					dispatchToolCalls := c.handleChoice(ctx, u)
					if dispatchToolCalls {
						// execute all pending function calls
						for _, call := range c.pendingFunctionCalls {
							// Only show "Running" message and proceed with execution for non-interactive commands
							toolDescription := call.ParsedToolCall.Description()
							functionCallRequestBlock := ui.NewFunctionCallRequestBlock().SetDescription(toolDescription)
							// c.doc.AddBlock(functionCallRequestBlock)
							c.Output <- functionCallRequestBlock

							output, err := call.ParsedToolCall.InvokeTool(ctx, tools.InvokeToolOptions{
								Kubeconfig: c.Kubeconfig,
								WorkDir:    c.workDir,
							})
							if err != nil {
								log.Error(err, "error executing action", "output", output)
								c.state = AgentStateDone
								c.pendingFunctionCalls = []ToolCallAnalysis{}
								continue
							}

							// Handle timeout message using UI blocks
							if execResult, ok := output.(*tools.ExecResult); ok && execResult != nil && execResult.StreamType == "timeout" {
								infoBlock := ui.NewAgentTextBlock().WithText("\nTimeout reached after 7 seconds\n")
								// c.doc.AddBlock(infoBlock)
								c.Output <- infoBlock
							}

							// Add the tool call result to maintain conversation flow
							functionCallRequestBlock.SetResult(output)

							// If shim is disabled, convert the result to a map and append FunctionCallResult
							result, err := tools.ToolResultToMap(output)
							if err != nil {
								log.Error(err, "error converting tool result to map", "output", output)
								c.state = AgentStateDone
								c.pendingFunctionCalls = []ToolCallAnalysis{}
								continue
							}

							c.currChatContent = append(c.currChatContent, gollm.FunctionCallResult{
								ID:     call.FunctionCall.ID,
								Name:   call.FunctionCall.Name,
								Result: result,
							})
						}
						// Clear pending function calls after execution
						c.pendingFunctionCalls = []ToolCallAnalysis{}
						c.state = AgentStateRunning
						c.currIteration = c.currIteration + 1
					} else {
						// if user has declined, we are done with this iteration
						c.state = AgentStateDone
					}
				default:
					klog.Errorf("invalid user input: %v", userInput)
				}
			}

			if c.state == AgentStateRunning {
				if c.currIteration >= c.MaxIterations {
					c.Output <- ui.NewAgentTextBlock().WithText("Maximum number of iterations reached.")
					c.state = AgentStateDone
					c.pendingFunctionCalls = []ToolCallAnalysis{}
					continue
				}

				var agentTextBlock *ui.AgentTextBlock

				// We create the agent text block here; this lets renderers render a "thinking" state
				// before the first response arrives.
				agentTextBlock = ui.NewAgentTextBlock()
				agentTextBlock.SetStreaming(true)
				// a.doc.AddBlock(agentTextBlock)
				c.Output <- agentTextBlock

				// we run the agentic loop for one iteration
				stream, err := c.llmChat.SendStreaming(ctx, c.currChatContent...)
				if err != nil {
					log.Error(err, "error sending streaming LLM response")
					c.state = AgentStateDone
					c.pendingFunctionCalls = []ToolCallAnalysis{}
					continue
				}

				// Clear our "response" now that we sent the last response
				c.currChatContent = nil

				if c.EnableToolUseShim {
					// convert the candidate response into a gollm.ChatResponse
					stream, err = candidateToShimCandidate(stream)
					if err != nil {
						c.state = AgentStateDone
						c.pendingFunctionCalls = []ToolCallAnalysis{}
						continue
					}
				}
				// Process each part of the response
				var functionCalls []gollm.FunctionCall

				var streamedText string

				for response, err := range stream {
					if err != nil {
						log.Error(err, "error reading streaming LLM response")
						c.state = AgentStateDone
						c.pendingFunctionCalls = []ToolCallAnalysis{}
						break
					}
					if response == nil {
						// end of streaming response
						break
					}
					// klog.Infof("response: %+v", response)

					if len(response.Candidates()) == 0 {
						log.Error(nil, "No candidates in response")
						c.state = AgentStateDone
						c.pendingFunctionCalls = []ToolCallAnalysis{}
						break
					}

					candidate := response.Candidates()[0]

					for _, part := range candidate.Parts() {
						// Check if it's a text response
						if text, ok := part.AsText(); ok {
							log.Info("text response", "text", text)
							streamedText += text
							agentTextBlock = ui.NewAgentTextBlock().WithText(text)
							agentTextBlock.SetStreaming(true)
							c.Output <- agentTextBlock
						}

						// Check if it's a function call
						if calls, ok := part.AsFunctionCalls(); ok && len(calls) > 0 {
							log.Info("function calls", "calls", calls)
							functionCalls = append(functionCalls, calls...)
						}
					}
				}
				if agentTextBlock != nil {
					agentTextBlock.SetStreaming(false)
					c.Output <- agentTextBlock
				}
				log.Info("streamedText", "streamedText", streamedText)

				// If no function calls to be made, we're done
				if len(functionCalls) == 0 {
					log.Info("No function calls to be made, so most likely the task is completed, so we're done.")
					c.state = AgentStateDone
					c.currChatContent = []any{}
					c.currIteration = 0
					c.pendingFunctionCalls = []ToolCallAnalysis{}
					// c.OutputCh <- ui.NewAgentTextBlock().WithText("Task completed.")
					continue
				}

				toolCallAnalysisResults, err := c.analyzeToolCalls(ctx, functionCalls)
				if err != nil {
					log.Error(err, "error analyzing tool calls")
					c.state = AgentStateDone
					c.pendingFunctionCalls = []ToolCallAnalysis{}
					continue
				}

				// mark the tools for dispatching
				c.pendingFunctionCalls = toolCallAnalysisResults

				interactiveToolCallIndex := -1
				modifiesResourceToolCallIndex := -1
				for i, result := range toolCallAnalysisResults {
					if result.ModifiesResourceStr != "no" {
						modifiesResourceToolCallIndex = i
					}
					if result.IsInteractive {
						interactiveToolCallIndex = i
					}
				}

				if interactiveToolCallIndex >= 0 {
					// Show error block for both shim enabled and disabled modes
					errorBlock := ui.NewErrorBlock().SetText(fmt.Sprintf("  %s\n", toolCallAnalysisResults[interactiveToolCallIndex].IsInteractiveError.Error()))
					// c.doc.AddBlock(errorBlock)
					c.Output <- errorBlock

					// For models with tool-use support (shim disabled), use proper FunctionCallResult
					// Note: This assumes the model supports sending FunctionCallResult
					c.currChatContent = append(c.currChatContent, gollm.FunctionCallResult{
						ID:     toolCallAnalysisResults[interactiveToolCallIndex].FunctionCall.ID,
						Name:   toolCallAnalysisResults[interactiveToolCallIndex].FunctionCall.Name,
						Result: map[string]any{"error": toolCallAnalysisResults[interactiveToolCallIndex].IsInteractiveError.Error()},
					})
					c.pendingFunctionCalls = []ToolCallAnalysis{} // reset pending function calls
					continue                                      // Skip execution for interactive commands
				}

				if !c.SkipPermissions && modifiesResourceToolCallIndex >= 0 {
					var commandDescriptions []string
					for _, call := range c.pendingFunctionCalls {
						commandDescriptions = append(commandDescriptions, call.ParsedToolCall.Description())
					}
					confirmationPrompt := "The following commands require your approval to run:\n* " + strings.Join(commandDescriptions, "\n* ")
					confirmationPrompt += "\nDo you want to proceed ?"

					optionsBlock := ui.NewInputOptionBlock().SetPrompt(confirmationPrompt)
					optionsBlock.AddOption("yes", "Yes", "yes", "y")
					optionsBlock.AddOption("yes_and_dont_ask_me_again", "Yes, and don't ask me again")
					optionsBlock.AddOption("no", "No", "no", "n")
					// c.doc.AddBlock(optionsBlock)
					c.Output <- optionsBlock

					// Request input from the user by sending a ui.InputOptionBlock on the output channel
					// remainining part of the loop will be now resumed when we receive a choice input
					// from the user.
					c.state = AgentStateWaiting
					continue
				}

				// now we are in the clear to dispatch the tool calls

				// execute all pending function calls
				for _, call := range c.pendingFunctionCalls {
					// Only show "Running" message and proceed with execution for non-interactive commands
					toolDescription := call.ParsedToolCall.Description()
					functionCallRequestBlock := ui.NewFunctionCallRequestBlock().SetDescription(toolDescription)
					// c.doc.AddBlock(functionCallRequestBlock)
					c.Output <- functionCallRequestBlock

					output, err := call.ParsedToolCall.InvokeTool(ctx, tools.InvokeToolOptions{
						Kubeconfig: c.Kubeconfig,
						WorkDir:    c.workDir,
					})
					if err != nil {
						log.Error(err, "error executing action", "output", output)
						c.state = AgentStateDone
						c.pendingFunctionCalls = []ToolCallAnalysis{}
						continue
					}

					// Handle timeout message using UI blocks
					if execResult, ok := output.(*tools.ExecResult); ok && execResult != nil && execResult.StreamType == "timeout" {
						infoBlock := ui.NewAgentTextBlock().WithText("\nTimeout reached after 7 seconds\n")
						// c.doc.AddBlock(infoBlock)
						c.Output <- infoBlock
					}

					// Add the tool call result to maintain conversation flow
					functionCallRequestBlock.SetResult(output)

					// If shim is disabled, convert the result to a map and append FunctionCallResult
					result, err := tools.ToolResultToMap(output)
					if err != nil {
						log.Error(err, "error converting tool result to map", "output", output)
						c.state = AgentStateDone
						c.pendingFunctionCalls = []ToolCallAnalysis{}
						continue
					}

					c.currChatContent = append(c.currChatContent, gollm.FunctionCallResult{
						ID:     call.FunctionCall.ID,
						Name:   call.FunctionCall.Name,
						Result: result,
					})
				}
				c.pendingFunctionCalls = []ToolCallAnalysis{}

				c.currIteration = c.currIteration + 1
			}
		}
	}()

	return nil
}

// The key idea is to treat all tool calls to be executed atomically or not
// If all tool calls are readonly call, it is straight forward
// if some of the tool calls are not readonly, then the interesting question is should the permission
// be asked for each of the tool call or only once for all the tool calls.
// I think treating all tool calls as atomic is the right thing to do.

type ToolCallAnalysis struct {
	FunctionCall        gollm.FunctionCall
	ParsedToolCall      *tools.ToolCall
	IsInteractive       bool
	IsInteractiveError  error
	ModifiesResourceStr string
}

func (c *Agent) analyzeToolCalls(ctx context.Context, toolCalls []gollm.FunctionCall) ([]ToolCallAnalysis, error) {
	toolCallAnalysis := make([]ToolCallAnalysis, len(toolCalls))
	for i, call := range toolCalls {
		toolCallAnalysis[i].FunctionCall = call
		toolCall, err := c.Tools.ParseToolInvocation(ctx, call.Name, call.Arguments)
		if err != nil {
			return nil, fmt.Errorf("error parsing tool call: %w", err)
		}
		toolCallAnalysis[i].IsInteractive, err = toolCall.GetTool().IsInteractive(call.Arguments)
		if err != nil {
			toolCallAnalysis[i].IsInteractiveError = err
		}
		modifiesResourceStr := toolCall.GetTool().CheckModifiesResource(call.Arguments)
		if modifiesResourceStr == "unknown" {
			if llmModifies, ok := call.Arguments["modifies_resource"].(string); ok {
				modifiesResourceStr = llmModifies
			}
		}
		toolCallAnalysis[i].ModifiesResourceStr = modifiesResourceStr
		toolCallAnalysis[i].ParsedToolCall = toolCall
	}
	return toolCallAnalysis, nil
}

func (c *Agent) handleChoice(ctx context.Context, choice int32) (dispatchToolCalls bool) {
	log := klog.FromContext(ctx)
	// if user input is a choice and use has declined the operation,
	// we need to abort all pending function calls.
	// update the currChatContent with the choice and keep the agent loop running.

	// Normalize the input
	switch choice {
	case 1:
		dispatchToolCalls = true
	case 2:
		c.SkipPermissions = true
		dispatchToolCalls = true
	case 3:
		infoBlock := ui.NewAgentTextBlock().WithText("Operation was skipped. User declined to run this operation.")
		// c.doc.AddBlock(infoBlock)
		c.Output <- infoBlock

		c.currChatContent = append(c.currChatContent, gollm.FunctionCallResult{
			ID:   c.pendingFunctionCalls[0].FunctionCall.ID,
			Name: c.pendingFunctionCalls[0].FunctionCall.Name,
			Result: map[string]any{
				"error":     "User declined to run this operation.",
				"status":    "declined",
				"retryable": false,
			},
		})
		c.pendingFunctionCalls = []ToolCallAnalysis{}
		dispatchToolCalls = false
	default:
		// This case should technically not be reachable due to AskForConfirmation loop
		err := fmt.Errorf("invalid confirmation choice: %q", choice)
		log.Error(err, "Invalid choice received from AskForConfirmation")
		// a.doc.AddBlock(ui.NewErrorBlock().SetText("Invalid choice received. Cancelling operation."))
		c.Output <- ui.NewErrorBlock().SetText("Invalid choice received. Cancelling operation.")
		c.pendingFunctionCalls = []ToolCallAnalysis{}
		dispatchToolCalls = false
	}
	return dispatchToolCalls
}

// RunOneRound executes a chat-based agentic loop with the LLM using function calling.
func (a *Agent) RunOneRound(ctx context.Context, query string) error {
	log := klog.FromContext(ctx)
	log.Info("Starting chat loop for query:", "query", query)

	// currChatContent tracks chat content that needs to be sent
	// to the LLM in each iteration of  the agentic loop below
	var currChatContent []any

	// Set the initial message to start the conversation
	currChatContent = []any{query}

	currentIteration := 0
	maxIterations := a.MaxIterations

	for currentIteration < maxIterations {
		log.Info("Starting iteration", "iteration", currentIteration)

		a.Recorder.Write(ctx, &journal.Event{
			Timestamp: time.Now(),
			Action:    "llm-chat",
			Payload:   []any{currChatContent},
		})

		var agentTextBlock *ui.AgentTextBlock

		// We create the agent text block here; this lets renderers render a "thinking" state
		// before the first response arrives.
		agentTextBlock = ui.NewAgentTextBlock()
		agentTextBlock.SetStreaming(true)
		a.doc.AddBlock(agentTextBlock)

		stream, err := a.llmChat.SendStreaming(ctx, currChatContent...)
		if err != nil {
			return err
		}

		// Clear our "response" now that we sent the last response
		currChatContent = nil

		if a.EnableToolUseShim {
			// convert the candidate response into a gollm.ChatResponse
			stream, err = candidateToShimCandidate(stream)
			if err != nil {
				return err
			}
		}

		// Process each part of the response
		var functionCalls []gollm.FunctionCall

		for response, err := range stream {
			if err != nil {
				log.Error(err, "error reading streaming LLM response")
				return fmt.Errorf("reading streaming LLM response: %w", err)
			}
			if response == nil {
				// end of streaming response
				break
			}
			klog.Infof("response: %+v", response)
			a.Recorder.Write(ctx, &journal.Event{
				Timestamp: time.Now(),
				Action:    "llm-response",
				Payload:   response,
			})

			if len(response.Candidates()) == 0 {
				log.Error(nil, "No candidates in response")
				return fmt.Errorf("no candidates in LLM response")
			}

			candidate := response.Candidates()[0]

			for _, part := range candidate.Parts() {
				// Check if it's a text response
				if text, ok := part.AsText(); ok {
					log.Info("text response", "text", text)
					a.Output <- ui.NewAgentTextBlock().WithText(text)
				}

				// Check if it's a function call
				if calls, ok := part.AsFunctionCalls(); ok && len(calls) > 0 {
					log.Info("function calls", "calls", calls)
					functionCalls = append(functionCalls, calls...)
					toolCallAnalysisResults, err := a.analyzeToolCalls(ctx, calls)
					if err != nil {
						return fmt.Errorf("error analyzing tool calls: %w", err)
					}
					a.pendingFunctionCalls = append(a.pendingFunctionCalls, toolCallAnalysisResults...)
				}
			}
		}

		if agentTextBlock != nil {
			agentTextBlock.SetStreaming(false)
		}

		// TODO(droot): Run all function calls in parallel
		// (may have to specify in the prompt to make these function calls independent)
		// NOTE: Currently, function calls are executed sequentially.
		// Suggestion: Use goroutines and sync.WaitGroup to parallelize execution if tool calls are independent.
		// Be careful with shared state and UI updates if running in parallel.

		for _, call := range functionCalls {
			toolCall, err := a.Tools.ParseToolInvocation(ctx, call.Name, call.Arguments)
			if err != nil {
				return fmt.Errorf("building tool call: %w", err)
			}

			// Check if the command is interactive using the tool's implementation
			isInteractive, err := toolCall.GetTool().IsInteractive(call.Arguments)
			klog.Infof("isInteractive: %t, err: %v, CallArguments: %+v", isInteractive, err, call.Arguments)

			// If interactive, handle based on whether we're using tool-use shim
			if isInteractive {
				// Show error block for both shim enabled and disabled modes
				errorBlock := ui.NewErrorBlock().SetText(fmt.Sprintf("  %s\n", err.Error()))
				a.doc.AddBlock(errorBlock)
				a.Output <- errorBlock

				if a.EnableToolUseShim {
					// Add the error as an observation
					observation := fmt.Sprintf("Result of running %q:\n%v", call.Name, err)
					currChatContent = append(currChatContent, observation)
				} else {
					// For models with tool-use support (shim disabled), use proper FunctionCallResult
					// Note: This assumes the model supports sending FunctionCallResult
					currChatContent = append(currChatContent, gollm.FunctionCallResult{
						ID:     call.ID,
						Name:   call.Name,
						Result: map[string]any{"error": err.Error()},
					})
				}
				continue // Skip execution for interactive commands
			}

			// Only show "Running" message and proceed with execution for non-interactive commands
			toolDescription := toolCall.Description()
			functionCallRequestBlock := ui.NewFunctionCallRequestBlock().SetDescription(toolDescription)
			a.doc.AddBlock(functionCallRequestBlock)
			a.Output <- functionCallRequestBlock

			// Ask for confirmation only if SkipPermissions is false AND the tool modifies resources.
			// Use the tool's CheckModifiesResource method to determine if the command modifies resources
			modifiesResourceStr := toolCall.GetTool().CheckModifiesResource(call.Arguments)

			// If our code detection returned "unknown", fall back to the LLM's assessment if available
			if modifiesResourceStr == "unknown" {
				if llmModifies, ok := call.Arguments["modifies_resource"].(string); ok {
					klog.Infof("Code detection returned 'unknown', falling back to LLM assessment: %s", llmModifies)
					modifiesResourceStr = llmModifies
				}
			}

			if !a.SkipPermissions && modifiesResourceStr != "no" {
				confirmationPrompt := `  Do you want to proceed ?`

				optionsBlock := ui.NewInputOptionBlock().SetPrompt(confirmationPrompt)
				optionsBlock.AddOption("yes", "Yes", "yes", "y")
				optionsBlock.AddOption("yes_and_dont_ask_me_again", "Yes, and don't ask me again")
				optionsBlock.AddOption("no", "No", "no", "n")
				a.doc.AddBlock(optionsBlock)

				selectedChoice, err := optionsBlock.Selection().Wait()
				if err != nil {
					if err == io.EOF {
						return nil
					}
					return fmt.Errorf("reading input: %w", err)
				}

				// Normalize the input
				switch selectedChoice {
				case "yes":
					// Proceed with the operation
				case "yes_and_dont_ask_me_again":
					a.SkipPermissions = true
				case "no":
					infoBlock := ui.NewAgentTextBlock().WithText("Operation was skipped. User declined to run this operation.")
					a.doc.AddBlock(infoBlock)
					a.Output <- infoBlock

					currChatContent = append(currChatContent, gollm.FunctionCallResult{
						ID:   call.ID,
						Name: call.Name,
						Result: map[string]any{
							"error":     "User declined to run this operation.",
							"status":    "declined",
							"retryable": false,
						},
					})
					continue
				default:
					// This case should technically not be reachable due to AskForConfirmation loop
					err := fmt.Errorf("invalid confirmation choice: %q", selectedChoice)
					log.Error(err, "Invalid choice received from AskForConfirmation")
					a.doc.AddBlock(ui.NewErrorBlock().SetText("Invalid choice received. Cancelling operation."))
					return err
				}
			}

			ctx := journal.ContextWithRecorder(ctx, a.Recorder)
			output, err := toolCall.InvokeTool(ctx, tools.InvokeToolOptions{
				Kubeconfig: a.Kubeconfig,
				WorkDir:    a.workDir,
			})
			if err != nil {
				log.Error(err, "error executing action", "output", output)
				return fmt.Errorf("executing action: %w", err)
			}

			// Handle timeout message using UI blocks
			if execResult, ok := output.(*tools.ExecResult); ok && execResult != nil && execResult.StreamType == "timeout" {
				infoBlock := ui.NewAgentTextBlock().WithText("\nTimeout reached after 7 seconds\n")
				a.doc.AddBlock(infoBlock)
				a.Output <- infoBlock
			}

			// Add the tool call result to maintain conversation flow
			if a.EnableToolUseShim {
				// If shim is enabled, format the result as a text observation
				observation := fmt.Sprintf("Result of running %q:\n%v", call.Name, output)
				currChatContent = append(currChatContent, observation)
			} else {
				functionCallRequestBlock.SetResult(output)

				// If shim is disabled, convert the result to a map and append FunctionCallResult
				result, err := tools.ToolResultToMap(output)
				if err != nil {
					log.Error(err, "error converting tool result to map", "output", output)
					return err
				}

				currChatContent = append(currChatContent, gollm.FunctionCallResult{
					ID:     call.ID,
					Name:   call.Name,
					Result: result,
				})
			}
		}

		// If no function calls were made, we're done
		if len(functionCalls) == 0 {
			log.Info("No function calls were made, so most likely the task is completed, so we're done.")
			return nil
		}

		currentIteration++
	}

	// If we've reached the maximum number of iterations
	log.Info("Max iterations reached", "iterations", maxIterations)
	errorBlock := ui.NewErrorBlock().SetText(fmt.Sprintf("Sorry, couldn't complete the task after %d iterations.\n", maxIterations))
	a.doc.AddBlock(errorBlock)
	a.Output <- errorBlock

	return fmt.Errorf("max iterations reached")
}

// generateFromTemplate generates a prompt for LLM. It uses the prompt from the provides template file or default.
func (a *Agent) generatePrompt(_ context.Context, defaultPromptTemplate string, data PromptData) (string, error) {
	promptTemplate := defaultPromptTemplate
	if a.PromptTemplateFile != "" {
		content, err := os.ReadFile(a.PromptTemplateFile)
		if err != nil {
			return "", fmt.Errorf("error reading template file: %v", err)
		}
		promptTemplate = string(content)
	}

	for _, extraPromptPath := range a.ExtraPromptPaths {
		content, err := os.ReadFile(extraPromptPath)
		if err != nil {
			return "", fmt.Errorf("error reading extra prompt path: %v", err)
		}
		promptTemplate += "\n" + string(content)
	}

	tmpl, err := template.New("promptTemplate").Parse(promptTemplate)
	if err != nil {
		return "", fmt.Errorf("building template for prompt: %w", err)
	}

	var result strings.Builder
	err = tmpl.Execute(&result, &data)
	if err != nil {
		return "", fmt.Errorf("evaluating template for prompt: %w", err)
	}
	return result.String(), nil
}

// PromptData represents the structure of the data to be filled into the template.
type PromptData struct {
	Query string
	Tools tools.Tools

	EnableToolUseShim bool
}

func (a *PromptData) ToolsAsJSON() string {
	var toolDefinitions []*gollm.FunctionDefinition

	for _, tool := range a.Tools.AllTools() {
		toolDefinitions = append(toolDefinitions, tool.FunctionDefinition())
	}

	json, err := json.MarshalIndent(toolDefinitions, "", "  ")
	if err != nil {
		return ""
	}
	return string(json)
}

func (a *PromptData) ToolNames() string {
	return strings.Join(a.Tools.Names(), ", ")
}

type ReActResponse struct {
	Thought string  `json:"thought"`
	Answer  string  `json:"answer,omitempty"`
	Action  *Action `json:"action,omitempty"`
}

type Action struct {
	Name             string `json:"name"`
	Reason           string `json:"reason"`
	Command          string `json:"command"`
	ModifiesResource string `json:"modifies_resource"`
}

func extractJSON(s string) (string, bool) {
	const jsonBlockMarker = "```json"

	first := strings.Index(s, jsonBlockMarker)
	last := strings.LastIndex(s, "```")
	if first == -1 || last == -1 || first == last {
		return "", false
	}
	data := s[first+len(jsonBlockMarker) : last]

	return data, true
}

// parseReActResponse parses the LLM response into a ReActResponse struct
// This function assumes the input contains exactly one JSON code block
// formatted with ```json and ``` markers. The JSON block is expected to
// contain a valid ReActResponse object.
func parseReActResponse(input string) (*ReActResponse, error) {
	cleaned, found := extractJSON(input)
	if !found {
		return nil, fmt.Errorf("no JSON code block found in %q", cleaned)
	}

	cleaned = strings.ReplaceAll(cleaned, "\n", "")
	cleaned = strings.TrimSpace(cleaned)

	var reActResp ReActResponse
	if err := json.Unmarshal([]byte(cleaned), &reActResp); err != nil {
		return nil, fmt.Errorf("parsing JSON %q: %w", cleaned, err)
	}
	return &reActResp, nil
}

// toMap converts the value to a map, going via JSON
func toMap(v any) (map[string]any, error) {
	j, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("converting %T to json: %w", v, err)
	}
	m := make(map[string]any)
	if err := json.Unmarshal(j, &m); err != nil {
		return nil, fmt.Errorf("converting json to map: %w", err)
	}
	return m, nil
}

func candidateToShimCandidate(iterator gollm.ChatResponseIterator) (gollm.ChatResponseIterator, error) {
	return func(yield func(gollm.ChatResponse, error) bool) {
		buffer := ""
		for response, err := range iterator {
			if err != nil {
				yield(nil, err)
				return
			}

			if len(response.Candidates()) == 0 {
				yield(nil, fmt.Errorf("no candidates in LLM response"))
				return
			}

			candidate := response.Candidates()[0]

			for _, part := range candidate.Parts() {
				if text, ok := part.AsText(); ok {
					buffer += text
					klog.Infof("text is %q", text)
				} else {
					yield(nil, fmt.Errorf("no text part found in candidate"))
					return
				}
			}
		}

		if buffer == "" {
			yield(nil, nil)
			return
		}

		parsedReActResp, err := parseReActResponse(buffer)
		if err != nil {
			yield(nil, fmt.Errorf("parsing ReAct response %q: %w", buffer, err))
			return
		}
		buffer = "" // TODO: any trailing text?
		yield(&ShimResponse{candidate: parsedReActResp}, nil)
	}, nil
}

type ShimResponse struct {
	candidate *ReActResponse
}

func (r *ShimResponse) UsageMetadata() any {
	return nil
}

func (r *ShimResponse) Candidates() []gollm.Candidate {
	return []gollm.Candidate{&ShimCandidate{candidate: r.candidate}}
}

type ShimCandidate struct {
	candidate *ReActResponse
}

func (c *ShimCandidate) String() string {
	return fmt.Sprintf("Thought: %s\nAnswer: %s\nAction: %s", c.candidate.Thought, c.candidate.Answer, c.candidate.Action)
}

func (c *ShimCandidate) Parts() []gollm.Part {
	var parts []gollm.Part
	if c.candidate.Thought != "" {
		parts = append(parts, &ShimPart{text: c.candidate.Thought})
	}
	if c.candidate.Answer != "" {
		parts = append(parts, &ShimPart{text: c.candidate.Answer})
	}
	if c.candidate.Action != nil {
		parts = append(parts, &ShimPart{action: c.candidate.Action})
	}
	return parts
}

type ShimPart struct {
	text   string
	action *Action
}

func (p *ShimPart) AsText() (string, bool) {
	return p.text, p.text != ""
}

func (p *ShimPart) AsFunctionCalls() ([]gollm.FunctionCall, bool) {
	if p.action != nil {
		functionCallArgs, err := toMap(p.action)
		if err != nil {
			return nil, false
		}
		delete(functionCallArgs, "name") // passed separately
		// delete(functionCallArgs, "reason")
		// delete(functionCallArgs, "modifies_resource")
		return []gollm.FunctionCall{
			{
				Name:      p.action.Name,
				Arguments: functionCallArgs,
			},
		}, true
	}
	return nil, false
}

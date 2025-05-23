// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package ui

// A collection of blocks that go into the history.

// AgentTextBlock is used to render agent textual responses
type AgentTextBlock struct {
	history *History

	// text is populated with the agent text output
	text string

	// Color is the foreground color of the text
	Color ColorValue

	// streaming is true if we are still streaming results in
	streaming bool
}

func NewAgentTextBlock() *AgentTextBlock {
	return &AgentTextBlock{}
}

func (b *AgentTextBlock) attached(history *History) {
	b.history = history
}

func (b *AgentTextBlock) History() *History {
	return b.history
}

func (b *AgentTextBlock) Text() string {
	return b.text
}

func (b *AgentTextBlock) Streaming() bool {
	return b.streaming
}

func (b *AgentTextBlock) SetStreaming(streaming bool) {
	b.streaming = streaming
	b.history.blockChanged(b)
}

func (b *AgentTextBlock) SetColor(color ColorValue) {
	b.Color = color
	b.history.blockChanged(b)
}

func (b *AgentTextBlock) SetText(agentText string) {
	b.text = agentText
	b.history.blockChanged(b)
}

func (b *AgentTextBlock) WithText(agentText string) *AgentTextBlock {
	b.SetText(agentText)
	return b
}

func (b *AgentTextBlock) AppendText(text string) {
	b.text = b.text + text
	b.history.blockChanged(b)
}

// FunctionCallRequestBlock is used to render the LLM's request to invoke a function
type FunctionCallRequestBlock struct {
	history *History

	// text is populated if this is agent text output
	text string
}

func NewFunctionCallRequestBlock() *FunctionCallRequestBlock {
	return &FunctionCallRequestBlock{}
}

func (b *FunctionCallRequestBlock) attached(history *History) {
	b.history = history
}

func (b *FunctionCallRequestBlock) History() *History {
	return b.history
}

func (b *FunctionCallRequestBlock) Text() string {
	return b.text
}

func (b *FunctionCallRequestBlock) SetText(agentText string) {
	b.text = agentText
	b.history.blockChanged(b)
}

func (b *FunctionCallRequestBlock) WithText(agentText string) *FunctionCallRequestBlock {
	b.SetText(agentText)
	return b
}

// ErrorBlock is used to render an error condition
type ErrorBlock struct {
	history *History

	// text is populated if this is agent text output
	text string
}

func NewErrorBlock() *ErrorBlock {
	return &ErrorBlock{}
}

func (b *ErrorBlock) attached(history *History) {
	b.history = history
}

func (b *ErrorBlock) History() *History {
	return b.history
}

func (b *ErrorBlock) Text() string {
	return b.text
}

func (b *ErrorBlock) SetText(agentText string) {
	b.text = agentText
	b.history.blockChanged(b)
}

func (b *ErrorBlock) WithText(agentText string) *ErrorBlock {
	b.SetText(agentText)
	return b
}

// InputTextBlock is used to prompt for user input
type InputTextBlock struct {
	history *History

	// text is populated when we have input from the user
	text Observable[string]
}

func NewInputTextBlock() *InputTextBlock {
	return &InputTextBlock{}
}

func (b *InputTextBlock) attached(history *History) {
	b.history = history
}

func (b *InputTextBlock) History() *History {
	return b.history
}

func (b *InputTextBlock) Observable() *Observable[string] {
	return &b.text
}

// InputOptionBlock is used to prompt for a selection from multiple choices
type InputOptionBlock struct {
	history *History

	// Options are the valid options that can be chosen
	Options []string

	// Prompt is the prompt to show the user
	Prompt string

	// text is populated when we have input from the user
	text Observable[string]
}

func NewInputOptionBlock() *InputOptionBlock {
	return &InputOptionBlock{}
}

func (b *InputOptionBlock) SetOptions(options []string) *InputOptionBlock {
	b.Options = options
	return b
}

// SetPrompt sets the prompt to show the user
func (b *InputOptionBlock) SetPrompt(prompt string) *InputOptionBlock {
	b.Prompt = prompt
	return b
}

func (b *InputOptionBlock) attached(history *History) {
	b.history = history
}

func (b *InputOptionBlock) History() *History {
	return b.history
}

func (b *InputOptionBlock) Observable() *Observable[string] {
	return &b.text
}

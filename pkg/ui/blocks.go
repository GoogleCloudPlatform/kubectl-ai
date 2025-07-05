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

package ui

import (
	"bytes"
	"context"
	"html/template"

	"k8s.io/klog/v2"
)

// AgentTextBlock is used to render agent textual responses
type AgentTextBlock struct {
	blockBase

	// text is populated with the agent text output
	text string

	// Color is the foreground color of the text
	Color ColorValue

	// streaming is true if we are still streaming results in
	streaming bool

	// html is populated with the rendered HTML
	// This is populated lazily, only when HTML() is called.
	html *template.HTML
}

func NewAgentTextBlock() *AgentTextBlock {
	return &AgentTextBlock{}
}

func (b *AgentTextBlock) Text() string {
	return b.text
}

func (b *AgentTextBlock) HTML() template.HTML {
	log := klog.FromContext(context.Background())

	cached := b.html
	if cached != nil {
		return *cached
	}
	text := b.text
	if b.doc != nil {
		htmlRenderer := b.doc.MarkdownHTMLRenderer()
		if htmlRenderer != nil {
			var buf bytes.Buffer
			if err := htmlRenderer.Convert([]byte(text), &buf); err != nil {
				log.Error(err, "Error rendering markdown to HTML")
			} else {
				v := template.HTML(buf.String())
				b.html = &v
				return v
			}
		}
	}

	return template.HTML(template.HTMLEscapeString(b.text))
}

func (b *AgentTextBlock) Streaming() bool {
	return b.streaming
}

func (b *AgentTextBlock) SetStreaming(streaming bool) {
	b.streaming = streaming
	b.doc.blockChanged(b)
}

func (b *AgentTextBlock) SetColor(color ColorValue) {
	b.Color = color
	b.doc.blockChanged(b)
}

func (b *AgentTextBlock) SetText(agentText string) {
	b.text = agentText
	b.doc.blockChanged(b)
}

func (b *AgentTextBlock) WithText(agentText string) *AgentTextBlock {
	b.SetText(agentText)
	return b
}

func (b *AgentTextBlock) AppendText(text string) {
	b.text = b.text + text
	b.html = nil
	b.doc.blockChanged(b)
}

// FunctionCallRequestBlock is used to render the LLM's request to invoke a function
type FunctionCallRequestBlock struct {
	blockBase

	// description describes the function call
	description string

	// result is populated after the function call has been executed
	result any
}

func NewFunctionCallRequestBlock() *FunctionCallRequestBlock {
	return &FunctionCallRequestBlock{}
}

func (b *FunctionCallRequestBlock) Description() string {
	return b.description
}

func (b *FunctionCallRequestBlock) Result() any {
	return b.result
}

func (b *FunctionCallRequestBlock) ResultHTML() template.HTML {
	if _, ok := b.result.(CanFormatAsHTML); ok {
		return b.result.(CanFormatAsHTML).FormatAsHTML()
	}
	htmlFragment := "Done"
	safeHTML := template.HTML(htmlFragment)
	return safeHTML
}

func (b *FunctionCallRequestBlock) SetDescription(description string) *FunctionCallRequestBlock {
	b.description = description
	b.doc.blockChanged(b)
	return b
}

func (b *FunctionCallRequestBlock) SetResult(result any) *FunctionCallRequestBlock {
	b.result = result
	b.doc.blockChanged(b)
	return b
}

// ErrorBlock is used to render an error condition
type ErrorBlock struct {
	blockBase

	// text is populated if this is agent text output
	text string
}

func NewErrorBlock() *ErrorBlock {
	return &ErrorBlock{}
}

func (b *ErrorBlock) Text() string {
	return b.text
}

func (b *ErrorBlock) SetText(agentText string) *ErrorBlock {
	b.text = agentText
	b.doc.blockChanged(b)
	return b
}

// InputTextBlock is used to prompt for user input
type InputTextBlock struct {
	blockBase

	// text is populated when we have input from the user
	text Observable[string]

	// editable is true if the input text block is editable
	editable bool
}

func NewInputTextBlock() *InputTextBlock {
	return &InputTextBlock{}
}

func (b *InputTextBlock) Observable() *Observable[string] {
	return &b.text
}

func (b *InputTextBlock) SetEditable(editable bool) *InputTextBlock {
	b.editable = editable
	b.doc.blockChanged(b)
	return b
}

func (b *InputTextBlock) Editable() bool {
	return b.editable
}

func (b *InputTextBlock) Text() (string, error) {
	return b.text.Get()
}

// InputOptionBlock is used to prompt for a selection from multiple choices
type InputOptionBlock struct {
	blockBase

	// Options are the valid options that can be chosen
	Options []InputOptionChoice

	// Prompt is the prompt to show the user
	Prompt string

	// selection is populated when we have input from the user
	selection Observable[string]

	// editable is true if the input option block is editable
	editable bool
}

type InputOptionChoice struct {
	// Key is the internal system identifier for the option
	Key string

	// Message is the text to show the user
	Message string

	// Aliases are alternative shortcuts for the option (other than the number),
	//typically used in terminal mode.
	Aliases []string
}

func NewInputOptionBlock() *InputOptionBlock {
	return &InputOptionBlock{}
}

// Editable returns true if the input option block is editable
func (b *InputOptionBlock) Editable() bool {
	return b.editable
}

func (b *InputOptionBlock) SetEditable(editable bool) *InputOptionBlock {
	b.editable = editable
	b.doc.blockChanged(b)
	return b
}

// AddOption adds an option to the input option block
func (b *InputOptionBlock) AddOption(key string, message string, aliases ...string) *InputOptionBlock {
	b.Options = append(b.Options, InputOptionChoice{
		Key:     key,
		Message: message,
		Aliases: aliases,
	})
	b.doc.blockChanged(b)
	return b
}

// SetPrompt sets the prompt to show the user
func (b *InputOptionBlock) SetPrompt(prompt string) *InputOptionBlock {
	b.Prompt = prompt
	b.doc.blockChanged(b)
	return b
}

func (b *InputOptionBlock) Selection() *Observable[string] {
	return &b.selection
}

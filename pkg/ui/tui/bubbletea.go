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

package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/journal"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/ui"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"k8s.io/klog/v2"
)

type BubbleTeaUserInterface struct {
	doc *ui.Document
	// journal journal.Recorder

	subscription     io.Closer
	markdownRenderer *glamour.TermRenderer

	// BubbleTea model
	model   *model
	program *tea.Program

	// Mutex for thread safety
	mu sync.Mutex
}

var _ ui.UI = &BubbleTeaUserInterface{}

type model struct {
	// blocks []ui.Block
	blockViews []blockView

	width  int
	height int
	ready  bool

	// Input handling
	// inputMode   bool
	// inputPrompt string
	// textInput textinput.Model

	// // Option selection
	// optionMode     bool
	// options        []ui.InputOptionChoice
	// optionPrompt   string
	// selectedOption int

	// // UI state
	// scrollOffset int
}

type blockView struct {
	block ui.Block

	textInput *textinput.Model
	options   *list.Model
}

// type msg interface{}

type (
	// // Window size change
	// windowSizeMsg struct {
	// 	width  int
	// 	height int
	// }

	// Document update
	documentUpdateMsg struct {
		blockViews []blockView
	}

	// // Input events
	// inputTextMsg struct {
	// 	text string
	// }

	// inputCursorMsg struct {
	// 	cursor int
	// }

	// // Option selection
	// optionSelectMsg struct {
	// 	index int
	// }

	// // Navigation
	// scrollMsg struct {
	// 	offset int
	// }

	// // Quit
	// quitMsg struct{}
)

func NewBubbleTeaUserInterface(doc *ui.Document, journal journal.Recorder) (*BubbleTeaUserInterface, error) {
	mdRenderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithPreservedNewLines(),
		glamour.WithEmoji(),
	)
	if err != nil {
		return nil, fmt.Errorf("error initializing the markdown renderer: %w", err)
	}

	u := &BubbleTeaUserInterface{
		doc: doc,
		// journal:          journal,
		markdownRenderer: mdRenderer,
	}

	// Create bubbletea model
	u.model = &model{}

	for _, block := range doc.Blocks() {
		bv := buildBlockView(block)
		u.model.blockViews = append(u.model.blockViews, bv)
	}

	// // Initialize text area
	// u.model.textArea.Placeholder = "Enter your input..."
	// u.model.textArea.Focus()

	// Subscribe to document changes
	subscription := doc.AddSubscription(u)
	u.subscription = subscription

	return u, nil
}

func buildBlockView(block ui.Block) blockView {
	bv := blockView{
		block: block,
	}

	switch block := block.(type) {
	case *ui.InputTextBlock:
		if block.Editable() {

			ti := textinput.New()
			// ti.Placeholder = "How can I help?"
			ti.Focus()
			// ti.CharLimit = 156
			ti.Width = 40

			bv.textInput = &ti
		}

	case *ui.InputOptionBlock:
		if block.Editable() {
			var items []list.Item
			for _, option := range block.Options {
				items = append(items, &optionItem{option: option})
			}
			delegate := list.NewDefaultDelegate()
			delegate.ShowDescription = false
			optionsModel := list.New(items, delegate, 40, 20)
			optionsModel.Title = block.Prompt
			bv.options = &optionsModel
		}
	}

	return bv
}

type optionItem struct {
	option ui.InputOptionChoice
}

var _ list.DefaultItem = &optionItem{}

func (o *optionItem) Title() string {
	return o.option.Message
}

func (o *optionItem) Description() string {
	return o.option.Message
}

func (o *optionItem) FilterValue() string {
	return o.option.Message
}

// type optionItemDelegate struct {
// 	// optionItem optionItem
// }

// func (u *optionItemDelegate) Height() int {
// 	return 1
// }

// func (u *optionItemDelegate) Spacing() int {
// 	return 0
// }

// func (u *optionItemDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
// 	return nil
// }

// func (u *optionItemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
// 	option := listItem.(optionItem).option

// 	style := lipgloss.NewStyle().
// 		Foreground(lipgloss.Color("#ffffff"))

// 	io.WriteString(w, style.Render(option.Message))
// }

func (u *BubbleTeaUserInterface) DocumentChanged(doc *ui.Document, block ui.Block) {
	u.mu.Lock()
	defer u.mu.Unlock()

	found := false

	blockID := block.ID()
	for i, blockView := range u.model.blockViews {
		if blockView.block.ID() == blockID {
			bv := buildBlockView(block)
			u.model.blockViews[i] = bv
			found = true
		}
	}

	if !found {
		bv := buildBlockView(block)
		u.model.blockViews = append(u.model.blockViews, bv)
	}

	// Send update message to bubbletea if program is running
	if u.program != nil {
		u.program.Send(documentUpdateMsg{blockViews: u.model.blockViews})
	}
}

func (u *BubbleTeaUserInterface) Start(ctx context.Context, cancel func()) error {
	// Create bubbletea program only when Run is called
	u.program = tea.NewProgram(
		u.model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
		tea.WithContext(ctx),
	)

	// Start the bubbletea program
	go func() {
		if _, err := u.program.Run(); err != nil {
			klog.Errorf("bubbletea program error: %v", err)
		}
		klog.Infof("bubbletea program exited")
		cancel()
	}()

	// // Wait for context cancellation
	// <-ctx.Done()

	// // Quit the program
	// if u.program != nil {
	// 	u.program.Quit()
	// }
	return nil
}

func (u *BubbleTeaUserInterface) Close() error {
	var errs []error

	if u.subscription != nil {
		if err := u.subscription.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if u.program != nil {
		u.program.Quit()
	}

	return errors.Join(errs...)
}

func (u *BubbleTeaUserInterface) ClearScreen() {
	// BubbleTea handles screen clearing automatically
}

// BubbleTea model implementation
func (m *model) Init() tea.Cmd {
	return tea.EnterAltScreen
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case documentUpdateMsg:
		return m.handleDocumentUpdate(msg)
		// case windowSizeMsg:
		// 	return m.handleWindowSize(tea.WindowSizeMsg{Width: msg.width, Height: msg.height})
	}

	if msg, ok := msg.(tea.WindowSizeMsg); ok {
		return m.handleWindowSize(msg)
	}

	if msg, ok := msg.(tea.KeyMsg); ok {
		var focused *blockView
		for i := range m.blockViews {
			blockView := &m.blockViews[i]
			if textInput := blockView.textInput; textInput != nil {
				if textInput.Focused() {
					focused = blockView
				}
			}
			if options := blockView.options; options != nil {
				focused = blockView
			}
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyRunes:
			switch string(msg.Runes) {
			case "q":
				return m, tea.Quit
			}
		case tea.KeyUp, tea.KeyDown:
			if focused != nil {
				if o := focused.options; o != nil {
					var cmd tea.Cmd
					newOptions, cmd := focused.options.Update(msg)
					focused.options = &newOptions
					return m, cmd
				}
			}

		case tea.KeyEnter:
			if focused != nil {
				if ti := focused.textInput; ti != nil {
					text := ti.Value()
					if text != "" {
						focused.block.(*ui.InputTextBlock).Observable().Set(text, nil)
						ti.Blur()
						//ti.SetValue("")
						return m, nil
					}
				}

				if opt := focused.options; opt != nil {
					selection := opt.SelectedItem().(*optionItem)

					if selection != nil {
						focused.block.(*ui.InputOptionBlock).Selection().Set(selection.option.Key, nil)
						return m, nil
					}
				}
			}
		}

		if focused != nil && focused.textInput != nil {
			var cmd tea.Cmd
			newTextInput, cmd := focused.textInput.Update(msg)
			focused.textInput = &newTextInput
			return m, cmd
		}
	}
	return m, nil
}

func (m *model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	var sb strings.Builder

	// m.inputMode = false
	// m.optionMode = false

	// Render blocks
	for i, blockView := range m.blockViews {
		if i > 0 {
			sb.WriteString("\n")
		}

		sb.WriteString(m.renderBlock(blockView))
	}

	// // Render input if in input mode
	// if m.inputMode {
	// 	sb.WriteString("\n\n")
	// 	sb.WriteString(m.renderInput())
	// }

	// // Render option selection if in option mode
	// if m.optionMode {
	// 	sb.WriteString("\n\n")
	// 	sb.WriteString(m.renderOptions())
	// }

	// Add help text
	sb.WriteString("\n\n")
	sb.WriteString(m.renderHelp())

	return sb.String()
}

func (m *model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	m.ready = true
	return m, nil
}

func (m *model) handleDocumentUpdate(msg documentUpdateMsg) (tea.Model, tea.Cmd) {
	m.blockViews = msg.blockViews
	return m, nil
}

func (m *model) renderBlock(blockView blockView) string {
	switch block := blockView.block.(type) {
	case *ui.ErrorBlock:
		return m.renderErrorBlock(block)
	case *ui.FunctionCallRequestBlock:
		return m.renderFunctionCallBlock(block)
	case *ui.AgentTextBlock:
		return m.renderAgentTextBlock(block)
	case *ui.InputTextBlock:
		if blockView.textInput != nil {
			return blockView.textInput.View()
		}
		return m.renderInputTextBlock(block)
	case *ui.InputOptionBlock:
		if blockView.options != nil {
			return blockView.options.View()
		}
		return m.renderInputOptionBlock(block)
	default:
		return fmt.Sprintf("Unknown block type: %T", block)
	}
}

func (m *model) renderErrorBlock(block *ui.ErrorBlock) string {
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#ff0000")).
		Bold(true)

	return style.Render("❌ Error: " + block.Text())
}

func (m *model) renderFunctionCallBlock(block *ui.FunctionCallRequestBlock) string {
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#00ff00")).
		Bold(true)

	return style.Render("🔄 " + block.Description())
}

func (m *model) renderAgentTextBlock(block *ui.AgentTextBlock) string {
	text := block.Text()

	// Try to render as markdown if available
	if block.Document() != nil && block.Document().MarkdownTerminalRenderer() != nil {
		rendered, err := block.Document().MarkdownTerminalRenderer().Render(text)
		if err == nil {
			return rendered
		}
	}

	// Fallback to plain text
	style := lipgloss.NewStyle()
	if block.Color != "" {
		switch block.Color {
		case ui.ColorRed:
			style = style.Foreground(lipgloss.Color("#ff0000"))
		case ui.ColorGreen:
			style = style.Foreground(lipgloss.Color("#00ff00"))
		case ui.ColorWhite:
			style = style.Foreground(lipgloss.Color("#ffffff"))
		}
	}

	return style.Render(text)
}

func (m *model) renderInputTextBlock(block *ui.InputTextBlock) string {

	text, err := block.Text()
	if err != nil {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ff0000")).
			Render("Error reading input: " + err.Error())
	}

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("#00ffff")).
		Render("User: " + text)
}

func (m *model) renderInputOptionBlock(block *ui.InputOptionBlock) string {
	selection, err := block.Selection().Get()
	if err != nil {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ff0000")).
			Render("Error reading selection: " + err.Error())
	}

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("#00ffff")).
		Render("Selected: " + selection)
}

// func (m *model) renderInput() string {
// 	var sb strings.Builder

// 	// Render prompt
// 	sb.WriteString(lipgloss.NewStyle().
// 		Foreground(lipgloss.Color("#ffff00")).
// 		Render(m.inputPrompt))
// 	sb.WriteString("\n")

// 	// Render the appropriate input component
// 	if m.textInput.Focused() {
// 		sb.WriteString(m.textInput.View())
// 	} else if m.textArea.Focused() {
// 		sb.WriteString(m.textArea.View())
// 	}

// 	return sb.String()
// }

// func (m *model) renderOptions() string {
// 	var sb strings.Builder
// 	sb.WriteString(lipgloss.NewStyle().
// 		Foreground(lipgloss.Color("#ffff00")).
// 		Bold(true).
// 		Render(m.optionPrompt))
// 	sb.WriteString("\n")

// 	for i, option := range m.options {
// 		style := lipgloss.NewStyle()
// 		if i == m.selectedOption {
// 			style = style.Foreground(lipgloss.Color("#00ff00")).Bold(true)
// 		} else {
// 			style = style.Foreground(lipgloss.Color("#ffffff"))
// 		}
// 		sb.WriteString(fmt.Sprintf("  %d) %s\n", i+1, style.Render(option.Message)))
// 	}

// 	return sb.String()
// }

func (m *model) renderHelp() string {
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")).
		Italic(true)

	return helpStyle.Render("Press 'q' to quit, ↑/↓ to navigate options")
}

// func (m *model) setInputValue(text string) {
// 	// Find the current editable input block and set its value
// 	for _, block := range m.blocks {
// 		if inputBlock, ok := block.(*ui.InputTextBlock); ok {
// 			if inputBlock.Editable() {
// 				inputBlock.Observable().Set(text, nil)
// 				inputBlock.SetEditable(false)
// 				break
// 			}
// 		}
// 	}
// }

// func (m *model) setOptionValue(optionKey string) {
// 	// Find the current editable option block and set its value
// 	for _, block := range m.blocks {
// 		if optionBlock, ok := block.(*ui.InputOptionBlock); ok {
// 			if optionBlock.Editable() {
// 				optionBlock.Selection().Set(optionKey, nil)
// 				break
// 			}
// 		}
// 	}
// }

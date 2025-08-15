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
	"context"
	"fmt"
	"io"
	"os"
	"os/user"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/agent"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"k8s.io/klog/v2"
)

const listHeight = 5

var (
	spinnerStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
	helpStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Margin(1, 0)
	dotStyle          = helpStyle.UnsetMargins()
	durationStyle     = dotStyle
	appStyle          = lipgloss.NewStyle().Margin(1, 2, 0, 2)
	titleStyle        = lipgloss.NewStyle().MarginLeft(2)
	listStyle         = lipgloss.NewStyle().MarginBottom(2)
	itemStyle         = lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("170"))
	paginationStyle   = list.DefaultStyles().PaginationStyle.PaddingLeft(4)
	quitTextStyle     = lipgloss.NewStyle().Margin(1, 0, 2, 4)
)

type item string

func (i item) FilterValue() string { return "" }

type itemDelegate struct{}

func (d itemDelegate) Height() int                             { return 1 }
func (d itemDelegate) Spacing() int                            { return 0 }
func (d itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(item)
	if !ok {
		return
	}

	str := fmt.Sprintf("%d. %s", index+1, i)

	fn := itemStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return selectedItemStyle.Render("> " + strings.Join(s, " "))
		}
	}

	fmt.Fprint(w, fn(str))
}

const gap = "\n\n"

// getCurrentUsername returns the current user's username, caching it to avoid repeated calls
func getCurrentUsername() string {
	currentUser, err := user.Current()
	if err != nil {
		// Fallback to environment variable or default
		if username := os.Getenv("USER"); username != "" {
			return username
		}
		return "You"
	}
	return currentUser.Username
}

// TUI is a rich terminal user interface for the agent.
type TUI struct {
	program *tea.Program
	agent   *agent.Agent
}

func NewTUI(agent *agent.Agent) *TUI {
	return &TUI{
		program: tea.NewProgram(newModel(agent), tea.WithAltScreen()),
		agent:   agent,
	}
}

func (u *TUI) Run(ctx context.Context) error {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-u.agent.Output:
				if !ok {
					return
				}
				u.program.Send(msg)
			}
		}
	}()

	_, err := u.program.Run()
	return err
}

func (u *TUI) ClearScreen() {
}

type resultMsg struct {
	duration time.Duration
	food     string
}

func (r resultMsg) String() string {
	if r.duration == 0 {
		return dotStyle.Render(strings.Repeat(".", 30))
	}
	return fmt.Sprintf("🍔 Ate %s %s", r.food,
		durationStyle.Render(r.duration.String()))
}

type (
	errMsg error
)

type model struct {
	viewport    viewport.Model
	textarea    textarea.Model
	senderStyle lipgloss.Style
	err         error

	agent    *agent.Agent
	spinner  spinner.Model
	results  []resultMsg
	messages []*api.Message
	quitting bool

	list     list.Model
	choice   string
	username string // cached username
}

func newModel(agent *agent.Agent) model {
	ta := textarea.New()
	ta.Placeholder = "Send a message..."
	ta.Focus()

	ta.Prompt = "┃ "
	ta.CharLimit = 280

	ta.SetWidth(30)
	ta.SetHeight(5)

	// Remove cursor line styling
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()

	ta.ShowLineNumbers = false

	items := []list.Item{
		item("Yes"),
		item("Yes, and don't ask me again"),
		item("No"),
	}

	const defaultWidth = 30

	l := list.New(items, itemDelegate{}, defaultWidth, listHeight)
	l.Title = "Do you want to proceed ?"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	l.Styles.Title = titleStyle

	vp := viewport.New(30, 5)
	vp.SetContent(`Welcome to the chat room!
Type a message and press Enter to send.`)

	ta.KeyMap.InsertNewline.SetEnabled(false)

	return model{
		agent:    agent,
		textarea: ta,
		viewport: vp,
		list:     l,
		// a lipgloss style for the sender
		senderStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("5")),
		username:    getCurrentUsername(),
		err:         nil,
	}
}

func (m model) Init() tea.Cmd {
	return textarea.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {

	var (
		tiCmd   tea.Cmd
		vpCmd   tea.Cmd
		listCmd tea.Cmd
	)

	m.textarea, tiCmd = m.textarea.Update(msg)
	m.viewport, vpCmd = m.viewport.Update(msg)
	m.list, listCmd = m.list.Update(msg)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.viewport.Width = msg.Width
		m.textarea.SetWidth(msg.Width)
		if m.agent.GetSession().AgentState == api.AgentStateWaitingForInput {
			m.list.SetWidth(msg.Width)
			// m.viewport.Height = msg.Height - m.list.Height() - lipgloss.Height(gap)
			// TODO: keeping the height of the viewport the same as the height of the textarea for now to avoid jerky UI
			m.viewport.Height = msg.Height - m.textarea.Height() - lipgloss.Height(gap)
		} else {
			m.viewport.Height = msg.Height - m.textarea.Height() - lipgloss.Height(gap)
		}
		if len(m.renderedMessages()) > 0 {
			// Wrap content before setting it.
			m.viewport.SetContent(lipgloss.NewStyle().Width(m.viewport.Width).Render(strings.Join(m.renderedMessages(), "\n")))
		}
		m.viewport.GotoBottom()
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc, tea.KeyCtrlD:
			return m, tea.Quit
		case tea.KeyEnter:
			if m.agent.GetSession().AgentState == api.AgentStateWaitingForInput {
				i, ok := m.list.SelectedItem().(item)
				if ok {
					m.choice = string(i)
					choiceIndex := m.list.Index()
					m.agent.Input <- &api.UserChoiceResponse{Choice: choiceIndex + 1}
				}
				return m, nil
			}

			m.messages = append(m.messages, &api.Message{
				Source:  api.MessageSourceUser,
				Type:    api.MessageTypeText,
				Payload: m.textarea.Value(),
			})
			m.viewport.SetContent(strings.Join(m.renderedMessages(), "\n"))
			m.agent.Input <- &api.UserInputResponse{Query: m.textarea.Value()}
			m.textarea.Reset()
			m.viewport.GotoBottom()
		}
	case *api.Message:
		m.messages = m.agent.GetSession().AllMessages()
		m.viewport.SetContent(strings.Join(m.renderedMessages(), "\n"))
		m.viewport.GotoBottom()

	// We handle errors just like any other message
	case errMsg:
		m.err = msg
		return m, nil
	}

	return m, tea.Batch(tiCmd, vpCmd, listCmd)

}

func (m model) renderedMessages() []string {
	allMessages := m.agent.GetSession().AllMessages()

	var messages []string
	for _, message := range allMessages {
		if message.Type == api.MessageTypeUserInputRequest && message.Payload == ">>>" {
			continue
		}
		messages = append(messages, m.renderMessage(message))
	}
	return messages
}

func (m model) View() string {
	if m.quitting {
		return quitTextStyle.Render("Not safe to quit yet.")
	}
	mainView := fmt.Sprintf(
		"%s%s",
		m.viewport.View(),
		gap,
	)
	if m.agent.GetSession().AgentState == api.AgentStateWaitingForInput {
		var choiceRequest *api.UserChoiceRequest
		if len(m.messages) > 0 {
			if lastMsg := m.messages[len(m.messages)-1]; lastMsg.Type == api.MessageTypeUserChoiceRequest {
				choiceRequest = lastMsg.Payload.(*api.UserChoiceRequest)
			}
		}

		if choiceRequest != nil {
			items := make([]list.Item, len(choiceRequest.Options))
			for i, option := range choiceRequest.Options {
				items[i] = item(option.Label)
			}
			m.list.SetItems(items)
			m.list.Title = "Select an option:"
			mainView += listStyle.Render(m.list.View())
		} else {
			mainView += m.textarea.View()
		}
	} else {
		mainView += m.textarea.View()
	}
	return mainView
}

func (m model) renderMessage(message *api.Message) string {
	sourceDisplayName := ""
	switch message.Source {
	case api.MessageSourceUser:
		sourceDisplayName = m.username
	case api.MessageSourceModel, api.MessageSourceAgent:
		sourceDisplayName = "AI"
	}
	text := m.senderStyle.Render(fmt.Sprintf("%s: ", sourceDisplayName))
	glamourRenderWidth := m.viewport.Width - m.viewport.Style.GetHorizontalFrameSize() - lipgloss.Width(text)

	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(glamourRenderWidth),
	)
	if err != nil {
		klog.Errorf("failed to create glamour renderer: %v", err)
		return fmt.Sprintf("error rendering message: %v", err)
	}

	var renderedText string
	var contentToRender string

	switch p := message.Payload.(type) {
	case string:
		contentToRender = p
	case *api.UserChoiceRequest:
		contentToRender = p.Prompt
	default:
		return "" // Don't render unknown payload types
	}

	switch message.Type {
	case api.MessageTypeToolCallRequest:
		contentToRender = fmt.Sprintf("Running: `%s`", contentToRender)
	case api.MessageTypeError:
		contentToRender = fmt.Sprintf("Error: %s", contentToRender)
	case api.MessageTypeToolCallResponse:
		return "" // Or a summary
	}

	renderedText, err = renderer.Render(contentToRender)
	if err != nil {
		klog.Errorf("failed to render markdown: %v", err)
		return text + contentToRender // Fallback to non-rendered
	}

	return text + renderedText
}

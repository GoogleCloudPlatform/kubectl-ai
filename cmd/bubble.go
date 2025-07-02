package main

// A simple example that shows how to send messages to a Bubble Tea program
// from outside the program using Program.Send(Msg).

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/agent"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"k8s.io/klog/v2"
)

var (
	spinnerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Margin(1, 0)
	dotStyle      = helpStyle.UnsetMargins()
	durationStyle = dotStyle
	appStyle      = lipgloss.NewStyle().Margin(1, 2, 0, 2)
)

const gap = "\n\n"

type BubbleUI struct {
	program *tea.Program
	agent   *agent.Agent
}

func NewBubbleUI(agent *agent.Agent) *BubbleUI {
	return &BubbleUI{
		program: tea.NewProgram(newModel(agent)),
		agent:   agent,
	}
}

func (u *BubbleUI) Run(ctx context.Context) error {
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

func (u *BubbleUI) ClearScreen() {
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
}

func newModel(agent *agent.Agent) model {
	const numLastResults = 5
	s := spinner.New()
	s.Style = spinnerStyle

	ta := textarea.New()
	ta.Placeholder = "Send a message..."
	ta.Focus()

	ta.Prompt = "┃ "
	ta.CharLimit = 280

	ta.SetWidth(30)
	ta.SetHeight(3)

	// Remove cursor line styling
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()

	ta.ShowLineNumbers = false

	vp := viewport.New(30, 5)
	vp.SetContent(`Welcome to the chat room!
Type a message and press Enter to send.`)

	ta.KeyMap.InsertNewline.SetEnabled(false)

	return model{
		agent:       agent,
		spinner:     s,
		textarea:    ta,
		viewport:    vp,
		senderStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("5")),
		results:     make([]resultMsg, numLastResults),
		err:         nil,
	}
}

func (m model) Init() tea.Cmd {
	return textarea.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {

	var (
		tiCmd tea.Cmd
		vpCmd tea.Cmd
	)

	m.textarea, tiCmd = m.textarea.Update(msg)
	m.viewport, vpCmd = m.viewport.Update(msg)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.viewport.Width = msg.Width
		m.textarea.SetWidth(msg.Width)
		m.viewport.Height = msg.Height - m.textarea.Height() - lipgloss.Height(gap)

		if len(m.renderedMessages()) > 0 {
			// Wrap content before setting it.
			m.viewport.SetContent(lipgloss.NewStyle().Width(m.viewport.Width).Render(strings.Join(m.renderedMessages(), "\n")))
		}
		m.viewport.GotoBottom()
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			fmt.Println(m.textarea.Value())
			return m, tea.Quit
		case tea.KeyEnter:
			m.agent.Input <- m.textarea.Value()

			// m.messages = append(m.messages, m.senderStyle.Render("You: ")+m.textarea.Value())
			// m.viewport.SetContent(lipgloss.NewStyle().Width(m.viewport.Width).Render(strings.Join(m.messages, "\n")))
			m.textarea.Reset()
			m.viewport.GotoBottom()
		}
	case *api.Message:
		klog.Infof("Received message: %v", msg)
		m.messages = m.agent.Session().AllMessages()
		// m.messages = append(m.messages, m.senderStyle.Render("You: ")+m.textarea.Value())
		m.viewport.SetContent(lipgloss.NewStyle().Width(m.viewport.Width).Render(strings.Join(m.renderedMessages(), "\n")))
		m.viewport.GotoBottom()
		return m, nil
	// We handle errors just like any other message
	case errMsg:
		m.err = msg
		return m, nil
	}

	return m, tea.Batch(tiCmd, vpCmd)

}

func (m model) renderedMessages() []string {
	allMessages := m.agent.Session().AllMessages()
	messages := make([]string, len(allMessages))
	for i, message := range allMessages {
		messages[i] = renderMessage(message)
	}
	return messages
}

func (m model) View() string {
	return fmt.Sprintf(
		"%s%s%s",
		m.viewport.View(),
		gap,
		m.textarea.View(),
	)
}

func renderMessage(message *api.Message) string {
	switch message.Type {
	case api.MessageTypeText:
		return message.Payload.(string)
	case api.MessageTypeError:
		return fmt.Sprintf("  Error: %s\n", message.Payload.(string))
	case api.MessageTypeToolCallRequest:
		return fmt.Sprintf("  Running: %s\n", message.Payload.(string))
	case api.MessageTypeToolCallResponse:
		return fmt.Sprintf("  Output : %s\n", "(...)")
	}
	return ""
}

// func (m model) View() string {
// 	var s string

// 	if m.quitting {
// 		s += "That’s all for today!"
// 	} else {
// 		s += m.spinner.View() + " Eating food..."
// 	}

// 	s += "\n\n"

// 	for _, message := range m.messages {
// 		s += renderMessage(message) + "\n"
// 	}

// 	if !m.quitting {
// 		s += helpStyle.Render("Press any key to exit")
// 	}

// 	if m.quitting {
// 		s += "\n"
// 	}

// 	return appStyle.Render(s)
// }

// func main() {
// 	p := tea.NewProgram(newModel())

// 	// Simulate activity
// 	go func() {
// 		for {
// 			pause := time.Duration(rand.Int63n(899)+100) * time.Millisecond // nolint:gosec
// 			time.Sleep(pause)

// 			// Send the Bubble Tea program a message from outside the
// 			// tea.Program. This will block until it is ready to receive
// 			// messages.
// 			p.Send(resultMsg{food: randomFood(), duration: pause})
// 		}
// 	}()

// 	if _, err := p.Run(); err != nil {
// 		fmt.Println("Error running program:", err)
// 		os.Exit(1)
// 	}
// }

// func randomFood() string {
// 	food := []string{
// 		"an apple", "a pear", "a gherkin", "a party gherkin",
// 		"a kohlrabi", "some spaghetti", "tacos", "a currywurst", "some curry",
// 		"a sandwich", "some peanut butter", "some cashews", "some ramen",
// 	}
// 	return food[rand.Intn(len(food))] // nolint:gosec
// }

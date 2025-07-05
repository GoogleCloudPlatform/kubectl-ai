package main

// A simple example that shows how to send messages to a Bubble Tea program
// from outside the program using Program.Send(Msg).

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
	// helpStyle         = list.DefaultStyles().HelpStyle.PaddingLeft(4).PaddingBottom(1)
	quitTextStyle = lipgloss.NewStyle().Margin(1, 0, 2, 4)
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

type BubbleUI struct {
	program *tea.Program
	agent   *agent.Agent
}

func NewBubbleUI(agent *agent.Agent) *BubbleUI {
	return &BubbleUI{
		program: tea.NewProgram(newModel(agent), tea.WithAltScreen()),
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

	list     list.Model
	choice   string
	username string // cached username
}

func newModel(agent *agent.Agent) model {
	// const numLastResults = 5
	// s := spinner.New()
	// s.Style = spinnerStyle

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
	// = listStyle
	// l.Styles.PaginationStyle = paginationStyle
	// l.Styles.HelpStyle = helpStyle

	vp := viewport.New(30, 5)
	vp.SetContent(`Welcome to the chat room!
Type a message and press Enter to send.`)

	ta.KeyMap.InsertNewline.SetEnabled(false)

	return model{
		agent: agent,
		// spinner:     s,
		textarea:    ta,
		viewport:    vp,
		list:        l,
		senderStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("5")),
		// results:     make([]resultMsg, numLastResults),
		username: getCurrentUsername(),
		err:      nil,
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
		if m.agent.Session().AgentState == api.AgentStateWaitingForInput {
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
		case tea.KeyCtrlC, tea.KeyEsc:
			fmt.Println(m.textarea.Value())
			return m, tea.Quit
		case tea.KeyEnter:
			if m.agent.Session().AgentState == api.AgentStateWaitingForInput {
				i := m.list.Index()
				if i != -1 {
					m.agent.Input <- int32(i + 1)
				}
				return m, nil
			} else {
				m.agent.Input <- m.textarea.Value()
				m.textarea.Reset()
			}
			// m.messages = append(m.messages, m.senderStyle.Render("You: ")+m.textarea.Value())
			// m.viewport.SetContent(lipgloss.NewStyle().Width(m.viewport.Width).Render(strings.Join(m.messages, "\n")))
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

	return m, tea.Batch(tiCmd, vpCmd, listCmd)

}

func (m model) renderedMessages() []string {
	allMessages := m.agent.Session().AllMessages()
	messages := make([]string, len(allMessages))
	for i, message := range allMessages {
		messages[i] = m.renderMessage(message)
	}
	return messages
}

func (m model) View() string {
	mainView := fmt.Sprintf(
		"%s%s",
		m.viewport.View(),
		gap,
	)
	if m.agent.Session().AgentState == api.AgentStateWaitingForInput {
		mainView += listStyle.Render(m.list.View())
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
	case api.MessageSourceModel:
		sourceDisplayName = "AI"
	default:
		sourceDisplayName = "AI"
	}
	text := m.senderStyle.Render(fmt.Sprintf("%s: ", sourceDisplayName))
	const glamourGutter = 2
	glamourRenderWidth := m.viewport.Width - m.viewport.Style.GetHorizontalFrameSize() - glamourGutter

	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(glamourRenderWidth),
	)
	if err != nil {
		return fmt.Sprintf("Error rendering message: %v", err)
	}

	var renderedText string
	switch message.Type {
	case api.MessageTypeText, api.MessageTypeUserInputRequest:
		renderedText, err = renderer.Render(message.Payload.(string))
		if err != nil {
			return fmt.Sprintf("Error rendering message: %v", err)
		}
	case api.MessageTypeError:
		renderedText, err = renderer.Render(fmt.Sprintf("  Error: %s\n", message.Payload.(string)))
		if err != nil {
			return fmt.Sprintf("Error rendering message: %v", err)
		}
	case api.MessageTypeToolCallRequest:
		renderedText, err = renderer.Render(fmt.Sprintf("  Running: `%s`\n", message.Payload.(string)))
		// renderedText = toolCallStyle.Render(renderedText)
		if err != nil {
			return fmt.Sprintf("Error rendering message: %v", err)
		}
	case api.MessageTypeToolCallResponse:
		renderedText, err = renderer.Render(fmt.Sprintf("  Output : %s\n", "(...)"))
		if err != nil {
			return fmt.Sprintf("Error rendering message: %v", err)
		}
		// TODO: figure out a way to render output of the tool call.
		// It can get noisy if we always output the entire output of the tool call.
		// by default, show the preview of the output and have a command line flag to show the entire output.
		return ""
	case api.MessageTypeUserChoiceRequest:
		renderedText, err = renderer.Render(message.Payload.(*api.UserChoiceRequest).Prompt)
		if err != nil {
			return fmt.Sprintf("Error rendering message: %v", err)
		}
	}
	return text + renderedText
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

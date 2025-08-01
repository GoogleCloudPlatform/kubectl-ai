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
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/agent"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/journal"
	"github.com/charmbracelet/glamour"
	"github.com/chzyer/readline"
	"golang.org/x/term"
	"k8s.io/klog/v2"
)

type computedStyle struct {
	Foreground     colorValue
	RenderMarkdown bool
}

type colorValue string

const (
	colorGreen colorValue = "green"
	colorWhite colorValue = "white"
	colorRed   colorValue = "red"
)

type styleOption func(s *computedStyle)

func foreground(color colorValue) styleOption {
	return func(s *computedStyle) {
		s.Foreground = color
	}
}

func renderMarkdown() styleOption {
	return func(s *computedStyle) {
		s.RenderMarkdown = true
	}
}

// TODO: rename this to CLI because the command line interface.
type TerminalUI struct {
	journal          journal.Recorder
	markdownRenderer *glamour.TermRenderer

	// Input handling fields (initialized once)
	rlInstance        *readline.Instance // For readline input
	ttyFile           *os.File           // For TTY input
	ttyReaderInstance *bufio.Reader      // For TTY input

	// This is useful in cases where stdin is already been used for providing the input to the agent (caller in this case)
	// in such cases, stdin is already consumed and closed and reading input results in IO error.
	// In such cases, we open /dev/tty and use it for taking input.
	useTTYForInput bool

	agent *agent.Agent
}

var _ UI = &TerminalUI{}

func getCustomTerminalWidth() int {
	// Check for user-configured width via environment variable
	if widthStr := os.Getenv("KUBECTL_AI_TERM_WIDTH"); widthStr != "" {

		if widthStr == "auto" {
			width, _, err := term.GetSize(int(os.Stdout.Fd()))

			if err != nil {
				klog.Warningf("Failed to get terminal size: %v, using default width", err)
				return 0
			}

			return width
		}

		if width, err := strconv.Atoi(widthStr); err == nil && width > 0 {
			return width
		}
		klog.Warningf("Invalid KUBECTL_AI_TERM_WIDTH value %q, using default", widthStr)
	}

	// Return 0 to indicate no custom width should be set (use glamour's default)
	return 0
}

func NewTerminalUI(agent *agent.Agent, useTTYForInput bool, journal journal.Recorder) (*TerminalUI, error) {
	width := getCustomTerminalWidth()

	options := []glamour.TermRendererOption{
		glamour.WithAutoStyle(),
		glamour.WithPreservedNewLines(),
		glamour.WithEmoji(),
	}

	// Only add WordWrap if a valid width is configured
	if width > 0 {
		options = append(options, glamour.WithWordWrap(width))
	}

	mdRenderer, err := glamour.NewTermRenderer(options...)
	if err != nil {
		return nil, fmt.Errorf("error initializing the markdown renderer: %w", err)
	}

	u := &TerminalUI{
		markdownRenderer: mdRenderer,
		journal:          journal,
		useTTYForInput:   useTTYForInput, // Store this flag
		agent:            agent,
	}

	return u, nil
}

func (u *TerminalUI) Run(ctx context.Context) error {
	// Channel to signal when the agent has exited
	agentExited := make(chan struct{})

	// Start a goroutine to handle agent output
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-u.agent.Output:
				if !ok {
					return
				}
				klog.Infof("agent output: %+v", msg)
				u.handleMessage(msg.(*api.Message))

				// Check if agent has exited in RunOnce mode
				if u.agent.Session().AgentState == api.AgentStateExited {
					klog.Info("Agent has exited, terminating UI")
					close(agentExited)
					return
				}
			}
		}
	}()

	// Block until context is cancelled or agent exits
	select {
	case <-ctx.Done():
		return nil
	case <-agentExited:
		return nil
	}
}

func (u *TerminalUI) ttyReader() (*bufio.Reader, error) {
	if u.ttyReaderInstance != nil {
		return u.ttyReaderInstance, nil
	}
	// Initialize TTY input
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("opening tty for input: %w", err)
	}
	u.ttyFile = tty // Store file handle for closing
	u.ttyReaderInstance = bufio.NewReader(tty)
	return u.ttyReaderInstance, nil
}

func (u *TerminalUI) readlineInstance() (*readline.Instance, error) {
	if u.rlInstance != nil {
		return u.rlInstance, nil
	}
	// Initialize readline input
	historyPath := filepath.Join(os.TempDir(), "kubectl-ai-history")
	rl, err := readline.NewEx(&readline.Config{
		Prompt:      ">>> ", // Default prompt for main input
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		HistoryFile: historyPath,
		// History enabled by default
	})
	if err != nil {
		// Log warning or fallback if readline init fails?
		klog.Warningf("Failed to initialize readline, input might be limited: %v", err)
		// Proceed without readline for now, or return error?
		// Returning error to make it explicit
		return nil, fmt.Errorf("creating readline instance: %w", err)
	}
	u.rlInstance = rl // Store readline instance
	return u.rlInstance, nil
}

func (u *TerminalUI) Close() error {
	var errs []error

	// Close the initialized input handler
	if u.rlInstance != nil {
		if err := u.rlInstance.Close(); err != nil {
			errs = append(errs, fmt.Errorf("closing readline instance: %w", err))
		}
	}
	if u.ttyFile != nil {
		if err := u.ttyFile.Close(); err != nil {
			errs = append(errs, fmt.Errorf("closing tty file: %w", err))
		}
	}
	return errors.Join(errs...)
}

func (u *TerminalUI) handleMessage(msg *api.Message) {
	text := ""
	var styleOptions []styleOption

	switch msg.Type {
	case api.MessageTypeText:
		text = msg.Payload.(string)
		switch msg.Source {
		case api.MessageSourceUser:
			// styleOptions = append(styleOptions, Foreground(ColorWhite))
			// since we print the message as user types, we don't need to print it again
			return
		case api.MessageSourceAgent:
			styleOptions = append(styleOptions, renderMarkdown(), foreground(colorGreen))
		case api.MessageSourceModel:
			styleOptions = append(styleOptions, renderMarkdown())
		}
	case api.MessageTypeError:
		styleOptions = append(styleOptions, foreground(colorRed))
		text = msg.Payload.(string)
	case api.MessageTypeToolCallRequest:
		styleOptions = append(styleOptions, foreground(colorGreen))
		text = fmt.Sprintf("  Running: %s\n", msg.Payload.(string))
	case api.MessageTypeToolCallResponse:
		styleOptions = append(styleOptions, foreground(colorGreen))
		output := formatToolCallResponse(msg.Payload)
		if output == "" {
			return
		}
		text = fmt.Sprintf("%s\n", output)
	case api.MessageTypeUserInputRequest:
		text = msg.Payload.(string)
		klog.Infof("Received user input request with payload: %q", text)

		// If this is the greeting message, just display it and don't prompt for input
		if text == "Hey there, what can I help you with today ?" {
			fmt.Printf("\n %s\n\n", text)
			return
		}

		var query string
		if u.useTTYForInput {
			tReader, err := u.ttyReader()
			if err != nil {
				klog.Errorf("Failed to get TTY reader: %v", err)
				return
			}
			fmt.Print("\n>>> ") // Print prompt manually
			query, err = tReader.ReadString('\n')
			if err != nil {
				klog.Errorf("Error reading from TTY: %v", err)
				u.agent.Input <- fmt.Errorf("error reading from TTY: %w", err)
				return
			}
			klog.Infof("Sending TTY input to agent: %q", query)
			u.agent.Input <- &api.UserInputResponse{Query: query}
		} else {
			rlInstance, err := u.readlineInstance()
			if err != nil {
				klog.Errorf("Failed to create readline instance: %v", err)
				u.agent.Input <- fmt.Errorf("error creating readline instance: %w", err)
				return
			}
			rlInstance.SetPrompt(">>> ") // Ensure correct prompt
			query, err = rlInstance.Readline()
			if err != nil {
				klog.Infof("Readline error: %v", err)
				switch err {
				case readline.ErrInterrupt: // Handle Ctrl+C
					u.agent.Input <- io.EOF
				case io.EOF: // Handle Ctrl+D
					u.agent.Input <- io.EOF
				default:
					u.agent.Input <- err
				}
			} else {
				klog.Infof("Sending readline input to agent: %q", query)
				u.agent.Input <- &api.UserInputResponse{Query: query}
			}
		}
		if query == "clear" || query == "reset" {
			u.ClearScreen()
		}
		return
	case api.MessageTypeUserChoiceRequest:
		choiceRequest := msg.Payload.(*api.UserChoiceRequest)
		prompt, _ := u.markdownRenderer.Render(choiceRequest.Prompt)
		fmt.Printf("\n%s\n", string(prompt))

		for i, option := range choiceRequest.Options {
			fmt.Printf("  %d. %s\n", i+1, option.Label)
		}
		fmt.Println()

		var choice int
		for {
			var line string
			var err error
			if u.useTTYForInput {
				tReader, err := u.ttyReader()
				if err != nil {
					klog.Errorf("Failed to get TTY reader: %v", err)
					return
				}
				fmt.Print("Enter your choice: ")
				line, err = tReader.ReadString('\n')
				if err != nil {
					klog.Errorf("Error reading from TTY: %v", err)
					u.agent.Input <- fmt.Errorf("error reading from TTY: %w", err)
					return
				}
			} else {
				rlInstance, err := u.readlineInstance()
				if err != nil {
					klog.Errorf("Failed to create readline instance: %v", err)
					u.agent.Input <- fmt.Errorf("error creating readline instance: %w", err)
					return
				}
				rlInstance.SetPrompt("Enter your choice: ")
				line, err = rlInstance.Readline()
				if err != nil {
					klog.Infof("Readline error: %v", err)
					switch err {
					case readline.ErrInterrupt, io.EOF:
						u.agent.Input <- io.EOF
						return
					default:
						u.agent.Input <- err
						return
					}
				}
			}

			input := strings.TrimSpace(strings.ToLower(line))
			choice = -1

			// Handle special cases for yes/no
			if input == "y" || input == "yes" {
				input = "1"
			}
			if input == "n" || input == "no" {
				input = "3"
			}

			choiceIdx, err := strconv.Atoi(input)
			if err == nil && choiceIdx > 0 && choiceIdx <= len(choiceRequest.Options) {
				choice = choiceIdx
				break
			}

			fmt.Println("Invalid choice. Please try again.")
		}
		u.agent.Input <- &api.UserChoiceResponse{Choice: choice}
		return
	default:
		klog.Warningf("unsupported message type: %v", msg.Type)
		return
	}

	computedStyle := &computedStyle{}
	for _, opt := range styleOptions {
		opt(computedStyle)
	}

	printText := text

	if computedStyle.RenderMarkdown && printText != "" {
		out, err := u.markdownRenderer.Render(printText)
		if err != nil {
			klog.Errorf("Error rendering markdown: %v", err)
		} else {
			printText = out
		}
	}
	reset := ""
	switch computedStyle.Foreground {
	case colorRed:
		fmt.Printf("\033[31m")
		reset += "\033[0m"
	case colorGreen:
		fmt.Printf("\033[32m")
		reset += "\033[0m"
	case colorWhite:
		fmt.Printf("\033[37m")
		reset += "\033[0m"

	case "":
	default:
		klog.Info("foreground color not supported by TerminalUI", "color", computedStyle.Foreground)
	}

	fmt.Printf("%s%s", printText, reset)
}

func (u *TerminalUI) ClearScreen() {
	fmt.Print("\033[H\033[2J")
}

func formatToolCallResponse(payload any) string {
	if payload == nil {
		return ""
	}

	// If payload is a JSON string try to unmarshal first
	if s, ok := payload.(string); ok {
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err == nil {
			return formatToolCallResponse(m)
		}
		return truncateString(s, 1000)
	}

	if m, ok := payload.(map[string]any); ok {
		if stdout, ok := m["stdout"]; ok {
			if outStr, ok := stdout.(string); ok {
				return truncateString(outStr, 1000)
			}
		}
		b, err := json.MarshalIndent(m, "", "  ")
		if err == nil {
			return truncateString(string(b), 1000)
		}
		return fmt.Sprint(m)
	}

	b, err := json.MarshalIndent(payload, "", "  ")
	if err == nil {
		return truncateString(string(b), 1000)
	}
	return fmt.Sprint(payload)
}

func truncateString(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "..."
}

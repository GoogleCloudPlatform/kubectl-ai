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
	"k8s.io/klog/v2"
)

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

func NewTerminalUI(journal journal.Recorder, useTTYForInput bool, agent *agent.Agent) (*TerminalUI, error) {
	mdRenderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithPreservedNewLines(),
		glamour.WithEmoji(),
	)
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
			}
		}
	}()

	// Block until context is cancelled
	<-ctx.Done()
	return nil
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
	var styleOptions []StyleOption

	switch msg.Type {
	case api.MessageTypeText:
		text = msg.Payload.(string)
		switch msg.Source {
		case api.MessageSourceUser:
			// styleOptions = append(styleOptions, Foreground(ColorWhite))
			// since we print the message as user types, we don't need to print it again
			return
		case api.MessageSourceAgent:
			styleOptions = append(styleOptions, RenderMarkdown(), Foreground(ColorGreen))
		case api.MessageSourceModel:
			styleOptions = append(styleOptions, RenderMarkdown())
		}
	case api.MessageTypeError:
		styleOptions = append(styleOptions, Foreground(ColorRed))
		text = msg.Payload.(string)
	case api.MessageTypeToolCallRequest:
		styleOptions = append(styleOptions, Foreground(ColorGreen))
		text = fmt.Sprintf("  Running: %s\n", msg.Payload.(string))
	case api.MessageTypeToolCallResponse:
		// TODO: we should print the tool call result here
		return
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
			u.agent.Input <- query
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
				u.agent.Input <- query
			}
		}
		return
	case api.MessageTypeUserChoiceRequest:
		choiceRequest := msg.Payload.(*api.UserChoiceRequest)
		styleOptions = append(styleOptions, Foreground(ColorWhite))
		// text = fmt.Sprintf("  %s\n", choiceRequest.Prompt)
		if choiceRequest.Prompt != "" {
			markdown, err := u.markdownRenderer.Render(choiceRequest.Prompt)
			if err != nil {
				klog.Errorf("Error rendering markdown: %v", err)
			} else {
				text = markdown
			}
		}
		choiceNumbers := []string{}
		for i, option := range choiceRequest.Options {
			choiceNumbers = append(choiceNumbers, strconv.Itoa(i+1))
			text += fmt.Sprintf("  %d) %s\n", i+1, option.Value)
		}
		fmt.Printf("%s\n", text)
		fmt.Printf("  Enter your choice (%s): ", strings.Join(choiceNumbers, ","))

		if u.useTTYForInput {
			tReader, err := u.ttyReader()
			if err != nil {
				u.agent.Input <- fmt.Errorf("error reading from TTY: %w", err)
				return
			}
			for {
				// fmt.Printf("%s\n", text)
				fmt.Printf("  Enter your choice (%s): ", strings.Join(choiceNumbers, ","))
				response, err := tReader.ReadString('\n')
				if err != nil {
					u.agent.Input <- err
					return
				}
				response = strings.TrimSpace(response)
				choiceIndex, err := strconv.Atoi(response)
				if err != nil {
					klog.Errorf("invalid choice: %v", err)
					return
				}
				optionKey := choiceRequest.Options[choiceIndex].Key
				if optionKey != "" {
					if choiceIndex == -1 {
						klog.Errorf("could not find option with key %q", optionKey)
						return
					}
					u.agent.Input <- int32(choiceIndex + 1)
					break // Exit loop on valid choice
				} else {
					fmt.Printf("  Invalid choice. Please enter one of: %s\n", strings.Join(choiceNumbers, ", "))
				}
			}
		} else {
			rlInstance, err := u.readlineInstance()
			if err != nil {
				u.agent.Input <- fmt.Errorf("error creating readline instance: %w", err)
				return
			}
			// Temporarily change prompt for option selection
			originalPrompt := rlInstance.Config.Prompt
			choicePrompt := fmt.Sprintf("  Enter your choice (%s): ", strings.Join(choiceNumbers, ","))
			rlInstance.SetPrompt(choicePrompt)
			// Ensure original prompt is restored even if errors occur
			defer rlInstance.SetPrompt(originalPrompt)

			countCTRL_D := 0
			for {
				// fmt.Printf(choicePrompt, strings.Join(choiceNumbers, ","))
				response, err := rlInstance.Readline()
				if err != nil {
					switch err {
					case readline.ErrInterrupt: // Handle Ctrl+C
						u.agent.Input <- io.EOF
					case io.EOF: // Handle Ctrl+D
						countCTRL_D++
						if countCTRL_D > 1 {
							u.agent.Input <- io.EOF
							return
						}
					default:
						u.agent.Input <- err
					}
				}

				response = strings.TrimSpace(response)
				choiceIndex, err := strconv.Atoi(response)
				if err != nil {
					klog.Errorf("invalid choice: %v", err)
					continue
				}
				optionKey := choiceRequest.Options[choiceIndex-1].Key
				if optionKey != "" {
					u.agent.Input <- int32(choiceIndex + 1)
					break // Exit loop on valid choice
				} else {
					fmt.Printf("\n  Invalid choice. Please enter one of: %s\n", strings.Join(choiceNumbers, ", "))
				}
			}
		}
		return
	default:
		klog.Warningf("unsupported message type: %v", msg.Type)
		return
	}

	computedStyle := &ComputedStyle{}
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
	case ColorRed:
		fmt.Printf("\033[31m")
		reset += "\033[0m"
	case ColorGreen:
		fmt.Printf("\033[32m")
		reset += "\033[0m"
	case ColorWhite:
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

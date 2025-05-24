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
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/journal"
	"github.com/charmbracelet/glamour"
	"github.com/chzyer/readline"
	"k8s.io/klog/v2"
)

type TerminalUI struct {
	journal          journal.Recorder
	markdownRenderer *glamour.TermRenderer

	subscription io.Closer

	// Input handling fields (initialized once)
	rlInstance        *readline.Instance // For readline input
	ttyFile           *os.File           // For TTY input
	ttyReaderInstance *bufio.Reader      // For TTY input

	// currentBlock is the block we are rendering
	currentBlock Block
	// currentBlockText is text of the currentBlock that we have already rendered to the screen
	currentBlockText string

	// This is useful in cases where stdin is already been used for providing the input to the agent (caller in this case)
	// in such cases, stdin is already consumed and closed and reading input results in IO error.
	// In such cases, we open /dev/tty and use it for taking input.
	useTTYForInput bool
}

var _ UI = &TerminalUI{}

func NewTerminalUI(doc *Document, journal journal.Recorder, useTTYForInput bool) (*TerminalUI, error) {
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
	}

	subscription := doc.AddSubscription(u)
	u.subscription = subscription

	return u, nil
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
	if u.subscription != nil {
		if err := u.subscription.Close(); err != nil {
			errs = append(errs, err)
		} else {
			u.subscription = nil
		}
	}
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

func (u *TerminalUI) DocumentChanged(doc *Document, block Block) {
	blockIndex := doc.IndexOf(block)

	if blockIndex != doc.NumBlocks()-1 {
		klog.Warningf("update to blocks other than the last block is not supported in terminal mode")
		return
	}

	if u.currentBlock != block {
		u.currentBlock = block
		if u.currentBlockText != "" {
			fmt.Printf("\n")
		}
		u.currentBlockText = ""
	}

	text := ""
	streaming := false

	var styleOptions []StyleOption
	switch block := block.(type) {
	case *ErrorBlock:
		styleOptions = append(styleOptions, Foreground(ColorRed))
		text = block.Text()
	case *FunctionCallRequestBlock:
		styleOptions = append(styleOptions, Foreground(ColorGreen))
		text = block.Text()
	case *AgentTextBlock:
		styleOptions = append(styleOptions, RenderMarkdown())
		if block.Color != "" {
			styleOptions = append(styleOptions, Foreground(block.Color))
		}
		text = block.Text()
		streaming = block.Streaming()
	case *InputTextBlock:
		var query string
		if u.useTTYForInput {
			tReader, err := u.ttyReader()
			if err != nil {
				block.Observable().Set("", fmt.Errorf("TTY reader not initialized"))
				return
			}
			fmt.Print("\n>>> ") // Print prompt manually
			query, err = tReader.ReadString('\n')
			if err != nil {
				block.Observable().Set("", err) // Set error (includes io.EOF)
			} else {
				block.Observable().Set(query, nil)
			}
		} else {
			rlInstance, err := u.readlineInstance()
			if err != nil {
				block.Observable().Set("", fmt.Errorf("error creating readline instance: %w", err))
				return
			}
			rlInstance.SetPrompt(">>> ") // Ensure correct prompt
			query, err = rlInstance.Readline()
			if err != nil {
				if err == readline.ErrInterrupt { // Handle Ctrl+C
					block.Observable().Set("", io.EOF)
				} else if err == io.EOF { // Handle Ctrl+D
					block.Observable().Set("", io.EOF)
				} else {
					block.Observable().Set("", err)
				}
			} else {
				block.Observable().Set(query, nil)
			}
		}
		return

	case *InputOptionBlock:
		fmt.Printf("%s\n", block.Prompt) // Print initial prompt text

		if u.useTTYForInput {
			tReader, err := u.ttyReader()
			if err != nil {
				block.Observable().Set("", fmt.Errorf("TTY reader not initialized"))
				return
			}
			for {
				fmt.Print("  Enter your choice (1,2,3): ") // Print loop prompt manually
				response, err := tReader.ReadString('\n')
				if err != nil {
					block.Observable().Set("", err)
					return
				}
				choice := strings.TrimSpace(response)
				if slices.Contains(block.Options, choice) {
					block.Observable().Set(choice, nil)
					break
				}
				fmt.Printf("  Invalid choice. Please enter one of: %s\n", strings.Join(block.Options, ", "))
			}
		} else {
			rlInstance, err := u.readlineInstance()
			if err != nil {
				block.Observable().Set("", fmt.Errorf("readline instance not initialized: %w", err))
				return
			}
			// Temporarily change prompt for option selection
			originalPrompt := rlInstance.Config.Prompt
			choicePrompt := "  Enter your choice (1,2,3): "
			rlInstance.SetPrompt(choicePrompt)
			// Ensure original prompt is restored even if errors occur
			defer rlInstance.SetPrompt(originalPrompt)

			for {
				response, err := rlInstance.Readline()
				if err != nil {
					if err == readline.ErrInterrupt { // Handle Ctrl+C
						block.Observable().Set("", io.EOF)
						return
					} else if err == io.EOF { // Handle Ctrl+D
						block.Observable().Set("", io.EOF)
						return
					} else {
						block.Observable().Set("", err)
						return
					}
				}

				choice := strings.TrimSpace(response)
				if slices.Contains(block.Options, choice) {
					block.Observable().Set(choice, nil)
					break // Exit loop on valid choice
				}
				// Print error message; readline will reprint the prompt
				fmt.Printf("\n  Invalid choice. Please enter one of: %s\n", strings.Join(block.Options, ", "))
			}
		}
		return
	}

	computedStyle := &ComputedStyle{}
	for _, opt := range styleOptions {
		opt(computedStyle)
	}

	if streaming && computedStyle.RenderMarkdown {
		// Because we can't render markdown incrementally,
		// we "hold back" the text if we are streaming markdown until streaming is done
		text = ""
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

	if u.currentBlockText != "" {
		if strings.HasPrefix(text, u.currentBlockText) {
			printText = strings.TrimPrefix(printText, u.currentBlockText)
		} else {
			klog.Warningf("text did not match text already rendered; text %q; currentBlockText %q", text, u.currentBlockText)
		}
	}
	u.currentBlockText = text

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

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
	"strings"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/journal"
	"github.com/charmbracelet/glamour"
	"k8s.io/klog/v2"
)

type TerminalUI struct {
	journal          journal.Recorder
	markdownRenderer *glamour.TermRenderer

	subscription io.Closer

	// currentBlock is the block we are rendering
	currentBlock Block
	// currentBlockText is text of the currentBlock that we have already rendered to the screen
	currentBlockText string
}

var _ UI = &TerminalUI{}

func NewTerminalUI(doc *Document, journal journal.Recorder) (*TerminalUI, error) {
	mdRenderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithPreservedNewLines(),
		glamour.WithEmoji(),
	)
	if err != nil {
		return nil, fmt.Errorf("error initializing the markdown renderer: %w", err)
	}
	u := &TerminalUI{markdownRenderer: mdRenderer, journal: journal}

	subscription := doc.AddSubscription(u)
	u.subscription = subscription

	return u, nil
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
		text = block.Text()
	case *InputTextBlock:
		fmt.Print("\n>>> ")
		reader := bufio.NewReader(os.Stdin)
		query, err := reader.ReadString('\n')
		if err != nil {
			block.Observable().Set("error")
		} else {
			block.Observable().Set(query)
		}
		return
	}

	computedStyle := &style{}
	for _, opt := range styleOptions {
		opt(computedStyle)
	}

	printText := text

	if u.currentBlockText != "" {
		if strings.HasPrefix(text, u.currentBlockText) {
			printText = strings.TrimPrefix(printText, u.currentBlockText)
		} else {
			klog.Warningf("text did not match text already rendered; text %q; currentBlockText %q", text, u.currentBlockText)
		}
	}
	u.currentBlockText = text

	// if computedStyle.renderMarkdown {
	// 	out, err := u.markdownRenderer.Render(printText)
	// 	if err != nil {
	// 		klog.Errorf("Error rendering markdown: %v", err)
	// 	} else {
	// 		printText = out
	// 	}
	// }

	reset := ""
	switch computedStyle.foreground {
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
		klog.Info("foreground color not supported by TerminalUI", "color", computedStyle.foreground)
	}

	fmt.Printf("%s%s", printText, reset)
}

func (u *TerminalUI) RenderOutput(ctx context.Context, s string, styleOptions ...StyleOption) {
	log := klog.FromContext(ctx)

	u.journal.Write(ctx, &journal.Event{
		Action: journal.ActionUIRender,
		Payload: map[string]any{
			"text": s,
		},
	})

	computedStyle := &style{}
	for _, opt := range styleOptions {
		opt(computedStyle)
	}

	if computedStyle.renderMarkdown {
		out, err := u.markdownRenderer.Render(s)
		if err != nil {
			log.Error(err, "Error rendering markdown")
		}
		s = out
	}

	reset := ""
	switch computedStyle.foreground {
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
		log.Info("foreground color not supported by TerminalUI", "color", computedStyle.foreground)
	}

	fmt.Printf("%s%s", s, reset)
}

func (u *TerminalUI) ClearScreen() {
	fmt.Print("\033[H\033[2J")
}

func (u *TerminalUI) AskForConfirmation(ctx context.Context, s string) bool {
	log := klog.FromContext(ctx)
	fmt.Printf("%s\n", s)
	var response string
	_, err := fmt.Scanln(&response)
	if err != nil {
		log.Error(err, "Error reading user input")
		return false
	}
	response = strings.ToLower(response)
	return response == "y" || response == "yes"
}

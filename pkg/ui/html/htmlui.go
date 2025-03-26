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

package html

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/journal"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/ui"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/ui/html/templates"
	"github.com/charmbracelet/glamour"
	"k8s.io/klog/v2"
)

type HTMLUserInterface struct {
	httpServer         *http.Server
	httpServerListener net.Listener

	doc              *ui.Document
	journal          journal.Recorder
	markdownRenderer *glamour.TermRenderer

	// subscription io.Closer

	// // currentBlock is the block we are rendering
	// currentBlock ui.Block
	// // currentBlockText is text of the currentBlock that we have already rendered to the screen
	// currentBlockText string
}

var _ ui.UI = &HTMLUserInterface{}

func NewHTMLUserInterface(doc *ui.Document, journal journal.Recorder) (*HTMLUserInterface, error) {
	listen := "localhost:8888"

	mux := http.NewServeMux()
	httpServer := &http.Server{
		Addr:    listen,
		Handler: mux,
	}

	u := &HTMLUserInterface{
		doc:     doc,
		journal: journal,
	}

	mux.HandleFunc("GET /", u.serveIndex)
	mux.HandleFunc("GET /doc-stream", u.serveDocStream)
	mux.HandleFunc("POST /send-message", u.handlePOSTSendMessage)
	mux.HandleFunc("POST /choose-option", u.handlePOSTChooseOption)

	httpServerListener, err := net.Listen("tcp", listen)
	if err != nil {
		return nil, fmt.Errorf("starting http server network listener: %w", err)
	}
	endpoint := httpServerListener.Addr()
	u.httpServerListener = httpServerListener
	u.httpServer = httpServer

	fmt.Fprintf(os.Stdout, "listening on http://%s\n", endpoint)

	mdRenderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithPreservedNewLines(),
		glamour.WithEmoji(),
	)
	if err != nil {
		return nil, fmt.Errorf("error initializing the markdown renderer: %w", err)
	}
	u.markdownRenderer = mdRenderer

	// subscription := doc.AddSubscription(u)
	// u.subscription = subscription

	return u, nil
}

// func (u *HTMLUserInterface) server() string {
// 	return u.httpServerListener.Addr().String()
// }

func (u *HTMLUserInterface) RunServer(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		// u.httpServer(ctx)
		u.httpServerListener.Close()
	}()
	return u.httpServer.Serve(u.httpServerListener)
}

func (u *HTMLUserInterface) serveIndex(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	log := klog.FromContext(ctx)

	var bb bytes.Buffer
	if err := renderTemplate(ctx, &bb, "index.html", nil); err != nil {
		log.Error(err, "rendering index.html")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(bb.Bytes())
}

func (u *HTMLUserInterface) handlePOSTSendMessage(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	log := klog.FromContext(ctx)

	if err := req.ParseForm(); err != nil {
		log.Error(err, "parsing form")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Info("got request", "values", req.Form)

	q := req.FormValue("q")
	if q == "" {
		http.Error(w, "missing query", http.StatusBadRequest)
		return
	}

	// TODO: Match by block id
	var inputBlock *ui.InputTextBlock
	for _, block := range u.doc.Blocks() {
		if block, ok := block.(*ui.InputTextBlock); ok {
			inputBlock = block
		}
	}

	if inputBlock == nil {
		log.Info("no input block found")
		http.Error(w, "no input block found", http.StatusInternalServerError)
		return
	}

	inputBlock.Observable().Set(q, nil)

	var bb bytes.Buffer
	bb.WriteString("ok")
	w.Write(bb.Bytes())
}

func (u *HTMLUserInterface) handlePOSTChooseOption(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	log := klog.FromContext(ctx)

	if err := req.ParseForm(); err != nil {
		log.Error(err, "parsing form")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Info("got request", "values", req.Form)

	optionKey := req.FormValue("option")
	if optionKey == "" {
		http.Error(w, "missing option", http.StatusBadRequest)
		return
	}

	// TODO: Match by block id
	var inputOptionBlock *ui.InputOptionBlock
	for _, block := range u.doc.Blocks() {
		if block, ok := block.(*ui.InputOptionBlock); ok {
			inputOptionBlock = block
		}
	}

	if inputOptionBlock == nil {
		log.Info("no input option lock found")
		http.Error(w, "no input option block found", http.StatusInternalServerError)
		return
	}

	inputOptionBlock.Observable().Set(optionKey, nil)
	var bb bytes.Buffer
	bb.WriteString("ok")
	w.Write(bb.Bytes())
}

func (u *HTMLUserInterface) serveDocStream(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	log := klog.FromContext(ctx)

	log.Info("in serverDocStream")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	rc := http.NewResponseController(w)

	sendAllBlocks := func() {
		var sse bytes.Buffer
		sse.WriteString("event: ReplaceAll\ndata: ")

		blocks := u.doc.Blocks()
		var html bytes.Buffer
		for _, block := range blocks {
			if err := u.renderBlock(ctx, &html, block); err != nil {
				log.Error(err, "rendering block")
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		for i, line := range bytes.Split(html.Bytes(), []byte("\n")) {
			if i != 0 {
				sse.WriteString("data: ")
			}
			sse.Write(line)
			sse.WriteString("\n")
		}
		sse.WriteString("\n")
		w.Write(sse.Bytes())
		log.Info("writing SSE", "data", sse.String())
		rc.Flush()
	}

	onDocChange := func(doc *ui.Document, block ui.Block) {
		sendAllBlocks()
	}

	subscription := u.doc.AddSubscription(ui.SubscriberFromFunc(onDocChange))
	defer subscription.Close()

	// Send initial message
	sendAllBlocks()

	clientGone := req.Context().Done()

	keepAliveTimer := time.NewTicker(5 * time.Second)
	defer keepAliveTimer.Stop()

	for {
		select {
		case <-clientGone:
			// client disconnected
			return
		case <-keepAliveTimer.C:
			// Send a ping
			log.V(4).Info("sending SSE ping")
			if _, err := fmt.Fprintf(w, "event: Ping\ndata: {}\n\n"); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
		}
	}
}

func renderTemplate(ctx context.Context, w io.Writer, key string, data any) error {
	log := klog.FromContext(ctx)
	tmpl, err := templates.LoadTemplate(key)
	if err != nil {
		return fmt.Errorf("loading template %q: %w", key, err)
	}
	if err := tmpl.Execute(w, data); err != nil {
		return fmt.Errorf("executing %q: %w", key, err)
	}

	log.Info("rendered page", "key", key)
	return nil
}

func (u *HTMLUserInterface) Close() error {
	var errs []error
	if u.httpServerListener != nil {
		if err := u.httpServerListener.Close(); err != nil {
			errs = append(errs, err)
		} else {
			u.httpServerListener = nil
		}
	}
	// if u.subscription != nil {
	// 	if err := u.subscription.Close(); err != nil {
	// 		errs = append(errs, err)
	// 	} else {
	// 		u.subscription = nil
	// 	}
	// }
	return errors.Join(errs...)
}

type HTMLWriter struct {
}

func (u *HTMLUserInterface) renderBlock(ctx context.Context, w io.Writer, block ui.Block) error {
	switch block := block.(type) {
	case *ui.ErrorBlock:
		return renderTemplate(ctx, w, "error_block.html", block)
	case *ui.FunctionCallRequestBlock:
		return renderTemplate(ctx, w, "function_call_request_block.html", block)
	case *ui.AgentTextBlock:
		return renderTemplate(ctx, w, "agent_text_block.html", block)
	case *ui.InputTextBlock:
		return renderTemplate(ctx, w, "input_text_block.html", block)
	case *ui.InputOptionBlock:
		return renderTemplate(ctx, w, "input_option_block.html", block)

	default:
		return fmt.Errorf("unknown block type %T", block)
	}
}

// func (u *HTMLUserInterface) DocumentChanged(doc *ui.Document, block ui.Block) {
// 	klog
// blockIndex := doc.IndexOf(block)

// if blockIndex != doc.NumBlocks()-1 {
// 	klog.Warningf("update to blocks other than the last block is not supported in terminal mode")
// 	return
// }

// if u.currentBlock != block {
// 	u.currentBlock = block
// 	if u.currentBlockText != "" {
// 		fmt.Printf("\n")
// 	}
// 	u.currentBlockText = ""
// }

// text := ""

// var styleOptions []ui.StyleOption
// switch block := block.(type) {
// case *ui.ErrorBlock:
// 	styleOptions = append(styleOptions, ui.Foreground(ui.ColorRed))
// 	text = block.Text()
// case *ui.FunctionCallRequestBlock:
// 	styleOptions = append(styleOptions, ui.Foreground(ui.ColorGreen))
// 	text = block.Text()
// case *ui.AgentTextBlock:
// 	styleOptions = append(styleOptions, ui.RenderMarkdown())
// 	if block.Color != "" {
// 		styleOptions = append(styleOptions, ui.Foreground(block.Color))
// 	}
// 	text = block.Text()
// case *ui.InputTextBlock:
// 	fmt.Print("\n>>> ")
// 	reader := bufio.NewReader(os.Stdin)
// 	query, err := reader.ReadString('\n')
// 	if err != nil {
// 		block.Observable().Set("", err)
// 	} else {
// 		block.Observable().Set(query, nil)
// 	}
// 	return

// case *ui.InputOptionBlock:
// 	fmt.Printf("%s\n", block.Prompt)

// 	for {
// 		fmt.Print("  Enter your choice (number): ")
// 		var response string
// 		_, err := fmt.Scanln(&response)
// 		if err != nil {
// 			block.Observable().Set("", err)
// 			break
// 		}

// 		choice := strings.TrimSpace(response)

// 		if slices.Contains(block.Options, choice) {
// 			block.Observable().Set(choice, nil)
// 			break
// 		}

// 		// If not returned, the choice was invalid
// 		fmt.Printf("  Invalid choice. Please enter one of: %s\n", strings.Join(block.Options, ", "))
// 		continue
// 	}
// 	return
// }

// computedStyle := &ui.ComputedStyle{}
// for _, opt := range styleOptions {
// 	opt(computedStyle)
// }

// printText := text

// if u.currentBlockText != "" {
// 	if strings.HasPrefix(text, u.currentBlockText) {
// 		printText = strings.TrimPrefix(printText, u.currentBlockText)
// 	} else {
// 		klog.Warningf("text did not match text already rendered; text %q; currentBlockText %q", text, u.currentBlockText)
// 	}
// }
// u.currentBlockText = text

// // TODO: Reintroduce markdown support (it's difficult with streaming)
// //
// // if computedStyle.renderMarkdown {
// // 	out, err := u.markdownRenderer.Render(printText)
// // 	if err != nil {
// // 		klog.Errorf("Error rendering markdown: %v", err)
// // 	} else {
// // 		printText = out
// // 	}
// // }

// reset := ""
// switch computedStyle.Foreground {
// case ui.ColorRed:
// 	fmt.Printf("\033[31m")
// 	reset += "\033[0m"
// case ui.ColorGreen:
// 	fmt.Printf("\033[32m")
// 	reset += "\033[0m"
// case ui.ColorWhite:
// 	fmt.Printf("\033[37m")
// 	reset += "\033[0m"

// case "":
// default:
// 	klog.Info("foreground color not supported by TerminalUI", "color", computedStyle.Foreground)
// }

// fmt.Printf("%s%s", printText, reset)
// }

// func (u *HTMLUserInterface) RenderOutput(ctx context.Context, s string, styleOptions ...StyleOption) {
// 	log := klog.FromContext(ctx)

// 	u.journal.Write(ctx, &journal.Event{
// 		Action: journal.ActionUIRender,
// 		Payload: map[string]any{
// 			"text": s,
// 		},
// 	})

// 	computedStyle := &style{}
// 	for _, opt := range styleOptions {
// 		opt(computedStyle)
// 	}

// 	if computedStyle.renderMarkdown {
// 		out, err := u.markdownRenderer.Render(s)
// 		if err != nil {
// 			log.Error(err, "Error rendering markdown")
// 		}
// 		s = out
// 	}

// 	reset := ""
// 	switch computedStyle.foreground {
// 	case ColorRed:
// 		fmt.Printf("\033[31m")
// 		reset += "\033[0m"
// 	case ColorGreen:
// 		fmt.Printf("\033[32m")
// 		reset += "\033[0m"
// 	case ColorWhite:
// 		fmt.Printf("\033[37m")
// 		reset += "\033[0m"

// 	case "":
// 	default:
// 		log.Info("foreground color not supported by TerminalUI", "color", computedStyle.foreground)
// 	}

// 	fmt.Printf("%s%s", s, reset)
// }

func (u *HTMLUserInterface) ClearScreen() {
	// fmt.Print("\033[H\033[2J")
}

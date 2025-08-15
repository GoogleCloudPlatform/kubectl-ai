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
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/agent"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/journal"
	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/ui"
	"github.com/charmbracelet/glamour"
	"golang.org/x/sync/errgroup"
	"k8s.io/klog/v2"
)

// Broadcaster manages a set of clients for Server-Sent Events.
type Broadcaster struct {
	clients   map[chan []byte]bool
	newClient chan chan []byte
	delClient chan chan []byte
	messages  chan []byte
	mu        sync.Mutex
}

// NewBroadcaster creates a new Broadcaster instance.
func NewBroadcaster() *Broadcaster {
	b := &Broadcaster{
		clients:   make(map[chan []byte]bool),
		newClient: make(chan (chan []byte)),
		delClient: make(chan (chan []byte)),
		messages:  make(chan []byte, 10),
	}
	return b
}

// Run starts the broadcaster's event loop.
func (b *Broadcaster) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case client := <-b.newClient:
			b.mu.Lock()
			b.clients[client] = true
			b.mu.Unlock()
		case client := <-b.delClient:
			b.mu.Lock()
			delete(b.clients, client)
			close(client)
			b.mu.Unlock()
		case msg := <-b.messages:
			b.mu.Lock()
			for client := range b.clients {
				select {
				case client <- msg:
				default:
					klog.Warning("SSE client buffer full, dropping message.")
				}
			}
			b.mu.Unlock()
		}
	}
}

// Broadcast sends a message to all connected clients.
func (b *Broadcaster) Broadcast(msg []byte) {
	b.messages <- msg
}

type HTMLUserInterface struct {
	httpServer         *http.Server
	httpServerListener net.Listener

	agent            *agent.Agent
	journal          journal.Recorder
	markdownRenderer *glamour.TermRenderer
	broadcaster      *Broadcaster
}

var _ ui.UI = &HTMLUserInterface{}

func NewHTMLUserInterface(agent *agent.Agent, listenAddress string, journal journal.Recorder) (*HTMLUserInterface, error) {
	mux := http.NewServeMux()

	u := &HTMLUserInterface{
		agent:       agent,
		journal:     journal,
		broadcaster: NewBroadcaster(),
	}

	httpServer := &http.Server{
		Addr:    listenAddress,
		Handler: mux,
	}

	mux.HandleFunc("GET /", u.serveIndex)
	mux.HandleFunc("GET /messages-stream", u.serveMessagesStream)
	mux.HandleFunc("POST /send-message", u.handlePOSTSendMessage)
	mux.HandleFunc("POST /choose-option", u.handlePOSTChooseOption)

	httpServerListener, err := net.Listen("tcp", listenAddress)
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

	return u, nil
}

func (u *HTMLUserInterface) Run(ctx context.Context) error {
	g, gctx := errgroup.WithContext(ctx)

	// Start the broadcaster
	g.Go(func() error {
		u.broadcaster.Run(gctx)
		return nil
	})

	// This goroutine listens to agent output and broadcasts it.
	g.Go(func() error {
		for {
			select {
			case <-gctx.Done():
				return nil
			case _, ok := <-u.agent.Output:
				if !ok {
					return nil // Channel closed
				}
				// We received a message from the agent. It's a signal that
				// the state has changed. We fetch the entire current state and
				// broadcast it to all connected clients.
				jsonData, err := u.getCurrentStateJSON()
				if err != nil {
					// Don't return an error, just log it and continue
					klog.Errorf("Error marshaling state for broadcast: %v", err)
					continue
				}
				u.broadcaster.Broadcast(jsonData)
			}
		}
	})

	g.Go(func() error {
		if err := u.httpServer.Serve(u.httpServerListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("error running http server: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		<-gctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := u.httpServer.Shutdown(shutdownCtx); err != nil {
			klog.Errorf("HTTP server shutdown error: %v", err)
		}
		return nil
	})

	return g.Wait()
}

//go:embed index.html
var indexHTML []byte

func (u *HTMLUserInterface) serveIndex(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write(indexHTML)
}

func (u *HTMLUserInterface) serveMessagesStream(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	log := klog.FromContext(ctx)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	clientChan := make(chan []byte, 10)
	u.broadcaster.newClient <- clientChan
	defer func() {
		u.broadcaster.delClient <- clientChan
	}()

	log.Info("SSE client connected")

	// Immediately send the current state to the new client
	initialData, err := u.getCurrentStateJSON()
	if err != nil {
		log.Error(err, "getting initial state for SSE client")
	} else {
		fmt.Fprintf(w, "data: %s\n\n", initialData)
		flusher.Flush()
	}

	for {
		select {
		case <-ctx.Done():
			log.Info("SSE client disconnected")
			return
		case msg := <-clientChan:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}
	}
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

	// Send the message to the agent
	u.agent.Input <- &api.UserInputResponse{Query: q}

	w.WriteHeader(http.StatusOK)
}

func (u *HTMLUserInterface) getCurrentStateJSON() ([]byte, error) {
	allMessages := u.agent.GetSession().AllMessages()
	// Create a copy of the messages to avoid race conditions
	var messages []*api.Message
	for _, message := range allMessages {
		if message.Type == api.MessageTypeUserInputRequest && message.Payload == ">>>" {
			continue
		}
		messages = append(messages, message)
	}

	agentState := u.agent.GetSession().AgentState

	data := map[string]interface{}{
		"messages":   messages,
		"agentState": agentState,
	}
	return json.Marshal(data)
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

	choice := req.FormValue("choice")
	if choice == "" {
		http.Error(w, "missing choice", http.StatusBadRequest)
		return
	}

	choiceIndex, err := strconv.Atoi(choice)
	if err != nil {
		http.Error(w, "invalid choice", http.StatusBadRequest)
		return
	}

	// Send the choice to the agent
	u.agent.Input <- &api.UserChoiceResponse{Choice: choiceIndex}

	w.WriteHeader(http.StatusOK)
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
	return errors.Join(errs...)
}

func (u *HTMLUserInterface) ClearScreen() {
	// Not applicable for HTML UI
}

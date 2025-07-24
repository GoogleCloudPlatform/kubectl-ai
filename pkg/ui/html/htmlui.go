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

// Replace single-agent fields with SessionManager and per-session broadcasters.
type HTMLUserInterface struct {
	httpServer         *http.Server
	httpServerListener net.Listener

	sessions *agent.SessionManager
	journal  journal.Recorder

	// markdownRenderer is used by future server-side rendering needs (kept for now)
	markdownRenderer *glamour.TermRenderer

	// broadcasterMap holds a broadcaster per sessionID.
	broadcasterMap map[string]*Broadcaster
	bMu            sync.Mutex // protects broadcasterMap

	promptGroups []api.PromptGroup
}

var _ ui.UI = &HTMLUserInterface{}

func NewHTMLUserInterface(sessions *agent.SessionManager, listenAddress string, journal journal.Recorder) (*HTMLUserInterface, error) {
	mux := http.NewServeMux()

	u := &HTMLUserInterface{
		sessions:       sessions,
		journal:        journal,
		broadcasterMap: make(map[string]*Broadcaster),
		promptGroups:   nil,
	}

	// API routes
	mux.HandleFunc("GET /api/prompts", u.handleGetPrompts)
	mux.HandleFunc("GET /api/sessions", u.handleListSessions)
	mux.HandleFunc("POST /api/sessions", u.handleCreateSession)
	mux.HandleFunc("POST /api/sessions/{id}/rename", u.handleRenameSession)
	mux.HandleFunc("GET /api/sessions/{id}/stream", u.handleSessionStream)
	mux.HandleFunc("POST /api/sessions/{id}/send-message", u.handlePOSTSendMessage)
	mux.HandleFunc("POST /api/sessions/{id}/choose-option", u.handlePOSTChooseOption)

	// Frontend
	mux.HandleFunc("/", u.serveIndex)

	httpServer := &http.Server{
		Addr:    listenAddress,
		Handler: mux,
	}

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

	// Load prompt library
	if pg, err := loadPromptLibrary(); err == nil {
		u.promptGroups = pg
	} else {
		klog.Warningf("failed to load prompt library: %v", err)
	}

	// Ensure there is at least one default session so the UI works out of the box.
	if _, err := u.ensureDefaultSession(context.Background()); err != nil {
		return nil, err
	}

	return u, nil
}

func (u *HTMLUserInterface) Run(ctx context.Context) error {
	g, gctx := errgroup.WithContext(ctx)

	// Start all broadcasters (each runs its own loop)
	g.Go(func() error {
		<-gctx.Done()
		return nil
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

// serveMessagesStream is now replaced by handleSessionStream (per-session).

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

	// Resolve sessionID from URL.
	sessionID := req.PathValue("id")
	ag, ok := u.sessions.GetAgent(sessionID)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	// Send the message to the agent
	ag.Input <- &api.UserInputResponse{Query: q}

	w.WriteHeader(http.StatusOK)
}

func (u *HTMLUserInterface) getCurrentStateJSON(agent *agent.Agent) ([]byte, error) {
	allMessages := agent.Session().AllMessages()
	// Create a copy of the messages to avoid race conditions
	var messages []*api.Message
	for _, message := range allMessages {
		if message.Type == api.MessageTypeUserInputRequest && message.Payload == ">>>" {
			continue
		}
		messages = append(messages, message)
	}

	agentState := agent.Session().AgentState

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

	sessionID := req.PathValue("id")
	ag, ok := u.sessions.GetAgent(sessionID)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	// Send the choice to the agent
	ag.Input <- &api.UserChoiceResponse{Choice: choiceIndex}

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

// ----------------------------
// Session & broadcaster helpers
// ----------------------------

// ensureDefaultSession creates the very first session if none exist and returns it.
func (u *HTMLUserInterface) ensureDefaultSession(ctx context.Context) (*agent.Agent, error) {
	sessions := u.sessions.ListSessions()
	if len(sessions) > 0 {
		ag, _ := u.sessions.GetAgent(sessions[0].ID)
		return ag, nil
	}

	ag, err := u.sessions.CreateSession(ctx)
	if err != nil {
		return nil, err
	}

	// Start the agent loop in background.
	go func() {
		if err := ag.Run(ctx, ""); err != nil {
			klog.Errorf("agent run error (default session): %v", err)
		}
	}()

	return ag, nil
}

func (u *HTMLUserInterface) getBroadcaster(sessionID string) *Broadcaster {
	u.bMu.Lock()
	defer u.bMu.Unlock()
	b, ok := u.broadcasterMap[sessionID]
	if !ok {
		b = NewBroadcaster()
		// Run broadcaster loop in background. It will stop when context is cancelled
		go b.Run(context.Background())
		u.broadcasterMap[sessionID] = b

		// Hook agent output to broadcaster once.
		if ag, ok := u.sessions.GetAgent(sessionID); ok {
			go func() {
				for range ag.Output {
					jsonData, err := u.getCurrentStateJSON(ag)
					if err != nil {
						klog.Errorf("marshal state for session %s: %v", sessionID, err)
						continue
					}
					b.Broadcast(jsonData)
				}
			}()
		}
	}
	return b
}

// -------------------
// HTTP handlers below
// -------------------

func (u *HTMLUserInterface) handleListSessions(w http.ResponseWriter, req *http.Request) {
	sessions := u.sessions.ListSessions()
	data, _ := json.Marshal(sessions)
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (u *HTMLUserInterface) handleCreateSession(w http.ResponseWriter, req *http.Request) {
	// Use a background context so the session survives beyond this HTTP request.
	ctx := context.Background()
	ag, err := u.sessions.CreateSession(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Start agent loop.
	go func() {
		if err := ag.Run(ctx, ""); err != nil {
			klog.Errorf("agent run error for session %s: %v", ag.Session().ID, err)
		}
	}()

	resp := map[string]string{"id": ag.Session().ID}
	json.NewEncoder(w).Encode(resp)
}

func (u *HTMLUserInterface) handleRenameSession(w http.ResponseWriter, req *http.Request) {
	sessionID := req.PathValue("id")
	if err := req.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	newName := req.FormValue("name")
	if newName == "" {
		http.Error(w, "missing name", http.StatusBadRequest)
		return
	}
	if err := u.sessions.RenameSession(sessionID, newName); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (u *HTMLUserInterface) handleSessionStream(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	log := klog.FromContext(ctx)

	sessionID := req.PathValue("id")
	ag, ok := u.sessions.GetAgent(sessionID)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	flusher, okF := w.(http.Flusher)
	if !okF {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	broadcaster := u.getBroadcaster(sessionID)

	clientChan := make(chan []byte, 10)
	broadcaster.newClient <- clientChan
	defer func() { broadcaster.delClient <- clientChan }()

	// Send initial state
	if initial, err := u.getCurrentStateJSON(ag); err == nil {
		fmt.Fprintf(w, "data: %s\n\n", initial)
		flusher.Flush()
	}

	log.Info("SSE client connected", "session", sessionID)

	for {
		select {
		case <-ctx.Done():
			log.Info("SSE client disconnected", "session", sessionID)
			return
		case msg := <-clientChan:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}
	}
}

func (u *HTMLUserInterface) handleGetPrompts(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	prompts := u.promptGroups
	if prompts == nil {
		prompts = []api.PromptGroup{}
	}
	json.NewEncoder(w).Encode(prompts)
}

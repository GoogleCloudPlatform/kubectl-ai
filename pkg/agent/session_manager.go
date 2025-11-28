package agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
	"k8s.io/klog/v2"
)

// SessionManager keeps track of active Agent sessions entirely in memory.
// It is meant for the web UI, where multiple concurrent conversations are
// expected.  Terminal / TUI modes will simply use a single default session
// and never interact with this type.
//
// The manager lives in package `agent` so that it can manipulate *Agent
// instances without creating an import cycle.
// All methods are safe for concurrent use.
type SessionManager struct {
	mu       sync.Mutex
	sessions map[string]*Agent

	// factory returns a fully-configured (but not yet initialised) Agent.
	factory func() *Agent
}

// NewSessionManager creates a new in-memory manager.  The factory must be
// provided; it will be invoked every time CreateSession is called.
func NewSessionManager(factory func() *Agent) *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Agent),
		factory:  factory,
	}
}

// CreateSession builds a new Agent via the factory, initialises it, stores it
// and returns the pointer.  The caller may start the agent.Run loop if desired.
func (sm *SessionManager) CreateSession(ctx context.Context) (*Agent, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	a := sm.factory()
	if err := a.Init(ctx); err != nil {
		return nil, err
	}

	sm.sessions[a.Session().ID] = a
	return a, nil
}

// GetAgent returns the *Agent for the given session ID, or false if not found.
func (sm *SessionManager) GetAgent(sessionID string) (*Agent, bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	ag, ok := sm.sessions[sessionID]
	return ag, ok
}

// ListSessions returns a shallow copy of the Session metadata for all active
// sessions.
func (sm *SessionManager) ListSessions() []*api.Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	out := make([]*api.Session, 0, len(sm.sessions))
	for _, ag := range sm.sessions {
		out = append(out, ag.Session())
	}
	return out
}

// RenameSession sets the Name of the session. Returns error if not found.
func (sm *SessionManager) RenameSession(sessionID, newName string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	ag, ok := sm.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found")
	}
	ag.UpdateSessionName(newName)
	return nil
}

// DeleteSession stops the Agent and removes the session. Returns error if not found.
func (sm *SessionManager) DeleteSession(sessionID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	ag, ok := sm.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found")
	}

	if err := ag.Close(); err != nil {
		klog.Warningf("closing agent for session %s: %v", sessionID, err)
	}

	delete(sm.sessions, sessionID)
	return nil
}

// Close shuts down all Agents and clears the manager.  Any errors encountered
// are logged and ignored – this is intended for best-effort cleanup at
// process exit.
func (sm *SessionManager) Close() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	for id, ag := range sm.sessions {
		if err := ag.Close(); err != nil {
			klog.Warningf("closing agent for session %s: %v", id, err)
		}
	}
	sm.sessions = make(map[string]*Agent)
}

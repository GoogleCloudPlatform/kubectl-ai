package journal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Recorder is an interface for recording a structured log of the agent's actions and observations.
type Recorder interface {
	io.Closer

	// Write will add an event to the recorder.
	Write(ctx context.Context, event *Event) error
}

// FileRecorder writes a structured log of the agent's actions and observations to a file.
type FileRecorder struct {
	mu                    sync.Mutex
	f                     *os.File
	session               SessionLog
	toolCallLocations     map[string]toolCallLocation
	lastModelMessageIndex int
}

type toolCallLocation struct {
	messageIndex int
	callIndex    int
}

// NewFileRecorder creates a new FileRecorder that writes to the given file.
func NewFileRecorder(path string) (*FileRecorder, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}

	now := time.Now()
	recorder := &FileRecorder{
		f: file,
		session: SessionLog{
			SessionID:   uuid.NewString(),
			StartTime:   now,
			LastUpdated: now,
			Messages:    []Message{},
		},
		toolCallLocations:     make(map[string]toolCallLocation),
		lastModelMessageIndex: -1,
	}

	if err := recorder.flushLocked(); err != nil {
		file.Close()
		return nil, err
	}

	return recorder, nil
}

// Close closes the file.
func (r *FileRecorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.f == nil {
		return nil
	}

	err := r.flushLocked()
	closeErr := r.f.Close()
	r.f = nil
	return errors.Join(err, closeErr)
}

// Write records an event to the underlying session log and flushes it to disk.
func (r *FileRecorder) Write(ctx context.Context, event *Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.f == nil {
		return fmt.Errorf("recorder closed")
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	switch event.Action {
	case ActionSessionMetadata:
		if metadata, ok := sessionMetadataFromPayload(event.Payload); ok {
			if metadata.SessionID != "" {
				r.session.SessionID = metadata.SessionID
			}
			if metadata.ProjectHash != "" {
				r.session.ProjectHash = metadata.ProjectHash
			}
			if r.session.StartTime.IsZero() {
				r.session.StartTime = event.Timestamp
			}
		}
	case ActionSessionMessage:
		if msg, ok := messageFromPayload(event.Payload); ok {
			if msg.Timestamp.IsZero() {
				msg.Timestamp = event.Timestamp
			}
			r.session.Messages = append(r.session.Messages, msg)
			if msg.Model != "" {
				r.lastModelMessageIndex = len(r.session.Messages) - 1
			}
		}
	case ActionToolRequest:
		if call, ok := toolCallFromRequest(event.Payload); ok {
			if call.Timestamp.IsZero() {
				call.Timestamp = event.Timestamp
			}
			if call.Status == "" {
				call.Status = "requested"
			}
			r.attachToolCall(call)
		}
	case ActionToolResponse:
		if call, ok := toolCallFromResponse(event.Payload); ok {
			if call.Timestamp.IsZero() {
				call.Timestamp = event.Timestamp
			}
			r.updateToolCall(call)
		}
	}

	if !event.Timestamp.IsZero() {
		r.session.LastUpdated = event.Timestamp
		if r.session.StartTime.IsZero() {
			r.session.StartTime = event.Timestamp
		}
	}

	return r.flushLocked()
}

func (r *FileRecorder) attachToolCall(call ToolCall) {
	if call.ID == "" {
		call.ID = uuid.NewString()
	}

	messageIndex := r.lastModelMessageIndex
	if messageIndex < 0 || messageIndex >= len(r.session.Messages) {
		messageIndex = len(r.session.Messages) - 1
	}
	if messageIndex < 0 {
		return
	}

	msg := &r.session.Messages[messageIndex]
	msg.ToolCalls = append(msg.ToolCalls, call)
	r.toolCallLocations[call.ID] = toolCallLocation{
		messageIndex: messageIndex,
		callIndex:    len(msg.ToolCalls) - 1,
	}
}

func (r *FileRecorder) updateToolCall(call ToolCall) {
	if call.ID == "" {
		return
	}

	location, ok := r.toolCallLocations[call.ID]
	if !ok {
		r.attachToolCall(call)
		return
	}

	if location.messageIndex < 0 || location.messageIndex >= len(r.session.Messages) {
		return
	}

	msg := &r.session.Messages[location.messageIndex]
	if location.callIndex < 0 || location.callIndex >= len(msg.ToolCalls) {
		return
	}

	existing := &msg.ToolCalls[location.callIndex]
	mergeToolCall(existing, call)
}

func mergeToolCall(dst *ToolCall, src ToolCall) {
	if src.Name != "" {
		dst.Name = src.Name
	}
	if src.Args != nil {
		dst.Args = src.Args
	}
	if len(src.Result) > 0 {
		dst.Result = src.Result
	}
	if src.Status != "" {
		dst.Status = src.Status
	}
	if !src.Timestamp.IsZero() {
		dst.Timestamp = src.Timestamp
	}
	if src.ResultDisplay != "" {
		dst.ResultDisplay = src.ResultDisplay
	}
	if src.DisplayName != "" {
		dst.DisplayName = src.DisplayName
	}
	if src.Description != "" {
		dst.Description = src.Description
	}
	if src.RenderOutputAsMarkdown {
		dst.RenderOutputAsMarkdown = src.RenderOutputAsMarkdown
	}
}

func (r *FileRecorder) flushLocked() error {
	if r.f == nil {
		return fmt.Errorf("recorder closed")
	}

	if _, err := r.f.Seek(0, 0); err != nil {
		return fmt.Errorf("seeking recorder: %w", err)
	}

	if err := r.f.Truncate(0); err != nil {
		return fmt.Errorf("truncating recorder: %w", err)
	}

	encoder := json.NewEncoder(r.f)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(r.session); err != nil {
		return fmt.Errorf("encoding session log: %w", err)
	}

	return r.f.Sync()
}

type Event struct {
	Timestamp time.Time `json:"timestamp"`
	Action    string    `json:"action"`
	Payload   any       `json:"payload,omitempty"`
}

const (
	ActionHTTPRequest  = "http.request"
	ActionHTTPResponse = "http.response"
	ActionHTTPError    = "http.error"

	ActionSessionMessage  = "session.message"
	ActionSessionMetadata = "session.metadata"
	ActionToolRequest     = "tool-request"
	ActionToolResponse    = "tool-response"
)

// ActionUIRender is for an event that indicates we wrote output to the UI
const ActionUIRender = "ui.render"

func sessionMetadataFromPayload(payload any) (SessionMetadata, bool) {
	switch v := payload.(type) {
	case SessionMetadata:
		return v, true
	case *SessionMetadata:
		if v == nil {
			return SessionMetadata{}, false
		}
		return *v, true
	case map[string]any:
		var meta SessionMetadata
		if err := decodeMap(v, &meta); err != nil {
			return SessionMetadata{}, false
		}
		return meta, true
	default:
		return SessionMetadata{}, false
	}
}

func messageFromPayload(payload any) (Message, bool) {
	switch v := payload.(type) {
	case Message:
		return v, true
	case *Message:
		if v == nil {
			return Message{}, false
		}
		return *v, true
	case map[string]any:
		var msg Message
		if err := decodeMap(v, &msg); err != nil {
			return Message{}, false
		}
		return msg, true
	default:
		return Message{}, false
	}
}

func toolCallFromRequest(payload any) (ToolCall, bool) {
	switch v := payload.(type) {
	case ToolCall:
		return v, true
	case *ToolCall:
		if v == nil {
			return ToolCall{}, false
		}
		return *v, true
	case ToolRequestEvent:
		return ToolCall{
			ID:          v.CallID,
			Name:        v.Name,
			Args:        v.Arguments,
			Description: v.Description,
			DisplayName: v.DisplayName,
		}, true
	case *ToolRequestEvent:
		if v == nil {
			return ToolCall{}, false
		}
		return ToolCall{
			ID:          v.CallID,
			Name:        v.Name,
			Args:        v.Arguments,
			Description: v.Description,
			DisplayName: v.DisplayName,
		}, true
	case map[string]any:
		var req ToolRequestEvent
		if err := decodeMap(v, &req); err != nil {
			return ToolCall{}, false
		}
		return ToolCall{
			ID:          req.CallID,
			Name:        req.Name,
			Args:        req.Arguments,
			Description: req.Description,
			DisplayName: req.DisplayName,
		}, true
	default:
		return ToolCall{}, false
	}
}

func toolCallFromResponse(payload any) (ToolCall, bool) {
	switch v := payload.(type) {
	case ToolCall:
		return v, true
	case *ToolCall:
		if v == nil {
			return ToolCall{}, false
		}
		return *v, true
	case ToolResponseEvent:
		return toolCallFromResponseEvent(v), true
	case *ToolResponseEvent:
		if v == nil {
			return ToolCall{}, false
		}
		return toolCallFromResponseEvent(*v), true
	case map[string]any:
		var res ToolResponseEvent
		if err := decodeMap(v, &res); err != nil {
			return ToolCall{}, false
		}
		return toolCallFromResponseEvent(res), true
	default:
		return ToolCall{}, false
	}
}

func toolCallFromResponseEvent(event ToolResponseEvent) ToolCall {
	call := ToolCall{
		ID:                     event.CallID,
		ResultDisplay:          event.ResultDisplay,
		RenderOutputAsMarkdown: false,
	}

	if event.Error != "" {
		call.Status = "error"
		call.Description = event.Error
	} else {
		call.Status = "success"
	}

	if event.Response != nil {
		call.Result = []interface{}{event.Response}
	}

	if call.ResultDisplay == "" && event.Response != nil {
		switch v := event.Response.(type) {
		case string:
			call.ResultDisplay = v
		default:
			if b, err := json.Marshal(v); err == nil {
				call.ResultDisplay = string(b)
			}
		}
	}

	return call
}

func decodeMap[T any](m map[string]any, target *T) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, target)
}

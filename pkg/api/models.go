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

package api

import (
	"fmt"
	"time"
)

type Session struct {
	ID               string
	Name             string
	ProviderID       string
	ModelID          string
	Environment      string
	Messages         []*Message
	AgentState       AgentState
	CreatedAt        time.Time
	LastModified     time.Time
	ChatMessageStore ChatMessageStore
	// MCP status information
	MCPStatus *MCPStatus
}

type AgentState string

const (
	AgentStateIdle            AgentState = "idle"
	AgentStateWaitingForInput AgentState = "waiting-for-input"
	AgentStateRunning         AgentState = "running"
	AgentStateInitializing    AgentState = "initializing"
	AgentStateDone            AgentState = "done"
	AgentStateExited          AgentState = "exited"
)

type MessageType string

const (
	MessageTypeText                  MessageType = "text"
	MessageTypeError                 MessageType = "error"
	MessageTypeToolCallRequest       MessageType = "tool-call-request"
	MessageTypeToolCallResponse      MessageType = "tool-call-response"
	MessageTypeUserInputRequest      MessageType = "user-input-request"
	MessageTypeUserInputResponse     MessageType = "user-input-response"
	MessageTypeUserChoiceRequest     MessageType = "user-choice-request"
	MessageTypeUserChoiceResponse    MessageType = "user-choice-response"
	MessageTypeSessionPickerRequest  MessageType = "session-picker-request"
	MessageTypeSessionPickerResponse MessageType = "session-picker-response"
)

type Message struct {
	ID        string
	Source    MessageSource
	Type      MessageType
	Payload   any
	Timestamp time.Time
}

type MessageSource string

const (
	MessageSourceUser  MessageSource = "user"
	MessageSourceAgent MessageSource = "agent"
	MessageSourceModel MessageSource = "model"
)

type UserChoiceRequest struct {
	Prompt  string
	Options []UserChoiceOption
}

type UserChoiceOption struct {
	Label string `json:"label,omitempty"`
	Value string `json:"value,omitempty"`
}

type UserChoiceResponse struct {
	Choice int `json:"choice"`
}

type UserInputResponse struct {
	Query string `json:"query"`
}

// SessionPickerRequest is sent to show an interactive session picker
type SessionPickerRequest struct {
	Sessions []SessionInfo `json:"sessions"`
}

// SessionInfo contains display information for a session
type SessionInfo struct {
	ID           string    `json:"id"`
	Name         string    `json:"name,omitempty"`
	ModelID      string    `json:"modelId,omitempty"`
	ProviderID   string    `json:"providerId,omitempty"`
	Environment  string    `json:"environment,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	LastModified time.Time `json:"lastModified"`
	MessageCount int       `json:"messageCount"`
}

// SessionPickerResponse is sent when user selects a session
type SessionPickerResponse struct {
	SessionID string `json:"sessionId"`
	Cancelled bool   `json:"cancelled"`
}

// MCPStatus represents the overall status of MCP servers and tools
type MCPStatus struct {
	ServerInfoList []ServerConnectionInfo `json:"serverInfoList,omitempty"`
	TotalServers   int                    `json:"totalServers,omitempty"`
	ConnectedCount int                    `json:"connectedCount,omitempty"`
	FailedCount    int                    `json:"failedCount,omitempty"`
	TotalTools     int                    `json:"totalTools,omitempty"`
	ClientEnabled  bool                   `json:"clientEnabled,omitempty"`
}

// ServerConnectionInfo holds connection status for a single MCP server
type ServerConnectionInfo struct {
	Name           string    `json:"name,omitempty"`
	Command        string    `json:"command,omitempty"`
	IsLegacy       bool      `json:"isLegacy,omitempty"`
	IsConnected    bool      `json:"isConnected,omitempty"`
	AvailableTools []MCPTool `json:"availableTools,omitempty"`
}

// MCPTool represents an MCP tool with basic information
type MCPTool struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Server      string `json:"server,omitempty"`
}

// ChatMessageStore defines the interface for managing storage of chat messages of a session.
type ChatMessageStore interface {
	AddChatMessage(record *Message) error
	SetChatMessages(newHistory []*Message) error
	ChatMessages() []*Message
	ClearChatMessages() error
}

func (s *Session) AllMessages() []*Message {
	if s.ChatMessageStore == nil {
		return nil
	}
	return s.ChatMessageStore.ChatMessages()
}

func (s *Session) String() string {
	envStr := ""
	if s.Environment != "" {
		envStr = fmt.Sprintf("\nEnvironment: %s", s.Environment)
	}
	return fmt.Sprintf("Session ID: %s\nProvider: %s\nModel: %s%s\nCreated At: %s\nLast Modified: %s\nAgent State: %s",
		s.ID, s.ProviderID, s.ModelID, envStr, s.CreatedAt.Format(time.RFC3339), s.LastModified.Format(time.RFC3339), s.AgentState)
}

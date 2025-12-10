package journal

import "time"

// SessionLog captures the full timeline of a kubectl-ai session for diagnostics.
type SessionLog struct {
	SessionID   string    `json:"sessionId"`
	ProjectHash string    `json:"projectHash"`
	StartTime   time.Time `json:"startTime"`
	LastUpdated time.Time `json:"lastUpdated"`
	Messages    []Message `json:"messages"`
}

// Message records a single user, agent, or model message that occurred in a session.
type Message struct {
	ID        string     `json:"id"`
	Timestamp time.Time  `json:"timestamp"`
	Type      string     `json:"type"`
	Content   string     `json:"content"`
	Thoughts  []Thought  `json:"thoughts,omitempty"`
	Tokens    *Tokens    `json:"tokens,omitempty"`
	Model     string     `json:"model,omitempty"`
	ToolCalls []ToolCall `json:"toolCalls,omitempty"`
}

// Thought captures intermediate reasoning steps produced during a session.
type Thought struct {
	Subject     string    `json:"subject"`
	Description string    `json:"description"`
	Timestamp   time.Time `json:"timestamp"`
}

// Tokens records token usage metrics for a message.
type Tokens struct {
	Input    int `json:"input"`
	Output   int `json:"output"`
	Cached   int `json:"cached"`
	Thoughts int `json:"thoughts"`
	Tool     int `json:"tool"`
	Total    int `json:"total"`
}

// ToolCall captures the lifecycle of a single tool invocation.
type ToolCall struct {
	ID                     string        `json:"id"`
	Name                   string        `json:"name"`
	Args                   interface{}   `json:"args"`
	Result                 []interface{} `json:"result"`
	Status                 string        `json:"status"`
	Timestamp              time.Time     `json:"timestamp"`
	ResultDisplay          string        `json:"resultDisplay"`
	DisplayName            string        `json:"displayName"`
	Description            string        `json:"description"`
	RenderOutputAsMarkdown bool          `json:"renderOutputAsMarkdown"`
}

// SessionMetadata provides contextual identifiers for a session log.
type SessionMetadata struct {
	SessionID   string `json:"sessionId,omitempty"`
	ProjectHash string `json:"projectHash,omitempty"`
}

// ToolRequestEvent describes the payload captured when a tool invocation is requested.
type ToolRequestEvent struct {
	CallID      string         `json:"id,omitempty"`
	Name        string         `json:"name,omitempty"`
	Arguments   map[string]any `json:"arguments,omitempty"`
	Description string         `json:"description,omitempty"`
	DisplayName string         `json:"displayName,omitempty"`
}

// ToolResponseEvent describes the payload captured after a tool invocation completes.
type ToolResponseEvent struct {
	CallID        string `json:"id,omitempty"`
	Response      any    `json:"response,omitempty"`
	Error         string `json:"error,omitempty"`
	ResultDisplay string `json:"resultDisplay,omitempty"`
}

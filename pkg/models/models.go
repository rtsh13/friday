package models

import "time"

// Message represents a conversation message in the agent loop.
type Message struct {
	Role      string    `json:"role"` // "user", "assistant", "system", "tool"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp,omitempty"`
}

// ToolCall represents a request from the LLM to execute a tool.
type ToolCall struct {
	Name   string            `json:"name"`
	Params map[string]string `json:"params"`
}

// ToolResult represents the output of a tool execution.
type ToolResult struct {
	ToolName string        `json:"tool_name"`
	Success  bool          `json:"success"`
	Output   string        `json:"output"`
	Error    string        `json:"error,omitempty"`
	Duration time.Duration `json:"duration,omitempty"`
}

// AgentState represents the current state of agent processing.
type AgentState int

const (
	StateIdle AgentState = iota
	StateThinking
	StateToolCall
	StateToolExecuting
	StateResponding
	StateError
)

func (s AgentState) String() string {
	return [...]string{"Idle", "Thinking", "Planning tool call", "Executing tool", "Responding", "Error"}[s]
}

// AgentEvent is sent during agent processing to update the UI.
type AgentEvent struct {
	State       AgentState
	Message     string
	ToolCall    *ToolCall
	ToolResult  *ToolResult
	FinalAnswer string
	Error       error
}

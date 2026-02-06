// Package types defines shared data structures for the telemetry debugger.
package types

import "time"

// Query represents a user query with metadata.
type Query struct {
	ID        string
	Text      string
	Timestamp time.Time
}

// RetrievedChunk represents a chunk retrieved from the RAG pipeline.
type RetrievedChunk struct {
	Content  string
	Score    float64
	Source   string
	Category string
	Metadata map[string]interface{}
}

// FunctionCall represents a request to execute a function.
type FunctionCall struct {
	Name      string                 `json:"name"`
	Params    map[string]interface{} `json:"params"`
	Critical  bool                   `json:"critical"`
	DependsOn []int                  `json:"depends_on,omitempty"`
}

// LLMResponse represents the structured response from the LLM.
type LLMResponse struct {
	Reasoning         string         `json:"reasoning"`
	ExecutionStrategy string         `json:"execution_strategy"`
	Functions         []FunctionCall `json:"functions"`
	Explanation       string         `json:"explanation"`
}

// ExecutionResult holds the result of a single function execution.
type ExecutionResult struct {
	Index      int
	Function   FunctionCall
	Success    bool
	Output     string
	Error      string
	Duration   time.Duration
	RetryCount int
}

// Message represents a message in the conversation history.
type Message struct {
	Role      string            `json:"role"`
	Content   string            `json:"content"`
	Timestamp time.Time         `json:"timestamp"`
	Functions []ExecutionResult `json:"functions,omitempty"`
	Metadata  map[string]any    `json:"metadata,omitempty"`
}

// FunctionDefinition describes a function from the YAML registry.
type FunctionDefinition struct {
	Name             string                 `yaml:"name"`
	Description      string                 `yaml:"description"`
	Category         string                 `yaml:"category"`
	Phase            string                 `yaml:"phase"`
	Parameters       []ParameterDefinition  `yaml:"parameters"`
	Outputs          map[string]interface{} `yaml:"outputs"`
	TimeoutSeconds   int                    `yaml:"timeout_seconds"`
	Reversible       bool                   `yaml:"reversible"`
	Destructive      bool                   `yaml:"destructive"`
	RequiresConfirm  bool                   `yaml:"requires_confirmation"`
	RollbackFunction string                 `yaml:"rollback_function,omitempty"`
	SnapshotRequired bool                   `yaml:"snapshot_required,omitempty"`
}

// ParameterDefinition describes a function parameter.
type ParameterDefinition struct {
	Name        string      `yaml:"name"`
	Type        string      `yaml:"type"`
	Required    bool        `yaml:"required"`
	Default     interface{} `yaml:"default,omitempty"`
	Description string      `yaml:"description"`
	Validation  string      `yaml:"validation,omitempty"`
}

// AgentState represents the current state of agent processing.
type AgentState int

const (
	StateIdle AgentState = iota
	StateThinking
	StateRetrieving
	StateToolCall
	StateToolExecuting
	StateResponding
	StateError
)

// String returns a human-readable state name.
func (s AgentState) String() string {
	names := [...]string{
		"Idle",
		"Thinking",
		"Retrieving context",
		"Planning tool call",
		"Executing tool",
		"Responding",
		"Error",
	}
	if int(s) < len(names) {
		return names[s]
	}
	return "Unknown"
}

// AgentEvent is sent during agent processing to update the UI.
type AgentEvent struct {
	State       AgentState
	Message     string
	ToolCall    *FunctionCall
	ToolResult  *ExecutionResult
	AllResults  []ExecutionResult
	FinalAnswer string
	Error       error
	ChunksFound int
}

// ToolInfo contains metadata about a tool for display.
type ToolInfo struct {
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Category    string                `json:"category"`
	Parameters  []ParameterDefinition `json:"parameters"`
}

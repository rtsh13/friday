package types

import "time"

type Query struct {
	ID        string
	Text      string
	Timestamp time.Time
}

type RetrievedChunk struct {
	Content  string
	Score    float64
	Source   string
	Category string
	Metadata map[string]interface{}
}

type FunctionCall struct {
	Name      string                 `json:"name"`
	Params    map[string]interface{} `json:"params"`
	Critical  bool                   `json:"critical"`
	DependsOn []int                  `json:"depends_on"`
}

type LLMResponse struct {
	Reasoning         string         `json:"reasoning"`
	ExecutionStrategy string         `json:"execution_strategy"`
	Functions         []FunctionCall `json:"functions"`
	Explanation       string         `json:"explanation"`
}

type ExecutionResult struct {
	Index      int
	Function   FunctionCall
	Success    bool
	Output     string
	Error      string
	Duration   time.Duration
	RetryCount int
}

type Message struct {
	Role      string                 `json:"role"`
	Content   string                 `json:"content"`
	Timestamp time.Time              `json:"timestamp"`
	Functions []ExecutionResult      `json:"functions,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

type FunctionDefinition struct {
	Name            string                       `yaml:"name"`
	Description     string                       `yaml:"description"`
	Category        string                       `yaml:"category"`
	Phase           string                       `yaml:"phase"`
	Parameters      []ParameterDefinition        `yaml:"parameters"`
	Outputs         map[string]interface{}       `yaml:"outputs"`
	TimeoutSeconds  int                          `yaml:"timeout_seconds"`
	Reversible      bool                         `yaml:"reversible"`
	Destructive     bool                         `yaml:"destructive"`
	RequiresConfirm bool                         `yaml:"requires_confirmation"`
}

type ParameterDefinition struct {
	Name        string      `yaml:"name"`
	Type        string      `yaml:"type"`
	Required    bool        `yaml:"required"`
	Default     interface{} `yaml:"default"`
	Description string      `yaml:"description"`
}
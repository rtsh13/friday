// Package tools provides the tool framework for cliche.
package tools

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/stratos/cliche/pkg/models"
)

// Tool defines the interface that all tools must implement.
type Tool interface {
	// Name returns the unique identifier for this tool.
	Name() string

	// Description returns a human-readable description for the LLM.
	Description() string

	// Parameters returns the parameter schema for validation.
	Parameters() []Parameter

	// Execute runs the tool with the given parameters.
	Execute(ctx context.Context, params map[string]string) models.ToolResult
}

// Parameter defines a tool parameter with validation rules.
type Parameter struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"` // "string", "int", "bool"
	Description string   `json:"description"`
	Required    bool     `json:"required"`
	Default     string   `json:"default,omitempty"`
	Enum        []string `json:"enum,omitempty"` // Valid values if restricted
}

// Registry manages tool registration and lookup.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry creates a new tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(tool Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := tool.Name()
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool already registered: %s", name)
	}

	r.tools[name] = tool
	return nil
}

// MustRegister adds a tool to the registry, panicking on error.
func (r *Registry) MustRegister(tool Tool) {
	if err := r.Register(tool); err != nil {
		panic(err)
	}
}

// Get retrieves a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, exists := r.tools[name]
	return tool, exists
}

// List returns all registered tool names.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// All returns all registered tools.
func (r *Registry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}
	return tools
}

// ToolInfo contains metadata about a tool for the LLM prompt.
type ToolInfo struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  []Parameter `json:"parameters"`
}

// ListTools returns all registered tools with their metadata.
func (r *Registry) ListTools() []ToolInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]ToolInfo, 0, len(r.tools))
	for _, tool := range r.tools {
		infos = append(infos, ToolInfo{
			Name:        tool.Name(),
			Description: tool.Description(),
			Parameters:  tool.Parameters(),
		})
	}
	return infos
}

// Executor handles tool execution with validation and timing.
type Executor struct {
	registry *Registry
}

// NewExecutor creates a new tool executor.
func NewExecutor(registry *Registry) *Executor {
	return &Executor{registry: registry}
}

// Execute runs a tool by name with the given parameters.
func (e *Executor) Execute(ctx context.Context, toolName string, params map[string]string) models.ToolResult {
	start := time.Now()

	tool, exists := e.registry.Get(toolName)
	if !exists {
		return models.ToolResult{
			ToolName: toolName,
			Success:  false,
			Error:    fmt.Sprintf("unknown tool: %s", toolName),
			Duration: time.Since(start),
		}
	}

	// Validate parameters
	if err := e.validateParams(tool, params); err != nil {
		return models.ToolResult{
			ToolName: toolName,
			Success:  false,
			Error:    fmt.Sprintf("validation failed: %v", err),
			Duration: time.Since(start),
		}
	}

	// Apply defaults
	params = e.applyDefaults(tool, params)

	// Execute
	result := tool.Execute(ctx, params)
	result.ToolName = toolName
	result.Duration = time.Since(start)

	return result
}

// validateParams checks required parameters and enum values.
func (e *Executor) validateParams(tool Tool, params map[string]string) error {
	for _, def := range tool.Parameters() {
		value, exists := params[def.Name]

		if def.Required && !exists {
			return fmt.Errorf("missing required parameter: %s", def.Name)
		}

		if exists && len(def.Enum) > 0 {
			valid := false
			for _, allowed := range def.Enum {
				if value == allowed {
					valid = true
					break
				}
			}
			if !valid {
				return fmt.Errorf("invalid value for %s: must be one of %v", def.Name, def.Enum)
			}
		}
	}
	return nil
}

// applyDefaults fills in default values for missing optional parameters.
func (e *Executor) applyDefaults(tool Tool, params map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range params {
		result[k] = v
	}

	for _, def := range tool.Parameters() {
		if _, exists := result[def.Name]; !exists && def.Default != "" {
			result[def.Name] = def.Default
		}
	}

	return result
}

// GenerateToolsPrompt creates the tools description for the LLM system prompt.
func (r *Registry) GenerateToolsPrompt() string {
	tools := r.ListTools()
	if len(tools) == 0 {
		return ""
	}

	prompt := `You have access to the following tools to help diagnose issues:

`
	for _, tool := range tools {
		prompt += fmt.Sprintf("### %s\n%s\n", tool.Name, tool.Description)
		if len(tool.Parameters) > 0 {
			prompt += "Parameters:\n"
			for _, p := range tool.Parameters {
				req := ""
				if p.Required {
					req = " (required)"
				}
				prompt += fmt.Sprintf("  - %s: %s%s\n", p.Name, p.Description, req)
				if p.Default != "" {
					prompt += fmt.Sprintf("    Default: %s\n", p.Default)
				}
			}
		}
		prompt += "\n"
	}

	prompt += `To use a tool, respond with ONLY a JSON object in this exact format:
{"tool": "tool_name", "params": {"param1": "value1"}}

Important:
- Output ONLY the JSON when using a tool, no other text
- Use one tool at a time
- If you don't need a tool, respond normally with helpful text
- After receiving tool output, interpret the results for the user`

	return prompt
}

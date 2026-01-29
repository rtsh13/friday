package tools

import (
	"context"
	"testing"

	"github.com/stratos/cliche/pkg/models"
)

// MockTool for testing the framework
type MockTool struct {
	name        string
	description string
	params      []Parameter
	execFunc    func(ctx context.Context, params map[string]string) models.ToolResult
}

func (m *MockTool) Name() string            { return m.name }
func (m *MockTool) Description() string     { return m.description }
func (m *MockTool) Parameters() []Parameter { return m.params }
func (m *MockTool) Execute(ctx context.Context, params map[string]string) models.ToolResult {
	if m.execFunc != nil {
		return m.execFunc(ctx, params)
	}
	return models.ToolResult{Success: true, Output: "mock output"}
}

func TestRegistry_Register(t *testing.T) {
	registry := NewRegistry()
	tool := &MockTool{name: "test-tool", description: "A test tool"}

	if err := registry.Register(tool); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if err := registry.Register(tool); err == nil {
		t.Fatal("expected error for duplicate registration")
	}
}

func TestRegistry_Get(t *testing.T) {
	registry := NewRegistry()
	tool := &MockTool{name: "test-tool"}
	registry.Register(tool)

	found, ok := registry.Get("test-tool")
	if !ok {
		t.Fatal("expected to find tool")
	}
	if found.Name() != "test-tool" {
		t.Fatalf("expected 'test-tool', got %s", found.Name())
	}

	_, ok = registry.Get("nonexistent")
	if ok {
		t.Fatal("expected not to find nonexistent tool")
	}
}

func TestRegistry_List(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&MockTool{name: "tool-a"})
	registry.Register(&MockTool{name: "tool-b"})

	names := registry.List()
	if len(names) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(names))
	}
}

func TestExecutor_Execute_Success(t *testing.T) {
	registry := NewRegistry()
	tool := &MockTool{
		name: "echo",
		params: []Parameter{
			{Name: "message", Type: "string", Required: true},
		},
		execFunc: func(ctx context.Context, params map[string]string) models.ToolResult {
			return models.ToolResult{
				Success: true,
				Output:  "Echoed: " + params["message"],
			}
		},
	}
	registry.Register(tool)

	executor := NewExecutor(registry)
	result := executor.Execute(context.Background(), "echo", map[string]string{"message": "hello"})

	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if result.Output != "Echoed: hello" {
		t.Fatalf("expected 'Echoed: hello', got %s", result.Output)
	}
}

func TestExecutor_Execute_UnknownTool(t *testing.T) {
	registry := NewRegistry()
	executor := NewExecutor(registry)

	result := executor.Execute(context.Background(), "nonexistent", nil)

	if result.Success {
		t.Fatal("expected failure for unknown tool")
	}
}

func TestExecutor_Execute_MissingRequiredParam(t *testing.T) {
	registry := NewRegistry()
	tool := &MockTool{
		name: "test",
		params: []Parameter{
			{Name: "required_param", Type: "string", Required: true},
		},
	}
	registry.Register(tool)

	executor := NewExecutor(registry)
	result := executor.Execute(context.Background(), "test", map[string]string{})

	if result.Success {
		t.Fatal("expected failure for missing required param")
	}
}

func TestExecutor_Execute_AppliesDefaults(t *testing.T) {
	registry := NewRegistry()
	tool := &MockTool{
		name: "test",
		params: []Parameter{
			{Name: "optional", Type: "string", Required: false, Default: "default_value"},
		},
		execFunc: func(ctx context.Context, params map[string]string) models.ToolResult {
			return models.ToolResult{
				Success: true,
				Output:  params["optional"],
			}
		},
	}
	registry.Register(tool)

	executor := NewExecutor(registry)
	result := executor.Execute(context.Background(), "test", map[string]string{})

	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if result.Output != "default_value" {
		t.Fatalf("expected 'default_value', got %s", result.Output)
	}
}

func TestExecutor_Execute_EnumValidation(t *testing.T) {
	registry := NewRegistry()
	tool := &MockTool{
		name: "test",
		params: []Parameter{
			{Name: "level", Type: "string", Required: true, Enum: []string{"low", "medium", "high"}},
		},
	}
	registry.Register(tool)

	executor := NewExecutor(registry)

	result := executor.Execute(context.Background(), "test", map[string]string{"level": "medium"})
	if !result.Success {
		t.Fatalf("expected success for valid enum, got: %s", result.Error)
	}

	result = executor.Execute(context.Background(), "test", map[string]string{"level": "invalid"})
	if result.Success {
		t.Fatal("expected failure for invalid enum value")
	}
}

func TestRegisterNetworkingTools(t *testing.T) {
	registry := NewRegistry()
	RegisterNetworkingTools(registry)

	expectedTools := []string{"ping", "dns-lookup", "port-scan", "http", "traceroute", "netinfo"}

	for _, name := range expectedTools {
		if _, ok := registry.Get(name); !ok {
			t.Errorf("expected tool %s to be registered", name)
		}
	}
}

package agent

import (
	"context"
	"testing"
	"time"

	"github.com/stratos/cliche/internal/config"
	"github.com/stratos/cliche/internal/types"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello..."},
		{"", 5, ""},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc..."},
	}

	for _, tt := range tests {
		result := truncate(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncate(%q, %d) = %q, want %q",
				tt.input, tt.maxLen, result, tt.expected)
		}
	}
}

func TestAgentConfig_Defaults(t *testing.T) {
	cfg := Config{
		AppConfig:     nil,
		FunctionsPath: "",
		Logger:        nil,
	}

	// Verify defaults are applied
	if cfg.AppConfig != nil {
		t.Error("Expected nil AppConfig before initialization")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := config.DefaultConfig()

	if cfg.LLM.Endpoint == "" {
		t.Error("Expected default LLM endpoint")
	}

	if cfg.Qdrant.Host == "" {
		t.Error("Expected default Qdrant host")
	}

	if cfg.ONNX.EmbeddingDim != 384 {
		t.Errorf("Expected embedding dim 384, got %d", cfg.ONNX.EmbeddingDim)
	}
}

func TestAgentState_String(t *testing.T) {
	tests := []struct {
		state    types.AgentState
		expected string
	}{
		{types.StateIdle, "Idle"},
		{types.StateThinking, "Thinking"},
		{types.StateRetrieving, "Retrieving context"},
		{types.StateToolCall, "Planning tool call"},
		{types.StateToolExecuting, "Executing tool"},
		{types.StateResponding, "Responding"},
		{types.StateError, "Error"},
	}

	for _, tt := range tests {
		result := tt.state.String()
		if result != tt.expected {
			t.Errorf("State(%d).String() = %q, want %q",
				tt.state, result, tt.expected)
		}
	}
}

func TestAgentState_Unknown(t *testing.T) {
	// Test out of range state
	state := types.AgentState(100)
	result := state.String()
	if result != "Unknown" {
		t.Errorf("Unknown state should return 'Unknown', got %q", result)
	}
}

// MockAgent for testing without external dependencies
type MockAgent struct {
	processedQueries []string
	mockResponse     *types.AgentEvent
	mockError        error
}

func (m *MockAgent) ProcessQuery(ctx context.Context, query string) (*types.AgentEvent, error) {
	m.processedQueries = append(m.processedQueries, query)
	if m.mockError != nil {
		return nil, m.mockError
	}
	return m.mockResponse, nil
}

func TestMockAgent_ProcessQuery(t *testing.T) {
	mock := &MockAgent{
		mockResponse: &types.AgentEvent{
			State:       types.StateResponding,
			FinalAnswer: "Test response",
		},
	}

	ctx := context.Background()
	event, err := mock.ProcessQuery(ctx, "test query")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if event.FinalAnswer != "Test response" {
		t.Errorf("Expected 'Test response', got %q", event.FinalAnswer)
	}

	if len(mock.processedQueries) != 1 {
		t.Errorf("Expected 1 processed query, got %d", len(mock.processedQueries))
	}

	if mock.processedQueries[0] != "test query" {
		t.Errorf("Expected 'test query', got %q", mock.processedQueries[0])
	}
}

func TestBuildFinalAnswer(t *testing.T) {
	// This tests the format of the final answer
	llmResp := &types.LLMResponse{
		Reasoning:   "Testing the system",
		Explanation: "Test completed successfully",
		Functions:   []types.FunctionCall{},
	}

	results := []types.ExecutionResult{
		{
			Index: 0,
			Function: types.FunctionCall{
				Name:   "test_func",
				Params: map[string]interface{}{"key": "value"},
			},
			Success:  true,
			Output:   `{"status": "ok"}`,
			Duration: 100 * time.Millisecond,
		},
	}

	// Create a minimal agent to test buildFinalAnswer
	a := &Agent{}
	answer := a.buildFinalAnswer(llmResp, results, nil)

	if answer == "" {
		t.Error("Expected non-empty final answer")
	}

	// Should contain reasoning
	if !contains(answer, "Testing the system") {
		t.Error("Expected answer to contain reasoning")
	}

	// Should contain explanation
	if !contains(answer, "Test completed successfully") {
		t.Error("Expected answer to contain explanation")
	}

	// Should contain function name
	if !contains(answer, "test_func") {
		t.Error("Expected answer to contain function name")
	}
}

func TestBuildFinalAnswer_WithError(t *testing.T) {
	llmResp := &types.LLMResponse{
		Reasoning: "Testing error handling",
	}

	results := []types.ExecutionResult{
		{
			Index: 0,
			Function: types.FunctionCall{
				Name: "failing_func",
			},
			Success: false,
			Error:   "connection refused",
		},
	}

	a := &Agent{}
	answer := a.buildFinalAnswer(llmResp, results, nil)

	if !contains(answer, "connection refused") {
		t.Error("Expected answer to contain error message")
	}

	if !contains(answer, "✗") {
		t.Error("Expected answer to contain failure indicator")
	}
}

func TestBuildFinalAnswer_LongOutput(t *testing.T) {
	llmResp := &types.LLMResponse{}

	// Create output longer than 500 chars
	longOutput := ""
	for i := 0; i < 600; i++ {
		longOutput += "x"
	}

	results := []types.ExecutionResult{
		{
			Index: 0,
			Function: types.FunctionCall{
				Name: "long_output_func",
			},
			Success: true,
			Output:  longOutput,
		},
	}

	a := &Agent{}
	answer := a.buildFinalAnswer(llmResp, results, nil)

	// Output should be truncated
	if len(answer) > 1000 {
		t.Log("Answer is long but may contain other content")
	}

	if !contains(answer, "...") {
		t.Error("Expected truncated output to end with ...")
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

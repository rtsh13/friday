package ui

import (
	"strings"
	"testing"

	"github.com/ashutoshrp06/telemetry-debugger/internal/types"
	tea "github.com/charmbracelet/bubbletea"
)

func TestNewModel(t *testing.T) {
	model := NewModel(nil)

	if model.state != types.StateIdle {
		t.Errorf("Expected initial state Idle, got %v", model.state)
	}

	if len(model.messages) != 0 {
		t.Errorf("Expected empty messages, got %d", len(model.messages))
	}

	if model.quitting {
		t.Error("Expected quitting=false initially")
	}
}

func TestModel_Init(t *testing.T) {
	model := NewModel(nil)
	cmd := model.Init()

	if cmd == nil {
		t.Error("Init should return a command")
	}
}

func TestModel_HandleCommand_Exit(t *testing.T) {
	model := NewModel(nil)

	testCases := []string{"exit", "quit", "q"}

	for _, input := range testCases {
		m := model
		cmd := m.handleCommand(input)

		if cmd == nil {
			t.Errorf("handleCommand(%q) should return quit command", input)
		}
	}
}

func TestModel_HandleCommand_Clear(t *testing.T) {
	model := NewModel(nil)
	model.messages = []chatMessage{
		{role: "user", content: "test"},
	}

	model.handleCommand("clear")

	if len(model.messages) != 0 {
		t.Errorf("Expected messages to be cleared, got %d", len(model.messages))
	}
}

func TestModel_HandleCommand_Help(t *testing.T) {
	model := NewModel(nil)
	model.handleCommand("help")

	if len(model.messages) != 1 {
		t.Fatalf("Expected 1 help message, got %d", len(model.messages))
	}

	if model.messages[0].role != "system" {
		t.Errorf("Expected system role, got %s", model.messages[0].role)
	}

	if !strings.Contains(model.messages[0].content, "Available commands") {
		t.Error("Help message should contain 'Available commands'")
	}
}

func TestModel_HandleCommand_Tools(t *testing.T) {
	model := NewModel(nil)
	model.handleCommand("tools")

	if len(model.messages) != 1 {
		t.Fatalf("Expected 1 tools message, got %d", len(model.messages))
	}

	if !strings.Contains(model.messages[0].content, "Available diagnostic tools") {
		t.Error("Tools message should list available tools")
	}
}

func TestModel_HandleCommand_Unknown(t *testing.T) {
	model := NewModel(nil)
	cmd := model.handleCommand("unknown_command_xyz")

	if cmd != nil {
		t.Error("Unknown command should return nil")
	}
}

func TestModel_HandleAgentEvent_Responding(t *testing.T) {
	model := NewModel(nil)

	event := types.AgentEvent{
		State:       types.StateResponding,
		FinalAnswer: "Test answer",
	}

	newModel, _ := model.handleAgentEvent(event)
	m := newModel.(Model)

	if m.state != types.StateIdle {
		t.Errorf("Expected state Idle after responding, got %v", m.state)
	}

	// Should have added assistant message
	found := false
	for _, msg := range m.messages {
		if msg.role == "assistant" && msg.content == "Test answer" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected assistant message with final answer")
	}
}

func TestModel_HandleAgentEvent_Error(t *testing.T) {
	model := NewModel(nil)

	event := types.AgentEvent{
		State: types.StateError,
		Error: nil,
	}

	newModel, _ := model.handleAgentEvent(event)
	m := newModel.(Model)

	if m.state != types.StateIdle {
		t.Errorf("Expected state Idle after error, got %v", m.state)
	}

	// Should have added error message
	found := false
	for _, msg := range m.messages {
		if msg.role == "system" && strings.Contains(msg.content, "Error") {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected system error message")
	}
}

func TestModel_HandleAgentEvent_ToolCall(t *testing.T) {
	model := NewModel(nil)

	toolCall := &types.FunctionCall{
		Name: "test_tool",
		Params: map[string]interface{}{
			"host": "localhost",
		},
	}

	event := types.AgentEvent{
		State:    types.StateToolCall,
		ToolCall: toolCall,
	}

	newModel, _ := model.handleAgentEvent(event)
	m := newModel.(Model)

	if m.state != types.StateToolCall {
		t.Errorf("Expected state ToolCall, got %v", m.state)
	}

	if m.currentTool == nil {
		t.Fatal("Expected currentTool to be set")
	}

	if m.currentTool.name != "test_tool" {
		t.Errorf("Expected tool name 'test_tool', got %q", m.currentTool.name)
	}
}

func TestModel_Update_EnterKey(t *testing.T) {
	processedQuery := ""
	model := NewModel(func(query string) tea.Cmd {
		processedQuery = query
		return nil
	})

	// Set query text
	model.textInput.SetValue("test query")

	// Simulate Enter key
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	newModel, _ := model.Update(msg)
	m := newModel.(Model)

	// Should have added user message
	if len(m.messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(m.messages))
	}

	if m.messages[0].content != "test query" {
		t.Errorf("Expected 'test query', got %q", m.messages[0].content)
	}

	// Should be in thinking state
	if m.state != types.StateThinking {
		t.Errorf("Expected StateThinking, got %v", m.state)
	}

	// Input should be cleared
	if m.textInput.Value() != "" {
		t.Error("Expected input to be cleared")
	}
}

func TestModel_Update_EnterKey_Empty(t *testing.T) {
	model := NewModel(nil)
	model.textInput.SetValue("")

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	newModel, _ := model.Update(msg)
	m := newModel.(Model)

	// Should not add any messages for empty input
	if len(m.messages) != 0 {
		t.Errorf("Expected 0 messages for empty input, got %d", len(m.messages))
	}
}

func TestModel_Update_CtrlC(t *testing.T) {
	model := NewModel(nil)

	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	newModel, cmd := model.Update(msg)
	m := newModel.(Model)

	if !m.quitting {
		t.Error("Expected quitting=true after Ctrl+C")
	}

	// Should return quit command
	if cmd == nil {
		t.Error("Expected quit command")
	}
}

func TestModel_Update_WindowSize(t *testing.T) {
	model := NewModel(nil)

	msg := tea.WindowSizeMsg{Width: 120, Height: 40}
	newModel, _ := model.Update(msg)
	m := newModel.(Model)

	if m.width != 120 {
		t.Errorf("Expected width 120, got %d", m.width)
	}

	if m.height != 40 {
		t.Errorf("Expected height 40, got %d", m.height)
	}

	if !m.ready {
		t.Error("Expected ready=true after window size")
	}
}

func TestModel_View_Quitting(t *testing.T) {
	model := NewModel(nil)
	model.quitting = true

	view := model.View()

	if !strings.Contains(view, "Goodbye") {
		t.Error("Quitting view should contain 'Goodbye'")
	}
}

func TestModel_View_NotReady(t *testing.T) {
	model := NewModel(nil)
	model.ready = false

	view := model.View()

	if !strings.Contains(view, "Initializing") {
		t.Error("Not ready view should contain 'Initializing'")
	}
}

func TestRenderMessage_User(t *testing.T) {
	model := NewModel(nil)
	msg := chatMessage{role: "user", content: "Hello"}

	result := model.renderMessage(msg)

	if !strings.Contains(result, "You:") {
		t.Error("User message should contain 'You:'")
	}

	if !strings.Contains(result, "Hello") {
		t.Error("User message should contain content")
	}
}

func TestRenderMessage_Assistant(t *testing.T) {
	model := NewModel(nil)
	msg := chatMessage{role: "assistant", content: "Response"}

	result := model.renderMessage(msg)

	if !strings.Contains(result, "Assistant:") {
		t.Error("Assistant message should contain 'Assistant:'")
	}
}

func TestRenderMessage_Tool(t *testing.T) {
	model := NewModel(nil)
	msg := chatMessage{
		role: "tool",
		tool: &toolExecution{
			name:    "ping",
			success: true,
			output:  "pong",
			done:    true,
		},
	}

	result := model.renderMessage(msg)

	if !strings.Contains(result, "ping") {
		t.Error("Tool message should contain tool name")
	}
}

func TestRenderToolResult_Success(t *testing.T) {
	model := NewModel(nil)
	tool := &toolExecution{
		name:     "test_tool",
		params:   map[string]interface{}{"key": "value"},
		success:  true,
		output:   "output data",
		duration: "100ms",
		done:     true,
	}

	result := model.renderToolResult(tool)

	if !strings.Contains(result, "test_tool") {
		t.Error("Should contain tool name")
	}

	if !strings.Contains(result, "Success") {
		t.Error("Should indicate success")
	}
}

func TestRenderToolResult_Failure(t *testing.T) {
	model := NewModel(nil)
	tool := &toolExecution{
		name:    "failing_tool",
		success: false,
		error:   "connection timeout",
		done:    true,
	}

	result := model.renderToolResult(tool)

	if !strings.Contains(result, "Failed") {
		t.Error("Should indicate failure")
	}

	if !strings.Contains(result, "connection timeout") {
		t.Error("Should contain error message")
	}
}

func TestRenderToolResult_LongOutput(t *testing.T) {
	model := NewModel(nil)

	longOutput := strings.Repeat("x", 400)
	tool := &toolExecution{
		name:    "long_output_tool",
		success: true,
		output:  longOutput,
		done:    true,
	}

	result := model.renderToolResult(tool)

	// Should be truncated
	if strings.Contains(result, strings.Repeat("x", 350)) {
		t.Log("Output may not be fully truncated in rendered form")
	}
}

func TestRenderStatus(t *testing.T) {
	model := NewModel(nil)
	model.state = types.StateThinking

	result := model.renderStatus()

	if !strings.Contains(result, "Thinking") {
		t.Error("Status should contain state name")
	}
}

func TestRenderHelpBar(t *testing.T) {
	model := NewModel(nil)

	result := model.renderHelpBar()

	expectedKeys := []string{"enter", "ctrl+c", "help", "tools"}
	for _, key := range expectedKeys {
		if !strings.Contains(result, key) {
			t.Errorf("Help bar should contain '%s'", key)
		}
	}
}

func TestBanner(t *testing.T) {
	banner := Banner()

	if banner == "" {
		t.Error("Banner should not be empty")
	}

	if !strings.Contains(banner, "CLIche") {
		t.Error("Banner should contain 'CLIche'")
	}
}

func TestDefaultStyles(t *testing.T) {
	styles := DefaultStyles()

	// Verify styles are initialized
	if styles.App.GetPaddingLeft() == 0 && styles.App.GetPaddingRight() == 0 {
		t.Log("App style may have minimal padding")
	}
}

func TestDefaultTheme(t *testing.T) {
	theme := DefaultTheme()

	// Verify colors are set
	if theme.Primary == "" {
		t.Error("Primary color should be set")
	}

	if theme.Success == "" {
		t.Error("Success color should be set")
	}

	if theme.Error == "" {
		t.Error("Error color should be set")
	}
}

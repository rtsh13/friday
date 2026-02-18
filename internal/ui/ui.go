// Package ui provides the terminal user interface using Bubble Tea.
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stratos/cliche/internal/types"
)

// Model is the Bubble Tea model for the telemetry debugger UI.
type Model struct {
	// UI Components
	textInput textinput.Model
	spinner   spinner.Model
	viewport  viewport.Model
	styles    Styles

	// State
	state       types.AgentState
	messages    []chatMessage
	currentTool *toolExecution
	width       int
	height      int
	ready       bool
	quitting    bool
	err         error

	// Agent interface (injected)
	processQuery func(query string) tea.Cmd
}

// chatMessage represents a message in the chat history.
type chatMessage struct {
	role    string // "user", "assistant", "system", "tool"
	content string
	tool    *toolExecution
}

// toolExecution tracks a tool call and its result.
type toolExecution struct {
	name     string
	params   map[string]interface{}
	output   string
	success  bool
	error    string
	duration string
	done     bool
}

// fixed UI line counts used to compute viewport height
const (
	bannerLines = 13 // banner height including padding
	inputLines  = 2  // prompt line + blank line above it
	helpLines   = 1  // help bar
)

// NewModel creates a new UI model.
func NewModel(processQuery func(query string) tea.Cmd) Model {
	ti := textinput.New()
	ti.Placeholder = "Describe your issue... (e.g., 'Check if gRPC service on port 50051 is healthy')"
	ti.Focus()
	ti.CharLimit = 500
	ti.Width = 80

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C3AED"))

	return Model{
		textInput:    ti,
		spinner:      s,
		styles:       DefaultStyles(),
		state:        types.StateIdle,
		messages:     make([]chatMessage, 0),
		processQuery: processQuery,
	}
}

// Init initializes the model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		m.spinner.Tick,
	)
}

// Update handles messages and updates the model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			if m.state == types.StateIdle {
				m.quitting = true
				return m, tea.Quit
			}
			// Cancel current operation
			m.state = types.StateIdle
			return m, nil

		case tea.KeyEnter:
			if m.state != types.StateIdle {
				return m, nil
			}

			query := strings.TrimSpace(m.textInput.Value())
			if query == "" {
				return m, nil
			}

			// Handle special commands
			if cmd := m.handleCommand(query); cmd != nil {
				m.updateViewportContent()
				return m, cmd
			}

			// Add user message to chat
			m.messages = append(m.messages, chatMessage{
				role:    "user",
				content: query,
			})

			// Clear input and start processing
			m.textInput.SetValue("")
			m.state = types.StateThinking
			m.updateViewportContent()

			// Trigger agent processing
			if m.processQuery != nil {
				cmds = append(cmds, m.processQuery(query))
			}

			return m, tea.Batch(cmds...)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textInput.Width = msg.Width - 10

		vpHeight := m.height - bannerLines - inputLines - helpLines
		if vpHeight < 1 {
			vpHeight = 1
		}

		if !m.ready {
			m.viewport = viewport.New(msg.Width, vpHeight)
			m.viewport.SetContent("")
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = vpHeight
		}
		m.updateViewportContent()

	case types.AgentEvent:
		newModel, cmd := m.handleAgentEvent(msg)
		m2 := newModel.(Model)
		m2.updateViewportContent()
		// Auto-scroll to bottom on new content
		m2.viewport.GotoBottom()
		return m2, cmd

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case errMsg:
		m.err = msg.err
		m.state = types.StateError
	}

	// Always forward key events to viewport (handles PgUp/PgDn/Up/Down for scrolling)
	if m.ready {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Update text input only when idle
	if m.state == types.StateIdle {
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// updateViewportContent re-renders all chat messages into the viewport.
func (m *Model) updateViewportContent() {
	if !m.ready {
		return
	}
	var b strings.Builder
	for _, msg := range m.messages {
		b.WriteString(m.renderMessage(msg))
		b.WriteString("\n")
	}
	// Show in-progress tool if any
	if m.currentTool != nil && !m.currentTool.done {
		b.WriteString(m.renderToolInProgress())
		b.WriteString("\n")
	}
	// Show status line inside viewport when processing
	if m.state != types.StateIdle {
		b.WriteString(m.renderStatus())
		b.WriteString("\n")
	}
	m.viewport.SetContent(b.String())
}

// errMsg wraps errors.
type errMsg struct{ err error }

// handleCommand processes special commands.
func (m *Model) handleCommand(input string) tea.Cmd {
	switch strings.ToLower(input) {
	case "exit", "quit", "q":
		m.quitting = true
		return tea.Quit

	case "clear":
		m.messages = make([]chatMessage, 0)
		m.textInput.SetValue("")
		return nil

	case "help", "?":
		m.messages = append(m.messages, chatMessage{
			role: "system",
			content: `Available commands:
  help, ?     Show this help
  clear       Clear chat history  
  exit, quit  Exit the debugger

Example queries:
  "Check if gRPC service on port 50051 is healthy"
  "Analyze TCP connection on eth0 port 8080"
  "Ping google.com"
  "Inspect network buffer settings"`,
		})
		m.textInput.SetValue("")
		return nil

	case "tools":
		m.messages = append(m.messages, chatMessage{
			role: "system",
			content: `Available diagnostic tools:
  
  Basic Network:
    ping, dns_lookup, port_scan, http_request, traceroute, netinfo
  
  TCP/gRPC:
    check_tcp_health, check_grpc_health, analyze_grpc_stream
  
  System:
    inspect_network_buffers, execute_sysctl_command
  
  Debugging:
    analyze_core_dump, analyze_memory_leak`,
		})
		m.textInput.SetValue("")
		return nil
	}

	return nil
}

// handleAgentEvent processes events from the agent.
func (m Model) handleAgentEvent(event types.AgentEvent) (tea.Model, tea.Cmd) {
	m.state = event.State

	switch event.State {
	case types.StateToolCall:
		if event.ToolCall != nil {
			m.currentTool = &toolExecution{
				name:   event.ToolCall.Name,
				params: event.ToolCall.Params,
			}
		}

	case types.StateToolExecuting:
		// Tool is running, spinner shows progress

	case types.StateResponding:
		// Handle tool results
		if event.ToolResult != nil && m.currentTool != nil {
			m.currentTool.success = event.ToolResult.Success
			m.currentTool.output = event.ToolResult.Output
			m.currentTool.error = event.ToolResult.Error
			m.currentTool.duration = event.ToolResult.Duration.String()
			m.currentTool.done = true

			// Add tool execution to messages
			m.messages = append(m.messages, chatMessage{
				role: "tool",
				tool: m.currentTool,
			})
			m.currentTool = nil
		}

		// Handle multiple results
		for _, result := range event.AllResults {
			// Skip if already added
			if event.ToolResult != nil && result.Index == event.ToolResult.Index {
				continue
			}
			tool := &toolExecution{
				name:     result.Function.Name,
				params:   result.Function.Params,
				success:  result.Success,
				output:   result.Output,
				error:    result.Error,
				duration: result.Duration.String(),
				done:     true,
			}
			m.messages = append(m.messages, chatMessage{
				role: "tool",
				tool: tool,
			})
		}

		// Add final answer
		if event.FinalAnswer != "" {
			m.messages = append(m.messages, chatMessage{
				role:    "assistant",
				content: event.FinalAnswer,
			})
		}
		m.state = types.StateIdle

	case types.StateError:
		m.err = event.Error
		errMsg := "An error occurred"
		if event.Error != nil {
			errMsg = event.Error.Error()
		}
		m.messages = append(m.messages, chatMessage{
			role:    "system",
			content: fmt.Sprintf("Error: %s", errMsg),
		})
		m.state = types.StateIdle
	}

	return m, m.spinner.Tick
}

// View renders the UI.
func (m Model) View() string {
	if m.quitting {
		return m.styles.SystemMessage.Render("Goodbye!\n")
	}

	if !m.ready {
		return "Initializing..."
	}

	var b strings.Builder

	// Banner
	b.WriteString(m.styles.BannerTitle.Render(Banner()))
	b.WriteString("\n\n")

	// Scrollable chat history via viewport
	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	// Input
	b.WriteString(m.styles.Prompt.Render("> "))
	if m.state == types.StateIdle {
		b.WriteString(m.textInput.View())
	} else {
		b.WriteString(m.styles.StatusText.Render("(processing...)"))
	}
	b.WriteString("\n")

	// Help bar
	b.WriteString(m.renderHelpBar())

	return m.styles.App.Render(b.String())
}

// renderMessage renders a single chat message.
func (m Model) renderMessage(msg chatMessage) string {
	switch msg.role {
	case "user":
		return m.styles.UserMessage.Render("You: " + msg.content)

	case "assistant":
		return m.styles.AssistantMessage.Render("Assistant: " + msg.content)

	case "system":
		return m.styles.SystemMessage.Render(msg.content)

	case "tool":
		if msg.tool != nil {
			return m.renderToolResult(msg.tool)
		}
	}
	return ""
}

// renderToolResult renders a completed tool execution.
func (m Model) renderToolResult(t *toolExecution) string {
	var b strings.Builder

	// Tool header
	header := fmt.Sprintf("Tool: %s", t.name)
	b.WriteString(m.styles.ToolName.Render(header))

	// Parameters (compact)
	if len(t.params) > 0 {
		params := make([]string, 0, len(t.params))
		for k, v := range t.params {
			params = append(params, fmt.Sprintf("%s=%v", k, v))
		}
		b.WriteString(" ")
		b.WriteString(m.styles.ToolParams.Render("(" + strings.Join(params, ", ") + ")"))
	}
	b.WriteString("\n")

	// Result
	if t.success {
		b.WriteString(m.styles.ToolSuccess.Render("  Success"))
		if t.duration != "" && t.duration != "0s" {
			b.WriteString(m.styles.ToolParams.Render(fmt.Sprintf(" (%s)", t.duration)))
		}
		b.WriteString("\n")
		if t.output != "" {
			// Indent and truncate output
			output := t.output
			if len(output) > 300 {
				output = output[:300] + "..."
			}
			lines := strings.Split(output, "\n")
			for _, line := range lines {
				if line != "" {
					b.WriteString(m.styles.ToolOutput.Render("  | " + line))
					b.WriteString("\n")
				}
			}
		}
	} else {
		b.WriteString(m.styles.ToolError.Render("  Failed: " + t.error))
		b.WriteString("\n")
	}

	return m.styles.ToolBox.Render(b.String())
}

// renderToolInProgress renders a tool that's currently executing.
func (m Model) renderToolInProgress() string {
	var b strings.Builder

	header := fmt.Sprintf("Tool: %s", m.currentTool.name)
	b.WriteString(m.styles.ToolName.Render(header))

	if len(m.currentTool.params) > 0 {
		params := make([]string, 0, len(m.currentTool.params))
		for k, v := range m.currentTool.params {
			params = append(params, fmt.Sprintf("%s=%v", k, v))
		}
		b.WriteString(" ")
		b.WriteString(m.styles.ToolParams.Render("(" + strings.Join(params, ", ") + ")"))
	}
	b.WriteString("\n")
	b.WriteString(m.spinner.View())
	b.WriteString(" ")
	b.WriteString(m.styles.StatusText.Render("Executing..."))

	return m.styles.ToolBox.Render(b.String())
}

// renderStatus renders the current processing status.
func (m Model) renderStatus() string {
	return fmt.Sprintf("%s %s",
		m.spinner.View(),
		m.styles.StateLabel.Render(m.state.String()+"..."),
	)
}

// renderHelpBar renders the bottom help bar.
func (m Model) renderHelpBar() string {
	help := []string{
		m.styles.HelpKey.Render("enter") + m.styles.HelpValue.Render(" send"),
		m.styles.HelpKey.Render("ctrl+c") + m.styles.HelpValue.Render(" quit"),
		m.styles.HelpKey.Render("↑/↓ pgup/pgdn") + m.styles.HelpValue.Render(" scroll"),
		m.styles.HelpKey.Render("help") + m.styles.HelpValue.Render(" commands"),
	}
	return m.styles.HelpBar.Render(strings.Join(help, "  |  "))
}
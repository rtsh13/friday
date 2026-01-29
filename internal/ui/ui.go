// Package ui provides the terminal user interface using Bubble Tea.
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stratos/cliche/pkg/models"
)

// Model is the Bubble Tea model for the cliche UI.
type Model struct {
	// UI Components
	textInput textinput.Model
	spinner   spinner.Model
	styles    Styles

	// State
	state       models.AgentState
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
	role    string // "user", "assistant", "system"
	content string
	tool    *toolExecution
}

// toolExecution tracks a tool call and its result.
type toolExecution struct {
	name     string
	params   map[string]string
	output   string
	success  bool
	error    string
	duration string
	done     bool
}

// NewModel creates a new UI model.
func NewModel(processQuery func(query string) tea.Cmd) Model {
	ti := textinput.New()
	ti.Placeholder = "Ask me anything... (try: 'Is github.com reachable?')"
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
		state:        models.StateIdle,
		messages:     make([]chatMessage, 0),
		processQuery: processQuery,
	}
}

// Message types for Bubble Tea
type (
	// AgentEventMsg wraps agent events for the UI
	AgentEventMsg models.AgentEvent

	// errMsg wraps errors
	errMsg struct{ err error }
)

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
			if m.state == models.StateIdle {
				m.quitting = true
				return m, tea.Quit
			}
			// Cancel current operation
			m.state = models.StateIdle
			return m, nil

		case tea.KeyEnter:
			if m.state != models.StateIdle {
				return m, nil
			}

			query := strings.TrimSpace(m.textInput.Value())
			if query == "" {
				return m, nil
			}

			// Handle special commands
			if cmd := m.handleCommand(query); cmd != nil {
				return m, cmd
			}

			// Add user message to chat
			m.messages = append(m.messages, chatMessage{
				role:    "user",
				content: query,
			})

			// Clear input and start processing
			m.textInput.SetValue("")
			m.state = models.StateThinking

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
		m.ready = true

	case AgentEventMsg:
		return m.handleAgentEvent(models.AgentEvent(msg))

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case errMsg:
		m.err = msg.err
		m.state = models.StateError
	}

	// Update text input
	if m.state == models.StateIdle {
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

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
  exit, quit  Exit cliche

Example queries:
  "Is github.com reachable?"
  "Ping 8.8.8.8"
  "Check if port 443 is open on google.com"`,
		})
		m.textInput.SetValue("")
		return nil

	case "tools":
		m.messages = append(m.messages, chatMessage{
			role: "system",
			content: `Available tools:
  • ping        - Check host reachability
  • dns-lookup  - Query DNS records
  • port-scan   - Check if ports are open
  • http        - Make HTTP requests
  • traceroute  - Trace network path
  • netinfo     - Get network interface info`,
		})
		m.textInput.SetValue("")
		return nil
	}

	return nil
}

// handleAgentEvent processes events from the agent.
func (m Model) handleAgentEvent(event models.AgentEvent) (tea.Model, tea.Cmd) {
	m.state = event.State

	switch event.State {
	case models.StateToolCall:
		if event.ToolCall != nil {
			m.currentTool = &toolExecution{
				name:   event.ToolCall.Name,
				params: event.ToolCall.Params,
			}
		}

	case models.StateToolExecuting:
		// Tool is running, spinner shows progress

	case models.StateResponding:
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

		if event.FinalAnswer != "" {
			m.messages = append(m.messages, chatMessage{
				role:    "assistant",
				content: event.FinalAnswer,
			})
		}
		m.state = models.StateIdle

	case models.StateError:
		m.err = event.Error
		m.messages = append(m.messages, chatMessage{
			role:    "system",
			content: fmt.Sprintf("Error: %v", event.Error),
		})
		m.state = models.StateIdle
	}

	return m, m.spinner.Tick
}

// View renders the UI.
func (m Model) View() string {
	if m.quitting {
		return m.styles.SystemMessage.Render("👋 Goodbye!\n")
	}

	if !m.ready {
		return "Initializing..."
	}

	var b strings.Builder

	// Banner
	b.WriteString(m.styles.BannerTitle.Render(Banner()))
	b.WriteString("\n\n")

	// Chat history
	for _, msg := range m.messages {
		b.WriteString(m.renderMessage(msg))
		b.WriteString("\n")
	}

	// Current tool execution (if any)
	if m.currentTool != nil && !m.currentTool.done {
		b.WriteString(m.renderToolInProgress())
		b.WriteString("\n")
	}

	// Status/spinner when processing
	if m.state != models.StateIdle {
		b.WriteString(m.renderStatus())
		b.WriteString("\n")
	}

	// Input
	b.WriteString("\n")
	b.WriteString(m.styles.Prompt.Render("❯ "))
	if m.state == models.StateIdle {
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
		return m.styles.AssistantMessage.Render("cliche: " + msg.content)

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
	header := fmt.Sprintf("🔧 %s", t.name)
	b.WriteString(m.styles.ToolName.Render(header))

	// Parameters
	if len(t.params) > 0 {
		params := make([]string, 0, len(t.params))
		for k, v := range t.params {
			params = append(params, fmt.Sprintf("%s=%q", k, v))
		}
		b.WriteString(" ")
		b.WriteString(m.styles.ToolParams.Render(strings.Join(params, " ")))
	}
	b.WriteString("\n")

	// Result
	if t.success {
		b.WriteString(m.styles.ToolSuccess.Render("  ✓ Success"))
		if t.duration != "" {
			b.WriteString(m.styles.ToolParams.Render(fmt.Sprintf(" (%s)", t.duration)))
		}
		b.WriteString("\n")
		if t.output != "" {
			// Indent output
			lines := strings.Split(t.output, "\n")
			for _, line := range lines {
				if line != "" {
					b.WriteString(m.styles.ToolOutput.Render("  │ " + line))
					b.WriteString("\n")
				}
			}
		}
	} else {
		b.WriteString(m.styles.ToolError.Render("  ✗ Failed: " + t.error))
		b.WriteString("\n")
	}

	return m.styles.ToolBox.Render(b.String())
}

// renderToolInProgress renders a tool that's currently executing.
func (m Model) renderToolInProgress() string {
	var b strings.Builder

	header := fmt.Sprintf("🔧 %s", m.currentTool.name)
	b.WriteString(m.styles.ToolName.Render(header))

	if len(m.currentTool.params) > 0 {
		params := make([]string, 0, len(m.currentTool.params))
		for k, v := range m.currentTool.params {
			params = append(params, fmt.Sprintf("%s=%q", k, v))
		}
		b.WriteString(" ")
		b.WriteString(m.styles.ToolParams.Render(strings.Join(params, " ")))
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
		m.styles.HelpKey.Render("help") + m.styles.HelpValue.Render(" commands"),
	}
	return m.styles.HelpBar.Render(strings.Join(help, "  •  "))
}

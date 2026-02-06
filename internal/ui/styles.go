package ui

import (
	"github.com/charmbracelet/lipgloss"
)

// Theme defines the visual style for the telemetry debugger.
type Theme struct {
	// Brand colors
	Primary   lipgloss.Color
	Secondary lipgloss.Color
	Accent    lipgloss.Color

	// Semantic colors
	Success lipgloss.Color
	Warning lipgloss.Color
	Error   lipgloss.Color
	Muted   lipgloss.Color

	// Text colors
	Text     lipgloss.Color
	TextDim  lipgloss.Color
	TextBold lipgloss.Color
}

// DefaultTheme returns the default color theme.
func DefaultTheme() Theme {
	return Theme{
		Primary:   lipgloss.Color("#7C3AED"), // Purple
		Secondary: lipgloss.Color("#06B6D4"), // Cyan
		Accent:    lipgloss.Color("#F59E0B"), // Amber

		Success: lipgloss.Color("#10B981"), // Emerald
		Warning: lipgloss.Color("#F59E0B"), // Amber
		Error:   lipgloss.Color("#EF4444"), // Red
		Muted:   lipgloss.Color("#6B7280"), // Gray

		Text:     lipgloss.Color("#F9FAFB"), // Near white
		TextDim:  lipgloss.Color("#9CA3AF"), // Gray
		TextBold: lipgloss.Color("#FFFFFF"), // White
	}
}

// Styles contains all the styled components for the UI.
type Styles struct {
	// App container
	App lipgloss.Style

	// Header/Banner
	Banner      lipgloss.Style
	BannerTitle lipgloss.Style

	// Input area
	Prompt lipgloss.Style
	Input  lipgloss.Style
	Cursor lipgloss.Style

	// Messages
	UserMessage      lipgloss.Style
	AssistantMessage lipgloss.Style
	SystemMessage    lipgloss.Style

	// Tool execution
	ToolBox     lipgloss.Style
	ToolName    lipgloss.Style
	ToolParams  lipgloss.Style
	ToolOutput  lipgloss.Style
	ToolSuccess lipgloss.Style
	ToolError   lipgloss.Style

	// Status
	Spinner    lipgloss.Style
	StatusText lipgloss.Style
	StateLabel lipgloss.Style

	// Help
	HelpKey   lipgloss.Style
	HelpValue lipgloss.Style
	HelpBar   lipgloss.Style
}

// NewStyles creates styled components from a theme.
func NewStyles(t Theme) Styles {
	return Styles{
		App: lipgloss.NewStyle().
			Padding(1, 2),

		Banner: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(t.Primary).
			Padding(0, 2).
			MarginBottom(1),

		BannerTitle: lipgloss.NewStyle().
			Foreground(t.Primary).
			Bold(true),

		Prompt: lipgloss.NewStyle().
			Foreground(t.Secondary).
			Bold(true),

		Input: lipgloss.NewStyle().
			Foreground(t.Text),

		UserMessage: lipgloss.NewStyle().
			Foreground(t.Secondary).
			Bold(true).
			PaddingLeft(2),

		AssistantMessage: lipgloss.NewStyle().
			Foreground(t.Text).
			PaddingLeft(2),

		SystemMessage: lipgloss.NewStyle().
			Foreground(t.Muted).
			Italic(true).
			PaddingLeft(2),

		ToolBox: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(t.Accent).
			Padding(0, 1).
			MarginLeft(2).
			MarginTop(1).
			MarginBottom(1),

		ToolName: lipgloss.NewStyle().
			Foreground(t.Accent).
			Bold(true),

		ToolParams: lipgloss.NewStyle().
			Foreground(t.TextDim),

		ToolOutput: lipgloss.NewStyle().
			Foreground(t.Text).
			PaddingLeft(1),

		ToolSuccess: lipgloss.NewStyle().
			Foreground(t.Success).
			Bold(true),

		ToolError: lipgloss.NewStyle().
			Foreground(t.Error).
			Bold(true),

		Spinner: lipgloss.NewStyle().
			Foreground(t.Primary),

		StatusText: lipgloss.NewStyle().
			Foreground(t.TextDim),

		StateLabel: lipgloss.NewStyle().
			Foreground(t.Primary).
			Bold(true),

		HelpKey: lipgloss.NewStyle().
			Foreground(t.Muted),

		HelpValue: lipgloss.NewStyle().
			Foreground(t.TextDim),

		HelpBar: lipgloss.NewStyle().
			Foreground(t.Muted).
			MarginTop(1),
	}
}

// DefaultStyles returns styles with the default theme.
func DefaultStyles() Styles {
	return NewStyles(DefaultTheme())
}

// Banner returns the ASCII art banner.
func Banner() string {
	banner := `
 ╔═════════════════════════════════════════════════════════════════╗
 ║                                                                 ║
 ║      ██████╗██╗     ██╗ ██████╗██╗  ██╗██████                   ║
 ║     ██╔════╝██║     ██║██╔════╝██║  ██║██╔══╝                   ║
 ║     ██║     ██║     ██║██║     ███████║█████╗                   ║
 ║     ██║     ██║     ██║██║     ██╔══██║██╔══╝                   ║
 ║     ╚██████╗███████╗██║╚██████╗██║  ██║███████╗                 ║
 ║      ╚═════╝╚══════╝╚═╝ ╚═════╝╚═╝  ╚═╝╚══════╝                 ║
 ║                                                                 ║
 ║             AI-Powered DevOps Debugging Assistant               ║
 ╚═════════════════════════════════════════════════════════════════╝`
	return banner
}

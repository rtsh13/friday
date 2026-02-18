package ui

import (
	"github.com/charmbracelet/lipgloss"
)

// Theme defines the visual style for CLICHE.
type Theme struct {
	Primary   lipgloss.Color
	Secondary lipgloss.Color
	Accent    lipgloss.Color
	Success   lipgloss.Color
	Warning   lipgloss.Color
	Error     lipgloss.Color
	Muted     lipgloss.Color
	Text      lipgloss.Color
	TextDim   lipgloss.Color
	TextBold  lipgloss.Color
}

// DefaultTheme returns the default color theme.
func DefaultTheme() Theme {
	return Theme{
		Primary:   lipgloss.Color("#7C3AED"), // Purple
		Secondary: lipgloss.Color("#06B6D4"), // Cyan
		Accent:    lipgloss.Color("#F59E0B"), // Amber
		Success:   lipgloss.Color("#10B981"), // Emerald
		Warning:   lipgloss.Color("#F59E0B"), // Amber
		Error:     lipgloss.Color("#EF4444"), // Red
		Muted:     lipgloss.Color("#6B7280"), // Gray
		Text:      lipgloss.Color("#F9FAFB"), // Near white
		TextDim:   lipgloss.Color("#9CA3AF"), // Gray
		TextBold:  lipgloss.Color("#FFFFFF"), // White
	}
}

// Styles contains all styled components.
type Styles struct {
	BannerTitle      lipgloss.Style
	Prompt           lipgloss.Style
	SectionHeader    lipgloss.Style
	Divider          lipgloss.Style
	UserMessage      lipgloss.Style
	AssistantMessage lipgloss.Style
	SystemMessage    lipgloss.Style
	ToolName         lipgloss.Style
	ToolParams       lipgloss.Style
	ToolOutput       lipgloss.Style
	ToolSuccess      lipgloss.Style
	ToolError        lipgloss.Style
	Spinner          lipgloss.Style
	StatusText       lipgloss.Style
	HelpKey          lipgloss.Style
	HelpValue        lipgloss.Style
}

// NewStyles creates styled components from a theme.
func NewStyles(t Theme) Styles {
	return Styles{
		BannerTitle: lipgloss.NewStyle().
			Foreground(t.Primary).
			Bold(true),

		Prompt: lipgloss.NewStyle().
			Foreground(t.Secondary).
			Bold(true),

		SectionHeader: lipgloss.NewStyle().
			Foreground(t.Secondary).
			Bold(true),

		Divider: lipgloss.NewStyle().
			Foreground(t.Muted),

		UserMessage: lipgloss.NewStyle().
			Foreground(t.Secondary).
			Bold(true),

		AssistantMessage: lipgloss.NewStyle().
			Foreground(t.Text),

		SystemMessage: lipgloss.NewStyle().
			Foreground(t.Muted).
			Italic(true),

		ToolName: lipgloss.NewStyle().
			Foreground(t.Accent).
			Bold(true),

		ToolParams: lipgloss.NewStyle().
			Foreground(t.TextDim),

		ToolOutput: lipgloss.NewStyle().
			Foreground(t.Text),

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

		HelpKey: lipgloss.NewStyle().
			Foreground(t.Muted),

		HelpValue: lipgloss.NewStyle().
			Foreground(t.TextDim),
	}
}

// DefaultStyles returns styles with the default theme.
func DefaultStyles() Styles {
	return NewStyles(DefaultTheme())
}

// Banner returns the ASCII art banner.
func Banner() string {
	return `
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
}

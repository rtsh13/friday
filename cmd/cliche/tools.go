package main

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/stratos/cliche/internal/tools"
)

var toolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "List available tools",
	Long: `List all available diagnostic tools.

These tools are automatically used by the AI agent when analyzing
your infrastructure. You can also reference them directly in queries.

Examples:
  cliche tools           # List all tools
  cliche tools --verbose # Show detailed info`,
	Run: func(cmd *cobra.Command, args []string) {
		runTools()
	},
}

func runTools() {
	registry := tools.NewRegistry()
	tools.RegisterNetworkingTools(registry)

	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7C3AED")).
		Bold(true)

	toolStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#F59E0B")).
		Bold(true)

	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#9CA3AF"))

	paramStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#06B6D4"))

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6B7280"))

	fmt.Println(headerStyle.Render("Available Tools"))
	fmt.Println()

	for _, tool := range registry.All() {
		fmt.Printf("  %s\n", toolStyle.Render("◆ "+tool.Name()))
		fmt.Printf("    %s\n", descStyle.Render(tool.Description()))

		params := tool.Parameters()
		if len(params) > 0 && verbose {
			fmt.Println("    Parameters:")
			for _, p := range params {
				req := ""
				if p.Required {
					req = " (required)"
				}
				fmt.Printf("      %s%s\n", paramStyle.Render(p.Name), req)
				fmt.Printf("        %s\n", descStyle.Render(p.Description))
			}
		}
		fmt.Println()
	}

	if !verbose {
		fmt.Println(dimStyle.Render("  Use --verbose for parameter details"))
	}
}

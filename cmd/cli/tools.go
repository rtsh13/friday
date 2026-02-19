package main

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/friday/internal/functions"
	"github.com/spf13/cobra"
)

var toolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "List available tools",
	Long: `List all available diagnostic tools.

These tools are automatically used by the AI agent when analyzing
your infrastructure. You can also reference them directly in queries.

Examples:
  friday tools           # List all tools
  friday tools --verbose # Show detailed info`,
	Run: func(cmd *cobra.Command, args []string) {
		runTools()
	},
}

func runTools() {
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

	categoryStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#10B981")).
		Bold(true)

	// Load function registry from YAML
	registry, err := functions.LoadRegistry("functions.yaml")
	if err != nil {
		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).
			Render(fmt.Sprintf("Failed to load functions: %v", err)))
		fmt.Println(descStyle.Render("\nMake sure functions.yaml exists in the current directory."))
		return
	}

	fmt.Println(headerStyle.Render("Available Tools"))
	fmt.Println()

	// Group functions by category
	categories := make(map[string][]string)
	for name := range registry.Functions {
		fn := registry.Functions[name]
		categories[fn.Category] = append(categories[fn.Category], name)
	}

	// Display by category
	for category, names := range categories {
		fmt.Printf("  %s\n", categoryStyle.Render(category))

		for _, name := range names {
			fn, _ := registry.Get(name)

			fmt.Printf("    %s\n", toolStyle.Render(fn.Name))
			fmt.Printf("      %s\n", descStyle.Render(fn.Description))

			if verbose && len(fn.Parameters) > 0 {
				fmt.Println("      Parameters:")
				for _, p := range fn.Parameters {
					req := ""
					if p.Required {
						req = " (required)"
					}
					fmt.Printf("        %s%s\n", paramStyle.Render(p.Name), req)
					if p.Description != "" {
						fmt.Printf("          %s\n", descStyle.Render(p.Description))
					}
				}
			}
		}
		fmt.Println()
	}

	totalCount := len(registry.Functions)
	fmt.Println(descStyle.Render(fmt.Sprintf("  Total: %d tools available", totalCount)))

	if !verbose {
		fmt.Println(descStyle.Render("  Use --verbose for parameter details"))
	}
}

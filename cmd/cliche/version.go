package main

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

// Version information - set at build time
var (
	Version   = "0.1.0-dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		printVersion()
	},
}

func printVersion() {
	logoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7C3AED")).
		Bold(true)

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#9CA3AF"))

	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#F9FAFB"))

	fmt.Println(logoStyle.Render("cliche"))
	fmt.Println()
	fmt.Printf("%s %s\n", labelStyle.Render("Version:"), valueStyle.Render(Version))
	fmt.Printf("%s %s\n", labelStyle.Render("Commit:"), valueStyle.Render(GitCommit))
	fmt.Printf("%s %s\n", labelStyle.Render("Built:"), valueStyle.Render(BuildDate))
}

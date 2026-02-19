package main

import (
	"fmt"
	"runtime"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	Version   = "0.1.0"
	GitCommit = "dev"
	BuildDate = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run:   runVersion,
}

func runVersion(cmd *cobra.Command, args []string) {
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7C3AED")).
		Bold(true)

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#9CA3AF"))

	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#06B6D4"))

	fmt.Println(titleStyle.Render("friday"))
	fmt.Println()
	fmt.Printf("%s %s\n", labelStyle.Render("Version:"), valueStyle.Render(Version))
	fmt.Printf("%s %s\n", labelStyle.Render("Git Commit:"), valueStyle.Render(GitCommit))
	fmt.Printf("%s %s\n", labelStyle.Render("Build Date:"), valueStyle.Render(BuildDate))
	fmt.Printf("%s %s\n", labelStyle.Render("Go Version:"), valueStyle.Render(runtime.Version()))
	fmt.Printf("%s %s/%s\n", labelStyle.Render("Platform:"), valueStyle.Render(runtime.GOOS), valueStyle.Render(runtime.GOARCH))
}

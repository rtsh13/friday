package main

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/stratos/cliche/internal/config"
)

var (
	setOllamaURL   string
	setOllamaModel string
	setMode        string
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View or modify configuration",
	Long: `View or modify cliche configuration.

Configuration is stored in ~/.cliche/config.json

Examples:
  cliche config                           # View current config
  cliche config --ollama-url http://...   # Set Ollama URL
  cliche config --model qwen2.5:14b       # Set model
  cliche config --mode local              # Set mode (local/remote)`,
	Run: func(cmd *cobra.Command, args []string) {
		runConfig()
	},
}

func init() {
	configCmd.Flags().StringVar(&setOllamaURL, "ollama-url", "", "Set Ollama API URL")
	configCmd.Flags().StringVar(&setOllamaModel, "model", "", "Set LLM model")
	configCmd.Flags().StringVar(&setMode, "mode", "", "Set mode: local or remote")
}

func runConfig() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Warning: Could not load config: %v\n", err)
		cfg = config.DefaultConfig()
	}

	// Check if any flags are set for modification
	modified := false

	if setOllamaURL != "" {
		cfg.OllamaURL = setOllamaURL
		modified = true
	}
	if setOllamaModel != "" {
		cfg.OllamaModel = setOllamaModel
		modified = true
	}
	if setMode != "" {
		if setMode != "local" && setMode != "remote" {
			fmt.Println("Error: mode must be 'local' or 'remote'")
			return
		}
		cfg.Mode = setMode
		modified = true
	}

	// Save if modified
	if modified {
		if err := cfg.Save(); err != nil {
			fmt.Printf("Error saving config: %v\n", err)
			return
		}
		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981")).Render("✓ Configuration saved"))
		fmt.Println()
	}

	// Display current config
	printConfig(cfg)
}

func printConfig(cfg config.Config) {
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7C3AED")).
		Bold(true)

	keyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#9CA3AF")).
		Width(20)

	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#F9FAFB"))

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6B7280"))

	fmt.Println(headerStyle.Render("cliche Configuration"))
	fmt.Println()

	fmt.Printf("%s %s\n", keyStyle.Render("Ollama URL:"), valueStyle.Render(cfg.OllamaURL))
	fmt.Printf("%s %s\n", keyStyle.Render("Model:"), valueStyle.Render(cfg.OllamaModel))
	fmt.Printf("%s %s\n", keyStyle.Render("Mode:"), valueStyle.Render(cfg.Mode))
	fmt.Printf("%s %v\n", keyStyle.Render("Redact Secrets:"), valueStyle.Render(fmt.Sprintf("%v", cfg.RedactSecrets)))
	fmt.Printf("%s %v\n", keyStyle.Render("Telemetry:"), valueStyle.Render(fmt.Sprintf("%v", cfg.Telemetry)))

	path, _ := config.ConfigPath()
	fmt.Println()
	fmt.Printf("%s %s\n", keyStyle.Render("Config file:"), dimStyle.Render(path))
}

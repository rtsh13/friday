package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/friday/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View or edit configuration",
	Long:  "View current configuration or create a default config file.",
	Run:   runConfig,
}

var (
	configInit bool
	configShow bool
)

func init() {
	configCmd.Flags().BoolVar(&configInit, "init", false, "Create default config file")
	configCmd.Flags().BoolVar(&configShow, "show", true, "Show current configuration")
}

func runConfig(cmd *cobra.Command, args []string) {
	if configInit {
		initConfig()
		return
	}

	// Only show config when --show is true (default) or explicitly set
	if configShow {
		showConfig()
	}
}

func initConfig() {
	// Check if config already exists
	if _, err := os.Stat("config.yaml"); err == nil {
		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B")).
			Render("config.yaml already exists. Use --show to view it."))
		return
	}

	// Create default config
	cfg := config.DefaultConfig()
	if err := cfg.Save("config.yaml"); err != nil {
		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).
			Render(fmt.Sprintf("Failed to create config: %v", err)))
		os.Exit(1)
	}

	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981")).
		Render("Created config.yaml with default settings."))
	fmt.Println("\nEdit this file to configure:")
	fmt.Println("  - LLM endpoint and model")
	fmt.Println("  - Qdrant connection settings")
	fmt.Println("  - ONNX model paths")
	fmt.Println("  - RAG parameters")
}

func showConfig() {
	cfg, err := loadConfig()
	if err != nil {
		cfg = config.DefaultConfig()
		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B")).
			Render("No config file found. Showing defaults:\n"))
	} else {
		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#06B6D4")).Bold(true).
			Render("Current Configuration:\n"))
	}

	// Pretty print config
	data, err := yaml.Marshal(cfg)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println(string(data))

	// Show config file locations
	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).
		Render("\nConfig file locations (in order of precedence):"))
	fmt.Println("  1. ./config.local.yaml")
	fmt.Println("  2. ./config.yaml")
	fmt.Println("  3. ~/.telemetry-debugger/config.yaml")
}

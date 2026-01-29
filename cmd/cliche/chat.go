package main

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stratos/cliche/internal/agent"
	"github.com/stratos/cliche/internal/config"
	"github.com/stratos/cliche/internal/ollama"
	"github.com/stratos/cliche/internal/ui"
)

func runInteractive() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Warning: Could not load config: %v\n", err)
		cfg = config.DefaultConfig()
	}

	// override the configs if the args are passed during runtime
	if ollamaURL != "" {
		cfg.OllamaURL = ollamaURL
	}
	if ollamaModel != "" {
		cfg.OllamaModel = ollamaModel
	}

	agentCfg := agent.Config{
		OllamaConfig: ollama.Config{
			BaseURL: cfg.OllamaURL,
			Model:   cfg.OllamaModel,
		},
	}
	agentInstance := agent.New(agentCfg)

	fmt.Print(lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B")).Render("Connecting to Ollama... "))
	if err := agentInstance.Ping(context.Background()); err != nil {
		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Render("✗"))
		fmt.Println()
		printConnectionHelp(cfg)
		os.Exit(1)
	}
	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981")).Render("✓"))
	fmt.Printf("Using model: %s\n\n", cfg.OllamaModel)

	model := ui.NewModel(func(query string) tea.Cmd {
		return agentInstance.ProcessQueryCmd(query)
	})

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running UI: %v\n", err)
		os.Exit(1)
	}
}

func printConnectionHelp(cfg config.Config) {
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
	cmdStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#06B6D4"))

	fmt.Println(errorStyle.Render("Could not connect to Ollama at " + cfg.OllamaURL))
	fmt.Println()
	fmt.Println(helpStyle.Render("Make sure Ollama is running:"))
	fmt.Println(cmdStyle.Render("  ollama serve"))
	fmt.Println()
	fmt.Println(helpStyle.Render("And pull the required model:"))
	fmt.Println(cmdStyle.Render("  ollama pull " + cfg.OllamaModel))
	fmt.Println()
	fmt.Println(helpStyle.Render("Or configure a different endpoint:"))
	fmt.Println(cmdStyle.Render("  cliche config --ollama-url http://your-server:11434"))
}

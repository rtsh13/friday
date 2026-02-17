package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/stratos/cliche/internal/agent"
	"github.com/stratos/cliche/internal/config"
	"github.com/stratos/cliche/internal/ui"
	"go.uber.org/zap"
)

var (
	configPath  string
	verbose     bool
	interactive bool
)

var rootCmd = &cobra.Command{
	Use:   "doclm [query]",
	Short: "AI-powered telemetry debugging assistant",
	Long: `
  ██████╗██╗     ██╗ ██████╗██╗  ██╗███████╗
 ██╔════╝██║     ██║██╔════╝██║  ██║██╔════╝
 ██║     ██║     ██║██║     ███████║█████╗  
 ██║     ██║     ██║██║     ██╔══██║██╔══╝  
 ╚██████╗███████╗██║╚██████╗██║  ██║███████╗
  ╚═════╝╚══════╝╚═╝ ╚═════╝╚═╝  ╚═╝╚══════╝

  AI-powered CLI tool for debugging telemetry and network issues.
  Supports gRPC, gNMI, YANG models, core dump analysis, and more.

Usage:
  doclm "Check gRPC health on port 50051"   Run a query
  doclm --it                                Interactive mode
  doclm tools                               List available tools
  doclm config                              View/edit configuration

Examples:
  doclm "Is the gRPC service on localhost:50051 healthy?"
  doclm "Analyze TCP connections on eth0 port 8080"
  doclm "Inspect network buffer settings"
  doclm --it`,

	Run: func(cmd *cobra.Command, args []string) {
		if interactive {
			runInteractive()
			return
		}

		if len(args) > 0 {
			runOneShot(args)
			return
		}

		cmd.Help()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().BoolVar(&interactive, "it", false, "Start interactive mode")
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "Path to config file")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")

	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(toolsCmd)
	rootCmd.AddCommand(versionCmd)
}

func runInteractive() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Printf("Warning: Could not load config: %v\n", err)
		cfg = config.DefaultConfig()
	}

	logger := createLogger()
	defer logger.Sync()

	// Create agent
	agentCfg := agent.Config{
		AppConfig:     cfg,
		FunctionsPath: "functions.yaml",
		Logger:        logger,
	}

	agentInstance, err := agent.New(agentCfg)
	if err != nil {
		printError("Failed to initialize agent", err)
		os.Exit(1)
	}
	defer agentInstance.Close()

	// Check LLM connectivity
	fmt.Print(lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B")).Render("Connecting to LLM... "))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := agentInstance.Ping(ctx); err != nil {
		cancel()
		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Render("✗"))
		fmt.Println()
		printConnectionHelp(cfg)
		os.Exit(1)
	}
	cancel()
	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981")).Render("✓"))
	fmt.Printf("Using model: %s\n\n", cfg.LLM.Model)

	// Create and run TUI
	model := ui.NewModel(func(query string) tea.Cmd {
		return agentInstance.ProcessQueryCmd(query)
	})

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running UI: %v\n", err)
		os.Exit(1)
	}
}

func runOneShot(args []string) {
	query := strings.Join(args, " ")

	cfg, err := loadConfig()
	if err != nil {
		cfg = config.DefaultConfig()
	}

	logger := createLogger()
	defer logger.Sync()

	// Create agent
	agentCfg := agent.Config{
		AppConfig:     cfg,
		FunctionsPath: "functions.yaml",
		Logger:        logger,
	}

	agentInstance, err := agent.New(agentCfg)
	if err != nil {
		printError("Failed to initialize agent", err)
		os.Exit(1)
	}
	defer agentInstance.Close()

	// Process query
	fmt.Printf("Query: %s\n\n", query)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	event, err := agentInstance.ProcessQuery(ctx, query)
	if err != nil {
		printError("Query failed", err)
		os.Exit(1)
	}

	// Print results
	if event.FinalAnswer != "" {
		fmt.Println(event.FinalAnswer)
	}
}

func loadConfig() (*config.Config, error) {
	if configPath != "" {
		return config.Load(configPath)
	}

	// Try standard locations
	return config.LoadFromPaths(
		"config.yaml",
		"config.local.yaml",
	)
}

func createLogger() *zap.Logger {
	if verbose {
		logger, _ := zap.NewDevelopment()
		return logger
	}
	logger, _ := zap.NewProduction()
	return logger
}

func printError(msg string, err error) {
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))
	fmt.Println(errorStyle.Render(fmt.Sprintf("Error: %s: %v", msg, err)))
}

func printConnectionHelp(cfg *config.Config) {
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
	cmdStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#06B6D4"))

	fmt.Println(errorStyle.Render("Could not connect to LLM at " + cfg.LLM.Endpoint))
	fmt.Println()
	fmt.Println(helpStyle.Render("Make sure vLLM is running:"))
	fmt.Println(cmdStyle.Render("  docker-compose up -d vllm"))
	fmt.Println()
	fmt.Println(helpStyle.Render("Or configure a different endpoint:"))
	fmt.Println(cmdStyle.Render("  Edit config.yaml and set llm.endpoint"))
}

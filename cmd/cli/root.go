package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/friday/internal/agent"
	"github.com/friday/internal/config"
	"github.com/friday/internal/ui"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	configPath  string
	verbose     bool
	interactive bool
)

var rootCmd = &cobra.Command{
	Use:   "friday [query]",
	Short: "AI-powered debugging assistant",
	Long: `
███████╗██████╗ ██╗██████╗  █████╗ ██╗   ██╗
██╔════╝██╔══██╗██║██╔══██╗██╔══██╗╚██╗ ██╔╝
█████╗  ██████╔╝██║██║  ██║███████║ ╚████╔╝ 
██╔══╝  ██╔══██╗██║██║  ██║██╔══██║  ╚██╔╝  
██║     ██║  ██║██║██████╔╝██║  ██║   ██║   
╚═╝     ╚═╝  ╚═╝╚═╝╚═════╝ ╚═╝  ╚═╝   ╚═╝  

  AI-powered CLI tool for debugging DevOps and network issues.

Usage:
  friday "Check gRPC health on port 50051"
  friday --it`,

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
	agentInstance := initAgent()
	defer agentInstance.Close()
	ui.Run(agentInstance)
}

func runOneShot(args []string) {
	query := strings.Join(args, " ")
	agentInstance := initAgent()
	defer agentInstance.Close()
	ui.RunOneShot(agentInstance, query)
}

// initAgent loads config, checks LLM connectivity, and returns a ready agent.
func initAgent() *agent.Agent {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Printf("Warning: Could not load config: %v\n", err)
		cfg = config.DefaultConfig()
	}

	logger := createLogger()

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

	// Check LLM connectivity.
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
	fmt.Printf("Using model: %s\n", cfg.LLM.Model)

	return agentInstance
}

func loadConfig() (*config.Config, error) {
	if configPath != "" {
		return config.Load(configPath)
	}
	return config.LoadFromPaths(
		"config.local.yaml",
		"config.yaml",
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
	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).
		Render(fmt.Sprintf("Error: %s: %v", msg, err)))
}

func printConnectionHelp(cfg *config.Config) {
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
	cmdStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#06B6D4"))
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))

	fmt.Println(errStyle.Render("Could not connect to LLM at " + cfg.LLM.Endpoint))
	fmt.Println()
	fmt.Println(helpStyle.Render("Make sure Ollama is running:"))
	fmt.Println(cmdStyle.Render("  ollama serve"))
	fmt.Println()
	fmt.Println(helpStyle.Render("Or configure a different endpoint:"))
	fmt.Println(cmdStyle.Render("  Edit config.yaml and set llm.endpoint"))
}

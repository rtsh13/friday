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
	"github.com/stratos/cliche/internal/ollama"
	"github.com/stratos/cliche/internal/rag"
	"github.com/stratos/cliche/internal/ui"
	"go.uber.org/zap"
)

var (
	ollamaURL   string
	ollamaModel string
	verbose     bool
	interactive bool

	// Global RAG pipeline (initialized once)
	ragPipeline *rag.Pipeline
	logger      *zap.Logger
)

var rootCmd = &cobra.Command{
	Use:   "cliche [query]",
	Short: "AI-powered DevOps debugging assistant",
	Long: `
   ██████╗██╗     ██╗ ██████╗██╗  ██╗███████╗
  ██╔════╝██║     ██║██╔════╝██║  ██║██╔════╝
  ██║     ██║     ██║██║     ███████║█████╗  
  ██║     ██║     ██║██║     ██╔══██║██╔══╝  
  ╚██████╗███████╗██║╚██████╗██║  ██║███████╗
   ╚═════╝╚══════╝╚═╝ ╚═════╝╚═╝  ╚═╝╚══════╝

  AI-powered CLI tool for DevOps engineers and SREs.
  Debug infrastructure issues using natural language.

Usage:
  cliche "Is github.com up?"   Run a one-shot query
  cliche --it                  Start interactive mode
  cliche tools                 List available tools
  cliche config                View/edit configuration
  cliche version               Show version info

Examples:
  cliche "what's using port 8080?"
  cliche "check if redis is running"
  cliche "show me failed systemd services"
  cliche --it`,

	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Initialize logger
		var err error
		if verbose {
			logger, err = zap.NewDevelopment()
		} else {
			logger, err = zap.NewProduction()
		}
		if err != nil {
			return fmt.Errorf("failed to create logger: %w", err)
		}

		// Load configuration
		cfg, err := config.Load()
		if err != nil {
			fmt.Printf("⚠️  Warning: Could not load config: %v\n", err)
			cfg = config.DefaultConfig()
		}

		// Initialize RAG pipeline (optional - continue if it fails)
		ragPipeline, err = rag.NewPipeline(
			cfg.Qdrant.Host,
			cfg.Qdrant.Port,
			cfg.Embedding.Endpoint,
			logger,
		)
		if err != nil {
			fmt.Printf("⚠️  Warning: Could not initialize RAG pipeline: %v\n", err)
			fmt.Println("Continuing without RAG context retrieval")
		}

		return nil
	},

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

	rootCmd.PersistentFlags().StringVar(&ollamaURL, "ollama-url", "", "Ollama API URL")
	rootCmd.PersistentFlags().StringVar(&ollamaModel, "model", "", "LLM model to use")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")

	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(toolsCmd)
	rootCmd.AddCommand(versionCmd)
}

// runOneShot executes a single query with RAG context + agent reasoning
func runOneShot(args []string) {
	query := strings.Join(args, " ")

	// Styling
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7C3AED")).
		Bold(true)

	infoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#06B6D4"))

	successStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#10B981"))

	warnStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#F59E0B"))

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#9CA3AF"))

	fmt.Printf("%s %s\n\n", headerStyle.Render("🔍 Query:"), query)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Step 1: Retrieve context via RAG (optional)
	var ragContext []string
	if ragPipeline != nil {
		fmt.Println(infoStyle.Render("📚 Retrieving relevant context..."))

		chunks, err := ragPipeline.Retrieve(ctx, query)
		if err != nil {
			fmt.Println(warnStyle.Render(fmt.Sprintf("⚠️  RAG retrieval failed: %v", err)))
		} else if len(chunks) > 0 {
			fmt.Printf("%s\n", successStyle.Render(fmt.Sprintf("✓ Retrieved %d chunks", len(chunks))))

			// Display retrieved context
			fmt.Println("\n" + headerStyle.Render("📖 Retrieved Context:"))
			fmt.Println(strings.Repeat("─", 80))

			sourceStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F59E0B"))

			scoreStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#06B6D4"))

			for i, chunk := range chunks {
				fmt.Printf("\n[%d] %s %.2f  |  %s %s\n",
					i+1,
					scoreStyle.Render("Score:"),
					chunk.Score,
					sourceStyle.Render("Source:"),
					chunk.Source)
				fmt.Printf("    %s\n", dimStyle.Render(truncateContent(chunk.Content, 150)))
				ragContext = append(ragContext, chunk.Content)
			}
			fmt.Println("\n" + strings.Repeat("─", 80))
		} else {
			fmt.Println(warnStyle.Render("⚠️  No relevant context found in knowledge base"))
		}
	} else {
		fmt.Println(warnStyle.Render("⚠️  RAG pipeline not available - skipping context retrieval"))
	}

	fmt.Println()
	fmt.Println(infoStyle.Render("🤔 Analyzing with LLM..."))
	fmt.Println()

	// Step 2: Build prompt with RAG context
	// This is where we'd integrate with the agent/LLM
	// For now, just show that RAG context would be passed
	if len(ragContext) > 0 {
		fmt.Println(dimStyle.Render("(RAG context would be passed to LLM for better reasoning)"))
	}

	fmt.Println()
	fmt.Println(dimStyle.Render("(Full agent execution with LLM + tool calling coming soon)"))
}

// runInteractive starts an interactive session with agent loop
func runInteractive() {
	// Load config to get Ollama settings
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Warning: Could not load config: %v\n", err)
		cfg = config.DefaultConfig()
	}

	// Override with CLI flags if provided
	if ollamaURL != "" {
		cfg.OllamaURL = ollamaURL
	}
	if ollamaModel != "" {
		cfg.OllamaModel = ollamaModel
	}

	// Create agent with Ollama and RAG config
	agentCfg := agent.Config{
		OllamaConfig: ollama.Config{
			BaseURL: cfg.OllamaURL,
			Model:   cfg.OllamaModel,
		},
		RAGConfig: &agent.RAGConfig{
			QdrantHost:        cfg.Qdrant.Host,
			QdrantPort:        cfg.Qdrant.Port,
			EmbeddingEndpoint: cfg.Embedding.Endpoint,
		},
	}
	agentInstance := agent.New(agentCfg)

	// Styling
	connectStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#F59E0B"))

	successStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#10B981"))

	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#EF4444"))

	// Check Ollama connection
	fmt.Print(connectStyle.Render("Connecting to Ollama... "))
	if err := agentInstance.Ping(context.Background()); err != nil {
		fmt.Println(errorStyle.Render("✗"))
		fmt.Println()
		printConnectionHelp(cfg)
		os.Exit(1)
	}
	fmt.Println(successStyle.Render("✓"))
	fmt.Printf("Using model: %s\n\n", cfg.OllamaModel)

	// Start TUI
	model := ui.NewModel(func(query string) tea.Cmd {
		return agentInstance.ProcessQueryCmd(query)
	})

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running UI: %v\n", err)
		os.Exit(1)
	}
}

// printConnectionHelp displays instructions for connecting to Ollama
func printConnectionHelp(cfg config.Config) {
	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#EF4444"))

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#9CA3AF"))

	cmdStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#06B6D4"))

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

// truncateContent truncates long content for display
func truncateContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen] + "..."
}

// package main

// import (
// 	"fmt"
// 	"os"
// 	"strings"

// 	"github.com/spf13/cobra"
// )

// var (
// 	ollamaURL   string
// 	ollamaModel string
// 	verbose     bool
// 	interactive bool
// )

// var rootCmd = &cobra.Command{
// 	Use:   "cliche [query]",
// 	Short: "AI-powered DevOps debugging assistant",
// 	Long: `CLICHÉ - AI-powered CLI tool for DevOps engineers and SREs.

// Debug networking, filesystem, process, and infrastructure issues
// using natural language queries and automated tool execution.

// Usage:
//   cliche "Is github.com up?"   One-shot query
//   cliche --it                  Interactive mode
//   cliche help                  Show this help
//   cliche tools                 List available tools
//   cliche config                Show/edit configuration`,

// 	Run: func(cmd *cobra.Command, args []string) {
// 		if interactive {
// 			runInteractive()
// 			return
// 		}

// 		if len(args) > 0 {
// 			runOneShot(args)
// 			return
// 		}

// 		cmd.Help()
// 	},
// }

// func Execute() {
// 	if err := rootCmd.Execute(); err != nil {
// 		os.Exit(1)
// 	}
// }

// func init() {
// 	rootCmd.Flags().BoolVar(&interactive, "it", false, "Start interactive mode")

// 	rootCmd.PersistentFlags().StringVar(&ollamaURL, "ollama-url", "", "Ollama API URL")
// 	rootCmd.PersistentFlags().StringVar(&ollamaModel, "model", "", "LLM model to use")
// 	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")

// 	// Remove chatCmd - no longer needed
// 	rootCmd.AddCommand(configCmd)
// 	rootCmd.AddCommand(toolsCmd)
// 	rootCmd.AddCommand(versionCmd)
// }

// func runOneShot(args []string) {
// 	query := strings.Join(args, " ")
// 	fmt.Printf("Query: %s\n\n", query)
// 	fmt.Println("(One-shot mode coming soon)")
// }

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	ollamaURL   string
	ollamaModel string
	verbose     bool
	interactive bool
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

func runOneShot(args []string) {
	query := strings.Join(args, " ")
	fmt.Printf("Query: %s\n\n", query)
	fmt.Println("One-shot mode coming soon")
}

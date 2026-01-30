package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ashutoshrp06/telemetry-debugger/internal/config"
	"github.com/ashutoshrp06/telemetry-debugger/internal/context"
	"github.com/ashutoshrp06/telemetry-debugger/internal/executor"
	"github.com/ashutoshrp06/telemetry-debugger/internal/functions"
	"github.com/ashutoshrp06/telemetry-debugger/internal/llm"
	"github.com/ashutoshrp06/telemetry-debugger/internal/rag"
	"github.com/ashutoshrp06/telemetry-debugger/internal/types"
	"github.com/ashutoshrp06/telemetry-debugger/internal/validator"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Load("config.yaml")
	if err != nil {
		fmt.Printf("❌ Error loading config: %v\n", err)
		os.Exit(1)
	}
	
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	
	inputValidator := validator.NewInputValidator()
	outputValidator := validator.NewOutputValidator()
	
	functionRegistry, err := functions.LoadRegistry("functions.yaml")
	if err != nil {
		logger.Fatal("Failed to load functions", zap.Error(err))
	}
	
	ragPipeline, err := rag.NewPipeline(
		cfg.Qdrant.Host,
		cfg.Qdrant.Port,
		cfg.Embedding.Endpoint,
		logger,
	)
	if err != nil {
		logger.Fatal("Failed to initialize RAG", zap.Error(err))
	}
	
	llmClient := llm.NewClient(
		cfg.LLM.Endpoint,
		cfg.LLM.Model,
		time.Duration(cfg.LLM.TimeoutSeconds)*time.Second,
	)
	
	exec := executor.NewExecutor(logger)
	txExec := executor.NewTransactionExecutor(exec)
	
	ctxManager := context.NewManager(cfg.Conversation.MaxMessages)
	
	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║  Telemetry Debugger v1.0                         ║")
	fmt.Println("║  Type your query or 'exit' to quit              ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")
	fmt.Println()
	
	scanner := bufio.NewScanner(os.Stdin)
	
	for {
		fmt.Print("> ")
		
		if !scanner.Scan() {
			break
		}
		
		query := scanner.Text()
		
		if strings.ToLower(strings.TrimSpace(query)) == "exit" {
			fmt.Println("Goodbye!")
			break
		}
		
		if strings.ToLower(strings.TrimSpace(query)) == "clear" {
			ctxManager.Clear()
			fmt.Println("✓ Context cleared")
			continue
		}
		
		if strings.TrimSpace(query) == "" {
			continue
		}
		
		if err := processQuery(
			context.Background(),
			query,
			inputValidator,
			outputValidator,
			ragPipeline,
			llmClient,
			txExec,
			functionRegistry,
			ctxManager,
			logger,
		); err != nil {
			fmt.Printf("❌ Error: %v\n\n", err)
		}
	}
}

func processQuery(
	ctx context.Context,
	query string,
	inputValidator *validator.InputValidator,
	outputValidator *validator.OutputValidator,
	ragPipeline *rag.Pipeline,
	llmClient *llm.Client,
	txExec *executor.TransactionExecutor,
	funcRegistry *functions.Registry,
	ctxManager *context.Manager,
	logger *zap.Logger,
) error {
	if err := inputValidator.Validate(query); err != nil {
		return fmt.Errorf("invalid input: %w", err)
	}
	
	sanitized := inputValidator.Sanitize(query)
	
	chunks, err := ragPipeline.Retrieve(ctx, sanitized)
	if err != nil {
		return fmt.Errorf("RAG retrieval failed: %w", err)
	}
	
	logger.Info("Retrieved chunks", zap.Int("count", len(chunks)))
	
	prompt := llm.BuildPrompt(query, chunks, funcRegistry.List())
	
	response, err := llmClient.Generate(ctx, prompt)
	if err != nil {
		return fmt.Errorf("LLM generation failed: %w", err)
	}
	
	llmResp, err := outputValidator.Validate(response, funcRegistry.Functions)
	if err != nil {
		fmt.Printf("\nRaw response:\n%s\n\n", response)
		return fmt.Errorf("validation failed: %w", err)
	}
	
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("REASONING:")
	fmt.Println(llmResp.Reasoning)
	fmt.Println()
	
	fmt.Println("PROPOSED ACTIONS:")
	for i, fn := range llmResp.Functions {
		fmt.Printf("  [%d] %s\n", i+1, fn.Name)
		paramsJSON, _ := json.MarshalIndent(fn.Params, "      ", "  ")
		fmt.Printf("      Params: %s\n", string(paramsJSON))
	}
	fmt.Println()
	
	results, err := txExec.ExecuteTransaction(ctx, llmResp.Functions)
	if err != nil {
		fmt.Printf("❌ Execution failed: %v\n", err)
	} else {
		fmt.Println("EXECUTION RESULTS:")
		for _, result := range results {
			status := "✓"
			if !result.Success {
				status = "✗"
			}
			fmt.Printf("  %s %s: %s\n", status, result.Function.Name, result.Output)
		}
	}
	fmt.Println()
	
	fmt.Println("EXPLANATION:")
	fmt.Println(llmResp.Explanation)
	fmt.Println(strings.Repeat("=", 70) + "\n")
	
	msg := types.Message{
		Role:      "user",
		Content:   query,
		Timestamp: time.Now(),
		Functions: results,
	}
	ctxManager.AddMessage(msg)
	
	return nil
}
// Package agent implements the core agent loop for the telemetry debugger.
package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ashutoshrp06/telemetry-debugger/internal/config"
	ctxmgr "github.com/ashutoshrp06/telemetry-debugger/internal/context"
	"github.com/ashutoshrp06/telemetry-debugger/internal/executor"
	"github.com/ashutoshrp06/telemetry-debugger/internal/functions"
	"github.com/ashutoshrp06/telemetry-debugger/internal/llm"
	"github.com/ashutoshrp06/telemetry-debugger/internal/rag"
	"github.com/ashutoshrp06/telemetry-debugger/internal/types"
	"github.com/ashutoshrp06/telemetry-debugger/internal/validator"
	tea "github.com/charmbracelet/bubbletea"
	"go.uber.org/zap"
)

// Agent orchestrates the interaction between user, LLM, RAG, and tool execution.
type Agent struct {
	cfg              *config.Config
	ragPipeline      *rag.Pipeline
	llmClient        *llm.Client
	executor         *executor.Executor
	txExecutor       *executor.TransactionExecutor
	functionRegistry *functions.Registry
	ctxManager       *ctxmgr.Manager
	inputValidator   *validator.InputValidator
	outputValidator  *validator.OutputValidator
	logger           *zap.Logger
}

// Config holds agent configuration.
type Config struct {
	AppConfig     *config.Config
	FunctionsPath string
	Logger        *zap.Logger
}

// New creates a new agent with all components initialized.
func New(cfg Config) (*Agent, error) {
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}

	if cfg.AppConfig == nil {
		cfg.AppConfig = config.DefaultConfig()
	}

	if cfg.FunctionsPath == "" {
		cfg.FunctionsPath = "functions.yaml"
	}

	// Load function registry
	funcRegistry, err := functions.LoadRegistry(cfg.FunctionsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load function registry: %w", err)
	}

	// Initialize RAG pipeline with ONNX embeddings
	ragPipeline, err := rag.NewPipeline(cfg.AppConfig, cfg.Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize RAG pipeline: %w", err)
	}

	// Initialize LLM client (vLLM)
	llmClient := llm.NewClient(
		cfg.AppConfig.LLM.Endpoint,
		cfg.AppConfig.LLM.Model,
		time.Duration(cfg.AppConfig.LLM.TimeoutSeconds)*time.Second,
	)

	// Initialize executor
	exec := executor.NewExecutor(cfg.Logger)
	txExec := executor.NewTransactionExecutor(exec)

	// Initialize context manager
	ctxManager := ctxmgr.NewManager(cfg.AppConfig.Conversation.MaxMessages)

	// Initialize validators
	inputValidator := validator.NewInputValidator()
	outputValidator := validator.NewOutputValidator()

	return &Agent{
		cfg:              cfg.AppConfig,
		ragPipeline:      ragPipeline,
		llmClient:        llmClient,
		executor:         exec,
		txExecutor:       txExec,
		functionRegistry: funcRegistry,
		ctxManager:       ctxManager,
		inputValidator:   inputValidator,
		outputValidator:  outputValidator,
		logger:           cfg.Logger,
	}, nil
}

// ProcessQueryCmd returns a Bubble Tea command that processes a query.
func (a *Agent) ProcessQueryCmd(query string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		event, err := a.process(ctx, query)
		if err != nil {
			return types.AgentEvent{
				State: types.StateError,
				Error: err,
			}
		}
		return event
	}
}

// ProcessQuery processes a query synchronously (for CLI mode).
func (a *Agent) ProcessQuery(ctx context.Context, query string) (*types.AgentEvent, error) {
	event, err := a.process(ctx, query)
	if err != nil {
		return nil, err
	}
	return &event, nil
}

// process handles the actual query processing.
func (a *Agent) process(ctx context.Context, query string) (types.AgentEvent, error) {
	// Validate input
	if err := a.inputValidator.Validate(query); err != nil {
		return types.AgentEvent{
			State: types.StateError,
			Error: fmt.Errorf("invalid input: %w", err),
		}, nil
	}

	sanitizedQuery := a.inputValidator.Sanitize(query)

	// Retrieve context from RAG
	chunks, err := a.ragPipeline.Retrieve(ctx, sanitizedQuery)
	if err != nil {
		a.logger.Warn("RAG retrieval failed, continuing without context",
			zap.Error(err))
		chunks = nil
	}

	a.logger.Info("Retrieved context",
		zap.Int("chunks_found", len(chunks)))

	// Build prompt with context and function registry
	prompt := llm.BuildPrompt(
		sanitizedQuery,
		chunks,
		a.functionRegistry.List(),
	)

	// Call LLM
	response, err := a.llmClient.Generate(ctx, prompt)
	if err != nil {
		return types.AgentEvent{
			State: types.StateError,
			Error: fmt.Errorf("LLM generation failed: %w", err),
		}, nil
	}

	// Parse and validate LLM response
	llmResp, err := a.outputValidator.Validate(response, a.functionRegistry.Functions)
	if err != nil {
		// If validation fails, return raw response
		a.logger.Warn("LLM response validation failed",
			zap.Error(err),
			zap.String("raw_response", truncate(response, 200)))

		return types.AgentEvent{
			State:       types.StateResponding,
			FinalAnswer: response,
			ChunksFound: len(chunks),
		}, nil
	}

	// If no functions to execute, return explanation
	if len(llmResp.Functions) == 0 {
		return types.AgentEvent{
			State:       types.StateResponding,
			FinalAnswer: llmResp.Explanation,
			ChunksFound: len(chunks),
		}, nil
	}

	// Execute functions through transaction executor
	results, execErr := a.txExecutor.ExecuteTransaction(ctx, llmResp.Functions)

	// Add to conversation context
	msg := types.Message{
		Role:      "user",
		Content:   query,
		Timestamp: time.Now(),
		Functions: results,
	}
	a.ctxManager.AddMessage(msg)

	// Build final answer with execution results
	finalAnswer := a.buildFinalAnswer(llmResp, results, execErr)

	// Return event with all results
	event := types.AgentEvent{
		State:       types.StateResponding,
		AllResults:  results,
		FinalAnswer: finalAnswer,
		ChunksFound: len(chunks),
	}

	// Add first tool call info for UI
	if len(llmResp.Functions) > 0 {
		event.ToolCall = &llmResp.Functions[0]
	}
	if len(results) > 0 {
		event.ToolResult = &results[0]
	}

	return event, nil
}

// buildFinalAnswer constructs a summary of the execution results.
func (a *Agent) buildFinalAnswer(llmResp *types.LLMResponse, results []types.ExecutionResult, execErr error) string {
	var sb strings.Builder

	// Reasoning
	if llmResp.Reasoning != "" {
		sb.WriteString("**Reasoning:**\n")
		sb.WriteString(llmResp.Reasoning)
		sb.WriteString("\n\n")
	}

	// Execution results
	if len(results) > 0 {
		sb.WriteString("**Execution Results:**\n")
		for i, result := range results {
			status := "✓"
			if !result.Success {
				status = "✗"
			}
			sb.WriteString(fmt.Sprintf("%d. %s %s", i+1, status, result.Function.Name))
			if result.Duration > 0 {
				sb.WriteString(fmt.Sprintf(" (%s)", result.Duration.Round(time.Millisecond)))
			}
			sb.WriteString("\n")

			if result.Success && result.Output != "" {
				// Truncate long output
				output := result.Output
				if len(output) > 500 {
					output = output[:500] + "..."
				}
				sb.WriteString(fmt.Sprintf("   %s\n", output))
			} else if !result.Success {
				sb.WriteString(fmt.Sprintf("   Error: %s\n", result.Error))
			}
		}
		sb.WriteString("\n")
	}

	// Execution error
	if execErr != nil {
		sb.WriteString(fmt.Sprintf("**Execution Warning:** %s\n\n", execErr.Error()))
	}

	// Explanation
	if llmResp.Explanation != "" {
		sb.WriteString("**Explanation:**\n")
		sb.WriteString(llmResp.Explanation)
	}

	return sb.String()
}

// Ping checks if the LLM is reachable.
func (a *Agent) Ping(ctx context.Context) error {
	// Simple health check - try to generate a minimal response
	_, err := a.llmClient.Generate(ctx, "Respond with OK")
	if err != nil {
		return fmt.Errorf("LLM not reachable: %w", err)
	}
	return nil
}

// ListTools returns available tool information.
func (a *Agent) ListTools() []types.ToolInfo {
	funcNames := a.functionRegistry.List()
	tools := make([]types.ToolInfo, 0, len(funcNames))

	for _, name := range funcNames {
		fn, exists := a.functionRegistry.Get(name)
		if !exists {
			continue
		}
		tools = append(tools, types.ToolInfo{
			Name:        fn.Name,
			Description: fn.Description,
			Category:    fn.Category,
			Parameters:  fn.Parameters,
		})
	}

	return tools
}

// ClearHistory clears the conversation history.
func (a *Agent) ClearHistory() {
	a.ctxManager.Clear()
}

// Close releases agent resources.
func (a *Agent) Close() error {
	if a.ragPipeline != nil {
		return a.ragPipeline.Close()
	}
	return nil
}

// LLMInfo returns information about the configured LLM.
func (a *Agent) LLMInfo() string {
	return fmt.Sprintf("%s @ %s", a.cfg.LLM.Model, a.cfg.LLM.Endpoint)
}

// truncate truncates a string to maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

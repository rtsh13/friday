// Package agent implements the core agent loop for the telemetry debugger.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/friday/internal/config"
	ctxmgr "github.com/friday/internal/context"
	"github.com/friday/internal/executor"
	"github.com/friday/internal/functions"
	"github.com/friday/internal/llm"
	"github.com/friday/internal/rag"
	"github.com/friday/internal/types"
	"github.com/friday/internal/validator"
	"go.uber.org/zap"
)

// Agent orchestrates the interaction between user, LLM, RAG, and tool execution.
type Agent struct {
	cfg              *config.Config
	ragPipeline      *rag.Pipeline
	llmClient        *llm.Client
	executor         *executor.Executor
	txExecutor       *executor.TransactionEngine
	functionRegistry *functions.Registry
	ctxManager       *ctxmgr.Manager
	inputValidator   *validator.InputValidator
	outputValidator  *validator.OutputValidator
	masterPromptPath string
	logger           *zap.Logger
}

// Config holds agent configuration.
type Config struct {
	AppConfig        *config.Config
	FunctionsPath    string
	MasterPromptPath string
	Logger           *zap.Logger
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

	if cfg.MasterPromptPath == "" {
		cfg.MasterPromptPath = "master_prompt.txt"
	}

	// Load function registry once — reused for both the agent and the transaction engine.
	funcRegistry, err := functions.LoadRegistry(cfg.FunctionsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load function registry: %w", err)
	}

	// Initialize RAG pipeline with ONNX embeddings.
	// Non-fatal: if ONNX runtime or Qdrant is unavailable the agent
	// continues without retrieval context (skipped gracefully in process()).
	ragPipeline, err := rag.NewPipeline(cfg.AppConfig, cfg.Logger)
	if err != nil {
		cfg.Logger.Warn("RAG pipeline unavailable, continuing without retrieval", zap.Error(err))
		ragPipeline = nil
	}

	// Initialize LLM client (vLLM) — pass temperature and max_tokens from config.
	llmClient := llm.NewClient(
		cfg.AppConfig.LLM.Endpoint,
		cfg.AppConfig.LLM.Model,
		time.Duration(cfg.AppConfig.LLM.TimeoutSeconds)*time.Second,
		cfg.AppConfig.LLM.Temperature,
		cfg.AppConfig.LLM.MaxTokens,
	)

	// Initialize executor components.
	exec := executor.NewExecutor(cfg.Logger)
	vRes := executor.NewVariableResolver()
	snapM := executor.NewSnapshotManager()

	txExec := executor.NewTransactionEngine(exec, vRes, snapM, funcRegistry)

	// Initialize context manager.
	ctxManager := ctxmgr.NewManager(cfg.AppConfig.Conversation.MaxMessages)

	// Initialize validators.
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
		masterPromptPath: cfg.MasterPromptPath,
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
	// Validate input.
	if err := a.inputValidator.Validate(query); err != nil {
		return types.AgentEvent{
			State: types.StateError,
			Error: fmt.Errorf("invalid input: %w", err),
		}, nil
	}

	sanitizedQuery := a.inputValidator.Sanitize(query)

	// Retrieve context from RAG.
	var chunks []types.RetrievedChunk
	if a.ragPipeline != nil {
		var ragErr error
		chunks, ragErr = a.ragPipeline.Retrieve(ctx, sanitizedQuery)
		if ragErr != nil {
			a.logger.Warn("RAG retrieval failed, continuing without context", zap.Error(ragErr))
			chunks = nil
		}
	}

	a.logger.Info("Retrieved context", zap.Int("chunks_found", len(chunks)))

	// Build full function definition slice from registry.
	funcDefs := make([]types.FunctionDefinition, 0, len(a.functionRegistry.Functions))
	for _, fn := range a.functionRegistry.Functions {
		funcDefs = append(funcDefs, fn)
	}

	// Build prompt using master_prompt.txt with all template variables substituted.
	prompt := llm.BuildPrompt(
		sanitizedQuery,
		chunks,
		funcDefs,
		a.ctxManager.GetMessages(),
		a.masterPromptPath,
	)

	// Call LLM.
	response, err := a.llmClient.Generate(ctx, prompt)
	if err != nil {
		return types.AgentEvent{
			State: types.StateError,
			Error: fmt.Errorf("LLM generation failed: %w", err),
		}, nil
	}

	// Parse and validate LLM response.
	llmResp, err := a.outputValidator.Validate(response, a.functionRegistry.Functions)
	if err != nil {
		a.logger.Warn("LLM response validation failed",
			zap.Error(err),
			zap.String("raw_response", truncate(response, 200)))

		return types.AgentEvent{
			State:       types.StateResponding,
			FinalAnswer: response,
			ChunksFound: len(chunks),
		}, nil
	}

	// If no functions to execute, return explanation directly.
	if len(llmResp.Functions) == 0 {
		return types.AgentEvent{
			State:       types.StateResponding,
			FinalAnswer: llmResp.Explanation,
			ChunksFound: len(chunks),
		}, nil
	}

	// Execute functions through the transaction engine.
	txReq := executor.TransactionRequest{
		Functions: llmResp.Functions,
		Strategy:  executor.ExecutionStrategy(llmResp.ExecutionStrategy),
	}
	txResults, execErr := a.txExecutor.ExecuteTransaction(ctx, txReq)

	// Flatten []executor.FunctionResult → []types.ExecutionResult.
	// Index is set explicitly so the UI skip-dedup logic works correctly.
	var results []types.ExecutionResult
	for i, fr := range txResults {
		outputStr := ""
		if fr.Output != nil {
			if b, jsonErr := json.Marshal(fr.Output); jsonErr == nil {
				outputStr = string(b)
			}
		}
		results = append(results, types.ExecutionResult{
			Index:    i,
			Function: types.FunctionCall{Name: fr.FunctionName},
			Output:   outputStr,
			Success:  fr.Success,
			Error:    errorString(fr.Error),
			Duration: fr.Duration,
		})
	}

	// Add sanitized query (not raw input) to conversation context.
	a.ctxManager.AddMessage(types.Message{
		Role:      "user",
		Content:   sanitizedQuery,
		Timestamp: time.Now(),
		Functions: results,
	})

	finalAnswer := a.buildFinalAnswer(llmResp, results, execErr)

	event := types.AgentEvent{
		State:       types.StateResponding,
		AllResults:  results,
		FinalAnswer: finalAnswer,
		ChunksFound: len(chunks),
	}

	if len(llmResp.Functions) > 0 {
		event.ToolCall = &llmResp.Functions[0]
	}
	if len(results) > 0 {
		event.ToolResult = &results[0]
	}

	return event, nil
}

// buildFinalAnswer constructs a human-readable summary of the execution results.
func (a *Agent) buildFinalAnswer(llmResp *types.LLMResponse, results []types.ExecutionResult, execErr error) string {
	var sb strings.Builder

	if llmResp.Reasoning != "" {
		sb.WriteString("**Reasoning:**\n")
		sb.WriteString(llmResp.Reasoning)
		sb.WriteString("\n\n")
	}

	if len(results) > 0 {
		sb.WriteString("**Execution Results:**\n")
		for i, result := range results {
			status := ""
			if !result.Success {
				status = "✗"
			}
			sb.WriteString(fmt.Sprintf("%d. %s %s", i+1, status, result.Function.Name))
			if result.Duration > 0 {
				sb.WriteString(fmt.Sprintf(" (%s)", result.Duration.Round(time.Millisecond)))
			}
			sb.WriteString("\n")

			if result.Success && result.Output != "" {
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

	if execErr != nil {
		sb.WriteString(fmt.Sprintf("**Execution Warning:** %s\n\n", execErr.Error()))
	}

	if llmResp.Explanation != "" {
		sb.WriteString("**Explanation:**\n")
		sb.WriteString(llmResp.Explanation)
	}

	return sb.String()
}

// Ping checks if the LLM is reachable.
func (a *Agent) Ping(ctx context.Context) error {
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

// ─── helpers ─────────────────────────────────────────────────────────────────

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

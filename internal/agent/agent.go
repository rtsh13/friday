// Package agent implements the core agent loop for cliche with RAG integration.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stratos/cliche/internal/ollama"
	"github.com/stratos/cliche/internal/rag"
	"github.com/stratos/cliche/internal/tools"
	"github.com/stratos/cliche/internal/types"
	"github.com/stratos/cliche/internal/ui"
	"github.com/stratos/cliche/pkg/models"
	"go.uber.org/zap"
)

// Agent orchestrates the interaction between user, RAG, LLM, and tools.
type Agent struct {
	llm      *ollama.Client
	rag      *rag.Pipeline // RAG pipeline for context retrieval
	registry *tools.Registry
	executor *tools.Executor
	history  []ollama.ChatMessage
	logger   *zap.Logger
}

// Config holds agent configuration.
type Config struct {
	OllamaConfig ollama.Config
	RAGConfig    *RAGConfig // Optional RAG configuration
}

// RAGConfig holds RAG pipeline configuration
type RAGConfig struct {
	QdrantHost        string
	QdrantPort        int
	EmbeddingEndpoint string
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		OllamaConfig: ollama.DefaultConfig(),
		RAGConfig:    nil, // RAG is optional
	}
}

// New creates a new agent with optional RAG integration.
func New(cfg Config) *Agent {
	logger, _ := zap.NewProduction()

	client := ollama.NewClient(cfg.OllamaConfig)
	registry := tools.NewRegistry()
	executor := tools.NewExecutor(registry)

	// Register default tools
	tools.RegisterNetworkingTools(registry)

	// Initialize RAG pipeline if configured
	var ragPipeline *rag.Pipeline
	if cfg.RAGConfig != nil {
		var err error
		ragPipeline, err = rag.NewPipeline(
			cfg.RAGConfig.QdrantHost,
			cfg.RAGConfig.QdrantPort,
			cfg.RAGConfig.EmbeddingEndpoint,
			logger,
		)
		if err != nil {
			logger.Warn("Failed to initialize RAG pipeline", zap.Error(err))
			ragPipeline = nil // Continue without RAG
		}
	}

	return &Agent{
		llm:      client,
		rag:      ragPipeline,
		registry: registry,
		executor: executor,
		history:  make([]ollama.ChatMessage, 0),
		logger:   logger,
	}
}

// ProcessQueryCmd returns a Bubble Tea command that processes a query.
// This allows the agent to run asynchronously and send events to the UI.
func (a *Agent) ProcessQueryCmd(query string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		event, err := a.process(ctx, query)
		if err != nil {
			return ui.AgentEventMsg{
				State: models.StateError,
				Error: err,
			}
		}
		return ui.AgentEventMsg(event)
	}
}

// process handles the actual query processing with RAG integration.
func (a *Agent) process(ctx context.Context, query string) (models.AgentEvent, error) {
	// Add user message to history
	a.history = append(a.history, ollama.ChatMessage{
		Role:    "user",
		Content: query,
	})

	// Step 1: Retrieve context from RAG (optional but improves reasoning)
	ragContext := ""
	if a.rag != nil {
		chunks, err := a.rag.Retrieve(ctx, query)
		if err != nil {
			a.logger.Warn("RAG retrieval failed", zap.Error(err))
		} else if len(chunks) > 0 {
			ragContext = a.formatRAGContext(chunks)
			a.logger.Info("Retrieved RAG context", zap.Int("chunks", len(chunks)))
		}
	}

	// Step 2: Build messages with system prompt and optional RAG context
	messages := a.buildMessages(ragContext)

	// Step 3: Call LLM
	resp, err := a.llm.Chat(ctx, messages)
	if err != nil {
		return models.AgentEvent{}, fmt.Errorf("LLM error: %w", err)
	}

	llmResponse := resp.Message.Content

	// Step 4: Check if LLM wants to use a tool
	toolCall, hasToolCall := a.parseToolCall(llmResponse)

	if hasToolCall {
		// Execute the tool
		result := a.executor.Execute(ctx, toolCall.Name, toolCall.Params)

		// Add tool interaction to history
		a.history = append(a.history, ollama.ChatMessage{
			Role:    "assistant",
			Content: fmt.Sprintf(`{"tool": "%s", "params": %s}`, toolCall.Name, mustMarshal(toolCall.Params)),
		})
		a.history = append(a.history, ollama.ChatMessage{
			Role:    "user",
			Content: fmt.Sprintf("Tool '%s' returned:\n%s", toolCall.Name, formatToolResult(result)),
		})

		// Get LLM to interpret the result (with RAG context still available)
		messages = a.buildMessages(ragContext)
		interpretResp, err := a.llm.Chat(ctx, messages)
		if err != nil {
			// Return tool result even if interpretation fails
			return models.AgentEvent{
				State:       models.StateResponding,
				ToolCall:    &toolCall,
				ToolResult:  &result,
				FinalAnswer: formatToolResult(result),
			}, nil
		}

		// Add interpretation to history
		a.history = append(a.history, ollama.ChatMessage{
			Role:    "assistant",
			Content: interpretResp.Message.Content,
		})

		return models.AgentEvent{
			State:       models.StateResponding,
			ToolCall:    &toolCall,
			ToolResult:  &result,
			FinalAnswer: interpretResp.Message.Content,
		}, nil
	}

	// No tool call, just return the response
	a.history = append(a.history, ollama.ChatMessage{
		Role:    "assistant",
		Content: llmResponse,
	})

	return models.AgentEvent{
		State:       models.StateResponding,
		FinalAnswer: llmResponse,
	}, nil
}

// buildMessages creates the full message list with system prompt and optional RAG context.
func (a *Agent) buildMessages(ragContext string) []ollama.ChatMessage {
	systemPrompt := a.buildSystemPrompt(ragContext)

	messages := make([]ollama.ChatMessage, 0, len(a.history)+1)
	messages = append(messages, ollama.ChatMessage{
		Role:    "system",
		Content: systemPrompt,
	})
	messages = append(messages, a.history...)

	return messages
}

// buildSystemPrompt creates the system prompt including RAG context and tool definitions.
func (a *Agent) buildSystemPrompt(ragContext string) string {
	basePrompt := `You are cliche, an AI-powered CLI assistant for DevOps engineers and SREs.
Your job is to help debug networking, filesystem, process, and infrastructure issues.

Guidelines:
- Be concise but thorough
- Use provided context to inform your answers
- When diagnosing issues, use available tools to gather real data
- Explain what you find and suggest fixes
- If a tool fails, try alternative approaches
- Always interpret tool results in the context of the user's question

`

	// Append RAG context if available
	if ragContext != "" {
		basePrompt += "## Context from Knowledge Base\n" + ragContext + "\n\n"
	}

	toolsPrompt := a.registry.GenerateToolsPrompt()
	return basePrompt + toolsPrompt
}

// formatRAGContext formats retrieved chunks for inclusion in system prompt
func (a *Agent) formatRAGContext(chunks []types.RetrievedChunk) string {
	if len(chunks) == 0 {
		return ""
	}

	context := ""
	for i, chunk := range chunks {
		context += fmt.Sprintf("[%d] Source: %s (Category: %s, Relevance: %.2f)\n",
			i+1, chunk.Source, chunk.Category, chunk.Score)
		context += fmt.Sprintf("Content: %s\n\n", chunk.Content)
	}

	return context
}

// parseToolCall attempts to extract a tool call from the LLM response.
func (a *Agent) parseToolCall(response string) (models.ToolCall, bool) {
	// Clean up response - remove markdown code blocks if present
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	// Try direct JSON parse first
	var parsed struct {
		Tool   string            `json:"tool"`
		Params map[string]string `json:"params"`
	}

	if err := json.Unmarshal([]byte(response), &parsed); err == nil && parsed.Tool != "" {
		return models.ToolCall{
			Name:   parsed.Tool,
			Params: parsed.Params,
		}, true
	}

	// Try to find JSON in the response
	jsonRegex := regexp.MustCompile(`\{[^{}]*"tool"[^{}]*\}`)
	matches := jsonRegex.FindAllString(response, -1)

	for _, match := range matches {
		if err := json.Unmarshal([]byte(match), &parsed); err == nil && parsed.Tool != "" {
			return models.ToolCall{
				Name:   parsed.Tool,
				Params: parsed.Params,
			}, true
		}
	}

	// Try to find nested JSON
	start := strings.Index(response, `{"tool"`)
	if start == -1 {
		return models.ToolCall{}, false
	}

	depth := 0
	end := -1
	for i := start; i < len(response); i++ {
		if response[i] == '{' {
			depth++
		} else if response[i] == '}' {
			depth--
			if depth == 0 {
				end = i + 1
				break
			}
		}
	}

	if end == -1 {
		return models.ToolCall{}, false
	}

	jsonStr := response[start:end]
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err == nil && parsed.Tool != "" {
		return models.ToolCall{
			Name:   parsed.Tool,
			Params: parsed.Params,
		}, true
	}

	return models.ToolCall{}, false
}

// Ping checks if the LLM is reachable.
func (a *Agent) Ping(ctx context.Context) error {
	return a.llm.Ping(ctx)
}

// ListTools returns available tool names.
func (a *Agent) ListTools() []string {
	return a.registry.List()
}

// ClearHistory clears the conversation history.
func (a *Agent) ClearHistory() {
	a.history = make([]ollama.ChatMessage, 0)
}

// LLMInfo returns information about the configured LLM.
func (a *Agent) LLMInfo() string {
	return a.llm.ModelInfo()
}

// formatToolResult formats a tool result for display.
func formatToolResult(r models.ToolResult) string {
	if r.Success {
		return r.Output
	}
	return fmt.Sprintf("Error: %s\n%s", r.Error, r.Output)
}

// mustMarshal marshals to JSON or returns empty object.
func mustMarshal(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

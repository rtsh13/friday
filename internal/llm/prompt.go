package llm

import (
	"fmt"
	"os"
	"strings"

	"github.com/stratos/cliche/internal/types"
)

const defaultMasterPromptPath = "master_prompt.txt"

// BuildPrompt loads master_prompt.txt and substitutes all four template
// variables. Falls back to a minimal inline prompt if the file cannot be read.
func BuildPrompt(
	query string,
	chunks []types.RetrievedChunk,
	functions []types.FunctionDefinition,
	history []types.Message,
	masterPromptPath string,
) string {
	if masterPromptPath == "" {
		masterPromptPath = defaultMasterPromptPath
	}

	raw, err := os.ReadFile(masterPromptPath)
	if err != nil {
		// Graceful degradation: build a minimal but still useful prompt.
		return buildFallbackPrompt(query, chunks, functions)
	}

	prompt := string(raw)
	prompt = strings.ReplaceAll(prompt, "{{FUNCTION_REGISTRY}}", buildFunctionRegistry(functions))
	prompt = strings.ReplaceAll(prompt, "{{RETRIEVED_CONTEXT}}", buildRetrievedContext(chunks))
	prompt = strings.ReplaceAll(prompt, "{{CONVERSATION_HISTORY}}", buildConversationHistory(history))
	prompt = strings.ReplaceAll(prompt, "{{USER_QUERY}}", query)
	return prompt
}

// ─── template section builders ────────────────────────────────────────────────

// buildFunctionRegistry formats the full function registry including parameter
// details so the model knows exact names, types, and whether they are required.
func buildFunctionRegistry(functions []types.FunctionDefinition) string {
	if len(functions) == 0 {
		return "No functions available."
	}

	var sb strings.Builder
	for _, fn := range functions {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", fn.Name, fn.Description))
		if len(fn.Parameters) > 0 {
			sb.WriteString("  Parameters:\n")
			for _, p := range fn.Parameters {
				req := "optional"
				if p.Required {
					req = "required"
				}
				line := fmt.Sprintf("    - %s (%s, %s)", p.Name, p.Type, req)
				if p.Description != "" {
					line += ": " + p.Description
				}
				if p.Default != nil {
					line += fmt.Sprintf(" [default: %v]", p.Default)
				}
				sb.WriteString(line + "\n")
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// buildRetrievedContext formats RAG chunks for insertion into the prompt.
func buildRetrievedContext(chunks []types.RetrievedChunk) string {
	if len(chunks) == 0 {
		return "No relevant documentation found."
	}

	var sb strings.Builder
	for i, chunk := range chunks {
		preview := chunk.Content
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		sb.WriteString(fmt.Sprintf("[%d] Source: %s (score: %.2f)\n%s\n\n",
			i+1, chunk.Source, chunk.Score, preview))
	}
	return sb.String()
}

// buildConversationHistory formats prior messages including tool results so the
// model can reference earlier diagnostics and avoid redundant calls.
func buildConversationHistory(history []types.Message) string {
	if len(history) == 0 {
		return "No previous conversation."
	}

	var sb strings.Builder
	for _, msg := range history {
		role := capitalize(msg.Role)
		sb.WriteString(fmt.Sprintf("%s: %s\n", role, msg.Content))

		if len(msg.Functions) > 0 {
			sb.WriteString("  Tool Results:\n")
			for _, fn := range msg.Functions {
				status := "✓"
				if !fn.Success {
					status = "✗"
				}
				output := truncateHistory(fn.Output, 200)
				sb.WriteString(fmt.Sprintf("    %s %s: %s\n", status, fn.Function.Name, output))
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// buildFallbackPrompt is used when master_prompt.txt cannot be read.
// It produces a compact but still structured prompt so the agent stays functional.
func buildFallbackPrompt(
	query string,
	chunks []types.RetrievedChunk,
	functions []types.FunctionDefinition,
) string {
	var sb strings.Builder

	sb.WriteString("You are an expert telemetry debugging assistant. Respond ONLY in valid JSON.\n\n")
	sb.WriteString(`Required JSON structure:
{
  "reasoning": "Your diagnostic reasoning",
  "execution_strategy": "stop_on_error",
  "functions": [
    {"name": "function_name", "params": {"param": "value"}, "critical": false}
  ],
  "explanation": "User-friendly explanation"
}

`)

	sb.WriteString("Available functions:\n")
	for _, fn := range functions {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", fn.Name, fn.Description))
		for _, p := range fn.Parameters {
			req := ""
			if p.Required {
				req = " (required)"
			}
			sb.WriteString(fmt.Sprintf("  - %s (%s)%s\n", p.Name, p.Type, req))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Retrieved context:\n")
	for i, chunk := range chunks {
		preview := chunk.Content
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		sb.WriteString(fmt.Sprintf("[%d] %s (%.2f)\n%s\n\n", i+1, chunk.Source, chunk.Score, preview))
	}

	sb.WriteString(fmt.Sprintf("User Query: %s\n\nRespond with valid JSON only.", query))
	return sb.String()
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func truncateHistory(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
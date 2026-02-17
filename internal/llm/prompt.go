package llm

import (
	"fmt"
	"strings"

	"github.com/stratos/cliche/internal/types"
)

func BuildPrompt(query string, chunks []types.RetrievedChunk, functionNames []string) string {
	var sb strings.Builder

	sb.WriteString(`You are an expert telemetry debugging assistant. Respond ONLY in valid JSON.

Required JSON structure:
{
  "reasoning": "Your diagnostic reasoning",
  "execution_strategy": "stop_on_error",
  "functions": [
    {"name": "function_name", "params": {"param": "value"}, "critical": false}
  ],
  "explanation": "User-friendly explanation"
}

Available functions:
`)

	for _, name := range functionNames {
		sb.WriteString(fmt.Sprintf("- %s\n", name))
	}

	sb.WriteString("\nRetrieved context:\n")
	for i, chunk := range chunks {
		preview := chunk.Content
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		sb.WriteString(fmt.Sprintf("[%d] %s (%.2f)\n%s\n\n", i+1, chunk.Source, chunk.Score, preview))
	}

	sb.WriteString(fmt.Sprintf("\nUser Query: %s\n\nRespond with valid JSON only.", query))

	return sb.String()
}

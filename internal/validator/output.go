package validator

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/friday/internal/types"
)

type OutputValidator struct{}

func NewOutputValidator() *OutputValidator {
	return &OutputValidator{}
}

func (v *OutputValidator) Validate(response string, availableFunctions map[string]types.FunctionDefinition) (*types.LLMResponse, error) {
	sanitized := sanitizeJSONString(response)

	var llmResp types.LLMResponse
	if err := json.Unmarshal([]byte(sanitized), &llmResp); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	if llmResp.Reasoning == "" {
		return nil, fmt.Errorf("missing reasoning field")
	}

	if llmResp.Explanation == "" {
		return nil, fmt.Errorf("missing explanation field")
	}

	for i, fn := range llmResp.Functions {
		if _, exists := availableFunctions[fn.Name]; !exists {
			return nil, fmt.Errorf("unknown function '%s' at index %d", fn.Name, i)
		}
	}

	return &llmResp, nil
}

// sanitizeJSONString fixes common LLM output issues before unmarshaling:
// - Strips markdown code fences (```json ... ```)
// - Escapes bare newlines and tabs inside JSON string values
func sanitizeJSONString(s string) string {
	s = strings.TrimSpace(s)

	// Strip markdown code fences the model sometimes emits.
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s, "\n"); idx != -1 {
			s = s[idx+1:]
		}
		if idx := strings.LastIndex(s, "```"); idx != -1 {
			s = s[:idx]
		}
		s = strings.TrimSpace(s)
	}

	// Walk the string and escape bare control characters inside JSON string values.
	var sb strings.Builder
	sb.Grow(len(s))

	inString := false
	escaped := false

	for i := 0; i < len(s); i++ {
		c := s[i]

		if escaped {
			sb.WriteByte(c)
			escaped = false
			continue
		}

		if c == '\\' && inString {
			sb.WriteByte(c)
			escaped = true
			continue
		}

		if c == '"' {
			inString = !inString
			sb.WriteByte(c)
			continue
		}

		if inString {
			switch c {
			case '\n':
				sb.WriteString(`\n`)
			case '\r':
				sb.WriteString(`\r`)
			case '\t':
				sb.WriteString(`\t`)
			default:
				sb.WriteByte(c)
			}
			continue
		}

		sb.WriteByte(c)
	}

	return sb.String()
}
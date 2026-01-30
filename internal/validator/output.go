package validator

import (
	"encoding/json"
	"fmt"

	"github.com/ashutoshrp06/telemetry-debugger/internal/types"
)

type OutputValidator struct{}

func NewOutputValidator() *OutputValidator {
	return &OutputValidator{}
}

func (v *OutputValidator) Validate(response string, availableFunctions map[string]types.FunctionDefinition) (*types.LLMResponse, error) {
	var llmResp types.LLMResponse
	if err := json.Unmarshal([]byte(response), &llmResp); err != nil {
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
package executor

import (
	"fmt"
	"regexp"
	"strings"
)

type VariableResolver struct {
	executionContext map[string]interface{}
}

func NewVariableResolver() *VariableResolver {
	return &VariableResolver{
		executionContext: make(map[string]interface{}),
	}
}

func (vr *VariableResolver) AddResult(functionName string, result interface{}) {
	vr.executionContext[functionName] = result
}

func (vr *VariableResolver) Resolve(value string) (string, error) {
	varPattern := regexp.MustCompile(`\$\{([^}]+)\}`)
	
	matches := varPattern.FindAllStringSubmatch(value, -1)
	
	result := value
	for _, match := range matches {
		varRef := match[1]
		resolved, err := vr.resolveReference(varRef)
		if err != nil {
			return "", err
		}
		result = strings.ReplaceAll(result, match[0], fmt.Sprintf("%v", resolved))
	}
	
	return result, nil
}

func (vr *VariableResolver) resolveReference(ref string) (interface{}, error) {
	// Placeholder: In real implementation, parse and resolve references
	// like "previous.port" or "func[0].result"
	return ref, nil
}
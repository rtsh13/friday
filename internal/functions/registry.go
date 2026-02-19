package functions

import (
	"os"

	"github.com/friday/internal/types"
	"gopkg.in/yaml.v3"
)

type Registry struct {
	Functions map[string]types.FunctionDefinition
}

func LoadRegistry(path string) (*Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config struct {
		Functions []types.FunctionDefinition `yaml:"functions"`
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	registry := &Registry{
		Functions: make(map[string]types.FunctionDefinition),
	}

	for _, fn := range config.Functions {
		registry.Functions[fn.Name] = fn
	}

	return registry, nil
}

func (r *Registry) Get(name string) (types.FunctionDefinition, bool) {
	fn, exists := r.Functions[name]
	return fn, exists
}

func (r *Registry) List() []string {
	names := make([]string, 0, len(r.Functions))
	for name := range r.Functions {
		names = append(names, name)
	}
	return names
}

func (r *Registry) Phase(functionName string) string {
	if fn, exists := r.Functions[functionName]; exists {
		return fn.Phase
	}
	return "read"
}

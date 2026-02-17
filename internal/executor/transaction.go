package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/stratos/cliche/internal/types"
)

// TransactionExecutor runs a list of function calls sequentially, resolving
// ${previous_function.field} variable references between steps.
type TransactionExecutor struct {
	executor *Executor
}

// NewTransactionExecutor creates a new transaction executor.
func NewTransactionExecutor(executor *Executor) *TransactionExecutor {
	return &TransactionExecutor{
		executor: executor,
	}
}

// ExecuteTransaction executes a list of function calls in order.
//
// Variable resolution: any parameter value matching ${func_name.field} is
// replaced with the corresponding output field of a previously-executed
// function before that call is dispatched. Nested paths are supported:
// ${check_tcp_health.recommended_buffer_size}, ${grpc.latency_ms}, etc.
//
// Execution stops on the first error from a function whose Critical flag is
// true, or when a non-critical function fails and the error cannot be
// recovered. The partial results collected so far are always returned.
func (te *TransactionExecutor) ExecuteTransaction(
	ctx context.Context,
	functions []types.FunctionCall,
) ([]types.ExecutionResult, error) {
	resolver := NewVariableResolver()
	results := make([]types.ExecutionResult, 0, len(functions))

	for i, fn := range functions {
		// Check context before each step.
		select {
		case <-ctx.Done():
			return results, fmt.Errorf("execution cancelled at step %d (%s): %w", i, fn.Name, ctx.Err())
		default:
		}

		// Resolve variable references in this function's parameters.
		resolvedFn, err := te.resolveFunction(fn, resolver)
		if err != nil {
			result := types.ExecutionResult{
				Index:    i,
				Function: fn,
				Success:  false,
				Error:    fmt.Sprintf("variable resolution failed: %v", err),
			}
			results = append(results, result)
			return results, fmt.Errorf("step %d (%s): %w", i, fn.Name, err)
		}

		// Execute.
		start := time.Now()
		output, execErr := te.executor.Execute(resolvedFn)
		duration := time.Since(start)

		result := types.ExecutionResult{
			Index:    i,
			Function: resolvedFn, // store with resolved params so callers see actual values used
			Success:  execErr == nil,
			Output:   output,
			Duration: duration,
		}
		if execErr != nil {
			result.Error = execErr.Error()
		}

		results = append(results, result)

		// On success, register output so later steps can reference it.
		if execErr == nil && output != "" {
			resolver.AddResult(fn.Name, output)
		}

		// Stop the chain on failure.
		if execErr != nil {
			return results, fmt.Errorf("step %d (%s) failed: %w", i, fn.Name, execErr)
		}
	}

	return results, nil
}

// resolveFunction returns a copy of fn with all parameter variables resolved.
// If the function has no variable references, fn is returned unchanged.
func (te *TransactionExecutor) resolveFunction(
	fn types.FunctionCall,
	resolver *VariableResolver,
) (types.FunctionCall, error) {
	if !ContainsVariables(fn.Params) {
		return fn, nil
	}

	resolvedParams, err := resolver.ResolveParams(fn.Params)
	if err != nil {
		return fn, fmt.Errorf("resolving params for %s: %w", fn.Name, err)
	}

	// Return a shallow copy with resolved params.
	return types.FunctionCall{
		Name:      fn.Name,
		Params:    resolvedParams,
		Critical:  fn.Critical,
		DependsOn: fn.DependsOn,
	}, nil
}

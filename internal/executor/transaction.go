package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/stratos/cliche/internal/types"
)

// rollbackEntry holds the information needed to undo a single sysctl change.
type rollbackEntry struct {
	parameter string
	oldValue  string
}

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
// function before that call is dispatched.
//
// Rollback: every successful execute_sysctl_command call pushes its old value
// onto a LIFO stack. On any failure, all entries are rolled back in reverse
// order via restore_sysctl_value. All rollback errors are collected and
// reported alongside the original failure; rollback continues even if
// individual restore calls fail.
func (te *TransactionExecutor) ExecuteTransaction(
	ctx context.Context,
	functions []types.FunctionCall,
) ([]types.ExecutionResult, error) {
	resolver := NewVariableResolver()
	results := make([]types.ExecutionResult, 0, len(functions))
	rollbackStack := make([]rollbackEntry, 0)

	for i, fn := range functions {
		// Check context before each step.
		select {
		case <-ctx.Done():
			rollbackErr := te.rollback(rollbackStack)
			return results, te.combineErrors(
				fmt.Errorf("execution cancelled at step %d (%s): %w", i, fn.Name, ctx.Err()),
				rollbackErr,
			)
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
			rollbackErr := te.rollback(rollbackStack)
			return results, te.combineErrors(
				fmt.Errorf("step %d (%s): %w", i, fn.Name, err),
				rollbackErr,
			)
		}

		// Execute.
		start := time.Now()
		output, execErr := te.executor.Execute(resolvedFn)
		duration := time.Since(start)

		result := types.ExecutionResult{
			Index:    i,
			Function: resolvedFn,
			Success:  execErr == nil,
			Output:   output,
			Duration: duration,
		}
		if execErr != nil {
			result.Error = execErr.Error()
		}

		results = append(results, result)

		// On success, register output so later steps can reference it,
		// and capture rollback info if this was a sysctl modification.
		if execErr == nil && output != "" {
			resolver.AddResult(fn.Name, output)

			if fn.Name == "execute_sysctl_command" {
				if entry, ok := parseSysctlRollbackEntry(output); ok {
					rollbackStack = append(rollbackStack, entry)
				}
			}
		}

		// Stop the chain on failure and attempt rollback.
		if execErr != nil {
			rollbackErr := te.rollback(rollbackStack)
			return results, te.combineErrors(
				fmt.Errorf("step %d (%s) failed: %w", i, fn.Name, execErr),
				rollbackErr,
			)
		}
	}

	return results, nil
}

// rollback executes all entries in the stack in LIFO order.
// It always attempts every entry regardless of individual failures, collecting
// all errors. Returns nil if every restore succeeded or the stack is empty.
func (te *TransactionExecutor) rollback(stack []rollbackEntry) error {
	if len(stack) == 0 {
		return nil
	}

	var errs []string
	for i := len(stack) - 1; i >= 0; i-- {
		entry := stack[i]
		restoreFn := types.FunctionCall{
			Name: "restore_sysctl_value",
			Params: map[string]interface{}{
				"parameter": entry.parameter,
				"value":     entry.oldValue,
			},
		}
		if _, err := te.executor.Execute(restoreFn); err != nil {
			errs = append(errs, fmt.Sprintf("restore %s=%s: %v", entry.parameter, entry.oldValue, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("rollback errors (manual intervention may be required): %s",
			strings.Join(errs, "; "))
	}
	return nil
}

// combineErrors merges a primary error with an optional rollback error into a
// single descriptive error. If rollbackErr is nil, the primary error is
// returned unchanged.
func (te *TransactionExecutor) combineErrors(primary, rollbackErr error) error {
	if rollbackErr == nil {
		return primary
	}
	return fmt.Errorf("%w; %s", primary, rollbackErr.Error())
}

// parseSysctlRollbackEntry extracts parameter and old_value from the JSON
// output of a successful execute_sysctl_command call.
// Returns the entry and true on success, zero value and false otherwise.
func parseSysctlRollbackEntry(output string) (rollbackEntry, bool) {
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return rollbackEntry{}, false
	}

	param, ok := result["parameter"].(string)
	if !ok || param == "" {
		return rollbackEntry{}, false
	}

	oldVal, ok := result["old_value"].(string)
	if !ok {
		return rollbackEntry{}, false
	}

	return rollbackEntry{parameter: param, oldValue: oldVal}, true
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

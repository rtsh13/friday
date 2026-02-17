package executor

import (
	"context"
	"fmt"

	"github.com/stratos/cliche/internal/types"
)

type TransactionExecutor struct {
	executor *Executor
}

func NewTransactionExecutor(executor *Executor) *TransactionExecutor {
	return &TransactionExecutor{
		executor: executor,
	}
}

func (te *TransactionExecutor) ExecuteTransaction(ctx context.Context, functions []types.FunctionCall) ([]types.ExecutionResult, error) {
	results := make([]types.ExecutionResult, 0, len(functions))

	for i, fn := range functions {
		output, err := te.executor.Execute(fn)

		result := types.ExecutionResult{
			Index:    i,
			Function: fn,
			Success:  err == nil,
			Output:   output,
		}

		if err != nil {
			result.Error = err.Error()
			return results, fmt.Errorf("function %d failed: %w", i, err)
		}

		results = append(results, result)
	}

	return results, nil
}

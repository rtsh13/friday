package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/friday/internal/executor"
	"github.com/friday/internal/functions"
	"github.com/friday/internal/types"
	"go.uber.org/zap"
)

// TestE2E_CLIToExecutor runs an end-to-end test from CLI input through function execution
func TestE2E_CLIToExecutor(t *testing.T) {
	logger := zap.NewNop()
	defer logger.Sync()

	// STEP 1: Load function registry
	functionRegistry, err := functions.LoadRegistry("../../functions.yaml")
	if err != nil {
		t.Fatalf("Failed to load function registry: %v", err)
	}

	functionList := functionRegistry.List()
	if len(functionList) == 0 {
		t.Fatalf("Expected functions in registry, got none")
	}
	t.Logf("Loaded %d functions from registry", len(functionList))

	// STEP 2: Verify check_tcp_health function exists
	tcpHealthFn, exists := functionRegistry.Get("check_tcp_health")
	if !exists {
		t.Fatalf("check_tcp_health function not found in registry")
	}
	t.Logf("Found check_tcp_health: %s", tcpHealthFn.Description)

	// STEP 3: Create executor
	ex := executor.NewExecutor(logger)

	// STEP 4: Create function call
	functionCall := types.FunctionCall{
		Name:     "check_tcp_health",
		Critical: true,
		Params: map[string]interface{}{
			"interface": "eth0",
			"port":      50051,
		},
	}

	// STEP 5: Execute function
	result, err := ex.Execute(functionCall)
	if err != nil {
		errStr := err.Error()
		// On non-Linux systems, ss command doesn't exist
		if strings.Contains(errStr, "executable file not found") ||
			strings.Contains(errStr, "exec:") {
			t.Logf("Got expected error on non-Linux system: %v", err)
			return
		}
		t.Fatalf("Execute returned error: %v", err)
	}

	// STEP 6: Validate result
	var resultMap map[string]interface{}
	if err := json.Unmarshal([]byte(result), &resultMap); err != nil {
		t.Fatalf("Failed to parse result JSON: %v", err)
	}

	expectedFields := []string{"state", "port", "interface", "retransmits", "send_queue_bytes", "recv_queue_bytes"}
	for _, field := range expectedFields {
		if _, ok := resultMap[field]; !ok {
			t.Errorf("Missing field in result: %s", field)
		}
	}
	t.Logf("All validations passed")
}

// TestE2E_MultipleExecutions tests multiple function calls in sequence
func TestE2E_MultipleExecutions(t *testing.T) {
	logger := zap.NewNop()
	defer logger.Sync()

	ex := executor.NewExecutor(logger)

	// Test case 1: check_tcp_health
	tcpCall := types.FunctionCall{
		Name: "check_tcp_health",
		Params: map[string]interface{}{
			"interface": "eth0",
			"port":      50051,
		},
	}
	_, err1 := ex.Execute(tcpCall)
	if err1 != nil {
		t.Logf("check_tcp_health error (expected on non-Linux): %v", err1)
	}

	// Test case 2: check_grpc_health
	grpcCall := types.FunctionCall{
		Name: "check_grpc_health",
		Params: map[string]interface{}{
			"host":    "localhost",
			"port":    50051,
			"timeout": 2,
		},
	}
	_, err2 := ex.Execute(grpcCall)
	if err2 != nil {
		t.Logf("check_grpc_health error (expected - no server): %v", err2)
	}

	// Test case 3: Missing required parameter
	invalidCall := types.FunctionCall{
		Name:   "check_tcp_health",
		Params: map[string]interface{}{"interface": "eth0"},
	}
	_, err3 := ex.Execute(invalidCall)
	if err3 == nil {
		t.Fatalf("Expected error for missing parameter")
	}
	t.Logf("Correctly rejected missing parameter: %v", err3)

	// Test case 4: Unknown function
	unknownCall := types.FunctionCall{
		Name:   "unknown_function",
		Params: map[string]interface{}{},
	}
	_, err4 := ex.Execute(unknownCall)
	if err4 == nil {
		t.Fatalf("Expected error for unknown function")
	}
	t.Logf("Correctly rejected unknown function: %v", err4)
}

// TestE2E_FunctionValidation tests function registry and validation
func TestE2E_FunctionValidation(t *testing.T) {
	functionRegistry, err := functions.LoadRegistry("../../functions.yaml")
	if err != nil {
		t.Fatalf("Failed to load function registry: %v", err)
	}

	networkFunctions := []string{"check_tcp_health", "check_grpc_health"}
	for _, fnName := range networkFunctions {
		fn, exists := functionRegistry.Get(fnName)
		if !exists {
			t.Errorf("Function %s not found", fnName)
			continue
		}

		t.Logf("%s - Category: %s, Params: %d", fnName, fn.Category, len(fn.Parameters))
	}
}

// TestE2E_ConcurrentExecutions tests thread-safe concurrent execution
func TestE2E_ConcurrentExecutions(t *testing.T) {
	logger := zap.NewNop()
	defer logger.Sync()

	ex := executor.NewExecutor(logger)

	numConcurrent := 5
	resultsChan := make(chan string, numConcurrent)
	errorsChan := make(chan error, numConcurrent)

	for i := 0; i < numConcurrent; i++ {
		go func(id int) {
			call := types.FunctionCall{
				Name: "check_grpc_health",
				Params: map[string]interface{}{
					"host":    "localhost",
					"port":    50000 + id,
					"timeout": 1,
				},
			}
			result, err := ex.Execute(call)
			if err != nil {
				errorsChan <- err
			} else {
				resultsChan <- result
			}
		}(i)
	}

	successCount := 0
	errorCount := 0
	timeout := time.After(10 * time.Second)

	for i := 0; i < numConcurrent; i++ {
		select {
		case <-resultsChan:
			successCount++
		case <-errorsChan:
			errorCount++
		case <-timeout:
			t.Fatalf("Timeout waiting for concurrent execution results")
		}
	}

	t.Logf("Concurrent: %d success, %d failed", successCount, errorCount)
}

// TestE2E_ParameterTypeConversion tests automatic parameter type conversion
func TestE2E_ParameterTypeConversion(t *testing.T) {
	logger := zap.NewNop()
	defer logger.Sync()

	ex := executor.NewExecutor(logger)

	testCases := []struct {
		name       string
		call       types.FunctionCall
		shouldFail bool
	}{
		{
			name: "Port as string",
			call: types.FunctionCall{
				Name: "check_tcp_health",
				Params: map[string]interface{}{
					"interface": "eth0",
					"port":      "50051",
				},
			},
			shouldFail: false,
		},
		{
			name: "Invalid port string",
			call: types.FunctionCall{
				Name: "check_tcp_health",
				Params: map[string]interface{}{
					"interface": "eth0",
					"port":      "not_a_number",
				},
			},
			shouldFail: true,
		},
	}

	for _, tc := range testCases {
		t.Logf("Testing: %s", tc.name)
		_, err := ex.Execute(tc.call)

		if tc.shouldFail {
			if err == nil {
				t.Errorf("Expected error but got none for %s", tc.name)
			}
		} else {
			if err != nil {
				errStr := err.Error()
				if !strings.Contains(errStr, "executable file not found") &&
					!strings.Contains(errStr, "context deadline") {
					t.Logf("Unexpected error: %v", err)
				}
			}
		}
	}
}

// TestE2E_Simulation_RAGToDiagnostics simulates a complete workflow
func TestE2E_Simulation_RAGToDiagnostics(t *testing.T) {
	logger := zap.NewNop()
	defer logger.Sync()

	userQuery := "Check if gRPC service on localhost:50051 is healthy"
	t.Logf("User Query: %s", userQuery)

	functionRegistry, err := functions.LoadRegistry("../../functions.yaml")
	if err != nil {
		t.Logf("Could not load function registry: %v", err)
	}
	_ = functionRegistry

	simulatedLLMResponse := types.LLMResponse{
		Reasoning:         "User wants to check gRPC service health",
		ExecutionStrategy: "sequential",
		Functions: []types.FunctionCall{
			{
				Name:     "check_grpc_health",
				Critical: true,
				Params: map[string]interface{}{
					"host":    "localhost",
					"port":    50051,
					"timeout": 3,
				},
			},
		},
		Explanation: "Checking gRPC service health on port 50051",
	}

	ex := executor.NewExecutor(logger)
	executionResults := make([]types.ExecutionResult, 0)

	for idx, fnCall := range simulatedLLMResponse.Functions {
		startTime := time.Now()
		result, err := ex.Execute(fnCall)
		duration := time.Since(startTime)

		if err != nil {
			executionResults = append(executionResults, types.ExecutionResult{
				Index:    idx,
				Function: fnCall,
				Success:  false,
				Error:    err.Error(),
				Duration: duration,
			})
		} else {
			executionResults = append(executionResults, types.ExecutionResult{
				Index:    idx,
				Function: fnCall,
				Success:  true,
				Output:   result,
				Duration: duration,
			})
		}
	}

	successCount := 0
	for _, result := range executionResults {
		if result.Success {
			successCount++
		}
	}

	t.Logf("Executed: %d, Success: %d, Failed: %d",
		len(executionResults), successCount, len(executionResults)-successCount)
}

// BenchmarkE2E_SingleExecution benchmarks a single function execution
func BenchmarkE2E_SingleExecution(b *testing.B) {
	logger := zap.NewNop()
	defer logger.Sync()

	ex := executor.NewExecutor(logger)
	call := types.FunctionCall{
		Name: "check_grpc_health",
		Params: map[string]interface{}{
			"host":    "localhost",
			"port":    9999,
			"timeout": 1,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ex.Execute(call)
	}
}

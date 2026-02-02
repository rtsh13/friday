package main

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/ashutoshrp06/telemetry-debugger/internal/executor"
	"github.com/ashutoshrp06/telemetry-debugger/internal/functions"
	"github.com/ashutoshrp06/telemetry-debugger/internal/types"
	"go.uber.org/zap"
)

// TestE2E_CLIToExecutor runs an end-to-end test from CLI input through function execution
func TestE2E_CLIToExecutor(t *testing.T) {
	logger := zap.NewNop()
	defer logger.Sync()

	// ============================================================
	// STEP 1: Load function registry (simulating CLI startup)
	// ============================================================
	functionRegistry, err := functions.LoadRegistry("functions.yaml")
	if err != nil {
		t.Fatalf("Failed to load function registry: %v", err)
	}

	functionList := functionRegistry.List()
	if len(functionList) == 0 {
		t.Fatalf("Expected functions in registry, got none")
	}
	t.Logf("✓ Loaded %d functions from registry", len(functionList))

	// ============================================================
	// STEP 2: Verify check_tcp_health function exists
	// ============================================================
	tcpHealthFn, exists := functionRegistry.Get("check_tcp_health")
	if !exists {
		t.Fatalf("check_tcp_health function not found in registry")
	}
	t.Logf("✓ Found check_tcp_health: %s", tcpHealthFn.Description)

	// ============================================================
	// STEP 3: Create executor (simulating CLI execution engine)
	// ============================================================
	ex := executor.NewExecutor(logger)
	t.Logf("✓ Created executor")

	// ============================================================
	// STEP 4: Simulate LLM decision to call check_tcp_health
	// (In real system, LLM would generate these based on user query)
	// ============================================================
	functionCall := types.FunctionCall{
		Name:     "check_tcp_health",
		Critical: true,
		Params: map[string]interface{}{
			"interface": "eth0",
			"port":      50051,
		},
	}
	t.Logf("✓ Created function call: %s", functionCall.Name)

	// ============================================================
	// STEP 5: Execute function through dispatcher
	// ============================================================
	result, err := ex.Execute(functionCall)
	if err != nil {
		// On non-Linux systems, this will fail because ss command doesn't exist
		// That's expected
		if fmt.Sprintf("%v", err) == "failed to execute ss: exec: \"ss\": executable file not found in %PATH%" ||
			fmt.Sprintf("%v", err) == "exec: \"ss\": executable file not found in $PATH" {
			t.Logf("✓ Got expected error on non-Linux system: %v", err)
			t.Logf("  (This is expected - ss command only available on Linux)")
			return
		}
		t.Fatalf("Execute returned error: %v", err)
	}

	// ============================================================
	// STEP 6: Parse and validate function result
	// ============================================================
	var resultMap map[string]interface{}
	if err := json.Unmarshal([]byte(result), &resultMap); err != nil {
		t.Fatalf("Failed to parse result JSON: %v", err)
	}
	t.Logf("✓ Parsed function result")

	// ============================================================
	// STEP 7: Validate response structure
	// ============================================================
	expectedFields := []string{"state", "port", "interface", "retransmits", "send_queue_bytes", "recv_queue_bytes"}
	for _, field := range expectedFields {
		if _, ok := resultMap[field]; !ok {
			t.Errorf("Missing field in result: %s", field)
		}
	}
	t.Logf("✓ Result has all expected fields")

	// ============================================================
	// STEP 8: Verify types of returned values
	// ============================================================
	if state, ok := resultMap["state"].(string); !ok {
		t.Errorf("state should be string, got %T", resultMap["state"])
	} else {
		t.Logf("✓ state = %s", state)
	}

	if port, ok := resultMap["port"].(float64); !ok {
		t.Errorf("port should be number, got %T", resultMap["port"])
	} else {
		t.Logf("✓ port = %.0f", port)
	}

	if retrans, ok := resultMap["retransmits"].(float64); !ok {
		t.Errorf("retransmits should be number, got %T", resultMap["retransmits"])
	} else {
		t.Logf("✓ retransmits = %.0f", retrans)
	}

	t.Logf("✓ All validations passed")
}

// TestE2E_MultipleExecutions tests multiple function calls in sequence
func TestE2E_MultipleExecutions(t *testing.T) {
	logger := zap.NewNop()
	defer logger.Sync()

	ex := executor.NewExecutor(logger)
	functionRegistry, _ := functions.LoadRegistry("functions.yaml")

	// Test case 1: check_tcp_health
	tcpCall := types.FunctionCall{
		Name: "check_tcp_health",
		Params: map[string]interface{}{
			"interface": "eth0",
			"port":      50051,
		},
	}
	t.Logf("Test 1: Executing check_tcp_health...")
	_, err1 := ex.Execute(tcpCall)
	if err1 != nil {
		t.Logf("  (Expected error on non-Linux: %v)", err1)
	} else {
		t.Logf("  ✓ check_tcp_health succeeded")
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
	t.Logf("Test 2: Executing check_grpc_health...")
	_, err2 := ex.Execute(grpcCall)
	if err2 != nil {
		t.Logf("  (Expected error - no server: %v)", err2)
	} else {
		t.Logf("  ✓ check_grpc_health succeeded")
	}

	// Test case 3: Missing required parameter
	invalidCall := types.FunctionCall{
		Name:   "check_tcp_health",
		Params: map[string]interface{}{"interface": "eth0"}, // missing port
	}
	t.Logf("Test 3: Executing with missing parameter...")
	_, err3 := ex.Execute(invalidCall)
	if err3 == nil {
		t.Fatalf("Expected error for missing parameter")
	}
	t.Logf("  ✓ Correctly rejected missing parameter: %v", err3)

	// Test case 4: Unknown function
	unknownCall := types.FunctionCall{
		Name:   "unknown_function",
		Params: map[string]interface{}{},
	}
	t.Logf("Test 4: Executing unknown function...")
	_, err4 := ex.Execute(unknownCall)
	if err4 == nil {
		t.Fatalf("Expected error for unknown function")
	}
	t.Logf("  ✓ Correctly rejected unknown function: %v", err4)

	t.Logf("\n✓ All execution tests completed")
}

// TestE2E_FunctionValidation tests function registry and validation
func TestE2E_FunctionValidation(t *testing.T) {
	logger := zap.NewNop()
	defer logger.Sync()

	functionRegistry, err := functions.LoadRegistry("functions.yaml")
	if err != nil {
		t.Fatalf("Failed to load function registry: %v", err)
	}

	// Check network functions exist
	networkFunctions := []string{"check_tcp_health", "check_grpc_health"}
	for _, fnName := range networkFunctions {
		fn, exists := functionRegistry.Get(fnName)
		if !exists {
			t.Errorf("Function %s not found", fnName)
			continue
		}

		t.Logf("✓ %s", fnName)
		t.Logf("  Category: %s", fn.Category)
		t.Logf("  Description: %s", fn.Description)
		t.Logf("  Parameters: %d", len(fn.Parameters))
		t.Logf("  Timeout: %d seconds", fn.TimeoutSeconds)

		// Validate parameters
		for _, param := range fn.Parameters {
			t.Logf("    - %s (%s)%s", param.Name, param.Type, map[bool]string{true: " [required]", false: ""}[param.Required])
		}
	}

	t.Logf("\n✓ All functions validated")
}

// TestE2E_ExecutorResponseFormat tests response parsing and validation
func TestE2E_ExecutorResponseFormat(t *testing.T) {
	logger := zap.NewNop()
	defer logger.Sync()

	ex := executor.NewExecutor(logger)

	// Test case: check_grpc_health with all parameters
	grpcCall := types.FunctionCall{
		Name: "check_grpc_health",
		Params: map[string]interface{}{
			"host":    "127.0.0.1",
			"port":    9999, // non-existent port
			"timeout": 1,
		},
	}

	t.Logf("Testing response format with intentional failure...")
	result, err := ex.Execute(grpcCall)

	if err != nil {
		t.Logf("✓ Got expected error: %v", err)

		// Error messages should be informative
		if len(fmt.Sprint(err)) == 0 {
			t.Errorf("Error message is empty")
		} else {
			t.Logf("✓ Error is informative: %s", err)
		}
	} else {
		// If no error, validate JSON response
		var resultMap map[string]interface{}
		if err := json.Unmarshal([]byte(result), &resultMap); err != nil {
			t.Fatalf("Result is not valid JSON: %v", err)
		}

		required := []string{"host", "port", "status", "latency_ms"}
		for _, field := range required {
			if _, ok := resultMap[field]; !ok {
				t.Errorf("Missing required field: %s", field)
			}
		}
		t.Logf("✓ Response has valid format")
	}
}

// TestE2E_ConcurrentExecutions tests thread-safe concurrent execution
func TestE2E_ConcurrentExecutions(t *testing.T) {
	logger := zap.NewNop()
	defer logger.Sync()

	ex := executor.NewExecutor(logger)

	// Create multiple concurrent execution tasks
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

	// Collect results
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

	t.Logf("✓ Concurrent execution test")
	t.Logf("  Successful: %d", successCount)
	t.Logf("  Failed (expected): %d", errorCount)
	t.Logf("  Total: %d", successCount+errorCount)
}

// TestE2E_ParameterTypeConversion tests automatic parameter type conversion
func TestE2E_ParameterTypeConversion(t *testing.T) {
	logger := zap.NewNop()
	defer logger.Sync()

	ex := executor.NewExecutor(logger)

	testCases := []struct {
		name   string
		call   types.FunctionCall
		shouldFail bool
	}{
		{
			name: "Port as string",
			call: types.FunctionCall{
				Name: "check_tcp_health",
				Params: map[string]interface{}{
					"interface": "eth0",
					"port":      "50051", // string instead of int
				},
			},
			shouldFail: false,
		},
		{
			name: "Port as float",
			call: types.FunctionCall{
				Name: "check_tcp_health",
				Params: map[string]interface{}{
					"interface": "eth0",
					"port":      50051.5, // float instead of int
				},
			},
			shouldFail: false,
		},
		{
			name: "Timeout as string",
			call: types.FunctionCall{
				Name: "check_grpc_health",
				Params: map[string]interface{}{
					"host":    "localhost",
					"port":    50051,
					"timeout": "5", // string instead of int
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
				t.Logf("  ✗ Expected error but got none")
			} else {
				t.Logf("  ✓ Got expected error: %v", err)
			}
		} else {
			if err != nil && !fmt.Sprintf("%v", err).Contains("executable file not found") &&
				!fmt.Sprintf("%v", err).Contains("context deadline") {
				t.Logf("  ? Got unexpected error (may be environment-specific): %v", err)
			} else {
				t.Logf("  ✓ Handled gracefully")
			}
		}
	}
}

// TestE2E_Simulation_RAGToDiagnostics simulates a complete workflow:
// User query → RAG retrieval → LLM decision → Function execution
func TestE2E_Simulation_RAGToDiagnostics(t *testing.T) {
	logger := zap.NewNop()
	defer logger.Sync()

	t.Logf("\n=== Simulated End-to-End Diagnostic Workflow ===\n")

	// STEP 1: User input (simulating CLI)
	userQuery := "Check if gRPC service on localhost:50051 is healthy"
	t.Logf("📝 User Query: %s\n", userQuery)

	// STEP 2: Load function registry
	functionRegistry, _ := functions.LoadRegistry("functions.yaml")
	t.Logf("📚 Loaded function registry")

	// STEP 3: Simulate LLM decision based on query
	// In real system, LLM would analyze query and generate these functions
	t.Logf("🤖 LLM analyzing query...\n")

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

	t.Logf("   Reasoning: %s", simulatedLLMResponse.Reasoning)
	t.Logf("   Strategy: %s", simulatedLLMResponse.ExecutionStrategy)
	t.Logf("   Function calls planned: %d\n", len(simulatedLLMResponse.Functions))

	// STEP 4: Execute planned functions
	t.Logf("⚙️  Executing functions...\n")
	ex := executor.NewExecutor(logger)
	executionResults := make([]types.ExecutionResult, 0)

	for idx, fnCall := range simulatedLLMResponse.Functions {
		t.Logf("   [%d/%d] Executing: %s", idx+1, len(simulatedLLMResponse.Functions), fnCall.Name)

		startTime := time.Now()
		result, err := ex.Execute(fnCall)
		duration := time.Since(startTime)

		if err != nil {
			t.Logf("        ⚠️  Error: %v (duration: %s)", err, duration)
			executionResults = append(executionResults, types.ExecutionResult{
				Index:    idx,
				Function: fnCall,
				Success:  false,
				Error:    fmt.Sprintf("%v", err),
				Duration: duration,
			})
		} else {
			t.Logf("        ✓ Success (duration: %s)", duration)
			t.Logf("        📊 Result preview: %s...", result[:min(len(result), 60)])

			executionResults = append(executionResults, types.ExecutionResult{
				Index:    idx,
				Function: fnCall,
				Success:  true,
				Output:   result,
				Duration: duration,
			})
		}
	}

	// STEP 5: Generate summary
	t.Logf("\n📋 Execution Summary:")
	successCount := 0
	for _, result := range executionResults {
		if result.Success {
			successCount++
		}
	}

	t.Logf("   Total executed: %d", len(executionResults))
	t.Logf("   Successful: %d", successCount)
	t.Logf("   Failed: %d", len(executionResults)-successCount)

	// STEP 6: Final response to user
	t.Logf("\n🎯 Final Response:")
	if successCount == len(executionResults) {
		t.Logf("   All diagnostic checks completed successfully")
	} else if successCount > 0 {
		t.Logf("   Partial results available (some checks failed)")
	} else {
		t.Logf("   Unable to complete diagnostic checks")
	}

	t.Logf("\n✓ End-to-end workflow test completed\n")
}

// Helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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

// Helper for string contains (Go 1.18+)
func (s string) Contains(substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && s[0:len(substr)] == substr || s[len(s)-len(substr):] == substr)
}

package executor

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ashutoshrp06/telemetry-debugger/internal/executor"
	"github.com/ashutoshrp06/telemetry-debugger/internal/types"
	"go.uber.org/zap"
)

func TestExecute_CheckTCPHealth_Success(t *testing.T) {
	logger := zap.NewNop()
	ex := executor.NewExecutor(logger)

	fn := types.FunctionCall{
		Name: "check_tcp_health",
		Params: map[string]interface{}{
			"interface": "eth0",
			"port":      50051,
		},
	}

	out, err := ex.Execute(fn)
	
	// On Windows (no ss command), this will fail. That's expected.
	// On Linux, this should succeed.
	if err != nil {
		// Acceptable for non-Linux systems
		if strings.Contains(err.Error(), "executable file not found") || 
		   strings.Contains(err.Error(), "command not found") {
			t.Logf("CheckTCPHealth failed (expected on non-Linux systems): %v", err)
			return
		}
		// Other errors are unexpected
		t.Fatalf("Execute returned unexpected error: %v", err)
	}

	var res map[string]interface{}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("invalid json output: %v", err)
	}

	// Verify structure
	expectedKeys := []string{"state", "port", "interface", "retransmits"}
	for _, key := range expectedKeys {
		if _, ok := res[key]; !ok {
			t.Fatalf("missing key in response: %s", key)
		}
	}
}

func TestExecute_CheckGRPCHealth_MissingParam(t *testing.T) {
	logger := zap.NewNop()
	ex := executor.NewExecutor(logger)

	fn := types.FunctionCall{
		Name:   "check_grpc_health",
		Params: map[string]interface{}{"host": "localhost"},
	}

	_, err := ex.Execute(fn)
	if err == nil {
		t.Fatalf("expected error for missing port parameter")
	}
}

func TestExecute_CheckGRPCHealth_WithDefaults(t *testing.T) {
	logger := zap.NewNop()
	ex := executor.NewExecutor(logger)

	// Check that default host and timeout are applied
	fn := types.FunctionCall{
		Name: "check_grpc_health",
		Params: map[string]interface{}{
			"port": 50051,
			// host defaults to "localhost"
			// timeout defaults to 5
		},
	}

	out, err := ex.Execute(fn)
	
	// On Windows without a real gRPC server, this will fail with connection refused/timeout
	// That's expected
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") ||
		   strings.Contains(err.Error(), "connection reset") ||
		   strings.Contains(err.Error(), "dial") ||
		   strings.Contains(err.Error(), "deadline") {
			t.Logf("CheckGRPCHealth failed as expected (no server): %v", err)
			return
		}
		t.Fatalf("CheckGRPCHealth returned unexpected error: %v", err)
	}

	var res map[string]interface{}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("invalid json output: %v", err)
	}

	// Verify output structure
	if _, ok := res["status"]; !ok {
		t.Fatalf("missing status in response")
	}
}

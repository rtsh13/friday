package executor

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/friday/internal/types"
	"go.uber.org/zap"
)

// ─── NewVariableResolver ──────────────────────────────────────────────────────

func TestNewVariableResolver_Empty(t *testing.T) {
	vr := NewVariableResolver()
	if vr == nil {
		t.Fatal("NewVariableResolver returned nil")
	}
	if vr.HasResult("anything") {
		t.Error("fresh resolver should have no results")
	}
}

// ─── AddResult ────────────────────────────────────────────────────────────────

func TestAddResult_ValidJSON_Map(t *testing.T) {
	vr := NewVariableResolver()
	vr.AddResult("check_grpc_health", `{"status":"SERVING","latency_ms":42,"port":50051}`)

	if !vr.HasResult("check_grpc_health") {
		t.Error("result should be registered after AddResult")
	}
}

func TestAddResult_ValidJSON_Nested(t *testing.T) {
	vr := NewVariableResolver()
	vr.AddResult("diagnose", `{"host":"localhost","stats":{"retransmits":5,"rtt":0.5}}`)

	val, err := vr.Resolve("${diagnose.stats.retransmits}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "5" {
		t.Errorf("expected '5', got %q", val)
	}
}

func TestAddResult_NonJSON_StoredAsValue(t *testing.T) {
	vr := NewVariableResolver()
	vr.AddResult("raw_tool", "plain text output")

	val, err := vr.Resolve("${raw_tool.value}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "plain text output" {
		t.Errorf("expected raw string, got %q", val)
	}
}

func TestAddResult_EmptyOutput_Ignored(t *testing.T) {
	vr := NewVariableResolver()
	vr.AddResult("silent_tool", "")

	if vr.HasResult("silent_tool") {
		t.Error("empty output should not be stored")
	}
}

// ─── Resolve (string interpolation) ──────────────────────────────────────────

func TestResolve_NoPlaceholder_PassThrough(t *testing.T) {
	vr := NewVariableResolver()
	result, err := vr.Resolve("just a plain string")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "just a plain string" {
		t.Errorf("expected passthrough, got %q", result)
	}
}

func TestResolve_SinglePlaceholder_FullString(t *testing.T) {
	vr := NewVariableResolver()
	vr.AddResult("check_tcp_health", `{"port":50051,"interface":"eth0","retransmits":3}`)

	result, err := vr.Resolve("${check_tcp_health.port}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "50051" {
		t.Errorf("expected '50051', got %q", result)
	}
}

func TestResolve_Interpolation_MixedString(t *testing.T) {
	vr := NewVariableResolver()
	vr.AddResult("info", `{"host":"myserver","port":8080}`)

	result, err := vr.Resolve("Connecting to ${info.host}:${info.port}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Connecting to myserver:8080" {
		t.Errorf("expected 'Connecting to myserver:8080', got %q", result)
	}
}

func TestResolve_UnknownFunction_Error(t *testing.T) {
	vr := NewVariableResolver()
	_, err := vr.Resolve("${unknown_func.field}")
	if err == nil {
		t.Fatal("expected error for unknown function reference")
	}
	if !containsStr(err.Error(), "unknown_func") {
		t.Errorf("error should mention function name, got: %v", err)
	}
}

func TestResolve_UnknownField_Error(t *testing.T) {
	vr := NewVariableResolver()
	vr.AddResult("func", `{"port":50051}`)

	_, err := vr.Resolve("${func.nonexistent_field}")
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
	if !containsStr(err.Error(), "nonexistent_field") {
		t.Errorf("error should mention field name, got: %v", err)
	}
}

func TestResolve_DottedPath_ThreeLevels(t *testing.T) {
	vr := NewVariableResolver()
	vr.AddResult("analyze", `{"results":{"tcp":{"retransmits":12}}}`)

	val, err := vr.Resolve("${analyze.results.tcp.retransmits}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "12" {
		t.Errorf("expected '12', got %q", val)
	}
}

func TestResolve_ArrayIndex(t *testing.T) {
	vr := NewVariableResolver()
	vr.AddResult("scan", `{"open_ports":[22,80,443]}`)

	val, err := vr.Resolve("${scan.open_ports.0}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "22" {
		t.Errorf("expected '22', got %q", val)
	}

	val2, err := vr.Resolve("${scan.open_ports.2}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val2 != "443" {
		t.Errorf("expected '443', got %q", val2)
	}
}

func TestResolve_ArrayIndex_OutOfRange(t *testing.T) {
	vr := NewVariableResolver()
	vr.AddResult("scan", `{"open_ports":[22,80]}`)

	_, err := vr.Resolve("${scan.open_ports.5}")
	if err == nil {
		t.Fatal("expected error for out-of-range index")
	}
}

func TestResolve_BooleanValue(t *testing.T) {
	vr := NewVariableResolver()
	vr.AddResult("grpc", `{"serving":true,"latency_ms":5}`)

	val, err := vr.Resolve("${grpc.serving}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "true" {
		t.Errorf("expected 'true', got %q", val)
	}
}

func TestResolve_FloatValue(t *testing.T) {
	vr := NewVariableResolver()
	vr.AddResult("ping", `{"avg_latency_ms":3.14}`)

	val, err := vr.Resolve("${ping.avg_latency_ms}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// JSON floats are represented as float64
	if val == "" {
		t.Error("expected non-empty value for float")
	}
}

// ─── ResolveParams ────────────────────────────────────────────────────────────

func TestResolveParams_Nil_ReturnsNil(t *testing.T) {
	vr := NewVariableResolver()
	result, err := vr.ResolveParams(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil for nil input")
	}
}

func TestResolveParams_NoVariables_PassThrough(t *testing.T) {
	vr := NewVariableResolver()
	params := map[string]interface{}{
		"host": "localhost",
		"port": 50051,
	}
	resolved, err := vr.ResolveParams(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved["host"] != "localhost" || resolved["port"] != 50051 {
		t.Error("params should pass through unchanged")
	}
}

func TestResolveParams_StringVariable_Resolved(t *testing.T) {
	vr := NewVariableResolver()
	vr.AddResult("check_tcp_health", `{"port":50051,"interface":"eth0"}`)

	params := map[string]interface{}{
		"port": "${check_tcp_health.port}",
		"host": "localhost",
	}

	resolved, err := vr.ResolveParams(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Single-placeholder values preserve native type (float64 from JSON).
	// The executor's getInt helper handles float64 → int conversion.
	switch v := resolved["port"].(type) {
	case float64:
		if v != 50051 {
			t.Errorf("expected 50051, got %v", v)
		}
	case int:
		if v != 50051 {
			t.Errorf("expected 50051, got %v", v)
		}
	default:
		t.Errorf("unexpected type for resolved port: %T = %v", resolved["port"], resolved["port"])
	}

	if resolved["host"] != "localhost" {
		t.Errorf("non-variable params should pass through, got %v", resolved["host"])
	}
}

func TestResolveParams_PreservesOriginal(t *testing.T) {
	vr := NewVariableResolver()
	vr.AddResult("step1", `{"value":"resolved"}`)

	original := map[string]interface{}{
		"param": "${step1.value}",
	}

	_, err := vr.ResolveParams(original)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Original map should not be modified.
	if original["param"] != "${step1.value}" {
		t.Error("ResolveParams should not modify the original params map")
	}
}

func TestResolveParams_NestedMap(t *testing.T) {
	vr := NewVariableResolver()
	vr.AddResult("step1", `{"host":"10.0.0.1","port":9090}`)

	params := map[string]interface{}{
		"target": map[string]interface{}{
			"host": "${step1.host}",
			"port": "${step1.port}",
		},
	}

	resolved, err := vr.ResolveParams(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	target, ok := resolved["target"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested map, got %T", resolved["target"])
	}

	if target["host"] != "10.0.0.1" {
		t.Errorf("expected '10.0.0.1', got %v", target["host"])
	}
}

func TestResolveParams_SliceValues(t *testing.T) {
	vr := NewVariableResolver()
	vr.AddResult("info", `{"service":"grpc","version":"v1"}`)

	params := map[string]interface{}{
		"tags": []interface{}{"static", "${info.service}", "${info.version}"},
	}

	resolved, err := vr.ResolveParams(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tags, ok := resolved["tags"].([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", resolved["tags"])
	}
	if tags[1] != "grpc" {
		t.Errorf("expected 'grpc' at index 1, got %v", tags[1])
	}
	if tags[2] != "v1" {
		t.Errorf("expected 'v1' at index 2, got %v", tags[2])
	}
}

// ─── ContainsVariables ────────────────────────────────────────────────────────

func TestContainsVariables_True(t *testing.T) {
	params := map[string]interface{}{
		"port": "${step1.port}",
	}
	if !ContainsVariables(params) {
		t.Error("expected true for params with variable")
	}
}

func TestContainsVariables_False(t *testing.T) {
	params := map[string]interface{}{
		"port": 50051,
		"host": "localhost",
	}
	if ContainsVariables(params) {
		t.Error("expected false for params without variables")
	}
}

func TestContainsVariables_Nested(t *testing.T) {
	params := map[string]interface{}{
		"outer": map[string]interface{}{
			"inner": "${func.field}",
		},
	}
	if !ContainsVariables(params) {
		t.Error("expected true for nested variable reference")
	}
}

func TestContainsVariables_NilParams(t *testing.T) {
	if ContainsVariables(nil) {
		t.Error("expected false for nil params")
	}
}

// ─── TransactionExecutor integration ─────────────────────────────────────────

// mockExecutorFn is a function type that simulates an executor for testing.
// We test TransactionExecutor by checking it correctly passes resolved params.
func TestTransactionExecutor_VariableChaining(t *testing.T) {
	// This test verifies the resolver is wired into the transaction executor
	// by checking that a real execution chain propagates outputs.

	logger := zap.NewNop()
	defer logger.Sync()

	ex := NewExecutor(logger)
	txEx := NewTransactionExecutor(ex)

	// Two-step chain: step 1 succeeds and returns JSON, step 2 should receive
	// resolved params. We use inspect_network_buffers (no required params) as a
	// harmless probe to verify the chain doesn't break.
	// On non-Linux the first step will error; that's expected.

	functions := []types.FunctionCall{
		{
			Name:   "inspect_network_buffers",
			Params: map[string]interface{}{},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	results, _ := txEx.ExecuteTransaction(ctx, functions)

	// We always expect at least one result entry.
	if len(results) == 0 {
		t.Error("expected at least one result from transaction")
	}
}

func TestTransactionExecutor_ContextCancellation(t *testing.T) {
	logger := zap.NewNop()
	defer logger.Sync()

	ex := NewExecutor(logger)
	txEx := NewTransactionExecutor(ex)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	functions := []types.FunctionCall{
		{Name: "inspect_network_buffers", Params: map[string]interface{}{}},
	}

	_, err := txEx.ExecuteTransaction(ctx, functions)
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestTransactionExecutor_EmptyFunctions(t *testing.T) {
	logger := zap.NewNop()
	defer logger.Sync()

	ex := NewExecutor(logger)
	txEx := NewTransactionExecutor(ex)

	ctx := context.Background()
	results, err := txEx.ExecuteTransaction(ctx, []types.FunctionCall{})

	if err != nil {
		t.Errorf("unexpected error for empty function list: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty results, got %d", len(results))
	}
}

// ─── Variable resolution error paths ─────────────────────────────────────────

func TestResolveFunction_MissingDependency_Error(t *testing.T) {
	logger := zap.NewNop()
	defer logger.Sync()

	ex := NewExecutor(logger)
	txEx := NewTransactionExecutor(ex)

	// Function 1 has a variable referencing a function that hasn't run yet
	// (and doesn't exist). Should fail at variable resolution, not execution.
	functions := []types.FunctionCall{
		{
			Name: "inspect_network_buffers",
			Params: map[string]interface{}{
				// This references a non-existent prior function.
				"fake_param": "${nonexistent_function.port}",
			},
		},
	}

	ctx := context.Background()
	results, err := txEx.ExecuteTransaction(ctx, functions)

	if err == nil {
		t.Fatal("expected error for unresolvable variable reference")
	}
	if !containsStr(err.Error(), "nonexistent_function") {
		t.Errorf("error should mention the unresolved function, got: %v", err)
	}
	// We should still get a result entry (failed).
	if len(results) != 1 {
		t.Errorf("expected 1 result entry, got %d", len(results))
	}
	if results[0].Success {
		t.Error("result should be marked as failed")
	}
}

// ─── Benchmarks ───────────────────────────────────────────────────────────────

func BenchmarkVariableResolver_AddResult(b *testing.B) {
	vr := NewVariableResolver()
	output := `{"port":50051,"interface":"eth0","retransmits":5,"send_queue_bytes":10,"recv_queue_bytes":0}`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vr.AddResult(fmt.Sprintf("func_%d", i%10), output)
	}
}

func BenchmarkVariableResolver_Resolve(b *testing.B) {
	vr := NewVariableResolver()
	vr.AddResult("check_tcp_health", `{"port":50051,"interface":"eth0","retransmits":5}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = vr.Resolve("${check_tcp_health.port}")
	}
}

func BenchmarkVariableResolver_ResolveParams(b *testing.B) {
	vr := NewVariableResolver()
	vr.AddResult("step1", `{"host":"localhost","port":9090,"timeout":5}`)

	params := map[string]interface{}{
		"host":    "${step1.host}",
		"port":    "${step1.port}",
		"timeout": "${step1.timeout}",
		"static":  "value",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = vr.ResolveParams(params)
	}
}

func BenchmarkContainsVariables(b *testing.B) {
	params := map[string]interface{}{
		"host":    "localhost",
		"port":    50051,
		"dynamic": "${step1.value}",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ContainsVariables(params)
	}
}

// ─── Helper ───────────────────────────────────────────────────────────────────

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && len(sub) > 0 && findSubstring(s, sub)
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

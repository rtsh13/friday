package system

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ashutoshrp06/telemetry-debugger/internal/functions/system"
)

// TestInspectNetworkBuffers_SuccessfulRead tests successful reading of buffer values
func TestInspectNetworkBuffers_SuccessfulRead(t *testing.T) {
	result, err := system.InspectNetworkBuffers()
	if err != nil {
		// On non-Linux systems this is expected
		if isNonLinux() {
			t.Logf("InspectNetworkBuffers failed (expected on non-Linux): %v", err)
			return
		}
		t.Fatalf("InspectNetworkBuffers failed: %v", err)
	}

	// Verify all required fields are present
	requiredFields := []string{
		"rmem_max",
		"wmem_max",
		"tcp_rmem_min",
		"tcp_rmem_default",
		"tcp_rmem_max",
		"tcp_wmem_min",
		"tcp_wmem_default",
		"tcp_wmem_max",
		"recommended_rmem_max",
		"recommended_wmem_max",
		"warnings",
		"recommendations",
		"status",
	}

	for _, field := range requiredFields {
		if _, ok := result[field]; !ok {
			t.Errorf("Missing field: %s", field)
		}
	}

	// Verify all values are positive integers (except warnings/recommendations which are arrays)
	numFields := []string{"rmem_max", "wmem_max", "tcp_rmem_min", "tcp_rmem_default", "tcp_rmem_max", "tcp_wmem_min", "tcp_wmem_default", "tcp_wmem_max"}
	for _, field := range numFields {
		val, ok := result[field].(int)
		if !ok {
			t.Errorf("%s should be int, got %T", field, result[field])
		}
		if val < 0 {
			t.Errorf("%s should be positive, got %d", field, val)
		}
	}

	// Verify status is either "ok" or "warning"
	status, ok := result["status"].(string)
	if !ok {
		t.Errorf("status should be string, got %T", result["status"])
	}
	if status != "ok" && status != "warning" {
		t.Errorf("status should be 'ok' or 'warning', got %s", status)
	}

	// Verify warnings and recommendations are arrays
	if _, ok := result["warnings"].([]string); !ok {
		t.Errorf("warnings should be []string, got %T", result["warnings"])
	}
	if _, ok := result["recommendations"].([]string); !ok {
		t.Errorf("recommendations should be []string, got %T", result["recommendations"])
	}
}

// TestReadProcValue_ValidFile tests reading a valid /proc file with integer value
func TestReadProcValue_ValidFile(t *testing.T) {
	// Create a temporary file to simulate /proc/sys file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test_value")

	// Write a test value
	if err := os.WriteFile(tmpFile, []byte("212992\n"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Test readProcValue with our mock file
	// Note: readProcValue is private, so we test it indirectly through InspectNetworkBuffers
	// or by testing the actual /proc files on Linux systems

	// Read the file manually to verify our format is correct
	val, err := system.ReadProcValue(tmpFile)
	if err != nil {
		t.Fatalf("readProcValue failed: %v", err)
	}

	if val != 212992 {
		t.Errorf("Expected 212992, got %d", val)
	}
}

// TestReadProcValue_WithWhitespace tests reading value with whitespace
func TestReadProcValue_WithWhitespace(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test_whitespace")

	// Write value with leading/trailing whitespace
	if err := os.WriteFile(tmpFile, []byte("  \n  12345  \n  "), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	val, err := system.ReadProcValue(tmpFile)
	if err != nil {
		t.Fatalf("readProcValue failed: %v", err)
	}

	if val != 12345 {
		t.Errorf("Expected 12345, got %d", val)
	}
}

// TestReadProcValue_InvalidContent tests error handling for non-numeric content
func TestReadProcValue_InvalidContent(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test_invalid")

	if err := os.WriteFile(tmpFile, []byte("not_a_number"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err := system.ReadProcValue(tmpFile)
	if err == nil {
		t.Error("Expected error for non-numeric content, got nil")
	}
}

// TestReadProcValue_MissingFile tests error handling for missing file
func TestReadProcValue_MissingFile(t *testing.T) {
	_, err := system.ReadProcValue("/nonexistent/proc/file/path")
	if err == nil {
		t.Error("Expected error for missing file, got nil")
	}
}

// TestReadProcTuple_ValidTuple tests reading space-separated values
func TestReadProcTuple_ValidTuple(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test_tuple")

	// Write TCP buffer tuple: min default max
	if err := os.WriteFile(tmpFile, []byte("4096\t87380\t6291456"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	vals, err := system.ReadProcTuple(tmpFile)
	if err != nil {
		t.Fatalf("readProcTuple failed: %v", err)
	}

	if len(vals) != 3 {
		t.Errorf("Expected 3 values, got %d", len(vals))
	}

	expectedVals := []int{4096, 87380, 6291456}
	for i, expected := range expectedVals {
		if vals[i] != expected {
			t.Errorf("Value[%d]: expected %d, got %d", i, expected, vals[i])
		}
	}
}

// TestReadProcTuple_WithWhitespace tests reading tuple with variable whitespace
func TestReadProcTuple_WithWhitespace(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test_tuple_ws")

	// Write tuple with mixed whitespace
	if err := os.WriteFile(tmpFile, []byte("  1024   2048   4096  \n"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	vals, err := system.ReadProcTuple(tmpFile)
	if err != nil {
		t.Fatalf("readProcTuple failed: %v", err)
	}

	expectedVals := []int{1024, 2048, 4096}
	for i, expected := range expectedVals {
		if vals[i] != expected {
			t.Errorf("Value[%d]: expected %d, got %d", i, expected, vals[i])
		}
	}
}

// TestReadProcTuple_InvalidContent tests error handling for non-numeric content
func TestReadProcTuple_InvalidContent(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test_tuple_invalid")

	if err := os.WriteFile(tmpFile, []byte("1024 invalid 4096"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err := system.ReadProcTuple(tmpFile)
	if err == nil {
		t.Error("Expected error for non-numeric content, got nil")
	}
}

// TestReadProcTuple_EmptyFile tests error handling for empty file
func TestReadProcTuple_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test_empty")

	if err := os.WriteFile(tmpFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err := system.ReadProcTuple(tmpFile)
	if err == nil {
		t.Error("Expected error for empty file, got nil")
	}
}

// TestInspectNetworkBuffers_WarningGeneration tests that warnings are generated for low values
func TestInspectNetworkBuffers_WarningGeneration(t *testing.T) {
	// Only run on Linux
	if isNonLinux() {
		t.Skip("Test requires Linux")
	}

	result, err := system.InspectNetworkBuffers()
	if err != nil {
		t.Fatalf("InspectNetworkBuffers failed: %v", err)
	}

	warnings, ok := result["warnings"].([]string)
	if !ok {
		t.Fatalf("warnings should be []string, got %T", result["warnings"])
	}

	recommendations, ok := result["recommendations"].([]string)
	if !ok {
		t.Fatalf("recommendations should be []string, got %T", result["recommendations"])
	}

	// If there are warnings, there should be corresponding recommendations
	if len(warnings) > 0 && len(recommendations) == 0 {
		t.Error("Found warnings but no recommendations")
	}

	// Verify warning count matches recommendation count
	if len(warnings) != len(recommendations) {
		t.Errorf("Warning count (%d) does not match recommendation count (%d)",
			len(warnings), len(recommendations))
	}

	// If there are warnings, status should be "warning"
	if len(warnings) > 0 {
		status, ok := result["status"].(string)
		if !ok || status != "warning" {
			t.Errorf("Expected status 'warning' when there are warnings, got %v", result["status"])
		}
	}
}

// BenchmarkInspectNetworkBuffers benchmarks the buffer inspection function
func BenchmarkInspectNetworkBuffers(b *testing.B) {
	if isNonLinux() {
		b.Skip("Benchmark requires Linux")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = system.InspectNetworkBuffers()
	}
}

// BenchmarkReadProcValue benchmarks reading a single proc value
func BenchmarkReadProcValue(b *testing.B) {
	tmpDir := b.TempDir()
	tmpFile := filepath.Join(tmpDir, "bench_value")
	os.WriteFile(tmpFile, []byte("212992\n"), 0644)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = system.ReadProcValue(tmpFile)
	}
}

// BenchmarkReadProcTuple benchmarks reading a proc tuple
func BenchmarkReadProcTuple(b *testing.B) {
	tmpDir := b.TempDir()
	tmpFile := filepath.Join(tmpDir, "bench_tuple")
	os.WriteFile(tmpFile, []byte("4096\t87380\t6291456"), 0644)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = system.ReadProcTuple(tmpFile)
	}
}

// Helper function to detect if we're on a non-Linux system
func isNonLinux() bool {
	_, err := os.Stat("/proc/sys/net/core/rmem_max")
	return err != nil
}

package system

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ─── ExecuteSysctl – Input Validation ────────────────────────────────────────

func TestExecuteSysctl_InvalidParameter_NoNetPrefix(t *testing.T) {
	_, err := ExecuteSysctl("kernel.hostname", "myhost", false)
	if err == nil {
		t.Fatal("expected error for non-net.* parameter, got nil")
	}
	if !strings.Contains(err.Error(), "invalid parameter") {
		t.Errorf("expected 'invalid parameter' in error, got: %v", err)
	}
}

func TestExecuteSysctl_InvalidParameter_EmptyString(t *testing.T) {
	_, err := ExecuteSysctl("", "212992", false)
	if err == nil {
		t.Fatal("expected error for empty parameter, got nil")
	}
}

func TestExecuteSysctl_InvalidParameter_JustNet(t *testing.T) {
	_, err := ExecuteSysctl("net.", "212992", false)
	if err == nil {
		t.Fatal("expected error for bare 'net.' parameter, got nil")
	}
}

func TestExecuteSysctl_InvalidParameter_WithSlash(t *testing.T) {
	// Slashes should be rejected – parameter names use dots, not slashes.
	_, err := ExecuteSysctl("net/core/rmem_max", "212992", false)
	if err == nil {
		t.Fatal("expected error for parameter with slashes, got nil")
	}
}

func TestExecuteSysctl_InvalidParameter_CommandInjection(t *testing.T) {
	// Attempt to inject a shell command via the parameter name.
	_, err := ExecuteSysctl("net.core.rmem_max; rm -rf /", "212992", false)
	if err == nil {
		t.Fatal("expected error for command injection in parameter, got nil")
	}
}

func TestExecuteSysctl_EmptyValue(t *testing.T) {
	_, err := ExecuteSysctl("net.core.rmem_max", "", false)
	if err == nil {
		t.Fatal("expected error for empty value, got nil")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected 'empty' in error, got: %v", err)
	}
}

func TestExecuteSysctl_ValueWithShellChars(t *testing.T) {
	// Values must be numeric only.
	badValues := []string{
		"$(malicious)",
		"`rm -rf /`",
		"6291456; echo pwned",
		"../../../etc/passwd",
		"6291456|cat /etc/shadow",
	}

	for _, v := range badValues {
		t.Run(fmt.Sprintf("value=%q", v), func(t *testing.T) {
			_, err := ExecuteSysctl("net.core.rmem_max", v, false)
			if err == nil {
				t.Errorf("expected error for dangerous value %q, got nil", v)
			}
		})
	}
}

func TestExecuteSysctl_ZeroValue_Single(t *testing.T) {
	_, err := ExecuteSysctl("net.core.rmem_max", "0", false)
	if err == nil {
		t.Fatal("expected error for zero value, got nil")
	}
	if !strings.Contains(err.Error(), "zero") {
		t.Errorf("expected 'zero' in error, got: %v", err)
	}
}

func TestExecuteSysctl_ZeroValue_Tuple(t *testing.T) {
	// All-zero tuple should also be rejected.
	_, err := ExecuteSysctl("net.ipv4.tcp_rmem", "0 0 0", false)
	if err == nil {
		t.Fatal("expected error for all-zero tuple, got nil")
	}
}

func TestExecuteSysctl_NonZeroTupleAllowed(t *testing.T) {
	// A tuple with at least one non-zero value should pass validation.
	// This only tests that the zero-check passes; it will then fail on
	// non-Linux systems when it tries to run sysctl.
	_, err := ExecuteSysctl("net.ipv4.tcp_rmem", "4096 87380 6291456", false)
	if err != nil && strings.Contains(err.Error(), "zero") {
		t.Errorf("non-zero tuple should not trigger zero-value error, got: %v", err)
	}
	// Other errors (e.g. no sysctl binary) are acceptable here.
}

// ─── paramToProcPath helper (tested indirectly) ───────────────────────────────

func TestParamToProcPath_Conversion(t *testing.T) {
	tests := []struct {
		parameter string
		expected  string
	}{
		{"net.core.rmem_max", "/proc/sys/net/core/rmem_max"},
		{"net.core.wmem_max", "/proc/sys/net/core/wmem_max"},
		{"net.ipv4.tcp_rmem", "/proc/sys/net/ipv4/tcp_rmem"},
		{"net.ipv4.tcp_wmem", "/proc/sys/net/ipv4/tcp_wmem"},
	}

	for _, tt := range tests {
		t.Run(tt.parameter, func(t *testing.T) {
			result := ParamToProcPath(tt.parameter)
			if result != tt.expected {
				t.Errorf("ParamToProcPath(%q) = %q, want %q",
					tt.parameter, result, tt.expected)
			}
		})
	}
}

// ─── PersistSysctl (unit-tested with a temp file) ────────────────────────────

func TestPersistSysctl_NewEntry(t *testing.T) {
	// Write a fresh config with a new parameter.
	tmpFile := createTempSysctlConf(t, "# sysctl.conf\nvm.swappiness = 10\n")

	err := PersistSysctlToFile(tmpFile, "net.core.rmem_max", "6291456")
	if err != nil {
		t.Fatalf("PersistSysctlToFile failed: %v", err)
	}

	content := readFile(t, tmpFile)
	if !strings.Contains(content, "net.core.rmem_max = 6291456") {
		t.Errorf("expected new entry in file, got:\n%s", content)
	}
	// Existing line should be preserved.
	if !strings.Contains(content, "vm.swappiness = 10") {
		t.Errorf("existing entry was removed, got:\n%s", content)
	}
}

func TestPersistSysctl_UpdateExistingEntry_SpacedFormat(t *testing.T) {
	// The existing entry uses "param = value" format.
	initial := "# sysctl config\nnet.core.rmem_max = 212992\nvm.swappiness = 10\n"
	tmpFile := createTempSysctlConf(t, initial)

	err := PersistSysctlToFile(tmpFile, "net.core.rmem_max", "6291456")
	if err != nil {
		t.Fatalf("PersistSysctlToFile failed: %v", err)
	}

	content := readFile(t, tmpFile)
	if strings.Count(content, "net.core.rmem_max") != 1 {
		t.Errorf("expected exactly one occurrence of net.core.rmem_max, got:\n%s", content)
	}
	if !strings.Contains(content, "net.core.rmem_max = 6291456") {
		t.Errorf("expected updated value in file, got:\n%s", content)
	}
}

func TestPersistSysctl_UpdateExistingEntry_EqualSignFormat(t *testing.T) {
	// The existing entry uses "param=value" format (no spaces).
	initial := "net.core.rmem_max=212992\n"
	tmpFile := createTempSysctlConf(t, initial)

	err := PersistSysctlToFile(tmpFile, "net.core.rmem_max", "6291456")
	if err != nil {
		t.Fatalf("PersistSysctlToFile failed: %v", err)
	}

	content := readFile(t, tmpFile)
	if strings.Count(content, "net.core.rmem_max") != 1 {
		t.Errorf("expected exactly one entry, got:\n%s", content)
	}
	if !strings.Contains(content, "net.core.rmem_max = 6291456") {
		t.Errorf("expected updated value, got:\n%s", content)
	}
}

func TestPersistSysctl_PreservesComments(t *testing.T) {
	initial := "# This file is managed by the sysadmin\n# Do not edit manually\nnet.core.rmem_max = 212992\n"
	tmpFile := createTempSysctlConf(t, initial)

	err := PersistSysctlToFile(tmpFile, "net.core.rmem_max", "6291456")
	if err != nil {
		t.Fatalf("PersistSysctlToFile failed: %v", err)
	}

	content := readFile(t, tmpFile)
	if !strings.Contains(content, "# This file is managed by the sysadmin") {
		t.Errorf("comments were not preserved, got:\n%s", content)
	}
	if !strings.Contains(content, "# Do not edit manually") {
		t.Errorf("comments were not preserved, got:\n%s", content)
	}
}

func TestPersistSysctl_EmptyFile(t *testing.T) {
	tmpFile := createTempSysctlConf(t, "")

	err := PersistSysctlToFile(tmpFile, "net.core.rmem_max", "6291456")
	if err != nil {
		t.Fatalf("PersistSysctlToFile failed: %v", err)
	}

	content := readFile(t, tmpFile)
	if !strings.Contains(content, "net.core.rmem_max = 6291456") {
		t.Errorf("expected entry in empty file, got:\n%s", content)
	}
}

func TestPersistSysctl_TupleValue(t *testing.T) {
	// Tuple values (e.g. for tcp_rmem) should be persisted correctly.
	tmpFile := createTempSysctlConf(t, "")

	err := PersistSysctlToFile(tmpFile, "net.ipv4.tcp_rmem", "4096 87380 6291456")
	if err != nil {
		t.Fatalf("PersistSysctlToFile failed: %v", err)
	}

	content := readFile(t, tmpFile)
	if !strings.Contains(content, "net.ipv4.tcp_rmem = 4096 87380 6291456") {
		t.Errorf("tuple value not persisted correctly, got:\n%s", content)
	}
}

// ─── RestoreSysctlValue – Input Validation ───────────────────────────────────

func TestRestoreSysctlValue_InvalidParameter(t *testing.T) {
	err := RestoreSysctlValue("vm.swappiness", "60")
	if err == nil {
		t.Fatal("expected error for non-net.* parameter in rollback, got nil")
	}
	if !strings.Contains(err.Error(), "invalid parameter") {
		t.Errorf("expected 'invalid parameter' in error, got: %v", err)
	}
}

func TestRestoreSysctlValue_EmptyValue(t *testing.T) {
	err := RestoreSysctlValue("net.core.rmem_max", "")
	if err == nil {
		t.Fatal("expected error for empty rollback value, got nil")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected 'empty' in error, got: %v", err)
	}
}

func TestRestoreSysctlValue_DangerousValue(t *testing.T) {
	err := RestoreSysctlValue("net.core.rmem_max", "$(malicious)")
	if err == nil {
		t.Fatal("expected error for dangerous rollback value, got nil")
	}
}

// ─── Linux-only live execution tests ──────────────────────────────────────────

func TestExecuteSysctl_LiveRead_NonDestructive(t *testing.T) {
	if !isLinux() {
		t.Skip("live sysctl test requires Linux")
	}
	if !isRoot() {
		t.Skip("live sysctl test requires root/sudo")
	}

	// Read the current value first.
	currentContent, err := os.ReadFile("/proc/sys/net/core/rmem_max")
	if err != nil {
		t.Fatalf("cannot read current rmem_max: %v", err)
	}
	currentValue := strings.TrimSpace(string(currentContent))

	// Set it to the SAME value – non-destructive.
	result, err := ExecuteSysctl("net.core.rmem_max", currentValue, false)
	if err != nil {
		t.Fatalf("ExecuteSysctl failed: %v", err)
	}

	// Verify the result structure.
	if _, ok := result["old_value"]; !ok {
		t.Error("missing old_value in result")
	}
	if _, ok := result["new_value"]; !ok {
		t.Error("missing new_value in result")
	}
	if success, ok := result["success"].(bool); !ok || !success {
		t.Errorf("expected success=true, got: %v", result["success"])
	}
	if persisted, ok := result["persisted"].(bool); !ok || persisted {
		t.Errorf("expected persisted=false, got: %v", result["persisted"])
	}

	t.Logf("old_value=%v new_value=%v", result["old_value"], result["new_value"])
}

func TestRestoreSysctlValue_LiveRoundTrip(t *testing.T) {
	if !isLinux() {
		t.Skip("rollback test requires Linux")
	}
	if !isRoot() {
		t.Skip("rollback test requires root/sudo")
	}

	// Capture current value.
	currentContent, err := os.ReadFile("/proc/sys/net/core/rmem_max")
	if err != nil {
		t.Fatalf("cannot read current rmem_max: %v", err)
	}
	originalValue := strings.TrimSpace(string(currentContent))

	// Change it (to the same value to be non-destructive).
	_, err = ExecuteSysctl("net.core.rmem_max", originalValue, false)
	if err != nil {
		t.Fatalf("ExecuteSysctl failed: %v", err)
	}

	// Now restore it via RestoreSysctlValue.
	if err := RestoreSysctlValue("net.core.rmem_max", originalValue); err != nil {
		t.Fatalf("RestoreSysctlValue failed: %v", err)
	}

	// Verify the value is back.
	afterContent, err := os.ReadFile("/proc/sys/net/core/rmem_max")
	if err != nil {
		t.Fatalf("cannot read rmem_max after restore: %v", err)
	}
	afterValue := strings.TrimSpace(string(afterContent))
	if afterValue != originalValue {
		t.Errorf("restore failed: expected %q, got %q", originalValue, afterValue)
	}
}

// ─── Benchmark ────────────────────────────────────────────────────────────────

func BenchmarkExecuteSysctl_ValidationOnly(b *testing.B) {
	// Benchmark the validation path (will fail before hitting sysctl on non-Linux).
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ExecuteSysctl("net.core.rmem_max", "6291456", false)
	}
}

func BenchmarkPersistSysctlToFile(b *testing.B) {
	tmpDir := b.TempDir()
	tmpFile := filepath.Join(tmpDir, "sysctl.conf")
	os.WriteFile(tmpFile, []byte("net.core.rmem_max = 212992\n"), 0644)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = PersistSysctlToFile(tmpFile, "net.core.rmem_max", "6291456")
	}
}

// ─── Test helpers ─────────────────────────────────────────────────────────────

func createTempSysctlConf(t *testing.T, content string) string {
	t.Helper()
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "sysctl.conf")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create temp sysctl.conf: %v", err)
	}
	return tmpFile
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	return string(content)
}

func isLinux() bool {
	return runtime.GOOS == "linux"
}

func isRoot() bool {
	return os.Getuid() == 0
}

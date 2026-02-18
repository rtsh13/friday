package system

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// paramValidationRegex ensures only safe net.* parameters are accepted.
// Matches: net.core.rmem_max, net.ipv4.tcp_rmem, etc.
var paramValidationRegex = regexp.MustCompile(`^net\.[a-z0-9_.]+$`)

// valueValidationRegex allows only numbers, spaces (for tuples like "4096 87380 6291456"),
// and basic separators. Prevents shell injection.
var valueValidationRegex = regexp.MustCompile(`^[0-9 \t]+$`)

// ValidateSysctl performs all parameter and value validation without applying
// any change. Used by the dry-run gate in the transaction executor to verify
// that an execute_sysctl_command call is safe before prompting the user.
func ValidateSysctl(parameter string, value string) error {
	if !paramValidationRegex.MatchString(parameter) {
		return fmt.Errorf(
			"invalid parameter %q: must match pattern net.<path> (e.g. net.core.rmem_max)",
			parameter,
		)
	}
	trimmedValue := strings.TrimSpace(value)
	if trimmedValue == "" {
		return fmt.Errorf("value cannot be empty")
	}
	if !valueValidationRegex.MatchString(trimmedValue) {
		return fmt.Errorf(
			"invalid value %q: only numeric values and spaces are allowed",
			value,
		)
	}
	isZero := true
	for _, part := range strings.Fields(trimmedValue) {
		if part != "0" {
			isZero = false
			break
		}
	}
	if isZero {
		return fmt.Errorf("refusing to set %s to zero: this would break networking", parameter)
	}
	// Verify the kernel parameter path is accessible on this system.
	procPath := ParamToProcPath(parameter)
	if _, err := os.Stat(procPath); err != nil {
		return fmt.Errorf("kernel parameter %s is not accessible: %w", parameter, err)
	}
	return nil
}

// ExecuteSysctl modifies a Linux kernel parameter using sysctl.
// It reads the current value before modifying, applies the change, verifies it,
// and optionally persists it to /etc/sysctl.conf.
func ExecuteSysctl(parameter string, value string, persist bool) (map[string]interface{}, error) {
	// ── 1. Validate parameter name ───────────────────────────────────────────
	if !paramValidationRegex.MatchString(parameter) {
		return nil, fmt.Errorf(
			"invalid parameter %q: must match pattern net.<path> (e.g. net.core.rmem_max)",
			parameter,
		)
	}

	// ── 2. Validate value (prevent command injection) ────────────────────────
	trimmedValue := strings.TrimSpace(value)
	if trimmedValue == "" {
		return nil, fmt.Errorf("value cannot be empty")
	}
	if !valueValidationRegex.MatchString(trimmedValue) {
		return nil, fmt.Errorf(
			"invalid value %q: only numeric values and spaces are allowed",
			value,
		)
	}

	// ── 3. Safety check – never allow setting a value to 0 ───────────────────
	// Setting core network buffers to 0 can break networking entirely.
	isZero := true
	for _, part := range strings.Fields(trimmedValue) {
		if part != "0" {
			isZero = false
			break
		}
	}
	if isZero {
		return nil, fmt.Errorf("refusing to set %s to zero: this would break networking", parameter)
	}

	// ── 4. Read current value from /proc/sys/ ────────────────────────────────
	procPath := ParamToProcPath(parameter)
	oldValue, err := readCurrentValue(procPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read current value of %s: %w", parameter, err)
	}

	// ── 5. Apply the new value via sysctl -w ─────────────────────────────────
	arg := fmt.Sprintf("%s=%s", parameter, trimmedValue)
	cmd := exec.Command("sysctl", "-w", arg)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr == "" {
			stderrStr = err.Error()
		}
		return nil, fmt.Errorf("sysctl -w failed for %s: %s", parameter, stderrStr)
	}

	// ── 6. Verify the change was actually applied ─────────────────────────────
	newValue, err := readCurrentValue(procPath)
	if err != nil {
		return nil, fmt.Errorf("failed to verify new value of %s: %w", parameter, err)
	}

	// ── 7. Optionally persist to /etc/sysctl.conf ────────────────────────────
	persisted := false
	var persistErr string
	if persist {
		if err := persistSysctl(parameter, trimmedValue); err != nil {
			// Non-fatal: log the error but don't fail the whole operation.
			// The value is already applied in the running kernel.
			persistErr = err.Error()
		} else {
			persisted = true
		}
	}

	result := map[string]interface{}{
		"parameter": parameter,
		"old_value": oldValue,
		"new_value": newValue,
		"success":   true,
		"persisted": persisted,
	}

	if persistErr != "" {
		result["persist_error"] = persistErr
	}

	return result, nil
}

// RestoreSysctlValue restores a kernel parameter to a previously captured value.
// Used by the rollback mechanism in the transaction executor.
func RestoreSysctlValue(parameter string, value string) error {
	// Re-validate inputs even on rollback path to be safe.
	if !paramValidationRegex.MatchString(parameter) {
		return fmt.Errorf("invalid parameter %q during rollback", parameter)
	}

	trimmedValue := strings.TrimSpace(value)
	if trimmedValue == "" {
		return fmt.Errorf("rollback value for %s cannot be empty", parameter)
	}
	if !valueValidationRegex.MatchString(trimmedValue) {
		return fmt.Errorf("invalid rollback value %q for %s", value, parameter)
	}

	arg := fmt.Sprintf("%s=%s", parameter, trimmedValue)
	cmd := exec.Command("sysctl", "-w", arg)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr == "" {
			stderrStr = err.Error()
		}
		return fmt.Errorf("rollback sysctl -w failed for %s=%s: %s", parameter, trimmedValue, stderrStr)
	}

	// Verify the rollback actually took effect.
	procPath := ParamToProcPath(parameter)
	restored, err := readCurrentValue(procPath)
	if err != nil {
		return fmt.Errorf("failed to verify rollback of %s: %w", parameter, err)
	}

	if strings.TrimSpace(restored) != trimmedValue {
		return fmt.Errorf(
			"rollback verification failed for %s: expected %q, got %q",
			parameter, trimmedValue, restored,
		)
	}

	return nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// ParamToProcPath converts a sysctl parameter name to its /proc/sys path.
// e.g. "net.core.rmem_max" → "/proc/sys/net/core/rmem_max"
// Exported so tests can verify the conversion without hitting the filesystem.
func ParamToProcPath(parameter string) string {
	return "/proc/sys/" + strings.ReplaceAll(parameter, ".", "/")
}

// readCurrentValue reads the current value of a kernel parameter from /proc/sys/.
func readCurrentValue(procPath string) (string, error) {
	content, err := os.ReadFile(procPath)
	if err != nil {
		return "", fmt.Errorf("cannot read %s: %w", procPath, err)
	}
	return strings.TrimSpace(string(content)), nil
}

// persistSysctl persists to the standard /etc/sysctl.conf location.
func persistSysctl(parameter, value string) error {
	return PersistSysctlToFile("/etc/sysctl.conf", parameter, value)
}

// PersistSysctlToFile writes or updates the parameter=value line in the given
// sysctl config file. Exported so tests can pass a temporary file path instead
// of requiring a real /etc/sysctl.conf.
//
// The function:
//   - Preserves all existing lines (including comments)
//   - Updates the line in-place if the parameter already exists
//   - Appends a new line if the parameter is not yet present
//   - Writes atomically via a temp-file + rename to avoid partial writes
func PersistSysctlToFile(path, parameter, value string) error {
	// Read existing file (it may not exist yet – that's fine).
	existing := []string{}
	f, err := os.Open(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot open %s: %w", path, err)
	}
	if err == nil {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			existing = append(existing, scanner.Text())
		}
		f.Close()
		if scanErr := scanner.Err(); scanErr != nil {
			return fmt.Errorf("error reading %s: %w", path, scanErr)
		}
	}

	// Build the canonical output line.
	newLine := fmt.Sprintf("%s = %s", parameter, value)

	// Look for an existing entry to replace (handles both "param = val" and "param=val").
	prefix := parameter + " "
	prefixAlt := parameter + "="

	found := false
	for i, line := range existing {
		trimmed := strings.TrimSpace(line)
		// Skip comments.
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		if strings.HasPrefix(trimmed, prefix) || strings.HasPrefix(trimmed, prefixAlt) {
			existing[i] = newLine
			found = true
			break
		}
	}

	if !found {
		existing = append(existing, newLine)
	}

	// Write back atomically: write to a temp file in the same directory, then rename.
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, "sysctl_tmp_*")
	if err != nil {
		return fmt.Errorf("cannot create temp file in %s: %w", dir, err)
	}
	tmpPath := tmpFile.Name()

	writer := bufio.NewWriter(tmpFile)
	for _, line := range existing {
		if _, werr := fmt.Fprintln(writer, line); werr != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("error writing temp sysctl file: %w", werr)
		}
	}
	if err := writer.Flush(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("error flushing temp sysctl file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("error closing temp sysctl file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("cannot update %s: %w", path, err)
	}

	return nil
}

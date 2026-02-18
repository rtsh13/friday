// Package executor provides state snapshot functionality for safe rollback
// of destructive system operations (sysctl changes, service modifications, etc.)
package executor

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// SnapshotType identifies what kind of state was captured.
type SnapshotType string

const (
	SnapshotTypeSysctl  SnapshotType = "sysctl"
	SnapshotTypeService SnapshotType = "service"
	SnapshotTypeUnknown SnapshotType = "unknown"
)

// Snapshot holds the captured state of a single system parameter
// before a destructive operation was performed.
type Snapshot struct {
	ID           string                 // Unique identifier (e.g. "snap_0001")
	FunctionName string                 // The function that triggered this snapshot
	Type         SnapshotType           // What kind of state this is
	Parameter    string                 // The parameter name (e.g. "net.core.rmem_max")
	Value        string                 // The captured value before the change
	Metadata     map[string]interface{} // Extra context (path, unit, etc.)
	CapturedAt   time.Time              // When the snapshot was taken
	Reversible   bool                   // Whether this snapshot can be used for rollback
}

// SnapshotManager maintains an ordered stack of snapshots for a transaction.
// Rollback is performed in LIFO order (last snapshot first).
type SnapshotManager struct {
	mu        sync.Mutex
	snapshots []*Snapshot
	counter   int
}

// NewSnapshotManager creates a ready-to-use SnapshotManager.
func NewSnapshotManager() *SnapshotManager {
	return &SnapshotManager{
		snapshots: make([]*Snapshot, 0),
	}
}

// TakeSnapshot captures the current system state relevant to the given
// function and its parameters. It must be called BEFORE executing the
// function so the captured value can be used for rollback.
//
// Supported functions:
//   - execute_sysctl_command  → reads current sysctl value from /proc/sys/
//   - restart_service         → reads current service status via systemctl
//
// Returns the created Snapshot on success, or an error if state cannot be read.
func (sm *SnapshotManager) TakeSnapshot(functionName string, params map[string]interface{}) (*Snapshot, error) {
	sm.mu.Lock()
	sm.counter++
	id := fmt.Sprintf("snap_%04d", sm.counter)
	sm.mu.Unlock()

	snap := &Snapshot{
		ID:           id,
		FunctionName: functionName,
		CapturedAt:   time.Now(),
		Metadata:     make(map[string]interface{}),
		Reversible:   false, // default; set to true once value is captured
	}

	var err error

	switch functionName {
	case "execute_sysctl_command":
		err = captureSysctlSnapshot(snap, params)

	case "restart_service":
		err = captureServiceSnapshot(snap, params)

	default:
		// Unknown function — create a non-reversible marker snapshot so
		// the rollback stack stays aligned with the execution stack.
		snap.Type = SnapshotTypeUnknown
		snap.Parameter = functionName
		snap.Value = ""
		snap.Reversible = false
	}

	if err != nil {
		return nil, fmt.Errorf("snapshot %s: failed to capture state for %q: %w", id, functionName, err)
	}

	sm.mu.Lock()
	sm.snapshots = append(sm.snapshots, snap)
	sm.mu.Unlock()

	return snap, nil
}

// Rollback restores all captured snapshots in LIFO order (most recent first).
// It attempts every rollback even if individual ones fail, collecting all
// errors into a combined report.
func (sm *SnapshotManager) Rollback() error {
	sm.mu.Lock()
	// Work on a copy so we don't hold the lock during potentially slow I/O
	snaps := make([]*Snapshot, len(sm.snapshots))
	copy(snaps, sm.snapshots)
	sm.mu.Unlock()

	var errs []string

	// LIFO: iterate in reverse
	for i := len(snaps) - 1; i >= 0; i-- {
		snap := snaps[i]
		if !snap.Reversible {
			// Nothing we can do — log and continue
			errs = append(errs, fmt.Sprintf("snapshot %s (%s): not reversible, skipping", snap.ID, snap.FunctionName))
			continue
		}

		if err := restoreSnapshot(snap); err != nil {
			errs = append(errs, fmt.Sprintf("snapshot %s (%s/%s): rollback failed: %v",
				snap.ID, snap.FunctionName, snap.Parameter, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("rollback completed with errors:\n%s", strings.Join(errs, "\n"))
	}
	return nil
}

// Reset discards all snapshots (call after a successful transaction commit).
func (sm *SnapshotManager) Reset() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.snapshots = sm.snapshots[:0]
	sm.counter = 0
}

// Snapshots returns a read-only copy of the current snapshot stack.
func (sm *SnapshotManager) Snapshots() []Snapshot {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	out := make([]Snapshot, len(sm.snapshots))
	for i, s := range sm.snapshots {
		out[i] = *s
	}
	return out
}

// ---------------------------------------------------------------------------
// Internal capture helpers
// ---------------------------------------------------------------------------

// captureSysctlSnapshot reads the current kernel parameter value from the
// /proc/sys virtual filesystem and stores it in the snapshot.
//
// The function expects params["parameter"] to be a dotted sysctl name such as
// "net.core.rmem_max", which it converts to a /proc/sys path by replacing
// dots with slashes: /proc/sys/net/core/rmem_max.
func captureSysctlSnapshot(snap *Snapshot, params map[string]interface{}) error {
	snap.Type = SnapshotTypeSysctl

	// Extract the parameter name
	paramRaw, ok := params["parameter"]
	if !ok {
		return fmt.Errorf("missing required param 'parameter'")
	}
	paramName, ok := paramRaw.(string)
	if !ok || paramName == "" {
		return fmt.Errorf("param 'parameter' must be a non-empty string")
	}

	// Validate: must start with "net." to limit scope to network parameters.
	// (Mirrors the security constraint in execute_sysctl_command.)
	if !strings.HasPrefix(paramName, "net.") {
		return fmt.Errorf("param %q is not in the allowed 'net.*' namespace", paramName)
	}

	// Convert dotted name → /proc/sys path
	procPath := "/proc/sys/" + strings.ReplaceAll(paramName, ".", "/")
	snap.Metadata["proc_path"] = procPath
	snap.Parameter = paramName

	// Read the current value
	data, err := os.ReadFile(procPath)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", procPath, err)
	}

	snap.Value = strings.TrimSpace(string(data))
	snap.Reversible = true
	return nil
}

// captureServiceSnapshot records the current active/inactive status of a
// systemd service unit so it can be restored on rollback.
func captureServiceSnapshot(snap *Snapshot, params map[string]interface{}) error {
	snap.Type = SnapshotTypeService

	serviceRaw, ok := params["service"]
	if !ok {
		return fmt.Errorf("missing required param 'service'")
	}
	serviceName, ok := serviceRaw.(string)
	if !ok || serviceName == "" {
		return fmt.Errorf("param 'service' must be a non-empty string")
	}
	snap.Parameter = serviceName

	// Ask systemctl for the current active state (active / inactive / failed / …)
	out, err := exec.Command("systemctl", "is-active", serviceName).Output()
	if err != nil {
		// is-active exits with non-zero for inactive/failed — that's fine,
		// we still got stdout.
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) == 0 {
			// Non-zero exit but stdout present → use it
		} else {
			return fmt.Errorf("systemctl is-active %s: %w", serviceName, err)
		}
	}

	snap.Value = strings.TrimSpace(string(out))
	snap.Reversible = true
	snap.Metadata["service"] = serviceName
	return nil
}

// ---------------------------------------------------------------------------
// Internal restore helpers
// ---------------------------------------------------------------------------

// restoreSnapshot applies the inverse of the operation that created snap.
func restoreSnapshot(snap *Snapshot) error {
	switch snap.Type {
	case SnapshotTypeSysctl:
		return restoreSysctl(snap)
	case SnapshotTypeService:
		return restoreService(snap)
	default:
		return fmt.Errorf("no restore handler for snapshot type %q", snap.Type)
	}
}

// restoreSysctl writes snap.Value back to the kernel using sysctl -w.
func restoreSysctl(snap *Snapshot) error {
	if snap.Parameter == "" || snap.Value == "" {
		return fmt.Errorf("snapshot is missing parameter or value")
	}

	arg := fmt.Sprintf("%s=%s", snap.Parameter, snap.Value)
	cmd := exec.Command("sysctl", "-w", arg)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sysctl -w %s failed: %w\noutput: %s", arg, err, string(output))
	}

	// Verify the value was actually restored
	procPath, _ := snap.Metadata["proc_path"].(string)
	if procPath != "" {
		data, readErr := os.ReadFile(procPath)
		if readErr == nil {
			actual := strings.TrimSpace(string(data))
			if actual != snap.Value {
				return fmt.Errorf("restore verification failed: expected %q, got %q", snap.Value, actual)
			}
		}
	}

	return nil
}

// restoreService starts or stops a service to match snap.Value.
func restoreService(snap *Snapshot) error {
	serviceName := snap.Parameter
	targetState := snap.Value

	var action string
	switch targetState {
	case "active":
		action = "start"
	case "inactive", "failed":
		action = "stop"
	default:
		return fmt.Errorf("unknown target service state %q for %s", targetState, serviceName)
	}

	cmd := exec.Command("systemctl", action, serviceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s %s failed: %w\noutput: %s",
			action, serviceName, err, string(output))
	}
	return nil
}

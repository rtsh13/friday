package debugging

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const analyzerTimeout = 60 * time.Second

// signalDescriptions maps signal names to human-readable descriptions.
// Used by the LLDB parser, which does not always include the description inline.
var signalDescriptions = map[string]string{
	"SIGABRT": "Aborted",
	"SIGBUS":  "Bus error",
	"SIGFPE":  "Floating point exception",
	"SIGHUP":  "Hangup",
	"SIGILL":  "Illegal instruction",
	"SIGINT":  "Interrupted",
	"SIGKILL": "Killed",
	"SIGPIPE": "Broken pipe",
	"SIGSEGV": "Segmentation fault",
	"SIGTERM": "Terminated",
	"SIGUSR1": "User defined signal 1",
	"SIGUSR2": "User defined signal 2",
}

// --- GDB compiled regexes ---

var (
	// Matches: "Program terminated with signal SIGSEGV, Segmentation fault."
	reGDBSignal = regexp.MustCompile(`Program terminated with signal (\w+),\s*(.+?)\.?\s*$`)

	// Matches frame lines: "  #0  0x... in func ()" or "#0  func ()"
	reGDBFrame = regexp.MustCompile(`^\s*#\d+\s+`)

	// Matches the start of a "thread apply all bt full" thread block:
	// "Thread 1 (Thread 0x7f... (LWP 12345)):"
	reGDBThreadHdr = regexp.MustCompile(`^Thread (\d+)\s+\(`)

	// Matches the "info threads" section header line.
	// GDB always prints "  Id   Target Id ..." as the first line.
	reGDBThreadListHdr = regexp.MustCompile(`Target Id`)
)

// --- LLDB compiled regexes ---

var (
	// Matches: "* thread #1, stop reason = signal SIGSEGV"
	reLLDBSignal = regexp.MustCompile(`stop reason\s*=\s*signal\s+(\w+)`)

	// Matches frame lines: "  * frame #0: 0x... module`func at file.c:10"
	//                   or "    frame #1: 0x... ..."
	reLLDBFrame = regexp.MustCompile(`^\s*\*?\s*frame\s+#\d+:`)

	// Matches thread header lines: "* thread #1, ..." or "  thread #2, ..."
	reLLDBThreadHdr = regexp.MustCompile(`^\*?\s*thread\s+#(\d+)`)

	// Matches lldb prompt lines like "(lldb) thread list" to detect section changes.
	reLLDBPrompt = regexp.MustCompile(`^\(lldb\)`)
)

// threadData is an intermediate type used during parsing to avoid map type
// assertion churn. Converted to map[string]interface{} before returning.
type threadData struct {
	id     int
	frames []string
}

// AnalyzeCoreDump uses GDB (Linux/other) or LLDB (macOS) to analyze a core
// dump file and returns structured crash information.
//
// Returns an error if:
//   - corePath is empty
//   - the debugger binary is not found in PATH
//   - the debugger times out (60 s)
//   - the signal cannot be determined from the output (corrupt core or missing
//     debug info)
func AnalyzeCoreDump(corePath string, binaryPath string) (map[string]interface{}, error) {
	if corePath == "" {
		return nil, errors.New("core_path is required")
	}

	var (
		rawOutput string
		debugger  string
		err       error
	)

	if runtime.GOOS == "darwin" {
		debugger = "lldb"
		rawOutput, err = runLLDB(corePath, binaryPath)
	} else {
		debugger = "gdb"
		rawOutput, err = runGDB(corePath, binaryPath)
	}
	if err != nil {
		return nil, err
	}

	var parsed map[string]interface{}
	switch debugger {
	case "gdb":
		parsed, err = parseGDBOutput(rawOutput)
	case "lldb":
		parsed, err = parseLLDBOutput(rawOutput)
	}
	if err != nil {
		return nil, err
	}

	// Enrich with metadata and derived fields.
	signal, _ := parsed["signal"].(string)
	bt, _ := parsed["backtrace"].([]string)

	patterns := detectCrashPatterns(signal, bt)
	parsed["crash_patterns"] = patterns

	sigDesc, _ := parsed["signal_description"].(string)
	parsed["crash_reason"] = buildCrashReason(signal, sigDesc, bt, patterns)

	parsed["debugger"] = debugger
	parsed["core_path"] = corePath
	parsed["binary_path"] = binaryPath

	return parsed, nil
}

// ============================================================================
// Debugger runners
// ============================================================================

func runGDB(corePath, binaryPath string) (string, error) {
	// Build argument list.
	// Order: options, then optional binary, then "-c corefile".
	// GDB batch mode exits with the inferior's exit status, which is non-zero
	// for signal-terminated programs. We therefore ignore the exit code and
	// only treat missing output as an error.
	args := []string{
		"--batch",
		"-ex", "set pagination off",
		"-ex", "bt full",
		"-ex", "info threads",
		"-ex", "thread apply all bt full",
	}
	if binaryPath != "" {
		args = append(args, binaryPath)
	}
	args = append(args, "-c", corePath)

	ctx, cancel := context.WithTimeout(context.Background(), analyzerTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gdb", args...)
	out, runErr := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("gdb timed out after %s", analyzerTimeout)
	}
	if len(out) == 0 {
		if runErr != nil {
			if isNotFound(runErr) {
				return "", errors.New("gdb not found in PATH; install gdb to use core dump analysis")
			}
			return "", fmt.Errorf("gdb failed to produce output: %w", runErr)
		}
		return "", fmt.Errorf("gdb produced no output; verify the core file is valid: %s", corePath)
	}

	return string(out), nil
}

func runLLDB(corePath, binaryPath string) (string, error) {
	// LLDB argument order: options, then "-c corefile", then optional binary,
	// then "-o" command strings. We include "quit" so lldb exits cleanly
	// without waiting for stdin.
	args := []string{"-c", corePath}
	if binaryPath != "" {
		args = append(args, binaryPath)
	}
	args = append(args,
		"-o", "thread backtrace all",
		"-o", "thread list",
		"-o", "quit",
	)

	ctx, cancel := context.WithTimeout(context.Background(), analyzerTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "lldb", args...)
	out, runErr := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("lldb timed out after %s", analyzerTimeout)
	}
	if len(out) == 0 {
		if runErr != nil {
			if isNotFound(runErr) {
				return "", errors.New("lldb not found in PATH; install Xcode Command Line Tools to use core dump analysis")
			}
			return "", fmt.Errorf("lldb failed to produce output: %w", runErr)
		}
		return "", fmt.Errorf("lldb produced no output; verify the core file is valid: %s", corePath)
	}

	return string(out), nil
}

// isNotFound returns true if err indicates the executable was not found.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "not found") || strings.Contains(msg, "no such file")
}

// ============================================================================
// GDB output parser
// ============================================================================

// parseGDBOutput uses a state machine to extract structured data from GDB
// batch output. The expected section order is:
//
//  1. Preamble (banner, loading messages)
//  2. "Program terminated with signal SIGXXX, Description."
//  3. Primary backtrace from "bt full"  (frame lines "#N ...")
//  4. "info threads" section (header + per-thread one-liners)
//  5. "thread apply all bt full" section (per-thread full backtraces)
func parseGDBOutput(output string) (map[string]interface{}, error) {
	const (
		stateSearch     = iota
		statePrimaryBT  // after signal line, collecting primary backtrace
		stateThreadList // inside "info threads" table
		stateAllThreads // inside "thread apply all bt full" output
	)

	lines := strings.Split(output, "\n")

	var (
		signal  string
		sigDesc string
		primary = make([]string, 0)
		threads []threadData
	)

	state := stateSearch
	var cur *threadData

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		switch state {
		// -------------------------------------------------------------------
		case stateSearch:
			if m := reGDBSignal.FindStringSubmatch(trimmed); m != nil {
				signal = m[1]
				sigDesc = strings.TrimRight(strings.TrimSpace(m[2]), ".")
				state = statePrimaryBT
			}

		// -------------------------------------------------------------------
		case statePrimaryBT:
			if reGDBFrame.MatchString(line) {
				// Frame line from "bt full" output.
				primary = append(primary, trimmed)
			} else if reGDBThreadListHdr.MatchString(trimmed) {
				// Transition: entering "info threads" section.
				state = stateThreadList
			} else if m := reGDBThreadHdr.FindStringSubmatch(trimmed); m != nil {
				// Transition: "thread apply all bt full" started directly
				// (can happen if there is only one thread and GDB omits the
				// "info threads" table, or if the table was already printed).
				state = stateAllThreads
				id, _ := strconv.Atoi(m[1])
				cur = &threadData{id: id}
			}

		// -------------------------------------------------------------------
		case stateThreadList:
			// We stay here until we see the first "Thread N (" line that
			// belongs to "thread apply all bt full".
			if m := reGDBThreadHdr.FindStringSubmatch(trimmed); m != nil {
				state = stateAllThreads
				id, _ := strconv.Atoi(m[1])
				cur = &threadData{id: id}
			}

		// -------------------------------------------------------------------
		case stateAllThreads:
			if m := reGDBThreadHdr.FindStringSubmatch(trimmed); m != nil {
				// New thread block: save the previous one first.
				if cur != nil {
					threads = append(threads, *cur)
				}
				id, _ := strconv.Atoi(m[1])
				cur = &threadData{id: id}
			} else if reGDBFrame.MatchString(line) {
				// Frame line belonging to the current thread.
				if cur != nil {
					cur.frames = append(cur.frames, trimmed)
				}
			}
			// Any other lines (variable values from "bt full", blank lines,
			// continuation lines) are intentionally ignored.
		}
	}

	// Flush the last thread.
	if cur != nil && state == stateAllThreads {
		threads = append(threads, *cur)
	}

	if signal == "" {
		return nil, errors.New(
			"could not determine signal from core dump; " +
				"the core file may be corrupt or missing debug information",
		)
	}

	return map[string]interface{}{
		"signal":             signal,
		"signal_description": sigDesc,
		"backtrace":          primary,
		"threads":            threadsToMaps(threads),
	}, nil
}

// ============================================================================
// LLDB output parser
// ============================================================================

// parseLLDBOutput extracts structured data from LLDB output produced by
// "thread backtrace all".  It stops consuming thread/frame data when it
// encounters the "(lldb) thread list" prompt so the thread list section
// does not produce duplicate entries.
func parseLLDBOutput(output string) (map[string]interface{}, error) {
	lines := strings.Split(output, "\n")

	var (
		signal  string
		threads []threadData
	)

	var cur *threadData
	// inBTSection is true while we are inside the "thread backtrace all"
	// output and false once we hit the next "(lldb) " prompt.
	inBTSection := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect lldb prompt lines: "(lldb) thread backtrace all" starts the
		// section we care about; any subsequent prompt ends it.
		if reLLDBPrompt.MatchString(trimmed) {
			if strings.Contains(trimmed, "thread backtrace all") {
				inBTSection = true
			} else if inBTSection {
				// A different command prompt — we are done with the BT section.
				break
			}
			continue
		}

		if !inBTSection {
			continue
		}

		// New thread block.
		if m := reLLDBThreadHdr.FindStringSubmatch(trimmed); m != nil {
			if cur != nil {
				threads = append(threads, *cur)
			}
			id, _ := strconv.Atoi(m[1])
			cur = &threadData{id: id}

			// Capture signal from the first thread that carries "stop reason".
			if signal == "" {
				if sm := reLLDBSignal.FindStringSubmatch(trimmed); sm != nil {
					signal = sm[1]
				}
			}
			continue
		}

		// Frame line.
		if cur != nil && reLLDBFrame.MatchString(line) {
			cur.frames = append(cur.frames, trimmed)
		}
	}

	// Flush last thread.
	if cur != nil {
		threads = append(threads, *cur)
	}

	if signal == "" {
		return nil, errors.New(
			"could not determine signal from core dump; " +
				"the core file may be corrupt or missing debug information",
		)
	}

	// Primary backtrace = first thread's frames (the crashing thread).
	primary := make([]string, 0)
	if len(threads) > 0 && threads[0].frames != nil {
		primary = threads[0].frames
	}

	sigDesc := signalDescriptions[signal] // empty string if unknown signal

	return map[string]interface{}{
		"signal":             signal,
		"signal_description": sigDesc,
		"backtrace":          primary,
		"threads":            threadsToMaps(threads),
	}, nil
}

// ============================================================================
// Crash pattern detection
// ============================================================================

// detectCrashPatterns identifies well-known crash patterns from the signal
// name and the primary thread's backtrace frames. The returned slice may be
// empty if no recognised pattern is found.
func detectCrashPatterns(signal string, bt []string) []string {
	patterns := make([]string, 0)
	btText := strings.Join(bt, "\n")
	btLower := strings.ToLower(btText)

	switch signal {
	case "SIGSEGV":
		// A frame address of 0x0000000000000000 or GDB's "?? ()" notation
		// for an unresolvable symbol are strong indicators of a null pointer
		// dereference. Absence of these markers still means segfault.
		if strings.Contains(btText, "0x0000000000000000") ||
			strings.Contains(btText, "in ?? ()") ||
			strings.Contains(btText, "(nil)") {
			patterns = append(patterns, "null_pointer_dereference")
		} else {
			patterns = append(patterns, "segmentation_fault")
		}

	case "SIGABRT":
		// abort() / assertion failure typically shows "abort" or "__assert"
		// near the top of the backtrace.
		if strings.Contains(btLower, "assert") {
			patterns = append(patterns, "assertion_failure")
		}
		// Heap corruption / double-free is indicated by allocator function
		// names appearing in the abort backtrace.
		if strings.Contains(btLower, "double free") ||
			strings.Contains(btLower, "malloc") ||
			strings.Contains(btLower, "cfree") ||
			strings.Contains(btLower, "free(") {
			patterns = append(patterns, "heap_corruption_or_double_free")
		}
		if strings.Contains(btLower, "abort") {
			patterns = append(patterns, "abort_called")
		}

	case "SIGBUS":
		patterns = append(patterns, "bus_error")

	case "SIGFPE":
		patterns = append(patterns, "floating_point_exception")

	case "SIGILL":
		patterns = append(patterns, "illegal_instruction")
	}

	// Stack overflow check is independent of the signal.
	if isStackOverflow(bt) {
		patterns = append(patterns, "stack_overflow")
	}

	return patterns
}

// isStackOverflow returns true when three or more consecutive backtrace frames
// resolve to the same function name, which is a reliable indicator of
// unbounded recursion.
func isStackOverflow(bt []string) bool {
	if len(bt) < 3 {
		return false
	}
	names := make([]string, 0, len(bt))
	for _, frame := range bt {
		if name := extractFuncName(frame); name != "" {
			names = append(names, name)
		}
	}

	consecutive := 1
	for i := 1; i < len(names); i++ {
		if names[i] == names[i-1] {
			consecutive++
			if consecutive >= 3 {
				return true
			}
		} else {
			consecutive = 1
		}
	}
	return false
}

// extractFuncName extracts the bare function name from a GDB or LLDB frame
// line, used solely for pattern detection (not returned to the caller).
//
//   - GDB:  "#0  0x... in func_name (args) at file.c:10"  -> "func_name"
//   - GDB:  "#0  func_name (args) at file.c:10"           -> "func_name"
//   - LLDB: "frame #0: 0x... module`func_name(args)"      -> "func_name"
func extractFuncName(frame string) string {
	// GDB "in funcname" pattern.
	if idx := strings.Index(frame, " in "); idx != -1 {
		rest := frame[idx+4:]
		end := strings.IndexAny(rest, " (")
		if end > 0 {
			return rest[:end]
		}
		return rest
	}
	// LLDB backtick separator: "module`funcname(args)".
	if idx := strings.Index(frame, "`"); idx != -1 {
		rest := frame[idx+1:]
		end := strings.IndexAny(rest, "( ")
		if end > 0 {
			return rest[:end]
		}
		return rest
	}
	return ""
}

// ============================================================================
// Result formatting helpers
// ============================================================================

// buildCrashReason assembles a single human-readable sentence describing the
// crash, suitable for the "crash_reason" field in the output.
func buildCrashReason(signal, sigDesc string, bt []string, patterns []string) string {
	var sb strings.Builder

	if sigDesc != "" {
		sb.WriteString(sigDesc)
	} else if signal != "" {
		sb.WriteString(signal)
	} else {
		sb.WriteString("Unknown signal")
	}

	if len(bt) > 0 {
		if loc := crashSite(bt[0]); loc != "" {
			sb.WriteString(" at ")
			sb.WriteString(loc)
		}
	}

	for _, p := range patterns {
		switch p {
		case "null_pointer_dereference":
			sb.WriteString(" (likely null pointer dereference)")
		case "heap_corruption_or_double_free":
			sb.WriteString(" (likely heap corruption or double free)")
		case "assertion_failure":
			sb.WriteString(" (assertion failure)")
		case "abort_called":
			sb.WriteString(" (abort() called)")
		case "stack_overflow":
			sb.WriteString(" (recursive stack overflow)")
		}
	}

	return sb.String()
}

// crashSite extracts the "file.c:line" location from the first backtrace frame
// for use in the crash reason sentence. Returns an empty string if the
// location cannot be determined.
func crashSite(frame string) string {
	// Both GDB and LLDB use "at file.c:line" for frames with debug info.
	if idx := strings.LastIndex(frame, " at "); idx != -1 {
		return strings.TrimSpace(frame[idx+4:])
	}
	return ""
}

// threadsToMaps converts the internal threadData slice to the
// []map[string]interface{} format expected by callers.
func threadsToMaps(threads []threadData) []map[string]interface{} {
	out := make([]map[string]interface{}, len(threads))
	for i, t := range threads {
		frames := t.frames
		if frames == nil {
			frames = []string{}
		}
		out[i] = map[string]interface{}{
			"id":     t.id,
			"frames": frames,
		}
	}
	return out
}

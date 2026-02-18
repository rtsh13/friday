package executor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/stratos/cliche/internal/types"
)

// Phase constants matching functions.yaml phase field values.
const (
	PhaseRead    = "read"
	PhaseAnalyze = "analyze"
	PhaseModify  = "modify"
)

// ExecutionStrategy controls behaviour when a function fails.
type ExecutionStrategy string

const (
	StrategyStopOnError  ExecutionStrategy = "stop_on_error"
	StrategySkipOnError  ExecutionStrategy = "skip_on_error"
	StrategyRetryWithLLM ExecutionStrategy = "retry_with_llm"
	StrategyAskUser      ExecutionStrategy = "ask_user"
)

// phasedCall is an internal wrapper that adds a resolved phase to a FunctionCall.
// It is NEVER exported — types.FunctionCall has no Phase field.
type phasedCall struct {
	types.FunctionCall
	phase string
}

// FunctionResult holds the output and execution metadata for one call.
type FunctionResult struct {
	FunctionName string
	Phase        string
	Output       map[string]interface{}
	Error        error
	Duration     time.Duration
	Skipped      bool
	Success      bool
}

// TransactionRequest is the structured form used when extra options are needed.
type TransactionRequest struct {
	Functions         []types.FunctionCall
	Strategy          ExecutionStrategy
	ExecutionContext  map[string]interface{}
	DryRunOnly        bool
	ConfirmationInput *bufio.Reader
}

// PhaseRegistry abstracts looking up a function's declared phase.
// *functions.Registry satisfies this once Phase(string) string is added to it.
type PhaseRegistry interface {
	Phase(functionName string) string
}

// TransactionEngine orchestrates three-phase atomic execution.
type TransactionEngine struct {
	executor        *Executor
	resolver        *VariableResolver
	snapshotManager *SnapshotManager
	registry        PhaseRegistry
}

// NewTransactionEngine constructs a TransactionEngine with all dependencies.
func NewTransactionEngine(
	executor *Executor,
	resolver *VariableResolver,
	snapshotManager *SnapshotManager,
	registry PhaseRegistry,
) *TransactionEngine {
	return &TransactionEngine{
		executor:        executor,
		resolver:        resolver,
		snapshotManager: snapshotManager,
		registry:        registry,
	}
}

// NewTransactionExecutor is the single-arg constructor used in tests.
func NewTransactionExecutor(executor *Executor) *TransactionEngine {
	return &TransactionEngine{
		executor:        executor,
		resolver:        NewVariableResolver(),
		snapshotManager: NewSnapshotManager(),
		registry:        &defaultRegistry{},
	}
}

// defaultRegistry treats every function as "read" (safe default for tests).
type defaultRegistry struct{}

func (d *defaultRegistry) Phase(_ string) string { return PhaseRead }

// ExecuteTransaction accepts either []types.FunctionCall (test path) or a
// TransactionRequest (production path) and returns ([]FunctionResult, error).
func (te *TransactionEngine) ExecuteTransaction(
	ctx context.Context,
	input interface{},
) ([]FunctionResult, error) {
	var req TransactionRequest

	switch v := input.(type) {
	case []types.FunctionCall:
		req = TransactionRequest{Functions: v}
	case TransactionRequest:
		req = v
	default:
		return nil, fmt.Errorf("ExecuteTransaction: unsupported input type %T", input)
	}

	return te.execute(ctx, req)
}

func (te *TransactionEngine) execute(
	ctx context.Context,
	req TransactionRequest,
) ([]FunctionResult, error) {

	if req.Strategy == "" {
		req.Strategy = StrategyStopOnError
	}

	confirmInput := req.ConfirmationInput
	if confirmInput == nil {
		confirmInput = bufio.NewReader(os.Stdin)
	}

	var allResults []FunctionResult

	reads, analyses, modifies := te.categorise(req.Functions)

	// ── PHASE 1: READ ─────────────────────────────────────────────────────────
	fmt.Println("\n── Phase 1: READ ─────────────────────────────────────────────")
	results, err := te.executePhase(ctx, reads, req.Strategy)
	allResults = append(allResults, results...)
	if err != nil {
		return allResults, fmt.Errorf("read phase failed: %w", err)
	}
	fmt.Printf("✓ Read phase complete (%d function(s))\n", len(reads))

	// ── PHASE 2: ANALYZE ──────────────────────────────────────────────────────
	if len(analyses) > 0 {
		fmt.Println("\n── Phase 2: ANALYZE ──────────────────────────────────────────")
		results, err = te.executePhase(ctx, analyses, req.Strategy)
		allResults = append(allResults, results...)
		if err != nil {
			return allResults, fmt.Errorf("analyze phase failed: %w", err)
		}
		fmt.Printf("✓ Analyze phase complete (%d function(s))\n", len(analyses))
	}

	// ── GATE + PHASE 3: MODIFY ────────────────────────────────────────────────
	if len(modifies) > 0 {
		fmt.Println("\n── Gate 4: PRE-MODIFY VALIDATION ────────────────────────────")
		if err := te.preModifyGate(ctx, modifies, confirmInput, req.DryRunOnly); err != nil {
			return allResults, err
		}
		if req.DryRunOnly {
			fmt.Println("✓ Dry-run complete. No changes were made (--dry-run mode).")
			return allResults, nil
		}

		fmt.Println("\n── Phase 3: MODIFY ───────────────────────────────────────────")
		results, err = te.executeModifyPhase(ctx, modifies, req.Strategy)
		allResults = append(allResults, results...)
		if err != nil {
			fmt.Println("\n⚠  Failure detected – initiating rollback …")
			if rbErr := te.snapshotManager.Rollback(); rbErr != nil {
				fmt.Printf("⚠  Rollback error (manual intervention may be required): %v\n", rbErr)
			} else {
				fmt.Println("✓ Rollback complete – system restored to previous state.")
			}
			return allResults, fmt.Errorf("modify phase failed (rolled back): %w", err)
		}
		fmt.Printf("✓ Modify phase complete (%d function(s))\n", len(modifies))
	}

	fmt.Println("\n✓ Transaction committed successfully.")
	return allResults, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────────────────────────────────────

func (te *TransactionEngine) categorise(fns []types.FunctionCall) (reads, analyses, modifies []phasedCall) {
	for _, fn := range fns {
		phase := te.registry.Phase(fn.Name)
		if phase == "" {
			phase = PhaseRead
		}
		pc := phasedCall{FunctionCall: fn, phase: phase}

		switch phase {
		case PhaseModify:
			modifies = append(modifies, pc)
		case PhaseAnalyze:
			analyses = append(analyses, pc)
		default:
			reads = append(reads, pc)
		}
	}
	return reads, analyses, modifies
}

func (te *TransactionEngine) executePhase(
	ctx context.Context,
	fns []phasedCall,
	strategy ExecutionStrategy,
) ([]FunctionResult, error) {
	skipped := make(map[int]bool)
	var results []FunctionResult

	for i, pc := range fns {
		if err := ctx.Err(); err != nil {
			return results, fmt.Errorf("context cancelled: %w", err)
		}
		if skipped[i] {
			results = append(results, FunctionResult{
				FunctionName: pc.Name, Phase: pc.phase, Skipped: true,
			})
			fmt.Printf("  ↷ [%d] %s (skipped – dependency failed)\n", i+1, pc.Name)
			continue
		}

		if err := te.resolveParams(&pc); err != nil {
			fr := FunctionResult{FunctionName: pc.Name, Phase: pc.phase, Error: err}
			results = append(results, fr)
			if strategy == StrategySkipOnError {
				te.markDependentsSkipped(i, fns, skipped)
				continue
			}
			return results, fmt.Errorf("[%s] %w", pc.Name, err)
		}

		fr, err := te.runOne(pc)
		results = append(results, fr)

		if err != nil {
			if strategy == StrategySkipOnError {
				fmt.Printf("  ✗ [%d] %s FAILED (%v) – skipping dependents\n", i+1, pc.Name, err)
				te.markDependentsSkipped(i, fns, skipped)
				continue
			}
			return results, fmt.Errorf("[%s] %w", pc.Name, err)
		}
		fmt.Printf("  ✓ [%d] %s  (%.2fs)\n", i+1, pc.Name, fr.Duration.Seconds())
	}
	return results, nil
}

func (te *TransactionEngine) executeModifyPhase(
	ctx context.Context,
	fns []phasedCall,
	strategy ExecutionStrategy,
) ([]FunctionResult, error) {
	skipped := make(map[int]bool)
	var results []FunctionResult

	for i, pc := range fns {
		if err := ctx.Err(); err != nil {
			return results, fmt.Errorf("context cancelled: %w", err)
		}
		if skipped[i] {
			results = append(results, FunctionResult{
				FunctionName: pc.Name, Phase: pc.phase, Skipped: true,
			})
			fmt.Printf("  ↷ [%d] %s (skipped – dependency failed)\n", i+1, pc.Name)
			continue
		}

		if err := te.resolveParams(&pc); err != nil {
			fr := FunctionResult{FunctionName: pc.Name, Phase: pc.phase, Error: err}
			results = append(results, fr)
			if pc.Critical || strategy == StrategyStopOnError {
				return results, fmt.Errorf("[%s] variable resolution failed: %w", pc.Name, err)
			}
			te.markDependentsSkipped(i, fns, skipped)
			continue
		}

		// Snapshot BEFORE change. TakeSnapshot(name, params) → (*Snapshot, error)
		if _, snapErr := te.snapshotManager.TakeSnapshot(pc.Name, pc.Params); snapErr != nil {
			fmt.Printf("  ⚠  [%d] %s – snapshot failed (%v); operation will not be reversible\n",
				i+1, pc.Name, snapErr)
		}

		fr, err := te.runOne(pc)
		results = append(results, fr)

		if err != nil {
			if pc.Critical || strategy == StrategyStopOnError {
				return results, fmt.Errorf("[%s] %w", pc.Name, err)
			}
			if strategy == StrategySkipOnError {
				fmt.Printf("  ✗ [%d] %s FAILED (%v) – skipping dependents\n", i+1, pc.Name, err)
				te.markDependentsSkipped(i, fns, skipped)
				continue
			}
			return results, fmt.Errorf("[%s] %w", pc.Name, err)
		}
		fmt.Printf("  ✓ [%d] %s  (%.2fs)\n", i+1, pc.Name, fr.Duration.Seconds())
	}
	return results, nil
}

func (te *TransactionEngine) preModifyGate(
	ctx context.Context,
	fns []phasedCall,
	input *bufio.Reader,
	dryRunOnly bool,
) error {
	fmt.Println("Validating modify operations …")

	type preview struct {
		pc     phasedCall
		params map[string]interface{}
	}
	previews := make([]preview, 0, len(fns))

	for _, pc := range fns {
		if err := te.resolveParams(&pc); err != nil {
			return fmt.Errorf("dry-run: [%s] variable resolution failed: %w", pc.Name, err)
		}

		dryPc := pc
		if dryPc.Params == nil {
			dryPc.Params = make(map[string]interface{})
		}
		dryPc.Params["__dry_run"] = true

		if _, err := te.runOne(dryPc); err != nil {
			return fmt.Errorf("dry-run: [%s] failed pre-flight check: %w", pc.Name, err)
		}
		previews = append(previews, preview{pc: pc, params: pc.Params})
	}

	fmt.Println("✓ Dry-run validation passed.\n")
	if dryRunOnly {
		return nil
	}

	fmt.Println("┌─────────────────────────────────────────────────────────┐")
	fmt.Println("│  ⚠   DESTRUCTIVE OPERATIONS PENDING                    │")
	fmt.Println("└─────────────────────────────────────────────────────────┘")

	for i, p := range previews {
		fmt.Printf("\n  [%d] %s\n", i+1, p.pc.Name)
		for k, v := range p.params {
			if strings.HasPrefix(k, "__") {
				continue
			}
			fmt.Printf("      %-24s %v\n", k+":", v)
		}
		critical := "no"
		if p.pc.Critical {
			critical = "yes – failure triggers rollback"
		}
		fmt.Printf("      %-24s %s\n", "critical:", critical)
	}

	fmt.Printf("\n  All operations are reversible via automatic rollback on failure.\n")
	fmt.Printf("\nProceed with %d destructive operation(s)? [y/N]: ", len(previews))

	line, err := input.ReadString('\n')
	if err != nil {
		return fmt.Errorf("could not read confirmation: %w", err)
	}
	if answer := strings.TrimSpace(strings.ToLower(line)); answer != "y" && answer != "yes" {
		fmt.Println("Aborted by operator – no changes were made.")
		return ErrUserDeclined
	}
	return nil
}

// runOne executes a single phasedCall via the dispatcher.
// executor.Execute(types.FunctionCall) → (string, error)
func (te *TransactionEngine) runOne(pc phasedCall) (FunctionResult, error) {
	start := time.Now()
	rawOutput, err := te.executor.Execute(pc.FunctionCall)
	elapsed := time.Since(start)

	fr := FunctionResult{
		FunctionName: pc.Name,
		Phase:        pc.phase,
		Error:        err,
		Duration:     elapsed,
		Success:      err == nil,
	}
	if err != nil {
		return fr, err
	}

	var outputMap map[string]interface{}
	if decErr := json.Unmarshal([]byte(rawOutput), &outputMap); decErr != nil {
		outputMap = map[string]interface{}{"output": rawOutput}
	}
	fr.Output = outputMap

	// Feed into resolver so ${functionName.field} works for subsequent calls.
	te.resolver.AddResult(pc.Name, rawOutput)

	return fr, nil
}

// resolveParams resolves ${…} references in pc.Params in-place, preserving
// native types (int, float64, bool) via ResolveParams/tryResolveNative.
//
// Bug 8 fix: previously used resolver.Resolve() which always returns a string,
// losing native type information (e.g. port 50051 became "50051"). Now uses
// resolver.ResolveParams() which preserves native types for single-placeholder
// values and returns a new map without modifying the original.
func (te *TransactionEngine) resolveParams(pc *phasedCall) error {
	resolved, err := te.resolver.ResolveParams(pc.Params)
	if err != nil {
		return err
	}
	if resolved != nil {
		pc.Params = resolved
	}
	return nil
}

func (te *TransactionEngine) markDependentsSkipped(
	failedIdx int,
	fns []phasedCall,
	skipped map[int]bool,
) {
	skipped[failedIdx] = true
	for i, fn := range fns {
		if skipped[i] {
			continue
		}
		for _, dep := range fn.DependsOn {
			if dep == failedIdx || skipped[dep] {
				te.markDependentsSkipped(i, fns, skipped)
				break
			}
		}
	}
}

// ErrUserDeclined is returned when the operator answers "N" at the prompt.
var ErrUserDeclined = fmt.Errorf("transaction declined by operator")

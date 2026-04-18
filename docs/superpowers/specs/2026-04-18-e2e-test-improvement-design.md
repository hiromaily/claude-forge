# E2E Test Improvement Design

Date: 2026-04-18

## Goal

Improve the reliability and maintainability of `pipeline_e2e_test.go` following the package restructuring (`internal/` → `engine/`, `intelligence/`, `handler/`, `pkg/`).

## Step 1: PhaseLog Completeness (Production Code)

**Problem**: 5 phases are excluded from PhaseLog verification in `TestE2E_SkippedPhasesInPhaseLog` via a hardcoded `excludedFromLog` map. This masks potential bugs where phases are silently consumed without logging.

**Solution**: Add `sm.PhaseLog()` calls for the 5 unlogged phases:

| Phase | Where to add PhaseLog | Model string |
|-------|----------------------|-------------|
| checkpoint-a | After checkpoint resolution (proceed/revise) in `pipeline_next_action.go` | `"checkpoint"` |
| checkpoint-b | Same location | `"checkpoint"` |
| final-commit | After `executeFinalCommit()` succeeds in `pipeline_next_action.go` | `"exec"` |
| pr-creation | When skipped due to `SkipPR=true` in `pipeline_next_action.go` | `"skipped"` |
| post-to-source | When skipped for text source type in `pipeline_next_action.go` | `"skipped"` |

**Test change**: Remove `excludedFromLog` map entirely. All phases except `setup` and `completed` must have a PhaseLog entry.

**File**: `internal/handler/tools/pipeline_next_action.go`

## Step 2: e2eConfig Extension and Setup Unification

**Problem**: `TestE2E_CheckpointRevisionFlow` duplicates 40 lines of workspace setup because `e2eConfig` lacks `autoApprove` and `skipPR` fields.

**Solution**: Extend `e2eConfig`:

```go
type e2eConfig struct {
    effort              string
    template            string
    reviewDesignVerdict string
    autoApprove         bool   // new: false → checkpoints pause
    skipPR              bool   // new: false → pr-creation runs
}
```

- Update `setupE2EWorkspace` to use `cfg.autoApprove` and `cfg.skipPR`
- Existing tests set `autoApprove: true, skipPR: true` explicitly
- `TestE2E_CheckpointRevisionFlow` uses `autoApprove: false` and removes its custom setup (lines 317-349)

**File**: `internal/handler/tools/pipeline_e2e_test.go`

## Step 3: Intermediate State Verification

**Problem**: Tests only verify final state (`currentPhase == completed`). Phase transition bugs (`pending → in_progress → completed`) are invisible.

**Solution**: Add an optional `onAction` callback to `e2eConfig`:

```go
type e2eConfig struct {
    // ...existing fields...
    onAction func(t *testing.T, action orchestrator.Action, state *state.State)
}
```

- Called after each `reportResult` with the current state read from disk
- Existing tests pass `onAction: nil` (no-op)
- New test `TestE2E_StateTransitions` uses the callback to verify:
  - `currentPhaseStatus` transitions correctly
  - `currentPhase` advances in the expected order
  - Skipped phases have `model="skipped"` in PhaseLog

**File**: `internal/handler/tools/pipeline_e2e_test.go`

## Step 4: Test Helper Consolidation

**Problem**: Relative testdata paths (`../../../pkg/ast/testdata`) are fragile and break on directory restructuring.

**Solution**: Add a `moduleRoot()` helper that walks up to find `go.mod`:

```go
func moduleRoot(t *testing.T) string {
    t.Helper()
    _, file, _, _ := runtime.Caller(0)
    dir := filepath.Dir(file)
    for {
        if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
            return dir
        }
        parent := filepath.Dir(dir)
        if parent == dir {
            t.Fatal("go.mod not found")
        }
        dir = parent
    }
}
```

Replace fragile relative paths in:
- `handler/tools/ast_summary_test.go`
- `handler/tools/ast_find_definition_test.go`
- `engine/state/manager_test.go`
- `intelligence/profile/analyzer_test.go`

Existing helper distribution (callTool in handlers_test.go, callNextAction in pipeline_next_action_test.go, etc.) is kept as-is — each helper stays with its cohesive test file.

**New file**: `internal/handler/tools/test_helpers_test.go`

## Files Changed

| File | Type | Description |
|------|------|-------------|
| `handler/tools/pipeline_next_action.go` | prod | PhaseLog for checkpoint, final-commit, pr-creation skip, post-to-source skip |
| `handler/tools/pipeline_e2e_test.go` | test | e2eConfig extension, setup unification, excludedFromLog removal, onAction callback, TestE2E_StateTransitions |
| `handler/tools/test_helpers_test.go` | new | moduleRoot() helper |
| `handler/tools/ast_summary_test.go` | test | moduleRoot-based path |
| `handler/tools/ast_find_definition_test.go` | test | moduleRoot-based path |
| `engine/state/manager_test.go` | test | moduleRoot-based path |
| `intelligence/profile/analyzer_test.go` | test | moduleRoot-based path |

## Execution Order

Steps 1-4 are sequential. Each step maintains green tests before proceeding to the next.

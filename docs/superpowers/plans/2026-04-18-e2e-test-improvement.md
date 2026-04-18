# E2E Test Improvement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Improve E2E test reliability and maintainability by ensuring all phases log to PhaseLog, unifying test setup, adding intermediate state verification, and eliminating fragile relative paths.

**Architecture:** Production code changes in `pipeline_next_action.go` add `PhaseLog` calls for 5 previously unlogged phases (checkpoint-a/b, final-commit, pr-creation skip, post-to-source skip). Test code changes extend `e2eConfig`, add state transition verification, and introduce a `moduleRoot()` helper to replace brittle `../../../` paths.

**Tech Stack:** Go 1.26, `mcp-go` framework, `testing` stdlib

**Spec:** `docs/superpowers/specs/2026-04-18-e2e-test-improvement-design.md`

---

### Task 1: Add PhaseLog for checkpoint resolution

**Files:**
- Modify: `mcp-server/internal/handler/tools/pipeline_next_action.go:189-241` (P8 checkpoint handler)

- [ ] **Step 1: Write the failing test**

Add a test in `pipeline_e2e_test.go` that verifies checkpoint phases appear in PhaseLog after pipeline completion with `autoApprove: false`:

```go
// Add at the end of pipeline_e2e_test.go
func TestE2E_CheckpointPhaseLog(t *testing.T) {
	t.Parallel()

	cfg := e2eConfig{
		effort:      state.EffortM,
		template:    state.TemplateStandard,
		autoApprove: false,
		skipPR:      true,
	}
	workspace, nextActionH, reportResultH := setupE2EWorkspace(t, cfg)

	// Run pipeline with auto-proceed at checkpoints (reuse CheckpointRevisionFlow loop logic
	// but always proceed).
	for range 60 {
		result, err := callNextAction(t, nextActionH, workspace)
		if err != nil {
			t.Fatalf("callNextAction: %v", err)
		}
		if result.IsError {
			t.Fatalf("MCP error: %s", textContent(result))
		}

		var resp nextActionResponse
		if err := json.Unmarshal([]byte(textContent(result)), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.Action.Type == orchestrator.ActionDone {
			break
		}

		reportPhase := resp.Action.Phase
		if resp.Action.Type == orchestrator.ActionCheckpoint && reportPhase == "" {
			reportPhase = resp.Action.Name
		}

		if resp.Action.Type == orchestrator.ActionCheckpoint {
			// Respond "proceed" to advance past checkpoint.
			result, err = callNextActionWithUserResponse(t, nextActionH, workspace, "proceed")
			if err != nil {
				t.Fatalf("callNextActionWithUserResponse: %v", err)
			}
			if result.IsError {
				t.Fatalf("MCP error on proceed: %s", textContent(result))
			}
			// Parse the response after proceed — it may be another action or done.
			if err := json.Unmarshal([]byte(textContent(result)), &resp); err != nil {
				t.Fatalf("unmarshal after proceed: %v", err)
			}
			if resp.Action.Type == orchestrator.ActionDone {
				break
			}
			reportPhase = resp.Action.Phase
			if resp.Action.Type == orchestrator.ActionCheckpoint && reportPhase == "" {
				reportPhase = resp.Action.Name
			}
		}

		switch resp.Action.Type {
		case orchestrator.ActionWriteFile:
			if err := os.WriteFile(resp.Action.Path, []byte(resp.Action.Content), 0o600); err != nil {
				t.Fatalf("write_file: %v", err)
			}
		case orchestrator.ActionSpawnAgent:
			approve := new(bool)
			*approve = true
			mockAgentExecute(t, workspace, resp.Action, cfg, approve)
		case orchestrator.ActionExec:
			// no-op
		}

		reportRes := callTool(t, reportResultH, map[string]any{
			"workspace":   workspace,
			"phase":       reportPhase,
			"tokens_used": 500,
			"duration_ms": 1000,
			"model":       "sonnet",
		})
		if reportRes.IsError {
			t.Fatalf("reportResult for %q: %s", reportPhase, textContent(reportRes))
		}
	}

	s, err := state.ReadState(workspace)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}

	logged := make(map[string]bool)
	for _, entry := range s.PhaseLog {
		logged[entry.Phase] = true
	}

	if !logged[state.PhaseCheckpointA] {
		t.Errorf("checkpoint-a not found in PhaseLog")
	}
	if !logged[state.PhaseCheckpointB] {
		t.Errorf("checkpoint-b not found in PhaseLog")
	}
}
```

Note: This test will NOT compile yet because `e2eConfig` doesn't have `autoApprove`/`skipPR` fields. We need Step 2 (Task 2) to add those. However, the intent of this test is clear — it serves as the design target. **Skip this test for now and come back after Task 2.**

- [ ] **Step 2: Add PhaseLog calls in checkpoint handler**

In `pipeline_next_action.go`, add `sm2.PhaseLog` after each checkpoint resolution:

After the `"proceed"` case (line ~195, after `sm2.PhaseComplete`):

```go
case "proceed":
    if completeErr := sm2.PhaseComplete(workspace, st.CurrentPhase); completeErr != nil {
        return errorf("checkpoint proceed %s: %v", st.CurrentPhase, completeErr)
    }
    // Record checkpoint resolution in PhaseLog for observability.
    _ = sm2.PhaseLog(workspace, st.CurrentPhase, 0, 0, "checkpoint")
```

After the `"revise"` case (line ~232, after the `sm2.Update` block closes):

```go
                    }
                    // Record checkpoint rewind in PhaseLog for observability.
                    _ = sm2.PhaseLog(workspace, st.CurrentPhase, 0, 0, "checkpoint")
```

After the `"abandon"` case (line ~234, before returning):

```go
case "abandon":
    if abandonErr := sm2.Abandon(workspace); abandonErr != nil {
        return errorf("checkpoint abandon: %v", abandonErr)
    }
    // Record checkpoint abandonment in PhaseLog.
    _ = sm2.PhaseLog(workspace, st.CurrentPhase, 0, 0, "checkpoint")
    return okJSON(nextActionResponse{
```

- [ ] **Step 3: Run tests**

```bash
cd mcp-server && go test ./internal/handler/tools/ -run TestE2E -count=1 -v
```

Expected: existing E2E tests pass. The new `TestE2E_CheckpointPhaseLog` test won't compile yet (blocked on Task 2).

- [ ] **Step 4: Commit**

```bash
git add mcp-server/internal/handler/tools/pipeline_next_action.go
git commit -m "feat(pipeline): add PhaseLog entries for checkpoint resolution"
```

---

### Task 2: Add PhaseLog for final-commit and extend e2eConfig

**Files:**
- Modify: `mcp-server/internal/handler/tools/pipeline_next_action.go:367-374` (final_commit handler)
- Modify: `mcp-server/internal/handler/tools/pipeline_e2e_test.go:22-72` (e2eConfig + setupE2EWorkspace)

- [ ] **Step 1: Add PhaseLog for final-commit**

In `pipeline_next_action.go`, after `executeFinalCommit` succeeds (line ~373):

```go
case orchestrator.ActionExec:
    // P4: intercept final_commit exec — handle entirely in Go.
    if len(action.Commands) > 0 && action.Commands[0] == "final_commit" {
        if finalErr := executeFinalCommit(workspace, sm2, kb); finalErr != nil {
            return errorf("final_commit: %v", finalErr)
        }
        // Record final-commit execution in PhaseLog for observability.
        _ = sm2.PhaseLog(workspace, state.PhaseFinalCommit, 0, 0, "exec")
        return okJSON(nextActionResponse{Action: orchestrator.NewDoneAction("pipeline completed", "")})
    }
```

- [ ] **Step 2: Extend e2eConfig and setupE2EWorkspace**

In `pipeline_e2e_test.go`, replace the `e2eConfig` struct and `setupE2EWorkspace`:

```go
// e2eConfig holds per-test pipeline configuration for E2E tests.
type e2eConfig struct {
	effort              string // state.EffortM, state.EffortS, state.EffortL
	template            string // state.TemplateStandard, TemplateLight, TemplateFull
	reviewDesignVerdict string // verdict written to review-design.md on first phase-3b spawn; defaults to "APPROVE" if empty
	autoApprove         bool   // when true, checkpoints are skipped automatically
	skipPR              bool   // when true, pr-creation phase is skipped
	onAction            func(t *testing.T, action orchestrator.Action, s *state.State) // optional per-action callback
}

// setupE2EWorkspace initialises a workspace with the given config and returns
// handler closures for pipeline_next_action and pipeline_report_result.
func setupE2EWorkspace(
	t *testing.T,
	cfg e2eConfig,
) (workspace string, nextActionH server.ToolHandlerFunc, reportResultH server.ToolHandlerFunc) {
	t.Helper()

	dir := t.TempDir()

	sm := state.NewStateManager("dev")
	if err := sm.Init(dir, "e2e-test"); err != nil {
		t.Fatalf("sm.Init: %v", err)
	}
	if err := sm.Configure(dir, state.PipelineConfig{
		Effort:        cfg.effort,
		FlowTemplate:  cfg.template,
		AutoApprove:   cfg.autoApprove,
		SkipPR:        cfg.skipPR,
		SkippedPhases: orchestrator.SkipsForTemplate(cfg.template),
	}); err != nil {
		t.Fatalf("sm.Configure: %v", err)
	}
	if err := sm.Update(func(s *state.State) error {
		s.BranchClassified = true
		return nil
	}); err != nil {
		t.Fatalf("sm.Update (BranchClassified): %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, state.ArtifactRequest),
		[]byte("# Request\n\ntest task\n"),
		0o600,
	); err != nil {
		t.Fatalf("write request.md: %v", err)
	}

	eng := orchestrator.NewEngine("", "")
	kb := history.NewKnowledgeBase("")
	nextActionH = PipelineNextActionHandler(sm, events.NewEventBus(), eng, "", nil, kb, nil)
	reportResultH = PipelineReportResultHandler(sm, events.NewEventBus(), kb)

	return dir, nextActionH, reportResultH
}
```

- [ ] **Step 3: Update all existing test call sites**

Add `autoApprove: true, skipPR: true` to every `e2eConfig{}` literal in existing tests:

In `TestE2E_Templates` (line ~254):
```go
cfg := e2eConfig{effort: tc.effort, template: tc.template, autoApprove: true, skipPR: true}
```

In `TestE2E_DesignRevisionCycle` (line ~280-284):
```go
cfg := e2eConfig{
    effort:              state.EffortM,
    template:            state.TemplateStandard,
    reviewDesignVerdict: "REVISE",
    autoApprove:         true,
    skipPR:              true,
}
```

In `TestE2E_ActionSequenceComplete` (line ~496):
```go
cfg := e2eConfig{effort: tc.effort, template: tc.template, autoApprove: true, skipPR: true}
```

In `TestE2E_SkippedPhasesInPhaseLog` (line ~549):
```go
cfg := e2eConfig{effort: state.EffortS, template: state.TemplateLight, autoApprove: true, skipPR: true}
```

- [ ] **Step 4: Run tests**

```bash
cd mcp-server && go test ./internal/handler/tools/ -run TestE2E -count=1 -v
```

Expected: all existing E2E tests pass with the new config fields.

- [ ] **Step 5: Commit**

```bash
git add mcp-server/internal/handler/tools/pipeline_next_action.go mcp-server/internal/handler/tools/pipeline_e2e_test.go
git commit -m "feat(pipeline): add PhaseLog for final-commit, extend e2eConfig with autoApprove/skipPR"
```

---

### Task 3: Unify CheckpointRevisionFlow setup, add CheckpointPhaseLog test

**Files:**
- Modify: `mcp-server/internal/handler/tools/pipeline_e2e_test.go:304-474` (TestE2E_CheckpointRevisionFlow)

- [ ] **Step 1: Rewrite TestE2E_CheckpointRevisionFlow to use setupE2EWorkspace**

Replace the custom workspace setup (lines 310-349) with `setupE2EWorkspace`:

```go
func TestE2E_CheckpointRevisionFlow(t *testing.T) {
	t.Parallel()

	cfg := e2eConfig{
		effort:      state.EffortM,
		template:    state.TemplateStandard,
		autoApprove: false,
		skipPR:      true,
	}
	workspace, nextActionH, reportResultH := setupE2EWorkspace(t, cfg)

	// Track how many times checkpoint-a returned an ActionCheckpoint.
	checkpointACount := 0
	// Track observed phases to verify the revision cycle occurred.
	var phaseSequence []string
	// pendingCheckpoint is set when a checkpoint action is returned; on the next
	// iteration the test sends user_response instead of calling reportResult.
	pendingCheckpoint := ""

	for range 60 {
		var result *mcp.CallToolResult
		var err error

		// If a checkpoint is pending from the previous iteration, respond to it
		// via user_response instead of doing a normal callNextAction.
		switch {
		case pendingCheckpoint == state.PhaseCheckpointA:
			checkpointACount++
			if checkpointACount == 1 {
				// First time at checkpoint-a: respond "revise" to trigger rewind.
				result, err = callNextActionWithUserResponse(t, nextActionH, workspace, "revise")
			} else {
				// Second time at checkpoint-a: respond "proceed" to advance.
				result, err = callNextActionWithUserResponse(t, nextActionH, workspace, "proceed")
			}
			pendingCheckpoint = ""
		case pendingCheckpoint != "":
			// For other checkpoints (checkpoint-b), just proceed.
			result, err = callNextActionWithUserResponse(t, nextActionH, workspace, "proceed")
			pendingCheckpoint = ""
		default:
			result, err = callNextAction(t, nextActionH, workspace)
		}

		if err != nil {
			t.Fatalf("callNextAction returned Go error: %v", err)
		}
		if result.IsError {
			t.Fatalf("callNextAction returned MCP error: %s", textContent(result))
		}

		var resp nextActionResponse
		if err := json.Unmarshal([]byte(textContent(result)), &resp); err != nil {
			t.Fatalf("unmarshal nextActionResponse: %v (raw: %s)", err, textContent(result))
		}

		if resp.Action.Type == orchestrator.ActionDone {
			break
		}

		reportPhase := resp.Action.Phase
		if resp.Action.Type == orchestrator.ActionCheckpoint && reportPhase == "" {
			reportPhase = resp.Action.Name
		}
		phaseSequence = append(phaseSequence, reportPhase)

		// If this is a checkpoint action, set pendingCheckpoint and skip reportResult.
		if resp.Action.Type == orchestrator.ActionCheckpoint {
			pendingCheckpoint = reportPhase
			continue
		}

		switch resp.Action.Type {
		case orchestrator.ActionWriteFile:
			if err := os.WriteFile(resp.Action.Path, []byte(resp.Action.Content), 0o600); err != nil {
				t.Fatalf("write_file %s: %v", resp.Action.Path, err)
			}
		case orchestrator.ActionSpawnAgent:
			alwaysApprove := new(bool)
			*alwaysApprove = true
			mockAgentExecute(t, workspace, resp.Action, cfg, alwaysApprove)
		case orchestrator.ActionExec:
			// No mock artifact write needed.
		default:
			t.Fatalf("unhandled action type %q for phase %q", resp.Action.Type, resp.Action.Phase)
		}

		reportRes := callTool(t, reportResultH, map[string]any{
			"workspace":   workspace,
			"phase":       reportPhase,
			"tokens_used": 500,
			"duration_ms": 1000,
			"model":       "sonnet",
		})
		if reportRes.IsError {
			t.Fatalf("callReportResult for phase %q returned MCP error: %s",
				reportPhase, textContent(reportRes))
		}
	}

	// Verify checkpoint-a was reached exactly twice (once before revise, once after).
	if checkpointACount != 2 {
		t.Errorf("checkpoint-a was reached %d times, want 2", checkpointACount)
	}

	// Verify the phase sequence shows phase-3 appearing at least twice.
	phase3Count := 0
	for _, p := range phaseSequence {
		if p == state.PhaseThree {
			phase3Count++
		}
	}
	if phase3Count < 2 {
		t.Errorf("phase-3 appeared %d times in sequence, want >= 2; sequence: %v",
			phase3Count, phaseSequence)
	}

	// Verify pipeline completed successfully.
	finalState, err := state.ReadState(workspace)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if finalState.CurrentPhase != state.PhaseCompleted {
		t.Errorf("currentPhase = %q, want %q", finalState.CurrentPhase, state.PhaseCompleted)
	}
}
```

- [ ] **Step 2: Add TestE2E_CheckpointPhaseLog test**

Add the `TestE2E_CheckpointPhaseLog` test from Task 1, Step 1 (it will now compile since `e2eConfig` has the required fields).

- [ ] **Step 3: Remove excludedFromLog from TestE2E_SkippedPhasesInPhaseLog**

Replace the `excludedFromLog` block (lines 579-598) with a direct check:

```go
	// Verify ALL phases from phase-1 to final-commit have a PhaseLog entry.
	// setup and completed don't get entries.
	for _, phase := range orchestrator.AllPhases {
		if phase == state.PhaseSetup || phase == state.PhaseCompleted {
			continue
		}
		if !allLogged[phase] {
			t.Errorf("phase %q has no PhaseLog entry — action may have been silently consumed", phase)
		}
	}
```

- [ ] **Step 4: Run tests**

```bash
cd mcp-server && go test ./internal/handler/tools/ -run TestE2E -count=1 -v
```

Expected: all tests pass. `TestE2E_SkippedPhasesInPhaseLog` now verifies ALL phases without exclusions. `TestE2E_CheckpointPhaseLog` verifies checkpoint-a and checkpoint-b appear in PhaseLog.

If `TestE2E_SkippedPhasesInPhaseLog` fails for `pr-creation` or `post-to-source`, that's expected — those PhaseLog entries are added in Task 4.

- [ ] **Step 5: Commit**

```bash
git add mcp-server/internal/handler/tools/pipeline_e2e_test.go
git commit -m "test(e2e): unify CheckpointRevisionFlow setup, remove excludedFromLog"
```

---

### Task 4: Add PhaseLog for pr-creation and post-to-source skips

**Files:**
- Modify: `mcp-server/internal/handler/tools/pipeline_next_action.go` (skip absorption loop)

The skip loop (P1, line ~274) already handles `PhaseLog` for skipped phases via `orchestrator.SkipSummaryPrefix`. The `pr-creation` and `post-to-source` phases are skipped through the same mechanism when they appear in `SkippedPhases`. Verify this is the case:

- [ ] **Step 1: Verify skip mechanism covers pr-creation and post-to-source**

Check if these phases go through the P1 skip loop when `SkipPR=true` or when source type is text. Run the existing test with verbose logging:

```bash
cd mcp-server && go test ./internal/handler/tools/ -run TestE2E_SkippedPhasesInPhaseLog -count=1 -v
```

If the test passes (from Task 3's `excludedFromLog` removal), the phases are already logged via the P1 skip loop. If it fails for `pr-creation` or `post-to-source`, add explicit `PhaseLog` calls.

- [ ] **Step 2: If needed — add PhaseLog for pr-creation skip**

If `pr-creation` is not going through the P1 skip loop, find where it's consumed and add:

```go
_ = sm2.PhaseLog(workspace, state.PhasePRCreation, 0, 0, "skipped")
```

- [ ] **Step 3: If needed — add PhaseLog for post-to-source skip**

Same approach for `post-to-source`.

- [ ] **Step 4: Run full E2E test suite**

```bash
cd mcp-server && go test ./internal/handler/tools/ -run TestE2E -count=1 -v
```

Expected: ALL tests pass, including `TestE2E_SkippedPhasesInPhaseLog` with zero exclusions.

- [ ] **Step 5: Commit**

```bash
git add mcp-server/internal/handler/tools/pipeline_next_action.go
git commit -m "feat(pipeline): ensure pr-creation and post-to-source skips are logged"
```

---

### Task 5: Add intermediate state verification

**Files:**
- Modify: `mcp-server/internal/handler/tools/pipeline_e2e_test.go` (runE2EPipeline + new test)

- [ ] **Step 1: Add onAction callback to runE2EPipeline**

After the `reportResult` call in `runE2EPipeline` (line ~200), add:

```go
		// Invoke optional per-action state verification callback.
		if cfg.onAction != nil {
			s, sErr := state.ReadState(workspace)
			if sErr != nil {
				t.Fatalf("runE2EPipeline: ReadState after %s: %v", resp.Action.Phase, sErr)
			}
			cfg.onAction(t, resp.Action, s)
		}
```

- [ ] **Step 2: Add TestE2E_StateTransitions test**

```go
func TestE2E_StateTransitions(t *testing.T) {
	t.Parallel()

	var prevPhase string
	var phaseOrder []string

	cfg := e2eConfig{
		effort:      state.EffortM,
		template:    state.TemplateStandard,
		autoApprove: true,
		skipPR:      true,
		onAction: func(t *testing.T, action orchestrator.Action, s *state.State) {
			t.Helper()
			phase := s.CurrentPhase

			// Track phase progression — each new phase should differ from the previous
			// (except during revision cycles where phase-3/3b may repeat).
			if phase != prevPhase && phase != "" {
				phaseOrder = append(phaseOrder, phase)
				prevPhase = phase
			}

			// Verify phase status is never empty during active execution.
			if s.CurrentPhaseStatus == "" && phase != state.PhaseCompleted {
				t.Errorf("currentPhaseStatus is empty during phase %q", phase)
			}
		},
	}
	workspace, nextActionH, reportResultH := setupE2EWorkspace(t, cfg)
	_, _ = runE2EPipeline(t, cfg, workspace, nextActionH, reportResultH)

	// Verify at least 5 distinct phases were observed (even light template has more).
	if len(phaseOrder) < 5 {
		t.Errorf("only %d distinct phases observed, want >= 5; phases: %v", len(phaseOrder), phaseOrder)
	}
}
```

- [ ] **Step 3: Run tests**

```bash
cd mcp-server && go test ./internal/handler/tools/ -run TestE2E -count=1 -v
```

Expected: all tests pass, including the new `TestE2E_StateTransitions`.

- [ ] **Step 4: Commit**

```bash
git add mcp-server/internal/handler/tools/pipeline_e2e_test.go
git commit -m "test(e2e): add intermediate state transition verification via onAction callback"
```

---

### Task 6: Add moduleRoot() helper and replace fragile paths

**Files:**
- Create: `mcp-server/internal/handler/tools/test_helpers_test.go`
- Modify: `mcp-server/internal/handler/tools/ast_summary_test.go:14-18`
- Modify: `mcp-server/internal/handler/tools/ast_find_definition_test.go:17,40,107,127`
- Modify: `mcp-server/internal/engine/state/manager_test.go:1184`
- Modify: `mcp-server/internal/intelligence/profile/analyzer_test.go:146-148`

- [ ] **Step 1: Create test_helpers_test.go with moduleRoot()**

```go
package tools

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// moduleRoot returns the absolute path to the mcp-server/ module root
// by walking up from the current file until go.mod is found.
// This is stable across directory restructuring — unlike relative paths
// that break when package nesting depth changes.
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found walking up from " + file)
		}
		dir = parent
	}
}
```

- [ ] **Step 2: Replace paths in ast_summary_test.go**

Replace the `astTestdataDir` function:

```go
// astTestdataDir returns the absolute path to pkg/ast/testdata/.
func astTestdataDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(moduleRoot(t), "pkg", "ast", "testdata")
}
```

Update the call site in `TestAstSummaryFromPath_Go` (line 21):
```go
path := filepath.Join(astTestdataDir(t), "sample.go")
```

And any other calls to `astTestdataDir()` — add `t` as an argument.

- [ ] **Step 3: Replace paths in ast_find_definition_test.go**

Replace all 4 occurrences of `"../../../pkg/ast/testdata/sample.go"` with:
```go
filepath.Join(moduleRoot(t), "pkg", "ast", "testdata", "sample.go"),
```

This requires adding `"path/filepath"` and `"os"` and `"runtime"` imports if not already present. Since `moduleRoot` is in the same package (`package tools`), it's available directly.

- [ ] **Step 4: Replace path in engine/state/manager_test.go**

The `manager_test.go` is in `package state_test`. It needs its own copy of `moduleRoot` since it's a different package.

Add a helper at the top of `manager_test.go` (or in a new `mcp-server/internal/engine/state/test_helpers_test.go`):

```go
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
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

Replace line 1184:
```go
goldenPath := filepath.Join(moduleRoot(t), "testdata", "state_init.json")
```

- [ ] **Step 5: Replace path in intelligence/profile/analyzer_test.go**

Same pattern — add `moduleRoot` helper (the package is `package profile`, different from the others), then replace lines 146-148:

```go
	// Repo root is the module root's parent (mcp-server → claude-forge).
	repoRoot := filepath.Dir(moduleRoot(t))
```

- [ ] **Step 6: Run all tests**

```bash
cd mcp-server && go test ./... -count=1
```

Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add mcp-server/internal/handler/tools/test_helpers_test.go \
  mcp-server/internal/handler/tools/ast_summary_test.go \
  mcp-server/internal/handler/tools/ast_find_definition_test.go \
  mcp-server/internal/engine/state/manager_test.go \
  mcp-server/internal/intelligence/profile/analyzer_test.go
git commit -m "test: replace fragile relative paths with moduleRoot() helper"
```

---

### Task 7: Final verification

- [ ] **Step 1: Run full test suite with race detector**

```bash
cd mcp-server && go test -race ./... -count=1
```

Expected: all tests pass with no race conditions.

- [ ] **Step 2: Run lint**

```bash
cd mcp-server && make go-lint-fast
```

Expected: no lint errors.

- [ ] **Step 3: Verify test count**

```bash
cd mcp-server && go test ./internal/handler/tools/ -v -count=1 2>&1 | grep -c '--- PASS'
```

Expected: count should be 2 higher than before (TestE2E_CheckpointPhaseLog + TestE2E_StateTransitions).

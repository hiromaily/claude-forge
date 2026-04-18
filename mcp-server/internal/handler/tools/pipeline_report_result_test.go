// Package tools — unit tests for pipeline_report_result MCP handler.
// Tests verify phase-log recording, artifact validation, verdict routing, and state transitions.
package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/state"
	"github.com/hiromaily/claude-forge/mcp-server/internal/intelligence/history"
	"github.com/hiromaily/claude-forge/mcp-server/pkg/events"
)

// ---------- helpers ----------

// initPRRWorkspace creates a temp workspace with an initialised state.json.
func initPRRWorkspace(t *testing.T, sm *state.StateManager) string {
	t.Helper()
	dir := t.TempDir()
	if err := sm.Init(dir, "test-spec"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return dir
}

// parsePRRResponse unmarshals reportResultOutcome from a content string.
func parsePRRResponse(t *testing.T, content string) reportResultOutcome {
	t.Helper()
	var resp reportResultOutcome
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		t.Fatalf("unmarshal reportResultOutcome: %v (content: %s)", err, content)
	}
	return resp
}

// ---------- TestPipelineReportResult ----------

func TestPipelineReportResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		phase           string
		setup           func(t *testing.T, sm *state.StateManager, dir string)
		tokensUsed      int
		durationMs      int
		model           string
		setupOnly       bool // pass setup_only=true to pipeline_report_result
		wantIsError     bool
		wantStateUpdate bool
		wantHint        string
		wantVerdict     string
		wantWarningNE   bool // want non-empty warning
		checkState      func(t *testing.T, dir string)
	}{
		{
			name:  "phase_log_recorded",
			phase: "phase-1",
			setup: func(t *testing.T, sm *state.StateManager, dir string) {
				t.Helper()
				// Write analysis.md so artifact validation passes for phase-1 related phases.
				// phase-1 has no artifact validation rule, so nothing needed.
			},
			tokensUsed:      1234,
			durationMs:      5678,
			model:           "sonnet",
			wantIsError:     false,
			wantStateUpdate: true,
			wantHint:        "proceed",
			checkState: func(t *testing.T, dir string) {
				t.Helper()
				s, err := state.ReadState(dir)
				if err != nil {
					t.Fatalf("ReadState: %v", err)
				}
				if len(s.PhaseLog) != 1 {
					t.Fatalf("PhaseLog len = %d, want 1", len(s.PhaseLog))
				}
				entry := s.PhaseLog[0]
				if entry.Phase != "phase-1" {
					t.Errorf("PhaseLog[0].Phase = %q, want %q", entry.Phase, "phase-1")
				}
				if entry.Tokens != 1234 {
					t.Errorf("PhaseLog[0].Tokens = %d, want 1234", entry.Tokens)
				}
				if entry.DurationMs != 5678 {
					t.Errorf("PhaseLog[0].DurationMs = %d, want 5678", entry.DurationMs)
				}
				if entry.Model != "sonnet" {
					t.Errorf("PhaseLog[0].Model = %q, want %q", entry.Model, "sonnet")
				}
			},
		},
		{
			name:  "unknown_phase_skip",
			phase: "post-to-source",
			setup: func(t *testing.T, sm *state.StateManager, dir string) {
				t.Helper()
			},
			wantIsError:     false,
			wantStateUpdate: true,
			wantWarningNE:   true, // warning about unknown phase artifact
			wantHint:        "proceed",
		},
		{
			name:  "artifact_invalid",
			phase: "phase-3",
			setup: func(t *testing.T, sm *state.StateManager, dir string) {
				t.Helper()
				// design.md absent — should trigger error
			},
			wantIsError: true,
		},
		{
			name:  "verdict_approve",
			phase: "phase-3b",
			setup: func(t *testing.T, sm *state.StateManager, dir string) {
				t.Helper()
				// Write review-design.md with APPROVE verdict.
				content := "## Review\n\n## Verdict: APPROVE\n\nLooks good.\n"
				if err := os.WriteFile(filepath.Join(dir, "review-design.md"), []byte(content), 0o644); err != nil {
					t.Fatalf("WriteFile review-design.md: %v", err)
				}
			},
			wantIsError:     false,
			wantStateUpdate: true,
			wantHint:        "proceed",
			wantVerdict:     "APPROVE",
			checkState: func(t *testing.T, dir string) {
				t.Helper()
				s, err := state.ReadState(dir)
				if err != nil {
					t.Fatalf("ReadState: %v", err)
				}
				// Phase should have advanced (phase-3b completed).
				if !slices.Contains(s.CompletedPhases, "phase-3b") {
					t.Errorf("phase-3b not in CompletedPhases after APPROVE; completed = %v", s.CompletedPhases)
				}
			},
		},
		{
			name:  "verdict_revise_design",
			phase: "phase-3b",
			setup: func(t *testing.T, sm *state.StateManager, dir string) {
				t.Helper()
				// Write review-design.md with REVISE verdict.
				content := "## Review\n\n## Verdict: REVISE\n\nNeeds work.\n"
				if err := os.WriteFile(filepath.Join(dir, "review-design.md"), []byte(content), 0o644); err != nil {
					t.Fatalf("WriteFile review-design.md: %v", err)
				}
			},
			wantIsError:     false,
			wantStateUpdate: true,
			wantHint:        "revision_required",
			wantVerdict:     "REVISE",
			checkState: func(t *testing.T, dir string) {
				t.Helper()
				s, err := state.ReadState(dir)
				if err != nil {
					t.Fatalf("ReadState: %v", err)
				}
				// DesignRevisions should have been incremented.
				if s.Revisions.DesignRevisions != 1 {
					t.Errorf("DesignRevisions = %d, want 1", s.Revisions.DesignRevisions)
				}
				// Phase should NOT have advanced (RevisionBump doesn't complete the phase).
				for _, p := range s.CompletedPhases {
					if p == "phase-3b" {
						t.Errorf("phase-3b must NOT be in CompletedPhases after REVISE")
					}
				}
			},
		},
		{
			name:  "verdict_revise_tasks",
			phase: "phase-4b",
			setup: func(t *testing.T, sm *state.StateManager, dir string) {
				t.Helper()
				// Write review-tasks.md with REVISE verdict.
				content := "## Review\n\n## Verdict: REVISE\n\nTasks need more detail.\n"
				if err := os.WriteFile(filepath.Join(dir, "review-tasks.md"), []byte(content), 0o644); err != nil {
					t.Fatalf("WriteFile review-tasks.md: %v", err)
				}
			},
			wantIsError:     false,
			wantStateUpdate: true,
			wantHint:        "revision_required",
			wantVerdict:     "REVISE",
			checkState: func(t *testing.T, dir string) {
				t.Helper()
				s, err := state.ReadState(dir)
				if err != nil {
					t.Fatalf("ReadState: %v", err)
				}
				if s.Revisions.TaskRevisions != 1 {
					t.Errorf("TaskRevisions = %d, want 1", s.Revisions.TaskRevisions)
				}
			},
		},
		{
			name:  "phase6_fail",
			phase: "phase-6",
			setup: func(t *testing.T, sm *state.StateManager, dir string) {
				t.Helper()
				// Initialize task so ReviewStatus and ImplRetries can be verified.
				tasks := map[string]state.Task{
					"1": {Title: "Task 1", ImplStatus: state.TaskStatusCompleted},
				}
				if err := sm.TaskInit(dir, tasks); err != nil {
					t.Fatalf("TaskInit: %v", err)
				}
				// Write review-1.md with FAIL verdict.
				content := "## Summary\n\n## Verdict: FAIL\n\nTests did not pass.\n"
				if err := os.WriteFile(filepath.Join(dir, "review-1.md"), []byte(content), 0o644); err != nil {
					t.Fatalf("WriteFile review-1.md: %v", err)
				}
			},
			wantIsError:     false,
			wantStateUpdate: true,
			wantHint:        "retry_impl",
			wantVerdict:     "FAIL",
			checkState: func(t *testing.T, dir string) {
				t.Helper()
				s, err := state.ReadState(dir)
				if err != nil {
					t.Fatalf("ReadState: %v", err)
				}
				// Phase should NOT have advanced.
				for _, p := range s.CompletedPhases {
					if p == "phase-6" {
						t.Errorf("phase-6 must NOT be in CompletedPhases after FAIL")
					}
				}
				// pipeline_report_result must record FAIL in state for deterministic retry.
				task := s.Tasks["1"]
				if task.ReviewStatus != state.TaskStatusCompletedFail {
					t.Errorf("Tasks[1].ReviewStatus = %q, want %q", task.ReviewStatus, state.TaskStatusCompletedFail)
				}
				if task.ImplRetries != 1 {
					t.Errorf("Tasks[1].ImplRetries = %d, want 1", task.ImplRetries)
				}
			},
		},
		{
			name:  "phase6_pass",
			phase: "phase-6",
			setup: func(t *testing.T, sm *state.StateManager, dir string) {
				t.Helper()
				// Write impl-1.md with PASS verdict.
				content := "## Summary\n\n## Verdict: PASS\n\nAll tests passed.\n"
				if err := os.WriteFile(filepath.Join(dir, "impl-1.md"), []byte(content), 0o644); err != nil {
					t.Fatalf("WriteFile impl-1.md: %v", err)
				}
			},
			wantIsError:     false,
			wantStateUpdate: true,
			wantHint:        "proceed",
			wantVerdict:     "PASS",
			checkState: func(t *testing.T, dir string) {
				t.Helper()
				s, err := state.ReadState(dir)
				if err != nil {
					t.Fatalf("ReadState: %v", err)
				}
				if !slices.Contains(s.CompletedPhases, "phase-6") {
					t.Errorf("phase-6 not in CompletedPhases after PASS; completed = %v", s.CompletedPhases)
				}
			},
		},
		{
			name:  "phase6_multi_task_pending_review",
			phase: "phase-6",
			setup: func(t *testing.T, sm *state.StateManager, dir string) {
				t.Helper()
				// Set up 2 tasks, both implemented. Task 1 reviewed (PASS), task 2 not yet.
				tasks := map[string]state.Task{
					"1": {ImplStatus: state.TaskStatusCompleted},
					"2": {ImplStatus: state.TaskStatusCompleted},
				}
				if err := sm.TaskInit(dir, tasks); err != nil {
					t.Fatalf("TaskInit: %v", err)
				}
				content := "## Summary\n\n## Verdict: PASS\n\nAll tests passed.\n"
				if err := os.WriteFile(filepath.Join(dir, "review-1.md"), []byte(content), 0o644); err != nil {
					t.Fatalf("WriteFile review-1.md: %v", err)
				}
			},
			wantIsError:     false,
			wantStateUpdate: true,
			wantHint:        "setup_continue",
			checkState: func(t *testing.T, dir string) {
				t.Helper()
				s, err := state.ReadState(dir)
				if err != nil {
					t.Fatalf("ReadState: %v", err)
				}
				// phase-6 must NOT advance while task 2 still needs review.
				for _, p := range s.CompletedPhases {
					if p == "phase-6" {
						t.Error("phase-6 must NOT be in CompletedPhases while task 2 is unreviewed")
					}
				}
			},
		},
		{
			name:  "phase6_structurally_valid_but_fail",
			phase: "phase-6",
			setup: func(t *testing.T, sm *state.StateManager, dir string) {
				t.Helper()
				// Write impl-1.md that is structurally valid (has verdict set) but verdict is FAIL.
				// This confirms ParseVerdict is used, not ArtifactResult.Valid.
				content := "## Acceptance Criteria\n- [x] AC-1: done\n\n## Verdict: FAIL\n\nFound regressions.\n"
				if err := os.WriteFile(filepath.Join(dir, "impl-1.md"), []byte(content), 0o644); err != nil {
					t.Fatalf("WriteFile impl-1.md: %v", err)
				}
			},
			wantIsError:     false,
			wantStateUpdate: true,
			wantHint:        "retry_impl",
			wantVerdict:     "FAIL",
		},
		{
			name:  "setup_only_skips_phase_complete",
			phase: "phase-5",
			setup: func(t *testing.T, sm *state.StateManager, dir string) {
				t.Helper()
			},
			setupOnly:       true,
			wantIsError:     false,
			wantStateUpdate: true,
			wantHint:        "setup_continue",
			checkState: func(t *testing.T, dir string) {
				t.Helper()
				s, err := state.ReadState(dir)
				if err != nil {
					t.Fatalf("ReadState: %v", err)
				}
				// Phase must NOT have advanced — phase-5 should NOT be in CompletedPhases.
				for _, p := range s.CompletedPhases {
					if p == "phase-5" {
						t.Errorf("phase-5 must NOT be in CompletedPhases when setup_only=true")
					}
				}
			},
		},
		{
			name:  "duplicate_phase_log_warning",
			phase: "phase-3",
			setup: func(t *testing.T, sm *state.StateManager, dir string) {
				t.Helper()
				// Pre-populate a phase-log entry for phase-3 to trigger duplicate warning.
				if err := sm.PhaseLog(dir, "phase-3", 100, 1000, "sonnet"); err != nil {
					t.Fatalf("pre-PhaseLog: %v", err)
				}
				// Write design.md so artifact validation passes.
				content := "## Overview\n\nContent here.\n"
				if err := os.WriteFile(filepath.Join(dir, "design.md"), []byte(content), 0o644); err != nil {
					t.Fatalf("WriteFile design.md: %v", err)
				}
			},
			wantIsError:     false,
			wantStateUpdate: true,
			wantWarningNE:   true, // duplicate warning expected
			wantHint:        "proceed",
		},
		{
			// After a retry implementer run (phase-5), tasks in ReviewStatus="completed_fail"
			// must be reset to "" and their stale review files deleted so the engine
			// dispatches a fresh reviewer instead of re-dispatching the implementer.
			name:  "phase5_clears_completed_fail_state",
			phase: "phase-5",
			setup: func(t *testing.T, sm *state.StateManager, dir string) {
				t.Helper()
				tasks := map[string]state.Task{
					"1": {
						Title:        "Task 1",
						ImplStatus:   state.TaskStatusCompleted,
						ReviewStatus: state.TaskStatusCompletedFail,
						ImplRetries:  1,
					},
				}
				if err := sm.TaskInit(dir, tasks); err != nil {
					t.Fatalf("TaskInit: %v", err)
				}
				// impl-1.md exists (retry implementer just ran).
				if err := os.WriteFile(filepath.Join(dir, "impl-1.md"), []byte("content"), 0o644); err != nil {
					t.Fatalf("WriteFile impl-1.md: %v", err)
				}
				// Stale review-1.md must be deleted after phase-5 completes.
				if err := os.WriteFile(filepath.Join(dir, "review-1.md"), []byte("## Verdict: FAIL\n"), 0o644); err != nil {
					t.Fatalf("WriteFile review-1.md: %v", err)
				}
			},
			wantIsError:     false,
			wantStateUpdate: true,
			wantHint:        "proceed",
			checkState: func(t *testing.T, dir string) {
				t.Helper()
				s, err := state.ReadState(dir)
				if err != nil {
					t.Fatalf("ReadState: %v", err)
				}
				task := s.Tasks["1"]
				if task.ReviewStatus != "" {
					t.Errorf("Tasks[1].ReviewStatus = %q after phase-5, want empty (cleared)", task.ReviewStatus)
				}
				if _, statErr := os.Stat(filepath.Join(dir, "review-1.md")); !os.IsNotExist(statErr) {
					t.Error("review-1.md should have been deleted by clearCompletedFailTasks")
				}
			},
		},
		{
			// Phase 5 completion gate: all tasks marked completed in state but
			// impl-2.md missing on disk — must block phase completion and reset
			// ImplStatus so the engine re-dispatches implementers.
			name:  "phase5_completion_gate_blocks_missing_impl",
			phase: "phase-5",
			setup: func(t *testing.T, sm *state.StateManager, dir string) {
				t.Helper()
				tasks := map[string]state.Task{
					"1": {Title: "Task 1", ImplStatus: state.TaskStatusCompleted},
					"2": {Title: "Task 2", ImplStatus: state.TaskStatusCompleted},
					"3": {Title: "Task 3", ImplStatus: state.TaskStatusCompleted},
				}
				if err := sm.TaskInit(dir, tasks); err != nil {
					t.Fatalf("TaskInit: %v", err)
				}
				// Only impl-1.md exists; impl-2.md and impl-3.md are missing.
				if err := os.WriteFile(filepath.Join(dir, "impl-1.md"), []byte("content"), 0o644); err != nil {
					t.Fatalf("WriteFile impl-1.md: %v", err)
				}
			},
			wantIsError:     false,
			wantStateUpdate: true,
			wantHint:        "setup_continue",
			wantWarningNE:   true, // warning about missing impl files
			checkState: func(t *testing.T, dir string) {
				t.Helper()
				s, err := state.ReadState(dir)
				if err != nil {
					t.Fatalf("ReadState: %v", err)
				}
				for _, p := range s.CompletedPhases {
					if p == "phase-5" {
						t.Error("phase-5 must NOT be in CompletedPhases when impl files are missing")
					}
				}
				// ImplStatus must be reset for tasks with missing impl files.
				if s.Tasks["2"].ImplStatus != "" {
					t.Errorf("Tasks[2].ImplStatus = %q, want empty (reset by gate)", s.Tasks["2"].ImplStatus)
				}
				if s.Tasks["3"].ImplStatus != "" {
					t.Errorf("Tasks[3].ImplStatus = %q, want empty (reset by gate)", s.Tasks["3"].ImplStatus)
				}
				// Task 1 has its impl file — ImplStatus must remain completed.
				if s.Tasks["1"].ImplStatus != state.TaskStatusCompleted {
					t.Errorf("Tasks[1].ImplStatus = %q, want %q (file exists)", s.Tasks["1"].ImplStatus, state.TaskStatusCompleted)
				}
			},
		},
		{
			// Phase 5 completion gate: human_gate tasks must be excluded from
			// the impl file check — they complete by user acknowledgement and
			// produce no impl file. Without this exclusion, human_gate tasks
			// would cause an infinite reset loop.
			name:  "phase5_completion_gate_skips_human_gate_tasks",
			phase: "phase-5",
			setup: func(t *testing.T, sm *state.StateManager, dir string) {
				t.Helper()
				tasks := map[string]state.Task{
					"1": {Title: "Task 1", ImplStatus: state.TaskStatusCompleted},
					"2": {Title: "Human task", ImplStatus: state.TaskStatusCompleted, ExecutionMode: state.ExecModeHumanGate, ReviewStatus: state.TaskStatusCompletedPass},
				}
				if err := sm.TaskInit(dir, tasks); err != nil {
					t.Fatalf("TaskInit: %v", err)
				}
				// Only task 1 has an impl file; task 2 is human_gate (no file expected).
				if err := os.WriteFile(filepath.Join(dir, "impl-1.md"), []byte("content"), 0o644); err != nil {
					t.Fatalf("WriteFile impl-1.md: %v", err)
				}
			},
			wantIsError:     false,
			wantStateUpdate: true,
			wantHint:        "proceed",
			checkState: func(t *testing.T, dir string) {
				t.Helper()
				s, err := state.ReadState(dir)
				if err != nil {
					t.Fatalf("ReadState: %v", err)
				}
				if !slices.Contains(s.CompletedPhases, "phase-5") {
					t.Errorf("phase-5 should be in CompletedPhases; human_gate task should not block; completed = %v", s.CompletedPhases)
				}
				// Human gate task's ImplStatus must remain completed (not reset).
				if s.Tasks["2"].ImplStatus != state.TaskStatusCompleted {
					t.Errorf("Tasks[2].ImplStatus = %q, want %q (human_gate must not be reset)", s.Tasks["2"].ImplStatus, state.TaskStatusCompleted)
				}
			},
		},
		{
			// Phase 5 completion gate: all impl files present — should advance.
			name:  "phase5_completion_gate_passes_all_impl_present",
			phase: "phase-5",
			setup: func(t *testing.T, sm *state.StateManager, dir string) {
				t.Helper()
				tasks := map[string]state.Task{
					"1": {Title: "Task 1", ImplStatus: state.TaskStatusCompleted},
					"2": {Title: "Task 2", ImplStatus: state.TaskStatusCompleted},
				}
				if err := sm.TaskInit(dir, tasks); err != nil {
					t.Fatalf("TaskInit: %v", err)
				}
				for _, k := range []string{"1", "2"} {
					if err := os.WriteFile(filepath.Join(dir, "impl-"+k+".md"), []byte("content"), 0o644); err != nil {
						t.Fatalf("WriteFile impl-%s.md: %v", k, err)
					}
				}
			},
			wantIsError:     false,
			wantStateUpdate: true,
			wantHint:        "proceed",
			checkState: func(t *testing.T, dir string) {
				t.Helper()
				s, err := state.ReadState(dir)
				if err != nil {
					t.Fatalf("ReadState: %v", err)
				}
				if !slices.Contains(s.CompletedPhases, "phase-5") {
					t.Errorf("phase-5 should be in CompletedPhases when all impl files exist; completed = %v", s.CompletedPhases)
				}
			},
		},
		{
			// Regression: stale-cache bug where PipelineReportResultHandler was
			// passed a global StateManager whose in-memory cache predated the
			// TaskInit call made inside PipelineNextActionHandler (P2 absorption).
			// With a stale cache of 0 tasks, the old code called sm.Update (via
			// PhaseLog) which overwrote disk with 0 tasks, making hasPending=false
			// and advancing phase-5 prematurely after only 1 task was implemented.
			// The fix creates a per-call sm2 inside PipelineReportResultHandler so
			// it always loads fresh state from disk, regardless of the global sm.
			name:  "phase5_stale_cache_does_not_advance_with_pending_tasks",
			phase: "phase-5",
			setup: func(t *testing.T, sm *state.StateManager, dir string) {
				t.Helper()
				// Simulate executeTaskInit: write 3 tasks via a *different* StateManager
				// (sm2), leaving the test's "global" sm with a stale empty cache.
				sm2 := state.NewStateManager("dev")
				if err := sm2.LoadFromFile(dir); err != nil {
					t.Fatalf("sm2.LoadFromFile: %v", err)
				}
				tasks := map[string]state.Task{
					"1": {Title: "Task 1"},
					"2": {Title: "Task 2"},
					"3": {Title: "Task 3"},
				}
				if err := sm2.TaskInit(dir, tasks); err != nil {
					t.Fatalf("sm2.TaskInit: %v", err)
				}
				// Only impl-1.md exists — tasks 2 and 3 are still pending.
				if err := os.WriteFile(filepath.Join(dir, "impl-1.md"), []byte("content"), 0o644); err != nil {
					t.Fatalf("WriteFile impl-1.md: %v", err)
				}
				// sm (the "global" handler sm) is NOT updated — its in-memory cache
				// still has 0 tasks, reproducing the stale-cache scenario.
			},
			wantIsError:     false,
			wantStateUpdate: true,
			wantHint:        "setup_continue", // must NOT advance to "proceed"
			checkState: func(t *testing.T, dir string) {
				t.Helper()
				s, err := state.ReadState(dir)
				if err != nil {
					t.Fatalf("ReadState: %v", err)
				}
				// Tasks must survive — the handler must not overwrite with 0 tasks.
				if len(s.Tasks) != 3 {
					t.Fatalf("Tasks count = %d after phase-5 report, want 3 (stale cache must not overwrite tasks)", len(s.Tasks))
				}
				// phase-5 must NOT be completed.
				for _, p := range s.CompletedPhases {
					if p == "phase-5" {
						t.Error("phase-5 must NOT be in CompletedPhases while tasks 2 and 3 are pending")
					}
				}
			},
		},
		{
			// Phase 6 completion gate: all tasks reviewed (PASS) but review-2.md
			// missing on disk — must block phase completion and reset ReviewStatus
			// so the engine re-dispatches reviewers.
			name:  "phase6_completion_gate_blocks_missing_review",
			phase: "phase-6",
			setup: func(t *testing.T, sm *state.StateManager, dir string) {
				t.Helper()
				tasks := map[string]state.Task{
					"1": {ImplStatus: state.TaskStatusCompleted, ReviewStatus: state.TaskStatusCompletedPass},
					"2": {ImplStatus: state.TaskStatusCompleted, ReviewStatus: state.TaskStatusCompletedPass},
				}
				if err := sm.TaskInit(dir, tasks); err != nil {
					t.Fatalf("TaskInit: %v", err)
				}
				// Only review-1.md exists; review-2.md is missing.
				content := "## Summary\n\n## Verdict: PASS\n\nAll good.\n"
				if err := os.WriteFile(filepath.Join(dir, "review-1.md"), []byte(content), 0o644); err != nil {
					t.Fatalf("WriteFile review-1.md: %v", err)
				}
			},
			wantIsError:     false,
			wantStateUpdate: true,
			wantHint:        "setup_continue",
			wantWarningNE:   true, // warning about missing review files
			checkState: func(t *testing.T, dir string) {
				t.Helper()
				s, err := state.ReadState(dir)
				if err != nil {
					t.Fatalf("ReadState: %v", err)
				}
				for _, p := range s.CompletedPhases {
					if p == "phase-6" {
						t.Error("phase-6 must NOT be in CompletedPhases when review files are missing")
					}
				}
				// ReviewStatus must be reset for tasks with missing review files.
				if s.Tasks["2"].ReviewStatus != "" {
					t.Errorf("Tasks[2].ReviewStatus = %q, want empty (reset by gate)", s.Tasks["2"].ReviewStatus)
				}
				// Task 1 has its review file — ReviewStatus must remain.
				if s.Tasks["1"].ReviewStatus != state.TaskStatusCompletedPass {
					t.Errorf("Tasks[1].ReviewStatus = %q, want %q (file exists)", s.Tasks["1"].ReviewStatus, state.TaskStatusCompletedPass)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sm := state.NewStateManager("dev")
			dir := initPRRWorkspace(t, sm)

			if tc.setup != nil {
				tc.setup(t, sm, dir)
			}

			h := PipelineReportResultHandler(sm, events.NewEventBus(), history.NewKnowledgeBase(""))
			params := map[string]any{
				"workspace":   dir,
				"phase":       tc.phase,
				"tokens_used": tc.tokensUsed,
				"duration_ms": tc.durationMs,
				"model":       tc.model,
			}
			if tc.setupOnly {
				params["setup_only"] = true
			}
			res := callTool(t, h, params)

			if tc.wantIsError {
				if !res.IsError {
					t.Errorf("test %q: expected MCP error, got success: %s", tc.name, textContent(res))
				}
				return
			}

			if res.IsError {
				t.Fatalf("test %q: unexpected MCP error: %s", tc.name, textContent(res))
			}

			resp := parsePRRResponse(t, textContent(res))

			if resp.StateUpdated != tc.wantStateUpdate {
				t.Errorf("test %q: StateUpdated = %v, want %v", tc.name, resp.StateUpdated, tc.wantStateUpdate)
			}

			if tc.wantHint != "" && resp.NextActionHint != tc.wantHint {
				t.Errorf("test %q: NextActionHint = %q, want %q", tc.name, resp.NextActionHint, tc.wantHint)
			}

			if tc.wantVerdict != "" && resp.VerdictParsed != tc.wantVerdict {
				t.Errorf("test %q: VerdictParsed = %q, want %q", tc.name, resp.VerdictParsed, tc.wantVerdict)
			}

			if tc.wantWarningNE && resp.Warning == "" {
				t.Errorf("test %q: expected non-empty warning, got empty", tc.name)
			}

			if tc.checkState != nil {
				tc.checkState(t, dir)
			}
		})
	}
}

// ---------- Phase-4 workflow rules integration tests ----------

// setupPhase4SpecWorkspace creates a tmpRoot/.specs/<spec>/ layout and
// initialises state for a phase-4 workflow-rules integration test.
// Returns (tmpRoot, workspace, StateManager).
func setupPhase4SpecWorkspace(t *testing.T, specName string) (string, string, *state.StateManager) {
	t.Helper()
	tmpRoot := t.TempDir()
	workspace := filepath.Join(tmpRoot, ".specs", specName)
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	sm := state.NewStateManager("dev")
	if err := sm.Init(workspace, specName); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return tmpRoot, workspace, sm
}

// callPhase4Report invokes PipelineReportResultHandler on phase-4 with a
// minimal set of parameters and returns the parsed response. Callers must
// write tasks.md (and any instructions.md) before calling this helper.
func callPhase4Report(t *testing.T, sm *state.StateManager, workspace string) reportResultOutcome {
	t.Helper()
	h := PipelineReportResultHandler(sm, events.NewEventBus(), history.NewKnowledgeBase(""))
	res := callTool(t, h, map[string]any{
		"workspace":   workspace,
		"phase":       "phase-4",
		"tokens_used": 100,
		"duration_ms": 500,
		"model":       "sonnet",
	})
	if res.IsError {
		t.Fatalf("unexpected MCP error: %s", textContent(res))
	}
	return parsePRRResponse(t, textContent(res))
}

// writeProtoRuleViolationFixture writes a canonical .specs/instructions.md
// with a single "proto-rule" (human_gate required for proto files) and a
// tasks.md that violates it (a sequential proto task). Shared by the phase-4
// workflow-rules tests so a rule-shape change only needs one update.
func writeProtoRuleViolationFixture(t *testing.T, tmpRoot, workspace string) {
	t.Helper()
	instructionsPath := filepath.Join(tmpRoot, ".specs", "instructions.md")
	instructionsBody := `---
rules:
  - id: proto-rule
    when:
      files_match: ["**/*.proto"]
    require: human_gate
    reason: "coordinate with proto repo"
---
`
	if err := os.WriteFile(instructionsPath, []byte(instructionsBody), 0o644); err != nil {
		t.Fatalf("write instructions: %v", err)
	}
	tasksBody := `# Tasks

## Task 1: Update deal proto

Add a new field to the deal proto.

mode: sequential
files:
- backend/pkg/api/deal.proto
`
	if err := os.WriteFile(filepath.Join(workspace, "tasks.md"), []byte(tasksBody), 0o644); err != nil {
		t.Fatalf("write tasks.md: %v", err)
	}
}

// TestReportResult_Phase4WorkflowRulesViolation verifies that when a phase-4
// tasks.md violates a rule in .specs/instructions.md, pipeline_report_result
// writes review-tasks.md, returns revision_required, and does NOT mark
// phase-4 as complete.
func TestReportResult_Phase4WorkflowRulesViolation(t *testing.T) {
	t.Parallel()

	// Layout: tmpRoot/.specs/<spec>/ is the workspace; repo root is tmpRoot.
	tmpRoot, workspace, sm := setupPhase4SpecWorkspace(t, "20260410-test-workflow-rules")
	writeProtoRuleViolationFixture(t, tmpRoot, workspace)

	resp := callPhase4Report(t, sm, workspace)

	if resp.NextActionHint != "revision_required" {
		t.Errorf("NextActionHint = %q, want %q", resp.NextActionHint, "revision_required")
	}
	if resp.VerdictParsed != "REVISE" {
		t.Errorf("VerdictParsed = %q, want %q", resp.VerdictParsed, "REVISE")
	}
	if len(resp.Findings) == 0 {
		t.Error("Findings is empty, want at least one violation")
	}
	if !resp.StateUpdated {
		t.Errorf("StateUpdated = false, want true")
	}
	if !strings.Contains(resp.Warning, "phase-4 workflow rules") {
		t.Errorf("Warning = %q, want substring %q", resp.Warning, "phase-4 workflow rules")
	}

	// Assert: review-tasks.md was written and contains the expected tokens.
	reviewPath := filepath.Join(workspace, "review-tasks.md")
	data, err := os.ReadFile(reviewPath)
	if err != nil {
		t.Fatalf("review-tasks.md not written: %v", err)
	}
	if !strings.Contains(string(data), "proto-rule") {
		t.Errorf("review-tasks.md missing rule ID 'proto-rule':\n%s", data)
	}
	if !strings.Contains(string(data), "REVISE") {
		t.Errorf("review-tasks.md missing 'REVISE' verdict:\n%s", data)
	}

	// Assert: phase-4 was NOT marked complete.
	s, err := state.ReadState(workspace)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if slices.Contains(s.CompletedPhases, "phase-4") {
		t.Errorf("phase-4 must NOT be in CompletedPhases after workflow rules violation; completed = %v", s.CompletedPhases)
	}

	// Assert: TaskRevisions was bumped so handlePhaseFour can enforce the
	// retry limit on the next pipeline_next_action call.
	if s.Revisions.TaskRevisions != 1 {
		t.Errorf("Revisions.TaskRevisions = %d, want 1", s.Revisions.TaskRevisions)
	}
}

// TestReportResult_Phase4WorkflowRulesBumpsTaskRevisions verifies that each
// phase-4 workflow-rules violation increments Revisions.TaskRevisions so the
// engine's MaxRevisionRetries guard (handlePhaseFour) eventually escalates.
func TestReportResult_Phase4WorkflowRulesBumpsTaskRevisions(t *testing.T) {
	t.Parallel()

	tmpRoot, workspace, sm := setupPhase4SpecWorkspace(t, "20260410-bump-task-revisions")
	writeProtoRuleViolationFixture(t, tmpRoot, workspace)

	// First call: violation → revision_required, TaskRevisions == 1.
	resp1 := callPhase4Report(t, sm, workspace)
	if resp1.NextActionHint != "revision_required" {
		t.Fatalf("first call: NextActionHint = %q, want %q", resp1.NextActionHint, "revision_required")
	}
	s, err := state.ReadState(workspace)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if s.Revisions.TaskRevisions != 1 {
		t.Errorf("after first violation: TaskRevisions = %d, want 1", s.Revisions.TaskRevisions)
	}

	// Second call: same violation → another bump, TaskRevisions == 2.
	// This matches MaxRevisionRetries, at which point handlePhaseFour returns
	// a checkpoint on the next pipeline_next_action call (covered in engine tests).
	resp2 := callPhase4Report(t, sm, workspace)
	if resp2.NextActionHint != "revision_required" {
		t.Fatalf("second call: NextActionHint = %q, want %q", resp2.NextActionHint, "revision_required")
	}
	s, err = state.ReadState(workspace)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if s.Revisions.TaskRevisions != 2 {
		t.Errorf("after second violation: TaskRevisions = %d, want 2", s.Revisions.TaskRevisions)
	}
}

// TestReportResult_Phase4NoInstructionsFile verifies that phase-4 completes
// normally (no revision_required) when .specs/instructions.md is absent.
func TestReportResult_Phase4NoInstructionsFile(t *testing.T) {
	t.Parallel()

	_, workspace, sm := setupPhase4SpecWorkspace(t, "20260410-no-rules")

	tasksBody := `# Tasks

## Task 1: Do thing

mode: sequential
`
	if err := os.WriteFile(filepath.Join(workspace, "tasks.md"), []byte(tasksBody), 0o644); err != nil {
		t.Fatalf("write tasks.md: %v", err)
	}

	resp := callPhase4Report(t, sm, workspace)
	if resp.NextActionHint == "revision_required" {
		t.Errorf("NextActionHint = revision_required without instructions.md, want proceed")
	}
	if resp.NextActionHint != "proceed" {
		t.Errorf("NextActionHint = %q, want %q", resp.NextActionHint, "proceed")
	}

	// Phase-4 should be in CompletedPhases.
	s, err := state.ReadState(workspace)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if !slices.Contains(s.CompletedPhases, "phase-4") {
		t.Errorf("phase-4 not in CompletedPhases; completed = %v", s.CompletedPhases)
	}
}

// TestReportResult_Phase4MalformedInstructions verifies that a malformed
// .specs/instructions.md surfaces as an MCP error at phase-4 completion
// rather than being silently ignored. This catches typo'd or broken rule
// files early instead of leaking through as "proceed".
func TestReportResult_Phase4MalformedInstructions(t *testing.T) {
	t.Parallel()

	tmpRoot, workspace, sm := setupPhase4SpecWorkspace(t, "20260410-malformed-rules")

	// Write a .specs/instructions.md with an unknown field ("requires" typo).
	instructionsPath := filepath.Join(tmpRoot, ".specs", "instructions.md")
	instructionsBody := `---
rules:
  - id: typo-rule
    when:
      files_match: ["**/*.go"]
    requires: human_gate
    reason: "typo test"
---
`
	if err := os.WriteFile(instructionsPath, []byte(instructionsBody), 0o644); err != nil {
		t.Fatalf("write instructions: %v", err)
	}

	// Valid tasks.md — violation should NOT be the failure mode; the load
	// error should surface first.
	tasksBody := `# Tasks

## Task 1: Do thing

mode: sequential
files:
- backend/pkg/foo.go
`
	if err := os.WriteFile(filepath.Join(workspace, "tasks.md"), []byte(tasksBody), 0o644); err != nil {
		t.Fatalf("write tasks.md: %v", err)
	}

	h := PipelineReportResultHandler(sm, events.NewEventBus(), history.NewKnowledgeBase(""))
	res := callTool(t, h, map[string]any{
		"workspace":   workspace,
		"phase":       "phase-4",
		"tokens_used": 10,
		"duration_ms": 50,
		"model":       "sonnet",
	})
	if !res.IsError {
		t.Fatalf("expected MCP error for malformed instructions.md, got success: %s", textContent(res))
	}
	if !strings.Contains(textContent(res), "workflow rules") {
		t.Errorf("error content = %q, want substring %q", textContent(res), "workflow rules")
	}

	// Phase-4 must NOT have advanced.
	s, err := state.ReadState(workspace)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if slices.Contains(s.CompletedPhases, "phase-4") {
		t.Errorf("phase-4 should NOT be in CompletedPhases on malformed rules; completed = %v", s.CompletedPhases)
	}
}

// TestReportResult_Phase4RulesPresentNoViolations verifies the happy path
// where .specs/instructions.md has rules but no task violates them: phase-4
// completes normally and any stale review-tasks.md left behind by a previous
// workflow-rules iteration is removed so phase-4b can write a fresh one.
func TestReportResult_Phase4RulesPresentNoViolations(t *testing.T) {
	t.Parallel()

	tmpRoot, workspace, sm := setupPhase4SpecWorkspace(t, "20260410-rules-no-violations")

	instructionsPath := filepath.Join(tmpRoot, ".specs", "instructions.md")
	instructionsBody := `---
rules:
  - id: proto-rule
    when:
      files_match: ["**/*.proto"]
    require: human_gate
    reason: "coordinate with proto repo"
---
`
	if err := os.WriteFile(instructionsPath, []byte(instructionsBody), 0o644); err != nil {
		t.Fatalf("write instructions: %v", err)
	}

	// Task does NOT touch a .proto file — no violation.
	tasksBody := `# Tasks

## Task 1: Refactor helper

mode: sequential
files:
- backend/pkg/util/helper.go
`
	if err := os.WriteFile(filepath.Join(workspace, "tasks.md"), []byte(tasksBody), 0o644); err != nil {
		t.Fatalf("write tasks.md: %v", err)
	}

	// Pre-seed a stale review-tasks.md from a prior workflow-rules iteration
	// (REVISE verdict). applyWorkflowRules must delete it on pass-through so
	// handlePhaseFourB does not read a stale verdict.
	stalePath := filepath.Join(workspace, "review-tasks.md")
	stale := "# Stale\n\n**Verdict:** REVISE\n\nLeftover from previous iteration.\n"
	if err := os.WriteFile(stalePath, []byte(stale), 0o644); err != nil {
		t.Fatalf("write stale review-tasks.md: %v", err)
	}

	resp := callPhase4Report(t, sm, workspace)

	if resp.NextActionHint != "proceed" {
		t.Errorf("NextActionHint = %q, want %q", resp.NextActionHint, "proceed")
	}

	s, err := state.ReadState(workspace)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if !slices.Contains(s.CompletedPhases, "phase-4") {
		t.Errorf("phase-4 not in CompletedPhases; completed = %v", s.CompletedPhases)
	}

	// Stale review-tasks.md must have been removed so the phase-4b reviewer
	// writes a fresh file.
	if _, statErr := os.Stat(stalePath); !os.IsNotExist(statErr) {
		t.Errorf("stale review-tasks.md should have been removed; Stat err = %v", statErr)
	}
}

// TestReportResult_Phase4FlatWorkspace verifies that a flat workspace (not
// under a .specs/ directory) falls through the workflow-rules check silently
// and completes phase-4 normally — the repo-root sanity check must prevent
// mis-resolving a repo root into an unrelated directory.
func TestReportResult_Phase4FlatWorkspace(t *testing.T) {
	t.Parallel()

	// Flat layout: workspace is directly a TempDir, not under .specs/.
	dir := t.TempDir()
	sm := state.NewStateManager("dev")
	if err := sm.Init(dir, "flat-spec"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	tasksBody := `# Tasks

## Task 1: Do thing

mode: sequential
files:
- backend/pkg/api/deal.proto
`
	if err := os.WriteFile(filepath.Join(dir, "tasks.md"), []byte(tasksBody), 0o644); err != nil {
		t.Fatalf("write tasks.md: %v", err)
	}

	resp := callPhase4Report(t, sm, dir)
	if resp.NextActionHint != "proceed" {
		t.Errorf("NextActionHint = %q, want %q (flat workspace should fall through)", resp.NextActionHint, "proceed")
	}

	s, err := state.ReadState(dir)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if !slices.Contains(s.CompletedPhases, "phase-4") {
		t.Errorf("phase-4 should be in CompletedPhases for flat workspace; completed = %v", s.CompletedPhases)
	}
}

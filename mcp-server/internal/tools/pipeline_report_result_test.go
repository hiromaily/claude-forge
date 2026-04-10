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

	"github.com/hiromaily/claude-forge/mcp-server/internal/history"
	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
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

// parsePRRResponse unmarshals reportResultResponse from a content string.
func parsePRRResponse(t *testing.T, content string) reportResultResponse {
	t.Helper()
	var resp reportResultResponse
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		t.Fatalf("unmarshal reportResultResponse: %v (content: %s)", err, content)
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

			h := PipelineReportResultHandler(sm, history.NewKnowledgeBase(""))
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

// TestReportResult_Phase4WorkflowRulesViolation verifies that when a phase-4
// tasks.md violates a rule in .specs/instructions.md, pipeline_report_result
// writes review-tasks.md, returns revision_required, and does NOT mark
// phase-4 as complete.
func TestReportResult_Phase4WorkflowRulesViolation(t *testing.T) {
	t.Parallel()

	// Layout: tmpRoot/.specs/<spec>/ is the workspace; repo root is tmpRoot.
	tmpRoot := t.TempDir()
	specName := "20260410-test-workflow-rules"
	workspace := filepath.Join(tmpRoot, ".specs", specName)
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}

	// Write .specs/instructions.md at the repo root.
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

	// Write a tasks.md that violates the rule: proto file with mode: sequential.
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

	// Initialise state in the workspace directly (matches existing test pattern).
	sm := state.NewStateManager("dev")
	if err := sm.Init(workspace, specName); err != nil {
		t.Fatalf("Init: %v", err)
	}

	h := PipelineReportResultHandler(sm, history.NewKnowledgeBase(""))
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

	resp := parsePRRResponse(t, textContent(res))

	if resp.NextActionHint != "revision_required" {
		t.Errorf("NextActionHint = %q, want %q", resp.NextActionHint, "revision_required")
	}
	if resp.VerdictParsed != "REVISE" {
		t.Errorf("VerdictParsed = %q, want %q", resp.VerdictParsed, "REVISE")
	}
	if len(resp.Findings) == 0 {
		t.Error("Findings is empty, want at least one violation")
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
}

// TestReportResult_Phase4NoInstructionsFile verifies that phase-4 completes
// normally (no revision_required) when .specs/instructions.md is absent.
func TestReportResult_Phase4NoInstructionsFile(t *testing.T) {
	t.Parallel()

	tmpRoot := t.TempDir()
	specName := "20260410-no-rules"
	workspace := filepath.Join(tmpRoot, ".specs", specName)
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	tasksBody := `# Tasks

## Task 1: Do thing

mode: sequential
`
	if err := os.WriteFile(filepath.Join(workspace, "tasks.md"), []byte(tasksBody), 0o644); err != nil {
		t.Fatalf("write tasks.md: %v", err)
	}

	sm := state.NewStateManager("dev")
	if err := sm.Init(workspace, specName); err != nil {
		t.Fatalf("Init: %v", err)
	}

	h := PipelineReportResultHandler(sm, history.NewKnowledgeBase(""))
	res := callTool(t, h, map[string]any{
		"workspace":   workspace,
		"phase":       "phase-4",
		"tokens_used": 10,
		"duration_ms": 50,
		"model":       "sonnet",
	})
	if res.IsError {
		t.Fatalf("unexpected MCP error: %s", textContent(res))
	}

	resp := parsePRRResponse(t, textContent(res))
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

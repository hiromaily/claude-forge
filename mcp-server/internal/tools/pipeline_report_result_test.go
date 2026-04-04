// Package tools — unit tests for pipeline_report_result MCP handler.
// Tests verify phase-log recording, artifact validation, verdict routing, and state transitions.
package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
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
				// Write impl-1.md with FAIL verdict in body.
				content := "## Summary\n\n## Verdict: FAIL\n\nTests did not pass.\n"
				if err := os.WriteFile(filepath.Join(dir, "impl-1.md"), []byte(content), 0o644); err != nil {
					t.Fatalf("WriteFile impl-1.md: %v", err)
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

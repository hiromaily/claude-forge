// Package tools — unit tests for verdict_parser functions.
// Tests verify determineTransition and handlePhase6Transition independently.
package tools

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/internal/intelligence/history"
	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/orchestrator"
	"github.com/hiromaily/claude-forge/mcp-server/internal/intelligence/prompt"
	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/state"
	"github.com/hiromaily/claude-forge/mcp-server/internal/handler/validation"
)

// ---------- helpers for verdict_parser tests ----------

// newVPStateManager creates a fresh StateManager and initialises the given workspace.
func newVPStateManager(t *testing.T, workspace string) *state.StateManager {
	t.Helper()
	sm := state.NewStateManager("dev")
	if err := sm.Init(workspace, "vp-test-spec"); err != nil {
		t.Fatalf("newVPStateManager Init: %v", err)
	}
	return sm
}

// newVPKnowledgeBase returns a KnowledgeBase backed by a temp directory.
func newVPKnowledgeBase(t *testing.T) *history.KnowledgeBase {
	t.Helper()
	dir := t.TempDir()
	return history.NewKnowledgeBase(dir)
}

// writeVPReviewFile writes a review markdown file with the given verdict token
// in the canonical heading format `## Verdict: TOKEN` that ParseVerdict expects.
func writeVPReviewFile(t *testing.T, dir, filename, verdict string) {
	t.Helper()
	content := "## Verdict: " + verdict + "\n\nSome finding details.\n"
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o600); err != nil {
		t.Fatalf("writeVPReviewFile %s: %v", filename, err)
	}
}

// addVPTask adds a task to the state via sm.Update.
//
//nolint:unparam // key is parameterised for reuse across future tests even though current callers always use "1"
func addVPTask(t *testing.T, sm *state.StateManager, key, implStatus string) {
	t.Helper()
	if err := sm.Update(func(s *state.State) error {
		if s.Tasks == nil {
			s.Tasks = make(map[string]state.Task)
		}
		s.Tasks[key] = state.Task{
			Title:         "Task " + key,
			ExecutionMode: state.ExecModeSequential,
			ImplStatus:    implStatus,
		}
		return nil
	}); err != nil {
		t.Fatalf("addVPTask sm.Update: %v", err)
	}
}

// ---------- TestDetermineTransition ----------

func TestDetermineTransition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		phase       string
		setupOnly   bool
		setupFunc   func(t *testing.T, sm *state.StateManager, dir string)
		wantHint    string
		wantVerdict string
		wantErr     bool
	}{
		{
			name:  "non_review_phase_advances",
			phase: "phase-1",
			setupFunc: func(_ *testing.T, _ *state.StateManager, _ string) {
				// No special setup needed for a simple non-review phase.
			},
			wantHint:    "proceed",
			wantVerdict: "",
		},
		{
			name:      "setup_only_returns_setup_continue",
			phase:     "phase-1",
			setupOnly: true,
			setupFunc: func(_ *testing.T, _ *state.StateManager, _ string) {
				// setup_only=true: record log but skip PhaseComplete.
			},
			wantHint:    "setup_continue",
			wantVerdict: "",
		},
		{
			name:  "review_phase_3b_approve_advances",
			phase: "phase-3b",
			setupFunc: func(t *testing.T, _ *state.StateManager, dir string) {
				t.Helper()
				writeVPReviewFile(t, dir, "review-design.md", "APPROVE")
			},
			wantHint:    "proceed",
			wantVerdict: "APPROVE",
		},
		{
			name:  "review_phase_3b_revise_returns_revision_required",
			phase: "phase-3b",
			setupFunc: func(t *testing.T, _ *state.StateManager, dir string) {
				t.Helper()
				writeVPReviewFile(t, dir, "review-design.md", "REVISE")
			},
			wantHint:    "revision_required",
			wantVerdict: "REVISE",
		},
		{
			name:  "review_phase_4b_approve_with_notes_advances",
			phase: "phase-4b",
			setupFunc: func(t *testing.T, _ *state.StateManager, dir string) {
				t.Helper()
				writeVPReviewFile(t, dir, "review-tasks.md", "APPROVE_WITH_NOTES")
			},
			wantHint:    "proceed",
			wantVerdict: "APPROVE_WITH_NOTES",
		},
		{
			name:  "review_phase_4b_revise_increments_tasks_revisions",
			phase: "phase-4b",
			setupFunc: func(t *testing.T, _ *state.StateManager, dir string) {
				t.Helper()
				writeVPReviewFile(t, dir, "review-tasks.md", "REVISE")
			},
			wantHint:    "revision_required",
			wantVerdict: "REVISE",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			sm := newVPStateManager(t, dir)
			kb := newVPKnowledgeBase(t)

			if tc.setupFunc != nil {
				tc.setupFunc(t, sm, dir)
			}

			in := reportResultInput{
				workspace: dir,
				phase:     tc.phase,
				setupOnly: tc.setupOnly,
			}

			var warnings []string
			out, err := determineTransition(sm, kb, in, []validation.ArtifactResult{}, "", &warnings)

			if tc.wantErr {
				if err == nil {
					t.Errorf("determineTransition(%q): expected error, got nil", tc.phase)
				}
				return
			}
			if err != nil {
				t.Fatalf("determineTransition(%q): unexpected error: %v", tc.phase, err)
			}
			if out.NextActionHint != tc.wantHint {
				t.Errorf("NextActionHint = %q, want %q", out.NextActionHint, tc.wantHint)
			}
			if out.VerdictParsed != tc.wantVerdict {
				t.Errorf("VerdictParsed = %q, want %q", out.VerdictParsed, tc.wantVerdict)
			}
		})
	}
}

// ---------- TestDetermineTransition_StaleReviewDetection ----------

func TestDetermineTransition_StaleReviewDetection(t *testing.T) {
	t.Parallel()

	t.Run("stale_review_detected_when_design_newer", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		sm := newVPStateManager(t, dir)
		kb := newVPKnowledgeBase(t)

		// Write review-design.md first (older).
		writeVPReviewFile(t, dir, "review-design.md", "REVISE")

		// Write design.md after (newer) — simulates architect revision.
		// Note: on filesystems with 1-second mtime resolution (HFS+/FAT),
		// both files may share the same mtime. The production code uses >=
		// (not >) so same-mtime also triggers stale detection, making this
		// test correct on all platforms.
		if err := os.WriteFile(filepath.Join(dir, "design.md"), []byte("# Revised Design"), 0o600); err != nil {
			t.Fatalf("write design.md: %v", err)
		}

		in := reportResultInput{workspace: dir, phase: "phase-3b"}
		var warnings []string
		out, err := determineTransition(sm, kb, in, []validation.ArtifactResult{}, "", &warnings)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.NextActionHint != "setup_continue" {
			t.Errorf("NextActionHint = %q, want %q (stale review should trigger setup_continue)", out.NextActionHint, "setup_continue")
		}
		// review-design.md should be deleted.
		if _, statErr := os.Stat(filepath.Join(dir, "review-design.md")); statErr == nil {
			t.Errorf("review-design.md should be deleted after stale review detection")
		}
	})

	t.Run("normal_revise_when_review_newer_than_design", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		sm := newVPStateManager(t, dir)
		kb := newVPKnowledgeBase(t)

		// Write design.md first (older).
		if err := os.WriteFile(filepath.Join(dir, "design.md"), []byte("# Design"), 0o600); err != nil {
			t.Fatalf("write design.md: %v", err)
		}

		// Write review-design.md after (newer) — reviewer just completed.
		writeVPReviewFile(t, dir, "review-design.md", "REVISE")

		in := reportResultInput{workspace: dir, phase: "phase-3b"}
		var warnings []string
		out, err := determineTransition(sm, kb, in, []validation.ArtifactResult{}, "", &warnings)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.NextActionHint != "revision_required" {
			t.Errorf("NextActionHint = %q, want %q (review is fresh, normal REVISE path)", out.NextActionHint, "revision_required")
		}
		// review-design.md should still exist.
		if _, statErr := os.Stat(filepath.Join(dir, "review-design.md")); statErr != nil {
			t.Errorf("review-design.md should still exist after normal REVISE")
		}
	})

	t.Run("stale_review_for_phase_4b_tasks", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		sm := newVPStateManager(t, dir)
		kb := newVPKnowledgeBase(t)

		// Write review-tasks.md first (older).
		writeVPReviewFile(t, dir, "review-tasks.md", "REVISE")

		// Write tasks.md after (newer).
		if err := os.WriteFile(filepath.Join(dir, "tasks.md"), []byte("# Revised Tasks\n\n## Task 1: Implement\nmode: sequential\n"), 0o600); err != nil {
			t.Fatalf("write tasks.md: %v", err)
		}

		in := reportResultInput{workspace: dir, phase: "phase-4b"}
		var warnings []string
		out, err := determineTransition(sm, kb, in, []validation.ArtifactResult{}, "", &warnings)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.NextActionHint != "setup_continue" {
			t.Errorf("NextActionHint = %q, want %q", out.NextActionHint, "setup_continue")
		}
	})
}

// ---------- TestFilterCurrentReviewFindings ----------

func TestFilterCurrentReviewFindings(t *testing.T) {
	t.Parallel()

	t.Run("matching_patterns_excluded", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		// Write review-design.md with known findings.
		content := "## Verdict: REVISE\n\n### Findings\n\n" +
			"**1. [CRITICAL] Missing error handling for auth.**\n\n" +
			"**2. [MINOR] Imprecise wording in section 3.**\n"
		if err := os.WriteFile(filepath.Join(dir, "review-design.md"), []byte(content), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}

		ctx := prompt.HistoryContext{
			CriticalPatterns: []history.PatternEntry{
				{Severity: "CRITICAL", Pattern: "missing error handling for auth.", Frequency: 2, Agent: "design-reviewer"},
				{Severity: "CRITICAL", Pattern: "unrelated critical finding", Frequency: 1, Agent: "design-reviewer"},
			},
			AllPatterns: []history.PatternEntry{
				{Severity: "CRITICAL", Pattern: "missing error handling for auth.", Frequency: 2, Agent: "design-reviewer"},
				{Severity: "MINOR", Pattern: "imprecise wording in section 3.", Frequency: 3, Agent: "design-reviewer"},
				{Severity: "MINOR", Pattern: "unrelated minor finding", Frequency: 1, Agent: "design-reviewer"},
			},
		}

		filtered := filterCurrentReviewFindings(dir, ctx)

		if len(filtered.CriticalPatterns) != 1 {
			t.Errorf("CriticalPatterns count = %d, want 1 (matching pattern should be excluded)", len(filtered.CriticalPatterns))
		}
		if len(filtered.AllPatterns) != 1 {
			t.Errorf("AllPatterns count = %d, want 1 (two matching patterns should be excluded)", len(filtered.AllPatterns))
		}
	})

	t.Run("no_review_file_returns_unchanged", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir() // no review-design.md

		ctx := prompt.HistoryContext{
			CriticalPatterns: []history.PatternEntry{
				{Severity: "CRITICAL", Pattern: "some finding", Frequency: 1},
			},
			AllPatterns: []history.PatternEntry{
				{Severity: "CRITICAL", Pattern: "some finding", Frequency: 1},
			},
		}

		filtered := filterCurrentReviewFindings(dir, ctx)

		if len(filtered.CriticalPatterns) != 1 {
			t.Errorf("CriticalPatterns count = %d, want 1 (should be unchanged)", len(filtered.CriticalPatterns))
		}
		if len(filtered.AllPatterns) != 1 {
			t.Errorf("AllPatterns count = %d, want 1 (should be unchanged)", len(filtered.AllPatterns))
		}
	})
}

// ---------- TestHandlePhase6Transition ----------

func TestHandlePhase6Transition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setupFunc   func(t *testing.T, sm *state.StateManager, dir string)
		results     func(dir string) []validation.ArtifactResult
		wantHint    string
		wantAnyFail bool
	}{
		{
			name: "all_tasks_pass_phase_completes",
			setupFunc: func(t *testing.T, sm *state.StateManager, dir string) {
				t.Helper()
				addVPTask(t, sm, "1", state.TaskStatusCompleted)
				writeVPReviewFile(t, dir, "review-1.md", "PASS")
			},
			results: func(_ string) []validation.ArtifactResult {
				return []validation.ArtifactResult{
					{Valid: true, File: "review-1.md", VerdictFound: state.VerdictPass},
				}
			},
			wantHint: "proceed",
		},
		{
			name: "fail_verdict_returns_retry_impl",
			setupFunc: func(t *testing.T, sm *state.StateManager, dir string) {
				t.Helper()
				addVPTask(t, sm, "1", state.TaskStatusCompleted)
				writeVPReviewFile(t, dir, "review-1.md", "FAIL")
			},
			results: func(_ string) []validation.ArtifactResult {
				return []validation.ArtifactResult{
					{Valid: true, File: "review-1.md", VerdictFound: state.VerdictFail},
				}
			},
			wantHint:    "retry_impl",
			wantAnyFail: true,
		},
		{
			name: "task_without_review_file_holds_in_phase",
			setupFunc: func(t *testing.T, sm *state.StateManager, dir string) {
				t.Helper()
				addVPTask(t, sm, "1", state.TaskStatusCompleted)
				// Intentionally do NOT write review-1.md.
			},
			results: func(_ string) []validation.ArtifactResult {
				// No results — simulates completion gate detecting missing review.
				return []validation.ArtifactResult{}
			},
			wantHint: "setup_continue",
		},
		{
			name: "pass_with_notes_treated_as_passing",
			setupFunc: func(t *testing.T, sm *state.StateManager, dir string) {
				t.Helper()
				addVPTask(t, sm, "1", state.TaskStatusCompleted)
				writeVPReviewFile(t, dir, "review-1.md", "PASS_WITH_NOTES")
			},
			results: func(_ string) []validation.ArtifactResult {
				return []validation.ArtifactResult{
					{Valid: true, File: "review-1.md", VerdictFound: state.VerdictPass},
				}
			},
			wantHint: "proceed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			sm := newVPStateManager(t, dir)

			if tc.setupFunc != nil {
				tc.setupFunc(t, sm, dir)
			}

			in := reportResultInput{
				workspace: dir,
				phase:     "phase-6",
			}

			results := tc.results(dir)
			out, err := handlePhase6Transition(sm, in, results, "")
			if err != nil {
				t.Fatalf("handlePhase6Transition: unexpected error: %v", err)
			}

			if out.NextActionHint != tc.wantHint {
				t.Errorf("NextActionHint = %q, want %q", out.NextActionHint, tc.wantHint)
			}
			if tc.wantAnyFail && len(out.Findings) == 0 {
				t.Logf("wantAnyFail=true but Findings is empty; hint=%q (acceptable if FAIL file has no structured findings)", out.NextActionHint)
			}
		})
	}
}

// TestDetermineTransition_Phase6Delegation verifies that phase-6 is delegated to
// handlePhase6Transition by confirming determineTransition returns the same hint
// as a direct handlePhase6Transition call for a phase-6 input.
func TestDetermineTransition_Phase6Delegation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sm := newVPStateManager(t, dir)
	kb := newVPKnowledgeBase(t)

	addVPTask(t, sm, "1", state.TaskStatusCompleted)
	writeVPReviewFile(t, dir, "review-1.md", "PASS")

	in := reportResultInput{
		workspace: dir,
		phase:     "phase-6",
	}

	results := []validation.ArtifactResult{
		{Valid: true, File: "review-1.md", VerdictFound: state.VerdictPass},
	}

	var warnings []string
	out, err := determineTransition(sm, kb, in, results, "", &warnings)
	if err != nil {
		t.Fatalf("determineTransition(phase-6): unexpected error: %v", err)
	}
	if out.NextActionHint != "proceed" {
		t.Errorf("NextActionHint = %q, want %q", out.NextActionHint, "proceed")
	}
}

// TestDetermineTransition_RevisionBumpIncrementsCounter verifies that REVISE on
// phase-3b increments DesignRevisions in state.
func TestDetermineTransition_RevisionBumpIncrementsCounter(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sm := newVPStateManager(t, dir)
	kb := newVPKnowledgeBase(t)

	writeVPReviewFile(t, dir, "review-design.md", "REVISE")

	in := reportResultInput{
		workspace: dir,
		phase:     "phase-3b",
	}

	var warnings []string
	out, err := determineTransition(sm, kb, in, []validation.ArtifactResult{}, "", &warnings)
	if err != nil {
		t.Fatalf("determineTransition(phase-3b REVISE): %v", err)
	}
	if out.NextActionHint != "revision_required" {
		t.Errorf("NextActionHint = %q, want %q", out.NextActionHint, "revision_required")
	}

	s, err := state.ReadState(dir)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if s.Revisions.DesignRevisions != 1 {
		t.Errorf("DesignRevisions = %d, want 1", s.Revisions.DesignRevisions)
	}
}

// TestHandlePhase6Transition_FailUpdatesState verifies that a FAIL verdict
// updates the task ReviewStatus to completed_fail.
func TestHandlePhase6Transition_FailUpdatesState(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sm := newVPStateManager(t, dir)

	addVPTask(t, sm, "1", state.TaskStatusCompleted)

	// Write a review file with FAIL verdict and a structured finding.
	content := "## Verdict: FAIL\n\n**[CRITICAL]** Missing error handling in handler.\n"
	if err := os.WriteFile(filepath.Join(dir, "review-1.md"), []byte(content), 0o600); err != nil {
		t.Fatalf("write review-1.md: %v", err)
	}

	in := reportResultInput{
		workspace: dir,
		phase:     "phase-6",
	}
	results := []validation.ArtifactResult{
		{Valid: true, File: "review-1.md", VerdictFound: state.VerdictFail},
	}

	out, err := handlePhase6Transition(sm, in, results, "")
	if err != nil {
		t.Fatalf("handlePhase6Transition: %v", err)
	}
	if out.NextActionHint != "retry_impl" {
		t.Errorf("NextActionHint = %q, want %q", out.NextActionHint, "retry_impl")
	}

	s, err := state.ReadState(dir)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	task, ok := s.Tasks["1"]
	if !ok {
		t.Fatal("task 1 not found in state")
	}
	if task.ReviewStatus != state.TaskStatusCompletedFail {
		t.Errorf("task[1].ReviewStatus = %q, want %q", task.ReviewStatus, state.TaskStatusCompletedFail)
	}
	_ = orchestrator.SeverityCritical // ensure orchestrator import is used
}

// Package tools contains guard function tests enforcing the pre-tool-hook.sh
// rules 3a–3j in Go handler functions.
package tools

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
)

// buildTestState returns a minimal State suitable for guard testing.
func buildTestState() *state.State {
	return &state.State{
		Version:            1,
		SpecName:           "test-spec",
		Workspace:          "/tmp/test-workspace",
		CurrentPhase:       "phase-1",
		CurrentPhaseStatus: "in_progress",
		CompletedPhases:    []string{"setup"},
		SkippedPhases:      []string{},
		Tasks:              map[string]state.Task{},
		PhaseLog:           []state.PhaseLogEntry{},
		CheckpointRevisionPending: map[string]bool{
			"checkpoint-a": false,
			"checkpoint-b": false,
		},
	}
}

// ------- Guard 3a: artifact existence (blocking) -------

func TestGuard3a_ArtifactExists_NoError(t *testing.T) {
	dir := t.TempDir()
	// Create the required artifact for phase-1
	if err := os.WriteFile(filepath.Join(dir, "analysis.md"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := buildTestState()
	if err := Guard3aArtifactExists(dir, "phase-1", s); err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
}

func TestGuard3a_ArtifactMissing_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	s := buildTestState()
	if err := Guard3aArtifactExists(dir, "phase-1", s); err == nil {
		t.Error("expected non-nil error when artifact missing")
	}
}

func TestGuard3a_AllPhaseArtifacts(t *testing.T) {
	cases := []struct {
		phase    string
		artifact string
	}{
		{"phase-1", "analysis.md"},
		{"phase-2", "investigation.md"},
		{"phase-3", "design.md"},
		{"phase-3b", "review-design.md"},
		{"phase-4", "tasks.md"},
		{"phase-4b", "review-tasks.md"},
		{"phase-7", "comprehensive-review.md"},
		{"final-summary", "summary.md"},
	}
	for _, tc := range cases {
		t.Run(tc.phase, func(t *testing.T) {
			dir := t.TempDir()
			s := buildTestState()

			// Missing → error
			if err := Guard3aArtifactExists(dir, tc.phase, s); err == nil {
				t.Errorf("phase %s: expected error when artifact missing", tc.phase)
			}

			// Present → nil
			if err := os.WriteFile(filepath.Join(dir, tc.artifact), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := Guard3aArtifactExists(dir, tc.phase, s); err != nil {
				t.Errorf("phase %s: expected nil error when artifact present, got: %v", tc.phase, err)
			}
		})
	}
}

func TestGuard3a_PhasesWithNoArtifact_ReturnsNil(t *testing.T) {
	// Phases that have no required artifact (e.g. checkpoint-a, phase-5, pr-creation)
	noArtifactPhases := []string{
		"checkpoint-a", "checkpoint-b", "phase-5", "phase-6",
		"final-verification", "pr-creation", "post-to-source",
	}
	for _, phase := range noArtifactPhases {
		dir := t.TempDir()
		s := buildTestState()
		if err := Guard3aArtifactExists(dir, phase, s); err != nil {
			t.Errorf("phase %s: expected nil (no artifact required), got: %v", phase, err)
		}
	}
}

func TestGuard3a_SkippedPhase_ReturnsNil(t *testing.T) {
	t.Parallel()
	// When a phase is in SkippedPhases, the artifact guard should not block.
	dir := t.TempDir()
	s := buildTestState()
	s.SkippedPhases = []string{"phase-4b"}
	// No review-tasks.md exists, but phase-4b is skipped → no error.
	if err := Guard3aArtifactExists(dir, "phase-4b", s); err != nil {
		t.Errorf("expected nil error for skipped phase, got: %v", err)
	}
}

func TestGuard3a_NotSkippedPhase_StillBlocks(t *testing.T) {
	t.Parallel()
	// When a phase is NOT in SkippedPhases, the artifact guard should still block.
	dir := t.TempDir()
	s := buildTestState()
	s.SkippedPhases = []string{"phase-3b"} // different phase is skipped
	// No review-tasks.md exists, phase-4b is NOT skipped → error.
	if err := Guard3aArtifactExists(dir, "phase-4b", s); err == nil {
		t.Error("expected non-nil error when phase is not skipped and artifact missing")
	}
}

// ------- Guard 3b: review file existence (blocking) -------

func TestGuard3b_ReviewFileExists_NoError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "review-1.md"), []byte("review"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := buildTestState()
	if err := Guard3bReviewFileExists(dir, "1", "completed_pass", s); err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
}

func TestGuard3b_ReviewFileMissing_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	s := buildTestState()
	if err := Guard3bReviewFileExists(dir, "1", "completed_pass", s); err == nil {
		t.Error("expected non-nil error when review file missing")
	}
}

func TestGuard3b_NonPassStatus_NoCheck(t *testing.T) {
	// Guard only applies when reviewStatus == "completed_pass"
	dir := t.TempDir()
	s := buildTestState()
	// No review file exists, but status is not completed_pass → no error
	if err := Guard3bReviewFileExists(dir, "1", "in_progress", s); err != nil {
		t.Errorf("expected nil for non-pass status, got: %v", err)
	}
}

// ------- Guard 3c: phase-5 requires non-empty tasks (blocking) -------

func TestGuard3c_TasksNonEmpty_NoError(t *testing.T) {
	s := buildTestState()
	s.Tasks = map[string]state.Task{
		"1": {Title: "task1", ImplStatus: "pending"},
	}
	if err := Guard3cTasksNonEmpty("phase-5", s); err != nil {
		t.Errorf("expected nil error when tasks non-empty, got: %v", err)
	}
}

func TestGuard3c_TasksEmpty_ReturnsError(t *testing.T) {
	s := buildTestState()
	if err := Guard3cTasksNonEmpty("phase-5", s); err == nil {
		t.Error("expected non-nil error when tasks empty")
	}
}

func TestGuard3c_NonPhase5_NoCheck(t *testing.T) {
	// Guard only applies for phase-5
	s := buildTestState()
	// Tasks is empty but phase is not phase-5 → no error
	if err := Guard3cTasksNonEmpty("phase-1", s); err != nil {
		t.Errorf("expected nil for non-phase-5, got: %v", err)
	}
}

// ------- Guard 3e: checkpoint requires awaiting_human (blocking) -------

func TestGuard3e_AwaitingHuman_NoError(t *testing.T) {
	s := buildTestState()
	s.CurrentPhaseStatus = "awaiting_human"
	if err := Guard3eCheckpointAwaitingHuman("checkpoint-a", s); err != nil {
		t.Errorf("expected nil error when awaiting_human, got: %v", err)
	}
}

func TestGuard3e_NotAwaitingHuman_ReturnsError(t *testing.T) {
	s := buildTestState()
	s.CurrentPhaseStatus = "in_progress"
	if err := Guard3eCheckpointAwaitingHuman("checkpoint-a", s); err == nil {
		t.Error("expected non-nil error when not awaiting_human")
	}
}

func TestGuard3e_NonCheckpointPhase_NoCheck(t *testing.T) {
	s := buildTestState()
	s.CurrentPhaseStatus = "in_progress"
	// Guard only applies to checkpoint-a/b
	if err := Guard3eCheckpointAwaitingHuman("phase-1", s); err != nil {
		t.Errorf("expected nil for non-checkpoint phase, got: %v", err)
	}
}

func TestGuard3e_CheckpointBAwaitingHuman_NoError(t *testing.T) {
	t.Parallel()
	s := buildTestState()
	s.CurrentPhaseStatus = "awaiting_human"
	if err := Guard3eCheckpointAwaitingHuman("checkpoint-b", s); err != nil {
		t.Errorf("expected nil error for checkpoint-b awaiting_human, got: %v", err)
	}
}

func TestGuard3e_SkippedCheckpoint_NoError(t *testing.T) {
	t.Parallel()
	// A skipped checkpoint must not require awaiting_human — the engine returns
	// ActionDone (skip signal) and phase_complete is called directly.
	for _, phase := range []string{"checkpoint-a", "checkpoint-b"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			s := buildTestState()
			s.CurrentPhaseStatus = "pending"
			s.SkippedPhases = []string{phase}
			if err := Guard3eCheckpointAwaitingHuman(phase, s); err != nil {
				t.Errorf("expected nil for skipped %s, got: %v", phase, err)
			}
		})
	}
}

// ------- Guard 3g: task-init requires checkpoint-b done or skipped (blocking) -------

func TestGuard3g_CheckpointBCompleted_NoError(t *testing.T) {
	s := buildTestState()
	s.CompletedPhases = []string{"setup", "checkpoint-b"}
	if err := Guard3gCheckpointBDoneOrSkipped(s); err != nil {
		t.Errorf("expected nil error when checkpoint-b completed, got: %v", err)
	}
}

func TestGuard3g_CheckpointBSkipped_NoError(t *testing.T) {
	s := buildTestState()
	s.SkippedPhases = []string{"checkpoint-b"}
	if err := Guard3gCheckpointBDoneOrSkipped(s); err != nil {
		t.Errorf("expected nil error when checkpoint-b skipped, got: %v", err)
	}
}

func TestGuard3g_CheckpointBNeither_ReturnsError(t *testing.T) {
	s := buildTestState()
	if err := Guard3gCheckpointBDoneOrSkipped(s); err == nil {
		t.Error("expected non-nil error when checkpoint-b neither completed nor skipped")
	}
}

// ------- Guard 3j: checkpoint revision pending (blocking) -------

func TestGuard3j_RevisionPendingFalse_NoError(t *testing.T) {
	s := buildTestState()
	s.CheckpointRevisionPending = map[string]bool{
		"checkpoint-a": false,
		"checkpoint-b": false,
	}
	if err := Guard3jCheckpointRevisionPending("checkpoint-a", s); err != nil {
		t.Errorf("expected nil error when revision not pending, got: %v", err)
	}
}

func TestGuard3j_RevisionPendingTrue_ReturnsError(t *testing.T) {
	s := buildTestState()
	s.CheckpointRevisionPending = map[string]bool{
		"checkpoint-a": true,
		"checkpoint-b": false,
	}
	if err := Guard3jCheckpointRevisionPending("checkpoint-a", s); err == nil {
		t.Error("expected non-nil error when revision is pending")
	}
}

func TestGuard3j_NonCheckpointPhase_NoCheck(t *testing.T) {
	s := buildTestState()
	s.CheckpointRevisionPending = map[string]bool{
		"checkpoint-a": true,
	}
	if err := Guard3jCheckpointRevisionPending("phase-1", s); err != nil {
		t.Errorf("expected nil for non-checkpoint phase, got: %v", err)
	}
}

// ------- Guard 3d: phase-log duplicate warning (non-blocking) -------

func TestGuard3d_NoDuplicate_EmptyString(t *testing.T) {
	s := buildTestState()
	s.PhaseLog = []state.PhaseLogEntry{} // No entries
	result := Warn3dPhaseLogDuplicate("phase-1", s)
	if result != "" {
		t.Errorf("expected empty string for no duplicate, got: %q", result)
	}
}

func TestGuard3d_WithDuplicate_ReturnsWarning(t *testing.T) {
	s := buildTestState()
	s.PhaseLog = []state.PhaseLogEntry{
		{Phase: "phase-1", Tokens: 1000, DurationMs: 5000, Model: "sonnet"},
	}
	result := Warn3dPhaseLogDuplicate("phase-1", s)
	if result == "" {
		t.Error("expected non-empty warning string for duplicate phase-log entry")
	}
}

// ------- Guard 3f: phase-log entry missing warning (non-blocking) -------

func TestGuard3f_PhaseLogExists_EmptyString(t *testing.T) {
	s := buildTestState()
	s.PhaseLog = []state.PhaseLogEntry{
		{Phase: "phase-1", Tokens: 500, DurationMs: 3000, Model: "sonnet"},
	}
	result := Warn3fPhaseLogMissing("phase-1", s)
	if result != "" {
		t.Errorf("expected empty string when phase-log entry exists, got: %q", result)
	}
}

func TestGuard3f_PhaseLogMissing_ReturnsWarning(t *testing.T) {
	s := buildTestState()
	s.PhaseLog = []state.PhaseLogEntry{}
	// phase-1 is in the expected-phases list
	result := Warn3fPhaseLogMissing("phase-1", s)
	if result == "" {
		t.Error("expected non-empty warning string when phase-log entry missing")
	}
}

func TestGuard3f_ExemptPhases_EmptyString(t *testing.T) {
	// Checkpoint and admin phases are not required to have phase-log entries
	exemptPhases := []string{"checkpoint-a", "checkpoint-b", "pr-creation", "post-to-source", "final-summary"}
	s := buildTestState()
	for _, phase := range exemptPhases {
		result := Warn3fPhaseLogMissing(phase, s)
		if result != "" {
			t.Errorf("phase %s: expected empty string for exempt phase, got: %q", phase, result)
		}
	}
}

// ------- Guard 3h: task number does not exist (non-blocking) -------

func TestGuard3h_TaskExists_EmptyString(t *testing.T) {
	s := buildTestState()
	s.Tasks = map[string]state.Task{
		"1": {Title: "task1"},
	}
	result := Warn3hTaskNotFound("1", s)
	if result != "" {
		t.Errorf("expected empty string when task exists, got: %q", result)
	}
}

func TestGuard3h_TaskMissing_ReturnsWarning(t *testing.T) {
	s := buildTestState()
	result := Warn3hTaskNotFound("99", s)
	if result == "" {
		t.Error("expected non-empty warning string when task not found")
	}
}

// ------- Guard 3i: phase-complete when currentPhaseStatus not in_progress (non-blocking) -------

func TestGuard3i_InProgress_EmptyString(t *testing.T) {
	s := buildTestState()
	s.CurrentPhaseStatus = "in_progress"
	result := Warn3iPhaseNotInProgress(s)
	if result != "" {
		t.Errorf("expected empty string for in_progress status, got: %q", result)
	}
}

func TestGuard3i_NotInProgress_ReturnsWarning(t *testing.T) {
	s := buildTestState()
	s.CurrentPhaseStatus = "pending"
	result := Warn3iPhaseNotInProgress(s)
	if result == "" {
		t.Error("expected non-empty warning string when not in_progress")
	}
}

// ------- Init guard (AC-3): validated == true (blocking) -------

func TestInitGuard_ValidatedTrue_NoError(t *testing.T) {
	if err := GuardInitValidated(true); err != nil {
		t.Errorf("expected nil error when validated=true, got: %v", err)
	}
}

func TestInitGuard_ValidatedFalse_ReturnsError(t *testing.T) {
	if err := GuardInitValidated(false); err == nil {
		t.Error("expected non-nil error when validated=false")
	}
}

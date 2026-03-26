// Package state_test contains tests for the StateManager.
// Task 2 covers: Init, Get, PhaseStart, PhaseComplete, PhaseFail,
// Abandon, Checkpoint, SkipPhase, RevisionBump, InlineRevisionBump,
// SetRevisionPending, ClearRevisionPending.
package state_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/state"
)

// loadState reads and unmarshals state.json from workspace.
func loadState(t *testing.T, workspace string) state.State {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(workspace, "state.json"))
	if err != nil {
		t.Fatalf("readState: %v", err)
	}
	var s state.State
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	return s
}

func newManager() *state.StateManager {
	return state.NewStateManager()
}

// ---------- Init ----------

func TestInit_CreatesStateJSON(t *testing.T) {
	dir := t.TempDir()
	m := newManager()

	if err := m.Init(dir, "test-spec"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	s := loadState(t, dir)

	if s.Version != 1 {
		t.Errorf("version: got %d, want 1", s.Version)
	}
	if s.SpecName != "test-spec" {
		t.Errorf("specName: got %q, want %q", s.SpecName, "test-spec")
	}
	if s.Workspace != dir {
		t.Errorf("workspace: got %q, want %q", s.Workspace, dir)
	}
	if s.Branch != nil {
		t.Errorf("branch: got %v, want nil", s.Branch)
	}
	if s.TaskType != nil {
		t.Errorf("taskType: got %v, want nil", s.TaskType)
	}
	if s.AutoApprove != false {
		t.Error("autoApprove: want false")
	}
	if s.SkipPr != false {
		t.Error("skipPr: want false")
	}
	if s.Debug != false {
		t.Error("debug: want false")
	}
	if s.CurrentPhase != "phase-1" {
		t.Errorf("currentPhase: got %q, want %q", s.CurrentPhase, "phase-1")
	}
	if s.CurrentPhaseStatus != "pending" {
		t.Errorf("currentPhaseStatus: got %q, want %q", s.CurrentPhaseStatus, "pending")
	}
	if len(s.CompletedPhases) != 1 || s.CompletedPhases[0] != "setup" {
		t.Errorf("completedPhases: got %v, want [\"setup\"]", s.CompletedPhases)
	}
	if len(s.SkippedPhases) != 0 {
		t.Errorf("skippedPhases: got %v, want []", s.SkippedPhases)
	}
	if s.Revisions.DesignRevisions != 0 || s.Revisions.TaskRevisions != 0 {
		t.Errorf("revisions: got %+v, want all zero", s.Revisions)
	}
	if s.CheckpointRevisionPending == nil {
		t.Fatal("checkpointRevisionPending is nil")
	}
	if s.CheckpointRevisionPending["checkpoint-a"] != false {
		t.Error("checkpointRevisionPending[checkpoint-a]: want false")
	}
	if s.CheckpointRevisionPending["checkpoint-b"] != false {
		t.Error("checkpointRevisionPending[checkpoint-b]: want false")
	}
	if s.Tasks == nil {
		t.Error("tasks: want non-nil empty map")
	}
	if len(s.PhaseLog) != 0 {
		t.Errorf("phaseLog: got %d entries, want 0", len(s.PhaseLog))
	}
	if s.Timestamps.Created == "" {
		t.Error("timestamps.created: want non-empty")
	}
	if s.Timestamps.LastUpdated == "" {
		t.Error("timestamps.lastUpdated: want non-empty")
	}
	if s.Timestamps.PhaseStarted != nil {
		t.Errorf("timestamps.phaseStarted: got %v, want nil", s.Timestamps.PhaseStarted)
	}
	if s.Error != nil {
		t.Errorf("error: got %v, want nil", s.Error)
	}
}

// ---------- Get ----------

func TestGet_TopLevelFields(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "myspec"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	cases := []struct {
		field string
		want  string
	}{
		{"specName", "myspec"},
		{"workspace", dir},
		{"currentPhase", "phase-1"},
		{"currentPhaseStatus", "pending"},
		{"autoApprove", "false"},
		{"skipPr", "false"},
		{"debug", "false"},
		{"useCurrentBranch", "false"},
		{"version", "1"},
	}
	for _, tc := range cases {
		got, err := m.Get(dir, tc.field)
		if err != nil {
			t.Errorf("Get(%q): unexpected error: %v", tc.field, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Get(%q): got %q, want %q", tc.field, got, tc.want)
		}
	}
}

func TestGet_NullField_ReturnNull(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	got, err := m.Get(dir, "branch")
	if err != nil {
		t.Fatalf("Get(branch): %v", err)
	}
	if got != "null" {
		t.Errorf("Get(branch): got %q, want %q", got, "null")
	}
}

func TestGet_DotNotation_TimestampsCreated(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	got, err := m.Get(dir, "timestamps.created")
	if err != nil {
		t.Fatalf("Get(timestamps.created): %v", err)
	}
	if got == "" || got == "null" {
		t.Errorf("Get(timestamps.created): got %q, want non-empty timestamp", got)
	}
}

func TestGet_DotNotation_RevisionsDesign(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	got, err := m.Get(dir, "revisions.designRevisions")
	if err != nil {
		t.Fatalf("Get(revisions.designRevisions): %v", err)
	}
	if got != "0" {
		t.Errorf("Get(revisions.designRevisions): got %q, want %q", got, "0")
	}
}

func TestGet_InvalidField_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	_, err := m.Get(dir, "nonExistentField")
	if err == nil {
		t.Error("Get(nonExistentField): expected error, got nil")
	}
}

// ---------- PhaseStart ----------

func TestPhaseStart_SetsInProgress(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.PhaseStart(dir, "phase-1"); err != nil {
		t.Fatalf("PhaseStart: %v", err)
	}

	s := loadState(t, dir)
	if s.CurrentPhase != "phase-1" {
		t.Errorf("currentPhase: got %q, want %q", s.CurrentPhase, "phase-1")
	}
	if s.CurrentPhaseStatus != "in_progress" {
		t.Errorf("currentPhaseStatus: got %q, want %q", s.CurrentPhaseStatus, "in_progress")
	}
	if s.Timestamps.PhaseStarted == nil {
		t.Error("timestamps.phaseStarted: want non-nil")
	}
	if s.Error != nil {
		t.Errorf("error: got %v, want nil", s.Error)
	}
}

func TestPhaseStart_InvalidPhase_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.PhaseStart(dir, "invalid-phase"); err == nil {
		t.Error("PhaseStart(invalid-phase): expected error, got nil")
	}
}

// ---------- PhaseComplete ----------

func TestPhaseComplete_AdvancesPhase(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.PhaseStart(dir, "phase-1"); err != nil {
		t.Fatalf("PhaseStart: %v", err)
	}

	if err := m.PhaseComplete(dir, "phase-1"); err != nil {
		t.Fatalf("PhaseComplete: %v", err)
	}

	s := loadState(t, dir)
	if s.CurrentPhase != "phase-2" {
		t.Errorf("currentPhase: got %q, want %q", s.CurrentPhase, "phase-2")
	}
	if s.CurrentPhaseStatus != "pending" {
		t.Errorf("currentPhaseStatus: got %q, want %q", s.CurrentPhaseStatus, "pending")
	}
	found := false
	for _, p := range s.CompletedPhases {
		if p == "phase-1" {
			found = true
		}
	}
	if !found {
		t.Errorf("completedPhases: phase-1 not found in %v", s.CompletedPhases)
	}
	if s.Timestamps.PhaseStarted != nil {
		t.Errorf("timestamps.phaseStarted: want nil after complete, got %v", s.Timestamps.PhaseStarted)
	}
}

func TestPhaseComplete_LastPhase_StatusCompleted(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.PhaseComplete(dir, "completed"); err != nil {
		t.Fatalf("PhaseComplete(completed): %v", err)
	}

	s := loadState(t, dir)
	if s.CurrentPhaseStatus != "completed" {
		t.Errorf("currentPhaseStatus: got %q, want %q", s.CurrentPhaseStatus, "completed")
	}
}

func TestPhaseComplete_InvalidPhase_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.PhaseComplete(dir, "bad-phase"); err == nil {
		t.Error("PhaseComplete(bad-phase): expected error, got nil")
	}
}

// ---------- PhaseFail ----------

func TestPhaseFail_SetsFailedWithError(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.PhaseFail(dir, "phase-1", "something broke"); err != nil {
		t.Fatalf("PhaseFail: %v", err)
	}

	s := loadState(t, dir)
	if s.CurrentPhaseStatus != "failed" {
		t.Errorf("currentPhaseStatus: got %q, want %q", s.CurrentPhaseStatus, "failed")
	}
	if s.Error == nil {
		t.Fatal("error: want non-nil")
	}
	if s.Error.Phase != "phase-1" {
		t.Errorf("error.phase: got %q, want %q", s.Error.Phase, "phase-1")
	}
	if s.Error.Message != "something broke" {
		t.Errorf("error.message: got %q, want %q", s.Error.Message, "something broke")
	}
	if s.Error.Timestamp == "" {
		t.Error("error.timestamp: want non-empty")
	}
}

func TestPhaseFail_InvalidPhase_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.PhaseFail(dir, "nonexistent", "msg"); err == nil {
		t.Error("PhaseFail(nonexistent): expected error, got nil")
	}
}

// ---------- Checkpoint ----------

func TestCheckpoint_SetsAwaitingHuman(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.Checkpoint(dir, "checkpoint-a"); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}

	s := loadState(t, dir)
	if s.CurrentPhase != "checkpoint-a" {
		t.Errorf("currentPhase: got %q, want %q", s.CurrentPhase, "checkpoint-a")
	}
	if s.CurrentPhaseStatus != "awaiting_human" {
		t.Errorf("currentPhaseStatus: got %q, want %q", s.CurrentPhaseStatus, "awaiting_human")
	}
}

func TestCheckpoint_InvalidPhase_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.Checkpoint(dir, "not-a-checkpoint"); err == nil {
		t.Error("Checkpoint(not-a-checkpoint): expected error, got nil")
	}
}

// ---------- Abandon ----------

func TestAbandon_SetsAbandoned(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.Abandon(dir); err != nil {
		t.Fatalf("Abandon: %v", err)
	}

	s := loadState(t, dir)
	if s.CurrentPhaseStatus != "abandoned" {
		t.Errorf("currentPhaseStatus: got %q, want %q", s.CurrentPhaseStatus, "abandoned")
	}
}

// ---------- SkipPhase ----------

func TestSkipPhase_AddsToSkippedAndAdvances(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.SkipPhase(dir, "phase-1"); err != nil {
		t.Fatalf("SkipPhase: %v", err)
	}

	s := loadState(t, dir)
	found := false
	for _, p := range s.SkippedPhases {
		if p == "phase-1" {
			found = true
		}
	}
	if !found {
		t.Errorf("skippedPhases: phase-1 not found in %v", s.SkippedPhases)
	}
	if s.CurrentPhase != "phase-2" {
		t.Errorf("currentPhase: got %q, want %q (should advance)", s.CurrentPhase, "phase-2")
	}
	if s.CurrentPhaseStatus != "pending" {
		t.Errorf("currentPhaseStatus: got %q, want %q", s.CurrentPhaseStatus, "pending")
	}
}

func TestSkipPhase_InvalidPhase_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.SkipPhase(dir, "bogus-phase"); err == nil {
		t.Error("SkipPhase(bogus-phase): expected error, got nil")
	}
}

// ---------- RevisionBump ----------

func TestRevisionBump_Design_Increments(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.RevisionBump(dir, "design"); err != nil {
		t.Fatalf("RevisionBump(design): %v", err)
	}
	if err := m.RevisionBump(dir, "design"); err != nil {
		t.Fatalf("RevisionBump(design) 2nd: %v", err)
	}

	s := loadState(t, dir)
	if s.Revisions.DesignRevisions != 2 {
		t.Errorf("revisions.designRevisions: got %d, want 2", s.Revisions.DesignRevisions)
	}
}

func TestRevisionBump_Tasks_Increments(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.RevisionBump(dir, "tasks"); err != nil {
		t.Fatalf("RevisionBump(tasks): %v", err)
	}

	s := loadState(t, dir)
	if s.Revisions.TaskRevisions != 1 {
		t.Errorf("revisions.taskRevisions: got %d, want 1", s.Revisions.TaskRevisions)
	}
}

func TestRevisionBump_InvalidType_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.RevisionBump(dir, "invalid"); err == nil {
		t.Error("RevisionBump(invalid): expected error, got nil")
	}
}

// ---------- InlineRevisionBump ----------

func TestInlineRevisionBump_Design_Increments(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.InlineRevisionBump(dir, "design"); err != nil {
		t.Fatalf("InlineRevisionBump(design): %v", err)
	}

	s := loadState(t, dir)
	if s.Revisions.DesignInlineRevisions != 1 {
		t.Errorf("revisions.designInlineRevisions: got %d, want 1", s.Revisions.DesignInlineRevisions)
	}
	if s.Revisions.DesignRevisions != 0 {
		t.Errorf("revisions.designRevisions: got %d, want 0 (should be unaffected)", s.Revisions.DesignRevisions)
	}
}

func TestInlineRevisionBump_Tasks_Increments(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.InlineRevisionBump(dir, "tasks"); err != nil {
		t.Fatalf("InlineRevisionBump(tasks): %v", err)
	}

	s := loadState(t, dir)
	if s.Revisions.TaskInlineRevisions != 1 {
		t.Errorf("revisions.taskInlineRevisions: got %d, want 1", s.Revisions.TaskInlineRevisions)
	}
}

func TestInlineRevisionBump_InvalidType_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.InlineRevisionBump(dir, "bogus"); err == nil {
		t.Error("InlineRevisionBump(bogus): expected error, got nil")
	}
}

// ---------- SetRevisionPending / ClearRevisionPending ----------

func TestSetRevisionPending_SetsTrue(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.SetRevisionPending(dir, "checkpoint-a"); err != nil {
		t.Fatalf("SetRevisionPending: %v", err)
	}

	s := loadState(t, dir)
	if !s.CheckpointRevisionPending["checkpoint-a"] {
		t.Error("checkpointRevisionPending[checkpoint-a]: want true")
	}
	if s.CheckpointRevisionPending["checkpoint-b"] {
		t.Error("checkpointRevisionPending[checkpoint-b]: want false (unaffected)")
	}
}

func TestClearRevisionPending_SetsFalse(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.SetRevisionPending(dir, "checkpoint-b"); err != nil {
		t.Fatalf("SetRevisionPending: %v", err)
	}
	if err := m.ClearRevisionPending(dir, "checkpoint-b"); err != nil {
		t.Fatalf("ClearRevisionPending: %v", err)
	}

	s := loadState(t, dir)
	if s.CheckpointRevisionPending["checkpoint-b"] {
		t.Error("checkpointRevisionPending[checkpoint-b]: want false after clear")
	}
}

func TestSetRevisionPending_InvalidCheckpoint_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.SetRevisionPending(dir, "checkpoint-c"); err == nil {
		t.Error("SetRevisionPending(checkpoint-c): expected error, got nil")
	}
}

func TestClearRevisionPending_InvalidCheckpoint_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.ClearRevisionPending(dir, "bad-checkpoint"); err == nil {
		t.Error("ClearRevisionPending(bad-checkpoint): expected error, got nil")
	}
}

// ---------- Mutex enforcement via Get / Init ----------

func TestGet_MissingStateFile_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	m := newManager()

	_, err := m.Get(dir, "specName")
	if err == nil {
		t.Error("Get on missing state.json: expected error, got nil")
	}
}

func TestPhaseStart_MissingStateFile_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	m := newManager()

	if err := m.PhaseStart(dir, "phase-1"); err == nil {
		t.Error("PhaseStart on missing state.json: expected error, got nil")
	}
}

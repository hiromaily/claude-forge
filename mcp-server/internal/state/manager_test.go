// Package state_test contains tests for the StateManager.
// Task 2 covers: Init, Get, PhaseStart, PhaseComplete, PhaseFail,
// Abandon, Checkpoint, SkipPhase, RevisionBump, InlineRevisionBump,
// SetRevisionPending, ClearRevisionPending.
// Task 3 covers: SetEffort, SetFlowTemplate, TaskInit, TaskUpdate,
// PhaseLog, PhaseStats.
// Task 4 adds: golden-file test for Init schema, exhaustive nextPhase test
// covering all 18 ValidPhases, and a concurrency test (10 goroutines, -race safe).
package state_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
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
	return state.NewStateManager("dev")
}

// ---------- Init ----------

func TestInit_CreatesStateJSON(t *testing.T) {
	dir := t.TempDir()
	m := newManager()

	if err := m.Init(dir, "test-spec"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	s := loadState(t, dir)

	if s.Version != 2 {
		t.Errorf("version: got %d, want 2", s.Version)
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
		{"version", "2"},
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

// ---------- SetEffort ----------

func TestSetEffort_ValidValues_Accepted(t *testing.T) {
	for _, effort := range state.ValidEfforts {
		dir := t.TempDir()
		m := newManager()
		if err := m.Init(dir, "s"); err != nil {
			t.Fatalf("Init: %v", err)
		}

		if err := m.SetEffort(dir, effort); err != nil {
			t.Errorf("SetEffort(%q): unexpected error: %v", effort, err)
			continue
		}

		s := loadState(t, dir)
		if s.Effort == nil || *s.Effort != effort {
			t.Errorf("SetEffort(%q): effort field not set correctly, got %v", effort, s.Effort)
		}
	}
}

func TestSetEffort_InvalidValue_ReturnsDescriptiveError(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	err := m.SetEffort(dir, "INVALID")
	if err == nil {
		t.Fatal("SetEffort(INVALID): expected error, got nil")
	}
	// Error must be descriptive — it should mention the invalid value or valid options.
	msg := err.Error()
	if msg == "" {
		t.Error("SetEffort error message must not be empty")
	}
	// Check that it references "INVALID" or lists valid options
	if !containsAny(msg, "INVALID", "S", "M", "L") {
		t.Errorf("SetEffort error not descriptive enough: %q", msg)
	}
}

func TestSetEffort_InvalidValues_AllRejected(t *testing.T) {
	invalid := []string{"", "xs", "XL", "LARGE", "medium"}
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	for _, v := range invalid {
		if err := m.SetEffort(dir, v); err == nil {
			t.Errorf("SetEffort(%q): expected error, got nil", v)
		}
	}
}

// ---------- SetFlowTemplate ----------

func TestSetFlowTemplate_ValidValues_Accepted(t *testing.T) {
	for _, tmpl := range state.ValidTemplates {
		dir := t.TempDir()
		m := newManager()
		if err := m.Init(dir, "s"); err != nil {
			t.Fatalf("Init: %v", err)
		}

		if err := m.SetFlowTemplate(dir, tmpl); err != nil {
			t.Errorf("SetFlowTemplate(%q): unexpected error: %v", tmpl, err)
			continue
		}

		s := loadState(t, dir)
		if s.FlowTemplate == nil || *s.FlowTemplate != tmpl {
			t.Errorf("SetFlowTemplate(%q): flowTemplate field not set correctly, got %v", tmpl, s.FlowTemplate)
		}
	}
}

func TestSetFlowTemplate_InvalidValue_ReturnsDescriptiveError(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	err := m.SetFlowTemplate(dir, "heavy")
	if err == nil {
		t.Fatal("SetFlowTemplate(heavy): expected error, got nil")
	}
	msg := err.Error()
	if msg == "" {
		t.Error("SetFlowTemplate error message must not be empty")
	}
	// Error must mention the invalid value or list valid options.
	if !containsAny(msg, "heavy", "light", "standard", "full") {
		t.Errorf("SetFlowTemplate error not descriptive enough: %q", msg)
	}
}

func TestSetFlowTemplate_InvalidValues_AllRejected(t *testing.T) {
	invalid := []string{"", "LITE", "Full", "medium", "custom"}
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	for _, v := range invalid {
		if err := m.SetFlowTemplate(dir, v); err == nil {
			t.Errorf("SetFlowTemplate(%q): expected error, got nil", v)
		}
	}
}

// ---------- TaskInit ----------

func TestTaskInit_StoresAllTasks_WritesState(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	tasks := map[string]state.Task{
		"1": {Title: "First task", ExecutionMode: "sequential", ImplStatus: "pending", ReviewStatus: "pending"},
		"2": {Title: "Second task", ExecutionMode: "parallel", ImplStatus: "pending", ReviewStatus: "pending"},
		"3": {Title: "Third task", ExecutionMode: "sequential", ImplStatus: "pending", ReviewStatus: "pending", ImplRetries: 0, ReviewRetries: 0},
	}

	if err := m.TaskInit(dir, tasks); err != nil {
		t.Fatalf("TaskInit: %v", err)
	}

	s := loadState(t, dir)
	if len(s.Tasks) != 3 {
		t.Errorf("tasks count: got %d, want 3", len(s.Tasks))
	}
	for num, want := range tasks {
		got, ok := s.Tasks[num]
		if !ok {
			t.Errorf("task %q not found in state", num)
			continue
		}
		if got.Title != want.Title {
			t.Errorf("task %q title: got %q, want %q", num, got.Title, want.Title)
		}
		if got.ExecutionMode != want.ExecutionMode {
			t.Errorf("task %q executionMode: got %q, want %q", num, got.ExecutionMode, want.ExecutionMode)
		}
	}
}

func TestTaskInit_EmptyMap_WritesEmptyTasks(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.TaskInit(dir, map[string]state.Task{}); err != nil {
		t.Fatalf("TaskInit(empty): %v", err)
	}

	s := loadState(t, dir)
	if len(s.Tasks) != 0 {
		t.Errorf("tasks: got %d entries, want 0", len(s.Tasks))
	}
}

// ---------- TaskUpdate ----------

func TestTaskUpdate_ImplStatus_UpdatesOnly(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	tasks := map[string]state.Task{
		"1": {Title: "Task one", ImplStatus: "pending", ReviewStatus: "pending"},
	}
	if err := m.TaskInit(dir, tasks); err != nil {
		t.Fatalf("TaskInit: %v", err)
	}

	if err := m.TaskUpdate(dir, "1", "implStatus", "completed"); err != nil {
		t.Fatalf("TaskUpdate(implStatus): %v", err)
	}

	s := loadState(t, dir)
	if s.Tasks["1"].ImplStatus != "completed" {
		t.Errorf("implStatus: got %q, want %q", s.Tasks["1"].ImplStatus, "completed")
	}
	// Other fields should remain unchanged.
	if s.Tasks["1"].ReviewStatus != "pending" {
		t.Errorf("reviewStatus: got %q, want %q (should be unchanged)", s.Tasks["1"].ReviewStatus, "pending")
	}
	if s.Tasks["1"].Title != "Task one" {
		t.Errorf("title: got %q, want %q (should be unchanged)", s.Tasks["1"].Title, "Task one")
	}
}

func TestTaskUpdate_ReviewStatus_UpdatesOnly(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	tasks := map[string]state.Task{
		"1": {Title: "Task one", ImplStatus: "completed", ReviewStatus: "pending"},
	}
	if err := m.TaskInit(dir, tasks); err != nil {
		t.Fatalf("TaskInit: %v", err)
	}

	if err := m.TaskUpdate(dir, "1", "reviewStatus", "completed_pass"); err != nil {
		t.Fatalf("TaskUpdate(reviewStatus): %v", err)
	}

	s := loadState(t, dir)
	if s.Tasks["1"].ReviewStatus != "completed_pass" {
		t.Errorf("reviewStatus: got %q, want %q", s.Tasks["1"].ReviewStatus, "completed_pass")
	}
	// implStatus should be untouched.
	if s.Tasks["1"].ImplStatus != "completed" {
		t.Errorf("implStatus: got %q, want %q (should be unchanged)", s.Tasks["1"].ImplStatus, "completed")
	}
}

func TestTaskUpdate_ImplRetries_ParsesInt(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	tasks := map[string]state.Task{
		"1": {Title: "T", ImplRetries: 0, ReviewRetries: 0},
	}
	if err := m.TaskInit(dir, tasks); err != nil {
		t.Fatalf("TaskInit: %v", err)
	}

	if err := m.TaskUpdate(dir, "1", "implRetries", "3"); err != nil {
		t.Fatalf("TaskUpdate(implRetries): %v", err)
	}

	s := loadState(t, dir)
	if s.Tasks["1"].ImplRetries != 3 {
		t.Errorf("implRetries: got %d, want 3", s.Tasks["1"].ImplRetries)
	}
	// reviewRetries should remain 0.
	if s.Tasks["1"].ReviewRetries != 0 {
		t.Errorf("reviewRetries: got %d, want 0 (should be unchanged)", s.Tasks["1"].ReviewRetries)
	}
}

func TestTaskUpdate_ReviewRetries_ParsesInt(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	tasks := map[string]state.Task{
		"1": {Title: "T"},
	}
	if err := m.TaskInit(dir, tasks); err != nil {
		t.Fatalf("TaskInit: %v", err)
	}

	if err := m.TaskUpdate(dir, "1", "reviewRetries", "2"); err != nil {
		t.Fatalf("TaskUpdate(reviewRetries): %v", err)
	}

	s := loadState(t, dir)
	if s.Tasks["1"].ReviewRetries != 2 {
		t.Errorf("reviewRetries: got %d, want 2", s.Tasks["1"].ReviewRetries)
	}
}

func TestTaskUpdate_ImplRetries_InvalidInt_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	tasks := map[string]state.Task{"1": {Title: "T"}}
	if err := m.TaskInit(dir, tasks); err != nil {
		t.Fatalf("TaskInit: %v", err)
	}

	if err := m.TaskUpdate(dir, "1", "implRetries", "not-a-number"); err == nil {
		t.Error("TaskUpdate(implRetries, not-a-number): expected error, got nil")
	}
}

func TestTaskUpdate_UnknownTask_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.TaskInit(dir, map[string]state.Task{"1": {Title: "T"}}); err != nil {
		t.Fatalf("TaskInit: %v", err)
	}

	if err := m.TaskUpdate(dir, "99", "implStatus", "completed"); err == nil {
		t.Error("TaskUpdate on unknown task: expected error, got nil")
	}
}

func TestTaskUpdate_UnknownField_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.TaskInit(dir, map[string]state.Task{"1": {Title: "T"}}); err != nil {
		t.Fatalf("TaskInit: %v", err)
	}

	if err := m.TaskUpdate(dir, "1", "unknownField", "value"); err == nil {
		t.Error("TaskUpdate with unknown field: expected error, got nil")
	}
}

// ---------- PhaseLog ----------

func TestPhaseLog_AppendsSingleEntry(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.PhaseLog(dir, "phase-1", 5000, 30000, "sonnet"); err != nil {
		t.Fatalf("PhaseLog: %v", err)
	}

	s := loadState(t, dir)
	if len(s.PhaseLog) != 1 {
		t.Fatalf("phaseLog length: got %d, want 1", len(s.PhaseLog))
	}
	entry := s.PhaseLog[0]
	if entry.Phase != "phase-1" {
		t.Errorf("phase: got %q, want %q", entry.Phase, "phase-1")
	}
	if entry.Tokens != 5000 {
		t.Errorf("tokens: got %d, want 5000", entry.Tokens)
	}
	if entry.DurationMs != 30000 {
		t.Errorf("duration_ms: got %d, want 30000", entry.DurationMs)
	}
	if entry.Model != "sonnet" {
		t.Errorf("model: got %q, want %q", entry.Model, "sonnet")
	}
	if entry.Timestamp == "" {
		t.Error("timestamp: want non-empty RFC3339 value")
	}
}

func TestPhaseLog_AppendsMultipleEntries(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	entries := []struct {
		phase      string
		tokens     int
		durationMs int
		model      string
	}{
		{"phase-1", 1000, 10000, "sonnet"},
		{"phase-2", 2000, 20000, "opus"},
		{"phase-3", 3000, 30000, "sonnet"},
	}

	for _, e := range entries {
		if err := m.PhaseLog(dir, e.phase, e.tokens, e.durationMs, e.model); err != nil {
			t.Fatalf("PhaseLog(%q): %v", e.phase, err)
		}
	}

	s := loadState(t, dir)
	if len(s.PhaseLog) != 3 {
		t.Fatalf("phaseLog length: got %d, want 3", len(s.PhaseLog))
	}
	for i, want := range entries {
		got := s.PhaseLog[i]
		if got.Phase != want.phase {
			t.Errorf("entry[%d].phase: got %q, want %q", i, got.Phase, want.phase)
		}
		if got.Tokens != want.tokens {
			t.Errorf("entry[%d].tokens: got %d, want %d", i, got.Tokens, want.tokens)
		}
		if got.DurationMs != want.durationMs {
			t.Errorf("entry[%d].duration_ms: got %d, want %d", i, got.DurationMs, want.durationMs)
		}
		if got.Model != want.model {
			t.Errorf("entry[%d].model: got %q, want %q", i, got.Model, want.model)
		}
		if got.Timestamp == "" {
			t.Errorf("entry[%d].timestamp: want non-empty", i)
		}
	}
}

// ---------- PhaseStats ----------

func TestPhaseStats_EmptyLog_ReturnsZeroes(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	result, err := m.PhaseStats(dir)
	if err != nil {
		t.Fatalf("PhaseStats: %v", err)
	}
	if result.TotalTokens != 0 {
		t.Errorf("totalTokens: got %d, want 0", result.TotalTokens)
	}
	if result.TotalDurationMs != 0 {
		t.Errorf("totalDurationMs: got %d, want 0", result.TotalDurationMs)
	}
	if len(result.Entries) != 0 {
		t.Errorf("entries: got %d, want 0", len(result.Entries))
	}
}

func TestPhaseStats_AggregatesCorrectly(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.PhaseLog(dir, "phase-1", 1000, 10000, "sonnet"); err != nil {
		t.Fatalf("PhaseLog 1: %v", err)
	}
	if err := m.PhaseLog(dir, "phase-2", 2000, 20000, "sonnet"); err != nil {
		t.Fatalf("PhaseLog 2: %v", err)
	}
	if err := m.PhaseLog(dir, "phase-3", 3000, 30000, "opus"); err != nil {
		t.Fatalf("PhaseLog 3: %v", err)
	}

	result, err := m.PhaseStats(dir)
	if err != nil {
		t.Fatalf("PhaseStats: %v", err)
	}

	if result.TotalTokens != 6000 {
		t.Errorf("totalTokens: got %d, want 6000", result.TotalTokens)
	}
	if result.TotalDurationMs != 60000 {
		t.Errorf("totalDurationMs: got %d, want 60000", result.TotalDurationMs)
	}
	if len(result.Entries) != 3 {
		t.Errorf("entries count: got %d, want 3", len(result.Entries))
	}
	// Verify per-phase data is present.
	if result.Entries[0].Phase != "phase-1" {
		t.Errorf("entries[0].phase: got %q, want %q", result.Entries[0].Phase, "phase-1")
	}
	if result.Entries[2].Model != "opus" {
		t.Errorf("entries[2].model: got %q, want %q", result.Entries[2].Model, "opus")
	}
}

// ---------- Golden-file test (AC-2) ----------

// TestInit_GoldenFile verifies that Init output matches testdata/state_init.json
// byte-for-byte after JSON round-trip, with dynamic fields (timestamps, workspace)
// normalized to stable placeholder values. Any schema drift — added, removed, or
// renamed JSON keys — will cause this test to fail.
func TestInit_GoldenFile(t *testing.T) {
	dir := t.TempDir()
	m := newManager()

	if err := m.Init(dir, "golden-spec"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Read produced state.json.
	rawProduced, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("read produced state.json: %v", err)
	}

	// Unmarshal both sides into State structs.
	var produced state.State
	if err := json.Unmarshal(rawProduced, &produced); err != nil {
		t.Fatalf("unmarshal produced: %v", err)
	}

	// Normalize dynamic fields before comparison.
	produced.Workspace = "WORKSPACE_PLACEHOLDER"
	produced.Timestamps.Created = "TIMESTAMP_PLACEHOLDER"
	produced.Timestamps.LastUpdated = "TIMESTAMP_PLACEHOLDER"

	// Re-marshal the normalized produced state.
	normalizedProduced, err := json.MarshalIndent(produced, "", "  ")
	if err != nil {
		t.Fatalf("marshal normalized produced: %v", err)
	}

	// Read the golden fixture.
	goldenPath := filepath.Join("..", "..", "testdata", "state_init.json")
	rawGolden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden fixture %q: %v", goldenPath, err)
	}

	// Unmarshal and re-marshal golden to canonicalize whitespace.
	var golden state.State
	if err := json.Unmarshal(rawGolden, &golden); err != nil {
		t.Fatalf("unmarshal golden: %v", err)
	}
	normalizedGolden, err := json.MarshalIndent(golden, "", "  ")
	if err != nil {
		t.Fatalf("marshal normalized golden: %v", err)
	}

	if string(normalizedProduced) != string(normalizedGolden) {
		t.Errorf("Init output does not match golden fixture.\nProduced:\n%s\n\nGolden:\n%s",
			normalizedProduced, normalizedGolden)
	}
}

// ---------- Exhaustive nextPhase test (AC-1) ----------

// TestNextPhase_ExhaustiveAllPhases verifies that PhaseComplete correctly
// advances through all 17 phases in ValidPhases in order.
// This exercises the internal nextPhase logic for every possible input.
//
// PhaseComplete advances currentPhase to phase[i+1]. When the next phase
// is "completed" (i.e., phase is "post-to-source" at index 15, or
// phase is "completed" at index 16), currentPhaseStatus is set to
// "completed" because the pipeline is terminal. For all other phases,
// currentPhaseStatus is "pending" and currentPhase is the next entry.
func TestNextPhase_ExhaustiveAllPhases(t *testing.T) {
	phases := state.ValidPhases // 18 entries

	for i, phase := range phases {
		t.Run(phase, func(t *testing.T) {
			dir := t.TempDir()
			m := newManager()
			if err := m.Init(dir, "s"); err != nil {
				t.Fatalf("Init: %v", err)
			}

			if err := m.PhaseComplete(dir, phase); err != nil {
				t.Fatalf("PhaseComplete(%q): %v", phase, err)
			}

			s := loadState(t, dir)

			// Determine the expected next phase.
			var wantNextPhase string
			if i < len(phases)-1 {
				wantNextPhase = phases[i+1]
			} else {
				wantNextPhase = "completed"
			}

			if s.CurrentPhase != wantNextPhase {
				t.Errorf("after PhaseComplete(%q): currentPhase = %q, want %q",
					phase, s.CurrentPhase, wantNextPhase)
			}

			// When the resulting currentPhase is "completed", the pipeline is
			// terminal and currentPhaseStatus must be "completed".
			// Otherwise currentPhaseStatus must be "pending".
			if wantNextPhase == "completed" {
				if s.CurrentPhaseStatus != "completed" {
					t.Errorf("after PhaseComplete(%q): currentPhaseStatus = %q, want %q",
						phase, s.CurrentPhaseStatus, "completed")
				}
			} else {
				if s.CurrentPhaseStatus != "pending" {
					t.Errorf("after PhaseComplete(%q): currentPhaseStatus = %q, want %q",
						phase, s.CurrentPhaseStatus, "pending")
				}
			}
		})
	}
}

// ---------- Concurrency test (AC-3) ----------

// TestPhaseLog_Concurrent10Goroutines spawns 10 goroutines that each call
// PhaseLog simultaneously. The test verifies that all entries are appended
// without data races (run with -race to enforce this).
func TestPhaseLog_Concurrent10Goroutines(t *testing.T) {
	const numGoroutines = 10

	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Use a WaitGroup to synchronize all goroutines.
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := range numGoroutines {
		go func(idx int) {
			defer wg.Done()
			phase := state.ValidPhases[idx%len(state.ValidPhases)]
			if err := m.PhaseLog(dir, phase, 1000*(idx+1), 500*(idx+1), "sonnet"); err != nil {
				// Cannot call t.Fatalf from a goroutine; use t.Errorf instead.
				t.Errorf("goroutine %d PhaseLog: %v", idx, err)
			}
		}(i)
	}

	wg.Wait()

	// After all goroutines complete, there should be exactly numGoroutines entries.
	s := loadState(t, dir)
	if len(s.PhaseLog) != numGoroutines {
		t.Errorf("phaseLog entries: got %d, want %d", len(s.PhaseLog), numGoroutines)
	}

	// Verify that total tokens equals the sum of 1000*1 + 1000*2 + ... + 1000*10 = 55000.
	totalTokens := 0
	for _, entry := range s.PhaseLog {
		totalTokens += entry.Tokens
	}
	if totalTokens != 55000 {
		t.Errorf("total tokens: got %d, want 55000", totalTokens)
	}
}

// ---------- SetBranch ----------

func TestSetBranch_SetsBranchField(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.SetBranch(dir, "feature/my-branch"); err != nil {
		t.Fatalf("SetBranch: %v", err)
	}

	s := loadState(t, dir)
	if s.Branch == nil {
		t.Fatal("branch: want non-nil")
	}
	if *s.Branch != "feature/my-branch" {
		t.Errorf("branch: got %q, want %q", *s.Branch, "feature/my-branch")
	}
}

func TestSetBranch_OverwritesPreviousBranch(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.SetBranch(dir, "feature/old"); err != nil {
		t.Fatalf("SetBranch(old): %v", err)
	}
	if err := m.SetBranch(dir, "feature/new"); err != nil {
		t.Fatalf("SetBranch(new): %v", err)
	}

	s := loadState(t, dir)
	if s.Branch == nil || *s.Branch != "feature/new" {
		t.Errorf("branch: got %v, want %q", s.Branch, "feature/new")
	}
}

// ---------- SetAutoApprove ----------

func TestSetAutoApprove_SetsAutoApproveTrue(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Verify initial value is false.
	s := loadState(t, dir)
	if s.AutoApprove {
		t.Error("autoApprove: initial value should be false")
	}

	if err := m.SetAutoApprove(dir); err != nil {
		t.Fatalf("SetAutoApprove: %v", err)
	}

	s = loadState(t, dir)
	if !s.AutoApprove {
		t.Error("autoApprove: got false, want true")
	}
}

func TestSetAutoApprove_PersistsToStateJSON(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.SetAutoApprove(dir); err != nil {
		t.Fatalf("SetAutoApprove: %v", err)
	}

	// Verify via Get that the field reads back correctly.
	got, err := m.Get(dir, "autoApprove")
	if err != nil {
		t.Fatalf("Get(autoApprove): %v", err)
	}
	if got != "true" {
		t.Errorf("Get(autoApprove): got %q, want %q", got, "true")
	}
}

// ---------- SetSkipPr ----------

func TestSetSkipPr_SetsSkipPrTrue(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Verify initial value is false.
	s := loadState(t, dir)
	if s.SkipPr {
		t.Error("skipPr: initial value should be false")
	}

	if err := m.SetSkipPr(dir); err != nil {
		t.Fatalf("SetSkipPr: %v", err)
	}

	s = loadState(t, dir)
	if !s.SkipPr {
		t.Error("skipPr: got false, want true")
	}
}

func TestSetSkipPr_PersistsToStateJSON(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.SetSkipPr(dir); err != nil {
		t.Fatalf("SetSkipPr: %v", err)
	}

	got, err := m.Get(dir, "skipPr")
	if err != nil {
		t.Fatalf("Get(skipPr): %v", err)
	}
	if got != "true" {
		t.Errorf("Get(skipPr): got %q, want %q", got, "true")
	}
}

// ---------- SetDebug ----------

func TestSetDebug_SetsDebugTrue(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Verify initial value is false.
	s := loadState(t, dir)
	if s.Debug {
		t.Error("debug: initial value should be false")
	}

	if err := m.SetDebug(dir); err != nil {
		t.Fatalf("SetDebug: %v", err)
	}

	s = loadState(t, dir)
	if !s.Debug {
		t.Error("debug: got false, want true")
	}
}

func TestSetDebug_PersistsToStateJSON(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.SetDebug(dir); err != nil {
		t.Fatalf("SetDebug: %v", err)
	}

	got, err := m.Get(dir, "debug")
	if err != nil {
		t.Fatalf("Get(debug): %v", err)
	}
	if got != "true" {
		t.Errorf("Get(debug): got %q, want %q", got, "true")
	}
}

// ---------- SetUseCurrentBranch ----------

func TestSetUseCurrentBranch_SetsFieldsCorrectly(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.SetUseCurrentBranch(dir, "feature/existing"); err != nil {
		t.Fatalf("SetUseCurrentBranch: %v", err)
	}

	s := loadState(t, dir)
	if !s.UseCurrentBranch {
		t.Error("useCurrentBranch: got false, want true")
	}
	if s.Branch == nil {
		t.Fatal("branch: want non-nil after SetUseCurrentBranch")
	}
	if *s.Branch != "feature/existing" {
		t.Errorf("branch: got %q, want %q", *s.Branch, "feature/existing")
	}
}

func TestSetUseCurrentBranch_PersistsToStateJSON(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.SetUseCurrentBranch(dir, "main"); err != nil {
		t.Fatalf("SetUseCurrentBranch: %v", err)
	}

	ucb, err := m.Get(dir, "useCurrentBranch")
	if err != nil {
		t.Fatalf("Get(useCurrentBranch): %v", err)
	}
	if ucb != "true" {
		t.Errorf("Get(useCurrentBranch): got %q, want %q", ucb, "true")
	}

	branch, err := m.Get(dir, "branch")
	if err != nil {
		t.Fatalf("Get(branch): %v", err)
	}
	if branch != "main" {
		t.Errorf("Get(branch): got %q, want %q", branch, "main")
	}
}

// ---------- ResumeInfo ----------

func TestResumeInfo_DefaultState(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	info, err := m.ResumeInfo(dir)
	if err != nil {
		t.Fatalf("ResumeInfo: %v", err)
	}

	if info.CurrentPhase != "phase-1" {
		t.Errorf("currentPhase: got %q, want %q", info.CurrentPhase, "phase-1")
	}
	if info.CurrentPhaseStatus != "pending" {
		t.Errorf("currentPhaseStatus: got %q, want %q", info.CurrentPhaseStatus, "pending")
	}
	if info.SpecName != "s" {
		t.Errorf("specName: got %q, want %q", info.SpecName, "s")
	}
	if info.AutoApprove {
		t.Error("autoApprove: want false")
	}
	if info.SkipPr {
		t.Error("skipPr: want false")
	}
	if info.Debug {
		t.Error("debug: want false")
	}
	if info.UseCurrentBranch {
		t.Error("useCurrentBranch: want false")
	}
	if info.TotalTasks != 0 {
		t.Errorf("totalTasks: got %d, want 0", info.TotalTasks)
	}
	if info.PhaseLogEntries != 0 {
		t.Errorf("phaseLogEntries: got %d, want 0", info.PhaseLogEntries)
	}
	if info.TotalTokens != 0 {
		t.Errorf("totalTokens: got %d, want 0", info.TotalTokens)
	}
	if info.TotalDurationMs != 0 {
		t.Errorf("totalDuration_ms: got %d, want 0", info.TotalDurationMs)
	}
	if info.CheckpointRevisionPending == nil {
		t.Fatal("checkpointRevisionPending: want non-nil")
	}
	if info.CheckpointRevisionPending["checkpoint-a"] {
		t.Error("checkpointRevisionPending[checkpoint-a]: want false")
	}
	if info.CheckpointRevisionPending["checkpoint-b"] {
		t.Error("checkpointRevisionPending[checkpoint-b]: want false")
	}
}

func TestResumeInfo_ReflectsSetAutoApprove(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.SetAutoApprove(dir); err != nil {
		t.Fatalf("SetAutoApprove: %v", err)
	}

	info, err := m.ResumeInfo(dir)
	if err != nil {
		t.Fatalf("ResumeInfo: %v", err)
	}
	if !info.AutoApprove {
		t.Error("autoApprove: want true after SetAutoApprove")
	}
}

func TestResumeInfo_PhaseLogEntriesCount(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.PhaseLog(dir, "phase-1", 1000, 5000, "sonnet"); err != nil {
		t.Fatalf("PhaseLog 1: %v", err)
	}
	if err := m.PhaseLog(dir, "phase-2", 2000, 10000, "sonnet"); err != nil {
		t.Fatalf("PhaseLog 2: %v", err)
	}

	info, err := m.ResumeInfo(dir)
	if err != nil {
		t.Fatalf("ResumeInfo: %v", err)
	}
	if info.PhaseLogEntries != 2 {
		t.Errorf("phaseLogEntries: got %d, want 2", info.PhaseLogEntries)
	}
	if info.TotalTokens != 3000 {
		t.Errorf("totalTokens: got %d, want 3000", info.TotalTokens)
	}
	if info.TotalDurationMs != 15000 {
		t.Errorf("totalDuration_ms: got %d, want 15000", info.TotalDurationMs)
	}
}

func TestResumeInfo_PendingAndCompletedTasks(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	tasks := map[string]state.Task{
		"1": {Title: "T1", ImplStatus: "completed", ReviewStatus: "completed_pass"},
		"2": {Title: "T2", ImplStatus: "pending", ReviewStatus: "pending"},
		"3": {Title: "T3", ImplStatus: "completed", ReviewStatus: "completed_fail"},
	}
	if err := m.TaskInit(dir, tasks); err != nil {
		t.Fatalf("TaskInit: %v", err)
	}

	info, err := m.ResumeInfo(dir)
	if err != nil {
		t.Fatalf("ResumeInfo: %v", err)
	}

	if info.TotalTasks != 3 {
		t.Errorf("totalTasks: got %d, want 3", info.TotalTasks)
	}
	// Task 1: implStatus=completed + reviewStatus=completed_pass → completed.
	// Task 2: implStatus=pending → pending.
	// Task 3: reviewStatus=completed_fail → pending (even though impl completed).
	if len(info.CompletedTasks) != 1 {
		t.Errorf("completedTasks: got %d, want 1", len(info.CompletedTasks))
	}
	if len(info.PendingTasks) != 2 {
		t.Errorf("pendingTasks: got %d, want 2", len(info.PendingTasks))
	}
}

func TestResumeInfo_TasksWithRetries(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	tasks := map[string]state.Task{
		"1": {Title: "T1", ImplRetries: 2, ReviewRetries: 0},
		"2": {Title: "T2", ImplRetries: 0, ReviewRetries: 1},
		"3": {Title: "T3", ImplRetries: 0, ReviewRetries: 0},
	}
	if err := m.TaskInit(dir, tasks); err != nil {
		t.Fatalf("TaskInit: %v", err)
	}

	info, err := m.ResumeInfo(dir)
	if err != nil {
		t.Fatalf("ResumeInfo: %v", err)
	}

	// Tasks 1 and 2 have retries; task 3 does not.
	if len(info.TasksWithRetries) != 2 {
		t.Errorf("tasksWithRetries: got %d entries, want 2", len(info.TasksWithRetries))
	}
}

func TestResumeInfo_MissingStateFile_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	m := newManager()

	_, err := m.ResumeInfo(dir)
	if err == nil {
		t.Error("ResumeInfo on missing state.json: expected error, got nil")
	}
}

// ---------- RefreshIndex ----------

// TestRefreshIndex_ErrorWhenScriptNotFound verifies that RefreshIndex returns
// a non-nil error gracefully when indexer.BuildSpecsIndex is not wired in this package.
// The current implementation delegates to the tools package (which calls indexer.BuildSpecsIndex)
// and returns a "not implemented" error, which satisfies the contract of failing gracefully.
func TestRefreshIndex_ErrorWhenScriptNotFound(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// RefreshIndex is intentionally not implemented in the state package
	// (delegated to tools.RefreshIndexHandler, which calls indexer.BuildSpecsIndex). It must return
	// a non-nil error rather than panic or silently succeed.
	err := m.RefreshIndex(dir)
	if err == nil {
		t.Error("RefreshIndex: expected non-nil error when script not in state package, got nil")
	}
}

// ---------- LoadFromFile ----------

// TestLoadFromFile_PopulatesCache verifies that after calling LoadFromFile,
// GetState() returns the loaded state (cache is warmed, Version == 2).
func TestLoadFromFile_PopulatesCache(t *testing.T) {
	dir := t.TempDir()

	// Use a separate manager to seed the workspace via Init.
	seeder := newManager()
	if err := seeder.Init(dir, "spec-cache"); err != nil {
		t.Fatalf("Init (seeder): %v", err)
	}

	// Now create a new manager and load from the file written above.
	m := newManager()
	if err := m.LoadFromFile(dir); err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}

	s, err := m.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if s == nil {
		t.Fatal("GetState: returned nil, want non-nil state")
	}
	if s.Version != 2 {
		t.Errorf("Version: got %d, want 2", s.Version)
	}
	if s.SpecName != "spec-cache" {
		t.Errorf("SpecName: got %q, want %q", s.SpecName, "spec-cache")
	}
}

// TestLoadFromFile_WorkspaceMismatch verifies that calling a mutating method
// with a different workspace after LoadFromFile returns a workspace-mismatch error.
func TestLoadFromFile_WorkspaceMismatch(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	// Seed both workspaces.
	seeder1 := newManager()
	if err := seeder1.Init(dir1, "spec-ws1"); err != nil {
		t.Fatalf("Init (ws1): %v", err)
	}
	seeder2 := newManager()
	if err := seeder2.Init(dir2, "spec-ws2"); err != nil {
		t.Fatalf("Init (ws2): %v", err)
	}

	// Bind manager to dir1 via LoadFromFile.
	m := newManager()
	if err := m.LoadFromFile(dir1); err != nil {
		t.Fatalf("LoadFromFile(dir1): %v", err)
	}

	// Attempt to call PhaseStart with dir2 — should fail with workspace mismatch.
	err := m.PhaseStart(dir2, "phase-1")
	if err == nil {
		t.Fatal("PhaseStart with different workspace: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "workspace mismatch") {
		t.Errorf("error message: got %q, want it to contain %q", err.Error(), "workspace mismatch")
	}
}

// TestLoadFromFile_MigratesV1File verifies that LoadFromFile on a v1 JSON file
// returns state with Version == 2 (auto-migration is applied).
func TestLoadFromFile_MigratesV1File(t *testing.T) {
	dir := t.TempDir()

	// Write a minimal v1 state.json directly to the workspace.
	v1JSON := `{
  "version": 1,
  "specName": "v1-spec",
  "workspace": "` + dir + `",
  "branch": null,
  "taskType": null,
  "effort": null,
  "flowTemplate": null,
  "autoApprove": false,
  "skipPr": false,
  "useCurrentBranch": false,
  "debug": false,
  "skippedPhases": [],
  "currentPhase": "phase-1",
  "currentPhaseStatus": "pending",
  "completedPhases": ["setup"],
  "revisions": {"designRevisions": 0, "taskRevisions": 0, "designInlineRevisions": 0, "taskInlineRevisions": 0},
  "checkpointRevisionPending": {"checkpoint-a": false, "checkpoint-b": false},
  "tasks": {},
  "phaseLog": [],
  "timestamps": {"created": "2024-01-01T00:00:00Z", "lastUpdated": "2024-01-01T00:00:00Z", "phaseStarted": null},
  "error": null
}`
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(v1JSON), 0o600); err != nil {
		t.Fatalf("WriteFile v1 state: %v", err)
	}

	m := newManager()
	if err := m.LoadFromFile(dir); err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}

	s, err := m.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if s.Version != 2 {
		t.Errorf("Version after migration: got %d, want 2", s.Version)
	}
}

// ---------- Update ----------

// TestUpdate_PersistsToDisk verifies that after Update mutates a field,
// reading the JSON file from disk reflects the change.
func TestUpdate_PersistsToDisk(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Mutate SpecName via Update.
	if err := m.Update(func(s *state.State) error {
		s.SpecName = "updated-spec"
		return nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Read the file directly from disk and verify the change.
	data, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var disk state.State
	if err := json.Unmarshal(data, &disk); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if disk.SpecName != "updated-spec" {
		t.Errorf("SpecName on disk: got %q, want %q", disk.SpecName, "updated-spec")
	}
}

// TestUpdate_StampsLastUpdated verifies that Update stamps Timestamps.LastUpdated
// with RFC3339Nano precision and that the stamped time is strictly after a
// reference time captured before the Update call.
func TestUpdate_StampsLastUpdated(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Capture the LastUpdated set by Init as the "before" reference.
	s0, err := m.GetState()
	if err != nil {
		t.Fatalf("GetState (before): %v", err)
	}
	before, err := time.Parse(time.RFC3339Nano, s0.Timestamps.LastUpdated)
	if err != nil {
		t.Fatalf("parse before LastUpdated %q: %v", s0.Timestamps.LastUpdated, err)
	}

	// Sleep a nanosecond to ensure strict ordering.
	time.Sleep(time.Nanosecond)

	// Call Update with a no-op mutation to trigger the timestamp stamp.
	if err := m.Update(func(s *state.State) error {
		s.Debug = true
		return nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	s1, err := m.GetState()
	if err != nil {
		t.Fatalf("GetState (after): %v", err)
	}
	after, err := time.Parse(time.RFC3339Nano, s1.Timestamps.LastUpdated)
	if err != nil {
		t.Fatalf("parse after LastUpdated %q: %v", s1.Timestamps.LastUpdated, err)
	}

	if !after.After(before) {
		t.Errorf("LastUpdated not strictly advanced: before=%v after=%v", before, after)
	}
}

// TestUpdate_ConcurrentSafe spawns 50 goroutines that each append a PhaseLogEntry
// via Update. The test verifies no data races (run with -race) and that the
// final count is exactly 50.
func TestUpdate_ConcurrentSafe(t *testing.T) {
	const numGoroutines = 50

	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Use a buffered channel to collect errors from goroutines safely.
	// Calling t.Errorf directly from a goroutine can panic if the goroutine
	// outlives the test (e.g., after a t.Fatalf in the main goroutine).
	errs := make(chan error, numGoroutines)

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := range numGoroutines {
		go func(idx int) {
			defer wg.Done()
			if err := m.Update(func(s *state.State) error {
				s.PhaseLog = append(s.PhaseLog, state.PhaseLogEntry{
					Phase:      "phase-1",
					Tokens:     idx + 1,
					DurationMs: (idx + 1) * 100,
					Model:      "sonnet",
					Timestamp:  "2024-01-01T00:00:00Z",
				})
				return nil
			}); err != nil {
				errs <- fmt.Errorf("goroutine %d Update: %w", idx, err)
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}

	s, err := m.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if len(s.PhaseLog) != numGoroutines {
		t.Errorf("PhaseLog entries: got %d, want %d", len(s.PhaseLog), numGoroutines)
	}
}

// TestUpdate_PersistFailureReturnsError verifies that Update returns a non-nil
// error when the disk write fails (directory placed at state.json path), and
// that sm.state is NOT mutated — the in-memory cache is only replaced after a
// successful write.
func TestUpdate_PersistFailureReturnsError(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	original, err := m.GetState()
	if err != nil {
		t.Fatalf("GetState before failure: %v", err)
	}
	originalSpecName := original.SpecName

	statePath := filepath.Join(dir, "state.json")

	// Replace state.json with a directory of the same name — this makes
	// os.WriteFile (or os.Rename) fail regardless of whether the process is root.
	if err := os.Remove(statePath); err != nil {
		t.Fatalf("Remove state.json: %v", err)
	}
	if err := os.Mkdir(statePath, 0o755); err != nil {
		t.Fatalf("Mkdir at state.json path: %v", err)
	}

	// Call Update — the mutation itself should succeed, but the disk write must fail.
	updateErr := m.Update(func(s *state.State) error {
		s.SpecName = "mutated"
		return nil
	})
	if updateErr == nil {
		t.Fatal("Update with unpersistable path: expected error, got nil")
	}

	// In-memory state must be unchanged — Update rolls back on persist failure.
	s, err := m.GetState()
	if err != nil {
		t.Fatalf("GetState after failed persist: %v", err)
	}
	if s.SpecName != originalSpecName {
		t.Errorf("SpecName in memory: got %q, want %q (state must not be mutated on persist failure)", s.SpecName, originalSpecName)
	}
}

// ---------- GetState ----------

// TestGetState_ReturnsCopy verifies that mutating a field of the returned copy
// does not affect subsequent GetState() calls.
func TestGetState_ReturnsCopy(t *testing.T) {
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "spec-copy"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Get the first copy.
	copy1, err := m.GetState()
	if err != nil {
		t.Fatalf("GetState (first): %v", err)
	}

	// Mutate the returned copy.
	copy1.SpecName = "mutated-copy"
	copy1.AutoApprove = true

	// Get a second copy — should not reflect the mutations made to copy1.
	copy2, err := m.GetState()
	if err != nil {
		t.Fatalf("GetState (second): %v", err)
	}
	if copy2.SpecName != "spec-copy" {
		t.Errorf("SpecName: got %q, want %q (mutation of copy1 should not affect sm.state)", copy2.SpecName, "spec-copy")
	}
	if copy2.AutoApprove != false {
		t.Errorf("AutoApprove: got %v, want false (mutation of copy1 should not affect sm.state)", copy2.AutoApprove)
	}
}

// ---------- Unbound manager ----------

// TestUnboundManager_AutoBindsOnFirstCall verifies that a NewStateManager()
// with no prior Init or LoadFromFile auto-binds to a workspace on the first
// mutating method call, and that a subsequent GetState() returns non-nil state.
func TestUnboundManager_AutoBindsOnFirstCall(t *testing.T) {
	dir := t.TempDir()

	// Seed a valid state.json using a separate manager (simulates a pre-existing workspace).
	seeder := newManager()
	if err := seeder.Init(dir, "spec-autobind"); err != nil {
		t.Fatalf("Init (seeder): %v", err)
	}

	// Create an unbound manager — no Init or LoadFromFile called.
	m := newManager()

	// First mutating call: should auto-bind to dir and succeed.
	if err := m.PhaseStart(dir, "phase-1"); err != nil {
		t.Fatalf("PhaseStart on unbound manager: %v", err)
	}

	// GetState() (no workspace argument) should now return non-nil state.
	s, err := m.GetState()
	if err != nil {
		t.Fatalf("GetState after auto-bind: %v", err)
	}
	if s == nil {
		t.Fatal("GetState after auto-bind: returned nil, want non-nil")
	}
}

// ---------- Configure ----------

func TestConfigure_AppliesAllFieldsInOneWrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "cfg-test"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	cfg := state.PipelineConfig{
		Effort:           "S",
		FlowTemplate:     "standard",
		AutoApprove:      true,
		SkipPR:           true,
		Debug:            true,
		UseCurrentBranch: true,
		Branch:           "feature/my-fix",
		SkippedPhases:    []string{"phase-3b"},
	}
	if err := m.Configure(dir, cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	s := loadState(t, dir)
	if s.Effort == nil || *s.Effort != "S" {
		t.Errorf("Effort = %v, want %q", s.Effort, "S")
	}
	if s.FlowTemplate == nil || *s.FlowTemplate != "standard" {
		t.Errorf("FlowTemplate = %v, want %q", s.FlowTemplate, "standard")
	}
	if !s.AutoApprove {
		t.Error("AutoApprove = false, want true")
	}
	if !s.SkipPr {
		t.Error("SkipPr = false, want true")
	}
	if !s.Debug {
		t.Error("Debug = false, want true")
	}
	if !s.UseCurrentBranch {
		t.Error("UseCurrentBranch = false, want true")
	}
	if s.Branch == nil || *s.Branch != "feature/my-fix" {
		t.Errorf("Branch = %v, want %q", s.Branch, "feature/my-fix")
	}
	found := false
	for _, p := range s.SkippedPhases {
		if p == "phase-3b" {
			found = true
		}
	}
	if !found {
		t.Errorf("SkippedPhases = %v, want to contain %q", s.SkippedPhases, "phase-3b")
	}
}

func TestConfigure_InvalidEffort_ReturnsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	err := m.Configure(dir, state.PipelineConfig{
		Effort:       "INVALID",
		FlowTemplate: "standard",
	})
	if err == nil {
		t.Fatal("Configure: want error for invalid effort, got nil")
	}
	if !strings.Contains(err.Error(), "INVALID") {
		t.Errorf("error %q does not mention invalid value", err.Error())
	}
}

func TestConfigure_InvalidFlowTemplate_ReturnsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	err := m.Configure(dir, state.PipelineConfig{
		Effort:       "M",
		FlowTemplate: "bad-template",
	})
	if err == nil {
		t.Fatal("Configure: want error for invalid flowTemplate, got nil")
	}
	if !strings.Contains(err.Error(), "bad-template") {
		t.Errorf("error %q does not mention invalid value", err.Error())
	}
}

func TestConfigure_InvalidPhase_ReturnsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	m := newManager()
	if err := m.Init(dir, "s"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	err := m.Configure(dir, state.PipelineConfig{
		Effort:        "M",
		FlowTemplate:  "standard",
		SkippedPhases: []string{"not-a-phase"},
	})
	if err == nil {
		t.Fatal("Configure: want error for invalid phase, got nil")
	}
	if !strings.Contains(err.Error(), "not-a-phase") {
		t.Errorf("error %q does not mention invalid value", err.Error())
	}
}

func TestConfigure_SkippedPhasesDoNotAdvanceCurrentPhaseUnlessLanding(t *testing.T) {
	t.Parallel()

	t.Run("skip non-current phase keeps currentPhase at phase-1", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		m := newManager()
		if err := m.Init(dir, "s"); err != nil {
			t.Fatalf("Init: %v", err)
		}
		if err := m.Configure(dir, state.PipelineConfig{
			Effort:        "M",
			FlowTemplate:  "standard",
			SkippedPhases: []string{"phase-3b"},
		}); err != nil {
			t.Fatalf("Configure: %v", err)
		}
		s := loadState(t, dir)
		if s.CurrentPhase != "phase-1" {
			t.Errorf("CurrentPhase = %q, want %q (phase-3b is not the current phase)", s.CurrentPhase, "phase-1")
		}
	})

	t.Run("skip current phase advances to next non-skipped", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		m := newManager()
		if err := m.Init(dir, "s"); err != nil {
			t.Fatalf("Init: %v", err)
		}
		if err := m.Configure(dir, state.PipelineConfig{
			Effort:        "M",
			FlowTemplate:  "standard",
			SkippedPhases: []string{"phase-1", "phase-2"},
		}); err != nil {
			t.Fatalf("Configure: %v", err)
		}
		s := loadState(t, dir)
		if s.CurrentPhase != "phase-3" {
			t.Errorf("CurrentPhase = %q, want %q (phase-1 and phase-2 skipped)", s.CurrentPhase, "phase-3")
		}
	})

	t.Run("multiple non-contiguous skips keep currentPhase at phase-1", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		m := newManager()
		if err := m.Init(dir, "s"); err != nil {
			t.Fatalf("Init: %v", err)
		}
		// Reproducer for the original bug: light flow S skips
		// [phase-4b, checkpoint-b, phase-7].
		// None of these are the initial phase-1, so currentPhase must stay at phase-1.
		if err := m.Configure(dir, state.PipelineConfig{
			Effort:        "S",
			FlowTemplate:  "light",
			SkippedPhases: []string{"phase-4b", "checkpoint-b", "phase-7"},
		}); err != nil {
			t.Fatalf("Configure: %v", err)
		}
		s := loadState(t, dir)
		if s.CurrentPhase != "phase-1" {
			t.Errorf("CurrentPhase = %q, want %q (none of the skipped phases are the initial phase)", s.CurrentPhase, "phase-1")
		}
	})
}

// ---------- helper ----------

// containsAny returns true if s contains any of the given substrings.
func containsAny(s string, substrings ...string) bool {
	for _, sub := range substrings {
		if len(sub) > 0 && strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

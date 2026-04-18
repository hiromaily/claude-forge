// Package analytics_test contains external tests for the analytics package.
package analytics_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/state"
	"github.com/hiromaily/claude-forge/mcp-server/internal/intelligence/analytics"
)

// writeStateStruct writes a state.State as state.json in dir.
func writeStateStruct(t *testing.T, dir string, s state.State) {
	t.Helper()

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		t.Fatalf("writeStateStruct marshal: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "state.json"), data, 0o600); err != nil {
		t.Fatalf("writeStateStruct write: %v", err)
	}
}

// TestNewCollector verifies NewCollector returns a non-nil Collector
// and that no exported fields exist on Collector.
func TestNewCollector(t *testing.T) {
	t.Parallel()

	col := analytics.NewCollector("/tmp/specs")
	if col == nil {
		t.Fatal("NewCollector returned nil")
	}
}

// TestCollect_BasicAggregation verifies that Collect sums PhaseLog entries,
// sets EstimatedCostUSD, counts phases, and sums retries.
func TestCollect_BasicAggregation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	effort := "M"
	flowTemplate := "standard"

	s := state.State{
		SpecName:        "test-collect",
		Workspace:       dir,
		Effort:          &effort,
		FlowTemplate:    &flowTemplate,
		CompletedPhases: []string{"phase-1", "phase-2", "phase-3"},
		SkippedPhases:   []string{"phase-3b"},
		PhaseLog: []state.PhaseLogEntry{
			{Phase: "phase-1", Tokens: 1000, DurationMs: 5000, Model: "sonnet"},
			{Phase: "phase-2", Tokens: 2000, DurationMs: 10000, Model: "sonnet"},
			{Phase: "phase-3", Tokens: 500, DurationMs: 3000, Model: "sonnet"},
		},
		Tasks: map[string]state.Task{
			"1": {Title: "Task 1", ImplRetries: 1, ReviewRetries: 2},
			"2": {Title: "Task 2", ImplRetries: 0, ReviewRetries: 1},
		},
	}

	writeStateStruct(t, dir, s)

	col := analytics.NewCollector("/tmp/specs")
	summary, err := col.Collect(dir)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if summary.TotalTokens != 3500 {
		t.Errorf("TotalTokens = %d, want 3500", summary.TotalTokens)
	}

	if summary.TotalDurationMs != 18000 {
		t.Errorf("TotalDurationMs = %d, want 18000", summary.TotalDurationMs)
	}

	if summary.TotalDuration != "18s" {
		t.Errorf("TotalDuration = %q, want %q", summary.TotalDuration, "18s")
	}

	wantCost := float64(3500) * 0.000006
	if summary.EstimatedCostUSD != wantCost {
		t.Errorf("EstimatedCostUSD = %f, want %f", summary.EstimatedCostUSD, wantCost)
	}

	if summary.PhasesExecuted != 3 {
		t.Errorf("PhasesExecuted = %d, want 3", summary.PhasesExecuted)
	}

	if summary.PhasesSkipped != 1 {
		t.Errorf("PhasesSkipped = %d, want 1", summary.PhasesSkipped)
	}

	// Retries: 1+2 + 0+1 = 4
	if summary.Retries != 4 {
		t.Errorf("Retries = %d, want 4", summary.Retries)
	}

	if summary.Effort != "M" {
		t.Errorf("Effort = %q, want %q", summary.Effort, "M")
	}

	if summary.FlowTemplate != "standard" {
		t.Errorf("FlowTemplate = %q, want %q", summary.FlowTemplate, "standard")
	}
}

// TestCollect_NilPointerFields verifies that nil pointer fields (*string) produce
// empty-string fallbacks in PipelineSummary.
func TestCollect_NilPointerFields(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	s := state.State{
		SpecName:        "nil-fields",
		Workspace:       dir,
		Effort:          nil,
		FlowTemplate:    nil,
		CompletedPhases: []string{},
		SkippedPhases:   []string{},
	}

	writeStateStruct(t, dir, s)

	col := analytics.NewCollector("/tmp/specs")
	summary, err := col.Collect(dir)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if summary.Effort != "" {
		t.Errorf("Effort = %q, want empty string", summary.Effort)
	}

	if summary.FlowTemplate != "" {
		t.Errorf("FlowTemplate = %q, want empty string", summary.FlowTemplate)
	}
}

// TestCollect_MissingReviewFiles verifies that absent review files produce zero findings,
// not an error.
func TestCollect_MissingReviewFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	s := state.State{
		SpecName:        "no-reviews",
		Workspace:       dir,
		CompletedPhases: []string{"phase-1"},
	}

	writeStateStruct(t, dir, s)

	col := analytics.NewCollector("/tmp/specs")
	summary, err := col.Collect(dir)
	if err != nil {
		t.Fatalf("Collect: unexpected error for missing review files: %v", err)
	}

	if summary.ReviewFindings.Critical != 0 {
		t.Errorf("ReviewFindings.Critical = %d, want 0", summary.ReviewFindings.Critical)
	}

	if summary.ReviewFindings.Minor != 0 {
		t.Errorf("ReviewFindings.Minor = %d, want 0", summary.ReviewFindings.Minor)
	}
}

// TestCollect_WithReviewFindings verifies that existing review files are parsed
// and findings are counted correctly.
func TestCollect_WithReviewFindings(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	s := state.State{
		SpecName:        "with-reviews",
		Workspace:       dir,
		CompletedPhases: []string{"phase-3"},
	}

	writeStateStruct(t, dir, s)

	// Write a review-design.md with findings.
	reviewContent := `## Verdict: APPROVE_WITH_NOTES

**1. [CRITICAL] Missing error handling in function X**
**2. [MINOR] Variable name could be clearer**
**3. [MINOR] Add comment to exported function**
`
	if err := os.WriteFile(filepath.Join(dir, "review-design.md"), []byte(reviewContent), 0o600); err != nil {
		t.Fatalf("write review-design.md: %v", err)
	}

	col := analytics.NewCollector("/tmp/specs")
	summary, err := col.Collect(dir)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if summary.ReviewFindings.Critical != 1 {
		t.Errorf("ReviewFindings.Critical = %d, want 1", summary.ReviewFindings.Critical)
	}

	if summary.ReviewFindings.Minor != 2 {
		t.Errorf("ReviewFindings.Minor = %d, want 2", summary.ReviewFindings.Minor)
	}
}

// TestCollect_EmptyPhaseLog verifies that an empty PhaseLog produces zero tokens and duration.
func TestCollect_EmptyPhaseLog(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	s := state.State{
		SpecName:        "empty-log",
		Workspace:       dir,
		CompletedPhases: []string{},
		SkippedPhases:   []string{},
		PhaseLog:        []state.PhaseLogEntry{},
	}

	writeStateStruct(t, dir, s)

	col := analytics.NewCollector("/tmp/specs")
	summary, err := col.Collect(dir)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if summary.TotalTokens != 0 {
		t.Errorf("TotalTokens = %d, want 0", summary.TotalTokens)
	}

	if summary.TotalDurationMs != 0 {
		t.Errorf("TotalDurationMs = %d, want 0", summary.TotalDurationMs)
	}

	if summary.TotalDuration != "0s" {
		t.Errorf("TotalDuration = %q, want %q", summary.TotalDuration, "0s")
	}

	if summary.EstimatedCostUSD != 0 {
		t.Errorf("EstimatedCostUSD = %f, want 0", summary.EstimatedCostUSD)
	}
}

// TestCollect_PipelineField verifies that the Pipeline field is set to the spec name.
func TestCollect_PipelineField(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	s := state.State{
		SpecName:  "my-test-spec",
		Workspace: dir,
	}

	writeStateStruct(t, dir, s)

	col := analytics.NewCollector("/tmp/specs")
	summary, err := col.Collect(dir)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if summary.Pipeline != "my-test-spec" {
		t.Errorf("Pipeline = %q, want %q", summary.Pipeline, "my-test-spec")
	}
}

// TestCollect_MissingStateJSON verifies that a missing state.json returns an error.
func TestCollect_MissingStateJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Do NOT write state.json

	col := analytics.NewCollector("/tmp/specs")
	_, err := col.Collect(dir)
	if err == nil {
		t.Fatal("Collect: expected error for missing state.json, got nil")
	}
}

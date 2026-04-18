// Package analytics_test contains smoke tests for the Reporter type.
package analytics_test

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/internal/intelligence/analytics"
	"github.com/hiromaily/claude-forge/mcp-server/internal/intelligence/history"
)

// writeStateJSON writes a minimal state.json fixture for testing.
func writeStateJSON(t *testing.T, dir string, state map[string]any) {
	t.Helper()
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal state fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), data, 0o600); err != nil {
		t.Fatalf("write state.json: %v", err)
	}
}

// completedState returns a minimal completed pipeline state map.
// taskRevisions is always 0; only designRevisions varies across tests.
func completedState(effort, flowTemplate string, designRev int) map[string]any {
	return map[string]any{
		"specName":                  "test-spec",
		"currentPhase":              "completed",
		"effort":                    effort,
		"flowTemplate":              flowTemplate,
		"completedPhases":           []string{"phase-1", "phase-2"},
		"skippedPhases":             []string{},
		"revisions":                 map[string]any{"designRevisions": designRev, "taskRevisions": 0},
		"tasks":                     map[string]any{},
		"phaseLog":                  []any{},
		"checkpointRevisionPending": map[string]any{},
		"timestamps":                map[string]any{},
	}
}

// abandonedState returns a minimal abandoned pipeline state map.
func abandonedState() map[string]any {
	return map[string]any{
		"specName":                  "abandoned-spec",
		"currentPhase":              "abandoned",
		"effort":                    "M",
		"flowTemplate":              "standard",
		"completedPhases":           []string{},
		"skippedPhases":             []string{},
		"revisions":                 map[string]any{"designRevisions": 0, "taskRevisions": 0},
		"tasks":                     map[string]any{},
		"phaseLog":                  []any{},
		"checkpointRevisionPending": map[string]any{},
		"timestamps":                map[string]any{},
	}
}

// TestDashboard_Empty verifies that an empty specs directory produces a
// zero-value RepoDashboard without panicking.
func TestDashboard_Empty(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()
	rep := analytics.NewReporter(specsDir, nil)

	dash, err := rep.Dashboard()
	if err != nil {
		t.Fatalf("Dashboard() unexpected error: %v", err)
	}

	if dash.TotalPipelines != 0 {
		t.Errorf("TotalPipelines = %d, want 0", dash.TotalPipelines)
	}

	if dash.Completed != 0 {
		t.Errorf("Completed = %d, want 0", dash.Completed)
	}

	if dash.Abandoned != 0 {
		t.Errorf("Abandoned = %d, want 0", dash.Abandoned)
	}

	if dash.MostCommonFindings == nil {
		t.Error("MostCommonFindings should be non-nil empty slice, got nil")
	}
}

// TestDashboard_MixedPipelines verifies counts, ReviewPassRate, and
// AvgRetriesPerPipeline for a mix of completed and abandoned pipelines.
func TestDashboard_MixedPipelines(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()

	// Two completed pipelines: one with zero revisions (passes), one with revisions (fails).
	spec1 := filepath.Join(specsDir, "spec-pass")
	if err := os.Mkdir(spec1, 0o750); err != nil {
		t.Fatalf("mkdir spec-pass: %v", err)
	}
	writeStateJSON(t, spec1, completedState("M", "standard", 0))

	spec2 := filepath.Join(specsDir, "spec-fail")
	if err := os.Mkdir(spec2, 0o750); err != nil {
		t.Fatalf("mkdir spec-fail: %v", err)
	}
	writeStateJSON(t, spec2, completedState("S", "lite", 1))

	spec3 := filepath.Join(specsDir, "spec-abandoned")
	if err := os.Mkdir(spec3, 0o750); err != nil {
		t.Fatalf("mkdir spec-abandoned: %v", err)
	}
	writeStateJSON(t, spec3, abandonedState())

	rep := analytics.NewReporter(specsDir, nil)

	dash, err := rep.Dashboard()
	if err != nil {
		t.Fatalf("Dashboard() unexpected error: %v", err)
	}

	if dash.TotalPipelines != 3 {
		t.Errorf("TotalPipelines = %d, want 3", dash.TotalPipelines)
	}

	if dash.Completed != 2 {
		t.Errorf("Completed = %d, want 2", dash.Completed)
	}

	if dash.Abandoned != 1 {
		t.Errorf("Abandoned = %d, want 1", dash.Abandoned)
	}

	// ReviewPassRate: 1 of 2 completed pipelines has zero revisions → 0.5
	wantPassRate := 0.5
	if dash.ReviewPassRate != wantPassRate {
		t.Errorf("ReviewPassRate = %f, want %f", dash.ReviewPassRate, wantPassRate)
	}
}

// TestDashboard_NilKnowledgeBase verifies that a nil KnowledgeBase does not
// cause a panic and produces an empty MostCommonFindings slice.
func TestDashboard_NilKnowledgeBase(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()
	rep := analytics.NewReporter(specsDir, nil)

	dash, err := rep.Dashboard()
	if err != nil {
		t.Fatalf("Dashboard() unexpected error: %v", err)
	}

	if dash.MostCommonFindings == nil {
		t.Error("MostCommonFindings should be non-nil empty slice, got nil")
	}

	if len(dash.MostCommonFindings) != 0 {
		t.Errorf("MostCommonFindings len = %d, want 0", len(dash.MostCommonFindings))
	}
}

// TestDashboard_NonNilKnowledgeBase verifies that a non-nil KnowledgeBase
// with no patterns still produces an empty MostCommonFindings slice.
func TestDashboard_NonNilKnowledgeBase(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()
	kb := history.NewKnowledgeBase(specsDir)
	rep := analytics.NewReporter(specsDir, kb)

	dash, err := rep.Dashboard()
	if err != nil {
		t.Fatalf("Dashboard() unexpected error: %v", err)
	}

	if dash.MostCommonFindings == nil {
		t.Error("MostCommonFindings should be non-nil empty slice, got nil")
	}
}

// TestDashboard_ByFlowTemplate verifies that ByFlowTemplate is populated.
func TestDashboard_ByFlowTemplate(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()

	spec1 := filepath.Join(specsDir, "spec-standard")
	if err := os.Mkdir(spec1, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeStateJSON(t, spec1, completedState("M", "standard", 0))

	spec2 := filepath.Join(specsDir, "spec-lite")
	if err := os.Mkdir(spec2, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeStateJSON(t, spec2, completedState("S", "lite", 0))

	rep := analytics.NewReporter(specsDir, nil)

	dash, err := rep.Dashboard()
	if err != nil {
		t.Fatalf("Dashboard() unexpected error: %v", err)
	}

	if _, ok := dash.ByFlowTemplate["standard"]; !ok {
		t.Error("ByFlowTemplate missing 'standard'")
	}

	if _, ok := dash.ByFlowTemplate["lite"]; !ok {
		t.Error("ByFlowTemplate missing 'lite'")
	}
}

// TestDashboard_TotalTokensAndCost verifies that TotalTokens and
// EstimatedTotalCostUSD are aggregated from PhaseLog entries.
func TestDashboard_TotalTokensAndCost(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()

	spec1 := filepath.Join(specsDir, "spec-with-tokens")
	if err := os.Mkdir(spec1, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	st := completedState("M", "standard", 0)
	st["phaseLog"] = []any{
		map[string]any{
			"phase":       "phase-1",
			"tokens":      1000,
			"duration_ms": 5000,
			"model":       "sonnet",
			"timestamp":   "2024-01-01T00:00:00Z",
		},
		map[string]any{
			"phase":       "phase-2",
			"tokens":      2000,
			"duration_ms": 10000,
			"model":       "sonnet",
			"timestamp":   "2024-01-01T00:01:00Z",
		},
	}
	writeStateJSON(t, spec1, st)

	rep := analytics.NewReporter(specsDir, nil)

	dash, err := rep.Dashboard()
	if err != nil {
		t.Fatalf("Dashboard() unexpected error: %v", err)
	}

	if dash.TotalTokens != 3000 {
		t.Errorf("TotalTokens = %d, want 3000", dash.TotalTokens)
	}

	wantCost := float64(3000) * 0.000006
	if math.Abs(dash.EstimatedTotalCostUSD-wantCost) > 1e-9 {
		t.Errorf("EstimatedTotalCostUSD = %v, want %v", dash.EstimatedTotalCostUSD, wantCost)
	}
}

// TestDashboard_AvgRetriesPerPipeline verifies that AvgRetriesPerPipeline
// is computed correctly from task retry counters.
func TestDashboard_AvgRetriesPerPipeline(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()

	spec1 := filepath.Join(specsDir, "spec-retries")
	if err := os.Mkdir(spec1, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	st := completedState("M", "standard", 0)
	st["tasks"] = map[string]any{
		"1": map[string]any{
			"title":         "Task 1",
			"executionMode": "sequential",
			"implStatus":    "completed",
			"reviewStatus":  "completed_pass",
			"implRetries":   2,
			"reviewRetries": 1,
		},
	}
	writeStateJSON(t, spec1, st)

	rep := analytics.NewReporter(specsDir, nil)

	dash, err := rep.Dashboard()
	if err != nil {
		t.Fatalf("Dashboard() unexpected error: %v", err)
	}

	// 1 completed pipeline with 3 total retries → avg = 3.0
	if dash.AvgRetriesPerPipeline != 3.0 {
		t.Errorf("AvgRetriesPerPipeline = %f, want 3.0", dash.AvgRetriesPerPipeline)
	}
}

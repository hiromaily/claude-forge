// Package analytics_test tests the analytics package.
package analytics_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/internal/analytics"
)

// writeFixtureState writes a minimal state.json to dir/name/state.json.
func writeFixtureState(t *testing.T, baseDir, name, currentPhase, effort string, tokens, durationMs int) {
	t.Helper()

	specDir := filepath.Join(baseDir, name)
	if err := os.MkdirAll(specDir, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", specDir, err)
	}

	effortVal := effort

	s := map[string]any{
		"version":      1,
		"specName":     name,
		"workspace":    specDir,
		"currentPhase": currentPhase,
		"effort":       &effortVal,
		"phaseLog": []map[string]any{
			{
				"phase":       "phase-1",
				"tokens":      tokens,
				"duration_ms": durationMs,
				"model":       "sonnet",
				"timestamp":   "2026-01-01T00:00:00Z",
			},
		},
		"completedPhases":           []string{},
		"skippedPhases":             []string{},
		"tasks":                     map[string]any{},
		"checkpointRevisionPending": map[string]any{},
		"revisions": map[string]any{
			"designRevisions":       0,
			"taskRevisions":         0,
			"designInlineRevisions": 0,
			"taskInlineRevisions":   0,
		},
		"timestamps": map[string]any{
			"created":     "2026-01-01T00:00:00Z",
			"lastUpdated": "2026-01-01T00:00:00Z",
		},
		"autoApprove":      false,
		"skipPr":           false,
		"useCurrentBranch": false,
		"debug":            false,
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}

	if err := os.WriteFile(filepath.Join(specDir, "state.json"), data, 0o600); err != nil {
		t.Fatalf("write fixture state.json: %v", err)
	}
}

func TestEstimateEmptySpecs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	est := analytics.NewEstimator(dir)

	result, err := est.Estimate("M")
	if err != nil {
		t.Fatalf("Estimate returned error: %v", err)
	}

	if result.SampleSize != 0 {
		t.Errorf("SampleSize = %d, want 0", result.SampleSize)
	}

	if result.Confidence != "low" {
		t.Errorf("Confidence = %q, want %q", result.Confidence, "low")
	}

	if result.Note == "" {
		t.Error("Note should not be empty for 0-sample case")
	}
}

func TestEstimateOnePipeline(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFixtureState(t, dir, "run1", "completed", "M", 1000, 60000)

	est := analytics.NewEstimator(dir)
	result, err := est.Estimate("M")
	if err != nil {
		t.Fatalf("Estimate returned error: %v", err)
	}

	if result.SampleSize != 1 {
		t.Errorf("SampleSize = %d, want 1", result.SampleSize)
	}

	if result.Confidence != "low" {
		t.Errorf("Confidence = %q, want %q", result.Confidence, "low")
	}

	// For 1 sample, P50 and P90 should be equal
	if result.Tokens.P50 != result.Tokens.P90 {
		t.Errorf("Tokens P50=%f != P90=%f for single sample", result.Tokens.P50, result.Tokens.P90)
	}

	if result.DurationMin.P50 != result.DurationMin.P90 {
		t.Errorf("DurationMin P50=%f != P90=%f for single sample", result.DurationMin.P50, result.DurationMin.P90)
	}

	if result.CostUSD.P50 != result.CostUSD.P90 {
		t.Errorf("CostUSD P50=%f != P90=%f for single sample", result.CostUSD.P50, result.CostUSD.P90)
	}

	// Verify actual values
	const expectedTokens float64 = 1000
	if result.Tokens.P50 != expectedTokens {
		t.Errorf("Tokens.P50 = %f, want %f", result.Tokens.P50, expectedTokens)
	}

	const expectedDurationMin float64 = 1.0 // 60000ms / 60000
	if result.DurationMin.P50 != expectedDurationMin {
		t.Errorf("DurationMin.P50 = %f, want %f", result.DurationMin.P50, expectedDurationMin)
	}
}

func TestEstimateThreePipelinesMediumConfidence(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Create 3 completed M pipelines with different token counts
	// tokens: 1000, 2000, 3000
	writeFixtureState(t, dir, "run1", "completed", "M", 1000, 60000)
	writeFixtureState(t, dir, "run2", "completed", "M", 2000, 120000)
	writeFixtureState(t, dir, "run3", "completed", "M", 3000, 180000)

	est := analytics.NewEstimator(dir)
	result, err := est.Estimate("M")
	if err != nil {
		t.Fatalf("Estimate returned error: %v", err)
	}

	if result.SampleSize != 3 {
		t.Errorf("SampleSize = %d, want 3", result.SampleSize)
	}

	if result.Confidence != "medium" {
		t.Errorf("Confidence = %q, want %q", result.Confidence, "medium")
	}

	// Sorted tokens: [1000, 2000, 3000]
	// P50 index: (3-1)/2 = 1 → value 2000
	// P90 index: ceil(3*0.9)-1 = ceil(2.7)-1 = 3-1 = 2 → value 3000
	const wantP50 float64 = 2000
	const wantP90 float64 = 3000

	if result.Tokens.P50 != wantP50 {
		t.Errorf("Tokens.P50 = %f, want %f", result.Tokens.P50, wantP50)
	}

	if result.Tokens.P90 != wantP90 {
		t.Errorf("Tokens.P90 = %f, want %f", result.Tokens.P90, wantP90)
	}
}

func TestEstimateTenPipelinesHighConfidence(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	for i := range 10 {
		tokens := (i + 1) * 1000
		dur := (i + 1) * 60000
		writeFixtureState(t, dir, "run"+string(rune('0'+i)), "completed", "S", tokens, dur)
	}

	est := analytics.NewEstimator(dir)
	result, err := est.Estimate("S")
	if err != nil {
		t.Fatalf("Estimate returned error: %v", err)
	}

	if result.SampleSize != 10 {
		t.Errorf("SampleSize = %d, want 10", result.SampleSize)
	}

	if result.Confidence != "high" {
		t.Errorf("Confidence = %q, want %q", result.Confidence, "high")
	}
}

func TestEstimateMixedEffortsFiltered(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// M effort pipelines
	writeFixtureState(t, dir, "m1", "completed", "M", 1000, 60000)
	writeFixtureState(t, dir, "m2", "completed", "M", 2000, 120000)
	// S effort pipelines — should not be counted in M query
	writeFixtureState(t, dir, "s1", "completed", "S", 500, 30000)
	writeFixtureState(t, dir, "s2", "completed", "S", 600, 36000)
	writeFixtureState(t, dir, "s3", "completed", "S", 700, 42000)

	est := analytics.NewEstimator(dir)

	// Query M — should only see 2 samples
	result, err := est.Estimate("M")
	if err != nil {
		t.Fatalf("Estimate returned error: %v", err)
	}

	if result.SampleSize != 2 {
		t.Errorf("M SampleSize = %d, want 2", result.SampleSize)
	}

	// Query S — should only see 3 samples
	resultS, err := est.Estimate("S")
	if err != nil {
		t.Fatalf("Estimate S returned error: %v", err)
	}

	if resultS.SampleSize != 3 {
		t.Errorf("S SampleSize = %d, want 3", resultS.SampleSize)
	}
}

func TestEstimateAbandonedExcluded(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// completed pipeline
	writeFixtureState(t, dir, "completed1", "completed", "M", 1000, 60000)
	// abandoned pipeline — should not be counted
	writeFixtureState(t, dir, "abandoned1", "abandoned", "M", 2000, 120000)
	// in-progress pipeline — should not be counted
	writeFixtureState(t, dir, "inprogress1", "phase-5", "M", 3000, 180000)

	est := analytics.NewEstimator(dir)
	result, err := est.Estimate("M")
	if err != nil {
		t.Fatalf("Estimate returned error: %v", err)
	}

	if result.SampleSize != 1 {
		t.Errorf("SampleSize = %d, want 1 (only completed pipelines)", result.SampleSize)
	}
}

func TestEstimateStoresSpesDirField(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	est := analytics.NewEstimator(dir)

	// Verify NewEstimator stores specsDir by checking it scans the right directory.
	// Write a fixture to this specific dir; it should be found.
	writeFixtureState(t, dir, "run1", "completed", "L", 5000, 300000)

	result, err := est.Estimate("L")
	if err != nil {
		t.Fatalf("Estimate returned error: %v", err)
	}

	if result.SampleSize != 1 {
		t.Errorf("SampleSize = %d, want 1; estimator does not seem to use stored specsDir", result.SampleSize)
	}
}

func TestEstimateConfidenceBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		count          int
		wantConfidence string
	}{
		{"n=0", 0, "low"},
		{"n=1", 1, "low"},
		{"n=2", 2, "low"},
		{"n=3", 3, "medium"},
		{"n=9", 9, "medium"},
		{"n=10", 10, "high"},
		{"n=11", 11, "high"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()

			for i := range tc.count {
				tokens := (i + 1) * 1000
				dur := (i + 1) * 60000
				name := "run" + string(rune('a'+i%26))
				if i >= 26 {
					name = "run" + string(rune('A'+i%26))
				}

				writeFixtureState(t, dir, name, "completed", "S", tokens, dur)
			}

			est := analytics.NewEstimator(dir)
			result, err := est.Estimate("S")
			if err != nil {
				t.Fatalf("Estimate returned error: %v", err)
			}

			if result.Confidence != tc.wantConfidence {
				t.Errorf("n=%d: Confidence = %q, want %q", tc.count, result.Confidence, tc.wantConfidence)
			}
		})
	}
}

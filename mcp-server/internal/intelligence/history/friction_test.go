// Package history_test — unit tests for history/friction.go.
package history_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/internal/intelligence/history"
)

// copyImprovementFixture copies the testdata fixture into the given directory
// under a subdirectory named "spec-sample", mimicking a real spec layout.
func copyImprovementFixture(t *testing.T, specsDir string) {
	t.Helper()

	specDir := filepath.Join(specsDir, "spec-sample")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatalf("mkdir spec-sample: %v", err)
	}

	data, err := os.ReadFile(filepath.Join("testdata", "improvement-sample.md"))
	if err != nil {
		t.Fatalf("read improvement-sample.md fixture: %v", err)
	}

	if err := os.WriteFile(filepath.Join(specDir, "improvement.md"), data, 0o600); err != nil {
		t.Fatalf("write improvement.md: %v", err)
	}
}

func TestFrictionBuild_returnsPoints(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()
	copyImprovementFixture(t, specsDir)

	fm := history.NewFrictionMap(specsDir)
	if err := fm.Build(); err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}

	points := fm.Points()
	if len(points) == 0 {
		t.Fatal("expected at least one FrictionPoint, got none")
	}

	// AC-1: at least one point has a Category matching one of the eight fixed categories.
	validCategories := map[string]bool{
		"error_handling":    true,
		"import_order":      true,
		"test_coverage":     true,
		"naming_convention": true,
		"type_safety":       true,
		"security":          true,
		"performance":       true,
		"documentation":     true,
	}

	found := false

	for _, p := range points {
		if validCategories[p.Category] {
			found = true

			break
		}
	}

	if !found {
		t.Errorf("no FrictionPoint with a valid category; got points: %+v", points)
	}
}

func TestFrictionBuild_emptyDir(t *testing.T) {
	t.Parallel()

	// AC-2: Build on a dir with no improvement.md files returns empty slice without error.
	specsDir := t.TempDir()

	fm := history.NewFrictionMap(specsDir)
	if err := fm.Build(); err != nil {
		t.Fatalf("Build() on empty dir returned error: %v", err)
	}

	points := fm.Points()
	if len(points) != 0 {
		t.Errorf("expected empty slice for dir with no improvement.md, got %d points", len(points))
	}
}

func TestFrictionBuild_nonExistentDir(t *testing.T) {
	t.Parallel()

	// Build on a non-existent directory should return no error (tolerates absent dir).
	fm := history.NewFrictionMap("/tmp/nonexistent-dir-for-friction-test-xyz")
	if err := fm.Build(); err != nil {
		t.Fatalf("Build() on non-existent dir returned error: %v", err)
	}

	if len(fm.Points()) != 0 {
		t.Errorf("expected empty slice for non-existent dir, got %d points", len(fm.Points()))
	}
}

func TestFrictionBuild_roundTrip(t *testing.T) {
	t.Parallel()

	// AC-4: Build → persist → Load fresh instance → same friction points restored.
	specsDir := t.TempDir()
	copyImprovementFixture(t, specsDir)

	fm := history.NewFrictionMap(specsDir)
	if err := fm.Build(); err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}

	original := fm.Points()
	if len(original) == 0 {
		t.Fatal("expected at least one FrictionPoint before persist")
	}

	originalTotal := fm.TotalReportsAnalyzed()

	// Persist to disk (Build should persist automatically; verify friction.json exists).
	frictionPath := filepath.Join(specsDir, "friction.json")
	if _, err := os.Stat(frictionPath); err != nil {
		t.Fatalf("friction.json not written after Build: %v", err)
	}

	// Load a fresh instance from the same specsDir.
	fm2 := history.NewFrictionMap(specsDir)
	if err := fm2.Load(); err != nil {
		t.Fatalf("Load() on fresh instance returned error: %v", err)
	}

	restored := fm2.Points()

	if len(restored) != len(original) {
		t.Fatalf("round-trip length mismatch: got %d, want %d", len(restored), len(original))
	}

	if fm2.TotalReportsAnalyzed() != originalTotal {
		t.Errorf("round-trip TotalReportsAnalyzed: got %d, want %d",
			fm2.TotalReportsAnalyzed(), originalTotal)
	}

	// Verify each point matches (by category and description).
	origMap := make(map[string]history.FrictionPoint, len(original))
	for _, p := range original {
		origMap[p.Category+"|"+p.Description] = p
	}

	for _, p := range restored {
		key := p.Category + "|" + p.Description
		if _, ok := origMap[key]; !ok {
			t.Errorf("restored point not found in original: %+v", p)
		}
	}
}

func TestFrictionLoad_absentFile(t *testing.T) {
	t.Parallel()

	// Load on a dir where friction.json does not exist should return no error (fail-open).
	specsDir := t.TempDir()

	fm := history.NewFrictionMap(specsDir)
	if err := fm.Load(); err != nil {
		t.Fatalf("Load() on absent friction.json returned error: %v", err)
	}

	if len(fm.Points()) != 0 {
		t.Errorf("expected empty points after Load on absent file, got %d", len(fm.Points()))
	}
}

func TestFrictionLoad_corruptedFile(t *testing.T) {
	t.Parallel()

	// Load on a corrupted friction.json should not panic and return an empty state.
	specsDir := t.TempDir()

	frictionPath := filepath.Join(specsDir, "friction.json")
	if err := os.WriteFile(frictionPath, []byte("not valid json {{{"), 0o600); err != nil {
		t.Fatalf("write corrupted friction.json: %v", err)
	}

	fm := history.NewFrictionMap(specsDir)
	// Load may return an error for corrupted JSON but must not panic.
	_ = fm.Load()

	// Points should be usable (possibly empty).
	_ = fm.Points()
}

func TestFrictionBuild_frictionJSONShape(t *testing.T) {
	t.Parallel()

	// Verify the on-disk JSON shape matches FrictionFile.
	specsDir := t.TempDir()
	copyImprovementFixture(t, specsDir)

	fm := history.NewFrictionMap(specsDir)
	if err := fm.Build(); err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	frictionPath := filepath.Join(specsDir, "friction.json")
	data, err := os.ReadFile(frictionPath)
	if err != nil {
		t.Fatalf("read friction.json: %v", err)
	}

	var ff struct {
		UpdatedAt            string           `json:"updatedAt"`
		TotalReportsAnalyzed int              `json:"totalReportsAnalyzed"`
		FrictionPoints       []map[string]any `json:"frictionPoints"`
	}

	if err := json.Unmarshal(data, &ff); err != nil {
		t.Fatalf("parse friction.json: %v", err)
	}

	if ff.UpdatedAt == "" {
		t.Error("updatedAt should not be empty in friction.json")
	}

	if ff.TotalReportsAnalyzed <= 0 {
		t.Errorf("totalReportsAnalyzed should be > 0, got %d", ff.TotalReportsAnalyzed)
	}

	for i, pt := range ff.FrictionPoints {
		if pt["category"] == nil {
			t.Errorf("frictionPoints[%d] missing category field", i)
		}

		if pt["description"] == nil {
			t.Errorf("frictionPoints[%d] missing description field", i)
		}

		if pt["frequency"] == nil {
			t.Errorf("frictionPoints[%d] missing frequency field", i)
		}
	}
}

func TestFrictionBuild_summaryFallback(t *testing.T) {
	t.Parallel()

	// Verify that Build falls back to summary.md when improvement.md is absent.
	specsDir := t.TempDir()

	specDir := filepath.Join(specsDir, "spec-summary-only")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Write a summary.md with an embedded Improvement Report section.
	summaryContent := `## Summary
Task completed.

## Improvement Report

_Retrospective on what would have made this work easier._

### Documentation

The function was missing godoc comments for exported symbols.

### Code Readability

No friction observed.

### AI Agent Support (Skills / Rules)

No friction observed.

## Pipeline Statistics
- Total tokens: 100000
`
	if err := os.WriteFile(filepath.Join(specDir, "summary.md"), []byte(summaryContent), 0o600); err != nil {
		t.Fatalf("write summary.md: %v", err)
	}

	fm := history.NewFrictionMap(specsDir)
	if err := fm.Build(); err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	if fm.TotalReportsAnalyzed() != 1 {
		t.Errorf("TotalReportsAnalyzed = %d, want 1", fm.TotalReportsAnalyzed())
	}

	points := fm.Points()
	if len(points) == 0 {
		t.Fatal("expected at least one FrictionPoint from summary.md fallback, got none")
	}

	// Should extract "documentation" category from the godoc line.
	found := false
	for _, p := range points {
		if p.Category == "documentation" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected documentation category from summary.md, got points: %+v", points)
	}
}

func TestFrictionBuild_improvementTakesPrecedence(t *testing.T) {
	t.Parallel()

	// When both improvement.md and summary.md exist, improvement.md takes precedence.
	specsDir := t.TempDir()

	specDir := filepath.Join(specsDir, "spec-both")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Write improvement.md with error handling content.
	improvementContent := "### Error Handling\n- Missing error check on file open.\n"
	if err := os.WriteFile(filepath.Join(specDir, "improvement.md"), []byte(improvementContent), 0o600); err != nil {
		t.Fatalf("write improvement.md: %v", err)
	}

	// Write summary.md with test coverage content.
	summaryContent := "## Improvement Report\n### Test Coverage\n- Missing unit tests for parser.\n"
	if err := os.WriteFile(filepath.Join(specDir, "summary.md"), []byte(summaryContent), 0o600); err != nil {
		t.Fatalf("write summary.md: %v", err)
	}

	fm := history.NewFrictionMap(specsDir)
	if err := fm.Build(); err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	points := fm.Points()
	// Should have error_handling from improvement.md, NOT test_coverage from summary.md.
	hasError := false
	hasTestCoverage := false
	for _, p := range points {
		if p.Category == "error_handling" {
			hasError = true
		}
		if p.Category == "test_coverage" {
			hasTestCoverage = true
		}
	}

	if !hasError {
		t.Error("expected error_handling from improvement.md, not found")
	}
	if hasTestCoverage {
		t.Error("should not have test_coverage from summary.md when improvement.md exists")
	}
}

func TestFrictionBuild_multipleSpecDirs(t *testing.T) {
	t.Parallel()

	// Verify that Build scans multiple subdirectories.
	specsDir := t.TempDir()

	// Create two spec dirs each with an improvement.md.
	for _, name := range []string{"spec-alpha", "spec-beta"} {
		dir := filepath.Join(specsDir, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}

		data, err := os.ReadFile(filepath.Join("testdata", "improvement-sample.md"))
		if err != nil {
			t.Fatalf("read fixture: %v", err)
		}

		if err := os.WriteFile(filepath.Join(dir, "improvement.md"), data, 0o600); err != nil {
			t.Fatalf("write improvement.md in %s: %v", name, err)
		}
	}

	fm := history.NewFrictionMap(specsDir)
	if err := fm.Build(); err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	if fm.TotalReportsAnalyzed() < 2 {
		t.Errorf("expected TotalReportsAnalyzed >= 2 for two spec dirs, got %d",
			fm.TotalReportsAnalyzed())
	}
}

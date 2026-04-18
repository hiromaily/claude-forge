package history_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/orchestrator"
	"github.com/hiromaily/claude-forge/mcp-server/internal/intelligence/history"
)

// makeFindings is a helper to construct []orchestrator.Finding slices.
func makeFindings(sev orchestrator.Severity, descriptions ...string) []orchestrator.Finding {
	findings := make([]orchestrator.Finding, len(descriptions))
	for i, d := range descriptions {
		findings[i] = orchestrator.Finding{Severity: sev, Description: d}
	}

	return findings
}

// TestPatternAccumulate_MergesNearIdentical verifies AC-1:
// Two Accumulate calls with near-identical normalised descriptions produce one
// PatternEntry with Frequency: 2.
func TestPatternAccumulate_MergesNearIdentical(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	acc := history.NewPatternAccumulator(dir)

	now := time.Now().UTC()

	// Two findings whose normalised text is very close (only one character differs).
	f1 := makeFindings(orchestrator.SeverityCritical, "Missing error handling for file read operations")
	f2 := makeFindings(orchestrator.SeverityCritical, "Missing error handling for file read operation")

	if err := acc.Accumulate(f1, "design-reviewer", now); err != nil {
		t.Fatalf("first Accumulate: %v", err)
	}

	if err := acc.Accumulate(f2, "design-reviewer", now.Add(time.Minute)); err != nil {
		t.Fatalf("second Accumulate: %v", err)
	}

	entries := acc.Entries()
	if len(entries) != 1 {
		t.Fatalf("want 1 merged entry, got %d: %+v", len(entries), entries)
	}

	if entries[0].Frequency != 2 {
		t.Errorf("want Frequency=2, got %d", entries[0].Frequency)
	}

	if entries[0].Severity != string(orchestrator.SeverityCritical) {
		t.Errorf("want Severity=CRITICAL, got %s", entries[0].Severity)
	}
}

// TestPatternAccumulate_KeepsDifferentCategories verifies that two findings in
// different categories are not merged even if their descriptions happen to score
// close after normalisation.
func TestPatternAccumulate_KeepsDifferentCategories(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	acc := history.NewPatternAccumulator(dir)

	now := time.Now().UTC()

	f1 := makeFindings(orchestrator.SeverityCritical, "Missing error handling for file read")
	f2 := makeFindings(orchestrator.SeverityMinor, "Missing documentation on exported types")

	if err := acc.Accumulate(f1, "design-reviewer", now); err != nil {
		t.Fatalf("first Accumulate: %v", err)
	}

	if err := acc.Accumulate(f2, "design-reviewer", now); err != nil {
		t.Fatalf("second Accumulate: %v", err)
	}

	entries := acc.Entries()
	if len(entries) < 2 {
		t.Fatalf("want at least 2 entries (different categories/severities), got %d", len(entries))
	}
}

// TestPatternAccumulate_PersistAndLoad verifies AC-2:
// After Accumulate + persist (implicit in Accumulate), a new PatternAccumulator
// calling Load restores the same entries and TotalReviewsAnalyzed.
func TestPatternAccumulate_PersistAndLoad(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	acc := history.NewPatternAccumulator(dir)

	now := time.Now().UTC()

	f1 := makeFindings(orchestrator.SeverityCritical, "Missing error handling for file read operations")
	f2 := makeFindings(orchestrator.SeverityCritical, "Missing error handling for file read operation")

	if err := acc.Accumulate(f1, "design-reviewer", now); err != nil {
		t.Fatalf("first Accumulate: %v", err)
	}

	if err := acc.Accumulate(f2, "design-reviewer", now.Add(time.Minute)); err != nil {
		t.Fatalf("second Accumulate: %v", err)
	}

	origEntries := acc.Entries()
	origTotal := acc.TotalReviewsAnalyzed()

	// Verify patterns.json was written.
	patternsPath := filepath.Join(dir, "patterns.json")
	if _, err := os.Stat(patternsPath); err != nil {
		t.Fatalf("patterns.json not written: %v", err)
	}

	// Load into a fresh accumulator.
	acc2 := history.NewPatternAccumulator(dir)
	if err := acc2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	loadedEntries := acc2.Entries()
	loadedTotal := acc2.TotalReviewsAnalyzed()

	if loadedTotal != origTotal {
		t.Errorf("TotalReviewsAnalyzed: want %d, got %d", origTotal, loadedTotal)
	}

	if len(loadedEntries) != len(origEntries) {
		t.Fatalf("entry count: want %d, got %d", len(origEntries), len(loadedEntries))
	}

	for i, e := range origEntries {
		l := loadedEntries[i]
		if e.Pattern != l.Pattern {
			t.Errorf("entry %d Pattern: want %q, got %q", i, e.Pattern, l.Pattern)
		}

		if e.Frequency != l.Frequency {
			t.Errorf("entry %d Frequency: want %d, got %d", i, e.Frequency, l.Frequency)
		}

		if e.Severity != l.Severity {
			t.Errorf("entry %d Severity: want %s, got %s", i, e.Severity, l.Severity)
		}
	}
}

// TestPatternAccumulate_TotalReviewsAnalyzed verifies that each Accumulate call
// increments TotalReviewsAnalyzed by one.
func TestPatternAccumulate_TotalReviewsAnalyzed(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	acc := history.NewPatternAccumulator(dir)

	now := time.Now().UTC()

	f1 := makeFindings(orchestrator.SeverityCritical, "error not checked")
	f2 := makeFindings(orchestrator.SeverityMinor, "name is poorly documented")

	if err := acc.Accumulate(f1, "design-reviewer", now); err != nil {
		t.Fatalf("Accumulate 1: %v", err)
	}

	if acc.TotalReviewsAnalyzed() != 1 {
		t.Errorf("after first Accumulate, want TotalReviewsAnalyzed=1, got %d", acc.TotalReviewsAnalyzed())
	}

	if err := acc.Accumulate(f2, "task-reviewer", now.Add(time.Minute)); err != nil {
		t.Fatalf("Accumulate 2: %v", err)
	}

	if acc.TotalReviewsAnalyzed() != 2 {
		t.Errorf("after second Accumulate, want TotalReviewsAnalyzed=2, got %d", acc.TotalReviewsAnalyzed())
	}
}

// TestPatternAccumulate_EmptyFindings verifies that Accumulate with an empty slice
// does not panic and still increments TotalReviewsAnalyzed.
func TestPatternAccumulate_EmptyFindings(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	acc := history.NewPatternAccumulator(dir)

	if err := acc.Accumulate(nil, "design-reviewer", time.Now().UTC()); err != nil {
		t.Fatalf("Accumulate(nil): %v", err)
	}

	if acc.TotalReviewsAnalyzed() != 1 {
		t.Errorf("want TotalReviewsAnalyzed=1, got %d", acc.TotalReviewsAnalyzed())
	}

	if len(acc.Entries()) != 0 {
		t.Errorf("want 0 entries for empty findings, got %d", len(acc.Entries()))
	}
}

// TestPatternQuery_FilterBySeverity verifies that Query filters by severity.
func TestPatternQuery_FilterBySeverity(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	acc := history.NewPatternAccumulator(dir)

	now := time.Now().UTC()

	critFindings := makeFindings(orchestrator.SeverityCritical, "Missing error handling for file read operations")
	minorFindings := makeFindings(orchestrator.SeverityMinor, "Missing documentation on exported types")

	if err := acc.Accumulate(critFindings, "design-reviewer", now); err != nil {
		t.Fatalf("Accumulate critical: %v", err)
	}

	if err := acc.Accumulate(minorFindings, "design-reviewer", now); err != nil {
		t.Fatalf("Accumulate minor: %v", err)
	}

	// Query critical only.
	results := acc.Query("", "CRITICAL", 10)
	for _, r := range results {
		if r.Severity != "CRITICAL" {
			t.Errorf("expected only CRITICAL entries, got %s", r.Severity)
		}
	}

	// Query minor only.
	minorResults := acc.Query("", "MINOR", 10)
	for _, r := range minorResults {
		if r.Severity != "MINOR" {
			t.Errorf("expected only MINOR entries, got %s", r.Severity)
		}
	}
}

// TestPatternQuery_FilterByAgent verifies that Query filters by agent.
func TestPatternQuery_FilterByAgent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	acc := history.NewPatternAccumulator(dir)

	now := time.Now().UTC()

	designFindings := makeFindings(orchestrator.SeverityCritical, "Missing error handling for file read operations")
	taskFindings := makeFindings(orchestrator.SeverityMinor, "Missing documentation on exported types")

	if err := acc.Accumulate(designFindings, "design-reviewer", now); err != nil {
		t.Fatalf("Accumulate design-reviewer: %v", err)
	}

	if err := acc.Accumulate(taskFindings, "task-reviewer", now); err != nil {
		t.Fatalf("Accumulate task-reviewer: %v", err)
	}

	// Query design-reviewer only.
	results := acc.Query("design-reviewer", "", 10)
	for _, r := range results {
		if r.Agent != "design-reviewer" {
			t.Errorf("expected only design-reviewer entries, got agent=%s", r.Agent)
		}
	}
}

// TestPatternQuery_LimitRespected verifies that Query respects the limit parameter.
func TestPatternQuery_LimitRespected(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	acc := history.NewPatternAccumulator(dir)

	now := time.Now().UTC()

	// Add several distinct findings.
	for i := range 5 {
		desc := []string{
			"error not checked in handler",
			"missing documentation on exported struct",
			"naming convention violated in function",
			"type assertion without check is unsafe",
			"security vulnerability in authentication flow",
		}[i]

		f := makeFindings(orchestrator.SeverityCritical, desc)
		if err := acc.Accumulate(f, "design-reviewer", now); err != nil {
			t.Fatalf("Accumulate %d: %v", i, err)
		}
	}

	results := acc.Query("", "", 2)
	if len(results) > 2 {
		t.Errorf("want at most 2 results, got %d", len(results))
	}
}

// TestPatternLoad_AbsentFile verifies that Load on an empty dir returns nil (fail-open).
func TestPatternLoad_AbsentFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	acc := history.NewPatternAccumulator(dir)

	if err := acc.Load(); err != nil {
		t.Errorf("Load on absent patterns.json: want nil, got %v", err)
	}

	if len(acc.Entries()) != 0 {
		t.Errorf("want 0 entries after Load on absent file, got %d", len(acc.Entries()))
	}
}

package prompt

import (
	"reflect"
	"testing"
	"time"

	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/orchestrator"
	"github.com/hiromaily/claude-forge/mcp-server/internal/intelligence/history"
)

func TestBuildContextFromResults_NilKBNilResults(t *testing.T) {
	t.Parallel()

	ctx := BuildContextFromResults(nil, nil)

	empty := HistoryContext{}
	if !reflect.DeepEqual(ctx, empty) {
		t.Errorf("expected empty HistoryContext, got %+v", ctx)
	}
}

func TestBuildContextFromResults_NilKBWithResults(t *testing.T) {
	t.Parallel()

	results := []history.SearchResult{
		{
			SpecName:   "20260101-test-spec",
			Similarity: 0.9,
			OneLiner:   "A test pipeline",
		},
	}

	ctx := BuildContextFromResults(results, nil)

	if len(ctx.SimilarPipelines) != 1 {
		t.Fatalf("SimilarPipelines: expected 1 entry, got %d", len(ctx.SimilarPipelines))
	}
	if ctx.SimilarPipelines[0].SpecName != "20260101-test-spec" {
		t.Errorf("SimilarPipelines[0].SpecName = %q, want %q",
			ctx.SimilarPipelines[0].SpecName, "20260101-test-spec")
	}
	if ctx.CriticalPatterns != nil {
		t.Errorf("CriticalPatterns: expected nil when kb is nil, got %v", ctx.CriticalPatterns)
	}
	if ctx.AllPatterns != nil {
		t.Errorf("AllPatterns: expected nil when kb is nil, got %v", ctx.AllPatterns)
	}
	if ctx.FrictionPoints != nil {
		t.Errorf("FrictionPoints: expected nil when kb is nil, got %v", ctx.FrictionPoints)
	}
}

func TestBuildContextFromResults_NonNilKBEmptyState(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	kb := history.NewKnowledgeBase(dir)

	ctx := BuildContextFromResults(nil, kb)

	// With non-nil KB and empty patterns, Query returns empty slices (not nil).
	if ctx.CriticalPatterns == nil {
		t.Error("CriticalPatterns should be empty slice (not nil) when kb is non-nil")
	}
	if ctx.AllPatterns == nil {
		t.Error("AllPatterns should be empty slice (not nil) when kb is non-nil")
	}
	if ctx.FrictionPoints == nil {
		t.Error("FrictionPoints should be empty slice (not nil) when kb is non-nil")
	}
	if len(ctx.CriticalPatterns) != 0 {
		t.Errorf("CriticalPatterns: expected 0, got %d", len(ctx.CriticalPatterns))
	}
	if len(ctx.AllPatterns) != 0 {
		t.Errorf("AllPatterns: expected 0, got %d", len(ctx.AllPatterns))
	}
	if len(ctx.FrictionPoints) != 0 {
		t.Errorf("FrictionPoints: expected 0, got %d", len(ctx.FrictionPoints))
	}
}

func TestBuildContextFromResults_KBWithPatterns(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	kb := history.NewKnowledgeBase(dir)

	now := time.Now()
	err := kb.Patterns.Accumulate([]orchestrator.Finding{
		{Severity: orchestrator.SeverityCritical, Description: "Missing error handling in db layer"},
		{Severity: orchestrator.SeverityMinor, Description: "Import order inconsistency"},
	}, "impl-reviewer", now)
	if err != nil {
		t.Fatalf("Accumulate: %v", err)
	}

	ctx := BuildContextFromResults(nil, kb)

	// CriticalPatterns uses Query("", "CRITICAL", 20) — should have 1 entry.
	if len(ctx.CriticalPatterns) != 1 {
		t.Errorf("CriticalPatterns: expected 1, got %d", len(ctx.CriticalPatterns))
	}
	if len(ctx.CriticalPatterns) > 0 && ctx.CriticalPatterns[0].Severity != "CRITICAL" {
		t.Errorf("CriticalPatterns[0].Severity = %q, want %q",
			ctx.CriticalPatterns[0].Severity, "CRITICAL")
	}

	// AllPatterns uses Query("", "", 20) — should have 2 entries.
	if len(ctx.AllPatterns) != 2 {
		t.Errorf("AllPatterns: expected 2, got %d", len(ctx.AllPatterns))
	}
}

func TestBuildContextFromResults_SimilarPipelinesAssigned(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	kb := history.NewKnowledgeBase(dir)

	results := []history.SearchResult{
		{SpecName: "spec-a", Similarity: 0.8},
		{SpecName: "spec-b", Similarity: 0.7},
	}

	ctx := BuildContextFromResults(results, kb)

	if len(ctx.SimilarPipelines) != 2 {
		t.Fatalf("SimilarPipelines: expected 2, got %d", len(ctx.SimilarPipelines))
	}
	if ctx.SimilarPipelines[0].SpecName != "spec-a" {
		t.Errorf("SimilarPipelines[0].SpecName = %q, want %q",
			ctx.SimilarPipelines[0].SpecName, "spec-a")
	}
}

func TestHistoryContextFields(t *testing.T) {
	t.Parallel()

	// Verifies the struct has the expected fields with the expected types.
	ctx := HistoryContext{
		SimilarPipelines: []history.SearchResult{{SpecName: "spec1"}},
		CriticalPatterns: []history.PatternEntry{{Pattern: "pat1"}},
		AllPatterns:      []history.PatternEntry{{Pattern: "pat2"}},
		FrictionPoints:   []history.FrictionPoint{{Category: "cat1"}},
	}

	if len(ctx.SimilarPipelines) != 1 {
		t.Errorf("SimilarPipelines: expected 1, got %d", len(ctx.SimilarPipelines))
	}
	if len(ctx.CriticalPatterns) != 1 {
		t.Errorf("CriticalPatterns: expected 1, got %d", len(ctx.CriticalPatterns))
	}
	if len(ctx.AllPatterns) != 1 {
		t.Errorf("AllPatterns: expected 1, got %d", len(ctx.AllPatterns))
	}
	if len(ctx.FrictionPoints) != 1 {
		t.Errorf("FrictionPoints: expected 1, got %d", len(ctx.FrictionPoints))
	}
}

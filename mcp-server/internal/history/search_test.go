package history_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hiromaily/claude-forge/mcp-server/internal/history"
)

// writeHistoryIndexFile writes a valid history-index.json to specsDir using encoding/json.
func writeHistoryIndexFile(t *testing.T, specsDir string, entries []history.IndexEntry) {
	t.Helper()
	idxFile := history.IndexFile{
		IndexedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		Entries:   entries,
	}
	data, err := json.Marshal(idxFile)
	if err != nil {
		t.Fatalf("marshal history index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(specsDir, "history-index.json"), data, 0o600); err != nil {
		t.Fatalf("write history-index.json: %v", err)
	}
}

func TestSearch_empty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	idx := history.New(dir)
	results, err := history.Search(idx, "some query", 10, "")
	if err != nil {
		t.Fatalf("Search() returned unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestSearch_taskTypeFilter(t *testing.T) {
	t.Parallel()

	entries := []history.IndexEntry{
		{
			SpecName: "spec-bugfix-1",
			OneLiner: "fix authentication token expiry bug",
			TaskType: "bugfix",
			Outcome:  "completed",
			Tags:     []string{"authentication", "token", "expiry", "bugfix"},
		},
		{
			SpecName: "spec-feature-1",
			OneLiner: "add user profile feature request",
			TaskType: "feature",
			Outcome:  "completed",
			Tags:     []string{"user", "profile", "feature", "request"},
		},
		{
			SpecName: "spec-bugfix-2",
			OneLiner: "fix database connection pool exhaustion",
			TaskType: "bugfix",
			Outcome:  "completed",
			Tags:     []string{"database", "connection", "pool", "exhaustion"},
		},
	}

	dir := t.TempDir()
	writeHistoryIndexFile(t, dir, entries)
	idx := history.New(dir)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	results, err := history.Search(idx, "fix authentication bug", 10, "bugfix")
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}

	for _, r := range results {
		if r.TaskType != "bugfix" {
			t.Errorf("expected only bugfix results, got TaskType=%q for %q", r.TaskType, r.SpecName)
		}
	}
}

func TestSearch_limit(t *testing.T) {
	t.Parallel()

	entries := []history.IndexEntry{
		{
			SpecName: "spec-1",
			OneLiner: "refactor authentication middleware layer",
			TaskType: "feature",
			Outcome:  "completed",
			Tags:     []string{"refactor", "authentication", "middleware", "layer"},
		},
		{
			SpecName: "spec-2",
			OneLiner: "refactor database query optimization pipeline",
			TaskType: "feature",
			Outcome:  "completed",
			Tags:     []string{"refactor", "database", "query", "optimization"},
		},
		{
			SpecName: "spec-3",
			OneLiner: "refactor cache invalidation strategy implementation",
			TaskType: "feature",
			Outcome:  "completed",
			Tags:     []string{"refactor", "cache", "invalidation", "strategy"},
		},
	}

	dir := t.TempDir()
	writeHistoryIndexFile(t, dir, entries)
	idx := history.New(dir)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	results, err := history.Search(idx, "refactor", 1, "")
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}

	if len(results) > 1 {
		t.Errorf("expected at most 1 result with limit=1, got %d", len(results))
	}
}

func TestSearch_designExcerpt(t *testing.T) {
	t.Parallel()

	specName := "spec-with-design"
	entries := []history.IndexEntry{
		{
			SpecName: specName,
			OneLiner: "implement search feature for user queries",
			TaskType: "feature",
			Outcome:  "completed",
			Tags:     []string{"search", "feature", "user", "queries"},
		},
	}

	dir := t.TempDir()
	writeHistoryIndexFile(t, dir, entries)

	// Create the spec subdirectory and design.md.
	specDir := filepath.Join(dir, specName)
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatalf("mkdir spec dir: %v", err)
	}
	designContent := "# Design: Search Feature\n\nThis design covers the implementation of the search feature " +
		"that allows users to find relevant items quickly using keyword matching and BM25 scoring algorithm. " +
		"The approach uses an inverted index."
	if err := os.WriteFile(filepath.Join(specDir, "design.md"), []byte(designContent), 0o600); err != nil {
		t.Fatalf("write design.md: %v", err)
	}

	idx := history.New(dir)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	results, err := history.Search(idx, "search feature user queries keyword", 10, "")
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}

	excerpt := results[0].DesignExcerpt
	if excerpt == "" {
		t.Error("expected non-empty DesignExcerpt when design.md exists")
	}
	// Excerpt must be at most 200 bytes.
	if len(excerpt) > 200 {
		t.Errorf("DesignExcerpt exceeds 200 bytes: %d bytes", len(excerpt))
	}
	// Excerpt must be the beginning of the design content.
	expected := designContent
	if len(expected) > 200 {
		expected = expected[:200]
	}
	if excerpt != expected {
		t.Errorf("DesignExcerpt mismatch:\ngot:  %q\nwant: %q", excerpt, expected)
	}
}

func TestSearch_noDesignMd(t *testing.T) {
	t.Parallel()

	specName := "spec-no-design"
	entries := []history.IndexEntry{
		{
			SpecName: specName,
			OneLiner: "implement cache invalidation strategy pattern",
			TaskType: "feature",
			Outcome:  "completed",
			Tags:     []string{"cache", "invalidation", "strategy", "pattern"},
		},
	}

	dir := t.TempDir()
	writeHistoryIndexFile(t, dir, entries)

	// Create spec dir without design.md.
	specDir := filepath.Join(dir, specName)
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatalf("mkdir spec dir: %v", err)
	}
	// Intentionally no design.md.

	idx := history.New(dir)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	results, err := history.Search(idx, "cache invalidation strategy pattern", 10, "")
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}

	if results[0].DesignExcerpt != "" {
		t.Errorf("expected empty DesignExcerpt when design.md absent, got %q", results[0].DesignExcerpt)
	}
}

// Package history_test — unit tests for history/index.go.
package history_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hiromaily/claude-forge/mcp-server/internal/intelligence/history"
)

// writeStateJSON writes a minimal state.json into specDir.
func writeStateJSON(t *testing.T, specDir string, currentPhase string, effort string, flowTemplate string) {
	t.Helper()

	lastUpdated := time.Now().UTC().Format(time.RFC3339)
	created := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)

	content := map[string]any{
		"version":      1,
		"specName":     filepath.Base(specDir),
		"currentPhase": currentPhase,
		"effort":       effort,
		"flowTemplate": flowTemplate,
		"phaseLog":     []any{},
		"timestamps": map[string]string{
			"created":     created,
			"lastUpdated": lastUpdated,
		},
	}

	data, err := json.MarshalIndent(content, "", "  ")
	if err != nil {
		t.Fatalf("marshal state.json: %v", err)
	}

	if err := os.WriteFile(filepath.Join(specDir, "state.json"), data, 0o600); err != nil {
		t.Fatalf("write state.json: %v", err)
	}
}

// writeRequestMD writes a minimal request.md into specDir.
func writeRequestMD(t *testing.T, specDir string, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(specDir, "request.md"), []byte(content), 0o600); err != nil {
		t.Fatalf("write request.md: %v", err)
	}
}

func TestNew_noAutoCall(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()
	h := history.New(specsDir)

	if h.Size() != 0 {
		t.Errorf("expected Size() == 0 before Build, got %d", h.Size())
	}

	entries := h.Entries()
	if entries == nil {
		t.Error("Entries() should return non-nil empty slice, got nil")
	}

	if len(entries) != 0 {
		t.Errorf("expected Entries() length 0 before Build, got %d", len(entries))
	}
}

func TestBuild_empty(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()
	h := history.New(specsDir)

	if err := h.Build(); err != nil {
		t.Fatalf("Build() returned error on empty dir: %v", err)
	}

	if h.Size() != 0 {
		t.Errorf("expected 0 entries for empty specsDir, got %d", h.Size())
	}
}

func TestBuild_skipsNonTerminal(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()

	// Create a spec with non-terminal phase.
	specDir := filepath.Join(specsDir, "spec-in-progress")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	writeStateJSON(t, specDir, "phase-2", "M", "standard")
	writeRequestMD(t, specDir, "# My Feature\nSome description")

	h := history.New(specsDir)
	if err := h.Build(); err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	if h.Size() != 0 {
		t.Errorf("expected non-terminal spec to be skipped, got %d entries", h.Size())
	}
}

func TestBuild_indexesCompleted(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()

	specDir := filepath.Join(specsDir, "spec-completed")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	writeStateJSON(t, specDir, "completed", "M", "standard")
	writeRequestMD(t, specDir, "---\nsource_type: github_issue\n---\n\nAdd new feature for users")

	h := history.New(specsDir)
	if err := h.Build(); err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	if h.Size() != 1 {
		t.Fatalf("expected 1 entry, got %d", h.Size())
	}

	entry := h.Entries()[0]
	if entry.SpecName != "spec-completed" {
		t.Errorf("expected SpecName 'spec-completed', got %q", entry.SpecName)
	}

	if entry.Outcome != "completed" {
		t.Errorf("expected Outcome 'completed', got %q", entry.Outcome)
	}

	if entry.OneLiner == "" {
		t.Error("expected non-empty OneLiner after stripping frontmatter")
	}
}

func TestBuild_indexesAbandoned(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()

	specDir := filepath.Join(specsDir, "spec-abandoned")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	writeStateJSON(t, specDir, "abandoned", "S", "lite")
	writeRequestMD(t, specDir, "# Bug Report\nResolve the crash in login")

	h := history.New(specsDir)
	if err := h.Build(); err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	if h.Size() != 1 {
		t.Fatalf("expected 1 entry, got %d", h.Size())
	}

	entry := h.Entries()[0]
	if entry.Outcome != "abandoned" {
		t.Errorf("expected Outcome 'abandoned', got %q", entry.Outcome)
	}
}

func TestBuild_idempotent(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()

	specDir := filepath.Join(specsDir, "spec-completed")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	writeStateJSON(t, specDir, "completed", "M", "standard")
	writeRequestMD(t, specDir, "# My Feature\nSome description")

	h := history.New(specsDir)

	if err := h.Build(); err != nil {
		t.Fatalf("first Build() error: %v", err)
	}

	sizeAfterFirst := h.Size()

	if err := h.Build(); err != nil {
		t.Fatalf("second Build() error: %v", err)
	}

	sizeAfterSecond := h.Size()

	if sizeAfterSecond != sizeAfterFirst {
		t.Errorf("Build is not idempotent: first=%d, second=%d", sizeAfterFirst, sizeAfterSecond)
	}

	// Verify no duplicate SpecName entries.
	entries := h.Entries()
	seen := make(map[string]int)
	for _, e := range entries {
		seen[e.SpecName]++
	}

	for name, count := range seen {
		if count > 1 {
			t.Errorf("duplicate SpecName %q found %d times", name, count)
		}
	}
}

func TestBuild_missingStateJSON(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()

	// Create a spec dir without state.json.
	specDir := filepath.Join(specsDir, "spec-no-state")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	h := history.New(specsDir)
	if err := h.Build(); err != nil {
		t.Fatalf("Build() should not error when state.json is missing: %v", err)
	}

	if h.Size() != 0 {
		t.Errorf("spec with missing state.json should be skipped, got %d entries", h.Size())
	}
}

func TestBuild_differentialUpdate(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()

	// Create first spec.
	spec1Dir := filepath.Join(specsDir, "spec-first")
	if err := os.MkdirAll(spec1Dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	writeStateJSON(t, spec1Dir, "completed", "M", "standard")
	writeRequestMD(t, spec1Dir, "# First Feature\nFirst description")

	h := history.New(specsDir)

	if err := h.Build(); err != nil {
		t.Fatalf("first Build() error: %v", err)
	}

	if h.Size() != 1 {
		t.Fatalf("expected 1 entry after first Build, got %d", h.Size())
	}

	// Add a second spec with a future lastUpdated so it's after the indexedAt watermark.
	futureTime := time.Now().UTC().Add(2 * time.Second).Format(time.RFC3339)
	spec2Dir := filepath.Join(specsDir, "spec-second")
	if err := os.MkdirAll(spec2Dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Write state.json with future lastUpdated so it's after indexedAt watermark.
	content := map[string]any{
		"version":      1,
		"specName":     "spec-second",
		"currentPhase": "completed",
		"effort":       "S",
		"flowTemplate": "lite",
		"phaseLog":     []any{},
		"timestamps": map[string]string{
			"created":     futureTime,
			"lastUpdated": futureTime,
		},
	}

	data, err := json.MarshalIndent(content, "", "  ")
	if err != nil {
		t.Fatalf("marshal state.json: %v", err)
	}

	if err := os.WriteFile(filepath.Join(spec2Dir, "state.json"), data, 0o600); err != nil {
		t.Fatalf("write state.json: %v", err)
	}

	writeRequestMD(t, spec2Dir, "# Second Feature\nSecond description")

	if err := h.Build(); err != nil {
		t.Fatalf("second Build() error: %v", err)
	}

	if h.Size() != 2 {
		t.Fatalf("expected 2 entries after second Build, got %d", h.Size())
	}

	// Verify both specs are present.
	names := make(map[string]bool)
	for _, e := range h.Entries() {
		names[e.SpecName] = true
	}

	if !names["spec-first"] {
		t.Error("expected spec-first to be present")
	}

	if !names["spec-second"] {
		t.Error("expected spec-second to be present")
	}
}

func TestBuild_indexFileWritten(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()

	specDir := filepath.Join(specsDir, "spec-test")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	writeStateJSON(t, specDir, "completed", "M", "standard")
	writeRequestMD(t, specDir, "# Test Feature\nTest description")

	h := history.New(specsDir)
	if err := h.Build(); err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	// Verify history-index.json was written.
	indexPath := filepath.Join(specsDir, "history-index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("history-index.json was not written: %v", err)
	}

	var indexFile struct {
		IndexedAt string               `json:"indexedAt"`
		Entries   []history.IndexEntry `json:"entries"`
	}

	if err := json.Unmarshal(data, &indexFile); err != nil {
		t.Fatalf("failed to parse history-index.json: %v", err)
	}

	if indexFile.IndexedAt == "" {
		t.Error("indexedAt should not be empty")
	}

	if len(indexFile.Entries) != 1 {
		t.Errorf("expected 1 entry in index file, got %d", len(indexFile.Entries))
	}
}

func TestBuild_absentRequestMD(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()

	// Create a completed spec without request.md.
	specDir := filepath.Join(specsDir, "spec-no-request")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	writeStateJSON(t, specDir, "completed", "M", "standard")
	// No request.md written.

	h := history.New(specsDir)
	if err := h.Build(); err != nil {
		t.Fatalf("Build() should not error when request.md is missing: %v", err)
	}

	// Spec should still be indexed (with empty OneLiner and Tags).
	if h.Size() != 1 {
		t.Fatalf("expected 1 entry even without request.md, got %d", h.Size())
	}

	entry := h.Entries()[0]
	if entry.OneLiner != "" {
		t.Errorf("expected empty OneLiner when request.md absent, got %q", entry.OneLiner)
	}

	if len(entry.Tags) != 0 {
		t.Errorf("expected empty Tags when request.md absent, got %v", entry.Tags)
	}
}

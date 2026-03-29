// Package indexer implements the pure-Go replacement for build-specs-index.sh.
package indexer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// writeFile is a test helper that writes content to a file, creating parent dirs.
func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

func TestExtractRequestSummary_WithFrontmatter(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "request.md"), `---
source_type: github_issue
task_type: feature
---

# My Feature Title

This is the body text of the request.
`)

	got := extractRequestSummary(dir)
	want := "# My Feature Title This is the body text of the request."
	if got != want {
		t.Errorf("extractRequestSummary() = %q, want %q", got, want)
	}
}

func TestExtractRequestSummary_WithoutFrontmatter(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "request.md"), `# Simple Title

Body without frontmatter.
`)

	got := extractRequestSummary(dir)
	want := "# Simple Title Body without frontmatter."
	if got != want {
		t.Errorf("extractRequestSummary() = %q, want %q", got, want)
	}
}

func TestExtractRequestSummary_Truncation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Create a body longer than 200 characters (no frontmatter)
	body := "This is a very long body that should be truncated at 200 characters. " +
		"We need to make sure the implementation correctly limits the output. " +
		"Adding more text to exceed two hundred characters total here for testing purposes."
	writeFile(t, filepath.Join(dir, "request.md"), body)

	got := extractRequestSummary(dir)
	if len(got) > 200 {
		t.Errorf("extractRequestSummary() length = %d, want <= 200", len(got))
	}

	// Verify it's a prefix of the normalized body
	if len(got) != 200 {
		t.Errorf("extractRequestSummary() length = %d, want exactly 200 (body is longer)", len(got))
	}
}

func TestExtractReviewFeedback_RevisedOnly(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// review-design.md with REVISE verdict and findings
	writeFile(t, filepath.Join(dir, "review-design.md"), `## Verdict

REVISE

## Findings

**1. [CRITICAL] Missing authentication check**
**2. [MINOR] Variable naming inconsistency**
`)

	// review-tasks.md with APPROVE verdict — should be excluded
	writeFile(t, filepath.Join(dir, "review-tasks.md"), `## Verdict

APPROVE

## Findings

**1. [MINOR] Some minor note**
`)

	got := extractReviewFeedback(dir)

	if len(got) != 1 {
		t.Fatalf("extractReviewFeedback() len = %d, want 1; got %+v", len(got), got)
	}

	if got[0].Source != "review-design" {
		t.Errorf("got[0].Source = %q, want %q", got[0].Source, "review-design")
	}

	if got[0].Verdict != "REVISE" {
		t.Errorf("got[0].Verdict = %q, want %q", got[0].Verdict, "REVISE")
	}

	if len(got[0].Findings) != 2 {
		t.Errorf("got[0].Findings len = %d, want 2; got %v", len(got[0].Findings), got[0].Findings)
	}
}

func TestExtractImplOutcomes_DigitGlob(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// review-1.md — should be included
	writeFile(t, filepath.Join(dir, "review-1.md"), `## Verdict

PASS
`)

	// review-design.md — should be excluded (not digit-prefixed)
	writeFile(t, filepath.Join(dir, "review-design.md"), `## Verdict

REVISE
`)

	// review-tasks.md — should be excluded (not digit-prefixed)
	writeFile(t, filepath.Join(dir, "review-tasks.md"), `## Verdict

APPROVE
`)

	// review-2.md — should be included with FAIL
	writeFile(t, filepath.Join(dir, "review-2.md"), `## Verdict

FAIL
`)

	got := extractImplOutcomes(dir)

	if len(got) != 2 {
		t.Fatalf("extractImplOutcomes() len = %d, want 2; got %+v", len(got), got)
	}

	// Build a map for easier verification (order may vary)
	byFile := make(map[string]string)
	for _, o := range got {
		byFile[o.ReviewFile] = o.Verdict
	}

	if byFile["review-1.md"] != "PASS" {
		t.Errorf("review-1.md verdict = %q, want %q", byFile["review-1.md"], "PASS")
	}

	if byFile["review-2.md"] != "FAIL" {
		t.Errorf("review-2.md verdict = %q, want %q", byFile["review-2.md"], "FAIL")
	}

	if _, ok := byFile["review-design.md"]; ok {
		t.Error("review-design.md should be excluded but was included")
	}

	if _, ok := byFile["review-tasks.md"]; ok {
		t.Error("review-tasks.md should be excluded but was included")
	}
}

func TestExtractImplPatterns_BacktickPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "impl-1.md"), `# Task 1: Implement feature

## Files Modified

- `+"`mcp-server/indexer/specs_index.go`"+` — new file
`)

	got := extractImplPatterns(dir)

	if len(got) != 1 {
		t.Fatalf("extractImplPatterns() len = %d, want 1; got %+v", len(got), got)
	}

	if got[0].TaskTitle != "Task 1: Implement feature" {
		t.Errorf("TaskTitle = %q, want %q", got[0].TaskTitle, "Task 1: Implement feature")
	}

	if len(got[0].FilesModified) != 1 {
		t.Fatalf("FilesModified len = %d, want 1; got %v", len(got[0].FilesModified), got[0].FilesModified)
	}

	if got[0].FilesModified[0] != "mcp-server/indexer/specs_index.go" {
		t.Errorf("FilesModified[0] = %q, want %q", got[0].FilesModified[0], "mcp-server/indexer/specs_index.go")
	}
}

func TestExtractImplPatterns_PlainPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "impl-1.md"), `# Task 2: Update handlers

## Files Modified

- scripts/stop-hook.sh
`)

	got := extractImplPatterns(dir)

	if len(got) != 1 {
		t.Fatalf("extractImplPatterns() len = %d, want 1; got %+v", len(got), got)
	}

	if len(got[0].FilesModified) != 1 {
		t.Fatalf("FilesModified len = %d, want 1; got %v", len(got[0].FilesModified), got[0].FilesModified)
	}

	if got[0].FilesModified[0] != "scripts/stop-hook.sh" {
		t.Errorf("FilesModified[0] = %q, want %q", got[0].FilesModified[0], "scripts/stop-hook.sh")
	}
}

func TestExtractImplPatterns_Max5Files(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "impl-1.md"), `# Task 3: Bulk changes

## Files Modified

- file1/a.go
- file2/b.go
- file3/c.go
- file4/d.go
- file5/e.go
- file6/f.go
- file7/g.go
`)

	got := extractImplPatterns(dir)

	if len(got) != 1 {
		t.Fatalf("extractImplPatterns() len = %d, want 1; got %+v", len(got), got)
	}

	if len(got[0].FilesModified) != 5 {
		t.Errorf("FilesModified len = %d, want 5 (capped); got %v", len(got[0].FilesModified), got[0].FilesModified)
	}
}

func TestDeriveOutcome_AllCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		currentPhase       string
		currentPhaseStatus string
		want               string
	}{
		{
			name:               "abandoned_status",
			currentPhase:       "phase-5",
			currentPhaseStatus: "abandoned",
			want:               "abandoned",
		},
		{
			name:               "failed_status",
			currentPhase:       "phase-5",
			currentPhaseStatus: "failed",
			want:               "failed",
		},
		{
			name:               "post-to-source_completed",
			currentPhase:       "post-to-source",
			currentPhaseStatus: "completed",
			want:               "completed",
		},
		{
			name:               "completed_pseudo_phase",
			currentPhase:       "completed",
			currentPhaseStatus: "completed",
			want:               "completed",
		},
		{
			name:               "in_progress",
			currentPhase:       "phase-3",
			currentPhaseStatus: "in_progress",
			want:               "in_progress",
		},
		{
			name:               "empty_state_file",
			currentPhase:       "",
			currentPhaseStatus: "",
			want:               "unknown",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			stateFile := filepath.Join(dir, "state.json")

			if tc.currentPhase != "" || tc.currentPhaseStatus != "" {
				content := `{"currentPhase":"` + tc.currentPhase + `","currentPhaseStatus":"` + tc.currentPhaseStatus + `"}`
				writeFile(t, stateFile, content)
			}
			// If both are empty, don't write the state file (tests the "no state file" path)

			got := deriveOutcome(stateFile)
			if got != tc.want {
				t.Errorf("deriveOutcome(%q, %q) = %q, want %q",
					tc.currentPhase, tc.currentPhaseStatus, got, tc.want)
			}
		})
	}
}

func TestBuildSpecsIndex_Empty(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()

	count, err := BuildSpecsIndex(specsDir)
	if err != nil {
		t.Fatalf("BuildSpecsIndex() error = %v", err)
	}

	if count != 0 {
		t.Errorf("BuildSpecsIndex() count = %d, want 0", count)
	}

	// Verify index.json was written with an empty array
	data, err := os.ReadFile(filepath.Join(specsDir, "index.json"))
	if err != nil {
		t.Fatalf("ReadFile(index.json): %v", err)
	}

	var entries []any
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("Unmarshal index.json: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("index.json entries len = %d, want 0", len(entries))
	}
}

func TestBuildSpecsIndex_MultipleWorkspaces(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()

	// Workspace 1: complete with state.json and request.md
	ws1 := filepath.Join(specsDir, "20260101-feature-foo")
	if err := os.MkdirAll(ws1, 0o755); err != nil {
		t.Fatalf("MkdirAll ws1: %v", err)
	}

	writeFile(t, filepath.Join(ws1, "state.json"), `{
		"specName": "feature-foo",
		"taskType": "feature",
		"currentPhase": "post-to-source",
		"currentPhaseStatus": "completed",
		"timestamps": {"created": "2026-01-01T00:00:00Z"}
	}`)

	writeFile(t, filepath.Join(ws1, "request.md"), `# Feature Foo

Build a new feature for the system.
`)

	// Workspace 2: minimal, no state.json
	ws2 := filepath.Join(specsDir, "20260102-bugfix-bar")
	if err := os.MkdirAll(ws2, 0o755); err != nil {
		t.Fatalf("MkdirAll ws2: %v", err)
	}

	writeFile(t, filepath.Join(ws2, "request.md"), `---
task_type: bugfix
---

# Bugfix Bar

Fix a critical bug.
`)

	count, err := BuildSpecsIndex(specsDir)
	if err != nil {
		t.Fatalf("BuildSpecsIndex() error = %v", err)
	}

	if count != 2 {
		t.Errorf("BuildSpecsIndex() count = %d, want 2", count)
	}

	// Read and verify index.json
	data, err := os.ReadFile(filepath.Join(specsDir, "index.json"))
	if err != nil {
		t.Fatalf("ReadFile(index.json): %v", err)
	}

	type indexEntry struct {
		SpecName       string  `json:"specName"`
		Timestamp      string  `json:"timestamp"`
		TaskType       *string `json:"taskType"`
		RequestSummary string  `json:"requestSummary"`
		Outcome        string  `json:"outcome"`
	}

	var entries []indexEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("Unmarshal index.json: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("entries len = %d, want 2; got %+v", len(entries), entries)
	}

	// Build map by specName for easier verification
	byName := make(map[string]indexEntry)
	for _, e := range entries {
		byName[e.SpecName] = e
	}

	// Verify workspace 1
	ws1Entry, ok := byName["feature-foo"]
	if !ok {
		t.Fatalf("entry for 'feature-foo' not found; got keys: %v", reflect.ValueOf(byName).MapKeys())
	}

	if ws1Entry.Outcome != "completed" {
		t.Errorf("ws1 Outcome = %q, want %q", ws1Entry.Outcome, "completed")
	}

	if ws1Entry.Timestamp != "2026-01-01T00:00:00Z" {
		t.Errorf("ws1 Timestamp = %q, want %q", ws1Entry.Timestamp, "2026-01-01T00:00:00Z")
	}

	if ws1Entry.TaskType == nil || *ws1Entry.TaskType != "feature" {
		t.Errorf("ws1 TaskType = %v, want %q", ws1Entry.TaskType, "feature")
	}

	// Verify workspace 2 (no state.json — falls back to dir basename)
	ws2Entry, ok := byName["20260102-bugfix-bar"]
	if !ok {
		t.Fatalf("entry for '20260102-bugfix-bar' not found; got keys: %v", reflect.ValueOf(byName).MapKeys())
	}

	if ws2Entry.Outcome != "unknown" {
		t.Errorf("ws2 Outcome = %q, want %q", ws2Entry.Outcome, "unknown")
	}

	if ws2Entry.Timestamp != "unknown" {
		t.Errorf("ws2 Timestamp = %q, want %q", ws2Entry.Timestamp, "unknown")
	}
}

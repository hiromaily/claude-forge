// Package tools — tests for SearchPatternsHandler and searchPatternsWithPaths.
// These tests exercise all 7 sub-cases specified in the design:
//   (a) absent index.json
//   (b) empty-array index.json
//   (c) absent request.md (passes empty query to BM25)
//   (d) review-feedback mode top-3
//   (e) impl mode top-2 (completed-only filter)
//   (f) explicit top_k override
//   (g) task_type boost ordering
package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/search"
	"github.com/mark3labs/mcp-go/mcp"
)

// buildIndex marshals entries as JSON and writes them to indexPath.
func buildIndex(t *testing.T, indexPath string, entries []search.IndexEntry) {
	t.Helper()
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal index: %v", err)
	}
	if err := os.WriteFile(indexPath, data, 0644); err != nil {
		t.Fatalf("write index: %v", err)
	}
}

// buildRequest writes text content to requestPath.
func buildRequest(t *testing.T, requestPath, content string) {
	t.Helper()
	if err := os.WriteFile(requestPath, []byte(content), 0644); err != nil {
		t.Fatalf("write request: %v", err)
	}
}

// callSearchPatterns calls searchPatternsWithPaths with a minimal CallToolRequest
// carrying the given extra args (task_type, top_k, mode).
func callSearchPatterns(t *testing.T, indexPath, requestPath string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	res, err := searchPatternsWithPaths(context.Background(), req, indexPath, requestPath)
	if err != nil {
		t.Fatalf("searchPatternsWithPaths returned unexpected Go error: %v", err)
	}
	return res
}

// makeEntry creates a simple IndexEntry for testing.
func makeEntry(specName, requestSummary, outcome string, taskType *string, feedback []search.ReviewFeedback, patterns []search.ImplPattern) search.IndexEntry {
	return search.IndexEntry{
		SpecName:       specName,
		Timestamp:      "2024-01-01T00:00:00Z",
		TaskType:       taskType,
		RequestSummary: requestSummary,
		ReviewFeedback: feedback,
		ImplPatterns:   patterns,
		Outcome:        outcome,
	}
}

func strPtr(s string) *string { return &s }

// ---------- sub-case (a): absent index.json ----------

func TestSearchPatternsHandlerAbsentIndex(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "index.json")
	requestPath := filepath.Join(dir, "workspace", "request.md")

	res := callSearchPatterns(t, indexPath, requestPath, map[string]any{})
	if res.IsError {
		t.Errorf("absent index.json should return okText(''), got error: %v", textContent(res))
	}
	if got := textContent(res); got != "" {
		t.Errorf("absent index.json: expected empty string, got %q", got)
	}
}

// ---------- sub-case (b): empty-array index.json ----------

func TestSearchPatternsHandlerEmptyArray(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "index.json")
	requestPath := filepath.Join(dir, "workspace", "request.md")

	buildIndex(t, indexPath, []search.IndexEntry{})

	res := callSearchPatterns(t, indexPath, requestPath, map[string]any{})
	if res.IsError {
		t.Errorf("empty array index.json should return okText(''), got error: %v", textContent(res))
	}
	if got := textContent(res); got != "" {
		t.Errorf("empty array: expected empty string, got %q", got)
	}
}

// ---------- sub-case (c): absent request.md ----------

func TestSearchPatternsHandlerAbsentRequestMd(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "index.json")
	requestPath := filepath.Join(dir, "workspace", "request.md")

	entries := []search.IndexEntry{
		makeEntry("spec-a", "mcp server feature implementation", "completed", nil,
			[]search.ReviewFeedback{{Source: "review-1.md", Verdict: "pass", Findings: []string{"good code"}}},
			[]search.ImplPattern{{TaskTitle: "Task 1", FilesModified: []string{"foo.go"}}},
		),
	}
	buildIndex(t, indexPath, entries)
	// Do NOT write request.md

	// With absent request.md, empty query produces all-zero BM25 scores; zero-score entries are excluded.
	// Handler must NOT early-return before calling BM25. Result is naturally empty.
	res := callSearchPatterns(t, indexPath, requestPath, map[string]any{
		"mode": "review-feedback",
	})
	if res.IsError {
		t.Errorf("absent request.md should not return error: %v", textContent(res))
	}
	if got := textContent(res); got != "" {
		t.Errorf("absent request.md: BM25 zero-score entries excluded, expected empty string, got %q", got)
	}
}

// ---------- sub-case (d): review-feedback mode top-3 ----------

func TestSearchPatternsHandlerReviewFeedbackMode(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "index.json")
	requestPath := filepath.Join(dir, "workspace", "request.md")

	if err := os.MkdirAll(filepath.Dir(requestPath), 0755); err != nil {
		t.Fatal(err)
	}

	entries := []search.IndexEntry{
		makeEntry("spec-alpha", "implement mcp server feature with golang tools", "completed", nil,
			[]search.ReviewFeedback{
				{Source: "review-1.md", Verdict: "pass", Findings: []string{"finding one", "finding two"}},
			},
			nil,
		),
		makeEntry("spec-beta", "mcp server golang implementation patterns", "completed", nil,
			[]search.ReviewFeedback{
				{Source: "review-2.md", Verdict: "pass", Findings: []string{"another finding"}},
			},
			nil,
		),
	}
	buildIndex(t, indexPath, entries)
	buildRequest(t, requestPath, "# Implement MCP Server\n\nmcp server golang feature implementation tools patterns")

	res := callSearchPatterns(t, indexPath, requestPath, map[string]any{
		"mode":  "review-feedback",
		"top_k": float64(3),
	})
	if res.IsError {
		t.Errorf("review-feedback mode returned error: %v", textContent(res))
	}
	got := textContent(res)
	if !strings.HasPrefix(got, "## Past Review Feedback (from similar pipelines)\n\n") {
		t.Errorf("review-feedback header missing in output:\n%s", got)
	}
	// Each finding should produce a bullet line matching the format string.
	if !strings.Contains(got, "- **[pass]** finding one _(from: review-1.md)_") {
		t.Errorf("review-feedback bullet format not found in output:\n%s", got)
	}
}

// ---------- sub-case (e): impl mode top-2, completed-only filter ----------

func TestSearchPatternsHandlerImplMode(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "index.json")
	requestPath := filepath.Join(dir, "workspace", "request.md")

	if err := os.MkdirAll(filepath.Dir(requestPath), 0755); err != nil {
		t.Fatal(err)
	}

	ft := strPtr("feature")
	entries := []search.IndexEntry{
		makeEntry("spec-completed", "implement mcp server golang feature tools patterns", "completed", ft,
			nil,
			[]search.ImplPattern{
				{TaskTitle: "Task 1: Add handler", FilesModified: []string{"tools/handler.go", "tools/registry.go"}},
			},
		),
		makeEntry("spec-abandoned", "implement mcp server golang feature tools patterns", "abandoned", ft,
			nil,
			[]search.ImplPattern{
				{TaskTitle: "Task 2: Abandoned task", FilesModified: []string{"tools/other.go"}},
			},
		),
	}
	buildIndex(t, indexPath, entries)
	buildRequest(t, requestPath, "# Feature\n\nimplement mcp server golang feature tools patterns")

	res := callSearchPatterns(t, indexPath, requestPath, map[string]any{
		"mode":      "impl",
		"top_k":     float64(2),
		"task_type": "feature",
	})
	if res.IsError {
		t.Errorf("impl mode returned error: %v", textContent(res))
	}
	got := textContent(res)
	if !strings.HasPrefix(got, "## Similar Past Implementations (from similar pipelines)\n\n") {
		t.Errorf("impl header missing in output:\n%s", got)
	}
	// completed entry's patterns should appear
	if !strings.Contains(got, "Task 1: Add handler") {
		t.Errorf("impl mode: completed entry pattern not found in output:\n%s", got)
	}
	// abandoned entry should NOT appear
	if strings.Contains(got, "Abandoned task") {
		t.Errorf("impl mode: abandoned entry should be filtered out, but found in output:\n%s", got)
	}
	// impl bullet format: "- **%s** (%s): %s — files: %s\n"
	if !strings.Contains(got, "- **Task 1: Add handler** (spec-completed):") {
		t.Errorf("impl bullet format not found in output:\n%s", got)
	}
	if !strings.Contains(got, "files: tools/handler.go, tools/registry.go") {
		t.Errorf("impl files list not found in output:\n%s", got)
	}
}

// ---------- sub-case (f): explicit top_k override ----------

func TestSearchPatternsHandlerTopKOverride(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "index.json")
	requestPath := filepath.Join(dir, "workspace", "request.md")

	if err := os.MkdirAll(filepath.Dir(requestPath), 0755); err != nil {
		t.Fatal(err)
	}

	// Create many entries that will score positively
	entries := make([]search.IndexEntry, 5)
	for i := range entries {
		entries[i] = makeEntry(
			"spec-"+string(rune('a'+i)),
			"implement mcp server golang feature tools patterns",
			"completed",
			nil,
			[]search.ReviewFeedback{
				{Source: "review.md", Verdict: "pass", Findings: []string{"finding " + string(rune('a'+i))}},
			},
			nil,
		)
	}
	buildIndex(t, indexPath, entries)
	buildRequest(t, requestPath, "implement mcp server golang feature tools patterns")

	// top_k=1 should limit to 1 result
	res := callSearchPatterns(t, indexPath, requestPath, map[string]any{
		"mode":  "review-feedback",
		"top_k": float64(1),
	})
	if res.IsError {
		t.Errorf("top_k=1 override returned error: %v", textContent(res))
	}
	got := textContent(res)
	// Count bullets — there should be exactly 1 finding shown
	bulletCount := strings.Count(got, "\n- **[")
	if bulletCount != 1 {
		t.Errorf("top_k=1 override: expected 1 bullet, got %d in output:\n%s", bulletCount, got)
	}

	// Also test top_k=0 uses mode-specific default (3 for review-feedback)
	res2 := callSearchPatterns(t, indexPath, requestPath, map[string]any{
		"mode":  "review-feedback",
		"top_k": float64(0),
	})
	if res2.IsError {
		t.Errorf("top_k=0 default returned error: %v", textContent(res2))
	}
	got2 := textContent(res2)
	bulletCount2 := strings.Count(got2, "\n- **[")
	if bulletCount2 != 3 {
		t.Errorf("top_k=0 review-feedback default: expected 3 bullets, got %d in output:\n%s", bulletCount2, got2)
	}
}

// ---------- sub-case (g): task_type boost ordering ----------

func TestSearchPatternsHandlerTaskTypeBoost(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "index.json")
	requestPath := filepath.Join(dir, "workspace", "request.md")

	if err := os.MkdirAll(filepath.Dir(requestPath), 0755); err != nil {
		t.Fatal(err)
	}

	ft := strPtr("feature")
	bx := strPtr("bugfix")
	// Both entries have the same requestSummary to give equal BM25 scores,
	// but spec-feature matches task_type "feature" and will be boosted.
	entries := []search.IndexEntry{
		makeEntry("spec-bugfix", "implement mcp server golang feature tools patterns review", "completed", bx,
			[]search.ReviewFeedback{
				{Source: "review-bugfix.md", Verdict: "pass", Findings: []string{"bugfix finding"}},
			},
			nil,
		),
		makeEntry("spec-feature", "implement mcp server golang feature tools patterns review", "completed", ft,
			[]search.ReviewFeedback{
				{Source: "review-feature.md", Verdict: "pass", Findings: []string{"feature finding"}},
			},
			nil,
		),
	}
	buildIndex(t, indexPath, entries)
	buildRequest(t, requestPath, "implement mcp server golang feature tools patterns review")

	res := callSearchPatterns(t, indexPath, requestPath, map[string]any{
		"mode":      "review-feedback",
		"top_k":     float64(2),
		"task_type": "feature",
	})
	if res.IsError {
		t.Errorf("task_type boost test returned error: %v", textContent(res))
	}
	got := textContent(res)
	// spec-feature (boosted) should appear before spec-bugfix (not boosted)
	featurePos := strings.Index(got, "review-feature.md")
	bugfixPos := strings.Index(got, "review-bugfix.md")
	if featurePos == -1 || bugfixPos == -1 {
		t.Fatalf("task_type boost: expected both entries in output:\n%s", got)
	}
	if featurePos >= bugfixPos {
		t.Errorf("task_type boost: feature entry should appear before bugfix entry\noutput:\n%s", got)
	}
}

// ---------- YAML frontmatter stripping ----------

func TestSearchPatternsHandlerFrontmatterStrip(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "index.json")
	requestPath := filepath.Join(dir, "workspace", "request.md")

	if err := os.MkdirAll(filepath.Dir(requestPath), 0755); err != nil {
		t.Fatal(err)
	}

	entries := []search.IndexEntry{
		makeEntry("spec-alpha", "implement mcp server golang feature tools patterns", "completed", nil,
			[]search.ReviewFeedback{
				{Source: "review-1.md", Verdict: "pass", Findings: []string{"some finding"}},
			},
			nil,
		),
	}
	buildIndex(t, indexPath, entries)
	// Write request.md with YAML frontmatter that doesn't match the corpus;
	// only the body content should be used for BM25 scoring.
	buildRequest(t, requestPath, "---\nsource_type: github_issue\ntask_type: feature\n---\n\n# Title\n\nimplement mcp server golang feature tools patterns")

	res := callSearchPatterns(t, indexPath, requestPath, map[string]any{
		"mode":  "review-feedback",
		"top_k": float64(1),
	})
	if res.IsError {
		t.Errorf("frontmatter strip test returned error: %v", textContent(res))
	}
	got := textContent(res)
	// If frontmatter is stripped, the body matches the corpus and we get a result.
	if got == "" {
		t.Errorf("frontmatter strip: expected non-empty result when body matches corpus, got empty string")
	}
	if !strings.Contains(got, "some finding") {
		t.Errorf("frontmatter strip: expected result with 'some finding', got:\n%s", got)
	}
}

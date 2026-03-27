// Package tools — integration tests for SearchPatternsHandler / searchPatternsWithPaths.
// TestSearchPatternsHandler covers all 7 sub-cases specified in design Section 4:
//
//	(a) absent index.json  → okText("")  (handler returns before BM25)
//	(b) empty-array index.json → okText("")
//	(c) absent request.md → handler passes empty string to BM25 (zero scores, NOT early-return)
//	(d) review-feedback mode top-3 → exact markdown format
//	(e) impl mode top-2 (completed-only filter) → exact markdown format
//	(f) explicit top_k override including top_k=0 (verifies mode-specific default path)
//	(g) task_type boost ordering → boosted entry appears first
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hiromaily/claude-forge/mcp-server/search"
)

// ---------- helpers for search_patterns tests ----------

// buildIndex marshals entries as JSON and writes them to indexPath.
func buildIndex(t *testing.T, indexPath string, entries []search.IndexEntry) {
	t.Helper()
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal index: %v", err)
	}
	if err := os.WriteFile(indexPath, data, 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
}

// buildRequest writes text content to requestPath, creating parent dirs as needed.
func buildRequest(t *testing.T, requestPath, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(requestPath), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(requestPath), err)
	}
	if err := os.WriteFile(requestPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write request: %v", err)
	}
}

// callSearchPatterns invokes searchPatternsWithPaths directly with the given args.
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

// makeEntry constructs an IndexEntry for use in test fixtures.
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

// ---------- TestSearchPatternsHandler ----------

func TestSearchPatternsHandler(t *testing.T) {
	// (a) absent index.json — handler returns okText("") at step 3, before BM25.
	t.Run("absent_index_json", func(t *testing.T) {
		dir := t.TempDir()
		indexPath := filepath.Join(dir, "index.json") // does not exist
		requestPath := filepath.Join(dir, "request.md")
		buildRequest(t, requestPath, "golang mcp server pattern scoring")

		res := callSearchPatterns(t, indexPath, requestPath, map[string]any{})
		if res.IsError {
			t.Errorf("absent index.json should return okText(''), got error: %v", textContent(res))
		}
		if got := textContent(res); got != "" {
			t.Errorf("absent index.json: expected empty string, got %q", got)
		}
	})

	// (b) empty-array index.json — handler returns okText("").
	t.Run("empty_array_index_json", func(t *testing.T) {
		dir := t.TempDir()
		indexPath := filepath.Join(dir, "index.json")
		requestPath := filepath.Join(dir, "request.md")
		buildIndex(t, indexPath, []search.IndexEntry{})
		buildRequest(t, requestPath, "golang mcp server pattern scoring")

		res := callSearchPatterns(t, indexPath, requestPath, map[string]any{})
		if res.IsError {
			t.Errorf("empty array index.json should return okText(''), got error: %v", textContent(res))
		}
		if got := textContent(res); got != "" {
			t.Errorf("empty array: expected empty string, got %q", got)
		}
	})

	// (c) absent request.md — handler passes empty string to BM25 (zero scores, NOT early-return).
	// An empty query returns zero-score results; all entries are excluded; handler returns okText("").
	t.Run("absent_request_md", func(t *testing.T) {
		dir := t.TempDir()
		indexPath := filepath.Join(dir, "index.json")
		requestPath := filepath.Join(dir, "workspace", "request.md") // does not exist

		entries := []search.IndexEntry{
			makeEntry("spec-a", "mcp server feature implementation", "completed", nil,
				[]search.ReviewFeedback{{Source: "review-1.md", Verdict: "pass", Findings: []string{"good code"}}},
				nil,
			),
		}
		buildIndex(t, indexPath, entries)
		// Do NOT write request.md

		res := callSearchPatterns(t, indexPath, requestPath, map[string]any{
			"mode": "review-feedback",
		})
		if res.IsError {
			t.Errorf("absent request.md should not return error: %v", textContent(res))
		}
		// Empty query → all-zero BM25 scores → all entries excluded → empty output.
		if got := textContent(res); got != "" {
			t.Errorf("absent request.md: expected empty string (zero-score entries excluded), got %q", got)
		}
	})

	// (d) review-feedback mode top-3 — correct markdown format with exact string equality on constants.
	t.Run("review_feedback_mode_top3", func(t *testing.T) {
		dir := t.TempDir()
		indexPath := filepath.Join(dir, "index.json")
		requestPath := filepath.Join(dir, "workspace", "request.md")

		entries := []search.IndexEntry{
			makeEntry("spec-alpha", "implement mcp server feature with golang tools patterns", "completed", nil,
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

		// Assert exact header constant.
		if !strings.HasPrefix(got, reviewFeedbackHeader) {
			t.Errorf("review-feedback: output does not start with expected header constant\ngot: %q", got)
		}
		// Assert exact bullet format for a known finding.
		wantBullet := fmt.Sprintf(reviewFeedbackBullet, "review-1.md", "finding one", "spec-alpha")
		if !strings.Contains(got, wantBullet) {
			t.Errorf("review-feedback: expected bullet %q in output:\n%s", wantBullet, got)
		}
	})

	// (e) impl mode top-2 (completed-only filter) — correct markdown format.
	t.Run("impl_mode_top2_completed_only", func(t *testing.T) {
		dir := t.TempDir()
		indexPath := filepath.Join(dir, "index.json")
		requestPath := filepath.Join(dir, "workspace", "request.md")

		ft := new("feature")
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
		buildRequest(t, requestPath, "implement mcp server golang feature tools patterns")

		res := callSearchPatterns(t, indexPath, requestPath, map[string]any{
			"mode":      "impl",
			"top_k":     float64(2),
			"task_type": "feature",
		})
		if res.IsError {
			t.Errorf("impl mode returned error: %v", textContent(res))
		}
		got := textContent(res)

		// Assert exact impl header constant.
		if !strings.HasPrefix(got, implHeader) {
			t.Errorf("impl mode: output does not start with expected header constant\ngot: %q", got)
		}
		// Assert exact bullet format for completed entry.
		wantBullet := fmt.Sprintf(implBullet,
			"spec-completed", *ft,
			"Task 1: Add handler",
			"tools/handler.go, tools/registry.go")
		if !strings.Contains(got, wantBullet) {
			t.Errorf("impl mode: expected bullet %q in output:\n%s", wantBullet, got)
		}
		// Abandoned entry must not appear.
		if strings.Contains(got, "Abandoned task") {
			t.Errorf("impl mode: abandoned entry should be filtered out, found in output:\n%s", got)
		}
	})

	// (f) explicit top_k override including top_k=0 (verifies mode-specific default path).
	t.Run("top_k_override", func(t *testing.T) {
		dir := t.TempDir()
		indexPath := filepath.Join(dir, "index.json")
		requestPath := filepath.Join(dir, "workspace", "request.md")

		// 5 identical entries — each will score identically against the query.
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

		// top_k=1 limits to 1 result.
		res1 := callSearchPatterns(t, indexPath, requestPath, map[string]any{
			"mode":  "review-feedback",
			"top_k": float64(1),
		})
		if res1.IsError {
			t.Errorf("top_k=1: unexpected error: %v", textContent(res1))
		}
		got1 := textContent(res1)
		bulletCount1 := strings.Count(got1, "\n- **[")
		if bulletCount1 != 1 {
			t.Errorf("top_k=1: expected 1 bullet, got %d in output:\n%s", bulletCount1, got1)
		}

		// top_k=0 uses mode-specific default: 3 for review-feedback.
		res0 := callSearchPatterns(t, indexPath, requestPath, map[string]any{
			"mode":  "review-feedback",
			"top_k": float64(0),
		})
		if res0.IsError {
			t.Errorf("top_k=0: unexpected error: %v", textContent(res0))
		}
		got0 := textContent(res0)
		bulletCount0 := strings.Count(got0, "\n- **[")
		if bulletCount0 != 3 {
			t.Errorf("top_k=0 review-feedback default (3): expected 3 bullets, got %d in output:\n%s", bulletCount0, got0)
		}
	})

	// (g) task_type boost ordering — matching task_type entry appears first.
	t.Run("task_type_boost_ordering", func(t *testing.T) {
		dir := t.TempDir()
		indexPath := filepath.Join(dir, "index.json")
		requestPath := filepath.Join(dir, "workspace", "request.md")

		ft := new("feature")
		bx := new("bugfix")
		// Both entries have identical requestSummary → equal BM25 scores.
		// spec-feature matches task_type "feature" and receives a 2× boost.
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
			t.Errorf("task_type boost returned error: %v", textContent(res))
		}
		got := textContent(res)
		if got == "" {
			t.Fatalf("task_type boost: expected non-empty output")
		}
		// spec-feature (boosted) must appear before spec-bugfix (not boosted).
		featurePos := strings.Index(got, "review-feature.md")
		bugfixPos := strings.Index(got, "review-bugfix.md")
		if featurePos == -1 || bugfixPos == -1 {
			t.Fatalf("task_type boost: expected both entries in output:\n%s", got)
		}
		if featurePos >= bugfixPos {
			t.Errorf("task_type boost: feature entry (boosted) should appear before bugfix entry\noutput:\n%s", got)
		}
	})
}

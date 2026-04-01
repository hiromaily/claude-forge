// Package tools — history_search handler tests.
package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hiromaily/claude-forge/mcp-server/internal/history"
)

// makeHistorySearchReq builds a mcp.CallToolRequest for the history_search tool.
func makeHistorySearchReq(query string, limit int, taskTypeFilter string) mcp.CallToolRequest {
	args := map[string]any{
		"query": query,
	}
	if limit > 0 {
		args["limit"] = float64(limit)
	}
	if taskTypeFilter != "" {
		args["task_type_filter"] = taskTypeFilter
	}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	return req
}

// buildHistoryFixtureSpec writes a minimal state.json and request.md into
// specsDir/<specName>/ so that history.HistoryIndex.Build() will index it.
func buildHistoryFixtureSpec(t *testing.T, specsDir, specName, taskType, outcome string) {
	t.Helper()
	dir := filepath.Join(specsDir, specName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir fixture spec %s: %v", specName, err)
	}
	created := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	lastUpdated := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	stateData := `{"specName":"` + specName + `","currentPhase":"` + outcome + `",` +
		`"taskType":"` + taskType + `",` +
		`"timestamps":{"created":"` + created + `","lastUpdated":"` + lastUpdated + `"}}`
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(stateData), 0o600); err != nil {
		t.Fatalf("write state.json for %s: %v", specName, err)
	}
	reqData := "# " + specName + " request\n\nThis is a sample request for " + specName +
		" involving " + taskType + " work.\n"
	if err := os.WriteFile(filepath.Join(dir, "request.md"), []byte(reqData), 0o600); err != nil {
		t.Fatalf("write request.md for %s: %v", specName, err)
	}
}

func TestHistorySearchHandler_empty(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()
	idx := history.New(specsDir)
	// Do not call Build — index is empty.

	req := makeHistorySearchReq("any query", 0, "")
	result, err := historySearchWithIndex(t.Context(), req, idx, specsDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if result.IsError {
		t.Fatalf("unexpected MCP error: %v", textContent(result))
	}

	var resp struct {
		Results   []json.RawMessage `json:"results"`
		IndexSize int               `json:"index_size"`
	}
	if err := json.Unmarshal([]byte(textContent(result)), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(resp.Results))
	}
	if resp.IndexSize != 0 {
		t.Errorf("expected index_size 0, got %d", resp.IndexSize)
	}
}

func TestHistorySearchHandler_results(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()
	buildHistoryFixtureSpec(t, specsDir, "spec-alpha", "feature", "completed")
	buildHistoryFixtureSpec(t, specsDir, "spec-beta", "bugfix", "completed")
	buildHistoryFixtureSpec(t, specsDir, "spec-gamma", "feature", "abandoned")

	idx := history.New(specsDir)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if idx.Size() != 3 {
		t.Fatalf("expected 3 indexed specs, got %d", idx.Size())
	}

	req := makeHistorySearchReq("feature request work", 0, "")
	result, err := historySearchWithIndex(t.Context(), req, idx, specsDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected MCP error: %v", textContent(result))
	}

	var resp struct {
		Results   []json.RawMessage `json:"results"`
		IndexSize int               `json:"index_size"`
	}
	if err := json.Unmarshal([]byte(textContent(result)), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.IndexSize != 3 {
		t.Errorf("expected index_size 3, got %d", resp.IndexSize)
	}
	if len(resp.Results) == 0 {
		t.Error("expected at least one result")
	}
}

func TestHistorySearchHandler_taskTypeFilter(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()
	buildHistoryFixtureSpec(t, specsDir, "spec-feature-1", "feature", "completed")
	buildHistoryFixtureSpec(t, specsDir, "spec-bugfix-1", "bugfix", "completed")
	buildHistoryFixtureSpec(t, specsDir, "spec-bugfix-2", "bugfix", "completed")

	idx := history.New(specsDir)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}

	req := makeHistorySearchReq("fix bug", 10, "bugfix")
	result, err := historySearchWithIndex(t.Context(), req, idx, specsDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected MCP error: %v", textContent(result))
	}

	var resp struct {
		Results []struct {
			TaskType string `json:"task_type"`
		} `json:"results"`
		IndexSize int `json:"index_size"`
	}
	if err := json.Unmarshal([]byte(textContent(result)), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.IndexSize != 3 {
		t.Errorf("expected index_size 3, got %d", resp.IndexSize)
	}
	for _, r := range resp.Results {
		if r.TaskType != "bugfix" {
			t.Errorf("expected task_type bugfix, got %q", r.TaskType)
		}
	}
}

func TestHistorySearchHandler_limitDefault(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()
	// Create 5 specs so we have more than the default limit of 3.
	for i := range 5 {
		name := "spec-limit-" + string(rune('a'+i))
		buildHistoryFixtureSpec(t, specsDir, name, "feature", "completed")
	}

	idx := history.New(specsDir)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}

	// No limit parameter — should default to 3.
	req := makeHistorySearchReq("feature work sample", 0, "")
	result, err := historySearchWithIndex(t.Context(), req, idx, specsDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected MCP error: %v", textContent(result))
	}

	var resp struct {
		Results   []json.RawMessage `json:"results"`
		IndexSize int               `json:"index_size"`
	}
	if err := json.Unmarshal([]byte(textContent(result)), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.IndexSize != 5 {
		t.Errorf("expected index_size 5, got %d", resp.IndexSize)
	}
	if len(resp.Results) > 3 {
		t.Errorf("expected at most 3 results (default limit), got %d", len(resp.Results))
	}
}

func TestHistorySearchHandler_limitOverride(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()
	buildHistoryFixtureSpec(t, specsDir, "spec-one", "feature", "completed")
	buildHistoryFixtureSpec(t, specsDir, "spec-two", "feature", "completed")
	buildHistoryFixtureSpec(t, specsDir, "spec-three", "feature", "completed")

	idx := history.New(specsDir)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Explicit limit=1.
	req := makeHistorySearchReq("feature", 1, "")
	result, err := historySearchWithIndex(t.Context(), req, idx, specsDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected MCP error: %v", textContent(result))
	}

	var resp struct {
		Results   []json.RawMessage `json:"results"`
		IndexSize int               `json:"index_size"`
	}
	if err := json.Unmarshal([]byte(textContent(result)), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Results) > 1 {
		t.Errorf("expected at most 1 result, got %d", len(resp.Results))
	}
}

// Package tools — history_get_friction_map handler tests.
package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hiromaily/claude-forge/mcp-server/internal/intelligence/history"
)

// makeHistoryGetFrictionMapReq builds a mcp.CallToolRequest for the history_get_friction_map tool.
func makeHistoryGetFrictionMapReq() mcp.CallToolRequest {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}
	return req
}

func TestHistoryGetFrictionMapHandler_empty(t *testing.T) {
	t.Parallel()

	kb := history.NewKnowledgeBase(t.TempDir())

	req := makeHistoryGetFrictionMapReq()
	result, err := historyGetFrictionMapWithKB(t.Context(), req, kb)
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
		FrictionPoints       []json.RawMessage `json:"friction_points"`
		TotalReportsAnalyzed int               `json:"total_reports_analyzed"`
	}
	if err := json.Unmarshal([]byte(textContent(result)), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.FrictionPoints) != 0 {
		t.Errorf("expected 0 friction points, got %d", len(resp.FrictionPoints))
	}
	if resp.TotalReportsAnalyzed != 0 {
		t.Errorf("expected total_reports_analyzed 0, got %d", resp.TotalReportsAnalyzed)
	}
}

// TestHistoryGetFrictionMapHandler_totalReportsAnalyzed verifies AC-2: the JSON response
// contains total_reports_analyzed equal to the value stored in the FrictionMap.
func TestHistoryGetFrictionMapHandler_totalReportsAnalyzed(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()

	// Create two spec sub-directories each with an improvement.md.
	for _, sub := range []string{"spec-alpha", "spec-beta"} {
		dir := filepath.Join(specsDir, sub)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("MkdirAll %s: %v", sub, err)
		}
		content := "- missing error handling in the parser\n- test coverage is insufficient\n"
		if err := os.WriteFile(filepath.Join(dir, "improvement.md"), []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile improvement.md for %s: %v", sub, err)
		}
	}

	kb := history.NewKnowledgeBase(specsDir)
	if err := kb.Friction.Build(); err != nil {
		t.Fatalf("FrictionMap.Build: %v", err)
	}

	wantTotal := kb.Friction.TotalReportsAnalyzed()
	if wantTotal == 0 {
		t.Fatal("expected FrictionMap.TotalReportsAnalyzed > 0 after Build")
	}

	req := makeHistoryGetFrictionMapReq()
	result, err := historyGetFrictionMapWithKB(t.Context(), req, kb)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected MCP error: %v", textContent(result))
	}

	var resp struct {
		FrictionPoints       []json.RawMessage `json:"friction_points"`
		TotalReportsAnalyzed int               `json:"total_reports_analyzed"`
	}
	if err := json.Unmarshal([]byte(textContent(result)), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if resp.TotalReportsAnalyzed != wantTotal {
		t.Errorf("total_reports_analyzed: got %d, want %d", resp.TotalReportsAnalyzed, wantTotal)
	}
}

func TestHistoryGetFrictionMapHandler_frictionPoints(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()

	dir := filepath.Join(specsDir, "spec-one")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := "- unchecked error return causes silent failure\n- missing documentation for exported functions\n"
	if err := os.WriteFile(filepath.Join(dir, "improvement.md"), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	kb := history.NewKnowledgeBase(specsDir)
	if err := kb.Friction.Build(); err != nil {
		t.Fatalf("FrictionMap.Build: %v", err)
	}

	req := makeHistoryGetFrictionMapReq()
	result, err := historyGetFrictionMapWithKB(t.Context(), req, kb)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected MCP error: %v", textContent(result))
	}

	var resp struct {
		FrictionPoints []struct {
			Category    string `json:"category"`
			Description string `json:"description"`
			Frequency   int    `json:"frequency"`
			Mitigation  string `json:"mitigation"`
		} `json:"friction_points"`
		TotalReportsAnalyzed int `json:"total_reports_analyzed"`
	}
	if err := json.Unmarshal([]byte(textContent(result)), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if len(resp.FrictionPoints) == 0 {
		t.Error("expected at least one friction point, got 0")
	}
	// Verify each friction point has a non-empty category and description.
	for i, fp := range resp.FrictionPoints {
		if fp.Category == "" {
			t.Errorf("friction_points[%d].category is empty", i)
		}
		if fp.Description == "" {
			t.Errorf("friction_points[%d].description is empty", i)
		}
		if fp.Frequency < 1 {
			t.Errorf("friction_points[%d].frequency: got %d, want >= 1", i, fp.Frequency)
		}
	}
	if resp.TotalReportsAnalyzed != 1 {
		t.Errorf("total_reports_analyzed: got %d, want 1", resp.TotalReportsAnalyzed)
	}
}

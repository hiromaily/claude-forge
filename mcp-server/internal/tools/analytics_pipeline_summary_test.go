// Package tools — tests for AnalyticsPipelineSummaryHandler.
package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hiromaily/claude-forge/mcp-server/internal/analytics"
)

// TestAnalyticsPipelineSummaryHandler_nil_collector verifies that a nil
// collector returns an MCP error result (IsError == true).
func TestAnalyticsPipelineSummaryHandler_nil_collector(t *testing.T) {
	t.Parallel()

	handler := AnalyticsPipelineSummaryHandler(nil)
	req := mcp.CallToolRequest{}

	result, err := handler(t.Context(), req)
	if err != nil {
		t.Fatalf("unexpected handler error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError == true for nil collector, got false")
	}

	var sb strings.Builder
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	errText := sb.String()
	if !strings.Contains(errText, "collector not available") {
		t.Errorf("expected error text to contain \"collector not available\", got: %q", errText)
	}
}

// TestAnalyticsPipelineSummaryHandler_missing_workspace verifies that a
// non-nil collector with a missing workspace returns an MCP error result.
func TestAnalyticsPipelineSummaryHandler_missing_workspace(t *testing.T) {
	t.Parallel()

	col := analytics.NewCollector("")
	handler := AnalyticsPipelineSummaryHandler(col)
	req := mcp.CallToolRequest{}

	result, err := handler(t.Context(), req)
	if err != nil {
		t.Fatalf("unexpected handler error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError == true when workspace is missing, got false")
	}
}

// TestAnalyticsPipelineSummaryHandler_happy_path writes a minimal state.json
// fixture to t.TempDir() and verifies the handler returns valid JSON with the
// expected pipeline summary fields.
func TestAnalyticsPipelineSummaryHandler_happy_path(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Write a minimal state.json fixture.
	stateJSON := `{
		"specName": "test-pipeline",
		"taskType": "feature",
		"effort": "M",
		"flowTemplate": "standard",
		"currentPhase": "completed",
		"status": "completed",
		"phaseLog": [
			{"phase": "phase-1", "tokens": 1000, "duration_ms": 5000, "model": "sonnet"},
			{"phase": "phase-2", "tokens": 2000, "duration_ms": 10000, "model": "sonnet"}
		],
		"completedPhases": ["phase-1", "phase-2"],
		"skippedPhases": ["phase-3b"],
		"tasks": {}
	}`

	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(stateJSON), 0o600); err != nil {
		t.Fatalf("failed to write state.json fixture: %v", err)
	}

	col := analytics.NewCollector("")
	handler := AnalyticsPipelineSummaryHandler(col)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"workspace": dir,
	}

	result, err := handler(t.Context(), req)
	if err != nil {
		t.Fatalf("unexpected handler error: %v", err)
	}
	if result.IsError {
		var eb strings.Builder
		for _, c := range result.Content {
			if tc, ok := c.(mcp.TextContent); ok {
				eb.WriteString(tc.Text)
			}
		}
		t.Fatalf("unexpected MCP error: %s", eb.String())
	}

	// Extract text content.
	var rb strings.Builder
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			rb.WriteString(tc.Text)
		}
	}
	responseText := rb.String()

	// Verify the response is valid JSON.
	var parsed map[string]any
	if err := json.Unmarshal([]byte(responseText), &parsed); err != nil {
		t.Fatalf("response is not valid JSON: %v\nresponse: %s", err, responseText)
	}

	// Verify expected fields are present.
	for _, field := range []string{
		"pipeline", "effort", "flow_template",
		"total_tokens", "total_duration_ms", "estimated_cost_usd",
		"phases_executed", "phases_skipped", "retries", "review_findings",
	} {
		if _, ok := parsed[field]; !ok {
			t.Errorf("expected field %q in response; got keys: %v", field, summaryMapKeys(parsed))
		}
	}

	// Verify specific computed values.
	if v, ok := parsed["pipeline"].(string); !ok || v != "test-pipeline" {
		t.Errorf("expected pipeline == \"test-pipeline\", got: %v", parsed["pipeline"])
	}
	if v, ok := parsed["total_tokens"].(float64); !ok || int(v) != 3000 {
		t.Errorf("expected total_tokens == 3000, got: %v", parsed["total_tokens"])
	}
	if v, ok := parsed["phases_executed"].(float64); !ok || int(v) != 2 {
		t.Errorf("expected phases_executed == 2, got: %v", parsed["phases_executed"])
	}
	if v, ok := parsed["phases_skipped"].(float64); !ok || int(v) != 1 {
		t.Errorf("expected phases_skipped == 1, got: %v", parsed["phases_skipped"])
	}
}

// summaryMapKeys returns the keys of a map[string]any for test error messages.
func summaryMapKeys(m map[string]any) []string {
	result := make([]string, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	return result
}

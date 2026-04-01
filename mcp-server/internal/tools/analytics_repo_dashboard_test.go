// Package tools — tests for AnalyticsRepoDashboardHandler.
package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hiromaily/claude-forge/mcp-server/internal/analytics"
)

// TestAnalyticsRepoDashboardHandler_nil_reporter verifies that a nil reporter
// returns an MCP error result (IsError == true).
func TestAnalyticsRepoDashboardHandler_nil_reporter(t *testing.T) {
	t.Parallel()

	handler := AnalyticsRepoDashboardHandler(nil)
	req := mcp.CallToolRequest{}

	result, err := handler(t.Context(), req)
	if err != nil {
		t.Fatalf("unexpected handler error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError == true for nil reporter, got false")
	}
}

// TestAnalyticsRepoDashboardHandler_empty_specs verifies that a reporter with
// an empty specs directory returns valid JSON with total_pipelines == 0.
func TestAnalyticsRepoDashboardHandler_empty_specs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rep := analytics.NewReporter(dir, nil)
	handler := AnalyticsRepoDashboardHandler(rep)
	req := mcp.CallToolRequest{}

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

	// Verify total_pipelines == 0.
	totalPipelines, ok := parsed["total_pipelines"]
	if !ok {
		t.Fatalf("expected field \"total_pipelines\" in response, keys: %v", dashboardMapKeys(parsed))
	}
	if v, ok := totalPipelines.(float64); !ok || v != 0 {
		t.Errorf("expected total_pipelines == 0, got: %v", totalPipelines)
	}

	// Verify expected top-level fields are present.
	for _, field := range []string{
		"total_pipelines",
		"completed",
		"abandoned",
		"total_tokens",
		"estimated_total_cost_usd",
		"review_pass_rate",
		"avg_retries_per_pipeline",
		"most_common_findings",
	} {
		if _, ok := parsed[field]; !ok {
			t.Errorf("expected field %q in response, keys: %v", field, dashboardMapKeys(parsed))
		}
	}
}

// dashboardMapKeys returns the keys of a map[string]any for test error messages.
func dashboardMapKeys(m map[string]any) []string {
	result := make([]string, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	return result
}

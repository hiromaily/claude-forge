// Package tools — tests for AnalyticsEstimateHandler.
package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hiromaily/claude-forge/mcp-server/analytics"
)

// TestAnalyticsEstimateHandler_NilEstimator verifies that a nil estimator
// returns an MCP error result (IsError=true).
func TestAnalyticsEstimateHandler_NilEstimator(t *testing.T) {
	t.Parallel()

	handler := AnalyticsEstimateHandler(nil)
	req := mcp.CallToolRequest{}

	result, err := handler(t.Context(), req)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsError {
		t.Error("expected IsError=true for nil estimator, got false")
	}
}

// TestAnalyticsEstimateHandler_EmptySpecs verifies that the handler returns
// confidence="low" and sample_size=0 when the specs directory is empty.
func TestAnalyticsEstimateHandler_EmptySpecs(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()
	est := analytics.NewEstimator(specsDir)
	handler := AnalyticsEstimateHandler(est)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"task_type": "feature",
		"effort":    "M",
	}

	result, err := handler(t.Context(), req)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
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

	// Parse and verify specific fields.
	var out map[string]any
	if err := json.Unmarshal([]byte(responseText), &out); err != nil {
		t.Fatalf("response is not valid JSON: %v — raw: %s", err, responseText)
	}

	sampleSize, ok := out["sample_size"].(float64)
	if !ok {
		t.Fatalf("sample_size missing or wrong type: %v", out["sample_size"])
	}
	if sampleSize != 0 {
		t.Errorf("got sample_size=%v, want 0", sampleSize)
	}

	confidence, ok := out["confidence"].(string)
	if !ok {
		t.Fatalf("confidence missing or wrong type: %v", out["confidence"])
	}
	if confidence != "low" {
		t.Errorf("got confidence=%q, want %q", confidence, "low")
	}
}

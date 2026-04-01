// Package tools — history_get_patterns handler tests.
package tools

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hiromaily/claude-forge/mcp-server/internal/history"
	"github.com/hiromaily/claude-forge/mcp-server/internal/orchestrator"
)

// makeHistoryGetPatternsReq builds a mcp.CallToolRequest for the history_get_patterns tool.
func makeHistoryGetPatternsReq(agentFilter, severityFilter string, limit int) mcp.CallToolRequest {
	args := map[string]any{}
	if agentFilter != "" {
		args["agent_filter"] = agentFilter
	}
	if severityFilter != "" {
		args["severity_filter"] = severityFilter
	}
	if limit > 0 {
		args["limit"] = float64(limit)
	}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	return req
}

func TestHistoryGetPatternsHandler_empty(t *testing.T) {
	t.Parallel()

	kb := history.NewKnowledgeBase(t.TempDir())

	req := makeHistoryGetPatternsReq("", "", 0)
	result, err := historyGetPatternsWithKB(t.Context(), req, kb)
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
		Patterns             []json.RawMessage `json:"patterns"`
		TotalReviewsAnalyzed int               `json:"total_reviews_analyzed"`
	}
	if err := json.Unmarshal([]byte(textContent(result)), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Patterns) != 0 {
		t.Errorf("expected 0 patterns, got %d", len(resp.Patterns))
	}
	if resp.TotalReviewsAnalyzed != 0 {
		t.Errorf("expected total_reviews_analyzed 0, got %d", resp.TotalReviewsAnalyzed)
	}
}

// TestHistoryGetPatternsHandler_agentFilter verifies that agent_filter="design-reviewer"
// returns only patterns from that agent. AC-1: a KB with one finding from each reviewer
// returns exactly one pattern entry when filtered by "design-reviewer".
func TestHistoryGetPatternsHandler_agentFilter(t *testing.T) {
	t.Parallel()

	kb := history.NewKnowledgeBase(t.TempDir())
	ts := time.Now().UTC()

	// Add one finding from design-reviewer.
	designFindings := []orchestrator.Finding{
		{Severity: orchestrator.SeverityCritical, Description: "missing error handling in handler"},
	}
	if err := kb.Patterns.Accumulate(designFindings, "design-reviewer", ts); err != nil {
		t.Fatalf("Accumulate design-reviewer: %v", err)
	}

	// Add one finding from task-reviewer.
	taskFindings := []orchestrator.Finding{
		{Severity: orchestrator.SeverityMinor, Description: "missing test coverage for edge cases"},
	}
	if err := kb.Patterns.Accumulate(taskFindings, "task-reviewer", ts); err != nil {
		t.Fatalf("Accumulate task-reviewer: %v", err)
	}

	// Filter by design-reviewer — should return exactly one pattern.
	req := makeHistoryGetPatternsReq("design-reviewer", "", 0)
	result, err := historyGetPatternsWithKB(t.Context(), req, kb)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected MCP error: %v", textContent(result))
	}

	var resp struct {
		Patterns []struct {
			Pattern  string `json:"pattern"`
			Severity string `json:"severity"`
			Agent    string `json:"agent"`
		} `json:"patterns"`
		TotalReviewsAnalyzed int `json:"total_reviews_analyzed"`
	}
	if err := json.Unmarshal([]byte(textContent(result)), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if len(resp.Patterns) != 1 {
		t.Fatalf("agent_filter=design-reviewer: expected 1 pattern, got %d; patterns: %v", len(resp.Patterns), resp.Patterns)
	}
	if resp.Patterns[0].Agent != "design-reviewer" {
		t.Errorf("pattern agent: got %q, want %q", resp.Patterns[0].Agent, "design-reviewer")
	}
	if resp.TotalReviewsAnalyzed != 2 {
		t.Errorf("total_reviews_analyzed: got %d, want 2", resp.TotalReviewsAnalyzed)
	}
}

func TestHistoryGetPatternsHandler_severityFilter(t *testing.T) {
	t.Parallel()

	kb := history.NewKnowledgeBase(t.TempDir())
	ts := time.Now().UTC()

	findings := []orchestrator.Finding{
		{Severity: orchestrator.SeverityCritical, Description: "unchecked error in handler"},
		{Severity: orchestrator.SeverityMinor, Description: "missing test coverage"},
	}
	if err := kb.Patterns.Accumulate(findings, "design-reviewer", ts); err != nil {
		t.Fatalf("Accumulate: %v", err)
	}

	req := makeHistoryGetPatternsReq("", "CRITICAL", 0)
	result, err := historyGetPatternsWithKB(t.Context(), req, kb)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected MCP error: %v", textContent(result))
	}

	var resp struct {
		Patterns []struct {
			Severity string `json:"severity"`
		} `json:"patterns"`
	}
	if err := json.Unmarshal([]byte(textContent(result)), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	for _, p := range resp.Patterns {
		if p.Severity != "CRITICAL" {
			t.Errorf("severity_filter=CRITICAL: got severity %q, want CRITICAL", p.Severity)
		}
	}
}

func TestHistoryGetPatternsHandler_limitDefault(t *testing.T) {
	t.Parallel()

	kb := history.NewKnowledgeBase(t.TempDir())
	ts := time.Now().UTC()

	// Add 15 distinct findings to exceed the default limit of 10.
	findings := []orchestrator.Finding{
		{Severity: orchestrator.SeverityCritical, Description: "missing error handling alpha"},
		{Severity: orchestrator.SeverityCritical, Description: "import order violation beta"},
		{Severity: orchestrator.SeverityMinor, Description: "test coverage gamma"},
		{Severity: orchestrator.SeverityMinor, Description: "naming convention delta"},
		{Severity: orchestrator.SeverityCritical, Description: "type safety epsilon"},
		{Severity: orchestrator.SeverityCritical, Description: "security zeta"},
		{Severity: orchestrator.SeverityMinor, Description: "performance eta"},
		{Severity: orchestrator.SeverityMinor, Description: "documentation theta"},
		{Severity: orchestrator.SeverityCritical, Description: "error check iota"},
		{Severity: orchestrator.SeverityCritical, Description: "unchecked panic kappa"},
		{Severity: orchestrator.SeverityMinor, Description: "benchmark lambda"},
		{Severity: orchestrator.SeverityMinor, Description: "godoc missing mu"},
	}
	if err := kb.Patterns.Accumulate(findings, "design-reviewer", ts); err != nil {
		t.Fatalf("Accumulate: %v", err)
	}

	// No limit parameter — should default to 10.
	req := makeHistoryGetPatternsReq("", "", 0)
	result, err := historyGetPatternsWithKB(t.Context(), req, kb)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected MCP error: %v", textContent(result))
	}

	var resp struct {
		Patterns []json.RawMessage `json:"patterns"`
	}
	if err := json.Unmarshal([]byte(textContent(result)), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Patterns) > 10 {
		t.Errorf("expected at most 10 patterns (default limit), got %d", len(resp.Patterns))
	}

	// Verify that an explicit limit of 3 is honoured.
	reqLimited := makeHistoryGetPatternsReq("", "", 3)
	resultLimited, err := historyGetPatternsWithKB(t.Context(), reqLimited, kb)
	if err != nil {
		t.Fatalf("unexpected error (limit=3): %v", err)
	}
	if resultLimited.IsError {
		t.Fatalf("unexpected MCP error (limit=3): %v", textContent(resultLimited))
	}
	var respLimited struct {
		Patterns []json.RawMessage `json:"patterns"`
	}
	if err := json.Unmarshal([]byte(textContent(resultLimited)), &respLimited); err != nil {
		t.Fatalf("unmarshal response (limit=3): %v", err)
	}
	if len(respLimited.Patterns) > 3 {
		t.Errorf("explicit limit=3: expected at most 3 patterns, got %d", len(respLimited.Patterns))
	}
}

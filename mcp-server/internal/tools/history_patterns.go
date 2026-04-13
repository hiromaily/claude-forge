// history_get_patterns MCP handler.
// HistoryGetPatternsHandler exposes the PatternAccumulator query as an MCP tool.
// It returns accumulated review finding patterns filtered by agent and/or severity.

package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hiromaily/claude-forge/mcp-server/internal/history"
)

const defaultPatternsLimit = 10

// HistoryGetPatternsHandler handles the "history_get_patterns" MCP tool.
// Accepts: agent_filter (optional), severity_filter (optional), limit (optional, default 10).
func HistoryGetPatternsHandler(kb *history.KnowledgeBase) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return historyGetPatternsWithKB(ctx, req, kb)
	}
}

// historyGetPatternsWithKB is the testable variant that accepts an explicit KnowledgeBase.
func historyGetPatternsWithKB(
	_ context.Context,
	req mcp.CallToolRequest,
	kb *history.KnowledgeBase,
) (*mcp.CallToolResult, error) {
	agentFilter := req.GetString("agent_filter", "")
	severityFilter := req.GetString("severity_filter", "")
	limit := req.GetInt("limit", 0)

	// Default limit is 10 when absent or zero.
	if limit <= 0 {
		limit = defaultPatternsLimit
	}

	patterns := kb.Patterns.Query(agentFilter, severityFilter, limit)
	totalReviewsAnalyzed := kb.Patterns.TotalReviewsAnalyzed()

	response := struct {
		Patterns             []history.PatternEntry `json:"patterns"`
		TotalReviewsAnalyzed int                    `json:"total_reviews_analyzed"`
	}{
		Patterns:             nonNilSlice(patterns),
		TotalReviewsAnalyzed: totalReviewsAnalyzed,
	}

	return okJSON(response)
}

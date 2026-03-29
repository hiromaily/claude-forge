// Package tools — history_get_friction_map MCP handler.
// HistoryGetFrictionMapHandler exposes the FrictionMap query as an MCP tool.
// It returns accumulated friction points extracted from improvement reports.
package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hiromaily/claude-forge/mcp-server/history"
)

// HistoryGetFrictionMapHandler handles the "history_get_friction_map" MCP tool.
// Accepts no parameters.
func HistoryGetFrictionMapHandler(kb *history.KnowledgeBase) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return historyGetFrictionMapWithKB(ctx, req, kb)
	}
}

// historyGetFrictionMapWithKB is the testable variant that accepts an explicit KnowledgeBase.
func historyGetFrictionMapWithKB(
	_ context.Context,
	_ mcp.CallToolRequest,
	kb *history.KnowledgeBase,
) (*mcp.CallToolResult, error) {
	points := kb.Friction.Points()
	totalReportsAnalyzed := kb.Friction.TotalReportsAnalyzed()

	response := struct {
		FrictionPoints       []history.FrictionPoint `json:"friction_points"`
		TotalReportsAnalyzed int                     `json:"total_reports_analyzed"`
	}{
		FrictionPoints:       points,
		TotalReportsAnalyzed: totalReportsAnalyzed,
	}

	// Ensure friction_points is never null in JSON output.
	if response.FrictionPoints == nil {
		response.FrictionPoints = []history.FrictionPoint{}
	}

	return okJSON(response)
}

// Package tools — analytics_repo_dashboard MCP handler.
// AnalyticsRepoDashboardHandler returns aggregate statistics across all
// pipeline runs in .specs/ (counts, averages, cost, review pass rate,
// common findings).
package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hiromaily/claude-forge/mcp-server/analytics"
)

// AnalyticsRepoDashboardHandler handles the "analytics_repo_dashboard" MCP tool.
// Accepts no parameters.
// When rep is nil, returns an MCP error result.
// When rep is non-nil, calls rep.Dashboard() and returns the result as JSON.
func AnalyticsRepoDashboardHandler(rep *analytics.Reporter) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if rep == nil {
			return mcp.NewToolResultError("reporter not available"), nil
		}
		dashboard, err := rep.Dashboard()
		if err != nil {
			return errorf("analytics_repo_dashboard: %v", err)
		}
		return okJSON(dashboard)
	}
}

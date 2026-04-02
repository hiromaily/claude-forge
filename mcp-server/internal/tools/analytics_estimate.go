// Package tools — analytics_estimate MCP handler.
// AnalyticsEstimateHandler returns P50/P90 predictions for tokens, duration,
// and cost for a given (task_type, effort) combination.
package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hiromaily/claude-forge/mcp-server/internal/analytics"
)

// AnalyticsEstimateHandler handles the "analytics_estimate" MCP tool.
// Accepts: effort (required, string).
// When est is nil, returns an MCP error result.
// When est is non-nil, calls est.Estimate and returns the result as JSON.
func AnalyticsEstimateHandler(est *analytics.Estimator) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if est == nil {
			return mcp.NewToolResultError("estimator not available"), nil
		}
		effort, err := req.RequireString("effort")
		if err != nil {
			return errorf("%v", err)
		}
		result, err := est.Estimate(effort)
		if err != nil {
			return errorf("analytics_estimate: %v", err)
		}
		return okJSON(result)
	}
}

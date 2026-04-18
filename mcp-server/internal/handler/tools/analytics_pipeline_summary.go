// analytics_pipeline_summary MCP handler.
// AnalyticsPipelineSummaryHandler returns token, duration, cost, and
// review-finding statistics for a single pipeline run.

package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hiromaily/claude-forge/mcp-server/internal/intelligence/analytics"
)

// AnalyticsPipelineSummaryHandler handles the "analytics_pipeline_summary" MCP tool.
// Accepts: workspace (required, string — absolute path to workspace directory).
// When col is nil, returns an MCP error result.
// When col is non-nil, calls col.Collect(workspace) and returns the summary as JSON.
func AnalyticsPipelineSummaryHandler(col *analytics.Collector) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if col == nil {
			return mcp.NewToolResultError("collector not available"), nil
		}

		workspace, err := req.RequireString("workspace")
		if err != nil {
			return errorf("%v", err)
		}

		summary, err := col.Collect(workspace)
		if err != nil {
			return errorf("analytics_pipeline_summary: %v", err)
		}

		return okJSON(summary)
	}
}

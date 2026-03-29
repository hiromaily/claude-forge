// Package tools — profile_get MCP handler.
// ProfileGetHandler returns the repository profile as JSON.
// The profile is computed once at server startup and cached for 7 days.
package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hiromaily/claude-forge/mcp-server/profile"
)

// ProfileGetHandler handles the "profile_get" MCP tool.
// Accepts: workspace (required, string — for API consistency only).
// When profiler is nil, returns an MCP error result.
// When profiler is non-nil, calls AnalyzeOrUpdate and returns the profile as JSON.
func ProfileGetHandler(profiler *profile.RepoProfiler) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if profiler == nil {
			return mcp.NewToolResultError("profiler not available"), nil
		}
		prof, err := profiler.AnalyzeOrUpdate()
		if err != nil {
			return errorf("profile_get: %v", err)
		}
		return okJSON(prof)
	}
}

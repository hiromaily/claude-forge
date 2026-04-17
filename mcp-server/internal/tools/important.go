// __IMPORTANT MCP tool handler.
// ImportantHandler is a discovery-only tool that returns session-start context
// including the dashboard URL when the events server is configured.
// It follows the __IMPORTANT naming convention used by Claude Code plugins to
// auto-inject information into the system prompt at session start.

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ImportantHandler returns a ToolHandlerFunc that provides session-start context.
// eventsPort is the port the dashboard/SSE server is listening on (from FORGE_EVENTS_PORT).
// When eventsPort is non-empty, the response includes the dashboard URL.
func ImportantHandler(eventsPort string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var b strings.Builder
		b.WriteString("claude-forge pipeline orchestrator is active.\n")

		if eventsPort != "" {
			fmt.Fprintf(&b, "\nDashboard: http://localhost:%s/\n", eventsPort)
			b.WriteString("Open the dashboard in your browser to monitor pipeline progress in real-time.\n")
		}

		b.WriteString("\nUse /forge <task> to start a pipeline.")
		return mcp.NewToolResultText(b.String()), nil
	}
}

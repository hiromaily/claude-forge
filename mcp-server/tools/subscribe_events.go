// Package tools — subscribe_events MCP tool handler.
// SubscribeEventsHandler is a discovery-only tool that returns the SSE endpoint
// URL when the events server is configured, or an informational message otherwise.
// It does not import or depend on the state package.
package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// subscribeEventsResponse is the JSON response when SSE is configured.
type subscribeEventsResponse struct {
	Endpoint string `json:"endpoint"`
}

// SubscribeEventsHandler returns a ToolHandlerFunc that reports the SSE endpoint URL.
// eventsPort is the port the SSE server is listening on (from FORGE_EVENTS_PORT).
// When eventsPort is non-empty, the handler returns {"endpoint":"http://localhost:<port>/events"}.
// When eventsPort is empty, the handler returns an informational text message.
func SubscribeEventsHandler(eventsPort string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if eventsPort == "" {
			return mcp.NewToolResultText("SSE event streaming is not configured. Set FORGE_EVENTS_PORT to enable it."), nil
		}
		resp := subscribeEventsResponse{
			Endpoint: fmt.Sprintf("http://localhost:%s/events", eventsPort),
		}
		return okJSON(resp)
	}
}

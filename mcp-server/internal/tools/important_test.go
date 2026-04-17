// Package tools — tests for __IMPORTANT MCP tool handler.
package tools

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestImportantHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		eventsPort string
		wantURL    bool
	}{
		{name: "port_set", eventsPort: "8099", wantURL: true},
		{name: "port_empty", eventsPort: "", wantURL: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler := ImportantHandler(tc.eventsPort)
			result, err := handler(t.Context(), mcp.CallToolRequest{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result == nil {
				t.Fatal("expected non-nil result")
			}
			if result.IsError {
				t.Fatal("expected non-error result")
			}
			if len(result.Content) == 0 {
				t.Fatal("expected non-empty content")
			}

			text := result.Content[0].(mcp.TextContent).Text
			if !strings.Contains(text, "claude-forge") {
				t.Errorf("expected text to mention claude-forge, got: %s", text)
			}

			hasDashboardURL := strings.Contains(text, "http://localhost:"+tc.eventsPort+"/")
			if tc.wantURL && !hasDashboardURL {
				t.Errorf("expected dashboard URL with port %s, got: %s", tc.eventsPort, text)
			}
			if !tc.wantURL && strings.Contains(text, "http://localhost") {
				t.Errorf("expected no dashboard URL when port is empty, got: %s", text)
			}
		})
	}
}

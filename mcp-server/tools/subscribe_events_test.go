// Package tools — tests for subscribe_events MCP tool handler.
package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestSubscribeEventsHandler_PortSet(t *testing.T) {
	handler := SubscribeEventsHandler("8080")
	req := mcp.CallToolRequest{}
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.IsError {
		t.Fatalf("expected non-error result, got error")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected non-empty content")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "8080") {
		t.Errorf("expected endpoint URL to contain port 8080, got: %s", text)
	}
	if !strings.Contains(text, "http://localhost:8080/events") {
		t.Errorf("expected endpoint URL http://localhost:8080/events, got: %s", text)
	}
}

func TestSubscribeEventsHandler_PortEmpty(t *testing.T) {
	handler := SubscribeEventsHandler("")
	req := mcp.CallToolRequest{}
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.IsError {
		t.Fatalf("expected non-error result when port is empty, got error")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected non-empty content")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if text == "" {
		t.Error("expected a disabled message, got empty string")
	}
	// The result should contain some indication that SSE is disabled/not configured.
	if !strings.Contains(strings.ToLower(text), "not") && !strings.Contains(strings.ToLower(text), "disabled") {
		t.Errorf("expected disabled message, got: %s", text)
	}
}

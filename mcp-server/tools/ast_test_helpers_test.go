// Package tools — shared test helpers for AST handler tests.
package tools

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// toolResultText extracts the text content from a CallToolResult.
// It is equivalent to textContent but named distinctly to avoid confusion
// with the non-AST handlers' helper.
func toolResultText(res *mcp.CallToolResult) string {
	return textContent(res)
}

// assertNotError fails t with a descriptive message if result is an error result.
func assertNotError(t *testing.T, result *mcp.CallToolResult, context string) {
	t.Helper()
	if result == nil {
		t.Fatalf("%s: result is nil", context)
	}
	if result.IsError {
		t.Fatalf("%s: expected IsError=false, got IsError=true with text: %s", context, toolResultText(result))
	}
}

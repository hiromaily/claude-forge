// Package tools — shared test helpers for AST handler tests.
package tools

import (
	"os"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// toolResultText extracts the text content from a CallToolResult.
// It is equivalent to textContent but named distinctly to avoid confusion
// with the non-AST handlers' helper.
func toolResultText(res *mcp.CallToolResult) string {
	return textContent(res)
}

// writeTestFile writes content to path, creating parent directories as needed.
func writeTestFile(path, content string) error {
	if err := os.MkdirAll(parentDir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// parentDir returns the parent directory of path using filepath logic.
func parentDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i]
		}
	}
	return "."
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

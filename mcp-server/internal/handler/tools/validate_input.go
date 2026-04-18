// validate_input MCP handler.
// ValidateInputHandler exposes validation.ValidateInput as an MCP tool.
// It always returns a JSON-serialised InputResult via okJSON, never an MCP error,
// even when the input is empty or invalid.

package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hiromaily/claude-forge/mcp-server/internal/handler/validation"
)

// ValidateInputHandler handles the "validate_input" MCP tool.
// Accepts: arguments (string, required).
// Returns: validation.InputResult serialised as JSON via okJSON.
// When arguments is empty the response still contains valid:false (not an MCP error).
func ValidateInputHandler() server.ToolHandlerFunc {
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		arguments := req.GetString("arguments", "")
		result := validation.ValidateInput(arguments)
		return okJSON(result)
	}
}

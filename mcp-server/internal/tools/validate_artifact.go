// validate_artifact MCP handler.
// ValidateArtifactHandler exposes validation.ValidateArtifacts as an MCP tool.
// It always returns the result slice serialised as a JSON array via okJSON.
// Missing workspace or phase parameters return an errorf response.

package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hiromaily/claude-forge/mcp-server/internal/validation"
)

// ValidateArtifactHandler handles the "validate_artifact" MCP tool.
// Accepts: workspace (string, required), phase (string, required).
// Returns: []validation.ArtifactResult serialised as a JSON array via okJSON.
// For all phases except phase-6 the array contains exactly one element.
// For phase-6 the array contains one element per impl-*.md file found in workspace.
func ValidateArtifactHandler() server.ToolHandlerFunc {
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace := req.GetString("workspace", "")
		if workspace == "" {
			return errorf("workspace parameter is required")
		}
		phase := req.GetString("phase", "")
		if phase == "" {
			return errorf("phase parameter is required")
		}
		results := validation.ValidateArtifacts(workspace, phase)
		return okJSON(results)
	}
}

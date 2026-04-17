package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
)

// PreferencesGetHandler returns the current user preferences from .specs/preferences.json.
func PreferencesGetHandler(sm *state.StateManager) server.ToolHandlerFunc {
	return func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		p, err := state.LoadPreferences(sm.SpecsDir())
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("preferences_get: %v", err)), nil
		}
		return okJSON(p)
	}
}

// PreferencesSetHandler writes user preferences to .specs/preferences.json.
// The preferences parameter is a JSON object; unknown fields are stripped
// via marshal/unmarshal through the typed Preferences struct.
func PreferencesSetHandler(sm *state.StateManager) server.ToolHandlerFunc {
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, ok := req.GetArguments()["preferences"]
		if !ok {
			return mcp.NewToolResultError("preferences_set: missing required parameter 'preferences'"), nil
		}
		b, err := json.Marshal(raw)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("preferences_set: marshal: %v", err)), nil
		}
		var p state.Preferences
		if err := json.Unmarshal(b, &p); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("preferences_set: invalid preferences: %v", err)), nil
		}
		if err := p.Validate(); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("preferences_set: %v", err)), nil
		}
		if err := state.SavePreferences(sm.SpecsDir(), p); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("preferences_set: %v", err)), nil
		}
		return okJSON(map[string]bool{"ok": true})
	}
}

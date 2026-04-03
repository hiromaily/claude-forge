// Package tools — shared helper functions for MCP tool handlers.
// These helpers eliminate repeated parameter extraction, state loading,
// and nil-safety patterns across handler files.
package tools

import (
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
)

// ---------- parameter extraction helpers ----------

// requireWorkspace extracts the required "workspace" parameter from the request.
// Returns the workspace string on success, or an MCP error result on failure.
func requireWorkspace(req mcp.CallToolRequest) (string, *mcp.CallToolResult, error) {
	workspace, err := req.RequireString("workspace")
	if err != nil {
		r, e := errorf("%v", err)
		return "", r, e
	}
	return workspace, nil, nil
}

// requireWorkspaceAndPhase extracts both "workspace" and "phase" required parameters.
// Returns the values on success, or an MCP error result on first failure.
func requireWorkspaceAndPhase(req mcp.CallToolRequest) (workspace, phase string, result *mcp.CallToolResult, err error) {
	workspace, result, err = requireWorkspace(req)
	if result != nil {
		return "", "", result, err
	}
	phase, perr := req.RequireString("phase")
	if perr != nil {
		r, e := errorf("%v", perr)
		return "", "", r, e
	}
	return workspace, phase, nil, nil
}

// requireWorkspaceAndString extracts "workspace" and one additional required string parameter.
// Returns the values on success, or an MCP error result on first failure.
func requireWorkspaceAndString(req mcp.CallToolRequest, param string) (workspace, value string, result *mcp.CallToolResult, err error) {
	workspace, result, err = requireWorkspace(req)
	if result != nil {
		return "", "", result, err
	}
	value, verr := req.RequireString(param)
	if verr != nil {
		r, e := errorf("%v", verr)
		return "", "", r, e
	}
	return workspace, value, nil, nil
}

// ---------- state loading helpers ----------

// loadStateOrError loads state for the workspace, returning an MCP error result on failure.
func loadStateOrError(workspace string) (*state.State, *mcp.CallToolResult, error) {
	s, serr := loadState(workspace)
	if serr != nil {
		r, e := errorf("load state: %v", serr)
		return nil, r, e
	}
	return s, nil, nil
}

// stateForEvent loads state to extract specName and phase for event publishing.
// Returns empty strings if state cannot be loaded (non-fatal).
func stateForEvent(workspace string) (specName, phase string) {
	s, serr := loadState(workspace)
	if serr == nil {
		return s.SpecName, s.CurrentPhase
	}
	return "", ""
}

// ---------- nil-safety helpers ----------

// nonNilSlice returns s if non-nil, or an empty slice of the same type.
// Use this to ensure JSON output contains [] instead of null.
func nonNilSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

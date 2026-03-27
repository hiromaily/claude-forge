// Package tools — ast_impact_scope MCP tool handler.
// AstImpactScopeHandler identifies all files in a source tree that call a given
// symbol, using a two-pass import-filter + call-site scan for Go and Bash, and
// a single-pass call-site scan for TypeScript and Python.
package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hiromaily/claude-forge/mcp-server/ast"
)

// AstImpactScopeHandler returns a ToolHandlerFunc for the impact_scope tool.
// Parameters:
//   - root_path (required): absolute path to the repository root to scan.
//   - file_path (required): path (relative or absolute) to the file that defines the symbol.
//   - symbol_name (required): the function/type/constant name to search for.
//   - language (required): source language ("go", "typescript", "python", "bash").
//
// It has no state manager (sm) dependency.
func AstImpactScopeHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		rootPath, err := req.RequireString("root_path")
		if err != nil {
			return errorf("%v", err)
		}
		filePath, err := req.RequireString("file_path")
		if err != nil {
			return errorf("%v", err)
		}
		symbolName, err := req.RequireString("symbol_name")
		if err != nil {
			return errorf("%v", err)
		}
		language, err := req.RequireString("language")
		if err != nil {
			return errorf("%v", err)
		}
		return astImpactScopeFromRoot(ctx, rootPath, filePath, symbolName, language)
	}
}

// impactScopeResponse is the JSON envelope returned by the impact_scope tool.
type impactScopeResponse struct {
	TargetFile    string            `json:"target_file"`
	Symbol        string            `json:"symbol"`
	Root          string            `json:"root"`
	Lang          string            `json:"lang"`
	AffectedFiles []ast.ImpactEntry `json:"affected_files"`
}

// astImpactScopeFromRoot is the testable core of AstImpactScopeHandler.
// It resolves the language, calls ast.FindCallers, and returns the result as
// a JSON-encoded impactScopeResponse.
//
// On error (unsupported language, invalid root_path, etc.) it returns a
// tool-error response (IsError=true) rather than a Go error.
func astImpactScopeFromRoot(ctx context.Context, rootPath, filePath, symbolName, language string) (*mcp.CallToolResult, error) {
	// Resolve language — language is required, so pass it as the explicit override.
	// We pass filePath so that resolveLanguage can fall back to extension detection
	// if language is somehow empty, but the parameter is required so it will always
	// be non-empty in practice.
	lang, err := resolveLanguage(filePath, language)
	if err != nil {
		return errorf("%v", err)
	}

	// Invoke the domain-level FindCallers.
	affected, err := ast.FindCallers(ctx, rootPath, lang, filePath, symbolName)
	if err != nil {
		return errorf("find callers: %v", err)
	}

	resp := impactScopeResponse{
		TargetFile:    filePath,
		Symbol:        symbolName,
		Root:          rootPath,
		Lang:          language,
		AffectedFiles: affected,
	}
	return okJSON(resp)
}

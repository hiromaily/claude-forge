// ast_dependency_graph MCP tool handler.
// AstDependencyGraphHandler walks a source tree and returns a file-level
// import graph as JSON. It does not import or depend on the state package.

package tools

import (
	"context"
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hiromaily/claude-forge/mcp-server/pkg/ast"
)

// AstDependencyGraphHandler returns a ToolHandlerFunc for the dependency_graph tool.
// Parameters:
//   - root_path (required): absolute path to the root of the source tree to scan.
//   - language (required): the language to analyse ("go", "typescript", "python", "bash").
//
// It has no state manager (sm) dependency.
func AstDependencyGraphHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		rootPath, err := req.RequireString("root_path")
		if err != nil {
			return errorf("%v", err)
		}
		language, err := req.RequireString("language")
		if err != nil {
			return errorf("%v", err)
		}
		return astDependencyGraphFromRoot(ctx, rootPath, language)
	}
}

// astDependencyGraphFromRoot is the testable core of AstDependencyGraphHandler.
// It resolves the language, calls ast.BuildDependencyGraph, marshals the result
// to JSON and returns it via okText.
// Tool-level errors (invalid path, unsupported language) are returned as
// IsError=true ToolResult values, not as Go errors.
func astDependencyGraphFromRoot(ctx context.Context, rootPath, language string) (*mcp.CallToolResult, error) {
	// resolveLanguage requires a filePath for extension-based auto-detection.
	// Since language is required here, pass an empty filePath; the language
	// parameter always takes precedence.
	lang, err := resolveLanguage("", language)
	if err != nil {
		return errorf("%v", err)
	}

	graph, err := ast.BuildDependencyGraph(ctx, rootPath, lang)
	if err != nil {
		return errorf("build dependency graph: %v", err)
	}

	data, err := json.Marshal(graph)
	if err != nil {
		return errorf("marshal result: %v", err)
	}

	return okText(string(data))
}

// Package tools — ast_find_definition MCP tool handler.
// AstFindDefinitionHandler searches for a named symbol declaration in a source file
// using tree-sitter AST parsing. It does not import or depend on the state package.
package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hiromaily/claude-forge/mcp-server/ast"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// AstFindDefinitionHandler returns a ToolHandlerFunc for the ast_find_definition tool.
// Parameters:
//   - file_path (required): absolute or relative path to the source file.
//   - symbol (required): the symbol name to look up.
//   - language (optional): explicit language override ("go", "typescript", "python", "bash").
//     When omitted, the language is auto-detected from the file extension.
//
// It has no state manager (sm) dependency.
func AstFindDefinitionHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filePath, err := req.RequireString("file_path")
		if err != nil {
			return errorf("%v", err)
		}
		symbol, err := req.RequireString("symbol")
		if err != nil {
			return errorf("%v", err)
		}
		language := req.GetString("language", "")
		return astFindDefinitionFromPath(ctx, filePath, language, symbol)
	}
}

// astFindDefinitionFromPath is the testable core of AstFindDefinitionHandler.
// It resolves the language, reads the file, and returns matching symbol definitions.
// Results:
//   - Symbol found once: IsError=false, text contains only the definition (no count header).
//   - Symbol found multiple times: IsError=false, text begins with "<N> matches found\n\n"
//     followed by each definition separated by double newlines.
//   - Symbol not found: IsError=false, empty text.
//   - File not found or language unsupported: IsError=true.
func astFindDefinitionFromPath(ctx context.Context, filePath, language, symbol string) (*mcp.CallToolResult, error) {
	// Resolve language from parameter or file extension.
	lang, err := resolveLanguage(filePath, language)
	if err != nil {
		return errorf("%v", err)
	}

	// Read the source file.
	src, err := os.ReadFile(filePath)
	if err != nil {
		return errorf("read file %q: %v", filePath, err)
	}

	// Invoke the domain-level FindDefinition.
	matches, err := ast.FindDefinition(ctx, src, lang, symbol)
	if err != nil {
		return errorf("find definition: %v", err)
	}

	// No matches: return empty success.
	if len(matches) == 0 {
		return okText("")
	}

	// Single match: return definition text without a count header.
	if len(matches) == 1 {
		return okText(matches[0])
	}

	// Multiple matches: prepend count header.
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d matches found\n\n", len(matches))
	for i, m := range matches {
		sb.WriteString(m)
		if i < len(matches)-1 {
			sb.WriteString("\n\n")
		}
	}
	return okText(sb.String())
}

// resolveLanguage returns the effective Language for the given file path and explicit
// language parameter. When language is non-empty it takes precedence. When language is
// empty the file extension is used for auto-detection.
func resolveLanguage(filePath, language string) (ast.Language, error) {
	if language != "" {
		lang := ast.Language(language)
		switch lang {
		case ast.Go, ast.TypeScript, ast.Python, ast.Bash:
			return lang, nil
		default:
			return "", fmt.Errorf("unsupported language %q; supported: go, typescript, python, bash", language)
		}
	}
	ext := strings.ToLower(filepath.Ext(filePath))
	lang, ok := ast.LangFromExtension(ext)
	if !ok {
		return "", fmt.Errorf("cannot detect language from extension %q; provide an explicit language parameter", ext)
	}
	return lang, nil
}

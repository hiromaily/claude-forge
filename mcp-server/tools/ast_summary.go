// Package tools — ast_summary MCP handler.
// AstSummaryHandler exposes the ast.Summarize function as an MCP tool.
// It accepts a file_path and an optional language parameter (auto-detected
// from the file extension when omitted).
package tools

import (
	"context"
	"os"
	"strings"

	"github.com/hiromaily/claude-forge/mcp-server/ast"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// AstSummaryHandler returns a ToolHandlerFunc that summarizes the AST of a
// source file. It has no dependency on StateManager.
func AstSummaryHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filePath := req.GetString("file_path", "")
		if filePath == "" {
			return errorf("file_path parameter is required")
		}
		language := req.GetString("language", "")
		return astSummaryFromPath(ctx, filePath, language)
	}
}

// astSummaryFromPath is the testable core of AstSummaryHandler.
// It reads the file at filePath, resolves the language (auto-detecting from
// the extension when language is empty), calls ast.Summarize, and formats
// the result as a markdown document.
func astSummaryFromPath(ctx context.Context, filePath, language string) (*mcp.CallToolResult, error) {
	// Resolve language (resolveLanguage is defined in ast_find_definition.go).
	lang, err := resolveLanguage(filePath, language)
	if err != nil {
		return errorf("%v", err)
	}

	// Read source file.
	src, err := os.ReadFile(filePath)
	if err != nil {
		return errorf("read file: %v", err)
	}

	// Parse and summarize.
	summary, err := ast.Summarize(ctx, src, lang)
	if err != nil {
		return errorf("summarize: %v", err)
	}

	// Format result as markdown.
	text := formatSummary(summary)
	return okText(text)
}

// formatSummary renders a Summary as a markdown document with grouped sections.
// Sections are omitted when the corresponding slice is empty.
func formatSummary(summary ast.Summary) string {
	var sb strings.Builder

	if len(summary.Functions) > 0 {
		sb.WriteString("## Functions\n\n")
		for _, sig := range summary.Functions {
			sb.WriteString("- `")
			sb.WriteString(sig)
			sb.WriteString("`\n")
		}
	}

	if len(summary.Types) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("## Types\n\n")
		for _, sig := range summary.Types {
			sb.WriteString("- `")
			sb.WriteString(sig)
			sb.WriteString("`\n")
		}
	}

	if len(summary.Constants) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("## Constants\n\n")
		for _, sig := range summary.Constants {
			sb.WriteString("- `")
			sb.WriteString(sig)
			sb.WriteString("`\n")
		}
	}

	return sb.String()
}

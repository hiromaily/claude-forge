// history_search MCP handler.
// HistorySearchHandler exposes the history index search as an MCP tool.
// It queries past pipeline runs from the history index using BM25 scoring
// and returns ranked results with metadata and design excerpts.

package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hiromaily/claude-forge/mcp-server/internal/history"
)

const defaultHistorySearchLimit = 3

// HistorySearchHandler handles the "history_search" MCP tool.
// Accepts: query (required), limit (optional, default 3).
func HistorySearchHandler(idx *history.HistoryIndex) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return historySearchWithIndex(ctx, req, idx, idx.SpecsDir())
	}
}

// historySearchWithIndex is the testable variant that accepts explicit idx and specsDir.
// specsDir is passed explicitly (not derived from idx) to allow tests to inject a t.TempDir()
// path, mirroring the searchPatternsWithPaths convention.
func historySearchWithIndex(
	_ context.Context,
	req mcp.CallToolRequest,
	idx *history.HistoryIndex,
	specsDir string,
) (*mcp.CallToolResult, error) {
	query := req.GetString("query", "")
	limit := req.GetInt("limit", 0)

	// Default limit is 3 when absent or zero.
	if limit <= 0 {
		limit = defaultHistorySearchLimit
	}

	// Build the search result using the history.Search function.
	// specsDir is threaded through so readDesignExcerpt can resolve design.md files.
	// We wrap idx with the provided specsDir by creating an adjusted index view.
	results, err := historySearchWithSpecsDir(idx, query, limit, specsDir)
	if err != nil {
		return errorf("history_search: %v", err)
	}

	response := struct {
		Results   []history.SearchResult `json:"results"`
		IndexSize int                    `json:"index_size"`
	}{
		Results:   results,
		IndexSize: idx.Size(),
	}

	return okJSON(response)
}

// historySearchWithSpecsDir calls history.Search but uses specsDir for design excerpt
// resolution. When specsDir matches idx.SpecsDir() this is a no-op pass-through.
// When specsDir differs (e.g. in tests), it calls history.SearchWithSpecsDir.
func historySearchWithSpecsDir(
	idx *history.HistoryIndex,
	query string,
	limit int,
	specsDir string,
) ([]history.SearchResult, error) {
	if specsDir == "" || specsDir == idx.SpecsDir() {
		return history.Search(idx, query, limit)
	}
	return history.SearchWithSpecsDir(idx, query, limit, specsDir)
}

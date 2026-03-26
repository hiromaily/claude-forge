// Package tools — search_patterns MCP handler.
// SearchPatternsHandler exposes BM25 scoring of .specs/index.json as an MCP tool.
// It reads the workspace's request.md as the query, strips YAML frontmatter,
// calls search.Score, and formats results in exact markdown parity with
// query-specs-index.sh output.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hiromaily/claude-forge/mcp-server/search"
	"github.com/hiromaily/claude-forge/mcp-server/state"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Output format constants — must match query-specs-index.sh printf strings exactly.
const (
	reviewFeedbackHeader = "## Past Review Feedback (from similar pipelines)\n\n"
	reviewFeedbackBullet = "- **[%s]** %s _(from: %s)_\n"
	implHeader           = "## Similar Past Implementations (from similar pipelines)\n\n"
	implBullet           = "- **%s** (%s): %s — files: %s\n"
)

// SearchPatternsHandler handles the "search_patterns" MCP tool.
// Accepts: workspace (required), task_type (optional), top_k (optional), mode (optional).
// sm is accepted for signature consistency with other handlers but is not used.
func SearchPatternsHandler(sm *state.StateManager) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace := req.GetString("workspace", "")
		if workspace == "" {
			return errorf("workspace parameter is required")
		}
		indexPath := filepath.Join(filepath.Dir(workspace), "index.json")
		requestPath := filepath.Join(workspace, "request.md")
		return searchPatternsWithPaths(ctx, req, indexPath, requestPath)
	}
}

// searchPatternsWithPaths is the testable variant that accepts explicit file paths.
// It is unexported but accessible to tests within package tools.
func searchPatternsWithPaths(
	ctx context.Context,
	req mcp.CallToolRequest,
	indexPath string,
	requestPath string,
) (*mcp.CallToolResult, error) {
	taskType := req.GetString("task_type", "")
	topK := req.GetInt("top_k", 0)
	mode := req.GetString("mode", "")

	// Step 3: return okText("") when indexPath is absent.
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return okText("")
		}
		return errorf("read index: %v", err)
	}

	// Step 4: decode index entries.
	var entries []search.IndexEntry
	if err := json.Unmarshal(indexData, &entries); err != nil {
		return errorf("unmarshal index: %v", err)
	}
	// Return okText("") when decoded array is empty.
	if len(entries) == 0 {
		return okText("")
	}

	// Step 5: read request.md; use empty string if absent (do NOT early-return).
	var requestBody string
	if raw, err := os.ReadFile(requestPath); err == nil {
		requestBody = string(raw)
	}
	// If request.md is absent, requestBody remains "".

	// Step 6: strip YAML frontmatter (lines between leading --- delimiters).
	requestBody = stripFrontmatter(requestBody)

	// Step 7: call BM25 scorer.
	scored := search.Score(entries, requestBody, taskType, search.DefaultBM25Params())

	// Step 8: apply mode-specific filters and top-K selection.
	isImpl := mode == "impl"
	if topK == 0 {
		if isImpl {
			topK = 2
		} else {
			topK = 3
		}
	}

	// Step 9: format output.
	if isImpl {
		return formatImplOutput(scored, topK)
	}
	return formatReviewFeedbackOutput(scored, topK)
}

// stripFrontmatter removes YAML frontmatter delimited by leading --- lines.
// If the content starts with "---\n", everything up to and including the closing
// "---\n" line is stripped. If no matching closing delimiter exists, the content
// is returned unchanged.
func stripFrontmatter(content string) string {
	if !strings.HasPrefix(content, "---\n") && content != "---" {
		return content
	}
	// Find the closing delimiter.
	rest := content[4:] // skip "---\n"
	idx := strings.Index(rest, "\n---\n")
	if idx == -1 {
		// Check if the content ends with "\n---" (no trailing newline)
		if strings.HasSuffix(rest, "\n---") {
			return ""
		}
		// No closing --- found; return content unchanged.
		return content
	}
	// Return everything after the closing "---\n".
	return rest[idx+5:] // skip "\n---\n"
}

// formatReviewFeedbackOutput formats BM25 results in review-feedback mode.
// It limits to topK scored entries and emits one bullet per finding.
func formatReviewFeedbackOutput(scored []search.ScoredEntry, topK int) (*mcp.CallToolResult, error) {
	if topK > len(scored) {
		topK = len(scored)
	}
	top := scored[:topK]

	var sb strings.Builder
	for _, se := range top {
		for _, rf := range se.Entry.ReviewFeedback {
			for _, finding := range rf.Findings {
				sb.WriteString(fmt.Sprintf(reviewFeedbackBullet, rf.Verdict, finding, rf.Source))
			}
		}
	}

	body := sb.String()
	if body == "" {
		return okText("")
	}
	return okText(reviewFeedbackHeader + body)
}

// formatImplOutput formats BM25 results in impl mode.
// It filters to entries where outcome == "completed", limits to topK, and emits
// one bullet per ImplPattern.
func formatImplOutput(scored []search.ScoredEntry, topK int) (*mcp.CallToolResult, error) {
	// Filter to completed entries first, then apply topK.
	completed := make([]search.ScoredEntry, 0, len(scored))
	for _, se := range scored {
		if se.Entry.Outcome == "completed" {
			completed = append(completed, se)
		}
	}

	if topK > len(completed) {
		topK = len(completed)
	}
	top := completed[:topK]

	var sb strings.Builder
	for _, se := range top {
		for _, pat := range se.Entry.ImplPatterns {
			files := strings.Join(pat.FilesModified, ", ")
			sb.WriteString(fmt.Sprintf(implBullet, pat.TaskTitle, se.Entry.SpecName, se.Entry.RequestSummary, files))
		}
	}

	body := sb.String()
	if body == "" {
		return okText("")
	}
	return okText(implHeader + body)
}

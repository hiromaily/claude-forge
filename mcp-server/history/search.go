// Package history implements the history index and search over past pipeline runs.
package history

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	bm25 "github.com/hiromaily/claude-forge/mcp-server/search"
)

// SearchResult is the per-result shape returned by the history_search MCP tool.
type SearchResult struct {
	SpecName        string  `json:"spec_name"`
	Similarity      float64 `json:"similarity"`
	TaskType        string  `json:"task_type"`
	Effort          string  `json:"effort"`
	FlowTemplate    string  `json:"flow_template"`
	OneLiner        string  `json:"one_liner"`
	DesignExcerpt   string  `json:"design_excerpt"`
	Outcome         string  `json:"outcome"`
	TokensTotal     int     `json:"tokens_total"`
	DurationTotalMs int     `json:"duration_total_ms"`
}

// Search queries the history index using BM25 scoring and returns results ordered
// by descending Similarity. When taskTypeFilter is non-empty, only entries with
// matching TaskType are included before scoring. Results are capped to limit.
func Search(idx *HistoryIndex, query string, limit int, taskTypeFilter string) ([]SearchResult, error) {
	entries := idx.Entries()
	if len(entries) == 0 {
		return []SearchResult{}, nil
	}

	// Apply taskTypeFilter as a hard pre-filter.
	filtered := entries
	if taskTypeFilter != "" {
		filtered = make([]IndexEntry, 0, len(entries))
		for _, e := range entries {
			if e.TaskType == taskTypeFilter {
				filtered = append(filtered, e)
			}
		}
	}

	if len(filtered) == 0 {
		return []SearchResult{}, nil
	}

	// Project IndexEntry slice to search.IndexEntry slice.
	searchEntries := make([]bm25.IndexEntry, len(filtered))
	for i, e := range filtered {
		searchEntries[i] = toSearchEntry(e)
	}

	// Run BM25 scoring.
	scored := bm25.Score(searchEntries, query, "", bm25.DefaultBM25Params())

	// Sort descending by score (Score already returns descending, but be explicit).
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	// Build result slice, capped to limit.
	if limit <= 0 {
		limit = len(scored)
	}
	if limit > len(scored) {
		limit = len(scored)
	}

	results := make([]SearchResult, 0, limit)

	for i := range limit {
		se := scored[i]
		// Find the original IndexEntry for this scored entry.
		var orig IndexEntry
		for _, e := range filtered {
			if e.SpecName == se.Entry.SpecName {
				orig = e
				break
			}
		}

		results = append(results, SearchResult{
			SpecName:        orig.SpecName,
			Similarity:      se.Score,
			TaskType:        orig.TaskType,
			Effort:          orig.Effort,
			FlowTemplate:    orig.FlowTemplate,
			OneLiner:        orig.OneLiner,
			DesignExcerpt:   readDesignExcerpt(idx.specsDir, orig.SpecName),
			Outcome:         orig.Outcome,
			TokensTotal:     orig.TokensTotal,
			DurationTotalMs: orig.DurationMs,
		})
	}

	return results, nil
}

// toSearchEntry projects a history.IndexEntry into a search.IndexEntry for BM25 scoring.
func toSearchEntry(e IndexEntry) bm25.IndexEntry {
	v := e.TaskType
	return bm25.IndexEntry{
		SpecName:       e.SpecName,
		RequestSummary: e.OneLiner + " " + strings.Join(e.Tags, " "),
		TaskType:       &v,
		Outcome:        e.Outcome,
	}
}

// readDesignExcerpt returns the first 200 bytes of the design.md file for a given
// spec. Returns "" if the file is absent or cannot be read.
func readDesignExcerpt(specsDir, specName string) string {
	path := filepath.Join(specsDir, specName, "design.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if len(data) > 200 {
		data = data[:200]
	}
	return string(data)
}

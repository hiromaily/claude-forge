// Package search implements BM25 scoring for the specs index.
//
// The [Scorer] takes a query string and a set of [Document] records
// (loaded from .specs/index.json) and returns ranked results using
// IDF-weighted term frequency with length normalisation (k1=1.5, b=0.75).
//
// Used by the search_patterns MCP tool to find similar past pipelines
// and inject relevant history into agent prompts.
//
// Import direction: search has no internal dependencies (leaf package).
package search

// Package indexer implements the specs index builder for the BM25 search
// system.
//
// [BuildSpecsIndex] scans a .specs/ directory, extracts metadata from each
// workspace (request summary, review feedback, implementation outcomes and
// patterns), and writes an index.json file consumed by the search_patterns
// MCP tool.
//
// Import direction: indexer → state (reads artifact filenames and state schemas).
package indexer

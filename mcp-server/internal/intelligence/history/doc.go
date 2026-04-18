// Package history provides cross-pipeline learning by indexing and querying
// completed pipeline runs stored in .specs/ directories.
//
// Key components:
//   - [Index]: scans state.json and request.md files, builds [IndexEntry]
//     records, persists them to .specs/history-index.json, and supports
//     incremental updates using an indexedAt watermark.
//   - [Search]: BM25-scored similarity search over indexed pipelines,
//     returning ranked matches for a given query.
//   - [Patterns]: extracts and Levenshtein-merges review findings from
//     past review-*.md files into normalized [PatternEntry] records.
//   - [Friction]: extracts AI friction points from past improvement.md
//     files into [FrictionPoint] records.
//   - [KnowledgeBase]: unified facade over Index, Patterns, and Friction
//     for pipeline_report_result to update after each completed run.
//
// Import direction: history → state (reads state.json schemas).
package history

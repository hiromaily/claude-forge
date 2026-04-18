// Package analytics aggregates pipeline history into cost/time predictions
// and per-pipeline summaries.
//
// It provides three MCP tool handlers:
//   - [AnalyticsEstimateHandler]: P50/P90 predictions for tokens, duration,
//     and cost for a given (task_type, effort) combination.
//   - [AnalyticsPipelineSummaryHandler]: token, duration, cost, and
//     review-finding statistics for a single pipeline run.
//   - [AnalyticsRepoDashboardHandler]: aggregate statistics across all
//     pipeline runs in .specs/.
//
// The [Collector] reads state.json and phase-log entries from completed
// pipeline workspaces; the [Estimator] computes percentile-based predictions
// from collected data.
//
// Import direction: analytics → state (reads state.json schemas).
package analytics

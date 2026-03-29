# Pipeline Summary: analytics-engine-mcp-tools

## Feature
Added analytics engine to the forge-state MCP server: `Collector`, `Estimator`, `Reporter` types in `mcp-server/analytics/` plus three new MCP tools (`analytics_pipeline_summary`, `analytics_repo_dashboard`, `analytics_estimate`).

## PR
https://github.com/hiromaily/claude-forge/pull/92

## What was built

### New package: `mcp-server/analytics/`
- **`Collector`** — reads `state.json` from a pipeline workspace, sums PhaseLog tokens/duration, counts phases, aggregates task retries, parses review findings via `orchestrator.ParseVerdict`
- **`Estimator`** — scans all completed pipelines in `.specs/`, filters by `(taskType, effort)`, computes P50/P90 percentiles for tokens, duration, and cost; confidence levels: `low` (<3), `medium` (3–9), `high` (≥10)
- **`Reporter`** — builds a `RepoDashboard` with total/completed/abandoned counts, ByTaskType/ByFlowTemplate breakdowns, ReviewPassRate, AvgRetriesPerPipeline, MostCommonFindings from `history.KnowledgeBase`

### New MCP tools (42 → 45)
| Tool | Description |
|---|---|
| `analytics_pipeline_summary` | Token, duration, cost, findings for one pipeline |
| `analytics_repo_dashboard` | Aggregate stats across all `.specs/` pipelines |
| `analytics_estimate` | P50/P90 predictions for a (taskType, effort) pair |

### Key implementation decisions
- `costPerToken = 0.000006` (Sonnet approximate rate)
- `errors.Is(err, os.ErrNotExist)` used (not `os.IsNotExist`) to handle wrapped errors from `ParseVerdict`
- `slices.Sort` per Go 1.26 modernize requirements
- `//nolint:cyclop,gocyclo` on `Dashboard()` — complexity inherent in multi-dimensional aggregation
- Nil dependency returns `IsError=true` MCP result (not Go error) — consistent with existing handler pattern

## Metrics
- **Total tokens:** 426,140
- **Total duration:** ~16 min
- **Tasks completed:** 13 / 13
- **Design revisions:** 1 inline
- **Task revisions:** 1 inline
- **Test coverage:** 12 packages all green; `golangci-lint` clean

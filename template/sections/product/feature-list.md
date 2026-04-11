## Feature list

- **Effort-aware scaling** — effort level (S/M/L) selects one of 3 flow templates (light/standard/full), from a lean pipeline to a full 10+ agent run with mandatory checkpoints
- **Deterministic hook guardrails** — PreToolUse hooks block source edits during analysis, block git commits during parallel execution, and block checkout to main/master during an active pipeline
- **AI review loops** — Design and task plans go through APPROVE/REVISE cycles with dedicated reviewer agents before implementation begins
- **Multi-phase pipeline** — 10 specialist agents across up to 12 phases (analysis → investigation → design → review → tasks → review → implementation → code review → comprehensive review → verification → PR → summary)
- **Parallel implementation** — Tasks marked `[parallel]` run concurrently with mkdir-based atomic locking for state updates
- **Human checkpoints** — Pause for human approval at design and task decomposition stages; skippable with `--auto` (except `full` template)
- **Improvement report** — Always-on retrospective appended to `summary.md` identifying documentation gaps, code readability friction, and AI agent support issues encountered during the run
- **Past implementation pattern injection** — Before each implementer invocation, `mcp__forge-state__search_patterns` (BM25 scorer) scans the specs index for similar past pipelines and injects their file-modification patterns into the prompt, surfacing real implementation examples rather than generic guidance
- **Disk-based state machine** — All progress tracked in `state.json` via the Go MCP server (44 MCP tools including `search_patterns`, `subscribe_events`, `ast_summary`, `ast_find_definition`, `dependency_graph`, `impact_scope`, `validate_input`, `validate_artifact`, `pipeline_init`, `pipeline_init_with_context`, `pipeline_next_action`, `pipeline_report_result`, `profile_get`, `history_search`, `history_get_patterns`, `history_get_friction_map`, `analytics_pipeline_summary`, `analytics_repo_dashboard`, and `analytics_estimate`); pipelines survive context compaction and session restarts
- **Resume and abandon** — Resume an interrupted pipeline from any phase; abandon cleanly when needed
- **Input validation** — Two-layer guard: deterministic `mcp__forge-state__validate_input` MCP tool (empty, too-short, malformed URL) + LLM semantic check blocks nonsensical or non-development requests before any tokens are spent on workspace setup
- **Phase metrics** — Every agent invocation logged with token count, duration, and model; included in the Final Summary
- **Source integration** — Accepts GitHub Issue URLs or Jira Issue URLs as input; posts the final summary back as a comment
- **Automatic PR creation** — Commits, pushes, and opens a GitHub PR with a structured summary; skippable with `--nopr`
- **Debug report** — `--debug` flag appends a `## Debug Report` to `summary.md` with execution flow diagnostics: token outliers, retry counts, revision cycles, and missing phase-log entries
- **Comprehensive test suite** — Automated tests covering state management, all hook scripts, and edge cases
- **Fail-open hooks** — Hooks never block non-pipeline work; gracefully degrade if `jq` is missing

---

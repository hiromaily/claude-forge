# MCP Tools Reference

The `forge-state` MCP server exposes **44 typed tool calls**. Tool names use underscores (MCP protocol requirement).

## Lifecycle

| MCP Tool | Description |
|---|---|
| `mcp__forge-state__init` | Create new workspace and state.json |
| `mcp__forge-state__pipeline_init` | Initialize pipeline with full context |
| `mcp__forge-state__pipeline_init_with_context` | Initialize with external context (Jira/GitHub) |
| `mcp__forge-state__pipeline_next_action` | Get next action for orchestrator to execute |
| `mcp__forge-state__pipeline_report_result` | Report phase result and advance pipeline |

## Phase Management

| MCP Tool | Description |
|---|---|
| `mcp__forge-state__phase_start` | Begin a phase |
| `mcp__forge-state__phase_complete` | Complete a phase (artifact guards enforced) |
| `mcp__forge-state__phase_fail` | Record phase failure |
| `mcp__forge-state__checkpoint` | Enter human checkpoint |
| `mcp__forge-state__skip_phase` | Skip a phase |
| `mcp__forge-state__abandon` | Abandon the pipeline |

## Revision Control

| MCP Tool | Description |
|---|---|
| `mcp__forge-state__revision_bump` | Full revision cycle (re-run phase) |
| `mcp__forge-state__inline_revision_bump` | Minor fixes without re-running |
| `mcp__forge-state__set_revision_pending` | Mark revision as pending |
| `mcp__forge-state__clear_revision_pending` | Clear pending revision |

## Configuration

| MCP Tool | Description |
|---|---|
| `mcp__forge-state__set_branch` | Set git branch name |
| `mcp__forge-state__set_effort` | Set effort level (S/M/L) |
| `mcp__forge-state__set_flow_template` | Set flow template (light/standard/full) |
| `mcp__forge-state__set_auto_approve` | Enable auto-approve for checkpoints |
| `mcp__forge-state__set_skip_pr` | Skip PR creation |
| `mcp__forge-state__set_debug` | Enable debug mode |
| `mcp__forge-state__set_use_current_branch` | Use current branch instead of creating new |

## Task Management

| MCP Tool | Description |
|---|---|
| `mcp__forge-state__task_init` | Initialize task list from tasks.md |
| `mcp__forge-state__task_update` | Update task implementation/review status |

## Metrics

| MCP Tool | Description |
|---|---|
| `mcp__forge-state__phase_log` | Log phase metrics (tokens, duration, model) |
| `mcp__forge-state__phase_stats` | Get phase statistics |

## Query

| MCP Tool | Description |
|---|---|
| `mcp__forge-state__get` | Get current pipeline state |
| `mcp__forge-state__resume_info` | Get resume information for interrupted pipeline |
| `mcp__forge-state__search_patterns` | BM25 search over past pipeline specs index |
| `mcp__forge-state__subscribe_events` | Get SSE endpoint URL (requires `FORGE_EVENTS_PORT`) |
| `mcp__forge-state__profile_get` | Get cached repository profile |
| `mcp__forge-state__history_search` | Search past pipeline history |
| `mcp__forge-state__history_get_patterns` | Get accumulated review finding patterns |
| `mcp__forge-state__history_get_friction_map` | Get AI friction points from improvement reports |

## Analytics

| MCP Tool | Description |
|---|---|
| `mcp__forge-state__analytics_pipeline_summary` | Token, duration, cost stats for a single run |
| `mcp__forge-state__analytics_repo_dashboard` | Aggregate stats across all pipeline runs |
| `mcp__forge-state__analytics_estimate` | P50/P90 predictions for new runs |

## Code Analysis

| MCP Tool | Description |
|---|---|
| `mcp__forge-state__ast_summary` | Tree-sitter AST summary of a source file |
| `mcp__forge-state__ast_find_definition` | Locate and return a symbol's definition |
| `mcp__forge-state__dependency_graph` | File-level import graph as JSON |
| `mcp__forge-state__impact_scope` | Find files that call a given symbol |

## Validation & Utility

| MCP Tool | Description |
|---|---|
| `mcp__forge-state__validate_input` | Validate pipeline input (empty, too-short, URL format) |
| `mcp__forge-state__validate_artifact` | Check artifact exists and meets content constraints |
| `mcp__forge-state__refresh_index` | Refresh the `.specs/index.json` |

## Guards (Enforced by MCP Handlers)

The MCP server enforces these guards deterministically:

| Guard | Tool | Condition |
|-------|------|-----------|
| Artifact required | `phase_complete` | Blocks if expected artifact file is missing |
| Checkpoint required | `phase_complete` | Blocks unless `awaiting_human` status |
| Phase ordering | `phase_start` | Blocks if previous phase not completed |

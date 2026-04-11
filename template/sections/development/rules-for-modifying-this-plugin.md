## Rules for Modifying This Plugin

### Consistency requirements
- When adding/changing an agent's input files → update BOTH the agent .md file AND the Agent Roster table in SKILL.md
- When adding a new phase → add its ID to the Go MCP server state package and ensure `pipeline_next_action` recognises it
- When changing hook behavior → verify the hook's exit code semantics (exit 0 = allow, exit 2 = block)
- When changing state.json schema → ensure the Go MCP server (`mcp-server/state/`), all hook scripts, and SKILL.md are aligned

### Testing
- MCP state commands: use `cd mcp-server && go test ./state/...` to run the Go unit tests for all 26 state-management commands.
- Hook scripts: pipe sample JSON to stdin and check exit code
  ```bash
  echo '{"tool_name":"Edit","tool_input":{"file_path":"/src/foo.go"}}' | bash scripts/pre-tool-hook.sh
  echo $?  # should be 0 (no active pipeline) or 2 (blocked)
  ```
- **Full hook test suite** (run after any change):
  ```bash
  bash scripts/test-hooks.sh   # run to see current test count (62 tests)
  ```
- **Go MCP server tests** (run after any change to mcp-server/):
  ```bash
  cd mcp-server && go test -race ./...
  ```

### Key design decisions (see [docs/architecture/technical-decisions.md](../../../docs/architecture/technical-decisions.md) for details)
- All agents use `model: sonnet` — cost optimization. Change to `opus` for agents that need stronger reasoning.
- Hooks are fail-open — if jq is missing or parsing fails, the action is allowed rather than blocked.
- State is file-based (state.json) — survives context compaction and session restarts.
- The orchestrator never reads source code — only small artifact files. This is a token economy rule.
- **Prefer deterministic enforcement over prompt instructions.** The orchestrator (SKILL.md) is an LLM and may skip or misinterpret instructions non-deterministically. When adding or changing pipeline behavior, first consider whether a hook script can enforce it deterministically (exit 2 = block). Use prompt instructions only for behavior that cannot be expressed as a state-based guard. Examples: checkpoint guards (P10-3), artifact guards, read-only phase enforcement — all implemented as hooks, not just prose.

### Script structure conventions

**pre-tool-hook.sh** — Enforces four rules: Rule 1 (read-only guard in phase-1/2 with workspace carve-out for artifact writes), Rule 2 (parallel phase-5 git commit block), Rule 3f (effort null warning on phase-start phase-1, non-blocking), Rule 5 (main/master checkout block). Sources `scripts/common.sh` for `find_active_workspace`.

**find_active_workspace** — `pre-tool-hook.sh` and `stop-hook.sh` share the same predicate and both source `scripts/common.sh`. `post-agent-hook.sh` uses a different filter (`status == "in_progress"` only) and does NOT source `common.sh`. Do not unify post-agent-hook.sh's copy into common.sh.

**MCP tool count** — `scripts/state-manager.sh` no longer exists; all 26 state-management commands are implemented in the Go MCP server (`mcp-server/`). The `forge-state` MCP server exposes all 26 commands as typed tool calls, plus additional MCP-only tools (currently `search_patterns`, `subscribe_events`, `ast_summary`, `ast_find_definition`, `dependency_graph`, `impact_scope`, `validate_input`, `validate_artifact`, `pipeline_init`, `pipeline_init_with_context`, `pipeline_next_action`, `pipeline_report_result`, `profile_get`, `history_search`, `history_get_patterns`, `history_get_friction_map`, `analytics_pipeline_summary`, `analytics_repo_dashboard`, and `analytics_estimate`). The total MCP tool count is **44**. When adding or removing a command, update the count in `CLAUDE.md` (here), `scripts/README.md`, and `README.md`. The count drifted to "22" in documentation before and was caught only in a comprehensive review pass — keep it accurate.

**MCP-only tools** — `search_patterns`, `subscribe_events`, `ast_summary`, `ast_find_definition`, `dependency_graph`, `impact_scope`, `validate_input`, `validate_artifact`, `pipeline_init`, `pipeline_init_with_context`, `pipeline_next_action`, `pipeline_report_result`, `profile_get`, `history_search`, `history_get_patterns`, `history_get_friction_map`, `analytics_pipeline_summary`, `analytics_repo_dashboard`, and `analytics_estimate` are exposed as MCP tools but have no shell equivalents. `search_patterns` performs BM25 scoring over `.specs/index.json` (see design rationale in `ARCHITECTURE.md`); no shell fallback exists. `subscribe_events` is a discovery tool that returns the SSE endpoint URL when `FORGE_EVENTS_PORT` is set; there is no shell equivalent. `ast_summary` parses a source file with tree-sitter and returns a compact markdown summary of exported signatures only. `ast_find_definition` locates and returns the definition of a named symbol in a source file. `dependency_graph` walks a source tree and returns a file-level import graph as JSON (nodes = files, edges = imports). `impact_scope` identifies files that call a given symbol via a two-pass import+call-site scan; returns a ranked list (`distance: -1` for TypeScript/Python). `validate_input` validates the raw pipeline input string (empty, too-short, URL format); no shell fallback exists. `validate_artifact` checks that the expected artifact file exists for a given phase and meets content constraints; always returns a JSON array. `pipeline_next_action` reads current pipeline state and returns the next action for the orchestrator to execute (spawn_agent, checkpoint, exec, write_file, or done); it reads agent `.md` files from the path set by the `FORGE_AGENTS_PATH` environment variable — this **must** be set in production to the absolute path of the `agents/` directory. `pipeline_report_result` records a phase-log entry, validates artifacts, parses review verdicts, and advances pipeline state. `profile_get` returns the cached repository profile (languages, CI system, linter configs, build/test commands) computed by `RepoProfiler`; triggers `AnalyzeOrUpdate()` on first call and returns a fresh or cached result. `history_search` queries the history index built from past `.specs/` pipeline runs using BM25 scoring and returns ranked similar past pipelines; accepts `query`, `limit`, and optional `task_type_filter`. `history_get_patterns` returns accumulated review-finding patterns (normalized, Levenshtein-merged) from past pipeline review phases; accepts `agent_filter`, `severity_filter`, and `limit`. `history_get_friction_map` returns accumulated AI friction points extracted from `improvement.md` files in past spec directories; accepts no parameters. `analytics_pipeline_summary` returns token, duration, cost, and review-finding statistics for a single pipeline run; accepts `workspace`. `analytics_repo_dashboard` returns aggregate statistics across all pipeline runs in `.specs/` (counts, averages, cost, review pass rate, common findings); accepts no parameters. `analytics_estimate` returns P50/P90 predictions for tokens, duration, and cost for a given `(task_type, effort)` combination based on historical pipeline runs; accepts `task_type` and `effort`.

**Canonical command list** (shell name → MCP tool name):

| Shell command | MCP tool | Category |
|---|---|---|
| `init` | `init` | Lifecycle |
| _(MCP-only)_ | `pipeline_init` | Lifecycle |
| _(MCP-only)_ | `pipeline_init_with_context` | Lifecycle |
| _(MCP-only)_ | `pipeline_next_action` | Lifecycle |
| _(MCP-only)_ | `pipeline_report_result` | Lifecycle |
| `get` | `get` | Read |
| `phase-start` | `phase_start` | Phase |
| `phase-complete` | `phase_complete` | Phase |
| `phase-fail` | `phase_fail` | Phase |
| `checkpoint` | `checkpoint` | Phase |
| `skip-phase` | `skip_phase` | Phase |
| `abandon` | `abandon` | Phase |
| `revision-bump` | `revision_bump` | Revision |
| `inline-revision-bump` | `inline_revision_bump` | Revision |
| `set-revision-pending` | `set_revision_pending` | Revision |
| `clear-revision-pending` | `clear_revision_pending` | Revision |
| `set-branch` | `set_branch` | Config |
| `set-effort` | `set_effort` | Config |
| `set-flow-template` | `set_flow_template` | Config |
| `set-auto-approve` | `set_auto_approve` | Config |
| `set-skip-pr` | `set_skip_pr` | Config |
| `set-debug` | `set_debug` | Config |
| `set-use-current-branch` | `set_use_current_branch` | Config |
| `task-init` | `task_init` | Task |
| `task-update` | `task_update` | Task |
| `phase-log` | `phase_log` | Metrics |
| `phase-stats` | `phase_stats` | Metrics |
| `resume-info` | `resume_info` | Query |
| _(MCP-only)_ | `search_patterns` | Query |
| _(MCP-only)_ | `subscribe_events` | Query |
| _(MCP-only)_ | `ast_summary` | Query |
| _(MCP-only)_ | `ast_find_definition` | Query |
| _(MCP-only)_ | `dependency_graph` | Query |
| _(MCP-only)_ | `impact_scope` | Query |
| _(MCP-only)_ | `profile_get` | Query |
| _(MCP-only)_ | `history_search` | Query |
| _(MCP-only)_ | `history_get_patterns` | Query |
| _(MCP-only)_ | `history_get_friction_map` | Query |
| _(MCP-only)_ | `analytics_pipeline_summary` | Query |
| _(MCP-only)_ | `analytics_repo_dashboard` | Query |
| _(MCP-only)_ | `analytics_estimate` | Query |
| _(MCP-only)_ | `validate_input` | Validation |
| _(MCP-only)_ | `validate_artifact` | Validation |
| `refresh-index` | `refresh_index` | Utility |

> `FORGE_AGENTS_PATH` must be set to the absolute path of the `agents/` directory in production for `pipeline_next_action` to resolve agent `.md` files correctly.

### What NOT to do
- Do NOT add `isolation: "worktree"` to any Agent tool call — breaks inter-task visibility
- Do NOT duplicate agent instructions in SKILL.md prompts — agents have their own system prompts
- Do NOT store state in memory/conversation — use state.json via the `mcp__forge-state__*` MCP tools
- Do NOT use bare `flock` without a mkdir fallback — macOS lacks `flock` by default. Hook scripts use mkdir-based atomic locking; follow the same pattern if adding new shell scripts that need locking.
- Do NOT reverse the Go package import direction: `tools` → `orchestrator` → `state`. Shared packages (`history`, `profile`, `prompt`, `validation`, `events`) may import `state` but never `orchestrator` or `tools`. Violating this causes an import cycle (caught by `import_cycle_test.go`). See [docs/architecture/go-package-layering.md](../../../docs/architecture/go-package-layering.md) for the full rule set.

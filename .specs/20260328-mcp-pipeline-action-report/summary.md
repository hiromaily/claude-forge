# Pipeline Summary — Issue #73: pipeline_next_action + pipeline_report_result

## What was built

Two new MCP tool handlers for the `forge-state` server that replace 13 runtime branching decisions currently made by `SKILL.md`:

### `pipeline_next_action`
- Delegates to `Engine.NextAction(sm, userResponse)` using a **per-call StateManager** pattern (`NewStateManager()` + `LoadFromFile(workspace)`) to avoid workspace-binding races
- For `ActionSpawnAgent` actions: enriches the prompt with agent `.md` contents + input artifact contents via `enrichPrompt()` using the `FORGE_AGENTS_PATH`-resolved agents directory
- Fail-open: missing agent file → returns action with `warning` field, no MCP error

### `pipeline_report_result`
- Records phase metrics via `PhaseLog`
- Validates artifacts (skips unknown phases with warning)
- Parses verdicts using `orchestrator.ParseVerdict()` — phase-3b/4b routes to `RevisionBump` or `PhaseComplete`; phase-6 uses `ParseVerdict(filepath.Join(workspace, result.File))` (not `ArtifactResult.Valid`)
- Uses `loadState()` helper consistent with all other state-reading handlers

### Supporting changes
- `RegisterAll` updated to 7-parameter signature (`eng *orchestrator.Engine`, `agentDir string`)
- `resolveAgentDir()` in `main.go`: 3-stage resolution (env var → runtime.Caller(0) fallback → "agents")
- Tool count: 36 → 38 everywhere in docs

## Files changed

`mcp-server/tools/pipeline_next_action.go` (new), `pipeline_next_action_test.go` (7 tests), `pipeline_report_result.go` (new), `pipeline_report_result_test.go` (10 tests), `registry.go`, `registry_test.go`, `handlers_test.go`, `main.go`, `CLAUDE.md`, `scripts/README.md`, `README.md`

## PR

https://github.com/hiromaily/claude-forge/pull/85

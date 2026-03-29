# Claude-Forge Scripts

Shell scripts for state management and hook enforcement. All scripts require `jq` and run on macOS (no flock dependency).

## Scripts

| Script | Purpose | Called by |
|--------|---------|-----------|
| [`common.sh`](common.sh) | Shared helper library sourced by `pre-tool-hook.sh` and `stop-hook.sh`. Provides the `find_active_workspace` function, which locates the most recently updated pipeline workspace whose `currentPhaseStatus` is not `completed` or `abandoned`. Not a standalone script — not executable. Note: `post-agent-hook.sh` intentionally does NOT source this file (it uses a different predicate). | Sourced by `pre-tool-hook.sh` and `stop-hook.sh` |
| [`pre-tool-hook.sh`](pre-tool-hook.sh) | PreToolUse hook. Enforces three rules: Rule 1 (blocks Edit/Write on source files during Phase 1-2 read-only enforcement, with a workspace carve-out for artifact writes), Rule 2 (blocks `git commit` during parallel Phase 5 tasks for batch-commit enforcement), Rule 5 (blocks `git checkout/switch` to `main` or `master` during an active pipeline). Sources `scripts/common.sh` for `find_active_workspace`. Fail-open: allows action if no active pipeline or jq unavailable. | Claude Code (automatic, via hooks.json) |
| [`post-agent-hook.sh`](post-agent-hook.sh) | PostToolUse hook for Agent tool calls. Validates output quality — warns if agent output is empty (< 50 chars) or missing expected verdict keywords (APPROVE/REVISE for review phases, PASS/FAIL for implementation review). | Claude Code (automatic, via hooks.json) |
| [`stop-hook.sh`](stop-hook.sh) | Stop hook. Prevents Claude from ending a conversation while a pipeline is active and `summary.md` has not been written. Allows stop at human checkpoints (awaiting_human), after completion, and after abandonment. | Claude Code (automatic, via hooks.json) |
| [`test-hooks.sh`](test-hooks.sh) | Automated test suite (58 tests). Covers hook scripts (`pre-tool-hook.sh`, `post-agent-hook.sh`, `stop-hook.sh`, `post-bash-hook.sh`, `common.sh`), pre-tool-hook rules (read-only, commit blocking, main/master checkout block), and edge cases (abandoned pipelines, special characters, numeric type preservation). The four deleted scripts (`state-manager.sh`, `validate-input.sh`, `build-specs-index.sh`, `query-specs-index.sh`) are no longer tested here — their Go equivalents are tested via `cd mcp-server && go test ./...`. | Manual (`bash scripts/test-hooks.sh`) |

> **Note on deleted scripts:** `state-manager.sh`, `validate-input.sh`, `build-specs-index.sh`, and `query-specs-index.sh` have been removed. All 26 state-management commands from `state-manager.sh` are now implemented in the Go MCP server (`mcp-server/`). The specs-index build functionality from `build-specs-index.sh` is now implemented in Go at `mcp-server/indexer/specs_index.go` (package `indexer`, function `BuildSpecsIndex`). Use `mcp__forge-state__*` MCP tools for all state-management and index operations.

## Exit Code Convention

- **exit 0** — allow the action (or no active pipeline detected)
- **exit 2** + stderr message — block the action with an explanation

Hook scripts follow a **fail-open** policy: if `jq` is not installed or `state.json` cannot be read, they exit 0 (allow) to avoid disrupting non-pipeline work.

## Testing

```bash
# Run the full hook test suite (58 tests)
bash scripts/test-hooks.sh

# Go MCP server tests (state management, indexer, etc.)
cd mcp-server && go test -race ./...

# Hook scripts — pipe sample JSON and check exit code
echo '{}' | bash scripts/pre-tool-hook.sh; echo "exit: $?"
echo '{}' | bash scripts/stop-hook.sh; echo "exit: $?"
```

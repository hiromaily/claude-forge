# Hooks & Guardrails

Hooks enforce critical constraints at the shell level — deterministic guards that cannot be misinterpreted by the LLM.

## Hook Types

| Hook | Script | Trigger |
| --- | --- | --- |
| PreToolUse | `pre-tool-hook.sh` | Before any tool execution |
| PostToolUse (Agent) | `post-agent-hook.sh` | After agent returns |
| PostToolUse (Bash) | `post-bash-hook.sh` | After bash command completes |
| Stop | `stop-hook.sh` | When pipeline tries to stop |

## Exit Code Semantics

- `exit 0` — allow the action
- `exit 2` — block the action (hard stop)

## PreToolUse Rules

### Rule 1: Read-Only Guard

During Phase 1 (Situation Analysis) and Phase 2 (Investigation), source file edits are blocked. Only artifact writes to the workspace directory are allowed.

```
Phase 1-2 active + Edit/Write tool → exit 2 (blocked)
Phase 1-2 active + Edit/Write to .specs/ → exit 0 (allowed)
```

### Rule 2: Parallel Commit Block

During parallel Phase 5 execution, git commits are blocked. The orchestrator batch-commits after the parallel group finishes.

```
Parallel tasks active + git commit → exit 2 (blocked)
Sequential task + git commit → exit 0 (allowed)
```

### Rule 3: Main/Master Checkout Block

During an active pipeline, checking out `main` or `master` is blocked to prevent accidentally leaving the feature branch.

```
Active pipeline + git checkout main → exit 2 (blocked)
No active pipeline + git checkout main → exit 0 (allowed)
```

## PostToolUse: Agent Output Validation

After each agent returns, `post-agent-hook.sh` checks output quality:
- Warns if output is empty or incoherent
- Uses `status == "in_progress"` filter (different from other hooks)

## PostToolUse: Auto-Commit

`post-bash-hook.sh` auto-commits `state.json` and `summary.md` after the post-to-source phase completes.

## Stop Hook: Completion Guard

`stop-hook.sh` prevents premature pipeline termination — the pipeline must complete the `post-to-source` phase and produce `summary.md`, or be explicitly abandoned.

## Shared Helpers

`scripts/common.sh` provides `find_active_workspace` — used by `pre-tool-hook.sh` and `stop-hook.sh`. Note: `post-agent-hook.sh` uses a different filter and does NOT source `common.sh`.

## Testing

```bash
# Run the full hook test suite (58 tests)
bash scripts/test-hooks.sh

# Manual testing with sample input
echo '{"tool_name":"Edit","tool_input":{"file_path":"/src/foo.go"}}' \
  | bash scripts/pre-tool-hook.sh
echo $?  # 0 (no active pipeline) or 2 (blocked)
```

# Concurrency Model (Phase 5)

When tasks are marked `[parallel]`:

1. Orchestrator launches multiple `implementer` agents simultaneously
2. Hook blocks `git commit` for any Bash call when parallel tasks are `in_progress`
3. After all parallel agents complete, orchestrator does one batch `git commit`
4. The Go MCP server uses mutex-based locking for concurrent state.json updates

Sequential tasks self-commit and run one at a time.

## Hook Enforcement

The `pre-tool-hook.sh` Rule 2 detects parallel execution by checking whether any tasks have `implStatus == "in_progress"` in `state.json`. If multiple tasks are in progress simultaneously, any `git commit` Bash call exits with code 2 (blocked).

See [Hooks & Guardrails](hooks.md) and [Guard Catalogue](guard-catalogue.md) (Rule R2) for details.

---
paths: ["**/*.sh", "**/Makefile"]
---

# Shell Script Best Practices

## Shebang and shell options

- Use `#!/usr/bin/env bash` — never `/bin/sh` (this project uses bash-specific features).
- Hook scripts use `set -uo pipefail` (not `-e`) so they can continue after non-fatal errors.
- Non-hook CLI scripts use `set -euo pipefail` to fail fast on any error.
- Match the existing script's `set` line when editing — mixing `-e` and non-`-e` within one script causes unpredictable behaviour.

## Fail-open pattern (hook scripts)

Hooks must never block legitimate non-pipeline work. Follow the fail-open pattern:

```bash
# If jq is missing, allow the action rather than blocking
command -v jq >/dev/null 2>&1 || exit 0

# Wrap jq calls with fallback so parse failures don't block
VALUE="$(jq -r '.field // empty' <<< "$INPUT" 2>/dev/null || true)"
[ -z "$VALUE" ] && exit 0
```

- Exit 0 = allow. Exit 2 + message to stderr = block. Never exit 1 from a hook.
- Always guard against missing `jq`, missing fields, and empty strings before acting.

## jq usage

- Prefer a single `jq` invocation over multiple chained calls. Combining extractions into one call halves the subprocess overhead and avoids operator-precedence bugs:
  ```bash
  # Good — single call
  read -r PHASE STATUS <<< "$(jq -r '[.currentPhase, .currentPhaseStatus] | @tsv' <<< "$STATE")"

  # Avoid — two calls
  PHASE="$(echo "$STATE" | jq -r '.currentPhase')"
  STATUS="$(echo "$STATE" | jq -r '.currentPhaseStatus')"
  ```
- When combining multiple outputs with comma in jq, use explicit parentheses to avoid pipe-precedence bugs:
  ```bash
  # Bug: pipe applies to all comma-separated expressions
  jq -r '.skippedPhases[], .currentPhase | select(. == "phase-1")'

  # Correct: group with parentheses
  jq -r '(.skippedPhases[], .currentPhase) | select(. == "phase-1")'
  ```
- Use `// empty` instead of `// ""` when a missing field should produce no output (avoids treating the string `"null"` as a value).

## File locking (concurrent writes)

State management is now handled by the Go MCP server (`mcp-server/state/manager.go`), which uses mutex-based locking for concurrent `state.json` updates. Do not implement shell-level file locking for state transitions — call the appropriate `mcp__forge-state__*` MCP tool instead.

If you need shell-level atomic locking for other purposes (not state.json), use `mkdir`-based locking as a portable fallback on macOS (which does not ship `flock` by default):
- Try `flock` first (available on Linux and macOS with Homebrew coreutils).
- Fall back to `mkdir`-based atomic locking.
- Retry up to 50 times before force-breaking a stale lock.
- Use `trap 'rm -rf "${lock_file}"' EXIT` to guarantee cleanup even on unexpected exit.

Do not add a bare `flock` call without the mkdir fallback — it will break on stock macOS.

## Error handling

- Use a `die()` helper for fatal errors in CLI scripts: `die() { echo "script-name: $*" >&2; exit 1; }`.
- Write all error/warning messages to stderr (`>&2`), never stdout — stdout is for machine-readable output consumed by callers.
- Prefer `|| true` over bare `|| :` for readability when suppressing errors on non-critical commands.

## Variable safety

- Quote all variable expansions: `"$VAR"`, `"$@"`, `"${array[@]}"`.
- Use `local` for all variables inside functions to avoid leaking into the global scope.
- Initialise variables before use when `set -u` is active, or use `${VAR:-default}` to provide a fallback.

## stdin/stdout conventions (hook scripts)

- Hook scripts receive the Claude tool call as JSON on stdin. Read it once into a variable at the top:
  ```bash
  INPUT="$(cat)"
  ```
  Do not call `cat` again — stdin is consumed on first read.
- When **blocking** (`exit 2`): write a plain-text message to **stderr only**. stdout must stay empty.
  ```bash
  echo "BLOCKED: <reason>" >&2
  exit 2
  ```
- When **allowing** (`exit 0`): both stdout and stderr should be empty.

## Naming conventions

- **Script-level (global) variables**: `UPPER_CASE` (e.g., `TOOL_NAME`, `INPUT`, `WORKSPACE`).
- **Function-local variables**: `lower_case` with `local` declaration (e.g., `local state_file`).
- **Functions**: `snake_case` (e.g., `find_active_workspace`, `check_phase1_warnings`).

## Testing

- Run `bash scripts/test-hooks.sh` after every change to any hook script. All existing tests must continue to pass.
- State management is now handled by the Go MCP server. To test state commands, use the Go test suite:
  ```bash
  cd mcp-server && go test ./state/...
  ```
- Pipe sample JSON to hook scripts directly to test them in isolation:
  ```bash
  echo '{"tool_name":"Edit","tool_input":{"file_path":"/src/foo.go"}}' \
    | bash scripts/pre-tool-hook.sh; echo "exit: $?"
  ```

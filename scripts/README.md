# Claude-Forge Scripts

Shell scripts for state management and hook enforcement. All scripts require `jq` and run on macOS (no flock dependency).

## Scripts

| Script | Purpose | Called by |
|--------|---------|-----------|
| [`state-manager.sh`](state-manager.sh) | Core CLI for pipeline state transitions. Manages `state.json` with 26 subcommands (init, phase-start, phase-complete, phase-fail, checkpoint, task-init, task-update, revision-bump, inline-revision-bump, set-branch, set-task-type, set-effort, set-flow-template, set-auto-approve, set-skip-pr, set-debug, set-use-current-branch, set-revision-pending, clear-revision-pending, skip-phase, phase-log, phase-stats, resume-info, abandon, get, refresh-index). Uses mkdir-based file locking for safe concurrent access during parallel Phase 5. | SKILL.md orchestrator (via Bash tool) |
| [`build-specs-index.sh`](build-specs-index.sh) | Scans all `.specs/<workspace>/` directories and builds `.specs/index.json` — a structured array of pipeline records with `specName`, `timestamp`, `taskType`, `requestSummary`, `reviewFeedback`, `implOutcomes`, `implPatterns`, and `outcome` fields. `implPatterns` is extracted from `impl-*.md` files in each workspace: one `{taskTitle, filesModified}` object per impl file, with file names stripped of absolute path prefixes and capped at 5 per impl file. Idempotent: rebuilds index from scratch on each run. Accepts an optional positional argument to override the `.specs/` directory path. | `state-manager.sh refresh-index` subcommand (or manual invocation) |
| [`query-specs-index.sh`](query-specs-index.sh) | Keyword-score matching against `.specs/index.json`. Reads the index built by `build-specs-index.sh`, scores past pipeline entries by taskType match (+2) and keyword overlap (+1 per word), and emits formatted markdown to stdout. Supports two modes via an optional third positional argument: the default (no arg or any value other than `impl`) emits a `## Past Review Feedback (from similar pipelines)` block with up to 3 top-scoring entries (requires non-empty `reviewFeedback`); the `impl` mode emits a `## Similar Past Implementations (from similar pipelines)` block with up to 2 top-scoring completed pipelines and up to 6 total `implPatterns` bullets, used in Phase 5 before the implementer agent loop. Returns empty stdout (exit 0) when the index is absent, empty, or no entries score >= 2. | SKILL.md orchestrator (via Bash tool, Phase 3 before architect agent, Phase 4 before task-decomposer agent, and Phase 5 before implementer agent loop) |
| [`validate-input.sh`](validate-input.sh) | Deterministic input validation. Checks for empty input, too-short descriptions (< 5 chars after flag stripping), flags-only input, and URL format validation (GitHub Issue, Jira Issue). Writes a timestamp marker on success for pre-tool-hook's init guard. Exit 0 = valid, exit 1 = invalid. | SKILL.md orchestrator (via Bash tool, before Workspace Setup) |
| [`pre-tool-hook.sh`](pre-tool-hook.sh) | PreToolUse hook. Blocks Edit/Write on source files during Phase 1-2 (read-only enforcement), blocks `git commit` during parallel Phase 5 tasks (batch-commit enforcement), blocks `phase-complete` for checkpoint phases unless `currentPhaseStatus` is `awaiting_human` (checkpoint guard), blocks `phase-complete` when required artifact files are missing (artifact guard), and blocks `state-manager.sh init` without prior `validate-input.sh` call (input validation guard). Fail-open: allows action if no active pipeline or jq unavailable. | Claude Code (automatic, via hooks.json) |
| [`post-agent-hook.sh`](post-agent-hook.sh) | PostToolUse hook for Agent tool calls. Validates output quality — warns if agent output is empty (< 50 chars) or missing expected verdict keywords (APPROVE/REVISE for review phases, PASS/FAIL for implementation review). | Claude Code (automatic, via hooks.json) |
| [`stop-hook.sh`](stop-hook.sh) | Stop hook. Prevents Claude from ending a conversation while a pipeline is active and `summary.md` has not been written. Allows stop at human checkpoints (awaiting_human), after completion, and after abandonment. | Claude Code (automatic, via hooks.json) |
| [`test-hooks.sh`](test-hooks.sh) | Automated test suite. Covers all state-manager commands, validate-input.sh checks, pre-tool-hook rules (read-only, commit blocking, checkpoint guard, artifact guard, input validation guard), post-agent-hook validation, stop-hook behavior, and edge cases (abandoned pipelines, special characters, numeric type preservation) — run to see current count. | Manual (`bash scripts/test-hooks.sh`) |

## Exit Code Convention

- **exit 0** — allow the action (or no active pipeline detected)
- **exit 2** + stderr message — block the action with an explanation

Hook scripts follow a **fail-open** policy: if `jq` is not installed or `state.json` cannot be read, they exit 0 (allow) to avoid disrupting non-pipeline work.

## Testing

```bash
# Run the full test suite
bash scripts/test-hooks.sh

# state-manager.sh — run commands against a temp workspace
mkdir -p /tmp/test && bash scripts/state-manager.sh init /tmp/test my-spec
bash scripts/state-manager.sh set-task-type /tmp/test feature
bash scripts/state-manager.sh set-auto-approve /tmp/test
bash scripts/state-manager.sh phase-log /tmp/test phase-1 5000 30000 sonnet
bash scripts/state-manager.sh phase-stats /tmp/test
bash scripts/state-manager.sh resume-info /tmp/test
rm -rf /tmp/test

# Hook scripts — pipe sample JSON and check exit code
echo '{}' | bash scripts/pre-tool-hook.sh; echo "exit: $?"
echo '{}' | bash scripts/stop-hook.sh; echo "exit: $?"
```

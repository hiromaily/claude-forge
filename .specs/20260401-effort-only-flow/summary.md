# feat: effort-only flow selection with always-prompt UX

Closes #108

## Summary

Removes task type entirely from the pipeline and makes all flow decisions driven exclusively by effort level (S, M, L). The 20-cell `(taskType, effort)` matrix is replaced with a 3-entry effort-only table.

### Key changes

- **Removed task type completely**: `DetectTaskType`, `ValidTaskTypes`, all `TaskType*` constants, and the `TaskType *string` field in `State` are deleted. Task type is not detected, not stored, not displayed, not indexed, and not used anywhere.
- **Removed XS effort**: XS was defined by its task-type-specific behavior (→ `direct`/`lite`). With task type gone, XS has no coherent definition. `--effort=XS` is now rejected at validation time.
- **Removed `direct` and `lite` templates**: Their skip sets existed only to serve XS cells and task-type-specific overrides.
- **Collapsed to 3-entry effort table**: S → `light`, M → `standard`, L → `full`.
- **New functions**: `EffortToTemplate(effort string) string` and `SkipsForEffort(effort string) []string`.
- **Always prompt for effort**: First call always returns `needs_user_confirmation` with all three options (S/M/L) and their phase previews. No `--auto` bypass.
- **Removed `set_task_type` MCP tool**: Tool count drops from 45 to 44.
- **Deleted `agents/analyst.md`**: The `lite` flow template and `agentAnalyst` dispatch are gone.
- **Simplified `derivePRTitle`**: Commit-type prefix derived from branch name (`feature/` → `feat`, `fix/` → `fix`, etc.).
- **Removed task type from analytics and search**: `ByTaskType`, `PipelineSummary.TaskType`, `IndexEntry.TaskType`, `task_type_filter` all removed.

### Skip sets

| Effort | Template | Skipped phases |
|--------|----------|----------------|
| S | `light` | `phase-4b`, `checkpoint-b`, `phase-7` |
| M | `standard` | `phase-4b`, `checkpoint-b` |
| L | `full` | _(none)_ |

## Test results

- `go build ./...`: PASS
- `go test -race ./...`: PASS (13 packages, 0 failures)
- `bash scripts/test-hooks.sh`: PASS (58/58 tests)

## Pipeline statistics

- Total tokens: 1,457,457
- Total duration: 4,948,659 ms (~82 min)
- Estimated cost: $8.74
- Phases executed: 14
- Phases skipped: 0
- Retries: 0
- Review findings: 0 critical, 7 minor (all addressed)
- PR: https://github.com/hiromaily/claude-forge/pull/116

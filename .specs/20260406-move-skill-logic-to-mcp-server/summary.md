# feat: absorb P1–P4 orchestration logic into MCP server

## Summary

This change absorbs four categories of LLM-delegated orchestration logic from `skills/forge/SKILL.md` into the Go MCP server, eliminating the non-deterministic orchestrator behaviour that caused five classes of production failures. Specifically:

- **P1** — The `skip:` completion loop is absorbed into `PipelineNextActionHandler`. The handler now resolves all consecutive skip signals internally via a new `PhaseCompleteSkipped` method on `StateManager`, returning only actionable results to the orchestrator.
- **P2** — `task_init` parsing is moved into the MCP server. A new `ParseTasksMd` lenient heuristic parser reads `tasks.md` directly; the engine emits a new `ActionTaskInit` type instead of the former `exec` + `task_init` command string.
- **P3** — `batch_commit` is implemented as `executeBatchCommit` in `git_ops.go`. The engine emits `ActionBatchCommit`; the handler builds the file list from `Tasks[k].Files` with a `git diff --name-only HEAD` fallback when the list is empty.
- **P4** — `final_commit` is implemented as `executeFinalCommit` in `git_ops.go`. The handler intercepts the `exec` + `final_commit` action, calls `handleReportResult` first (preserving the state-ordering invariant), then performs the `git add -f` / `commit --amend` / `push --force-with-lease` sequence in Go.

P5 (merging `pipeline_report_result` into `pipeline_next_action`) was explicitly deferred to a separate PR. The tool count remains at 44.

### Files changed
- `mcp-server/internal/orchestrator/actions.go` — 2 new action type constants + constructors
- `mcp-server/internal/orchestrator/engine.go` — emit new action types from `handlePhaseFive`
- `mcp-server/internal/orchestrator/engine_test.go` — updated for new action types
- `mcp-server/internal/state/manager.go` — `PhaseCompleteSkipped` method
- `mcp-server/internal/state/manager_test.go` — 2 new tests for `PhaseCompleteSkipped`
- `mcp-server/internal/tools/pipeline_next_action.go` — P1–P4 dispatch loop
- `mcp-server/internal/tools/pipeline_next_action_test.go` — updated + 3 new absorption subtests
- `mcp-server/internal/tools/pipeline_report_result.go` — dead `NeedsBatchCommit`-clearing block removed
- `mcp-server/internal/tools/task_ops.go` — new: `ParseTasksMd`, `executeTaskInit`
- `mcp-server/internal/tools/task_ops_test.go` — new: 8 table-driven subtests
- `mcp-server/internal/tools/git_ops.go` — new: `repoRoot`, `executeBatchCommit`, `executeFinalCommit`
- `mcp-server/internal/tools/git_ops_test.go` — new: `TestRepoRoot`, `TestExecuteBatchCommit_EmptyFiles`
- `mcp-server/internal/tools/pipeline_integration_test.go` — updated round-trip tests
- `skills/forge/SKILL.md` — P1–P4 prose removed; Step 2 loop reduced from 9 to 5 action types

PR: https://github.com/hiromaily/claude-forge/pull/131

## Verification Report

### Typecheck
- Status: PASS
- Errors: 0 (golangci-lint: 0 issues)

### Test Suite
- Total: 13 packages passed, 0 failed
- Failures: none

### Overall: PASS

## Pipeline Statistics
- Total tokens: 1,338,796
- Total duration: 2,705,741 ms
- Estimated cost: $8.03
- Phases executed: 14
- Phases skipped: 0
- Retries: 0
- Review findings: 0 critical, 12 minor

## Improvement Report

_Retrospective on what would have made this work easier. **Applied 2026-04-07.**_

### Documentation

The `tasks.md` format produced by the `task-decomposer` agent was not documented anywhere in a machine-readable form. The only reference was `minimalTasksContent` in `engine.go` (a constant string for the minimal case) and the agent's `.md` instructions, which describe the richer format informally. This forced the investigation phase to derive the canonical format from both sources and commit it as a comment inside `parseTasksMd`. If the format had been documented as a schema in `agents/task-decomposer.md` the analysis and design phases would have been shorter.

The import-DAG constraint (`tools -> orchestrator -> state`) was documented in `CLAUDE.md` but not prominently visible to agents reading only the architecture files. The investigation had to read `import_cycle_test.go` to confirm the boundary. `docs/architecture/go-package-layering.md` covers this but is not directly referenced from `CLAUDE.md`'s "Before You Start Working" checklist. A direct pointer there would reduce lookup cost for future agents.

### Code Readability

The six engine call sites that emit `ActionDone{Summary:"skip:<phase-id>"}` are spread across `engine.go` without a centralised comment explaining the skip-signal protocol. A reader encountering `NewDoneAction(SkipSummaryPrefix+PhaseCheckpointA, "")` for the first time must search the codebase to understand that the caller is expected to call `PhaseComplete` and loop. A package-level comment on `SkipSummaryPrefix` in `actions.go` documenting the consumer-side loop contract would reduce this cognitive load for future implementers.

The `NeedsBatchCommit` setting and clearing logic was split across two non-adjacent blocks in `pipeline_report_result.go` with no cross-references. After P3 the clearing block was removed, but before the change a reader had to hold both sites in mind to understand the lifecycle. Linking "set" and "clear" sites by comment would help future refactors.

### AI Agent Support (Skills / Rules)

The `testing.md` rule does not explicitly state that the `mcp-server/` directory is a separate Go module requiring `cd mcp-server` before any `go` command. Agents running from the repo root may invoke `go test ./...` without the directory change and observe no failures (there are no Go files at the root). Adding one line — "All `go` commands must be run from inside `mcp-server/`" — would prevent this class of agent mistake.

The designer agent's instructions do not include a checklist item for "list all exhaustive-switch sites that need updating when adding a new constant". The two new `ActionType` constants required updating four switch sites, and the review caught a `_ = iter` suppression that masked the missing cycle-detection guard in the P2/P3/P4 dispatch loop. Surfacing this class of impact earlier (in the design phase) would prevent it from appearing as a review finding.

### Other

The `Guard3eCheckpointAwaitingHuman` conflict for the `checkpoint-a` auto-skip case was the hardest blocker to reason about during investigation. The guard's existing exception checks `slices.Contains(s.SkippedPhases, phase)`, but the `handleCheckpointA` skip is driven by `(phase-4 in skippedPhases) && autoApprove` — a different predicate. This asymmetry required introducing `PhaseCompleteSkipped`. The BACKLOG.md entry for this issue helped frame the problem, but the guard bypass options were not pre-enumerated. Recording design options alongside BACKLOG entries at discovery time would shorten the design phase for future work touching the same guard.

# Summary: mcp-server-deterministic-refactor

## What was done

Task 1 of 9 was implemented: `reportResultCore` was extracted from `handleReportResult` in `mcp-server/internal/tools/pipeline_report_result.go`. The inner response type was renamed from `reportResultResponse` to `reportResultOutcome` throughout the file. `handleReportResult` was reduced to a thin wrapper that calls `reportResultCore` and returns `okJSON(out)`. `PipelineReportResultHandler` was left unchanged. All 13 Go packages pass `go test -race ./...` and all 62 hook tests pass.

This refactor creates the callable `reportResultCore(sm, kb, in) (reportResultOutcome, error)` foundation that Task 2 depends on — enabling `PipelineNextActionHandler` to invoke report-result logic without going through the MCP wire format, which is the core goal of the deterministic 2-call loop design.

## PR

https://github.com/hiromaily/claude-forge/pull/146

Source: https://github.com/hiromaily/claude-forge/issues/145

## Remaining work (Tasks 2–9)

- **Task 2**: Add P5 block to `PipelineNextActionHandler` — define `previousResult`/`reportResultEmbedded` types, add `parsePreviousResult` helper, and call `reportResultCore` when `previous_tokens > 0` or `previous_model != ""` for non-terminal phases.
- **Task 3**: Register four optional parameters (`previous_tokens`, `previous_duration_ms`, `previous_model`, `previous_setup_only`) on the `pipeline_next_action` tool in `registry.go`.
- **Task 4**: Add P5 unit tests (three cases: proceed, revision-required, setup-continue) and integration tests in `pipeline_next_action_test.go` and `pipeline_integration_test.go`.
- **Task 5**: Decompose `pipeline_report_result.go` into `verdict_parser.go`, `phase_transition.go`, and `artifact_guard.go`. Create `verdict_parser_test.go` with direct unit tests.
- **Task 6**: Decompose `pipeline_init_with_context.go` into `context_fetcher.go` and `workspace_init.go`.
- **Task 7**: Extract response helpers from `handlers.go` into `response_helpers.go`. `helpers.go` stays unchanged.
- **Task 8**: Rewrite `skills/forge/SKILL.md` to the 2-call loop — remove `pipeline_report_result` from the main loop.
- **Task 9**: Full verification pass — `go test -race ./...`, `bash scripts/test-hooks.sh`, `TestRegisterAllCount`.

## Pipeline Statistics
- Total tokens: 1,030,986
- Total duration: 2,993,228 ms (49m 53s)
- Estimated cost: $6.19
- Phases executed: 14
- Phases skipped: 0
- Retries: 0
- Review findings: 0 critical, 6 minor

## Improvement Report

_Retrospective on what would have made this work easier._

### Documentation

`analysis.md` and `investigation.md` were not persisted to disk, removing visibility into friction encountered during earlier phases. The design for the 2-call deterministic loop spans three interdependent files (`pipeline_next_action.go`, `registry.go`, `SKILL.md`) with no cross-reference comment in the source tying them together. A short comment block at the top of `pipeline_next_action.go` explaining the intended 2-call contract would reduce discovery burden for future implementers.

### Code Readability

`pipeline_report_result.go` co-locates wire-format parsing, artifact guards, verdict determination, phase transition logic, and response helpers. The `reportResultResponse` → `reportResultOutcome` rename required a grep across the whole `mcp-server/` tree to confirm zero residual references — unnecessary if the original name had more clearly indicated its scope.

### AI Agent Support (Skills / Rules)

The pipeline ran only Task 1 of 9, leaving 8 tasks unimplemented. The orchestrator's task-dispatch logic did not continue to Tasks 2–9 in this run. A `resume` flow or explicit "tasks remaining" warning in the final-summary phase would surface this to the operator earlier. No existing CLAUDE.md rule addresses the pattern of large handler files accumulating multiple concerns; a "single responsibility per file" guideline for `mcp-server/internal/tools/` would better justify decomposition tasks.

### Other

The design revision loop (phase-3 architect repeatedly dispatched instead of phase-3b reviewer) added multiple unnecessary architecture agent runs. The phase-3b `revisionPending` flag was only cleared by re-running the design-reviewer directly, bypassing the engine's returned action. This is a known orchestrator loop bug worth addressing in the MCP server engine logic.

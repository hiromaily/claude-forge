## Summary

Added `mcp-server/internal/tools/pipeline_e2e_test.go` (269 lines) — four end-to-end integration tests that drive the full pipeline from `setup` to `completed` using mock artifact writes in place of a real LLM agent. The tests exercise `PipelineNextActionHandler` and `PipelineReportResultHandler` in a loop, covering cross-phase ordering regressions that were not previously caught by unit or integration tests. Three test variants (standard, light, full templates) are handled as table-driven subtests in `TestE2E_Templates`, and a fourth standalone test `TestE2E_DesignRevisionCycle` exercises the REVISE-then-APPROVE path with assertion on `DesignRevisions == 1`. All 13 Go packages pass under `go test -race ./...`.

PR: https://github.com/hiromaily/claude-forge/pull/151

## Pipeline Statistics
- Total tokens: 310,636
- Total duration: 844,570 ms
- Estimated cost: $1.86
- Phases executed: 10
- Phases skipped: 2
- Retries: 0
- Review findings: 0 critical, 3 minor

## Improvement Report

_Retrospective on what would have made this work easier._

### Documentation

The task decomposer specified running `golangci-lint` against a single `_test.go` file path rather than a package path (`./internal/tools/...`). Running the linter on a single test file produces typecheck errors because sibling test-file helpers are not visible. The task description should always specify package-scope lint invocations. A note in `.claude/rules/` clarifying this convention would prevent future confusion.

The investigation phase had to re-read `pipeline_report_result.go` multiple times to determine whether `phase5CompletionGate` blocked on missing `impl-N.md` files or on `ImplStatus` state. A short inline comment near the `phase5CompletionGate` call explaining that task status is updated by `determineTransition` (not by the mock) would shorten investigation.

### Code Readability

`handlePhaseThreeB` silently bypasses checkpoint-a when `AutoApprove=true` and APPROVE verdict is present — dispatching phase-4 directly without emitting a checkpoint action. This is not obvious from the function name. A comment at the dispatch point would prevent future agents from re-verifying this.

`BranchClassified` is not part of `PipelineConfig`, requiring a separate `sm.Update` call after `sm.Configure`. Either including it in `PipelineConfig` or adding a comment near the struct noting the separate-update requirement would reduce friction.

### AI Agent Support (Skills / Rules)

The pipeline engine has a bug where, after phase-3b returns REVISE, `pipeline_next_action` (with `previous_action_complete=false`) dispatches the architect (phase-3) correctly, but `pipeline_next_action` (with `previous_action_complete=true`) after the architect revision reads the old `review-design.md` (still REVISE) and returns `revision_required` instead of dispatching the design-reviewer. This creates an infinite loop that required manual workaround (manually spawning the design-reviewer). This should be tracked as a bug fix in BACKLOG.md.

### Other

The comprehensive-review mid-run statistics differed from the end-of-run analytics figures because the analytics tool accumulates data incrementally. The convention to use `analytics_pipeline_summary` as the authoritative source is correctly documented and worked as expected.

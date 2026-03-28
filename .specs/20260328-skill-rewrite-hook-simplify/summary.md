# Pipeline Summary

**Request:** [MCP-Orch-A7] SKILL.md rewrite (~50 lines) + hook simplification + integration test
**Feature branch:** `feature/skill-rewrite-hook-simplify`
**Pull Request:** #86 (https://github.com/hiromaily/claude-forge/pull/86)
**Date:** 2026-03-28

## Tasks

| # | Title | Verdict |
|---|-------|---------|
| 1 | Extend `NewExecAction`/`NewWriteFileAction` constructors | PASS |
| 2 | Update engine.go call sites | PASS |
| 3 | Update Go unit tests (constructor + engine phase assertions) | PASS |
| 4 | Create `scripts/common.sh` | PASS |
| 5 | Simplify `scripts/pre-tool-hook.sh` | PASS |
| 6 | Simplify `scripts/stop-hook.sh` | PASS |
| 7 | Update `scripts/validate-input.sh` + `post-bash-hook.sh` | PASS |
| 8 | Rewrite `skills/forge/SKILL.md` | PASS |
| 9 | Add Go integration tests | PASS |
| 10 | Update `scripts/test-hooks.sh` | PASS |
| 11 | Update `CLAUDE.md` | PASS |
| 12 | Update `scripts/README.md` + `README.md` | PASS |

## Comprehensive Review

**Verdict: IMPROVED** — One stale README.md bullet fixed ("Deterministic hook guardrails" still described artifact enforcement; updated to reflect actual Rule 5 behavior).

## Notes

- Design required 2 revision cycles for Phase 3: one to resolve a CRITICAL bug (exec/write_file actions had empty `Phase` field, which would cause `pipeline_report_result` to return an MCP error), resolved by extending `NewExecAction`/`NewWriteFileAction` constructors with a `phase` parameter.
- `SKILL.md` reduced from 1,759 lines to 49 lines.
- `pre-tool-hook.sh` reduced from 355 lines to 53 lines (Rules 3a–3j and Rule 4 removed).
- `stop-hook.sh` reduced from 59 lines to 21 lines.
- Test suite reduced from 327 to 246 tests (obsolete Rule 3/4 guards removed).
- Tool count note: acceptance criteria said "23-tool count" but the actual count is 38; confirmed to be a typo in the issue.

## Test Results

- `bash scripts/test-hooks.sh`: **246 passed, 0 failed**
- `cd mcp-server && go test ./...`: **all 8 packages pass**
- `cd mcp-server && golangci-lint run`: **0 issues**

## Execution Stats

| Phase | Tokens | Duration | Model |
|-------|--------|----------|-------|
| phase-1 | 58,509 | 136.8s | sonnet |
| phase-2 | 114,673 | 267.6s | sonnet |
| phase-3 (×2) | 117,993 | 332.5s | sonnet |
| phase-3b (×4) | 194,049 | 354.7s | sonnet |
| phase-4 | 48,835 | 105.5s | sonnet |
| phase-4b | 30,939 | 32.3s | sonnet |
| task-1-impl (tasks 1+2) | 54,848 | 140.3s | sonnet |
| task-3-impl | 69,810 | 169.6s | sonnet |
| task-4-impl | 30,207 | 60.7s | sonnet |
| task-5-impl | 54,583 | 229.3s | sonnet |
| task-6-impl | 40,733 | 141.9s | sonnet |
| task-7-impl | 37,406 | 86.8s | sonnet |
| task-8-impl | 32,445 | 84.3s | sonnet |
| task-9-impl | 88,079 | 179.2s | sonnet |
| task-10-impl | 74,562 | 283.5s | sonnet |
| task-11-impl | 33,136 | 78.8s | sonnet |
| task-12-impl | 54,287 | 624.1s | sonnet |
| task reviews | 92,912 | 192.3s | sonnet |
| phase-7 | 85,613 | 197.3s | sonnet |
| final-verification | 27,383 | 47.7s | sonnet |
| **TOTAL** | **1,341,002** | **3,746s** | |

## Improvement Report

_Retrospective on what would have made this work easier._

### Documentation

The `pipeline_next_action` action struct (`orchestrator/actions.go`) lacked inline documentation clarifying which fields are populated by which constructors. The `Phase` field being empty for exec/write_file actions was not discoverable without reading all constructor implementations — a comment on the struct or constructor would have prevented the CRITICAL design finding in Phase 3b.

### Code Readability

The engine's stub synthesis functions (`handleDocsStubSynthesis`, `handleBugfixStubSynthesis`) were called without a phase argument despite returning phase-sensitive actions. The implicit phase dependency (inferred from the caller context) made the data flow non-obvious. Threading the phase as an explicit parameter (as implemented in Tasks 1–2) is the correct pattern.

### AI Agent Support (Skills / Rules)

The CLAUDE.md prohibition on `find_active_workspace` unification ("Do not unify them into a shared library") was absolute but the actual constraint was that the *predicates* differ. A more precise statement ("Do not unify copies with different predicates; copies with identical predicates may share a library") would have let the analyst reach the correct conclusion in Phase 1 rather than flagging it as a tension to resolve in design.

### Other

The acceptance criterion "README.md updated to reflect new 23-tool count" contained a typo (actual count is 38). Typos in acceptance criteria cause wasted investigation time verifying whether the spec is correct or the code is wrong. Acceptance criteria should be verified against the codebase before they are written into the issue.

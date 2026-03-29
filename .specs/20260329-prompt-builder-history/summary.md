# Pipeline Summary

**Request:** Dynamic 4-layer prompt builder with history + pattern context injection (issue #77)
**Feature branch:** `feature/prompt-builder-history`
**Pull Request:** #90 (https://github.com/hiromaily/claude-forge/pull/90)
**Date:** 20260329

## Tasks

| # | Title | Verdict |
|---|-------|---------|
| 1 | Create `prompt/templates.go` — inclusion rules | PASS |
| 2 | Create `prompt/context.go` — `HistoryContext` + `BuildContextFromResults` | PASS_WITH_NOTES |
| 3 | Create `prompt/builder.go` — `BuildPrompt` + token budget guard | PASS |
| 4 | Create `prompt/builder_test.go` — unit tests | PASS_WITH_NOTES |
| 5 | Extend `PipelineNextActionHandler` with `histIdx` + `kb` | PASS_WITH_NOTES |
| 6 | Update `registry.go` call site | PASS |
| 7 | Update 11 test call sites | PASS |

## Comprehensive Review

Verdict: **CLEAN** — No fixes required. All 10 packages pass with the race detector.

## Notes

- Task 2: `BuildContextFromResults` returns `HistoryContext{SimilarPipelines: results}` when `kb==nil` and results are non-nil — functionally superior to the AC's literal text, preserves data.
- Tasks 6 and 7 were implemented as part of Task 5 (build correctness required it); impl files created after the fact.
- `profile = ""` is hard-coded in `enrichPrompt` pending the C1 task that will implement `profile_get`.
- Layer 4's BM25 query uses `state.SpecName` as the search query (e.g. `prompt-builder-history`) — low semantic value but correct fallback; improves when `OneLiner` field is added to state.

## Test Results

- `go test -race ./prompt/...` — 16 tests PASS
- `go test ./tools/...` — all PASS  
- `go test ./orchestrator/...` — PASS (import cycle intact)
- `bash scripts/test-hooks.sh` — 246 tests PASS
- `go tool golangci-lint run` — 0 issues

## Execution Stats

| Phase | Tokens | Duration | Model |
|-------|--------|----------|-------|
| phase-1 | 72,439 | 103.8s | sonnet |
| phase-2 | 88,031 | 192.9s | sonnet |
| phase-3 | 32,157 | 95.5s | sonnet |
| phase-3b (×2) | 73,064 | 123.0s | sonnet |
| phase-4 | 34,734 | 56.7s | sonnet |
| phase-4b (×2) | 62,974 | 80.2s | sonnet |
| task-1-impl | 28,934 | 62.1s | sonnet |
| task-2-impl | 58,209 | 156.2s | sonnet |
| task-3-impl | 52,986 | 146.5s | sonnet |
| task-4-impl | 57,468 | 142.2s | sonnet |
| task-5-impl | 51,554 | 150.9s | sonnet |
| task-1-review | 45,860 | 121.6s | sonnet |
| task-5-review | 36,336 | 82.1s | sonnet |
| phase-7 | 49,947 | 99.3s | sonnet |
| final-verification | 24,961 | 33.3s | sonnet |
| **TOTAL** | **769,654** | **1647.0s** | |

## Improvement Report

_Retrospective on what would have made this work easier._

### Documentation

The `state.State` struct does not have an `OneLiner` field — this was discovered during analysis when the design referenced `state.OneLiner` as the BM25 query source. The struct's fields are not documented outside the code itself. Adding a short comment block to `state.State` listing fields and their purpose would prevent this kind of mismatch. Specifically, documenting that the `SpecName` field is a directory-name slug (not a human-readable task description) would have guided the design toward the correct query source immediately.

### Code Readability

The `PipelineNextActionHandler` function is large. Finding where to add the history context injection required careful reading of the closure structure. The function body comment density is low — a one-line comment before each logical section (read agent file, assemble artifacts, build prompt) would help future contributors locate the injection point without reading every line.

### AI Agent Support (Skills / Rules)

The `go-test.md` rule file is clear and comprehensive. No friction observed from missing rules. The convention of `package prompt` (same-package tests) vs `package prompt_test` (external) was resolved quickly from the rule file.

### Other

The `history.Search` function signature change (query source) was ambiguous between the design document and the task acceptance criteria. A design review note called this out but the implementer still had to infer the resolution. Keeping acceptance criteria and design document in sync on this kind of "source of query string" detail would reduce implementer ambiguity.

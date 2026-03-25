# Pipeline Summary

**Request:** [F8-c] Inject past implementation patterns into implementer prompts
**Feature branch:** `feature/inject-impl-patterns`
**Pull Request:** #57 (https://github.com/hiromaily/claude-forge/pull/57)
**Date:** 2026-03-25

## Tasks

| # | Title | Verdict |
|---|-------|---------|
| 1 | Add `extract_impl_patterns` to `build-specs-index.sh` | PASS_WITH_NOTES |
| 2 | Add `impl` mode to `query-specs-index.sh` | PASS |
| 3 | Inject `past_impl_patterns` into Phase 5 of `SKILL.md` | PASS |
| 4 | Add `run_qsi_impl` helper and Tests 21–26 to `test-hooks.sh` | PASS_WITH_NOTES |
| 5 | Update docs — `CLAUDE.md` + `scripts/README.md` | PASS |

## Comprehensive Review

**Verdict: IMPROVED** — Fixed a bug in `extract_impl_patterns` where bold-label bullets (`- **Created:** \`path\``) and prose bullets under `### \`filename\`` sub-sections were being emitted as file names. The fix uses a two-step validate-then-extract strategy (backtick-enclosed path with file-path heuristic guard). Index rebuilt with clean data.

## Notes

- Task 1 added 5 bonus tests (14a–14e) beyond the design spec, causing final test count to be 339 rather than the predicted 315. Reconciled correctly in `CLAUDE.md`.
- Test 24 deviation: uses absent `request.md` to ensure score=0 rather than keyword mismatch — more deterministic and documented.
- The `fix-checkpoint-design-review` workspace uses `### \`filename\`` sub-headers with prose bullets — extractor correctly produces `filesModified: []` (graceful degradation).

## Test Results

339 passed, 0 failed

## Execution Stats

| Phase | Tokens | Duration | Model |
|-------|--------|----------|-------|
| phase-1 | 38,642 | 98.9s | sonnet |
| phase-2 | 62,535 | 207.7s | sonnet |
| phase-3 | 41,866 | 138.0s | sonnet |
| phase-3b | 69,637 | 169.7s | sonnet |
| phase-4 | 39,870 | 102.7s | sonnet |
| phase-4b | 51,965 | 81.2s | sonnet |
| task-1-impl | 49,593 | 293.7s | sonnet |
| task-2-impl | 51,523 | 284.7s | sonnet |
| task-3-impl | 31,111 | 120.7s | sonnet |
| task-4-impl | 46,578 | 364.8s | sonnet |
| task-5-impl | 29,432 | 149.5s | sonnet |
| task-1-review | 43,853 | 125.5s | sonnet |
| phase-7 | 62,671 | 335.5s | sonnet |
| final-verification | 24,124 | 46.3s | sonnet |
| **TOTAL** | **643,400** | **2519.5s** | |

## Improvement Report

_Retrospective on what would have made this work easier._

### Documentation

The `impl-*.md` format is undocumented — there is no spec for which section headings are canonical. This caused the extractor to encounter 4+ heading variants (`## Files Modified`, `## Files Created`, `## Files Created or Modified`, `### \`filename\`` sub-sections). A brief schema note in `CLAUDE.md` or `agents/implementer.md` documenting the expected `impl-*.md` structure would prevent future format drift and make extractors easier to write correctly.

### Code Readability

`build-specs-index.sh` uses inline awk programs for JSON extraction. The existing `extract_impl_outcomes` and new `extract_impl_patterns` functions follow the same structural pattern, which is good. However, awk-based multi-step extraction (especially with backtick-stripping and path-heuristic guards) is difficult to read and maintain. Comments explaining the bullet-validation logic would help future maintainers.

### AI Agent Support (Skills / Rules)

The `impl-*.md` output format is produced by the implementer agent but consumed by `build-specs-index.sh`. These two components evolved independently, causing the extractor to discover format variants only at build time. A convention documented in `.claude/rules/` for implementer output format would create a stable contract between producer and consumer.

# Pipeline Summary

**Request:** [F8-b] Inject past review feedback into architect and task-decomposer prompts
**Feature branch:** `feature/inject-past-review-feedback`
**Pull Request:** #56 (https://github.com/hiromaily/claude-forge/pull/56)
**Date:** 20260325

## Tasks

| # | Title | Verdict |
|---|-------|---------|
| 1 | Create query-specs-index.sh script | PASS |
| 2 | Add query-specs-index.sh tests to test-hooks.sh | PASS_WITH_NOTES |
| 3 | Inject past feedback into SKILL.md Phase 3 and Phase 4 prompts | PASS |
| 4 | Update documentation | PASS |

## Comprehensive Review

Verdict: IMPROVED. Two fixes applied:
- Test 19 assertion strengthened from `<= 3` to `== 3` (exact count enforcement)
- `scripts/README.md` row ordering fixed so `query-specs-index.sh` is adjacent to `build-specs-index.sh`

## Notes

- Task 2 PASS_WITH_NOTES: Test 18 does not assert finding text content (low priority); `/tmp/qsi-stderr` global path follows pre-existing `run_bis` pattern
- All current `.specs/index.json` entries have `reviewFeedback: []` (no REVISE events yet), so the script returns empty stdout in production until REVISE verdicts accumulate — this is by design and gracefully handled
- The scoring algorithm intentionally allows taskType-only matches (score = 2) to surface structural reminders like "update CLAUDE.md subcommand count" that apply to all tasks of the same type

## Test Results

309 passed, 0 failed

## Execution Stats

| Phase | Tokens | Duration | Model |
|-------|--------|----------|-------|
| phase-1 | 56,671 | 104.1s | sonnet |
| phase-2 | 73,130 | 246.4s | sonnet |
| phase-3 | 39,896 | 121.6s | sonnet |
| phase-3b (run 1) | 26,208 | 49.8s | sonnet |
| phase-3b (run 2) | 30,401 | 66.5s | sonnet |
| phase-4 | 27,934 | 68.8s | sonnet |
| phase-4b | 20,866 | 24.0s | sonnet |
| task-1-impl | 45,640 | 269.0s | sonnet |
| task-2-impl | 37,156 | 123.4s | sonnet |
| task-3-impl | 28,943 | 112.9s | sonnet |
| task-4-impl | 31,805 | 126.7s | sonnet |
| task-1-review | 29,774 | 78.5s | sonnet |
| task-2-review | 30,806 | 75.6s | sonnet |
| task-3-review | 30,485 | 62.1s | sonnet |
| phase-7 | 48,861 | 148.4s | sonnet |
| final-verification | 22,388 | 25.1s | sonnet |
| **TOTAL** | **580,964** | **1703.8s** | |

## Improvement Report

_Retrospective on what would have made this work easier._

### Documentation

The `.specs/index.json` schema was clear from `build-specs-index.sh` source, but no standalone documentation of the schema exists (e.g., in `scripts/README.md`). Future scripts querying the index would benefit from a schema reference section.

### Code Readability

The awk frontmatter-strip logic in `build-specs-index.sh` is duplicated verbatim in `query-specs-index.sh`. A shared utility function or documented "pattern to copy" in `scripts/README.md` would reduce this drift risk.

### AI Agent Support (Skills / Rules)

The SKILL.md Phase 3 and Phase 4 blocks have multiple entry points (first run, REVISE re-run, Checkpoint user re-run). The design needed explicit disambiguation of which entry point captures the `past_feedback` variable. A rule in CLAUDE.md noting "variables captured in Phase 3/4 blocks persist across REVISE cycles" would help future contributors.

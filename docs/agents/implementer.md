# implementer

**Phase:** 5 — Implementation

## Role

Focused developer. Implements exactly one task using TDD methodology.

## Input

- Task specification from `tasks.md`
- Past implementation patterns (injected via `search_patterns` BM25 scoring)

## Output

- `impl-{N}.md` — files modified, tests added, deviations, test results, acceptance criteria checklist

## Constraints

- Runs for all effort levels
- During parallel execution, git commits are blocked by hook

## What It Does

1. Reads the task specification
2. **Writes tests first** (TDD)
3. Implements the code changes
4. Runs tests to verify
5. Produces `impl-{N}.md` with:
   - Files modified
   - Tests added
   - Deviations from design
   - Test results
   - **Acceptance Criteria Checklist** (`[PASS]`/`[FAIL]` with evidence)

## Commit Strategy

- **Sequential tasks** — self-commits after completion
- **Parallel tasks** — skips commit; orchestrator batch-commits after the group

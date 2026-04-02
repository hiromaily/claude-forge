# impl-reviewer

**Phase:** 6 — Code Review

## Role

Diff-based code reviewer. Evaluates implementation against acceptance criteria, design alignment, test quality, code quality, and regression status.

## Input

- `impl-{N}.md` — implementation summary from Phase 5

## Output

- `review-{N}.md` — verdict (PASS, PASS_WITH_NOTES, or FAIL)

## Constraints

- Skipped for `investigation` task type
- Up to 2 retries per task on FAIL

## Review Approach

1. Self-executes `git diff main...HEAD -- <changed files>` to obtain the diff
2. Evaluates against acceptance criteria
3. Checks design alignment
4. Assesses test quality and coverage
5. Reviews code quality
6. Checks for regressions

## Verdicts

| Verdict | Meaning | Pipeline Action |
| --- | --- | --- |
| **PASS** | Implementation is correct | Proceed to next task |
| **PASS_WITH_NOTES** | Acceptable with minor observations | Proceed |
| **FAIL** | Issues found | Re-run Phase 5 (up to 2 retries) |

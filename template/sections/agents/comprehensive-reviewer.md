# comprehensive-reviewer

**Phase:** 7 — Comprehensive Review

## Role

Holistic cross-task reviewer. Examines all implementation tasks together for cross-cutting concerns.

## Input

- All `impl-{N}.md` and `review-{N}.md` files
- Full feature diff (`git diff main...HEAD`)

## Output

- `comprehensive-review.md` — verdict (CLEAN or IMPROVED)

## Constraints

- Skipped for effort S (`light` template)

## What It Reviews

- **Naming consistency** across all changed files
- **Code duplication** introduced by separate tasks
- **Interface coherence** between components
- **Completeness** — all design elements implemented
- **Test coverage gaps** — missing edge cases

## Verdicts

| Verdict | Meaning |
| --- | --- |
| **CLEAN** | No issues found |
| **IMPROVED** | Issues found and fixed directly |

Unlike other reviewers, this agent **fixes issues directly** rather than sending back for revision.

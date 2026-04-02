# design-reviewer

**Phase:** 3b — Design Review

## Role

Critical quality gate for designs. Reviews coverage, completeness, consistency, test strategy, contradictions, and scope creep.

## Input

- `design.md` — design document from Phase 3

## Output

- `review-design.md` — verdict (APPROVE, APPROVE_WITH_NOTES, or REVISE) with findings

## Constraints

- Skipped for `bugfix`, `docs`, `refactor` task types

## Verdicts

| Verdict | Meaning | Pipeline Action |
| --- | --- | --- |
| **APPROVE** | Design is ready | Proceed to Checkpoint A |
| **APPROVE_WITH_NOTES** | Minor issues only | Orchestrator applies inline fixes, re-reviews |
| **REVISE** | Critical findings | Re-run Phase 3 (architect) |

Findings are classified as CRITICAL or MINOR. Only CRITICAL findings trigger a REVISE verdict.

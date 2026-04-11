# task-reviewer

**Phase:** 4b — Tasks Review

## Role

Critical quality gate for task lists. Verifies design coverage, deletion tasks, test updates, dependency correctness, parallel safety, and acceptance criteria specificity.

## Input

- `tasks.md` — task list from Phase 4

## Output

- `review-tasks.md` — verdict (APPROVE, APPROVE_WITH_NOTES, or REVISE) with findings

## Constraints

- Skipped for effort S and M (only runs for effort L / `full` template)

## Verdicts

| Verdict | Meaning | Pipeline Action |
| --- | --- | --- |
| **APPROVE** | Tasks are ready | Proceed to Checkpoint B |
| **APPROVE_WITH_NOTES** | Minor issues | Orchestrator applies inline fixes |
| **REVISE** | Critical findings | Re-run Phase 4 (task-decomposer) |

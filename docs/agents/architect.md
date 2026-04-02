# architect

**Phase:** 3 — Design

## Role

Software designer. Synthesizes analysis and investigation into a concrete design document.

## Input

- `investigation.md` — findings from Phase 2 (may be absent for some task types)

## Output

- `design.md` — approach, architecture changes, data model, test strategy, risk mitigation

## Constraints

- Skipped for `docs` task type (orchestrator writes a stub `design.md`)

## What It Does

1. Reads investigation findings
2. Proposes an approach with rationale
3. Documents architectural changes, data model updates
4. Defines a test strategy
5. Identifies risks and mitigation plans
6. Produces `design.md` consumed by the [design-reviewer](/agents/design-reviewer)

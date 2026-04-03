# investigator

**Phase:** 2 — Investigation

## Role

Deep-dive researcher. Builds on the situation analysis to uncover root causes, edge cases, integration points, deletion/rename impacts, and open questions.

## Input

- `analysis.md` — structural index from Phase 1

## Output

- `investigation.md` — findings, root causes, open questions

## Constraints

- **Read-only** — cannot edit or write source files (enforced by hook)
- Runs for all effort levels

## What It Does

1. Reads `analysis.md` to understand the codebase context
2. Goes deeper: examines root causes, edge cases, prior art
3. Identifies integration points and deletion/rename impacts
4. Flags open questions requiring human decisions
5. Produces findings consumed by the [architect](/agents/architect) in Phase 3

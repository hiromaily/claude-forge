# situation-analyst

**Phase:** 1 — Situation Analysis

## Role

Read-only codebase explorer. Maps relevant files, interfaces, types, data flows, and tests for a given task. Produces a structural index — never proposes changes.

## Input

- `request.md` — the user's task description

## Output

- `analysis.md` — structured index of relevant code

## Constraints

- **Read-only** — cannot edit or write source files (enforced by hook)
- Can write to the workspace directory (`.specs/`)
- Runs for all effort levels

## What It Does

1. Reads `request.md` to understand the task
2. Explores the codebase using Glob, Grep, and Read tools
3. Maps relevant files, interfaces, types, and data flows
4. Identifies existing tests related to the change
5. Produces a structured markdown index

The output is consumed by the [investigator](/agents/investigator) in Phase 2.

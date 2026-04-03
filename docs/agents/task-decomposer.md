# task-decomposer

**Phase:** 4 — Task Decomposition

## Role

Breaks an approved design into numbered, dependency-aware implementation tasks.

## Input

- `design.md` — approved design document

## Output

- `tasks.md` — numbered tasks with file assignments, acceptance criteria, parallel/sequential markers

## Constraints

- May be skipped based on effort level

## What It Does

1. Reads the approved design
2. Creates ordered, numbered tasks
3. Assigns files to each task
4. Defines acceptance criteria per task
5. Marks tasks as `[parallel]` or `[sequential]`
6. Identifies dependencies between tasks

# Workflow Instructions (`.specs/instructions.md`)

A per-repository file that lets you deterministically require `mode: human_gate`
on tasks matching specific conditions. Rules are evaluated by Go code at
phase-4 completion; there is no LLM involvement in enforcement.

## File location

`.specs/instructions.md` at the **repository root**. The file is optional —
pipelines run unchanged when it is absent.

## File format

The file is markdown with a single YAML frontmatter block. Only the
frontmatter is read by the validator; the markdown body is for human notes.

```markdown
---
rules:
  - id: main-proto
    when:
      files_match:
        - "backend/**/*.proto"
        - "backend/gen/proto/**"
    require: human_gate
    reason: "main-proto の PR マージ状態を確認してください"

  - id: destructive-migration
    when:
      files_match:
        - "backend/migrations/**/*.sql"
      title_matches: "(?i)drop\\s+(table|column)"
    require: human_gate
    reason: "破壊的マイグレーションのため stakeholder 確認が必要です"
---

# Notes (ignored by validator)

Any markdown after the closing `---` is free-form documentation for humans.
```

## Rule schema

| Field | Type | Required | Description |
|---|---|---|---|
| `id` | string | yes | Unique identifier used in error messages |
| `when.files_match` | list of glob | at least one of files_match / title_matches | Doublestar glob patterns matched against each task's `files:` list |
| `when.title_matches` | Go regex | see above | Regex matched against the task title |
| `require` | string | yes | Must be `human_gate` (MVP) |
| `reason` | string | yes | Shown in the human_gate prompt when the task is reached |

### `when` semantics

- `files_match` is OR across patterns (any glob matching any file is a hit).
- `title_matches` and `files_match` are ANDed when both are present.
- Globs use [`doublestar` syntax](https://github.com/bmatcuk/doublestar) —
  `**` matches any number of path segments.
- Regex uses Go's stdlib `regexp` package. Use `(?i)` for case-insensitive.

## Evaluation flow

1. task-decomposer runs in phase-4 and writes `tasks.md`.
2. `pipeline_report_result` is called with `phase=phase-4`.
3. The Go validator reads `.specs/instructions.md` from the repo root.
4. Each task in `tasks.md` is checked against every rule.
5. If a task matches a rule's `when` but does not have `mode: human_gate`,
   a violation is recorded.
6. On any violation: `review-tasks.md` is written, `next_action_hint` is set
   to `revision_required`, and the existing revision loop re-runs
   task-decomposer. Phase-4 does **not** complete.
7. On zero violations: phase-4 proceeds to phase-4b (AI task-reviewer) as usual.

## Failure modes

| Condition | Behaviour |
|---|---|
| File missing | Zero rules, pipeline unchanged |
| Frontmatter missing | Hard error: "missing YAML frontmatter" |
| Unknown field (typo) | Hard error: "field X not found" |
| `require:` not `human_gate` | Hard error at load time |
| Invalid regex in `title_matches` | Hard error at load time |
| Rule with no `when` conditions | Hard error at load time |
| Glob matches zero tasks | Not an error (rule is just not triggered) |

## Example: claude-forge

In the claude-forge repository, two rules are most useful:

1. **main-proto coordination** — any task touching `.proto` files needs
   the external main-proto PR to be merged first.
2. **destructive migration approval** — any SQL migration containing
   `DROP TABLE` / `DROP COLUMN` needs stakeholder sign-off.

Both are expressible with `files_match` + `title_matches` and do not
require LLM judgement — they are pure pattern matches.

## Not supported (out of scope)

- Natural-language rules interpreted by LLM agents.
- Reading source file content (only task `files:` + title are inspected).
- Rule application in phases other than phase-4.
- `require:` values other than `human_gate`.
- User-level / global instruction files.

## See also

- Design: `docs/superpowers/specs/2026-04-10-workflow-instructions-design.md`
- Agent integration: `agents/task-decomposer.md`
- Engine entry point: `mcp-server/internal/handler/tools/pipeline_report_result.go`

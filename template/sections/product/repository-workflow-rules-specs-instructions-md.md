## Repository workflow rules (`.specs/instructions.md`)

You can commit a `.specs/instructions.md` file to your repository to enforce
deterministic workflow rules at phase-4 completion. When a task matches a
rule but is missing `mode: human_gate`, the engine automatically triggers
REVISE and re-runs task-decomposer with the violation findings.

### Quick example — claude-forge

```markdown
---
rules:
  - id: main-proto
    when:
      files_match:
        - "backend/**/*.proto"
        - "backend/gen/proto/**"
    require: human_gate
    reason: "make sure PR for main-proto repository"

  - id: destructive-migration
    when:
      files_match:
        - "backend/migrations/**/*.sql"
      title_matches: "(?i)drop\\s+(table|column)"
    require: human_gate
    reason: "Stakeholder verification is required for this destructive migration."
---
```

**Scope:** workflow rules only — not coding style, domain knowledge, or
personal preferences. Keep those in `CLAUDE.md` / `AGENTS.md` /
`.kiro/steering/`.

See [`docs/reference/workflow-instructions.md`](../../../docs/reference/workflow-instructions.md)
for the full schema, evaluation flow, and failure modes.

---

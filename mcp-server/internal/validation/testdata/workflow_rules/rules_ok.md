---
rules:
  - id: akupara-proto
    when:
      files_match:
        - "backend/**/*.proto"
        - "backend/gen/proto/**"
    require: human_gate
    reason: "akupara-proto coordination required"

  - id: destructive-migration
    when:
      files_match:
        - "backend/migrations/**/*.sql"
      title_matches: "(?i)drop\\s+(table|column)"
    require: human_gate
    reason: "stakeholder approval required"
---

# Human-readable notes (ignored by the validator)

The YAML frontmatter above is the source of truth.

---
rules:
  - id: both
    when:
      files_match:
        - "backend/migrations/**/*.sql"
      title_matches: "(?i)drop"
    require: human_gate
    reason: "combined"
---

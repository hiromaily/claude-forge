---
rules:
  - id: main-proto
    when:
      files_match:
        - "backend/**/*.proto"
    require: human_gate
    reason: "main-proto coordination required"

  - id: drop-col
    when:
      title_matches: "(?i)drop column"
    require: human_gate
    reason: "stakeholder approval required"
---

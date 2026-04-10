---
rules:
  - id: akupara-proto
    when:
      files_match:
        - "backend/**/*.proto"
    require: human_gate
    reason: "akupara-proto coordination required"

  - id: drop-col
    when:
      title_matches: "(?i)drop column"
    require: human_gate
    reason: "stakeholder approval required"
---

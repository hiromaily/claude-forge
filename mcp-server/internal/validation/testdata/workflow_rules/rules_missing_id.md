---
rules:
  - when:
      files_match: ["**/*.go"]
    require: human_gate
    reason: "no id"
---

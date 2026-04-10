---
rules:
  - id: bad-glob
    when:
      files_match: ["[invalid"]
    require: human_gate
    reason: "invalid glob test"
---

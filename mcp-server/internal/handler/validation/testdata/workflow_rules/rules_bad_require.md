---
rules:
  - id: bad-require
    when:
      files_match: ["**/*.go"]
    require: something_else
    reason: "non-human_gate require"
---

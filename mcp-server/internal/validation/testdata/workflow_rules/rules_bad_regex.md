---
rules:
  - id: bad-regex
    when:
      title_matches: "["     # unterminated character class
    require: human_gate
    reason: "invalid regex test"
---

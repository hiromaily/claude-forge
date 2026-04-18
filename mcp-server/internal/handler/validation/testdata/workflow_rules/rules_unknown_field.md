---
rules:
  - id: typo-rule
    when:
      files_match: ["**/*.go"]
    requires: human_gate    # note: "requires" not "require"
    reason: "typo test"
---

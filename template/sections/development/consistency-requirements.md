## Consistency Requirements

- When adding/changing an agent's input files → update BOTH the agent .md file AND the Agent Roster table in SKILL.md
- When adding a new phase → add its ID to the Go MCP server state package and ensure `pipeline_next_action` recognises it
- When changing hook behavior → verify the hook's exit code semantics (exit 0 = allow, exit 2 = block)
- When changing state.json schema → ensure the Go MCP server (`mcp-server/state/`), all hook scripts, and SKILL.md are aligned

---

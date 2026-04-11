## MCP Local Development

For contributors working on the MCP server source:

```bash
make setup-manual   # build + install + register via claude mcp add
```

After restarting, the `mcp__forge-state__*` tool calls in `SKILL.md` will route to the running server process. Verify with `/mcp` (should show `forge-state` as `Connected`).

---

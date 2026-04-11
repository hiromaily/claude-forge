## MCP Server Registration

The `forge-state` MCP server is the sole state-management interface. All 26 state-management commands are typed MCP tool calls. See [SETUP.md](../../../SETUP.md) for the complete setup guide.

### Auto-registration (plugin install)

When installed as a plugin, the MCP server is registered automatically:

1. `plugin.json` declares `"mcpServers": "./.mcp.json"`
2. `.mcp.json` defines the `forge-state` server (stdio transport, `${CLAUDE_PLUGIN_ROOT}/bin/forge-state-mcp`)
3. The `Setup` hook in `hooks/hooks.json` runs `scripts/setup.sh` to download the pre-built binary from GitHub Releases

No manual `claude mcp add` is needed. See [SETUP.md](../../../SETUP.md) for details.

---

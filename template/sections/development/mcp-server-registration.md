## MCP Server Registration

The `forge-state` MCP server is the sole state-management interface. All 26 state-management commands are typed MCP tool calls. See [SETUP.md](../../../SETUP.md) for the complete setup guide.

### Auto-registration (plugin install)

When installed as a plugin, the MCP server is registered automatically:

1. `plugin.json` declares `"mcpServers": "./.mcp.json"`
2. `.mcp.json` defines the `forge-state` server (stdio transport, `${CLAUDE_PLUGIN_ROOT}/bin/forge-state-mcp`)
3. The `Setup` hook in `hooks/hooks.json` runs `scripts/setup.sh` to download the pre-built binary from GitHub Releases

No manual `claude mcp add` is needed. See [SETUP.md](../../../SETUP.md) for details.

### Local development

For contributors working on the MCP server source:

```bash
make setup-manual   # build + install + register via claude mcp add
```

After restarting, the `mcp__forge-state__*` tool calls in `SKILL.md` will route to the running server process. Verify with `/mcp` (should show `forge-state` as `Connected`).

### No shell fallback

All 26 state-management commands are implemented exclusively in the Go MCP server (`mcp-server/`). There is no shell fallback for `search_patterns`, `validate_input`, or other MCP-only tools — use the MCP tools directly.

### MCP library usage (`github.com/mark3labs/mcp-go`)

Key API surface used in `mcp-server/`:

```go
// Server construction and stdio transport
srv := server.NewMCPServer("forge-state", "1.0.0")
server.ServeStdio(srv)   // package-level function, not a method on srv

// Registering a tool
srv.AddTool(mcp.NewTool("tool_name",
    mcp.WithDescription("..."),
    mcp.WithString("param", mcp.Required(), mcp.Description("...")),
    mcp.WithNumber("num_param", mcp.Description("...")),
), HandlerFunc)

// Reading parameters inside a handler
workspace, err := req.RequireString("workspace")   // returns error if missing
value := req.GetString("key", "default")           // returns default if missing
num := req.GetInt("tokens", 0)
flag := req.GetBool("validated", false)
args := req.GetArguments()                         // map[string]any for complex params

// Returning results
mcp.NewToolResultText("ok")                        // success with text
mcp.NewToolResultError("error message")            // IsError=true response
```

Tool names use underscores (`phase_complete`), not hyphens — MCP protocol requirement. The corresponding MCP tool call name is `mcp__forge-state__phase_complete`.

### Go module setup

The MCP server lives in `mcp-server/` as a **separate Go module** (`go.mod` with its own `module` path). This keeps the Go build hermetic from the rest of the repo (which has no Go code). Run `go mod tidy` from inside `mcp-server/` after adding dependencies. The `make build` / `make install` targets handle this automatically.

### Go package layering

The `mcp-server/internal/` packages form a strict one-way import DAG enforced by `import_cycle_test.go`:

```
tools → orchestrator → state
```

- `state` must never import `orchestrator` or `tools`
- `orchestrator` must never import `tools`
- Shared packages (`history`, `profile`, `prompt`, `validation`, `events`) may import `state` but not `orchestrator` or `tools`

See [`docs/architecture/go-package-layering.md`](../../../docs/architecture/go-package-layering.md) for the full rule set and rationale.

---

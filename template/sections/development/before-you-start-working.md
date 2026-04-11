## Before You Start Working

1. Read `BACKLOG.md` for known issues and improvement candidates
2. Read `docs/architecture/` for design rationale if making structural changes (see `ARCHITECTURE.md` for the index)
3. Check `agents/README.md` for the current agent roster
4. See the Canonical command list above for available MCP tools (all 26 state commands are in the Go MCP server)
5. Read `.claude/rules/git.md` for Git branch and commit conventions
6. Read `.claude/rules/shell-script.md` for Bash scripting conventions before editing any `.sh` file
7. Read [`docs/architecture/go-package-layering.md`](../../../docs/architecture/go-package-layering.md) before editing any Go code in `mcp-server/` — the one-way import DAG (`tools → orchestrator → state`) is enforced by a compile-time test and violations cause import cycles

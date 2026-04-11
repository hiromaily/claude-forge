## Development Constraints

- Do NOT add `isolation: "worktree"` to any Agent tool call — breaks inter-task visibility
- Do NOT duplicate agent instructions in SKILL.md prompts — agents have their own system prompts
- Do NOT store state in memory/conversation — use state.json via the `mcp__forge-state__*` MCP tools
- Do NOT use bare `flock` without a mkdir fallback — macOS lacks `flock` by default. Hook scripts use mkdir-based atomic locking; follow the same pattern if adding new shell scripts that need locking.
- Do NOT reverse the Go package import direction: `tools` → `orchestrator` → `state`. Shared packages (`history`, `profile`, `prompt`, `validation`, `events`) may import `state` but never `orchestrator` or `tools`. Violating this causes an import cycle (caught by `import_cycle_test.go`). See [docs/architecture/go-package-layering.md](../../../docs/architecture/go-package-layering.md) for the full rule set.

---

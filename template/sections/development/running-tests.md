## Running tests

```bash
# Hook script tests (62 tests)
cd claude-forge
bash scripts/test-hooks.sh

# Go MCP server tests
cd claude-forge/mcp-server
go test -race ./...
```

The hook test suite covers all hook scripts (`pre-tool-hook.sh`, `post-agent-hook.sh`, `stop-hook.sh`, `post-bash-hook.sh`, `common.sh`), pre-tool-hook rules (read-only, commit blocking, main/master checkout block), and edge cases like abandoned pipelines and special characters in spec names. The Go test suite covers all 26 state-management commands and MCP-only tools.

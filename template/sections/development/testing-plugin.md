## Testing the Plugin

- MCP state commands: use `cd mcp-server && go test ./state/...` to run the Go unit tests for all 26 state-management commands.
- Hook scripts: pipe sample JSON to stdin and check exit code
  ```bash
  echo '{"tool_name":"Edit","tool_input":{"file_path":"/src/foo.go"}}' | bash scripts/pre-tool-hook.sh
  echo $?  # should be 0 (no active pipeline) or 2 (blocked)
  ```
- **Full hook test suite** (run after any change):
  ```bash
  bash scripts/test-hooks.sh   # run to see current test count (62 tests)
  ```
- **Go MCP server tests** (run after any change to mcp-server/):
  ```bash
  cd mcp-server && go test -race ./...
  ```

---

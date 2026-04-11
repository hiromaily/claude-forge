## Go Module Setup

The MCP server lives in `mcp-server/` as a **separate Go module** (`go.mod` with its own `module` path). This keeps the Go build hermetic from the rest of the repo (which has no Go code). Run `go mod tidy` from inside `mcp-server/` after adding dependencies. The `make build` / `make install` targets handle this automatically.

---

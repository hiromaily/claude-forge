# Environment Variables

## Required

### `FORGE_AGENTS_PATH`

Absolute path to the `agents/` directory. Required for `pipeline_next_action` to resolve agent `.md` files at runtime.

Set automatically by `make setup`. For manual setup, pass via `claude mcp add --env`.

## Optional

### `FORGE_SPECS_DIR`

Override the default `.specs/` directory used by the engine. Useful for testing or running multiple pipelines in different locations.

Default: `.specs/` (relative to the project root)

### `FORGE_EVENTS_PORT`

Port for the SSE events endpoint. When set, the `subscribe_events` MCP tool returns the SSE URL for real-time pipeline event monitoring.

Default: not set (SSE disabled)

## Setup

Environment variables are configured automatically when using `make setup`. For manual setup:

```bash
claude mcp add forge-state \
  --scope user \
  --transport stdio \
  --cmd forge-state-mcp \
  --env FORGE_AGENTS_PATH=/path/to/agents
```

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

Port for the SSE events endpoint **and** the bundled web dashboard. When set, the MCP server starts a local HTTP listener that serves:

- `GET /events` — Server-Sent Events stream consumed by the `subscribe_events` MCP tool
- `GET /` — zero-dependency dashboard (single embedded HTML page) that subscribes to `/events` and renders pipeline phase transitions in real time

Open `http://localhost:<port>/` in a browser after starting any pipeline. The dashboard auto-reconnects on stream drop and supports per-workspace filtering.

When the configured port is already in use, the server automatically retries on a random port in the range **8100–8200**. The actual URL is logged to stderr. The HTTP listener binds to `127.0.0.1` only.

Default: `8099` (set in `.mcp.json` for plugin installs; unset = HTTP listener disabled)

## Setup

Environment variables are configured automatically when using `make setup`. For manual setup:

```bash
claude mcp add forge-state \
  --scope user \
  --transport stdio \
  --cmd forge-state-mcp \
  --env FORGE_AGENTS_PATH=/path/to/agents
```

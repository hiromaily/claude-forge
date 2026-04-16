# Real-time Dashboard

A zero-dependency web UI that shows pipeline phase transitions live in your browser. Useful when you want to leave Claude Code running in one window and watch progress from another, or when you want a teammate to look in without taking over the session.

## What it shows

A reverse-chronological timeline of every event the pipeline emits:

| Event | Color | When it fires |
|---|---|---|
| `phase-start` | purple | A phase has begun executing |
| `phase-complete` | green | A phase finished successfully |
| `phase-fail` | red | A phase exited with an error |
| `checkpoint` | amber | The pipeline is paused waiting for human input |
| `abandon` | red (faded) | The pipeline was abandoned |

Each event row shows the phase name, the spec name, an outcome badge (`completed` / `in_progress` / `failed` / `awaiting_human` / `abandoned`), and a wall-clock timestamp.

A header strip exposes the connection status (`live` / `disconnected`), the cumulative event count for the session, a workspace filter, and a `clear` button. The empty state tells you to run `/forge <task>` to see events appear.

## Enabling it

The dashboard ships inside the `forge-state` MCP binary as an embedded HTML asset. It is **opt-in** — the HTTP listener that serves it does not start unless you set the `FORGE_EVENTS_PORT` environment variable.

To enable it, register the MCP server with `FORGE_EVENTS_PORT` in its env block. Pick any free local port:

```bash
claude mcp add forge-state \
  --scope user \
  --transport stdio \
  --cmd forge-state-mcp \
  --env FORGE_AGENTS_PATH=/absolute/path/to/agents \
  --env FORGE_EVENTS_PORT=9876
```

Then open `http://localhost:9876/` in any browser. The page subscribes to `/events` and starts rendering the moment a pipeline begins emitting events.

If the port cannot be bound (already in use), the MCP server logs the failure to stderr and continues to serve the stdio transport without the dashboard. The pipeline itself is never blocked by dashboard issues.

## Per-workspace filtering

When several pipelines have run in the same `.specs/` directory, the workspace dropdown lets you focus on a single one. The selection is passed to the server as `?workspace=<absolute-path>` and the SSE stream is filtered server-side, so old events from other workspaces are not even sent to the browser.

## Reconnect behaviour

The dashboard uses the browser-native [EventSource](https://developer.mozilla.org/docs/Web/API/EventSource) API. If the MCP server is stopped and restarted, the browser reconnects automatically once the listener is back up. The on-screen timeline is **not persisted** across reloads — closing the tab clears the view, and a fresh page only sees events that arrive after it loaded.

## Security notes

- The HTTP listener binds to all interfaces on the configured port. On a shared machine, prefer a port that is firewalled, or run the MCP server inside a sandboxed account.
- There is no authentication. Treat the dashboard URL as you would any other localhost dev server.
- The dashboard ships as a single embedded file with no external CDN, no fonts, no analytics, and no third-party scripts. The only network call it makes is to `/events` on the same origin.

## What the dashboard does not do (yet)

The first cut is observability-only. It cannot:

- Pause, resume, or branch a running pipeline (no intervention API)
- Show artifact diffs at checkpoints
- Persist past runs (closing the tab loses the view)
- Drive a remote pipeline from a different machine
- Show token / cost burndown against a budget

These are tracked under the Devin-Class Autonomy gap analysis in `BACKLOG.md` (Layer B intervention API, Layer C budget enforcement). The current dashboard is the foundation those features will build on.

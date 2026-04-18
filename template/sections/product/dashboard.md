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

Each event row shows the phase name, the spec name, an outcome badge (`completed` / `in_progress` / `failed` / `awaiting_human` / `abandoned`), and a wall-clock timestamp. Phase-complete events include clickable links to the generated artifact (e.g. `analysis.md`, `design.md`) that open an in-dashboard viewer.

A header strip exposes the connection status (`live` / `disconnected`), the cumulative event count for the session, a workspace filter, and a `clear` button. The empty state tells you to run `/forge <task>` to see events appear.

## Artifact viewer

When a phase completes, the dashboard shows links to the artifacts it produced (e.g. `design.md` after Phase 3, `review-design.md` after Phase 3b). Clicking a link opens a modal overlay that fetches and displays the raw markdown content.

At checkpoint events, the viewer also shows links to all related artifacts for the current checkpoint — for example, checkpoint-a displays `analysis.md`, `investigation.md`, `design.md`, and `review-design.md` so you can review the work before approving.

The artifact endpoint (`GET /api/artifact`) serves only `.md` files from within the workspace directory. Path traversal is blocked and access is restricted to loopback requests.

## Checkpoint interaction

At checkpoint events (`checkpoint-a`, `checkpoint-b`), the dashboard displays an **approve** button and an optional **message textarea**. You can:

- **Approve without a message** — equivalent to typing "proceed" in the Claude Code session.
- **Approve with a message** — the message is injected into the next agent's prompt as a `## Human Feedback` section. This lets you steer the AI's next phase without switching back to the terminal.

The message is written to `checkpoint-message.txt` in the workspace directory. When the next agent is spawned, `enrichPrompt` reads the file, appends its content to the agent prompt, and deletes the file (one-shot delivery). This is handled entirely server-side — no SKILL.md changes or LLM interpretation required.

## Mobile support

The dashboard is responsive and optimised for smartphone-sized viewports (≤ 640px):

- Header controls wrap to full width with larger touch targets
- Event cards switch from a 3-column grid to a 2-row stacked layout
- Artifact viewer and checkpoint form adapt to narrow screens
- Buttons and inputs meet recommended touch target sizes

## Enabling it

The dashboard ships inside the `forge-state` MCP binary as an embedded HTML asset. When installed as a plugin, it is **enabled by default** — `.mcp.json` sets `FORGE_EVENTS_PORT=8099`, so the dashboard starts automatically on every session.

Open `http://localhost:8099/` in any browser. The page subscribes to `/events` and starts rendering the moment a pipeline begins emitting events.

### Port fallback

If port 8099 is already in use (e.g. another Claude Code session), the server automatically retries on a random port in the range **8100–8200** (up to 10 attempts). The actual URL is logged to stderr:

```
forge-state: port 8099 in use, trying fallback range 8100–8200
forge-state: dashboard ready at http://localhost:8142/
```

If all fallback attempts fail, the MCP server continues to serve the stdio transport without the dashboard. The pipeline itself is never blocked by dashboard issues.

### Manual override

To use a different port, register the MCP server with a custom `FORGE_EVENTS_PORT`:

```bash
claude mcp add forge-state \
  --scope user \
  --transport stdio \
  --cmd forge-state-mcp \
  --env FORGE_AGENTS_PATH=/absolute/path/to/agents \
  --env FORGE_EVENTS_PORT=9876
```

## Per-workspace filtering

When several pipelines have run in the same `.specs/` directory, the workspace dropdown lets you focus on a single one. The selection is passed to the server as `?workspace=<absolute-path>` and the SSE stream is filtered server-side, so old events from other workspaces are not even sent to the browser.

## Reconnect behaviour

The dashboard uses the browser-native [EventSource](https://developer.mozilla.org/docs/Web/API/EventSource) API. If the MCP server is stopped and restarted, the browser reconnects automatically once the listener is back up. The on-screen timeline is **not persisted** across reloads — closing the tab clears the view, and a fresh page only sees events that arrive after it loaded.

## Security notes

- The HTTP listener binds to `127.0.0.1` (localhost only). It is not accessible from other machines on the network.
- There is no authentication. Treat the dashboard URL as you would any other localhost dev server.
- The dashboard ships as a single embedded file with no external CDN, no fonts, no analytics, and no third-party scripts. The only network call it makes is to `/events` on the same origin.

## What the dashboard does not do (yet)

- Pause, resume, or branch a running pipeline mid-phase
- Show artifact diffs between revisions
- Render markdown with rich formatting (currently displays raw text)
- Drive a remote pipeline from a different machine
- Show token / cost burndown against a budget

These are tracked under the Devin-Class Autonomy gap analysis in `BACKLOG.md` (Layer B intervention API, Layer C budget enforcement). The current dashboard is the foundation those features will build on.

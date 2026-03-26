# forge-state MCP Server

The `forge-state` MCP server exposes 28 typed tool calls for managing pipeline state and subscribing to real-time phase transition events. The server communicates over stdio (MCP protocol) and optionally listens on an HTTP port for Server-Sent Events (SSE).

## SSE Event Streaming

### Event Schema

Every phase transition event has the following six fields:

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `event` | `string` | Transition verb | `"phase-complete"` |
| `phase` | `string` | Phase identifier | `"phase-3"` |
| `specName` | `string` | Spec name from `state.json` | `"my-feature"` |
| `workspace` | `string` | Absolute path of the workspace | `"/home/user/.specs/20260326-my-feature"` |
| `timestamp` | `string` | RFC3339 UTC timestamp | `"2026-03-26T12:34:56Z"` |
| `outcome` | `string` | Normalized terminal status | `"completed"` |

**`event` values:** `"phase-start"` | `"phase-complete"` | `"phase-fail"` | `"checkpoint"` | `"abandon"`

**`outcome` values:** `"in_progress"` | `"completed"` | `"failed"` | `"awaiting_human"` | `"abandoned"`

Example event payload:

```json
{
  "event": "phase-complete",
  "phase": "phase-5",
  "specName": "sse-event-streaming",
  "workspace": "/home/user/project/.specs/20260326-sse-event-streaming",
  "timestamp": "2026-03-26T12:34:56Z",
  "outcome": "completed"
}
```

### `FORGE_EVENTS_PORT` — enabling the SSE server

Set this environment variable to a port number to start the HTTP SSE endpoint at `GET /events`. When absent or empty, no port is bound and SSE is silently disabled.

```bash
FORGE_EVENTS_PORT=9876 forge-state-mcp
```

Once running, subscribe to the event stream with `curl`:

```bash
curl -N http://localhost:9876/events
```

Each event arrives as an SSE `data:` line:

```
data: {"event":"phase-complete","phase":"phase-3","specName":"my-feature","workspace":"/path/to/workspace","timestamp":"2026-03-26T12:34:56Z","outcome":"completed"}

data: {"event":"phase-start","phase":"phase-4","specName":"my-feature","workspace":"/path/to/workspace","timestamp":"2026-03-26T12:35:00Z","outcome":"in_progress"}
```

#### Filtering by workspace

Append a `?workspace=` query parameter to receive only events from a specific pipeline:

```bash
curl -N "http://localhost:9876/events?workspace=/path/to/workspace"
```

#### Client disconnect

The SSE handler detects client disconnects via context cancellation and cleans up the subscriber automatically. No manual cleanup is required.

### `FORGE_SLACK_WEBHOOK_URL` — Slack notifications

Set this environment variable to a Slack Incoming Webhook URL to receive Slack notifications on significant pipeline transitions.

```bash
FORGE_SLACK_WEBHOOK_URL=https://hooks.slack.com/services/T.../B.../... forge-state-mcp
```

Notifications are sent for:
- `phase-complete` — a phase finished successfully
- `phase-fail` — a phase failed
- `abandon` — the pipeline was abandoned

Notification failures (network errors, non-2xx responses) are logged to stderr and never propagate to the caller. All notifications are sent asynchronously and do not block state mutations.

## `subscribe_events` MCP Tool

The `subscribe_events` tool is a discovery tool that returns the SSE endpoint URL so in-process consumers know where to connect.

**Tool name:** `mcp__forge-state__subscribe_events`

**Parameters:** none

**Response when `FORGE_EVENTS_PORT` is set:**

```json
{
  "endpoint": "http://localhost:9876/events"
}
```

**Response when `FORGE_EVENTS_PORT` is not set:**

```
SSE event streaming is not configured. Set FORGE_EVENTS_PORT to enable it.
```

The tool does not establish a subscription itself — it only returns the endpoint URL for the caller to connect to independently.

## Building and Installing

```bash
make install
```

This compiles the binary and copies `forge-state-mcp` to `$(GOBIN)` or `~/.local/bin`.

## Running Tests

```bash
cd mcp-server
go test ./...
```

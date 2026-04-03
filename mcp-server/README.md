# forge-state MCP Server

The `forge-state` MCP server exposes 28 typed tool calls for managing pipeline state and subscribing to real-time phase transition events. The server communicates over stdio (MCP protocol) and optionally listens on an HTTP port for Server-Sent Events (SSE).

## Pipeline Phases

The pipeline progresses through 18 phases in a fixed canonical order. The Engine (`orchestrator/engine.go`) dispatches the next action based on the current phase in `state.json`. Phase IDs are defined in `orchestrator/phases.go` and `state/state.go`.

| # | Phase ID | Description | Actor | Output |
|---|----------|-------------|-------|--------|
| 0 | `setup` | Initialize workspace, create `state.json` and `request.md` | Orchestrator | `state.json`, `request.md` |
| 1 | `phase-1` | Situation analysis — read-only codebase survey | situation-analyst agent | `analysis.md` |
| 2 | `phase-2` | Deep-dive investigation — root causes, edge cases, risks | investigator agent | `investigation.md` |
| 3 | `phase-3` | Architecture and design | architect agent | `design.md` |
| 4 | `phase-3b` | AI design review (APPROVE / APPROVE_WITH_NOTES / REVISE) | design-reviewer agent | `review-design.md` |
| 5 | `checkpoint-a` | Human reviews and approves/rejects the design | Human | approval or feedback |
| 6 | `phase-4` | Task decomposition from design | task-decomposer agent | `tasks.md` |
| 7 | `phase-4b` | AI task review (APPROVE / APPROVE_WITH_NOTES / REVISE) | task-reviewer agent | `review-tasks.md` |
| 8 | `checkpoint-b` | Human reviews and approves/rejects the task plan | Human | approval or feedback |
| 9 | `phase-5` | Implementation — code changes per task (sequential or parallel) | implementer agent | code files, `impl-{N}.md` |
| 10 | `phase-6` | Code review per task (PASS / PASS_WITH_NOTES / FAIL) | impl-reviewer agent | `review-{N}.md` |
| 11 | `phase-7` | Comprehensive cross-cutting review of all changes | comprehensive-reviewer agent | `comprehensive-review.md` |
| 12 | `final-verification` | Full typecheck + test suite run; fix failures if possible | verifier agent | _(fixes applied directly)_ |
| 13 | `pr-creation` | `git push` + `gh pr create` — PR number is now known | Orchestrator | PR on GitHub |
| 14 | `final-summary` | Generate `summary.md` with PR number, execution stats, improvement report | Orchestrator | `summary.md` |
| 15 | `final-commit` | Amend last commit to include `summary.md` + `state.json`, then force-push | Orchestrator | PR branch updated |
| 16 | `post-to-source` | Post summary comment to GitHub Issue or Jira Issue (if applicable) | Orchestrator | issue comment |
| 17 | `completed` | Terminal state — pipeline finished | System | — |

### Effort-based phase skipping

Not all phases run on every pipeline. The effort level (S / M / L) determines which phases are skipped:

| Effort | Skipped phases |
|--------|---------------|
| **S** (light) | `phase-4b`, `checkpoint-b`, `phase-7` |
| **M** (standard) | `phase-4b`, `checkpoint-b` |
| **L** (full) | _(none)_ |

Skipped phases are recorded in `state.json.skippedPhases` during workspace setup. The Engine's skip gate (`Decision 14`) returns a `skip:` done action for any phase in this list, and the orchestrator calls `phase_complete` to advance past it.

---

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

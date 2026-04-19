# Remote Dashboard Control

Status: draft v3 (2026-04-19)

## Overview

This document explores the architecture required to allow the forge Dashboard to
be accessed from external devices (smartphone, tablet, remote machine) and to
have checkpoint approvals automatically resume the Claude pipeline — without
requiring any terminal input.

Security hardening is deferred to a later phase. The design here is for the
initial development / dogfooding phase only.

## Problem Statement

### Current flow

```text
Claude (terminal)          Dashboard (browser)         state.json
       │                          │                         │
       │── pipeline_next_action ──▶ engine: checkpoint      │
       │                          │                         │
       │── checkpoint() ──────────────────────────────────▶ │ status=awaiting_human
       │                          │                         │
       │  [AskUserQuestion]       │── approve button ──────▶│ PhaseComplete()
       │  blocking on             │                         │ status=pending (next phase)
       │  terminal input          │   "approved" ✓          │
       │                          │                         │
       │  [nothing happens]       │                         │
       │  still blocked           │                         │
```

When the user clicks "approve" in the Dashboard, `approveCheckpointHandler` calls
`sm.PhaseComplete()`, advancing `state.json` to the next phase. However:

1. **No event is published to EventBus** — `approveCheckpointHandler` does not
   have access to `bus`, so the EventBus never learns about the approval.
2. **Claude is blocked on `AskUserQuestion`** waiting for terminal input. Even
   if an event were published, nothing is listening.

Additionally, the Dashboard server binds to `127.0.0.1` only, so access from
external devices is not possible.

### What is already implemented

- **`checkpoint-message.txt` injection**: when the user approves a checkpoint
  via the Dashboard with an optional message, the message is written to
  `checkpoint-message.txt` in the workspace. `enrichPrompt` in
  `pipeline_next_action.go` reads and removes the file, injecting the message
  into the next agent's prompt automatically.
- **`currentPhaseStatus = "awaiting_human"`** is set directly by
  `pipeline_next_action` when it returns an `ActionCheckpoint`, eliminating the
  window between the checkpoint action and the `checkpoint()` MCP call.
- **EventBus + SSE**: the event bus is running and the dashboard SSE stream
  works; the missing link is that `approveCheckpointHandler` does not publish
  to it.

### Goals

1. Dashboard approval automatically resumes the pipeline — no terminal action
   required.
2. Dashboard accessible from external devices on the same network (smartphone,
   tablet) via a `0.0.0.0` bind mode.
3. Foundation for multi-pipeline monitoring and task submission (Phase 2).

---

## Phase 1: EventBus Long-Poll + Local Network Access

### Core mechanism: EventBus long-poll in `pipeline_next_action`

Instead of SKILL.md blocking on `AskUserQuestion` at checkpoints, the MCP tool
itself waits for a state change. When `pipeline_next_action` is called while
`currentPhaseStatus == "awaiting_human"` and no `user_response` is provided:

1. The handler subscribes to the in-process `EventBus`.
2. Waits up to **15 seconds** (safely within MCP tool-call timeout) for a
   `phase-complete` event on the current checkpoint phase.
3. **Event arrives** (Dashboard called `PhaseComplete` → EventBus published):
   reload state → fall through to `eng.NextAction` → `sm.PhaseStart` → return
   the next real action (e.g. `spawn_agent` for phase-4). Claude proceeds with
   no terminal interaction.
4. **Timeout elapses** with no event: let `eng.NextAction` run (returns
   the same checkpoint action), and set `StillWaiting: true` in the response.
   SKILL.md calls `pipeline_next_action()` again immediately.

```text
Claude (no AskUserQuestion)        MCP server                  Dashboard
       │                                │                           │
       │── pipeline_next_action() ─────▶│                           │
       │                                │ subscribe EventBus        │
       │   [MCP call blocked ~15s]      │ wait up to 15s            │
       │                                │                           │
       │                                │◀── PhaseComplete() ───────│ user clicks approve
       │                                │ event: phase-complete      │
       │                                │ reload state              │
       │                                │ eng.NextAction            │
       │◀─ {type: "spawn_agent"} ───────│ PhaseStart               │
       │                                │                           │
       │  continue pipeline             │                           │
```

On timeout, `pipeline_next_action` returns `{type: "checkpoint", still_waiting: true}`.
SKILL.md calls `pipeline_next_action` again immediately (no sleep). The 15s
server-side delay provides natural pacing.

### Terminal user path

The terminal path is unaffected in correctness, with a max 15s delay:

1. Claude is blocked in the `pipeline_next_action` long-poll.
2. User types "proceed" in terminal → Claude Code queues the message.
3. After 15s (or earlier if Dashboard fires), the tool returns — either with the
   next action (Dashboard approved) or `still_waiting: true` (timeout).
4. If `still_waiting`, Claude processes the queued "proceed" → calls
   `pipeline_next_action(user_response="proceed")`.
5. P8 block calls `sm.PhaseComplete` + the engine returns the next action.

### Missing link: bus not wired into `approveCheckpointHandler`

`approveCheckpointHandler` currently receives only `*state.StateManager`. After
calling `sm.PhaseComplete()` it must also publish a `phase-complete` event so
the long-poll wakes up:

```go
// after sm.PhaseComplete succeeds:
bus.Publish(events.Event{
    Event:     "phase-complete",
    Phase:     req.Phase,
    Workspace: req.Workspace,
    Outcome:   "completed",
    Timestamp: time.Now().UTC().Format(time.RFC3339),
})
```

`server.go` must pass `bus` to `approveCheckpointHandler`.

### SKILL.md change (minimal)

The only SKILL.md change is the checkpoint action handler:

```text
- `checkpoint`:
  1. Call checkpoint(workspace, phase=action.name) to register the pause.
  2. Present action.present_to_user to the user and mention that the Dashboard
     can be used to approve without terminal input.
  3. Immediately call pipeline_next_action(workspace) (no user_response, no
     previous_*). If still_waiting: true, call again. Repeat until a
     non-checkpoint action is returned.
  4. If the user types in the terminal (proceed/revise/abandon): their message
     is queued during the 15s long-poll. On the next pipeline_next_action call,
     pass user_response=<message> instead of looping.
```

### Changes summary

| Component | Change | Scope |
| --- | --- | --- |
| `dashboard/server.go` | Pass `bus` to `approveCheckpointHandler` | Trivial |
| `dashboard/intervention.go` | Add `bus` param; publish `phase-complete` after `PhaseComplete` | Small |
| `handler/tools/pipeline_next_action.go` | Add long-poll path when `awaiting_human` + no `user_response` | Medium |
| `skills/forge/SKILL.md` | Replace `AskUserQuestion` with immediate re-call loop on `still_waiting` | Small |
| `nextActionResponse` | Add `StillWaiting bool` field | Trivial |

### Remote access via local network (`FORGE_DASHBOARD_BIND_ALL`)

No auth or ngrok is required for the initial development phase. Adding
`FORGE_DASHBOARD_BIND_ALL=1` makes the dashboard bind to `0.0.0.0` instead of
`127.0.0.1` and disables the `isLocalRequest` origin check:

```text
smartphone browser (same WiFi)
       │
       ▼ HTTP
  192.168.x.x:8099  ←  forge Dashboard server (0.0.0.0:8099)
```

The implementation:
- `server.go`: read `FORGE_DASHBOARD_BIND_ALL` at startup; use `0.0.0.0` when
  set, else keep `127.0.0.1`.
- `intervention.go`: skip `isLocalRequest` check when `FORGE_DASHBOARD_BIND_ALL`
  is set. Pass a `publicMode bool` flag from `server.go` to the handlers.

Starting the dashboard in public mode:

```bash
FORGE_EVENTS_PORT=8099 FORGE_DASHBOARD_BIND_ALL=1 forge-state-mcp
```

Then open `http://<host-ip>:8099` from any device on the same network.

**Security note:** This is intentionally insecure and meant for local development
only. Anyone on the same network can approve checkpoints and abandon pipelines.
Bearer-token auth and ngrok support are deferred to a future phase.

---

## Phase 2: Task Submission from Web UI

### 3.1 Executive Summary

Phase 2 enables submitting forge pipeline tasks from the Dashboard Web UI. A task
runner embedded in the MCP server process starts an Anthropic Agent SDK session for
each submitted task. The Agent SDK session runs the forge pipeline in a full multi-turn
conversation with complete tool-use support. Because the session writes to the same
`.specs/` workspace tree and publishes to the same in-process `EventBus`, the
Dashboard's SSE stream and checkpoint approval flow work identically for both
interactive (Claude Code) and SDK-run pipelines — the control plane is unified.

`claude --print` (`-p`) is stateless and unsuitable for multi-turn pipeline
conversations. The correct tool is the **Anthropic Agent SDK**, which supports
multi-turn conversations with full tool-use programmatically.

### 3.2 HTTP API

```
POST /api/task/submit
Authorization: Bearer <token>   (required when FORGE_DASHBOARD_TOKEN is set)
Content-Type: application/json

{
  "input":  "https://github.com/org/repo/issues/42",
  "effort": "M",
  "flags":  ["--auto"]
}
```

Response (202 Accepted):
```json
{
  "task_id": "20260419-42-fix-login-timeout",
  "status":  "queued"
}
```

```
GET /api/tasks
Authorization: Bearer <token>   (required when FORGE_DASHBOARD_TOKEN is set)
```

Response (200 OK):
```json
{
  "tasks": [
    {
      "task_id":    "20260419-42-fix-login-timeout",
      "input":      "https://github.com/org/repo/issues/42",
      "status":     "running",
      "workspace":  ".specs/20260419-42-fix-login-timeout",
      "queued_at":  "2026-04-19T10:30:00Z",
      "started_at": "2026-04-19T10:30:05Z"
    }
  ]
}
```

**Validation**: the `input` field is validated using the existing
`handler/validation.ValidateInput` function (same path as `pipeline_init`). `effort`
must be `S`, `M`, or `L` (or absent, in which case forge selects automatically).
`flags` entries are allowlisted to `["--auto"]` only in the initial implementation.

**Decoder**: a new `taskSubmitRequest` struct with a dedicated `json.NewDecoder`
(not `decodeRequest` from `intervention.go` — that one uses `DisallowUnknownFields`
and has a different body shape). The new decoder follows the same
`http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)` pattern.

### 3.3 Go Package Layout

```text
mcp-server/internal/taskrunner/
  runner.go          — Runner struct: goroutine pool, task queue, lifecycle
  task.go            — Task struct: ID, input, effort, flags, status, timestamps
  queue.go           — in-memory queue + tasks.json persistence
mcp-server/internal/dashboard/
  task_submit.go     — POST /api/task/submit handler
  task_list.go       — GET /api/tasks handler
```

**Dependency direction** (must comply with import DAG `tools → orchestrator → state`):

```text
dashboard/task_submit.go → taskrunner (enqueue only)
taskrunner/runner.go     → engine/state (ReadState for outcome, not PhaseComplete)
taskrunner/queue.go      → engine/state (ReadState only, for resume scan)
```

`taskrunner` must NOT import `handler/tools` or `engine/orchestrator`.
`taskrunner` only reads `state.json` to determine task outcomes (same pattern as
`queue_report` in `queue-design.md`).

### 3.4 `StartOptions` Extension

`StartOptions` (defined in `mcp-server/internal/dashboard/server.go`) gains a
`TaskRunner` field:

```go
type StartOptions struct {
    PhaseLabels map[string]string
    TaskRunner  *taskrunner.Runner   // nil → task submission endpoints return 501
}
```

The `Start` function registers `POST /api/task/submit` and `GET /api/tasks` when
`opts.TaskRunner != nil`. When nil, the routes are registered but return
`501 Not Implemented` (avoids nil-dereference panics if the runner fails to start).

This extends the existing `*StartOptions` pattern without changing `Start`'s signature.

### 3.5 Agent SDK Runtime Options

The Phase 2 implementation pipeline must choose the runtime based on Go SDK
availability at that time. Three options are documented here in preference order:

1. **Go Anthropic SDK** (preferred): keeps the Agent session in-process with the MCP
   server, avoids cross-language dependencies. Use if a Go Anthropic SDK with multi-turn
   conversation and tool-use support is available at implementation time.
2. **Node.js subprocess**: a Node.js process using the `@anthropic-ai/sdk` package,
   started by the `taskrunner.Runner`. The subprocess receives the task via stdin JSON
   and writes progress events to stdout. Adds a Node.js runtime dependency.
3. **Python subprocess**: same pattern as option 2 using the `anthropic` Python package.
   Fallback if neither Go nor Node.js SDKs are suitable. If a subprocess is used,
   annotate the `os/exec` call with `//nolint:gosec // G204` (`.golangci.yml` already
   suppresses G204).

The HTTP API contract (`POST /api/task/submit`, `GET /api/tasks`, `tasks.json`
persistence) is runtime-independent. Only the internal Agent session launch mechanism
changes based on SDK choice.

### 3.6 `artifactHandler` Public Mode Fix (Phase 2 prerequisite)

`mcp-server/internal/dashboard/artifact.go` currently ignores `publicMode` and calls
`isLocalRequest(r)` directly (line 29), blocking external devices from viewing
artifacts even in public mode.

Required fix (not implemented in this documentation pipeline):

```go
// Current (incorrect in public mode):
if !isLocalRequest(r) {

// Fixed:
if !publicMode && !isLocalRequest(r) {
```

`artifactHandler` must accept a `publicMode bool` parameter, added via closure
(same constructor pattern as `approveCheckpointHandler` and `abandonHandler`).
`server.go` registers it as `artifactHandler(public)`.

This is a Phase 2 prerequisite: external devices need to fetch artifact `.md` files
(design.md, tasks.md) to make the remote dashboard useful. **This Go change is not
implemented in this documentation pipeline** and must be carried out in the Phase 2
Go implementation pipeline.

### 3.7 Task Runner Lifecycle

**Startup**: `Runner.Start(ctx context.Context)` launches a fixed-size goroutine
pool (default: 1 worker). The pool reads from an in-memory channel fed by `Enqueue`.

**Crash recovery**: on `Runner.Start()`, the runner scans `.specs/tasks.json` for
tasks with `status: queued` or `status: in_progress` and re-enqueues them. The
runner only re-enqueues tasks that have `source: "dashboard"` — this discriminator
field prevents the runner from accidentally re-enqueuing pipelines that were started
by an interactive Claude Code session.

**Agent session**: each task starts an Agent SDK session. The session runs the forge
pipeline interactively (multi-turn, full tool-use across the complete pipeline
lifecycle). The session has access to `FORGE_EVENTS_PORT` and writes to `.specs/`
on the same machine, so its pipeline publishes events to the same in-process EventBus
and the same dashboard SSE stream. The exact SDK invocation mechanism (Go SDK, Node.js
subprocess, or Python subprocess) is deferred to the Phase 2 Go implementation pipeline
based on SDK availability at that time (see §3.5).

**Workspace slug**: pre-generated from the input URL using slug-derivation logic
(source ID extracted from the URL: issue number for GitHub, lowercase key for Jira).
The slug is passed to the Agent SDK session so it can pass `workspace_slug` in
`user_confirmation` to `pipeline_init_with_context`. This uses the existing
`applyWorkspaceSlug` path in `pipeline_init_with_context.go` with no changes to forge.

**Outcome determination**: after the session ends, the runner reads the workspace
`state.json` directly (no MCP tool calls) to determine the outcome. Same deterministic
rule as `queue_report`: `currentPhase == "completed"` → success; anything else → failed.

**Persistence**: `tasks.json` in `.specs/` holds the task queue state. Written
atomically after each status transition (write to temp file + `os.Rename`). The
`source: "dashboard"` field is always written by the HTTP submission handler so
recovery scans can discriminate dashboard tasks from interactive pipelines. Format:

```json
{
  "tasks": [
    {
      "task_id":     "20260419-42-fix-login-timeout",
      "input":       "https://github.com/org/repo/issues/42",
      "effort":      "M",
      "flags":       ["--auto"],
      "source":      "dashboard",
      "status":      "completed",
      "workspace":   ".specs/20260419-42-fix-login-timeout",
      "slug":        "42",
      "queued_at":   "2026-04-19T10:30:00Z",
      "started_at":  "2026-04-19T10:30:05Z",
      "finished_at": "2026-04-19T10:45:12Z"
    }
  ]
}
```

### 3.8 Authentication

**Environment variable**: `FORGE_DASHBOARD_TOKEN`. When set (non-empty), all
mutation endpoints (`POST /api/task/submit`, `POST /api/checkpoint/approve`,
`POST /api/pipeline/abandon`) require `Authorization: Bearer <token>`. Token
comparison uses `crypto/subtle.ConstantTimeCompare` to avoid timing attacks.

When `FORGE_DASHBOARD_TOKEN` is not set, behavior is unchanged from Phase 1
(`publicMode` governs access). Token enforcement is explicitly disabled when
`FORGE_DASHBOARD_TOKEN` is empty, making the opt-in ergonomic for local development.

**Backward-compatibility note for the Phase 2 implementation pipeline**: Adding
`FORGE_DASHBOARD_TOKEN` enforcement to the existing Phase 1 endpoints
(`POST /api/checkpoint/approve`, `POST /api/pipeline/abandon`) is a breaking change
for any existing deployment that sets `FORGE_DASHBOARD_BIND_ALL=1` without setting
`FORGE_DASHBOARD_TOKEN`. The implementation must make token enforcement opt-in —
only active when `FORGE_DASHBOARD_TOKEN` is non-empty. Never enforce the token
unconditionally.

### 3.9 Dashboard UI Changes

The `dashboard.html` (currently 777 lines, zero-dependency) adds:

1. **Task submission form** (visible only when `publicMode` is active):
   - The mechanism for detecting `publicMode` on the client side — e.g. a
     `GET /api/server-info` endpoint or a value embedded in the HTML at serve time —
     is left to the Phase 2 Go implementation pipeline. The intent is to show the
     form only when `publicMode=true`; the detection mechanism requires Go code which
     is out of scope for this documentation pipeline.
   - Text input for `input` (URL or free text)
   - Dropdown for `effort` (S / M / L / Auto)
   - Submit button → `POST /api/task/submit`
   - Shows returned `task_id` and status

2. **Task list panel**:
   - Polls `GET /api/tasks` every 10 seconds
   - Columns: Task ID, Input, Status, Started At
   - Clicking a row filters the phase timeline to that workspace's events

3. **Multi-workspace SSE filtering**:
   - The existing timeline view is filtered by `workspace` from SSE event data
   - When a task is selected from the task list, only events matching that
     workspace are shown in the timeline

### 3.10 Comparison Table: forge-queue vs Phase 2

| Dimension | forge-queue | Phase 2 Dashboard |
|---|---|---|
| Submission | `queue.yaml` file, `/forge-queue` skill | `POST /api/task/submit` HTTP |
| Parallelism | Sequential (1 task at a time) | Sequential (1 worker, expandable) |
| Persistence | `queue.yaml` | `.specs/tasks.json` |
| Input types | Issue URLs only (`--auto` forced) | Issue URLs + free text + flags |
| Session runtime | Separate `claude -p` per task (stateless) | Agent SDK per task (multi-turn) |
| Why `claude -p` / SDK | Context isolation per batch task | Multi-turn pipeline requires live context |
| Workspace slug | Pre-generated by `queue_next` | Pre-generated by `taskrunner` |
| Result recording | `queue_report` MCP tool | `runner.go` reads `state.json` directly |
| Monitoring | CLI only | Dashboard SSE + task list |

### Test Strategy

**`mcp-server/internal/taskrunner/` unit tests**:
- `queue_test.go`: enqueue/dequeue round-trip, `tasks.json` atomic write, duplicate
  task_id rejection, crash-recovery scan (only `source: "dashboard"` tasks re-enqueued)
- `runner_test.go`: worker goroutine picks up tasks, Agent SDK session lifecycle,
  outcome determination from `state.json` (`completed` → success, anything else → failed)
- Slug generation: GitHub URL → issue number, Jira URL → lowercase key

**`mcp-server/internal/dashboard/` handler tests**:
- `task_submit_test.go`: validates `input`, rejects unknown effort values, returns 202
  with `task_id`, rejects requests without token when `FORGE_DASHBOARD_TOKEN` is set,
  returns 501 when no `TaskRunner` is wired
- `task_list_test.go`: returns current task list from runner, handles empty list
- `artifact_test.go` (extend existing): `artifactHandler` with `publicMode=true`
  returns artifact without loopback check

**Integration** (manual):
- Submit a GitHub issue URL via `POST /api/task/submit`, verify SSE events appear for
  the spawned pipeline workspace, verify `tasks.json` updated after completion

---

## Implementation Roadmap

### Phase 1 (implement now)

1. **`dashboard/server.go`**: read `FORGE_DASHBOARD_BIND_ALL` env var; when set,
   bind to `0.0.0.0` and pass `publicMode=true` to handlers.

2. **`dashboard/intervention.go`**: add `bus *events.EventBus` and
   `publicMode bool` to handler constructors.
   - When `publicMode`: skip `isLocalRequest` check.
   - After `sm.PhaseComplete()` succeeds: publish `phase-complete` event to `bus`.

3. **`handler/tools/pipeline_next_action.go`**: add long-poll block between P0
   and `eng.NextAction`. When `currentPhaseStatus == "awaiting_human"` and no
   `user_response`:
   - Subscribe to `bus`, select on: EventBus channel (`phase-complete` for
     current phase), 15s timer, `ctx.Done()`.
   - On `phase-complete`: reload state (`sm2.LoadFromFile`), fall through to
     `eng.NextAction`.
   - On timeout/ctx: fall through to `eng.NextAction` (returns same checkpoint
     action); set `StillWaiting: true` on `nextActionResponse`.

4. **`nextActionResponse`**: add `StillWaiting bool \`json:"still_waiting,omitempty"\``.

5. **`skills/forge/SKILL.md`**: checkpoint action handler — remove
   `AskUserQuestion`, add immediate re-call loop on `still_waiting: true`.

### Phase 2 (future)

Implement the Agent SDK-based task runner and Dashboard submission form as
specified in §3.1–§3.10 of this document. Key deliverables:

1. **`mcp-server/internal/taskrunner/`**: new package with `Runner`, `Task`, and
   queue persistence (`tasks.json`). Choose Agent SDK runtime based on Go SDK
   availability at implementation time (Go SDK preferred; Node.js or Python
   subprocess as fallbacks — see §3.5).

2. **`dashboard/task_submit.go` + `dashboard/task_list.go`**: register
   `POST /api/task/submit` and `GET /api/tasks` when `opts.TaskRunner != nil`
   (see §3.2 and §3.4).

3. **`dashboard/artifact.go`**: apply the `publicMode bool` fix so external
   devices can fetch artifact `.md` files (prerequisite — see §3.6).

4. **`dashboard/server.go`**: wire `TaskRunner` into `StartOptions`; add
   `FORGE_DASHBOARD_TOKEN` bearer-token middleware for mutation endpoints (see §3.8).

5. **`dashboard.html`**: add task submission form and task list panel (see §3.9).

The HTTP API contract (`POST /api/task/submit`, `GET /api/tasks`, `tasks.json`
persistence format) is fixed by this document and must not change without updating
this research document first.

# Remote Dashboard Control

Status: draft v2 (2026-04-19)

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

## Phase 2: Task Submission from Web UI (research)

Submitting tasks from the Web UI requires a programmatic Claude session.
`claude --print` (`-p`) is stateless and unsuitable for multi-turn pipeline
conversations. The correct tool is the **Anthropic Agent SDK**, which supports
multi-turn conversations with full tool-use programmatically.

### Architecture

```text
Web UI  ──  POST /api/task/submit  ──▶  Dashboard server
                                              │
                                        enqueue task
                                              │
                                        task runner
                                        (Agent SDK)
                                              │
                                        multi-turn agent session
                                        runs forge pipeline
                                              │
                                        .specs/ workspace
                                        state.json  ←──────  same EventBus
                                                             same Dashboard SSE
```

Because the Agent SDK-run pipeline writes to the same `state.json` and publishes
to the same `EventBus`, the Dashboard's SSE stream and checkpoint approval
mechanism work identically for both interactive (Claude Code) and SDK-run
pipelines. The control plane is unified.

### Task submission endpoint

```json
POST /api/task/submit
{
  "input":  "https://github.com/org/repo/issues/42",
  "effort": "M",
  "flags":  ["--auto"]
}
```

Returns a task ID. The task appears in the Dashboard as a new pipeline and can
be monitored and approved through the same SSE + checkpoint flow.

### Integration with forge-queue

The `forge-queue` design (see `queue-design.md`) already covers sequential
batch execution via `claude -p` subprocesses. Phase 2 extends that model to
support SDK-based execution with Dashboard-driven task submission:

- `forge-queue` → sequential batch via `claude -p`, no Dashboard submission
- Phase 2 → on-demand submission from Dashboard, SDK-based execution

These can coexist. The Dashboard task submission endpoint simply enqueues into
the same runner.

### What Phase 2 requires

- Anthropic Agent SDK integration (Python or TypeScript runner, or Go SDK if
  available)
- A persistent task runner service (separate process or goroutine pool)
- Task queue persistence (simple file-based or in-memory)
- Dashboard: task submission form + task list view
- Authentication hardening (token-based, since the endpoint accepts task inputs)

Phase 2 is a separate initiative. No implementation plan is provided here.

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

Design and implement the Agent SDK-based task runner and Dashboard submission
form as a separate initiative.

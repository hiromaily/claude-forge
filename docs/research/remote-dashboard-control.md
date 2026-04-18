# Remote Dashboard Control

Status: draft v1 (2026-04-18)

## Overview

This document explores the architecture required to allow the forge Dashboard to
be accessed from external devices (smartphone, tablet, remote machine) and to
have checkpoint approvals automatically resume the Claude pipeline — without
requiring any terminal input.

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

When the user clicks "approve" in the Dashboard, `sm.PhaseComplete()` advances
`state.json` to the next phase. However, Claude is blocked on `AskUserQuestion`
waiting for terminal input. The approval is written to state, but Claude never
wakes up.

Additionally, the Dashboard server binds to `127.0.0.1` only, so access from
external devices is not possible without changes.

### Goals

1. Dashboard approval automatically resumes the pipeline — no terminal action required.
2. Dashboard accessible from external devices (smartphone, remote machine) via ngrok.
3. Foundation for multi-pipeline monitoring and task submission (Phase 2).

---

## Phase 1: EventBus Long-Poll + Remote Access

### Core mechanism: EventBus long-poll in `pipeline_next_action`

Instead of SKILL.md blocking on `AskUserQuestion` at checkpoints, the MCP tool
itself waits for a state change. When `pipeline_next_action` is called while
`currentPhaseStatus == "awaiting_human"` and no `user_response` is provided:

1. The handler subscribes to the in-process `EventBus`.
2. Waits up to **N seconds** (e.g. 15 s — safely within MCP tool-call timeout)
   for a `phase-complete` event on the current checkpoint phase.
3. **Event arrives** (Dashboard called `PhaseComplete` → EventBus published):
   reload state → `eng.NextAction` → `sm.PhaseStart` → return the next real
   action (e.g. `spawn_agent` for phase-4). Claude proceeds with no terminal
   interaction.
4. **Timeout elapses** with no event: return `{type: "checkpoint",
   still_waiting: true}` with the same checkpoint presentation text.

```text
Claude (no AskUserQuestion)        MCP server                  Dashboard
       │                                │                           │
       │── pipeline_next_action() ─────▶│                           │
       │                                │ subscribe EventBus        │
       │   [MCP call blocked]           │ wait up to 15s            │
       │                                │                           │
       │                                │◀── PhaseComplete() ───────│ user clicks approve
       │                                │ event: phase-complete      │
       │                                │ reload state              │
       │                                │ eng.NextAction            │
       │◀─ {type: "spawn_agent"} ───────│ PhaseStart               │
       │                                │                           │
       │  continue pipeline             │                           │
```

If no Dashboard approval arrives within 15 s, the tool returns `still_waiting:
true`. SKILL.md calls `pipeline_next_action` again immediately (no sleep, no
terminal prompt). The 15 s server-side delay provides natural pacing.

### Terminal user path

The terminal path is unaffected in correctness, with a max 15 s delay:

1. Claude is blocked on the `pipeline_next_action` long-poll.
2. User types "proceed" in terminal → Claude Code queues the message.
3. After 15 s (or earlier if Dashboard fires), the tool returns `still_waiting:
   true`.
4. Claude processes the queued "proceed" → calls
   `pipeline_next_action(user_response="proceed")`.
5. P8 block calls `sm.PhaseComplete` + the engine returns the next action.

### SKILL.md change (minimal)

The only SKILL.md change is the checkpoint action handler:

```text
- `checkpoint`: Call checkpoint(). Output action.present_to_user + Dashboard URL.
  Then immediately call pipeline_next_action() (no user_response, no AskUserQuestion).
  - If still_waiting: true  → call pipeline_next_action() again immediately.
  - If non-checkpoint action returned → Dashboard approved; proceed normally.
  - User may type in terminal at any time; their message is processed after the
    current pipeline_next_action() call returns (max 15 s delay).
```

### Changes summary

| Component | Change | Scope |
| --- | --- | --- |
| `pipeline_next_action.go` | Add long-poll path when `awaiting_human` + no `user_response` | Medium |
| `SKILL.md` | Replace `AskUserQuestion` with immediate re-call on `still_waiting` | Small |
| `dashboard.html` | After approve: show "Pipeline will continue automatically" | Trivial |
| `intervention.go` | Optional: bearer token auth (`FORGE_DASHBOARD_TOKEN`) | Small |

### Remote access via ngrok

No server binding changes are required. The Dashboard stays on `127.0.0.1`.
ngrok forwards external traffic to the local port:

```text
smartphone browser
       │
       ▼ HTTPS
  ngrok tunnel (e.g. https://abc123.ngrok.io)
       │
       ▼ HTTP (loopback)
  127.0.0.1:8099  ←  forge Dashboard server
```

The `isLocalRequest` guard in `intervention.go` sees only the ngrok agent's
loopback connection and passes it through unchanged.

The `Origin` header from the browser will be the ngrok URL, which currently
fails the same-origin check. Fix: when `FORGE_DASHBOARD_TOKEN` is set and the
request carries a valid `Authorization: Bearer <token>` header, bypass the
origin check entirely — token auth supersedes CSRF protection.

**Security model:**

| Mode | Binding | Auth |
| --- | --- | --- |
| Default (current) | `127.0.0.1` | loopback + same-origin (unchanged) |
| Remote (ngrok) | `127.0.0.1` | `FORGE_DASHBOARD_TOKEN` bearer token |

Setting up ngrok:

```bash
# start the MCP server with dashboard
FORGE_EVENTS_PORT=8099 FORGE_DASHBOARD_TOKEN=<secret> forge-state-mcp

# in another terminal, expose via ngrok
ngrok http 8099 --request-header-add "Authorization: Bearer <secret>"
```

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

1. **`pipeline_next_action.go`**: Add EventBus long-poll when
   `currentPhaseStatus == "awaiting_human"` and no `user_response` provided.
   - Subscribe to `bus` (already threaded into the handler).
   - Select on: EventBus channel (`phase-complete` for current phase), 15 s
     ticker, `ctx.Done()`.
   - On match: reload state, run engine, call `PhaseStart`, return action.
   - On timeout: return checkpoint action with `still_waiting: true`.

2. **`SKILL.md`**: Checkpoint action handler — remove `AskUserQuestion`, add
   immediate re-call loop on `still_waiting: true`.

3. **`intervention.go`**: Add optional bearer token middleware.
   - Read `FORGE_DASHBOARD_TOKEN` from env at server start.
   - If set: accept any origin when `Authorization: Bearer <token>` matches;
     reject otherwise (regardless of loopback).
   - If unset: existing `isLocalRequest` behavior unchanged.

4. **`dashboard.html`**: Post-approve UX — replace "approved" button label
   with "✓ Pipeline will continue automatically".

### Phase 2 (future)

Design and implement the Agent SDK-based task runner and Dashboard submission
form as a separate initiative.

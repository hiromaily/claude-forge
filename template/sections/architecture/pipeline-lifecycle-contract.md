# Pipeline Lifecycle Contract

> **Status: Implemented** — All gaps have been resolved. `pipeline_next_action` calls `sm.PhaseStart()` and emits `phase-start` events; `pipeline_report_result` emits `phase-complete` events; checkpoint events are emitted when setting `awaiting_human`.

## Purpose

This document defines the **mandatory state-transition contract** for pipeline phases. Every phase must follow a symmetric `PhaseStart` / `PhaseComplete` lifecycle. No component may bypass this contract.

## The Problem This Solves

The pipeline has two execution paths:

1. **Standalone handlers** (`phase_start`, `phase_complete` MCP tools) — call `sm.PhaseStart()` and `sm.PhaseComplete()` with proper state updates and event emission
2. **Pipeline engine** (`pipeline_next_action` + `pipeline_report_result`) — drives the main loop but historically bypassed `sm.PhaseStart()`, causing:
   - `CurrentPhaseStatus` stuck at `"pending"` instead of `"in_progress"`
   - `Timestamps.PhaseStarted` never set
   - `phase-start` event never emitted (dashboard shows nothing until completion)
   - `phase-complete` event never emitted from `pipeline_report_result`

This contract eliminates the inconsistency by requiring all paths to honour the same lifecycle.

## Phase Lifecycle

Every phase transition follows this sequence. No step may be skipped.

```
pending ──[PhaseStart]──> in_progress ──[PhaseComplete]──> (next phase: pending)
   │                           │
   │                           ├──[PhaseFail]──> failed
   │                           └──[Checkpoint]──> awaiting_human
   │
   └──[PhaseCompleteSkipped]──> (next phase: pending)
```

### State Mutations

| Transition | Method | Sets `CurrentPhaseStatus` | Sets `Timestamps.PhaseStarted` | Emits Event |
|---|---|---|---|---|
| **Start** | `sm.PhaseStart(workspace, phase)` | `"in_progress"` | `nowISO()` | `phase-start` |
| **Complete** | `sm.PhaseComplete(workspace, phase)` | `"pending"` (next) or `"completed"` | `nil` | `phase-complete` |
| **Fail** | `sm.PhaseFail(workspace, msg)` | `"failed"` | _(unchanged)_ | `phase-fail` |
| **Checkpoint** | `sm.Checkpoint(workspace, phase, ...)` | `"awaiting_human"` | _(unchanged)_ | `checkpoint` |
| **Skip** | `sm.PhaseCompleteSkipped(workspace, phase)` | `"pending"` (next) | `nil` | _(none)_ |

**Note on event emission**: `sm.PhaseStart()` and `sm.PhaseComplete()` are pure state mutations — they do not emit events themselves. The caller is responsible for calling `publishEvent()` after a successful state mutation. This keeps `StateManager` free of `EventBus` dependencies (see [Design Decisions](#why-events-are-emitted-at-the-handler-level-not-in-statemanager)).

### Invariants

1. **Symmetric start/complete**: Every `PhaseStart` must be followed by exactly one `PhaseComplete`, `PhaseFail`, or `Abandon`. No phase may complete without first being started.
2. **Single writer**: Only one component transitions a given phase. In the pipeline loop, `pipeline_next_action` owns `PhaseStart` and `pipeline_report_result` owns `PhaseComplete`.
3. **Event-state consistency**: Events are emitted **after** the corresponding state mutation succeeds, never before. If the mutation fails, no event is emitted.
4. **Idempotency**: `Engine.NextAction()` is read-only — it never mutates state. It returns a signal; the caller (`pipeline_next_action`) is responsible for state transitions.

## Execution Paths

### Path 1: Pipeline Engine (primary)

The main execution loop. Used for all automated pipeline runs via `/forge`.

```
pipeline_next_action
  ├── eng.NextAction() → Action        [read-only decision]
  ├── sm.PhaseStart(workspace, phase)  [state: pending → in_progress]
  ├── publishEvent("phase-start")      [dashboard notification]
  └── return Action to orchestrator    [orchestrator executes it]

[orchestrator executes action: Agent, exec, write_file]

pipeline_report_result
  ├── sm.PhaseLog(...)                 [record metrics]
  ├── determineTransition()
  │   └── sm.PhaseComplete(...)        [state: in_progress → pending (next)]
  ├── publishEvent("phase-complete")   [dashboard notification]
  └── return next_action_hint
```

**Action-type variations:**

| Action type | `phase-start` emitted? | `agent-dispatch` emitted? | Reported via |
|---|---|---|---|
| `spawn_agent` | Yes | Yes (with agent name) | `pipeline_report_result` |
| `exec` | Yes | No | `pipeline_report_result` (P5 embedded path) |
| `write_file` | Yes | No | `pipeline_report_result` (P5 embedded path) |
| `checkpoint` | No (see Path 3) | No | Checkpoint flow |
| `done` (skip) | No | No | P1 skip loop (internal) |

**P1 skip loop**: When `Engine.NextAction()` returns `ActionDone` with `SkipSummaryPrefix`, `pipeline_next_action` absorbs it internally — calls `sm.PhaseCompleteSkipped()` and re-invokes `eng.NextAction()` in a bounded loop (max 20 iterations). No `phase-start` or `phase-complete` events are emitted for skipped phases.

### Path 2: Standalone Handlers (debug / manual)

Individual `phase_start` / `phase_complete` MCP tools. Used for manual state manipulation and debugging.

```
PhaseStartHandler
  ├── guard checks (e.g., tasks non-empty for phase-5)
  ├── sm.PhaseStart(workspace, phase)
  └── publishEvent("phase-start")

PhaseCompleteHandler
  ├── guard checks (artifact exists, not awaiting human, no pending revision)
  ├── sm.PhaseComplete(workspace, phase)
  └── publishEvent("phase-complete")
```

### Path 3: Checkpoint Flow

Human-review gates. `pipeline_next_action` detects checkpoint phases and absorbs the checkpoint state transition by calling `sm.Checkpoint(workspace, st.CurrentPhase)`, which sets both `CurrentPhase` and `CurrentPhaseStatus = "awaiting_human"`. The orchestrator presents the checkpoint to the user. On the next `pipeline_next_action` call, the handler long-polls (up to 50 s) for a Dashboard approval event; if the user responds via terminal, the response is passed as `user_response`.

```
pipeline_next_action (checkpoint action detected)
  ├── sm.Checkpoint(): CurrentPhaseStatus = "awaiting_human"
  ├── publish "checkpoint" event
  └── return Action{type: "checkpoint"} to orchestrator

pipeline_next_action (2nd call, no user_response)
  ├── long-poll up to 50 s for Dashboard "phase-complete" event
  ├── Dashboard approves → reload state → return next action
  └── timeout → return Action{type: "checkpoint", still_waiting: true}

pipeline_next_action (with user_response from terminal)
  ├── "proceed" → sm.PhaseComplete(workspace, phase)
  ├── "revise"  → sm.Update() to rewind state
  └── "abandon" → sm.Abandon()
```

**Note**: The standalone `CheckpointHandler` (`handlers.go`) is retained for manual debugging and backward compatibility but is no longer called by the orchestrator for standard checkpoints (checkpoint-a, checkpoint-b). The pipeline engine path now uses `sm.Checkpoint()` directly.

## Event Taxonomy

Events are the dashboard's view of pipeline state. They must form a coherent timeline.

| Event | Emitter | When | Outcome |
|---|---|---|---|
| `pipeline-init` | `pipeline_init_with_context` | Workspace created | `in_progress` |
| `phase-start` | `pipeline_next_action` or `PhaseStartHandler` | Phase begins | `in_progress` |
| `agent-dispatch` | `pipeline_next_action` | Agent spawned (detail: agent name) | `dispatched` |
| `action-complete` | `pipeline_next_action` (P5 embedded report path) | Agent/exec finished (detail: model) | `completed` |
| `phase-complete` | `pipeline_next_action` (P5 path), `pipeline_report_result`, or `PhaseCompleteHandler` | Phase done | `completed` |
| `phase-fail` | `PhaseFailHandler` | Phase failed | `failed` |
| `checkpoint` | `pipeline_next_action` or `CheckpointHandler` | Awaiting human | `awaiting_human` |
| `revision-required` | `pipeline_next_action` | Review verdict REVISE | `failed` |
| `pipeline-complete` | `pipeline_next_action` | All phases done | `completed` |
| `abandon` | `AbandonHandler` | Pipeline abandoned | `abandoned` |

### Expected Event Sequence per Phase

A normal `spawn_agent` phase produces:

```
phase-start (in_progress)
  → agent-dispatch (dispatched)
  → action-complete (completed)
  → phase-complete (completed)
```

An `exec` or `write_file` phase produces:

```
phase-start (in_progress)
  → action-complete (completed)
  → phase-complete (completed)
```

## Design Decisions

### Why PhaseStart lives in pipeline_next_action

`pipeline_next_action` is the single entry point for phase transitions in the pipeline loop. Placing `PhaseStart` here (rather than in the Engine or StateManager) ensures:

- **Locality**: The start transition is adjacent to the dispatch decision, making the code easy to audit
- **Symmetry**: `pipeline_next_action` starts phases; `pipeline_report_result` completes them
- **Engine purity**: `Engine.NextAction()` remains a pure function of state — no side effects
- **Layer compliance**: The `tools → orchestrator → state` import direction is preserved

### Why standalone handlers are retained

`phase_start` and `phase_complete` MCP tools remain available for:

- Manual state recovery after interruptions
- Debugging pipeline state in development
- Future CLI tooling that operates outside the pipeline loop

They follow the same contract and must not conflict with the pipeline engine path.

### Why events are emitted at the handler level, not in StateManager

`StateManager` is a pure state-persistence layer with no external dependencies. Adding `EventBus` would violate the `tools → orchestrator → state` layering:

```
tools (publishEvent + sm.PhaseStart)
  → orchestrator (Engine — read-only)
    → state (StateManager — persistence only)
```

Events are a **presentation concern** (dashboard, Slack). They belong at the handler level where the bus is available.

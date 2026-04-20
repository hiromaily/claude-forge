# Pipeline Lifecycle Contract

When modifying pipeline code (`mcp-server/internal/handler/tools/pipeline_next_action.go`, `pipeline_report_result.go`, `handlers.go`, or `orchestrator/engine.go`), follow these rules.

**Full specification**: `docs/architecture/pipeline-lifecycle-contract.md` (SSOT: `template/sections/architecture/pipeline-lifecycle-contract.md`)

> **Implementation status**: Fully implemented. `pipeline_next_action` calls `sm.PhaseStart()` and emits `phase-start`/`checkpoint` events; `pipeline_report_result` emits `phase-complete` events.

## Mandatory Rules

### 1. Every phase must follow PhaseStart then PhaseComplete

No component may advance a phase without calling `sm.PhaseStart()` first. The state must transition `pending -> in_progress -> completed` (or `failed`/`abandoned`).

**Violation example** (prohibited):
```go
// BAD: completing a phase that was never started
action, _ := eng.NextAction(sm, "")
// ... execute action ...
sm.PhaseComplete(workspace, phase) // PhaseStart was never called
```

**Correct**:
```go
action, _ := eng.NextAction(sm, "")
sm.PhaseStart(workspace, action.Phase)
publishEvent(bus, nil, "phase-start", action.Phase, ...)
// ... execute action ...
// (pipeline_report_result handles PhaseComplete)
```

### 2. Engine.NextAction() is read-only

`Engine.NextAction()` must never mutate state. It reads `state.json` and returns an `Action` signal. The caller (`pipeline_next_action`) is responsible for executing state transitions.

### 3. Event emission follows state mutation

Events are emitted **after** the corresponding `StateManager` method succeeds. Never emit a `phase-start` event without first calling `sm.PhaseStart()`.

### 4. Ownership boundaries

| Responsibility | Owner |
|---|---|
| Phase start (`pending -> in_progress`) | `pipeline_next_action` |
| Phase complete (`in_progress -> completed`) | `pipeline_report_result` via `determineTransition()` |
| Checkpoint (`in_progress -> awaiting_human`) | `pipeline_next_action` (absorbed; calls `sm.Checkpoint()` internally) |
| Checkpoint resolution | `pipeline_next_action` (with `user_response`) |

### 5. Event pairs

Every `phase-start` event must eventually be followed by a `phase-complete`, `phase-fail`, or `abandon` event for the same phase. An orphaned `phase-start` with no completion event indicates a bug.

## When Modifying Pipeline Code

- Read `template/sections/architecture/pipeline-lifecycle-contract.md` before making changes
- Verify that `PhaseStart` and `PhaseComplete` are both called for every phase
- Run `cd mcp-server && go test ./internal/handler/tools/... -count=1` after changes
- Check dashboard event timeline shows `phase-start -> agent-dispatch -> action-complete -> phase-complete` for each phase

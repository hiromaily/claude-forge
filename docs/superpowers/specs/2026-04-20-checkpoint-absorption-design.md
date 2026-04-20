# Checkpoint Absorption — Deterministic Dashboard Approval

## Problem

When a user clicks "Approve" on the Dashboard at an `AWAITING_HUMAN` checkpoint,
the API succeeds (`✓ Pipeline will continue automatically`) but the terminal
does not advance. Root cause analysis identified two issues:

1. **Server-side (resolved)**: `approveCheckpointHandler` now publishes a
   `phase-complete` event to the EventBus, and `pipeline_next_action` long-polls
   for this event. The server-side wiring is correct.

2. **Orchestrator-side (this design)**: The orchestrator (Claude Code LLM)
   must implement a `still_waiting` polling loop per SKILL.md instructions.
   LLMs are non-deterministic — the orchestrator may use `AskUserQuestion`
   instead, or fail to re-call `pipeline_next_action`, leaving no subscriber
   listening for the Dashboard event.

Additionally, the current flow requires 3 MCP tool calls before the long-poll
starts (`pipeline_next_action` → `checkpoint()` → `pipeline_next_action`),
increasing the surface area for LLM deviation.

## Goal

- Make Dashboard checkpoint approval work deterministically
- Reduce LLM-dependent steps from 3 tool calls to 2
- Extend the long-poll window to minimize re-call iterations
- Document MCP protocol constraints as SSOT for future design decisions

## Design

### 1. Checkpoint Absorption into `pipeline_next_action`

Absorb the `checkpoint()` MCP tool's state transition into `pipeline_next_action`
so the orchestrator never needs to call `checkpoint()` separately.

**Current flow (3 tool calls + LLM loop)**:

```text
Orchestrator              MCP Server               Dashboard
    │                         │                        │
    │─ pipeline_next_action ─▶│                        │
    │◀─ ActionCheckpoint ─────│                        │
    │                         │                        │
    │─ checkpoint() ─────────▶│ sm.Checkpoint()        │  ← redundant
    │◀─ "ok" ────────────────│                        │
    │                         │                        │
    │─ pipeline_next_action ─▶│ long-poll 15s          │
    │◀─ still_waiting ───────│                        │  ← LLM must re-call
    │                         │                        │
    │─ pipeline_next_action ─▶│ long-poll 15s          │  ← non-deterministic
    │                    .... │ ...                    │─ Approve
    │◀─ next action ─────────│                        │
```

**Proposed flow (2 tool calls, 1st is instant)**:

```text
Orchestrator              MCP Server               Dashboard
    │                         │                        │
    │─ pipeline_next_action ─▶│                        │
    │  (1st call)             │ sm.Checkpoint() absorbed│
    │◀─ ActionCheckpoint ─────│ present_to_user + text │
    │                         │                        │
    │  [presents text]        │                        │
    │                         │                        │
    │─ pipeline_next_action ─▶│ long-poll 50s          │
    │  (2nd call)             │                        │─ Approve (within 50s)
    │◀─ next action ─────────│ event received, reload │
    │                         │                        │
    │  continue pipeline      │                        │
```

### 2. Code Changes

#### 2.1 `pipeline_next_action.go` — Checkpoint absorption (L574-585)

Replace the current `sm2.Update()` call with `sm2.Checkpoint()`:

```go
// Before:
if action.Type == orchestrator.ActionCheckpoint {
    sm2.Update(func(s *state.State) error {
        s.CurrentPhaseStatus = "awaiting_human"
        return nil
    })
    publishEvent(bus, nil, "checkpoint", ...)
}

// After:
if action.Type == orchestrator.ActionCheckpoint {
    if err := sm2.Checkpoint(workspace, action.Phase); err != nil {
        appendWarning(fmt.Sprintf("Checkpoint: %v", err))
    }
    publishEvent(bus, nil, "checkpoint", ...)
}
```

`sm.Checkpoint()` sets both `CurrentPhase` and `CurrentPhaseStatus = awaiting_human`,
which is a superset of the current `Update()` that only sets `CurrentPhaseStatus`.

#### 2.2 `pipeline_next_action.go` — Long-poll timeout extension

```go
// Before:
const checkpointLongPollTimeout = 15 * time.Second

// After:
const checkpointLongPollTimeout = 50 * time.Second
```

50 seconds provides a 10-second margin against the default 60-second MCP tool
call timeout. See `docs/architecture/mcp-protocol-constraints.md` for the
rationale.

#### 2.3 Long-poll entry condition (unchanged)

The existing long-poll entry condition (L295-301) requires no changes:

```go
if userResponse == "" {
    subID, eventCh := bus.Subscribe()
    st, stErr := sm2.GetState()
    if stErr == nil && st.CurrentPhaseStatus == state.StatusAwaitingHuman {
        // enter long-poll
    }
}
```

- **1st call**: State loaded from disk is not yet `awaiting_human` → long-poll
  skipped → ActionCheckpoint returned → checkpoint absorbed at L574
- **2nd call**: State loaded from disk is `awaiting_human` → long-poll starts (50s)

### 3. SKILL.md Changes

Replace the current checkpoint handling (L112-149, 38 lines) with:

```text
- `checkpoint`: Present `action.present_to_user` to the user.
  Mention that the Dashboard can be used to approve without terminal input.
  Then immediately call `pipeline_next_action(workspace)` (no `user_response`,
  no `previous_*`). The server long-polls up to 50s for Dashboard approval.
  - If `still_waiting: true`: call `pipeline_next_action(workspace)` again
    immediately. Repeat until a non-checkpoint action is returned.
  - If the user types a response in the terminal (proceed / revise / abandon):
    on the next call, pass `user_response=<response>`.
  - If a non-checkpoint action is returned: proceed normally.
  Do NOT call `checkpoint()` — pipeline_next_action handles the state
  transition internally.
```


Key changes:

- `checkpoint()` MCP tool call removed
- Long-poll duration documented (50s)
- post-to-source checkpoint special case preserved as-is
- Terminal user response path unchanged (P8 block handles it)

### 4. `--auto` Flag Compatibility

No impact. When `--auto` conditions are met, the engine returns
`ActionSpawnAgent` or `ActionDone` (skip) — never `ActionCheckpoint`.
The absorption code only executes for `ActionCheckpoint`, so auto-skip
paths are unaffected.

| Condition | Engine return | Absorption code |
| --------- | ------------- | --------------- |
| `--auto` + APPROVE verdict | `ActionSpawnAgent` (phase-4) | Not reached |
| `--auto` + phase-4 skipped | `ActionDone` (skip prefix) | Not reached |
| No `--auto` or REVISE verdict | `ActionCheckpoint` | **Executed** |

### 5. `CheckpointHandler` Retention

The standalone `CheckpointHandler` (in `handlers.go`) is **not deleted**.
It remains available for:

- Manual debugging via MCP tool calls
- Backward compatibility with older SKILL.md versions
- The post-to-source checkpoint which uses a different flow

SKILL.md no longer instructs the orchestrator to call it for standard
checkpoints (checkpoint-a, checkpoint-b).

### 6. Terminal User Path

The terminal user response path is preserved via the existing P8 block
(L198-257) in `pipeline_next_action`:

1. Orchestrator presents checkpoint text (1st call return)
2. Orchestrator calls `pipeline_next_action` → 50s long-poll (2nd call)
3. User types "proceed" in terminal → Claude Code queues the message
4. After 50s timeout, orchestrator processes queued message
5. Next `pipeline_next_action(user_response="proceed")` → P8 handles it

Maximum latency: 50s between user typing and pipeline advancing.
Acceptable because checkpoints are review gates, not interactive prompts.

## Test Strategy

### Modified tests

- `pipeline_next_action_test.go`: Update long-poll timeout references (15s → 50s)

### New test cases

| Test | Validates |
| ---- | --------- |
| `TestCheckpointAbsorption_SetsAwaitingHuman` | `sm.Checkpoint()` called internally, state is `awaiting_human` |
| `TestCheckpointAbsorption_NoExternalCheckpointCall` | Long-poll works without separate `checkpoint()` call |
| `TestLongPoll_50sTimeout` | Timeout constant is 50s |
| `TestAutoApprove_BypassesAbsorption` | `--auto` + APPROVE → absorption code not reached |

### Unaffected tests

- `TestApproveCheckpoint_*` (intervention_test.go) — Dashboard API unchanged
- `TestCheckpointHandlerPublishesEvent` (handlers_test.go) — handler retained
- Hook tests (`test-hooks.sh`) — `awaiting_human` state unchanged

## Impact Summary

| Component | Change | Size |
| --------- | ------ | ---- |
| `pipeline_next_action.go` | Checkpoint absorption + long-poll 50s | Medium |
| `pipeline_next_action_test.go` | New tests + timeout updates | Medium |
| `SKILL.md` | Checkpoint handling simplification | Small |
| `handlers.go` | No change (CheckpointHandler retained) | None |
| `docs/architecture/mcp-protocol-constraints.md` | New: MCP constraint documentation | New |
| `template/sections/architecture/mcp-protocol-constraints.md` | SSOT source | New |
| `docs/ja/architecture/mcp-protocol-constraints.md` | Japanese translation | New |

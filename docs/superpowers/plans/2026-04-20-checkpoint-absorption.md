# Checkpoint Absorption Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Dashboard checkpoint approval deterministic by absorbing the `checkpoint()` state transition into `pipeline_next_action` and extending the long-poll from 15s to 50s.

**Architecture:** Replace the `sm2.Update()` call that only sets `CurrentPhaseStatus` with `sm2.Checkpoint()` that sets both `CurrentPhase` and `CurrentPhaseStatus`. Extend the long-poll timeout constant from 15s to 50s. Update SKILL.md to remove the `checkpoint()` MCP tool call instruction.

**Tech Stack:** Go 1.26, mcp-go, existing EventBus/StateManager

**Spec:** `docs/superpowers/specs/2026-04-20-checkpoint-absorption-design.md`

---

### Task 1: Extend long-poll timeout from 15s to 50s

**Files:**

- Modify: `mcp-server/internal/handler/tools/pipeline_next_action.go:27-29`

- [ ] **Step 1: Update the timeout constant**

In `mcp-server/internal/handler/tools/pipeline_next_action.go`, change line 27-29:

```go
// Before:
// checkpointLongPollTimeout is the maximum time pipeline_next_action blocks
// waiting for a Dashboard-triggered phase-complete event at a checkpoint.
// 15 seconds is safely within the MCP tool-call timeout on all observed clients.
const checkpointLongPollTimeout = 15 * time.Second

// After:
// checkpointLongPollTimeout is the maximum time pipeline_next_action blocks
// waiting for a Dashboard-triggered phase-complete event at a checkpoint.
// 50 seconds provides a 10-second margin against the default 60-second MCP
// tool-call timeout. See docs/architecture/mcp-protocol-constraints.md.
const checkpointLongPollTimeout = 50 * time.Second
```

- [ ] **Step 2: Run tests to verify no regressions**

Run: `cd mcp-server && go test -race ./internal/handler/tools/... -count=1 -run TestPipelineNextAction`

Expected: All existing tests pass. The long-poll tests use `context.WithTimeout` for short-circuiting, so the 50s constant does not affect test execution time.

- [ ] **Step 3: Commit**

```bash
git -C /Users/hiroki.yasui/go/src/github.com/legalforce/worktree2/dealon-app/poc/claude-forge add mcp-server/internal/handler/tools/pipeline_next_action.go
git -C /Users/hiroki.yasui/go/src/github.com/legalforce/worktree2/dealon-app/poc/claude-forge commit -m "perf(pipeline): extend checkpoint long-poll timeout from 15s to 50s"
```

---

### Task 2: Absorb checkpoint state transition into `pipeline_next_action`

**Files:**

- Modify: `mcp-server/internal/handler/tools/pipeline_next_action.go:570-585`

- [ ] **Step 1: Replace `sm2.Update()` with `sm2.Checkpoint()`**

In `mcp-server/internal/handler/tools/pipeline_next_action.go`, replace lines 570-585:

```go
// Before (lines 570-585):
		// Eliminate the window between pipeline_next_action returning a checkpoint action
		// and the orchestrator calling mcp__forge-state__checkpoint().
		// Set currentPhaseStatus to "awaiting_human" immediately so the stop hook permits
		// session exit even if the conversation ends before the orchestrator calls checkpoint().
		if action.Type == orchestrator.ActionCheckpoint {
			if updateErr := sm2.Update(func(s *state.State) error {
				s.CurrentPhaseStatus = "awaiting_human"
				return nil
			}); updateErr != nil {
				// Fail-open: warn but still return the action.
				appendWarning(fmt.Sprintf("set awaiting_human: %v", updateErr))
			}
			if st, stErr := sm2.GetState(); stErr == nil {
				publishEvent(bus, nil, "checkpoint", action.Phase, st.SpecName, workspace, "awaiting_human")
			}
		}
```

Replace with:

```go
// After:
		// Absorb the checkpoint state transition that was previously done by the
		// standalone checkpoint() MCP tool. sm2.Checkpoint() sets both CurrentPhase
		// and CurrentPhaseStatus=awaiting_human, which is a superset of the previous
		// Update() that only set CurrentPhaseStatus. This eliminates the need for
		// the orchestrator to call checkpoint() as a separate MCP tool call.
		if action.Type == orchestrator.ActionCheckpoint {
			if chkErr := sm2.Checkpoint(workspace, action.Phase); chkErr != nil {
				// Fail-open: warn but still return the action.
				appendWarning(fmt.Sprintf("Checkpoint: %v", chkErr))
			}
			if st, stErr := sm2.GetState(); stErr == nil {
				publishEvent(bus, nil, "checkpoint", action.Phase, st.SpecName, workspace, "awaiting_human")
			}
		}
```

- [ ] **Step 2: Run tests to verify no regressions**

Run: `cd mcp-server && go test -race ./internal/handler/tools/... -count=1`

Expected: All existing tests pass. The `TestPipelineNextAction_LongPoll` test pre-sets `CurrentPhaseStatus = awaiting_human`, so the absorption code path (which runs on the *first* call, before `awaiting_human` is set) does not interfere with existing long-poll tests.

- [ ] **Step 3: Commit**

```bash
git -C /Users/hiroki.yasui/go/src/github.com/legalforce/worktree2/dealon-app/poc/claude-forge add mcp-server/internal/handler/tools/pipeline_next_action.go
git -C /Users/hiroki.yasui/go/src/github.com/legalforce/worktree2/dealon-app/poc/claude-forge commit -m "feat(pipeline): absorb checkpoint state transition into pipeline_next_action"
```

---

### Task 3: Add tests for checkpoint absorption

**Files:**

- Modify: `mcp-server/internal/handler/tools/pipeline_next_action_test.go`

- [ ] **Step 1: Write test for checkpoint absorption setting `awaiting_human`**

Add the following test after the existing `TestPipelineNextAction_LongPoll_Timeout` function (after line 1637):

```go
// TestPipelineNextAction_CheckpointAbsorption verifies that when
// pipeline_next_action returns an ActionCheckpoint, the handler internally
// calls sm.Checkpoint() to set CurrentPhaseStatus=awaiting_human and
// CurrentPhase to the checkpoint phase — without requiring a separate
// checkpoint() MCP tool call.
func TestPipelineNextAction_CheckpointAbsorption(t *testing.T) {
	t.Parallel()

	// Set up workspace at checkpoint-a with status=in_progress (not yet awaiting_human).
	// This simulates the first pipeline_next_action call that encounters a checkpoint.
	workspace, sm := initWorkspaceForNextAction(t, state.PhaseCheckpointA, func(s *state.State) error {
		s.CurrentPhaseStatus = state.StatusInProgress
		// Mark prerequisite phases as completed so the engine returns ActionCheckpoint.
		s.CompletedPhases = []string{
			state.PhaseOne, state.PhaseTwo, state.PhaseThree, state.PhaseThreeB,
		}
		return nil
	})
	bus := events.NewEventBus()
	eng := orchestrator.NewEngine("", "")
	h := PipelineNextActionHandler(sm, bus, eng, "", nil, nil, nil)

	result, err := callNextAction(t, h, workspace)
	if err != nil {
		t.Fatalf("PipelineNextActionHandler: %v", err)
	}
	if result.IsError {
		t.Fatalf("PipelineNextActionHandler returned error: %s", textContent(result))
	}

	var resp nextActionResponse
	if jsonErr := json.Unmarshal([]byte(textContent(result)), &resp); jsonErr != nil {
		t.Fatalf("unmarshal response: %v", jsonErr)
	}
	if resp.Type != orchestrator.ActionCheckpoint {
		t.Fatalf("action type = %q, want %q", resp.Type, orchestrator.ActionCheckpoint)
	}

	// Verify that the handler absorbed the checkpoint: state on disk should now
	// have CurrentPhaseStatus=awaiting_human AND CurrentPhase=checkpoint-a.
	sm2 := state.NewStateManager("dev")
	if err := sm2.LoadFromFile(workspace); err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	st, err := sm2.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if st.CurrentPhaseStatus != state.StatusAwaitingHuman {
		t.Errorf("CurrentPhaseStatus = %q, want %q", st.CurrentPhaseStatus, state.StatusAwaitingHuman)
	}
	if st.CurrentPhase != state.PhaseCheckpointA {
		t.Errorf("CurrentPhase = %q, want %q", st.CurrentPhase, state.PhaseCheckpointA)
	}
}
```

- [ ] **Step 2: Write test verifying long-poll works without separate `checkpoint()` call**

Add the following test:

```go
// TestPipelineNextAction_CheckpointAbsorption_ThenLongPoll verifies the
// two-phase flow: first call absorbs checkpoint (returns ActionCheckpoint),
// second call enters long-poll and wakes on Dashboard approval.
func TestPipelineNextAction_CheckpointAbsorption_ThenLongPoll(t *testing.T) {
	t.Parallel()

	workspace, sm := initWorkspaceForNextAction(t, state.PhaseCheckpointA, func(s *state.State) error {
		s.CurrentPhaseStatus = state.StatusInProgress
		s.CompletedPhases = []string{
			state.PhaseOne, state.PhaseTwo, state.PhaseThree, state.PhaseThreeB,
		}
		return nil
	})
	bus := events.NewEventBus()
	eng := orchestrator.NewEngine("", "")
	h := PipelineNextActionHandler(sm, bus, eng, "", nil, nil, nil)

	// Phase 1: first call absorbs the checkpoint.
	result1, err := callNextAction(t, h, workspace)
	if err != nil {
		t.Fatalf("1st call: %v", err)
	}
	var resp1 nextActionResponse
	if jsonErr := json.Unmarshal([]byte(textContent(result1)), &resp1); jsonErr != nil {
		t.Fatalf("unmarshal 1st: %v", jsonErr)
	}
	if resp1.Type != orchestrator.ActionCheckpoint {
		t.Fatalf("1st call type = %q, want checkpoint", resp1.Type)
	}

	// Phase 2: second call should enter long-poll (state is now awaiting_human).
	// Simulate Dashboard approval after a short delay.
	go func() {
		time.Sleep(20 * time.Millisecond)
		sm2 := state.NewStateManager("dev")
		if loadErr := sm2.LoadFromFile(workspace); loadErr != nil {
			return
		}
		_ = sm2.PhaseComplete(workspace, state.PhaseCheckpointA)
		bus.Publish(events.Event{
			Event:     "phase-complete",
			Phase:     "checkpoint-a",
			Workspace: workspace,
			Outcome:   "completed",
		})
	}()

	// Create a fresh handler with a fresh StateManager for the 2nd call
	// (mimics pipeline_next_action's per-call sm2 creation).
	h2 := PipelineNextActionHandler(sm, bus, eng, "", nil, nil, nil)
	result2, err := callNextAction(t, h2, workspace)
	if err != nil {
		t.Fatalf("2nd call: %v", err)
	}

	var resp2 nextActionResponse
	if jsonErr := json.Unmarshal([]byte(textContent(result2)), &resp2); jsonErr != nil {
		t.Fatalf("unmarshal 2nd: %v", jsonErr)
	}
	if resp2.StillWaiting {
		t.Error("2nd call: StillWaiting=true, expected Dashboard approval to wake long-poll")
	}
	if resp2.Type == orchestrator.ActionCheckpoint {
		t.Errorf("2nd call type = %q, want non-checkpoint action after approval", resp2.Type)
	}
}
```

- [ ] **Step 3: Write test verifying timeout constant is 50s**

Add the following test:

```go
// TestCheckpointLongPollTimeout_Is50s documents the timeout constant value.
// If this test fails, the MCP protocol constraints document
// (docs/architecture/mcp-protocol-constraints.md) must also be updated.
func TestCheckpointLongPollTimeout_Is50s(t *testing.T) {
	t.Parallel()
	if checkpointLongPollTimeout != 50*time.Second {
		t.Errorf("checkpointLongPollTimeout = %v, want 50s", checkpointLongPollTimeout)
	}
}
```

- [ ] **Step 4: Run all new tests**

Run: `cd mcp-server && go test -race ./internal/handler/tools/... -count=1 -run "TestPipelineNextAction_CheckpointAbsorption|TestCheckpointLongPollTimeout"`

Expected: All 3 new tests pass.

- [ ] **Step 5: Run full test suite to verify no regressions**

Run: `cd mcp-server && go test -race ./... -count=1`

Expected: All tests pass.

- [ ] **Step 6: Commit**

```bash
git -C /Users/hiroki.yasui/go/src/github.com/legalforce/worktree2/dealon-app/poc/claude-forge add mcp-server/internal/handler/tools/pipeline_next_action_test.go
git -C /Users/hiroki.yasui/go/src/github.com/legalforce/worktree2/dealon-app/poc/claude-forge commit -m "test(pipeline): add checkpoint absorption and long-poll timeout tests"
```

---

### Task 4: Update SKILL.md checkpoint handling

**Files:**

- Modify: `skills/forge/SKILL.md:112-149`

- [ ] **Step 1: Replace the checkpoint action handler in SKILL.md**

In `skills/forge/SKILL.md`, replace lines 112-149 (the `checkpoint` action handler) with:

```text
   - `checkpoint`: Present `action.present_to_user` to the user.
     Mention that the Dashboard can be used to approve without terminal input.
     Then **immediately** call `mcp__forge-state__pipeline_next_action(workspace)`
     (no `user_response`, no `previous_*`). The server long-polls up to 50 s
     waiting for a Dashboard approval event.
     - If the response has `still_waiting: true`: call `pipeline_next_action(workspace)`
       again immediately (no `user_response`). Repeat until a non-checkpoint action is
       returned or the user provides a terminal response.
     - If the user types a response in the terminal (proceed / revise / abandon) while
       still_waiting is looping: on the next `pipeline_next_action` call, pass
       `user_response=<response>` instead of looping.
     - If a non-checkpoint action is returned: Dashboard approved; proceed normally.
     - **Special: `post-to-source` checkpoint** — when `action.name`
       is `"post-to-source"`:
       1. Ask the user whether to post the work report (use AskUserQuestion
          with options "post" / "skip").
       2. If the user chooses **"post"** and `action.post_method` is present:
          a. Read the body content from `action.post_method.body_source`.
          b. Post the comment using the method specified in `post_method`:
             - If `post_method.mcp_tool` is set: call it with `post_method.mcp_params`
               and the body content (pass body as the `body` parameter).
             - Else if `post_method.command` is set: execute the command via Bash.
             - Else if `post_method.instruction` is set: follow the instruction as a fallback guide.
          c. Report success or failure to the user.
       3. If the user chooses **"skip"**: do nothing.
       Pass the user's response to `pipeline_next_action(workspace, user_response=<response>)`.
     Do NOT call `checkpoint()` — `pipeline_next_action` handles the checkpoint
     state transition internally.
     On every `pipeline_next_action` call for a checkpoint (still_waiting loops and
     terminal-response call alike), omit `previous_action_complete` (or pass false),
     and pass `previous_tokens=0, previous_duration_ms=0` with no `previous_model`
     or `previous_setup_only`
     (checkpoints have no agent cost; omitting `previous_action_complete` causes the P5 block to be skipped).
```

- [ ] **Step 2: Update the Rules section reference to long-poll duration**

In `skills/forge/SKILL.md`, line 186, update "This is the Dashboard long-poll loop" to reference 50s:

```text
- When `still_waiting: true` is returned: call `pipeline_next_action(workspace)` again immediately with no `previous_*` or `user_response`. This is the Dashboard long-poll loop (50 s per iteration) — keep calling until a non-still_waiting response arrives or the user types a terminal response.
```

- [ ] **Step 3: Commit**

```bash
git -C /Users/hiroki.yasui/go/src/github.com/legalforce/worktree2/dealon-app/poc/claude-forge add skills/forge/SKILL.md
git -C /Users/hiroki.yasui/go/src/github.com/legalforce/worktree2/dealon-app/poc/claude-forge commit -m "feat(skill): remove checkpoint() call, simplify checkpoint handling in SKILL.md"
```

---

### Task 5: Update pipeline rule in `.claude/rules/pipeline.md`

**Files:**

- Modify: `.claude/rules/pipeline.md`

- [ ] **Step 1: Update the ownership table**

In `.claude/rules/pipeline.md`, update the ownership table to reflect that `pipeline_next_action` now owns the checkpoint state transition:

Find the table:

```markdown
| Responsibility | Owner |
|---|---|
| Phase start (`pending -> in_progress`) | `pipeline_next_action` |
| Phase complete (`in_progress -> completed`) | `pipeline_report_result` via `determineTransition()` |
| Checkpoint (`in_progress -> awaiting_human`) | `pipeline_next_action` (checkpoint action) |
| Checkpoint resolution | `pipeline_next_action` (with `user_response`) |
```

Update the Checkpoint row comment:

```markdown
| Responsibility | Owner |
|---|---|
| Phase start (`pending -> in_progress`) | `pipeline_next_action` |
| Phase complete (`in_progress -> completed`) | `pipeline_report_result` via `determineTransition()` |
| Checkpoint (`in_progress -> awaiting_human`) | `pipeline_next_action` (absorbed; calls `sm.Checkpoint()` internally) |
| Checkpoint resolution | `pipeline_next_action` (with `user_response`) |
```

- [ ] **Step 2: Commit**

```bash
git -C /Users/hiroki.yasui/go/src/github.com/legalforce/worktree2/dealon-app/poc/claude-forge add .claude/rules/pipeline.md
git -C /Users/hiroki.yasui/go/src/github.com/legalforce/worktree2/dealon-app/poc/claude-forge commit -m "docs(rules): update pipeline ownership table for checkpoint absorption"
```

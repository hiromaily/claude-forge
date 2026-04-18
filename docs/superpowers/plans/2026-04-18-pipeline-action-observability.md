# Pipeline Action Observability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix silent action skipping bugs by adding observability to internally-absorbed pipeline actions, and strengthen e2e tests to verify complete action sequences instead of only final state.

**Architecture:** Add PhaseLog entries for P1 skip absorptions. Add artifact validation guards for P2-P4 internal absorptions. Enhance e2e tests to record and assert full action sequences, detect gaps, and verify skipped-phase completeness per template.

**Tech Stack:** Go 1.26, stdlib testing

---

### Task 1: Add PhaseLog entries for P1 skip absorptions

**Files:**
- Modify: `poc/claude-forge/mcp-server/internal/handler/tools/pipeline_next_action.go`
- Modify: `poc/claude-forge/mcp-server/internal/handler/tools/pipeline_next_action_test.go`

The P1 skip loop calls `PhaseCompleteSkipped` but never writes a PhaseLog entry. This means skipped phases appear in `CompletedPhases` but not in `PhaseLog`, violating the accounting invariant and making skip-related bugs invisible.

- [ ] **Step 1: Write the test**

In `pipeline_next_action_test.go`, add a test that verifies skipped phases get PhaseLog entries:

```go
func TestPipelineNextAction_SkipPhaseLogged(t *testing.T) {
	t.Parallel()

	// Set up a workspace at checkpoint-a with checkpoint-a in SkippedPhases.
	// When pipeline_next_action absorbs the skip, it should write a PhaseLog entry.
	dir := t.TempDir()
	sm := state.NewStateManager("dev")
	if err := sm.Init(dir, "test-skip-log"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := sm.Configure(dir, state.PipelineConfig{
		Effort:        state.EffortS,
		FlowTemplate:  state.TemplateLight,
		SkippedPhases: []string{"checkpoint-a"},
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	// Advance to checkpoint-a by completing preceding phases.
	for _, phase := range []string{state.PhaseOne, state.PhaseTwo, state.PhaseThree, state.PhaseThreeB} {
		if err := sm.PhaseStart(dir, phase); err != nil {
			t.Fatalf("PhaseStart(%s): %v", phase, err)
		}
		if err := sm.PhaseComplete(dir, phase); err != nil {
			t.Fatalf("PhaseComplete(%s): %v", phase, err)
		}
	}

	eng := orchestrator.NewEngine("", "")
	bus := events.NewEventBus()
	kb := history.NewKnowledgeBase("")
	h := PipelineNextActionHandler(sm, bus, eng, "", nil, kb, nil)

	res := callTool(t, h, map[string]any{
		"workspace": dir,
	})
	if res.IsError {
		t.Fatalf("handler returned MCP error: %v", textContent(res))
	}

	// Verify checkpoint-a appears in PhaseLog with model "skipped".
	st, err := sm.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	found := false
	for _, entry := range st.PhaseLog {
		if entry.Phase == "checkpoint-a" && entry.Model == "skipped" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("PhaseLog should contain checkpoint-a with model=skipped, got %v", st.PhaseLog)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd poc/claude-forge/mcp-server && go test ./internal/handler/tools/... -count=1 -run TestPipelineNextAction_SkipPhaseLogged`
Expected: FAIL (no PhaseLog entry for skipped phase)

- [ ] **Step 3: Add PhaseLog entry in P1 skip loop**

In `pipeline_next_action.go`, after the `PhaseCompleteSkipped` call (line 279), add a PhaseLog entry:

```go
if skipErr := sm2.PhaseCompleteSkipped(workspace, skipPhase); skipErr != nil {
	return errorf("skip phase_complete %s: %v", skipPhase, skipErr)
}
// Record the skip in PhaseLog for observability.
if logErr := sm2.PhaseLog(workspace, state.PhaseLogEntry{
	Phase:    skipPhase,
	Tokens:   0,
	Duration: 0,
	Model:    "skipped",
}); logErr != nil {
	// Non-fatal: log warning but continue.
	appendWarning(fmt.Sprintf("skip phase-log %s: %v", skipPhase, logErr))
}
```

Note: `appendWarning` is defined later in the function (line 300). Move the `appendWarning` closure definition to before the P1 loop, or use a local slice to collect warnings.

Actually, `appendWarning` depends on `resp` which is created at line 297 (after P1). Instead, collect skip warnings in a slice and merge later:

```go
var skipWarnings []string

for iter := range maxDispatchIter {
	// ... existing skip logic ...
	if logErr := sm2.PhaseLog(workspace, state.PhaseLogEntry{
		Phase:    skipPhase,
		Tokens:   0,
		Duration: 0,
		Model:    "skipped",
	}); logErr != nil {
		skipWarnings = append(skipWarnings, fmt.Sprintf("skip phase-log %s: %v", skipPhase, logErr))
	}
	// ... rest of loop ...
}

resp := nextActionResponse{Action: action}
// ... appendWarning definition ...
for _, w := range skipWarnings {
	appendWarning(w)
}
```

- [ ] **Step 4: Check PhaseLog method signature**

Verify that `sm2.PhaseLog` accepts the right type. Read `state/manager.go` to confirm the method exists and its signature. If `PhaseLog` is a field on `State` (not a method), use `sm2.Update` instead:

```go
if logErr := sm2.Update(func(s *state.State) error {
	s.PhaseLog = append(s.PhaseLog, state.PhaseLogEntry{
		Phase:    skipPhase,
		Tokens:   0,
		Duration: 0,
		Model:    "skipped",
	})
	return nil
}); logErr != nil {
	skipWarnings = append(skipWarnings, fmt.Sprintf("skip phase-log %s: %v", skipPhase, logErr))
}
```

- [ ] **Step 5: Run test**

Run: `cd poc/claude-forge/mcp-server && go test ./internal/handler/tools/... -count=1 -run TestPipelineNextAction_SkipPhaseLogged`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add poc/claude-forge/mcp-server/internal/handler/tools/pipeline_next_action.go poc/claude-forge/mcp-server/internal/handler/tools/pipeline_next_action_test.go
git commit -m "fix(pipeline): add PhaseLog entries for P1 skip absorptions"
```

---

### Task 2: Strengthen e2e test — verify full action sequence

**Files:**
- Modify: `poc/claude-forge/mcp-server/internal/handler/tools/pipeline_e2e_test.go`

The current e2e tests only check `currentPhase == completed` and whether specific phases appear in `PhaseLog`. They don't verify the **complete sequence** of actions. This means missing actions are invisible.

- [ ] **Step 1: Add action recording to `runE2EPipeline`**

Modify `runE2EPipeline` to return the full list of actions seen:

Change the return type from `bool` to `([]orchestrator.Action, bool)`:

```go
func runE2EPipeline(
	t *testing.T,
	cfg e2eConfig,
	workspace string,
	nextActionH server.ToolHandlerFunc,
	reportResultH server.ToolHandlerFunc,
) (actions []orchestrator.Action, revisionCycleDetected bool) {
	t.Helper()

	approveOverride := new(bool)
	revisionCycleDetected = false

	for range 60 {
		// ... existing callNextAction code ...

		if resp.Action.Type == orchestrator.ActionDone {
			actions = append(actions, resp.Action)
			return actions, revisionCycleDetected
		}

		actions = append(actions, resp.Action)

		// ... rest of existing loop (mockAgentExecute, reportResult, etc.) ...
	}

	t.Fatalf("runE2EPipeline: pipeline did not reach ActionDone within 60 iterations")
	return nil, false
}
```

- [ ] **Step 2: Update existing callers**

All existing callers of `runE2EPipeline` currently receive `bool`. Update them to receive `([]orchestrator.Action, bool)`:

```go
// In TestE2E_Templates:
_, _ = runE2EPipeline(t, cfg, workspace, nextActionH, reportResultH)

// In TestE2E_DesignRevisionCycle:
_, revisionDetected := runE2EPipeline(t, cfg, workspace, nextActionH, reportResultH)

// In TestE2E_CheckpointRevisionFlow (if exists):
// Update similarly
```

- [ ] **Step 3: Add `TestE2E_ActionSequenceComplete`**

```go
func TestE2E_ActionSequenceComplete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		template string
		effort   string
		// wantMinActions is the minimum number of actions (spawn_agent + checkpoint + exec + write_file + done).
		// This catches silent skips: if an action is absorbed without trace, the count drops.
		wantMinActions int
	}{
		{name: "standard", template: state.TemplateStandard, effort: state.EffortM, wantMinActions: 12},
		{name: "light", template: state.TemplateLight, effort: state.EffortS, wantMinActions: 8},
		{name: "full", template: state.TemplateFull, effort: state.EffortL, wantMinActions: 14},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := e2eConfig{effort: tc.effort, template: tc.template}
			workspace, nextActionH, reportResultH := setupE2EWorkspace(t, cfg)
			actions, _ := runE2EPipeline(t, cfg, workspace, nextActionH, reportResultH)

			if len(actions) < tc.wantMinActions {
				var phases []string
				for _, a := range actions {
					phase := a.Phase
					if phase == "" {
						phase = a.Name
					}
					phases = append(phases, a.Type+":"+phase)
				}
				t.Errorf("action count = %d, want >= %d; sequence: %v",
					len(actions), tc.wantMinActions, phases)
			}

			// Verify the last action is ActionDone.
			last := actions[len(actions)-1]
			if last.Type != orchestrator.ActionDone {
				t.Errorf("last action type = %q, want %q", last.Type, orchestrator.ActionDone)
			}

			// Verify no duplicate spawn_agent phases (outside revision cycles).
			spawnPhases := make(map[string]int)
			for _, a := range actions {
				if a.Type == orchestrator.ActionSpawnAgent {
					spawnPhases[a.Phase]++
				}
			}
			for phase, count := range spawnPhases {
				// phase-3 and phase-3b can repeat in revision cycles.
				if phase != state.PhaseThree && phase != state.PhaseThreeB && count > 1 {
					t.Errorf("spawn_agent for phase %q dispatched %d times, want 1", phase, count)
				}
			}
		})
	}
}
```

- [ ] **Step 4: Add `TestE2E_SkippedPhasesInPhaseLog`**

Verify that skipped phases appear in PhaseLog with model "skipped" (after Task 1 fix):

```go
func TestE2E_SkippedPhasesInPhaseLog(t *testing.T) {
	t.Parallel()

	cfg := e2eConfig{effort: state.EffortS, template: state.TemplateLight}
	workspace, nextActionH, reportResultH := setupE2EWorkspace(t, cfg)
	_, _ = runE2EPipeline(t, cfg, workspace, nextActionH, reportResultH)

	s, err := state.ReadState(workspace)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}

	logged := phaseLogSet(s)

	// Light template skips: checkpoint-a, phase-4b, checkpoint-b, phase-6.
	// These should appear in PhaseLog with model "skipped" after Task 1 fix.
	expectedSkips := orchestrator.SkipsForTemplate(state.TemplateLight)
	for _, skip := range expectedSkips {
		if !logged[skip] {
			t.Errorf("skipped phase %q not found in PhaseLog; expected model=skipped entry", skip)
		}
	}

	// Verify ALL phases from setup to completed have a PhaseLog entry.
	// This catches the "consumed but not logged" bug.
	for _, phase := range state.AllPhases {
		if phase == state.PhaseSetup || phase == state.PhaseCompleted {
			continue // setup and completed don't have PhaseLog entries
		}
		if !logged[phase] {
			t.Errorf("phase %q has no PhaseLog entry — action was silently consumed", phase)
		}
	}
}
```

- [ ] **Step 5: Run tests**

Run: `cd poc/claude-forge/mcp-server && go test ./internal/handler/tools/... -count=1 -run TestE2E`
Expected: all pass

- [ ] **Step 6: Determine correct `wantMinActions` values**

If the `wantMinActions` values are wrong, the test will tell you the actual count. Adjust the values to match the observed action counts (these are minimums, so set them to the actual count minus 1 for safety margin).

- [ ] **Step 7: Commit**

```bash
git add poc/claude-forge/mcp-server/internal/handler/tools/pipeline_e2e_test.go
git commit -m "test(e2e): verify full action sequences and skipped-phase PhaseLog entries"
```

---

### Task 3: Add artifact validation guards for P2 internal absorptions

**Files:**
- Modify: `poc/claude-forge/mcp-server/internal/handler/tools/pipeline_next_action.go`
- Modify: `poc/claude-forge/mcp-server/internal/handler/tools/pipeline_next_action_test.go`

P2 (task_init) and P3 (batch_commit) absorptions execute internally but don't validate that their artifacts were written. If `executeTaskInit` silently fails (e.g., malformed tasks.md), the pipeline continues with missing state.

- [ ] **Step 1: Write the test**

```go
func TestPipelineNextAction_TaskInitArtifactValidation(t *testing.T) {
	t.Parallel()

	// Set up a workspace at phase-5 with Tasks empty.
	// pipeline_next_action should execute task_init internally.
	// After task_init, Tasks should be populated in state.
	dir := t.TempDir()
	sm := state.NewStateManager("dev")
	if err := sm.Init(dir, "test-task-init"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := sm.Configure(dir, state.PipelineConfig{
		Effort:       state.EffortM,
		FlowTemplate: state.TemplateStandard,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	// Write tasks.md with a valid task so task_init can parse it.
	tasksContent := "# Tasks\n\n## Task 1: Implement\n\nDo the thing.\n\nmode: sequential\n"
	if err := os.WriteFile(filepath.Join(dir, state.ArtifactTasks), []byte(tasksContent), 0o600); err != nil {
		t.Fatalf("write tasks.md: %v", err)
	}

	// Advance to phase-5.
	for _, phase := range []string{
		state.PhaseOne, state.PhaseTwo, state.PhaseThree, state.PhaseThreeB,
		state.PhaseCheckpointA, state.PhaseFour, state.PhaseFourB, state.PhaseCheckpointB,
	} {
		if err := sm.PhaseStart(dir, phase); err != nil {
			t.Fatalf("PhaseStart(%s): %v", phase, err)
		}
		if err := sm.PhaseComplete(dir, phase); err != nil {
			t.Fatalf("PhaseComplete(%s): %v", phase, err)
		}
	}

	eng := orchestrator.NewEngine("", "")
	bus := events.NewEventBus()
	kb := history.NewKnowledgeBase("")
	h := PipelineNextActionHandler(sm, bus, eng, "", nil, kb, nil)

	res := callTool(t, h, map[string]any{
		"workspace": dir,
	})
	if res.IsError {
		t.Fatalf("handler returned MCP error: %v", textContent(res))
	}

	// After task_init absorption, state.Tasks should be populated.
	st, err := sm.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if len(st.Tasks) == 0 {
		t.Errorf("Tasks should be populated after task_init absorption, got empty")
	}
}
```

- [ ] **Step 2: Run test**

Run: `cd poc/claude-forge/mcp-server && go test ./internal/handler/tools/... -count=1 -run TestPipelineNextAction_TaskInitArtifactValidation`
Expected: PASS (task_init already works for valid tasks.md)

- [ ] **Step 3: Add post-task_init state validation in pipeline_next_action.go**

After the `executeTaskInit` call in the P2 block, add a validation:

```go
case orchestrator.ActionTaskInit:
	if taskErr := executeTaskInit(action.Phase, sm2); taskErr != nil {
		return errorf("task_init: %v", taskErr)
	}
	// Validate that task_init populated state.Tasks.
	if st2, stErr := sm2.GetState(); stErr == nil && len(st2.Tasks) == 0 {
		return errorf("task_init: tasks not populated after execution — tasks.md may be malformed or missing")
	}
	action, err = eng.NextAction(sm2, "")
	// ...
```

- [ ] **Step 4: Run full test suite**

Run: `cd poc/claude-forge/mcp-server && go test -race ./... -count=1`
Expected: all pass

- [ ] **Step 5: Commit**

```bash
git add poc/claude-forge/mcp-server/internal/handler/tools/pipeline_next_action.go poc/claude-forge/mcp-server/internal/handler/tools/pipeline_next_action_test.go
git commit -m "fix(pipeline): add artifact validation for P2 task_init absorption"
```

---

### Task 4: Full test suite verification

**Files:** (verification only)

- [ ] **Step 1: Run full test suite**

Run: `cd poc/claude-forge/mcp-server && go test -race ./... -count=1`
Expected: all 16 packages pass

- [ ] **Step 2: Verify e2e tests catch skip gaps**

Run: `cd poc/claude-forge/mcp-server && go test ./internal/handler/tools/... -count=1 -run TestE2E -v 2>&1 | head -50`
Expected: all TestE2E tests pass, with action counts visible

- [ ] **Step 3: Verify PhaseLog completeness**

Run: `cd poc/claude-forge/mcp-server && go test ./internal/handler/tools/... -count=1 -run TestE2E_SkippedPhasesInPhaseLog -v`
Expected: PASS (all phases including skipped ones have PhaseLog entries)

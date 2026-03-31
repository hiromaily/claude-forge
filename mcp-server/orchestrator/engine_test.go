package orchestrator

import (
	"reflect"
	"slices"
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/state"
)

func boolPtr(b bool) *bool { return &b }

// newTestStateManager creates a StateManager loaded from a temporary directory
// with an initialised state.json.
func newTestStateManager(t *testing.T, phase string, modify func(*state.State) error) *state.StateManager {
	t.Helper()

	dir := t.TempDir()
	sm := state.NewStateManager()

	if err := sm.Init(dir, "test-spec"); err != nil {
		t.Fatalf("sm.Init: %v", err)
	}

	// Apply modifier if provided
	if modify != nil {
		if err := sm.Update(modify); err != nil {
			t.Fatalf("sm.Update (modifier): %v", err)
		}
	}

	// Advance to the desired phase
	if phase != "" && phase != "phase-1" {
		// Set the current phase directly via update
		if err := sm.Update(func(s *state.State) error {
			s.CurrentPhase = phase
			return nil
		}); err != nil {
			t.Fatalf("sm.Update (set phase): %v", err)
		}
	}

	return sm
}

// stubVerdictReader returns a verdictReader stub that always returns the given verdict.
func stubVerdictReader(v Verdict) func(string) (Verdict, []Finding, error) {
	return func(_ string) (Verdict, []Finding, error) {
		return v, nil, nil
	}
}

// stubSourceTypeReader returns a sourceTypeReader stub that always returns the given type.
func stubSourceTypeReader(st string) func(string) string {
	return func(_ string) string {
		return st
	}
}

// TestNextAction_Idempotency verifies that calling NextAction twice on the same
// StateManager with identical inputs produces identical results (no mutation).
func TestNextAction_Idempotency(t *testing.T) {
	t.Parallel()

	sm := newTestStateManager(t, "phase-1", nil)

	eng := &Engine{
		agentDir:         "/test/agents",
		specsDir:         "/test/specs",
		verdictReader:    stubVerdictReader(VerdictApprove),
		sourceTypeReader: stubSourceTypeReader("text"),
	}

	action1, err1 := eng.NextAction(sm, "")
	if err1 != nil {
		t.Fatalf("first NextAction: %v", err1)
	}

	action2, err2 := eng.NextAction(sm, "")
	if err2 != nil {
		t.Fatalf("second NextAction: %v", err2)
	}

	if !reflect.DeepEqual(action1, action2) {
		t.Errorf("NextAction is not idempotent:\nfirst  = %+v\nsecond = %+v", action1, action2)
	}
}

// TestNewEngine_StoresAgentDir verifies that NewEngine stores agentDir correctly.
func TestNewEngine_StoresAgentDir(t *testing.T) {
	t.Parallel()

	eng := NewEngine("/path/to/agents", "/path/to/specs")
	if eng.agentDir != "/path/to/agents" {
		t.Errorf("agentDir = %q, want %q", eng.agentDir, "/path/to/agents")
	}
	if eng.specsDir != "/path/to/specs" {
		t.Errorf("specsDir = %q, want %q", eng.specsDir, "/path/to/specs")
	}
}

// TestReadSourceType groups tests for the readSourceType helper.
func TestReadSourceType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T) string // returns workspace dir
		want  string
	}{
		{
			name: "present",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				content := "---\nsource_type: github_issue\nsource_url: https://example.com\n---\n\n# Title\n"
				if err := writeFileForTest(dir+"/request.md", content); err != nil {
					t.Fatalf("writeFileForTest: %v", err)
				}
				return dir
			},
			want: "github_issue",
		},
		{
			name: "absent",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				content := "---\nsource_url: https://example.com\n---\n\n# Title\n"
				if err := writeFileForTest(dir+"/request.md", content); err != nil {
					t.Fatalf("writeFileForTest: %v", err)
				}
				return dir
			},
			want: "text",
		},
		{
			name: "unreadable",
			setup: func(t *testing.T) string {
				t.Helper()
				return "/nonexistent/path/that/cannot/exist"
			},
			want: "text",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := tc.setup(t)
			got := readSourceType(dir)
			if got != tc.want {
				t.Errorf("readSourceType(%q) = %q, want %q", dir, got, tc.want)
			}
		})
	}
}

// TestSortedTaskKeys groups tests for the sortedTaskKeys helper.
func TestSortedTaskKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		tasks map[string]state.Task
		want  []string
	}{
		{
			name: "numeric_order",
			tasks: map[string]state.Task{
				"3": {Title: "third"},
				"1": {Title: "first"},
				"2": {Title: "second"},
			},
			want: []string{"1", "2", "3"},
		},
		{
			name:  "empty_map",
			tasks: map[string]state.Task{},
			want:  []string{},
		},
		{
			name: "mixed_keys",
			tasks: map[string]state.Task{
				"alpha": {Title: "a"},
				"2":     {Title: "two"},
				"1":     {Title: "one"},
			},
			want: []string{"1", "2", "alpha"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := sortedTaskKeys(tc.tasks)
			if tc.name == "empty_map" {
				if len(got) != 0 {
					t.Errorf("sortedTaskKeys(empty) = %v, want []", got)
				}
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("sortedTaskKeys = %v, want %v", got, tc.want)
			}
		})
	}
}

// nextActionTestCase describes a single subtest row in TestNextAction.
type nextActionTestCase struct {
	name string
	// setupSM builds the state manager; receives t so it can call TempDir/writeFileForTest.
	setupSM func(t *testing.T) *state.StateManager
	// eng overrides; if nil, a default stub engine (approve + text) is used.
	engFn func() *Engine
	// assertions on the returned action
	wantErr           bool     // true: expect non-nil error from NextAction
	wantType          string
	wantAgent         string
	wantSummary       string
	wantPhase         string   // non-empty: assert action.Phase equals this value
	wantParallelIDs   []string // nil means "do not check"; empty slice means "assert len==0"
	wantInputContains string   // non-empty: assert InputFiles contains this value
	wantSetupOnly     *bool    // non-nil: assert action.SetupOnly equals *wantSetupOnly
}

// defaultEng returns an Engine with stubbed readers (approve + text).
func defaultEng() *Engine {
	return &Engine{
		agentDir:         "/test/agents",
		specsDir:         "/test/specs",
		verdictReader:    stubVerdictReader(VerdictApprove),
		sourceTypeReader: stubSourceTypeReader("text"),
	}
}

// TestNextAction is the consolidated table-driven test for Engine.NextAction.
// It covers all 26+ decision branches defined in engine.go.
//
//nolint:maintidx // table test for 26+ branches; length is inherent
func TestNextAction(t *testing.T) {
	t.Parallel()

	tests := []nextActionTestCase{
		// ── Decision 14: Phase skip gate ─────────────────────────────────────
		{
			name: "skip_gate",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "phase-2", func(s *state.State) error {
					s.SkippedPhases = []string{"phase-2"}
					return nil
				})
			},
			wantType:    ActionDone,
			wantSummary: SkipSummaryPrefix + "phase-2",
		},

		// ── Decision 15: lite flow template ──────────────────────────────────
		{
			name: "lite_flow_template",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				lite := TemplateLite
				return newTestStateManager(t, "phase-1", func(s *state.State) error {
					s.FlowTemplate = &lite
					return nil
				})
			},
			wantType:  ActionSpawnAgent,
			wantAgent: agentAnalyst,
		},

		// ── Decision 15: standard flow template ──────────────────────────────
		{
			name: "standard_flow_template",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "phase-1", nil)
			},
			wantType:  ActionSpawnAgent,
			wantAgent: agentSituationAnalyst,
		},

		// ── Decision 16: docs stub synthesis (uses TempDir for disk I/O) ──────
		{
			name: "docs_stub_synthesis",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				docs := TaskTypeDocs
				return newTestStateManager(t, "phase-1", func(s *state.State) error {
					s.TaskType = &docs
					s.CompletedPhases = []string{"setup", PhaseOne}
					return nil
				})
			},
			wantType:  ActionWriteFile,
			wantPhase: PhaseOne,
		},

		// ── Decision 17: bugfix stub synthesis (uses TempDir for disk I/O) ───
		{
			name: "bugfix_stub_synthesis",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				bugfix := TaskTypeBugfix
				return newTestStateManager(t, "phase-3", func(s *state.State) error {
					s.TaskType = &bugfix
					s.CompletedPhases = []string{"setup", PhaseOne, PhaseTwo, PhaseThree}
					return nil
				})
			},
			wantType:  ActionWriteFile,
			wantPhase: PhaseThree,
		},

		// ── Decision 16: docs stub synthesis exec step (both stubs present, tasks empty) ──
		{
			name: "docs_stub_synthesis_exec",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				docs := TaskTypeDocs
				sm := newTestStateManager(t, "phase-1", func(s *state.State) error {
					s.TaskType = &docs
					s.CompletedPhases = []string{"setup", PhaseOne}
					return nil
				})
				st, err := sm.GetState()
				if err != nil {
					t.Fatalf("GetState: %v", err)
				}
				// Write both stub files so synthesis advances to exec step
				if err := writeFileForTest(st.Workspace+"/design.md", "# Design\n"); err != nil {
					t.Fatalf("writeFileForTest design.md: %v", err)
				}
				if err := writeFileForTest(st.Workspace+"/tasks.md", "## Task 1\n"); err != nil {
					t.Fatalf("writeFileForTest tasks.md: %v", err)
				}
				return sm
			},
			wantType:      ActionExec,
			wantPhase:     PhaseOne,
			wantSetupOnly: boolPtr(false),
		},

		// ── Decision 17: bugfix stub synthesis exec step (both stubs present, tasks empty) ──
		{
			name: "bugfix_stub_synthesis_exec",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				bugfix := TaskTypeBugfix
				sm := newTestStateManager(t, "phase-3", func(s *state.State) error {
					s.TaskType = &bugfix
					s.CompletedPhases = []string{"setup", PhaseOne, PhaseTwo, PhaseThree}
					return nil
				})
				st, err := sm.GetState()
				if err != nil {
					t.Fatalf("GetState: %v", err)
				}
				// Write both stub files so synthesis advances to exec step
				if err := writeFileForTest(st.Workspace+"/design.md", "# Design\n"); err != nil {
					t.Fatalf("writeFileForTest design.md: %v", err)
				}
				if err := writeFileForTest(st.Workspace+"/tasks.md", "## Task 1\n"); err != nil {
					t.Fatalf("writeFileForTest tasks.md: %v", err)
				}
				return sm
			},
			wantType:      ActionExec,
			wantPhase:     PhaseThree,
			wantSetupOnly: boolPtr(false),
		},

		// ── Phase 2: investigator ─────────────────────────────────────────────
		{
			name: "phase2_investigator",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "phase-2", nil)
			},
			wantType:  ActionSpawnAgent,
			wantAgent: agentInvestigator,
		},

		// ── Phase 3: architect ────────────────────────────────────────────────
		{
			name: "phase3_architect",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "phase-3", nil)
			},
			wantType:  ActionSpawnAgent,
			wantAgent: agentArchitect,
		},

		// ── Decision 18: design review — REVISE verdict re-spawns architect ──
		{
			name: "design_review_revise",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				sm := newTestStateManager(t, "phase-3b", nil)
				st, err := sm.GetState()
				if err != nil {
					t.Fatalf("GetState: %v", err)
				}
				if err := writeFileForTest(st.Workspace+"/review-design.md", "## Verdict: REVISE\n"); err != nil {
					t.Fatalf("writeFileForTest: %v", err)
				}
				return sm
			},
			engFn: func() *Engine {
				return &Engine{
					agentDir:         "/test/agents",
					specsDir:         "/test/specs",
					verdictReader:    stubVerdictReader(VerdictRevise),
					sourceTypeReader: stubSourceTypeReader("text"),
				}
			},
			wantType:  ActionSpawnAgent,
			wantAgent: agentArchitect,
		},

		// ── Decision 18: design review — APPROVE triggers checkpoint ─────────
		{
			name: "design_review_approve_checkpoint",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				sm := newTestStateManager(t, "phase-3b", nil)
				st, err := sm.GetState()
				if err != nil {
					t.Fatalf("GetState: %v", err)
				}
				if err := writeFileForTest(st.Workspace+"/review-design.md", "## Verdict: APPROVE\n"); err != nil {
					t.Fatalf("writeFileForTest: %v", err)
				}
				return sm
			},
			engFn: func() *Engine {
				return &Engine{
					agentDir:         "/test/agents",
					specsDir:         "/test/specs",
					verdictReader:    stubVerdictReader(VerdictApprove),
					sourceTypeReader: stubSourceTypeReader("text"),
				}
			},
			wantType: ActionCheckpoint,
		},

		// ── Decision 20: auto-approve design review skips checkpoint ──────────
		{
			name: "auto_approve_design_review",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				sm := newTestStateManager(t, "phase-3b", func(s *state.State) error {
					s.AutoApprove = true
					return nil
				})
				st, err := sm.GetState()
				if err != nil {
					t.Fatalf("GetState: %v", err)
				}
				if err := writeFileForTest(st.Workspace+"/review-design.md", "## Verdict: APPROVE\n"); err != nil {
					t.Fatalf("writeFileForTest: %v", err)
				}
				return sm
			},
			engFn: func() *Engine {
				return &Engine{
					agentDir:         "/test/agents",
					specsDir:         "/test/specs",
					verdictReader:    stubVerdictReader(VerdictApprove),
					sourceTypeReader: stubSourceTypeReader("text"),
				}
			},
			wantType:  ActionSpawnAgent,
			wantAgent: agentTaskDecomposer,
		},

		// ── Decision 21: design retry limit escalates to human ────────────────
		{
			name: "design_retry_limit",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				sm := newTestStateManager(t, "phase-3b", func(s *state.State) error {
					s.Revisions.DesignRevisions = 2
					return nil
				})
				st, err := sm.GetState()
				if err != nil {
					t.Fatalf("GetState: %v", err)
				}
				if err := writeFileForTest(st.Workspace+"/review-design.md", "## Verdict: REVISE\n"); err != nil {
					t.Fatalf("writeFileForTest: %v", err)
				}
				return sm
			},
			engFn: func() *Engine {
				return &Engine{
					agentDir:         "/test/agents",
					specsDir:         "/test/specs",
					verdictReader:    stubVerdictReader(VerdictRevise),
					sourceTypeReader: stubSourceTypeReader("text"),
				}
			},
			wantType: ActionCheckpoint,
		},

		// ── Checkpoint-A ─────────────────────────────────────────────────────
		{
			name: "checkpoint_a",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "checkpoint-a", nil)
			},
			wantType: ActionCheckpoint,
		},

		// ── Phase 4: task decomposer ──────────────────────────────────────────
		{
			name: "phase4_task_decomposer",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "phase-4", nil)
			},
			wantType:  ActionSpawnAgent,
			wantAgent: agentTaskDecomposer,
		},

		// ── Decision 19: task review — REVISE verdict re-spawns task decomposer
		{
			name: "task_review_revise",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				sm := newTestStateManager(t, "phase-4b", nil)
				st, err := sm.GetState()
				if err != nil {
					t.Fatalf("GetState: %v", err)
				}
				if err := writeFileForTest(st.Workspace+"/review-tasks.md", "## Verdict: REVISE\n"); err != nil {
					t.Fatalf("writeFileForTest: %v", err)
				}
				return sm
			},
			engFn: func() *Engine {
				return &Engine{
					agentDir:         "/test/agents",
					specsDir:         "/test/specs",
					verdictReader:    stubVerdictReader(VerdictRevise),
					sourceTypeReader: stubSourceTypeReader("text"),
				}
			},
			wantType:  ActionSpawnAgent,
			wantAgent: agentTaskDecomposer,
		},

		// ── Decision 20: auto-approve task review skips checkpoint ────────────
		{
			name: "auto_approve_task_review",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				sm := newTestStateManager(t, "phase-4b", func(s *state.State) error {
					s.AutoApprove = true
					return nil
				})
				st, err := sm.GetState()
				if err != nil {
					t.Fatalf("GetState: %v", err)
				}
				if err := writeFileForTest(st.Workspace+"/review-tasks.md", "## Verdict: APPROVE\n"); err != nil {
					t.Fatalf("writeFileForTest: %v", err)
				}
				return sm
			},
			engFn: func() *Engine {
				return &Engine{
					agentDir:         "/test/agents",
					specsDir:         "/test/specs",
					verdictReader:    stubVerdictReader(VerdictApprove),
					sourceTypeReader: stubSourceTypeReader("text"),
				}
			},
			wantType:  ActionSpawnAgent,
			wantAgent: agentImplementer,
		},

		// ── Decision 21: task retry limit escalates to human ─────────────────
		{
			name: "task_retry_limit",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				sm := newTestStateManager(t, "phase-4b", func(s *state.State) error {
					s.Revisions.TaskRevisions = 2
					return nil
				})
				st, err := sm.GetState()
				if err != nil {
					t.Fatalf("GetState: %v", err)
				}
				if err := writeFileForTest(st.Workspace+"/review-tasks.md", "## Verdict: REVISE\n"); err != nil {
					t.Fatalf("writeFileForTest: %v", err)
				}
				return sm
			},
			engFn: func() *Engine {
				return &Engine{
					agentDir:         "/test/agents",
					specsDir:         "/test/specs",
					verdictReader:    stubVerdictReader(VerdictRevise),
					sourceTypeReader: stubSourceTypeReader("text"),
				}
			},
			wantType: ActionCheckpoint,
		},

		// ── Checkpoint-B ─────────────────────────────────────────────────────
		{
			name: "checkpoint_b",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "checkpoint-b", nil)
			},
			wantType: ActionCheckpoint,
		},

		// ── Decision 27: phase-5 task_init setup — empty tasks emits setup exec ──
		{
			name: "phase5_task_init_setup",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "phase-5", nil) // no tasks set
			},
			wantType:      ActionExec,
			wantSetupOnly: boolPtr(true),
		},

		// ── Decision 28: phase-5 branch creation setup — no branch emits setup exec ──
		{
			name: "phase5_branch_setup",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "phase-5", func(s *state.State) error {
					s.Tasks = map[string]state.Task{
						"1": {Title: "Task 1", ExecutionMode: "sequential", ImplStatus: "pending"},
					}
					// Branch is nil and UseCurrentBranch is false (default)
					return nil
				})
			},
			wantType:      ActionExec,
			wantSetupOnly: boolPtr(true),
		},

		// ── Decision 28 bypass: UseCurrentBranch=true skips branch setup ──
		{
			name: "phase5_use_current_branch",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "phase-5", func(s *state.State) error {
					s.UseCurrentBranch = true
					s.Tasks = map[string]state.Task{
						"1": {Title: "Task 1", ExecutionMode: "sequential", ImplStatus: "pending"},
					}
					return nil
				})
			},
			wantType:      ActionSpawnAgent,
			wantSetupOnly: boolPtr(false),
		},

		// ── Decision 22: phase-5 sequential task — ParallelTaskIDs empty ─────
		{
			name: "phase5_sequential",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "phase-5", func(s *state.State) error {
					s.UseCurrentBranch = true
					s.Tasks = map[string]state.Task{
						"1": {Title: "Task 1", ExecutionMode: "sequential", ImplStatus: "pending"},
						"2": {Title: "Task 2", ExecutionMode: "sequential", ImplStatus: "pending"},
					}
					return nil
				})
			},
			wantType:        ActionSpawnAgent,
			wantParallelIDs: []string{}, // assert len == 0
		},

		// ── Decision 22: phase-5 parallel group — ParallelTaskIDs == ["1","2"]
		{
			name: "phase5_parallel_two",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "phase-5", func(s *state.State) error {
					s.UseCurrentBranch = true
					s.Tasks = map[string]state.Task{
						"1": {Title: "Task 1", ExecutionMode: "parallel", ImplStatus: "pending"},
						"2": {Title: "Task 2", ExecutionMode: "parallel", ImplStatus: "pending"},
					}
					return nil
				})
			},
			wantType:        ActionSpawnAgent,
			wantParallelIDs: []string{"1", "2"},
		},

		// ── Decision 22: phase-5 single parallel task dispatches via ParallelSpawnAction ──
		{
			name: "phase5_parallel_one",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "phase-5", func(s *state.State) error {
					s.UseCurrentBranch = true
					s.Tasks = map[string]state.Task{
						"1": {Title: "Task 1", ExecutionMode: "parallel", ImplStatus: "pending"},
						"2": {Title: "Task 2", ExecutionMode: "sequential", ImplStatus: "pending"},
					}
					return nil
				})
			},
			wantType:        ActionSpawnAgent,
			wantParallelIDs: []string{"1"},
		},

		// ── Decision 22: phase-5 three parallel tasks ────────────────────────
		{
			name: "phase5_parallel_three",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "phase-5", func(s *state.State) error {
					s.UseCurrentBranch = true
					s.Tasks = map[string]state.Task{
						"1": {Title: "Task 1", ExecutionMode: "parallel", ImplStatus: "pending"},
						"2": {Title: "Task 2", ExecutionMode: "parallel", ImplStatus: "pending"},
						"3": {Title: "Task 3", ExecutionMode: "parallel", ImplStatus: "pending"},
					}
					return nil
				})
			},
			wantType:        ActionSpawnAgent,
			wantParallelIDs: []string{"1", "2", "3"},
		},

		// ── Decision 23: phase-6 FAIL verdict retries implementation ──────────
		{
			name: "phase6_impl_fail",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				sm := newTestStateManager(t, "phase-6", func(s *state.State) error {
					s.Tasks = map[string]state.Task{
						"1": {Title: "Task 1", ExecutionMode: "sequential", ImplStatus: "completed", ReviewStatus: ""},
					}
					return nil
				})
				st, err := sm.GetState()
				if err != nil {
					t.Fatalf("GetState: %v", err)
				}
				if err := writeFileForTest(st.Workspace+"/review-1.md", "## Verdict: FAIL\n"); err != nil {
					t.Fatalf("writeFileForTest: %v", err)
				}
				return sm
			},
			engFn: func() *Engine {
				return &Engine{
					agentDir:         "/test/agents",
					specsDir:         "/test/specs",
					verdictReader:    stubVerdictReader(VerdictFail),
					sourceTypeReader: stubSourceTypeReader("text"),
				}
			},
			wantType:  ActionSpawnAgent,
			wantAgent: agentImplementer,
		},

		// ── Decision 23: phase-6 retry limit escalates to human ───────────────
		{
			name: "phase6_impl_retry_limit",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				sm := newTestStateManager(t, "phase-6", func(s *state.State) error {
					s.Tasks = map[string]state.Task{
						"1": {
							Title:         "Task 1",
							ExecutionMode: "sequential",
							ImplStatus:    "completed",
							ReviewStatus:  "",
							ImplRetries:   2,
						},
					}
					return nil
				})
				st, err := sm.GetState()
				if err != nil {
					t.Fatalf("GetState: %v", err)
				}
				if err := writeFileForTest(st.Workspace+"/review-1.md", "## Verdict: FAIL\n"); err != nil {
					t.Fatalf("writeFileForTest: %v", err)
				}
				return sm
			},
			engFn: func() *Engine {
				return &Engine{
					agentDir:         "/test/agents",
					specsDir:         "/test/specs",
					verdictReader:    stubVerdictReader(VerdictFail),
					sourceTypeReader: stubSourceTypeReader("text"),
				}
			},
			wantType: ActionCheckpoint,
		},

		// ── Phase 7: comprehensive reviewer ──────────────────────────────────
		{
			name: "phase7_comprehensive_reviewer",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "phase-7", nil)
			},
			wantType:  ActionSpawnAgent,
			wantAgent: agentComprehensiveReview,
		},

		// ── Final verification ────────────────────────────────────────────────
		{
			name: "final_verification",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "final-verification", nil)
			},
			wantType:  ActionSpawnAgent,
			wantAgent: agentVerifier,
		},

		// ── Decision 24: pr-creation exec (SkipPr == false) ─────────────────
		{
			name: "pr_creation_exec",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "pr-creation", nil)
			},
			wantType:  ActionExec,
			wantPhase: PhasePRCreation,
		},

		// ── Decision 24: SkipPr flag skips pr-creation ────────────────────────
		{
			name: "skip_pr",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "pr-creation", func(s *state.State) error {
					s.SkipPr = true
					return nil
				})
			},
			wantType:    ActionDone,
			wantSummary: SkipSummaryPrefix + "pr-creation",
		},

		// ── Decision 25: final-summary for investigation ─────────────────────
		{
			name: "final_summary_investigation",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				investigation := TaskTypeInvestigation
				return newTestStateManager(t, "final-summary", func(s *state.State) error {
					s.TaskType = &investigation
					return nil
				})
			},
			wantType:          ActionSpawnAgent,
			wantInputContains: "investigation.md",
		},

		// ── Decision 25: final-summary for feature uses verifier (comprehensive review already done in phase-7)
		{
			name: "final_summary_feature",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				feature := TaskTypeFeature
				return newTestStateManager(t, "final-summary", func(s *state.State) error {
					s.TaskType = &feature
					return nil
				})
			},
			wantType:  ActionSpawnAgent,
			wantAgent: agentVerifier,
		},

		// ── Decision 26: post-to-source github_issue → exec ───────────────────
		{
			name: "post_to_source_github_issue",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "post-to-source", nil)
			},
			engFn: func() *Engine {
				return &Engine{
					agentDir:         "/test/agents",
					specsDir:         "/test/specs",
					verdictReader:    stubVerdictReader(VerdictApprove),
					sourceTypeReader: stubSourceTypeReader("github_issue"),
				}
			},
			wantType:  ActionExec,
			wantPhase: PhasePostToSource,
		},

		// ── Decision 26: post-to-source text → skip (done with skip prefix) ──
		{
			name: "post_to_source_text",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "post-to-source", nil)
			},
			engFn: func() *Engine {
				return &Engine{
					agentDir:         "/test/agents",
					specsDir:         "/test/specs",
					verdictReader:    stubVerdictReader(VerdictApprove),
					sourceTypeReader: stubSourceTypeReader("text"),
				}
			},
			wantType:    ActionDone,
			wantSummary: SkipSummaryPrefix + "post-to-source",
		},

		// ── Decision 26: post-to-source jira_issue → checkpoint ───────────────
		{
			name: "post_to_source_jira_issue",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "post-to-source", nil)
			},
			engFn: func() *Engine {
				return &Engine{
					agentDir:         "/test/agents",
					specsDir:         "/test/specs",
					verdictReader:    stubVerdictReader(VerdictApprove),
					sourceTypeReader: stubSourceTypeReader("jira_issue"),
				}
			},
			wantType: ActionCheckpoint,
		},

		// ── Completed phase ───────────────────────────────────────────────────
		{
			name: "completed",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "completed", nil)
			},
			wantType: ActionDone,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sm := tc.setupSM(t)

			var eng *Engine
			if tc.engFn != nil {
				eng = tc.engFn()
			} else {
				eng = defaultEng()
			}

			action, err := eng.NextAction(sm, "")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("NextAction: expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("NextAction: %v", err)
			}

			if action.Type != tc.wantType {
				t.Errorf("action.Type = %q, want %q", action.Type, tc.wantType)
			}

			if tc.wantAgent != "" && action.Agent != tc.wantAgent {
				t.Errorf("action.Agent = %q, want %q", action.Agent, tc.wantAgent)
			}

			if tc.wantSummary != "" && action.Summary != tc.wantSummary {
				t.Errorf("action.Summary = %q, want %q", action.Summary, tc.wantSummary)
			}

			if tc.wantPhase != "" && action.Phase != tc.wantPhase {
				t.Errorf("action.Phase = %q, want %q", action.Phase, tc.wantPhase)
			}

			// wantParallelIDs: nil means skip check; empty slice means assert len==0
			if tc.wantParallelIDs != nil {
				if len(tc.wantParallelIDs) == 0 {
					if len(action.ParallelTaskIDs) != 0 {
						t.Errorf("ParallelTaskIDs = %v, want empty for sequential task", action.ParallelTaskIDs)
					}
				} else {
					if !reflect.DeepEqual(action.ParallelTaskIDs, tc.wantParallelIDs) {
						t.Errorf("ParallelTaskIDs = %v, want %v", action.ParallelTaskIDs, tc.wantParallelIDs)
					}
				}
			}

			if tc.wantInputContains != "" {
				if !slices.Contains(action.InputFiles, tc.wantInputContains) {
					t.Errorf("InputFiles = %v; expected to contain %q", action.InputFiles, tc.wantInputContains)
				}
			}

			if tc.wantSetupOnly != nil {
				if action.SetupOnly != *tc.wantSetupOnly {
					t.Errorf("action.SetupOnly = %v, want %v", action.SetupOnly, *tc.wantSetupOnly)
				}
			}
		})
	}
}

func TestStripDatePrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"20260330-soa-2899-task-status", "soa-2899-task-status"},
		{"20260330-x", "x"},
		{"soa-2899", "soa-2899"},         // no date prefix
		{"1234567-foo", "1234567-foo"},    // only 7 digits
		{"12345678-foo", "foo"},           // 8 digits
		{"1234567x-foo", "1234567x-foo"}, // non-digit in prefix
		{"", ""},                          // empty string
		{"12345678", "12345678"},          // no hyphen after 8 digits
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := stripDatePrefix(tt.input)
			if got != tt.want {
				t.Errorf("stripDatePrefix(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDeriveBranchName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		specName string
		want     string
	}{
		{"20260330-soa-2899-task-status", "forge/soa-2899-task-status"},
		{"soa-2899-task-status", "forge/soa-2899-task-status"},
		{"My Feature", "forge/my-feature"},
	}

	for _, tt := range tests {
		t.Run(tt.specName, func(t *testing.T) {
			t.Parallel()
			st := &state.State{SpecName: tt.specName}
			got := deriveBranchName(st)
			if got != tt.want {
				t.Errorf("deriveBranchName(%q) = %q, want %q", tt.specName, got, tt.want)
			}
		})
	}
}

func TestDeriveBranchName_Truncation(t *testing.T) {
	t.Parallel()

	long := "20260330-soa-2899-this-is-a-very-long-specification-name-that-exceeds-sixty-characters-limit"
	got := deriveBranchName(&state.State{SpecName: long})

	// Must start with forge/ and body must be <= 60 chars.
	const prefix = "forge/"
	body := got[len(prefix):]
	if len(body) > 60 {
		t.Errorf("branch body length = %d (> 60): %q", len(body), body)
	}
	// Must not end with a hyphen (truncated at word boundary).
	if body[len(body)-1] == '-' {
		t.Errorf("branch body ends with hyphen: %q", body)
	}
}

func TestDerivePRTitle(t *testing.T) {
	t.Parallel()

	bugfix := TaskTypeBugfix
	feature := TaskTypeFeature
	docs := TaskTypeDocs

	tests := []struct {
		name     string
		taskType *string
		specName string
		want     string
	}{
		{"bugfix", &bugfix, "20260330-soa-2899-fix-status", "fix: soa 2899 fix status"},
		{"feature", &feature, "20260330-add-auth", "feat: add auth"},
		{"docs", &docs, "update-readme", "docs: update readme"},
		{"nil_type", nil, "some-task", "feat: some task"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			st := &state.State{SpecName: tt.specName, TaskType: tt.taskType}
			got := derivePRTitle(st)
			if got != tt.want {
				t.Errorf("derivePRTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

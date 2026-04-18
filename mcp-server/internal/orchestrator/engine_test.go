package orchestrator

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
)

// newTestStateManager creates a StateManager loaded from a temporary directory
// with an initialised state.json.
func newTestStateManager(t *testing.T, phase string, modify func(*state.State) error) *state.StateManager {
	t.Helper()

	dir := t.TempDir()
	sm := state.NewStateManager("dev")

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

// TestClosingRef groups tests for the ClosingRef exported helper.
func TestClosingRef(t *testing.T) {
	t.Parallel()

	writeRequest := func(t *testing.T, content string) string {
		t.Helper()
		dir := t.TempDir()
		if err := writeFileForTest(dir+"/request.md", content); err != nil {
			t.Fatalf("writeFileForTest: %v", err)
		}
		return dir
	}

	tests := []struct {
		name  string
		setup func(t *testing.T) string
		want  string
	}{
		{
			name: "github_with_id",
			setup: func(t *testing.T) string {
				t.Helper()
				return writeRequest(t, "---\nsource_type: github_issue\nsource_id: 42\n---\n")
			},
			want: "\n\nCloses #42",
		},
		{
			name: "github_no_id",
			setup: func(t *testing.T) string {
				t.Helper()
				return writeRequest(t, "---\nsource_type: github_issue\n---\n")
			},
			want: "",
		},
		{
			name: "text_source",
			setup: func(t *testing.T) string {
				t.Helper()
				return writeRequest(t, "---\nsource_type: text\nsource_id: 99\n---\n")
			},
			want: "",
		},
		{
			name: "jira_source",
			setup: func(t *testing.T) string {
				t.Helper()
				return writeRequest(t, "---\nsource_type: jira_issue\nsource_id: 10\n---\n")
			},
			want: "",
		},
		{
			name: "missing_request_md",
			setup: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := tc.setup(t)
			got := ClosingRef(dir)
			if got != tc.want {
				t.Errorf("ClosingRef(%q) = %q, want %q", dir, got, tc.want)
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
	wantErr                bool // true: expect non-nil error from NextAction
	wantType               string
	wantAgent              string
	wantSummary            string
	wantPhase              string   // non-empty: assert action.Phase equals this value
	wantParallelIDs        []string // nil means "do not check"; empty slice means "assert len==0"
	wantInputContains      string   // non-empty: assert InputFiles contains this value
	wantSetupOnly          *bool    // non-nil: assert action.SetupOnly equals *wantSetupOnly
	wantCommandContains    string   // non-empty: assert any Commands element contains this value
	wantCommandNotContains string   // non-empty: assert no Commands element contains this value
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
					s.Revisions.DesignRevisions = state.MaxRevisionRetries
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

		// ── Phase 4: task decomposer (fresh run — no review-tasks.md) ────────
		{
			name: "phase4_task_decomposer",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "phase-4", nil)
			},
			wantType:  ActionSpawnAgent,
			wantAgent: agentTaskDecomposer,
		},

		// ── Phase 4: revision run — workflow-rules wrote review-tasks.md ─────
		// The engine must re-spawn task-decomposer with review-tasks.md in
		// InputFiles so the decomposer sees the findings. Mirrors the
		// phase-4b REVISE path in handlePhaseFourB.
		{
			name: "phase4_revision_run",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				sm := newTestStateManager(t, "phase-4", func(s *state.State) error {
					s.Revisions.TaskRevisions = 1
					return nil
				})
				st, err := sm.GetState()
				if err != nil {
					t.Fatalf("GetState: %v", err)
				}
				if err := writeFileForTest(st.Workspace+"/review-tasks.md", "## Verdict: REVISE\n\n- rule proto-rule violated\n"); err != nil {
					t.Fatalf("writeFileForTest: %v", err)
				}
				return sm
			},
			wantType:          ActionSpawnAgent,
			wantAgent:         agentTaskDecomposer,
			wantInputContains: state.ArtifactReviewTasks,
		},

		// ── Phase 4: workflow-rules retry limit escalates to human ───────────
		// When TaskRevisions already hit MaxRevisionRetries, handlePhaseFour
		// emits a checkpoint named "task-workflow-rules-retry-limit" (distinct
		// from the phase-4b "task-retry-limit") so logs and analytics can tell
		// the two escalation sources apart.
		{
			name: "phase4_workflow_rules_retry_limit",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				sm := newTestStateManager(t, "phase-4", func(s *state.State) error {
					s.Revisions.TaskRevisions = state.MaxRevisionRetries
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
			wantType: ActionCheckpoint,
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
					s.Revisions.TaskRevisions = state.MaxRevisionRetries
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

		// ── Decision 27: phase-5 task_init setup — empty tasks emits ActionTaskInit ──
		{
			name: "phase5_task_init_setup",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "phase-5", nil) // no tasks set
			},
			wantType:      ActionTaskInit,
			wantSetupOnly: new(true),
		},

		// ── Decision 28 removed: branch creation now happens during init ──
		// Phase 5 with nil branch should proceed to spawn implementer (not create_branch setup).
		{
			name: "phase5_nil_branch_proceeds",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "phase-5", func(s *state.State) error {
					s.Tasks = map[string]state.Task{
						"1": {Title: "Task 1", ExecutionMode: "sequential", ImplStatus: "pending"},
					}
					// Branch is nil — no longer triggers create_branch setup.
					return nil
				})
			},
			wantType:  ActionSpawnAgent,
			wantAgent: "implementer",
		},

		// ── UseCurrentBranch=true also proceeds to implementer ──
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
			wantType:  ActionSpawnAgent,
			wantAgent: "implementer",
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

		// ── Decision 29: phase-5 batch commit — NeedsBatchCommit=true emits ActionBatchCommit ──
		{
			name: "phase5_batch_commit",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "phase-5", func(s *state.State) error {
					s.Tasks = map[string]state.Task{
						"1": {Title: "Task 1", ExecutionMode: "parallel", ImplStatus: "completed"},
					}
					s.NeedsBatchCommit = true
					return nil
				})
			},
			wantType:      ActionBatchCommit,
			wantSetupOnly: new(true),
		},

		// ── Decision 23: phase-6 FAIL verdict retries implementation ──────────
		{
			// Transitional fallback: ReviewStatus not yet set by pipeline_report_result.
			// Engine reads the review file and dispatches the retry, including the
			// review file in InputFiles so the implementer has feedback context.
			name: "phase6_impl_fail_transitional",
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
			wantType:          ActionSpawnAgent,
			wantAgent:         agentImplementer,
			wantInputContains: "review-1.md",
		},

		// ── Decision 23: phase-6 completed_fail dispatches retry (idempotent) ─
		{
			// Primary path after pipeline_report_result has set ReviewStatus.
			// Engine uses state as guard (no file read), so NextAction is idempotent.
			name: "phase6_impl_fail_completed_fail",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				sm := newTestStateManager(t, "phase-6", func(s *state.State) error {
					s.Tasks = map[string]state.Task{
						"1": {
							Title:         "Task 1",
							ExecutionMode: "sequential",
							ImplStatus:    "completed",
							ReviewStatus:  state.TaskStatusCompletedFail,
							ImplRetries:   1,
						},
					}
					return nil
				})
				st, err := sm.GetState()
				if err != nil {
					t.Fatalf("GetState: %v", err)
				}
				// review-1.md exists (not deleted) — engine must NOT re-read it.
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
			wantType:          ActionSpawnAgent,
			wantAgent:         agentImplementer,
			wantInputContains: "review-1.md",
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
							ReviewStatus:  state.TaskStatusCompletedFail,
							ImplRetries:   state.MaxRevisionRetries,
						},
					}
					return nil
				})
				return sm
			},
			wantType: ActionCheckpoint,
		},

		// ── Decision 23: phase-6 all tasks reviewed → done (phase transition) ─
		{
			// Regression guard: before fix, this path incorrectly dispatched
			// agentVerifier with "verification.md" instead of advancing to phase-7.
			name: "phase6_all_reviewed_done",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "phase-6", func(s *state.State) error {
					s.Tasks = map[string]state.Task{
						"1": {
							Title:         "Task 1",
							ExecutionMode: "sequential",
							ImplStatus:    "completed",
							ReviewStatus:  state.TaskStatusCompletedPass,
						},
						"2": {
							Title:         "Task 2",
							ExecutionMode: "sequential",
							ImplStatus:    "completed",
							ReviewStatus:  state.TaskStatusCompletedPassNote,
						},
					}
					return nil
				})
			},
			wantType:    ActionDone,
			wantSummary: SkipSummaryPrefix + PhaseSix,
		},

		// ── Decision 23: phase-6 empty Tasks skips the phase (edge case, mirrors phase-5) ─
		{
			name: "phase6_empty_tasks_skips",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "phase-6", func(s *state.State) error {
					s.Tasks = map[string]state.Task{}
					return nil
				})
			},
			wantType: ActionDone,
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

		// ── Decision 24: pr-creation emits push_branch first (BranchPushed == false) ──
		{
			name: "pr_creation_push_branch",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "pr-creation", nil) // BranchPushed defaults to false
			},
			wantType:  ActionPushBranch,
			wantPhase: PhasePRCreation,
		},

		// ── Decision 24: pr-creation exec after branch pushed (BranchPushed == true) ──
		{
			name: "pr_creation_exec",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "pr-creation", func(s *state.State) error {
					s.BranchPushed = true
					return nil
				})
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

		// ── Decision 24: github_issue source adds Closes #N to PR body ───────
		{
			name: "pr_creation_github_issue_closes",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "pr-creation", func(s *state.State) error {
					s.BranchPushed = true
					return nil
				})
			},
			engFn: func() *Engine {
				return &Engine{
					agentDir:         "/test/agents",
					specsDir:         "/test/specs",
					verdictReader:    stubVerdictReader(VerdictApprove),
					sourceTypeReader: stubSourceTypeReader(state.SourceTypeGitHub),
					sourceIDReader:   func(_ string) string { return "42" },
				}
			},
			wantType:            ActionExec,
			wantPhase:           PhasePRCreation,
			wantCommandContains: "Closes #42",
		},

		// ── Decision 24: non-github source does NOT add Closes to PR body ────
		{
			name: "pr_creation_text_source_no_closes",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "pr-creation", func(s *state.State) error {
					s.BranchPushed = true
					return nil
				})
			},
			engFn: func() *Engine {
				return &Engine{
					agentDir:         "/test/agents",
					specsDir:         "/test/specs",
					verdictReader:    stubVerdictReader(VerdictApprove),
					sourceTypeReader: stubSourceTypeReader("text"),
					sourceIDReader:   func(_ string) string { return "42" },
				}
			},
			wantType:               ActionExec,
			wantPhase:              PhasePRCreation,
			wantCommandNotContains: "Closes #",
		},

		// ── Decision 25: final-summary uses fixed input file list ───────────
		{
			name: "final_summary_fixed_inputs",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "final-summary", nil)
			},
			wantType:          ActionSpawnAgent,
			wantAgent:         agentVerifier,
			wantInputContains: "comprehensive-review.md",
		},

		// ── Decision 27: final-commit exec action ───────────────────────────
		{
			name: "final_commit_exec",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "final-commit", nil)
			},
			wantType:  ActionExec,
			wantPhase: PhaseFinalCommit,
		},

		// ── Decision 27: final-commit skipped when SkipPr ────────────────
		{
			name: "final_commit_skip_when_nopr",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "final-commit", func(s *state.State) error {
					s.SkipPr = true
					return nil
				})
			},
			wantType:    ActionDone,
			wantSummary: SkipSummaryPrefix + PhaseFinalCommit,
		},

		// ── Decision 27: final-commit skipped when pr-creation in skippedPhases ──
		{
			name: "final_commit_skip_when_pr_skipped",
			setupSM: func(t *testing.T) *state.StateManager {
				t.Helper()
				return newTestStateManager(t, "final-commit", func(s *state.State) error {
					s.SkippedPhases = append(s.SkippedPhases, PhasePRCreation)
					return nil
				})
			},
			wantType:    ActionDone,
			wantSummary: SkipSummaryPrefix + PhaseFinalCommit,
		},

		// ── Decision 26: post-to-source github_issue → checkpoint with post/skip ──
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
					sourceURLReader:  func(_ string) string { return "https://github.com/org/repo/issues/42" },
				}
			},
			wantType: ActionCheckpoint,
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

		// ── Decision 26: post-to-source jira_issue → checkpoint with post/skip options ──
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
					sourceURLReader:  func(_ string) string { return "https://example.atlassian.net/browse/PROJ-123" },
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

			if tc.wantCommandContains != "" {
				found := false
				for _, cmd := range action.Commands {
					if strings.Contains(cmd, tc.wantCommandContains) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Commands = %v; expected one element to contain %q", action.Commands, tc.wantCommandContains)
				}
			}

			if tc.wantCommandNotContains != "" {
				for _, cmd := range action.Commands {
					if strings.Contains(cmd, tc.wantCommandNotContains) {
						t.Errorf("Commands = %v; expected no element to contain %q", action.Commands, tc.wantCommandNotContains)
						break
					}
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
		{"1234567-foo", "1234567-foo"},   // only 7 digits
		{"12345678-foo", "foo"},          // 8 digits
		{"1234567x-foo", "1234567x-foo"}, // non-digit in prefix
		{"", ""},                         // empty string
		{"12345678", "12345678"},         // no hyphen after 8 digits
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
		{"20260330-soa-2899-task-status", "feature/soa-2899-task-status"},
		{"soa-2899-task-status", "feature/soa-2899-task-status"},
		{"My Feature", "feature/my-feature"},
	}

	for _, tt := range tests {
		t.Run(tt.specName, func(t *testing.T) {
			t.Parallel()
			st := &state.State{SpecName: tt.specName}
			got := DeriveBranchName(st)
			if got != tt.want {
				t.Errorf("DeriveBranchName(%q) = %q, want %q", tt.specName, got, tt.want)
			}
		})
	}
}

func TestDeriveBranchName_Truncation(t *testing.T) {
	t.Parallel()

	long := "20260330-soa-2899-this-is-a-very-long-specification-name-that-exceeds-sixty-characters-limit"
	got := DeriveBranchName(&state.State{SpecName: long})

	// Must start with feature/ and body must be <= 60 chars.
	const prefix = "feature/"
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

	tests := []struct {
		name     string
		branch   string
		specName string
		want     string
	}{
		{"feature_prefix", "feature/add-auth", "20260330-add-auth", "feat: add auth"},
		{"fix_prefix", "fix/soa-2899-fix-status", "20260330-soa-2899-fix-status", "fix: soa 2899 fix status"},
		{"refactor_prefix", "refactor/clean-up", "clean-up", "refactor: clean up"},
		{"docs_prefix", "docs/update-readme", "update-readme", "docs: update readme"},
		{"chore_prefix", "chore/bump-deps", "bump-deps", "chore: bump deps"},
		{"feature_default_prefix", "feature/some-task", "some-task", "feat: some task"},
		{"no_branch_defaults_feat", "", "some-task", "feat: some task"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var branch *string
			if tt.branch != "" {
				b := tt.branch
				branch = &b
			}
			st := &state.State{SpecName: tt.specName, Branch: branch}
			got := derivePRTitle(st)
			if got != tt.want {
				t.Errorf("derivePRTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestReadSourceURL groups tests for the readSourceURL helper.
func TestReadSourceURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T) string
		want  string
	}{
		{
			name: "present",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				content := "---\nsource_type: jira_issue\nsource_url: https://example.atlassian.net/browse/PROJ-123\nsource_id: PROJ-123\n---\n\n# Title\n"
				if err := writeFileForTest(dir+"/request.md", content); err != nil {
					t.Fatalf("writeFileForTest: %v", err)
				}
				return dir
			},
			want: "https://example.atlassian.net/browse/PROJ-123",
		},
		{
			name: "absent",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				content := "---\nsource_type: text\n---\n\n# Title\n"
				if err := writeFileForTest(dir+"/request.md", content); err != nil {
					t.Fatalf("writeFileForTest: %v", err)
				}
				return dir
			},
			want: "",
		},
		{
			name: "unreadable",
			setup: func(t *testing.T) string {
				t.Helper()
				return "/nonexistent/path/that/cannot/exist"
			},
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := tc.setup(t)
			got := readSourceURL(dir)
			if got != tc.want {
				t.Errorf("readSourceURL(%q) = %q, want %q", dir, got, tc.want)
			}
		})
	}
}

// TestPostToSource_CheckpointOptions verifies that github_issue and jira_issue
// post-to-source checkpoints return "post"/"skip" options with the source URL.
func TestPostToSource_CheckpointOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		sourceType   string
		sourceURL    string
		wantName     string
		wantURLInMsg string
	}{
		{
			name:         "jira_issue",
			sourceType:   "jira_issue",
			sourceURL:    "https://example.atlassian.net/browse/PROJ-123",
			wantName:     "post-to-source",
			wantURLInMsg: "https://example.atlassian.net/browse/PROJ-123",
		},
		{
			name:         "github_issue",
			sourceType:   "github_issue",
			sourceURL:    "https://github.com/org/repo/issues/42",
			wantName:     "post-to-source",
			wantURLInMsg: "https://github.com/org/repo/issues/42",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sm := newTestStateManager(t, "post-to-source", nil)
			eng := &Engine{
				agentDir:         "/test/agents",
				specsDir:         "/test/specs",
				verdictReader:    stubVerdictReader(VerdictApprove),
				sourceTypeReader: stubSourceTypeReader(tc.sourceType),
				sourceURLReader:  func(_ string) string { return tc.sourceURL },
			}

			action, err := eng.NextAction(sm, "")
			if err != nil {
				t.Fatalf("NextAction: %v", err)
			}

			if action.Type != ActionCheckpoint {
				t.Fatalf("action.Type = %q, want %q", action.Type, ActionCheckpoint)
			}
			if action.Name != tc.wantName {
				t.Errorf("action.Name = %q, want %q", action.Name, tc.wantName)
			}

			wantOptions := []string{"post", "skip"}
			if !slices.Equal(action.Options, wantOptions) {
				t.Errorf("action.Options = %v, want %v", action.Options, wantOptions)
			}
			if !strings.Contains(action.PresentToUser, tc.wantURLInMsg) {
				t.Errorf("action.PresentToUser does not contain URL %q: %q", tc.wantURLInMsg, action.PresentToUser)
			}
		})
	}
}

// TestHandlePhaseFour_WorkflowRulesRetryLimitCheckpoint verifies that when
// TaskRevisions has reached MaxRevisionRetries and review-tasks.md exists,
// handlePhaseFour emits a checkpoint with the workflow-rules-specific name
// and the ["approve","abandon"] option set. The distinct name lets logs and
// analytics differentiate this escalation from the phase-4b task-retry-limit.
func TestHandlePhaseFour_WorkflowRulesRetryLimitCheckpoint(t *testing.T) {
	t.Parallel()

	sm := newTestStateManager(t, "phase-4", func(s *state.State) error {
		s.Revisions.TaskRevisions = state.MaxRevisionRetries
		return nil
	})
	st, err := sm.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if err := writeFileForTest(st.Workspace+"/review-tasks.md", "## Verdict: REVISE\n"); err != nil {
		t.Fatalf("writeFileForTest: %v", err)
	}

	action, err := defaultEng().NextAction(sm, "")
	if err != nil {
		t.Fatalf("NextAction: %v", err)
	}
	if action.Type != ActionCheckpoint {
		t.Fatalf("action.Type = %q, want %q", action.Type, ActionCheckpoint)
	}
	if action.Name != "task-workflow-rules-retry-limit" {
		t.Errorf("action.Name = %q, want %q", action.Name, "task-workflow-rules-retry-limit")
	}
	wantOptions := []string{"approve", "abandon"}
	if !slices.Equal(action.Options, wantOptions) {
		t.Errorf("action.Options = %v, want %v", action.Options, wantOptions)
	}
	if !strings.Contains(action.PresentToUser, "workflow-rules") {
		t.Errorf("action.PresentToUser = %q, want substring %q", action.PresentToUser, "workflow-rules")
	}
}

// TestHandlePhaseFour_RevisionRunIncludesReviewTasks verifies that when
// review-tasks.md exists and TaskRevisions is still under the retry limit,
// handlePhaseFour re-spawns task-decomposer with both design.md and
// review-tasks.md in InputFiles so the decomposer sees the findings.
func TestHandlePhaseFour_RevisionRunIncludesReviewTasks(t *testing.T) {
	t.Parallel()

	sm := newTestStateManager(t, "phase-4", func(s *state.State) error {
		s.Revisions.TaskRevisions = 1
		return nil
	})
	st, err := sm.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if err := writeFileForTest(st.Workspace+"/review-tasks.md", "## Verdict: REVISE\n"); err != nil {
		t.Fatalf("writeFileForTest: %v", err)
	}

	action, err := defaultEng().NextAction(sm, "")
	if err != nil {
		t.Fatalf("NextAction: %v", err)
	}
	if action.Type != ActionSpawnAgent {
		t.Fatalf("action.Type = %q, want %q", action.Type, ActionSpawnAgent)
	}
	if action.Agent != agentTaskDecomposer {
		t.Errorf("action.Agent = %q, want %q", action.Agent, agentTaskDecomposer)
	}
	wantInputs := []string{state.ArtifactDesign, state.ArtifactReviewTasks}
	if !slices.Equal(action.InputFiles, wantInputs) {
		t.Errorf("action.InputFiles = %v, want %v", action.InputFiles, wantInputs)
	}
	if !strings.Contains(action.Prompt, "workflow-rules") {
		t.Errorf("action.Prompt = %q, want substring %q", action.Prompt, "workflow-rules")
	}
}

// TestHandlePhaseOne_CombinedAgent verifies that handlePhaseOne dispatches the
// combined analyst-investigator agent when PhaseTwo is in SkippedPhases, and the
// standard situation-analyst otherwise.
func TestHandlePhaseOne_CombinedAgent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		skippedPhases []string
		wantAgent     string
	}{
		{
			name:          "phase_two_skipped_uses_combined_agent",
			skippedPhases: []string{PhaseTwo},
			wantAgent:     agentAnalystInvestigator,
		},
		{
			name:          "phase_two_not_skipped_uses_situation_analyst",
			skippedPhases: []string{},
			wantAgent:     agentSituationAnalyst,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sm := newTestStateManager(t, "phase-1", func(s *state.State) error {
				s.SkippedPhases = tc.skippedPhases
				return nil
			})

			action, err := defaultEng().NextAction(sm, "")
			if err != nil {
				t.Fatalf("NextAction: %v", err)
			}
			if action.Type != ActionSpawnAgent {
				t.Fatalf("action.Type = %q, want %q", action.Type, ActionSpawnAgent)
			}
			if action.Agent != tc.wantAgent {
				t.Errorf("action.Agent = %q, want %q", action.Agent, tc.wantAgent)
			}
		})
	}
}

// TestHandlePhaseThree_InvestigationOptional verifies that handlePhaseThree includes
// investigation.md in InputFiles only when Phase 2 was not skipped.
func TestHandlePhaseThree_InvestigationOptional(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		phaseTwoSkipped    bool
		wantContainsInvest bool
	}{
		{
			name:               "investigation_absent_not_included",
			phaseTwoSkipped:    true,
			wantContainsInvest: false,
		},
		{
			name:               "investigation_present_included",
			phaseTwoSkipped:    false,
			wantContainsInvest: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sm := newTestStateManager(t, "phase-3", func(s *state.State) error {
				if tc.phaseTwoSkipped {
					s.SkippedPhases = []string{PhaseTwo}
				}
				return nil
			})

			action, err := defaultEng().NextAction(sm, "")
			if err != nil {
				t.Fatalf("NextAction: %v", err)
			}
			if action.Type != ActionSpawnAgent {
				t.Fatalf("action.Type = %q, want %q", action.Type, ActionSpawnAgent)
			}

			containsInvest := slices.Contains(action.InputFiles, state.ArtifactInvestigation)
			if containsInvest != tc.wantContainsInvest {
				t.Errorf("InputFiles contains investigation.md = %v, want %v; files = %v",
					containsInvest, tc.wantContainsInvest, action.InputFiles)
			}
		})
	}
}

// TestHandleCheckpointA_PhaseFourSkipped verifies checkpoint-a message and auto-skip
// behaviour when PhaseFour is in SkippedPhases.
func TestHandleCheckpointA_PhaseFourSkipped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		skippedPhases   []string
		autoApprove     bool
		wantType        string
		wantMsgContains string
	}{
		{
			name:            "phase_four_skipped_no_auto_approve_shows_phase5_msg",
			skippedPhases:   []string{PhaseFour},
			autoApprove:     false,
			wantType:        ActionCheckpoint,
			wantMsgContains: "Phase 5",
		},
		{
			name:            "phase_four_skipped_auto_approve_skips_checkpoint",
			skippedPhases:   []string{PhaseFour},
			autoApprove:     true,
			wantType:        ActionDone,
			wantMsgContains: "",
		},
		{
			name:            "phase_four_not_skipped_shows_phase4_msg",
			skippedPhases:   []string{},
			autoApprove:     false,
			wantType:        ActionCheckpoint,
			wantMsgContains: "Phase 4",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sm := newTestStateManager(t, "checkpoint-a", func(s *state.State) error {
				s.SkippedPhases = tc.skippedPhases
				s.AutoApprove = tc.autoApprove
				return nil
			})

			action, err := defaultEng().NextAction(sm, "")
			if err != nil {
				t.Fatalf("NextAction: %v", err)
			}
			if action.Type != tc.wantType {
				t.Errorf("action.Type = %q, want %q", action.Type, tc.wantType)
			}
			if tc.wantMsgContains != "" && !strings.Contains(action.PresentToUser, tc.wantMsgContains) {
				t.Errorf("PresentToUser = %q; expected to contain %q", action.PresentToUser, tc.wantMsgContains)
			}
			if tc.wantType == ActionDone && !strings.HasPrefix(action.Summary, SkipSummaryPrefix) {
				t.Errorf("action.Summary = %q; expected skip prefix %q", action.Summary, SkipSummaryPrefix)
			}
		})
	}
}

// TestHandlePhaseThreeB_AutoApprove_PhaseFourSkipped verifies that when AutoApprove=true,
// verdict=APPROVE, and PhaseFour is in SkippedPhases, handlePhaseThreeB returns
// done("skip:phase-3b"). When PhaseFour is not skipped, it returns the task decomposer.
func TestHandlePhaseThreeB_AutoApprove_PhaseFourSkipped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		skippedPhases []string
		wantType      string
		wantAgent     string
		wantSummary   string
	}{
		{
			name:          "phase_four_skipped_returns_done_skip",
			skippedPhases: []string{PhaseFour},
			wantType:      ActionDone,
			wantSummary:   SkipSummaryPrefix + PhaseThreeB,
		},
		{
			name:          "phase_four_not_skipped_spawns_decomposer",
			skippedPhases: []string{},
			wantType:      ActionSpawnAgent,
			wantAgent:     agentTaskDecomposer,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sm := newTestStateManager(t, "phase-3b", func(s *state.State) error {
				s.AutoApprove = true
				s.SkippedPhases = tc.skippedPhases
				return nil
			})
			st, err := sm.GetState()
			if err != nil {
				t.Fatalf("GetState: %v", err)
			}
			if err := writeFileForTest(st.Workspace+"/review-design.md", "## Verdict: APPROVE\n"); err != nil {
				t.Fatalf("writeFileForTest: %v", err)
			}

			eng := &Engine{
				agentDir:         "/test/agents",
				specsDir:         "/test/specs",
				verdictReader:    stubVerdictReader(VerdictApprove),
				sourceTypeReader: stubSourceTypeReader("text"),
			}

			action, err := eng.NextAction(sm, "")
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
		})
	}
}

// TestHandlePhaseFive_MinimalTasks verifies that when PhaseFour is in SkippedPhases,
// tasks is empty, and tasks.md does not exist, handlePhaseFive returns a write_file
// action with SetupOnly=true. When tasks.md already exists, it falls through to task_init.
func TestHandlePhaseFive_MinimalTasks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		writeTasksMd  bool
		wantType      string
		wantSetupOnly bool
		wantContent   string
	}{
		{
			name:          "tasks_md_absent_returns_write_file",
			writeTasksMd:  false,
			wantType:      ActionWriteFile,
			wantSetupOnly: true,
			wantContent:   "## Task 1",
		},
		{
			name:          "tasks_md_present_falls_through_to_task_init",
			writeTasksMd:  true,
			wantType:      ActionTaskInit,
			wantSetupOnly: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sm := newTestStateManager(t, "phase-5", func(s *state.State) error {
				s.SkippedPhases = []string{PhaseFour}
				// Tasks is empty to trigger the setup path
				return nil
			})
			st, err := sm.GetState()
			if err != nil {
				t.Fatalf("GetState: %v", err)
			}

			if tc.writeTasksMd {
				if err := writeFileForTest(st.Workspace+"/tasks.md", "# Tasks\n\n## Task 1: Implement\n"); err != nil {
					t.Fatalf("writeFileForTest: %v", err)
				}
			}

			action, err := defaultEng().NextAction(sm, "")
			if err != nil {
				t.Fatalf("NextAction: %v", err)
			}
			if action.Type != tc.wantType {
				t.Errorf("action.Type = %q, want %q", action.Type, tc.wantType)
			}
			if action.SetupOnly != tc.wantSetupOnly {
				t.Errorf("action.SetupOnly = %v, want %v", action.SetupOnly, tc.wantSetupOnly)
			}
			if tc.wantContent != "" && !strings.Contains(action.Content, tc.wantContent) {
				t.Errorf("action.Content does not contain %q: %q", tc.wantContent, action.Content)
			}
		})
	}
}

// TestHandlePhaseFive_HumanGate verifies that the engine emits ActionHumanGate
// when the first pending task has execution mode "human_gate".
func TestHandlePhaseFive_HumanGate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		tasks    map[string]state.Task
		wantType string
		wantName string
	}{
		{
			name: "human_gate_task_emits_action",
			tasks: map[string]state.Task{
				"1": {
					Title:         "Merge PR in external repo",
					ExecutionMode: state.ExecModeHumanGate,
					ImplStatus:    "pending",
				},
				"2": {
					Title:         "Update dependencies",
					ExecutionMode: state.ExecModeSequential,
					ImplStatus:    "pending",
					DependsOn:     []int{1},
				},
			},
			wantType: ActionHumanGate,
			wantName: "1",
		},
		{
			name: "completed_human_gate_skipped",
			tasks: map[string]state.Task{
				"1": {
					Title:         "Merge PR in external repo",
					ExecutionMode: state.ExecModeHumanGate,
					ImplStatus:    state.TaskStatusCompleted,
				},
				"2": {
					Title:         "Update dependencies",
					ExecutionMode: state.ExecModeSequential,
					ImplStatus:    "pending",
				},
			},
			wantType: ActionSpawnAgent,
			wantName: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sm := newTestStateManager(t, "phase-5", func(s *state.State) error {
				s.Tasks = tc.tasks
				return nil
			})

			action, err := defaultEng().NextAction(sm, "")
			if err != nil {
				t.Fatalf("NextAction: %v", err)
			}
			if action.Type != tc.wantType {
				t.Errorf("action.Type = %q, want %q", action.Type, tc.wantType)
			}
			if tc.wantName != "" && action.Name != tc.wantName {
				t.Errorf("action.Name = %q, want %q", action.Name, tc.wantName)
			}
		})
	}
}

func TestBranchPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		branch string
		want   string
	}{
		{"feature/soa-2899", "feature"},
		{"fix/login-bug", "fix"},
		{"refactor/cleanup", "refactor"},
		{"main", "main"},
		{"", ""},
	}

	for _, tc := range tests {
		t.Run(tc.branch, func(t *testing.T) {
			t.Parallel()
			got := branchPrefix(tc.branch)
			if got != tc.want {
				t.Errorf("branchPrefix(%q) = %q, want %q", tc.branch, got, tc.want)
			}
		})
	}
}

func TestMaybeRenameBranch_needed(t *testing.T) {
	t.Parallel()

	sm := newTestStateManager(t, PhaseCheckpointA, func(s *state.State) error {
		s.Branch = new(string("feature/soa-2899"))
		return nil
	})
	st, err := sm.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	// Write design.md with bug-fix content
	if err := writeFileForTest(st.Workspace+"/design.md", "# Design\n\nFix the login validation bug.\n"); err != nil {
		t.Fatalf("writeFileForTest: %v", err)
	}

	eng := defaultEng()
	action, needed := eng.maybeRenameBranch(st)
	if !needed {
		t.Fatal("maybeRenameBranch returned needed=false, want true")
	}
	if action.Type != ActionRenameBranch {
		t.Errorf("action.Type = %q, want %q", action.Type, ActionRenameBranch)
	}
	if action.OldBranch != "feature/soa-2899" {
		t.Errorf("action.OldBranch = %q, want %q", action.OldBranch, "feature/soa-2899")
	}
	if action.NewBranch != "fix/soa-2899" {
		t.Errorf("action.NewBranch = %q, want %q", action.NewBranch, "fix/soa-2899")
	}
}

func TestMaybeRenameBranch_already_classified(t *testing.T) {
	t.Parallel()

	sm := newTestStateManager(t, PhaseCheckpointA, func(s *state.State) error {
		s.Branch = new(string("feature/soa-2899"))
		s.BranchClassified = true
		return nil
	})
	st, err := sm.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if err := writeFileForTest(st.Workspace+"/design.md", "# Design\n\nFix the bug.\n"); err != nil {
		t.Fatalf("writeFileForTest: %v", err)
	}

	eng := defaultEng()
	_, needed := eng.maybeRenameBranch(st)
	if needed {
		t.Error("maybeRenameBranch returned needed=true for already classified branch")
	}
}

func TestMaybeRenameBranch_same_type(t *testing.T) {
	t.Parallel()

	sm := newTestStateManager(t, PhaseCheckpointA, func(s *state.State) error {
		s.Branch = new(string("feature/add-user-auth"))
		return nil
	})
	st, err := sm.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	// Content has no special keywords → defaults to "feature", matches current prefix
	if err := writeFileForTest(st.Workspace+"/design.md", "# Design\n\nAdd a new user authentication page.\n"); err != nil {
		t.Fatalf("writeFileForTest: %v", err)
	}

	eng := defaultEng()
	_, needed := eng.maybeRenameBranch(st)
	if needed {
		t.Error("maybeRenameBranch returned needed=true when classified type matches current prefix")
	}
}

func TestMaybeRenameBranch_use_current_branch(t *testing.T) {
	t.Parallel()

	sm := newTestStateManager(t, PhaseCheckpointA, func(s *state.State) error {
		s.Branch = new(string("feature/soa-2899"))
		s.UseCurrentBranch = true
		return nil
	})
	st, err := sm.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if err := writeFileForTest(st.Workspace+"/design.md", "# Design\n\nFix the bug.\n"); err != nil {
		t.Fatalf("writeFileForTest: %v", err)
	}

	eng := defaultEng()
	_, needed := eng.maybeRenameBranch(st)
	if needed {
		t.Error("maybeRenameBranch returned needed=true when UseCurrentBranch is set")
	}
}

func TestMaybeRenameBranch_no_slash_in_branch(t *testing.T) {
	t.Parallel()

	sm := newTestStateManager(t, PhaseCheckpointA, func(s *state.State) error {
		s.Branch = new(string("main"))
		return nil
	})
	st, err := sm.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if err := writeFileForTest(st.Workspace+"/design.md", "# Design\n\nFix the bug.\n"); err != nil {
		t.Fatalf("writeFileForTest: %v", err)
	}

	eng := defaultEng()
	_, needed := eng.maybeRenameBranch(st)
	if needed {
		t.Error("maybeRenameBranch returned needed=true for branch without slash")
	}
}

func TestClassifyBranchType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    string
	}{
		// English
		{name: "en_bug", content: "This design fixes a bug in the login flow", want: BranchTypeFix},
		{name: "en_fix", content: "Fix the validation error on submit", want: BranchTypeFix},
		{name: "en_hotfix", content: "Hotfix for production crash", want: BranchTypeFix},
		{name: "en_defect", content: "Address defect in email parser", want: BranchTypeFix},
		{name: "en_refactor", content: "Refactor the handler layer for clarity", want: BranchTypeRefactor},
		{name: "en_restructure", content: "Restructure the package layout", want: BranchTypeRefactor},
		{name: "en_reorganize", content: "Reorganize test helpers", want: BranchTypeRefactor},
		{name: "en_documentation", content: "Add documentation for the API", want: BranchTypeDocs},
		{name: "en_readme", content: "Update the README with examples", want: BranchTypeDocs},
		{name: "en_dependency", content: "Upgrade dependency versions", want: BranchTypeChore},
		{name: "en_migration", content: "Database migration for new schema", want: BranchTypeChore},
		{name: "en_config", content: "Update config for staging environment", want: BranchTypeChore},
		{name: "en_feature", content: "Add user authentication with OAuth", want: BranchTypeFeature},

		// Japanese
		{name: "ja_bug", content: "ログインフローのバグを修正", want: BranchTypeFix},
		{name: "ja_fix", content: "バリデーションエラーの修正を行う", want: BranchTypeFix},
		{name: "ja_fuguai", content: "メール送信の不具合を調査", want: BranchTypeFix},
		{name: "ja_shougai", content: "本番環境の障害対応", want: BranchTypeFix},
		{name: "ja_refactor", content: "ハンドラ層のリファクタリング", want: BranchTypeRefactor},
		{name: "ja_saikousei", content: "パッケージの再構成", want: BranchTypeRefactor},
		{name: "ja_document", content: "APIのドキュメント追加", want: BranchTypeDocs},
		{name: "ja_bunsho", content: "仕様の文書化", want: BranchTypeDocs},
		{name: "ja_shiryou", content: "設計資料を更新", want: BranchTypeDocs},
		{name: "ja_izon", content: "依存パッケージの更新", want: BranchTypeChore},
		{name: "ja_upgrade", content: "Goバージョンのアップグレード", want: BranchTypeChore},
		{name: "ja_ikou", content: "DBスキーマの移行作業", want: BranchTypeChore},
		{name: "ja_settei", content: "環境設定の変更", want: BranchTypeChore},

		// German
		{name: "de_fehler", content: "Fehler im Login-Flow beheben", want: BranchTypeFix},
		{name: "de_bugfix", content: "Bugfix für Produktionsabsturz", want: BranchTypeFix},
		{name: "de_refaktorierung", content: "Refaktorierung der Handler-Schicht", want: BranchTypeRefactor},
		{name: "de_umstrukturierung", content: "Umstrukturierung des Paketlayouts", want: BranchTypeRefactor},
		{name: "de_dokumentation", content: "Dokumentation für die API hinzufügen", want: BranchTypeDocs},
		{name: "de_abhaengigkeit", content: "Abhängigkeit aktualisieren", want: BranchTypeChore},
		{name: "de_configuration", content: "Configuration der Staging-Umgebung", want: BranchTypeChore},

		// French
		{name: "fr_correctif", content: "Correctif pour le crash en production", want: BranchTypeFix},
		{name: "fr_bogue", content: "Résoudre le bogue dans le parseur", want: BranchTypeFix},
		{name: "fr_refactorisation", content: "Refactorisation de la couche handler", want: BranchTypeRefactor},
		{name: "fr_restructuration", content: "Restructuration du layout des packages", want: BranchTypeRefactor},
		{name: "fr_dependence", content: "Mise à jour de la dépendance", want: BranchTypeChore},
		{name: "fr_configuration", content: "Configuration de l'environnement", want: BranchTypeChore},

		// Priority: fix > refactor when both present
		{name: "priority_fix_over_refactor", content: "Refactor and fix the bug", want: BranchTypeFix},
		// Priority: fix > docs when both present
		{name: "priority_fix_over_docs", content: "Fix documentation bug", want: BranchTypeFix},

		// Case insensitivity
		{name: "case_insensitive_FIX", content: "FIX the broken tests", want: BranchTypeFix},
		{name: "case_insensitive_REFACTOR", content: "REFACTOR the service layer", want: BranchTypeRefactor},
		{name: "case_insensitive_DOCUMENTATION", content: "DOCUMENTATION update needed", want: BranchTypeDocs},

		// Default
		{name: "empty_input", content: "", want: BranchTypeFeature},
		{name: "no_keywords", content: "Add new user profile page with avatar upload", want: BranchTypeFeature},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ClassifyBranchType(tc.content)
			if got != tc.want {
				t.Errorf("ClassifyBranchType(%q) = %q, want %q", tc.content, got, tc.want)
			}
		})
	}
}

func TestNewRenameBranchAction(t *testing.T) {
	t.Parallel()
	action := NewRenameBranchAction("checkpoint-a", "feature/soa-2899", "fix/soa-2899")
	want := Action{
		Type:      ActionRenameBranch,
		Phase:     "checkpoint-a",
		OldBranch: "feature/soa-2899",
		NewBranch: "fix/soa-2899",
		SetupOnly: true,
	}
	if !reflect.DeepEqual(action, want) {
		t.Errorf("NewRenameBranchAction() = %+v, want %+v", action, want)
	}
}

func TestHandlePhaseThree_RevisionIncludesReviewDesign(t *testing.T) {
	t.Parallel()

	// When phase-3b is in CompletedPhases (checkpoint-a revision rewind),
	// handlePhaseThree should include review-design.md in InputFiles.
	sm := newTestStateManager(t, "phase-3", func(s *state.State) error {
		s.CompletedPhases = []string{"setup", "phase-1", "phase-2", "phase-3", "phase-3b"}
		return nil
	})

	eng := defaultEng()
	action, err := eng.NextAction(sm, "")
	if err != nil {
		t.Fatalf("NextAction: %v", err)
	}
	if action.Type != ActionSpawnAgent {
		t.Fatalf("action.Type = %q, want %q", action.Type, ActionSpawnAgent)
	}
	if action.Agent != agentArchitect {
		t.Fatalf("action.Agent = %q, want %q", action.Agent, agentArchitect)
	}

	hasReviewDesign := slices.Contains(action.InputFiles, state.ArtifactReviewDesign)
	if !hasReviewDesign {
		t.Errorf("InputFiles = %v, want to contain %q for revision run", action.InputFiles, state.ArtifactReviewDesign)
	}
}

func TestHandlePhaseThree_FreshRunNoReviewDesign(t *testing.T) {
	t.Parallel()

	// When phase-3b is NOT in CompletedPhases (fresh run),
	// handlePhaseThree should NOT include review-design.md in InputFiles.
	sm := newTestStateManager(t, "phase-3", nil)

	eng := defaultEng()
	action, err := eng.NextAction(sm, "")
	if err != nil {
		t.Fatalf("NextAction: %v", err)
	}
	if action.Type != ActionSpawnAgent {
		t.Fatalf("action.Type = %q, want %q", action.Type, ActionSpawnAgent)
	}

	hasReviewDesign := slices.Contains(action.InputFiles, state.ArtifactReviewDesign)
	if hasReviewDesign {
		t.Errorf("InputFiles = %v, should NOT contain %q for fresh run", action.InputFiles, state.ArtifactReviewDesign)
	}
}

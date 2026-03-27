package orchestrator

import (
	"reflect"
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/state"
)

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

// TestNextAction_SkipGate verifies that phases in SkippedPhases return a skip done action.
func TestNextAction_SkipGate(t *testing.T) {
	t.Parallel()

	sm := newTestStateManager(t, "phase-2", func(s *state.State) error {
		s.SkippedPhases = []string{"phase-2"}
		return nil
	})

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

	if action.Type != ActionDone {
		t.Errorf("action.Type = %q, want %q", action.Type, ActionDone)
	}

	if action.Summary != SkipSummaryPrefix+"phase-2" {
		t.Errorf("action.Summary = %q, want %q", action.Summary, SkipSummaryPrefix+"phase-2")
	}
}

// TestNextAction_LiteFlowTemplate verifies Decision 15: lite template uses analyst agent.
func TestNextAction_LiteFlowTemplate(t *testing.T) {
	t.Parallel()

	lite := TemplateLite
	sm := newTestStateManager(t, "phase-1", func(s *state.State) error {
		s.FlowTemplate = &lite
		return nil
	})

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

	if action.Agent != agentAnalyst {
		t.Errorf("action.Agent = %q, want %q", action.Agent, agentAnalyst)
	}
}

// TestNextAction_StandardFlowTemplate verifies standard template uses situation-analyst agent.
func TestNextAction_StandardFlowTemplate(t *testing.T) {
	t.Parallel()

	sm := newTestStateManager(t, "phase-1", nil)

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

	if action.Agent != agentSituationAnalyst {
		t.Errorf("action.Agent = %q, want %q", action.Agent, agentSituationAnalyst)
	}
}

// TestNextAction_SkipPR verifies Decision 24: SkipPr flag skips pr-creation.
func TestNextAction_SkipPR(t *testing.T) {
	t.Parallel()

	sm := newTestStateManager(t, "pr-creation", func(s *state.State) error {
		s.SkipPr = true
		return nil
	})

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

	if action.Type != ActionDone {
		t.Errorf("action.Type = %q, want %q", action.Type, ActionDone)
	}

	if action.Summary != SkipSummaryPrefix+"pr-creation" {
		t.Errorf("action.Summary = %q, want %q", action.Summary, SkipSummaryPrefix+"pr-creation")
	}
}

// TestNextAction_PostToSource_GithubIssue verifies Decision 26: github_issue → exec action.
func TestNextAction_PostToSource_GithubIssue(t *testing.T) {
	t.Parallel()

	sm := newTestStateManager(t, "post-to-source", nil)

	eng := &Engine{
		agentDir:         "/test/agents",
		specsDir:         "/test/specs",
		verdictReader:    stubVerdictReader(VerdictApprove),
		sourceTypeReader: stubSourceTypeReader("github_issue"),
	}

	action, err := eng.NextAction(sm, "")
	if err != nil {
		t.Fatalf("NextAction: %v", err)
	}

	if action.Type != ActionExec {
		t.Errorf("action.Type = %q, want %q", action.Type, ActionExec)
	}
}

// TestNextAction_PostToSource_Text verifies Decision 26: text → done action.
func TestNextAction_PostToSource_Text(t *testing.T) {
	t.Parallel()

	sm := newTestStateManager(t, "post-to-source", nil)

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

	if action.Type != ActionDone {
		t.Errorf("action.Type = %q, want %q", action.Type, ActionDone)
	}
}

// TestNextAction_PostToSource_JiraIssue verifies Decision 26: jira_issue → checkpoint action.
func TestNextAction_PostToSource_JiraIssue(t *testing.T) {
	t.Parallel()

	sm := newTestStateManager(t, "post-to-source", nil)

	eng := &Engine{
		agentDir:         "/test/agents",
		specsDir:         "/test/specs",
		verdictReader:    stubVerdictReader(VerdictApprove),
		sourceTypeReader: stubSourceTypeReader("jira_issue"),
	}

	action, err := eng.NextAction(sm, "")
	if err != nil {
		t.Fatalf("NextAction: %v", err)
	}

	if action.Type != ActionCheckpoint {
		t.Errorf("action.Type = %q, want %q", action.Type, ActionCheckpoint)
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

// TestReadSourceType_Present verifies that readSourceType returns the value from front matter.
func TestReadSourceType_Present(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	content := "---\nsource_type: github_issue\nsource_url: https://example.com\n---\n\n# Title\n"
	if err := writeFileForTest(dir+"/request.md", content); err != nil {
		t.Fatalf("writeFileForTest: %v", err)
	}

	got := readSourceType(dir)
	if got != "github_issue" {
		t.Errorf("readSourceType = %q, want %q", got, "github_issue")
	}
}

// TestReadSourceType_Absent verifies that readSourceType returns "text" when field is absent.
func TestReadSourceType_Absent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	content := "---\nsource_url: https://example.com\n---\n\n# Title\n"
	if err := writeFileForTest(dir+"/request.md", content); err != nil {
		t.Fatalf("writeFileForTest: %v", err)
	}

	got := readSourceType(dir)
	if got != "text" {
		t.Errorf("readSourceType = %q, want %q", got, "text")
	}
}

// TestReadSourceType_Unreadable verifies that readSourceType returns "text" when file unreadable.
func TestReadSourceType_Unreadable(t *testing.T) {
	t.Parallel()

	got := readSourceType("/nonexistent/path/that/cannot/exist")
	if got != "text" {
		t.Errorf("readSourceType = %q, want %q", got, "text")
	}
}

// TestSortedTaskKeys_NumericOrder verifies that sortedTaskKeys returns keys in numeric ascending order.
func TestSortedTaskKeys_NumericOrder(t *testing.T) {
	t.Parallel()

	tasks := map[string]state.Task{
		"3": {Title: "third"},
		"1": {Title: "first"},
		"2": {Title: "second"},
	}

	got := sortedTaskKeys(tasks)
	want := []string{"1", "2", "3"}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("sortedTaskKeys = %v, want %v", got, want)
	}
}

// TestSortedTaskKeys_EmptyMap verifies that sortedTaskKeys handles empty map.
func TestSortedTaskKeys_EmptyMap(t *testing.T) {
	t.Parallel()

	tasks := map[string]state.Task{}
	got := sortedTaskKeys(tasks)

	if len(got) != 0 {
		t.Errorf("sortedTaskKeys(empty) = %v, want []", got)
	}
}

// TestSortedTaskKeys_MixedKeys verifies that numeric keys sort before non-numeric.
func TestSortedTaskKeys_MixedKeys(t *testing.T) {
	t.Parallel()

	tasks := map[string]state.Task{
		"alpha": {Title: "a"},
		"2":     {Title: "two"},
		"1":     {Title: "one"},
	}

	got := sortedTaskKeys(tasks)

	// Numeric keys should come first, then lexicographic
	if len(got) != 3 {
		t.Fatalf("sortedTaskKeys length = %d, want 3", len(got))
	}
	if got[0] != "1" || got[1] != "2" {
		t.Errorf("sortedTaskKeys = %v; want numeric keys first: [1, 2, alpha]", got)
	}
}

// TestNextAction_AutoApproveDesignReview verifies Decision 20: auto-approve skips checkpoint.
func TestNextAction_AutoApproveDesignReview(t *testing.T) {
	t.Parallel()

	sm := newTestStateManager(t, "phase-3b", func(s *state.State) error {
		s.AutoApprove = true
		return nil
	})

	eng := &Engine{
		agentDir:         "/test/agents",
		specsDir:         "/test/specs",
		verdictReader:    stubVerdictReader(VerdictApprove),
		sourceTypeReader: stubSourceTypeReader("text"),
	}

	// Create a review file in the temp dir
	st, err := sm.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	reviewContent := "## Verdict: APPROVE\n"
	if err := writeFileForTest(st.Workspace+"/review-design.md", reviewContent); err != nil {
		t.Fatalf("writeFileForTest: %v", err)
	}

	action, err := eng.NextAction(sm, "")
	if err != nil {
		t.Fatalf("NextAction: %v", err)
	}

	// With auto-approve and APPROVE verdict, should proceed to task decomposer without checkpoint
	if action.Type != ActionSpawnAgent {
		t.Errorf("action.Type = %q, want %q", action.Type, ActionSpawnAgent)
	}
	if action.Agent != agentTaskDecomposer {
		t.Errorf("action.Agent = %q, want %q", action.Agent, agentTaskDecomposer)
	}
}

// TestNextAction_DesignReviewRevise verifies Decision 18: REVISE verdict re-spawns architect.
func TestNextAction_DesignReviewRevise(t *testing.T) {
	t.Parallel()

	sm := newTestStateManager(t, "phase-3b", nil)

	eng := &Engine{
		agentDir:         "/test/agents",
		specsDir:         "/test/specs",
		verdictReader:    stubVerdictReader(VerdictRevise),
		sourceTypeReader: stubSourceTypeReader("text"),
	}

	// Create review file to trigger verdict reading
	st, err := sm.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if err := writeFileForTest(st.Workspace+"/review-design.md", "## Verdict: REVISE\n"); err != nil {
		t.Fatalf("writeFileForTest: %v", err)
	}

	action, err := eng.NextAction(sm, "")
	if err != nil {
		t.Fatalf("NextAction: %v", err)
	}

	if action.Agent != agentArchitect {
		t.Errorf("action.Agent = %q, want %q", action.Agent, agentArchitect)
	}
}

// TestNextAction_RetryLimit verifies Decision 21: revision count >= 2 escalates to human.
func TestNextAction_RetryLimit(t *testing.T) {
	t.Parallel()

	sm := newTestStateManager(t, "phase-3b", func(s *state.State) error {
		s.Revisions.DesignRevisions = 2
		return nil
	})

	eng := &Engine{
		agentDir:         "/test/agents",
		specsDir:         "/test/specs",
		verdictReader:    stubVerdictReader(VerdictRevise),
		sourceTypeReader: stubSourceTypeReader("text"),
	}

	// Create review file
	st, err := sm.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if err := writeFileForTest(st.Workspace+"/review-design.md", "## Verdict: REVISE\n"); err != nil {
		t.Fatalf("writeFileForTest: %v", err)
	}

	action, err := eng.NextAction(sm, "")
	if err != nil {
		t.Fatalf("NextAction: %v", err)
	}

	if action.Type != ActionCheckpoint {
		t.Errorf("action.Type = %q, want %q", action.Type, ActionCheckpoint)
	}
}

// TestNextAction_Phase5Sequential verifies Decision 22: sequential tasks use SpawnAgent without ParallelTaskIDs.
func TestNextAction_Phase5Sequential(t *testing.T) {
	t.Parallel()

	sm := newTestStateManager(t, "phase-5", func(s *state.State) error {
		s.Tasks = map[string]state.Task{
			"1": {Title: "Task 1", ExecutionMode: "sequential", ImplStatus: "pending"},
			"2": {Title: "Task 2", ExecutionMode: "sequential", ImplStatus: "pending"},
		}
		return nil
	})

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

	if len(action.ParallelTaskIDs) != 0 {
		t.Errorf("ParallelTaskIDs = %v, want empty for sequential task", action.ParallelTaskIDs)
	}
}

// TestNextAction_Phase5Parallel verifies Decision 22: parallel tasks set ParallelTaskIDs.
func TestNextAction_Phase5Parallel(t *testing.T) {
	t.Parallel()

	sm := newTestStateManager(t, "phase-5", func(s *state.State) error {
		s.Tasks = map[string]state.Task{
			"1": {Title: "Task 1", ExecutionMode: "parallel", ImplStatus: "pending"},
			"2": {Title: "Task 2", ExecutionMode: "parallel", ImplStatus: "pending"},
		}
		return nil
	})

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

	want := []string{"1", "2"}
	if !reflect.DeepEqual(action.ParallelTaskIDs, want) {
		t.Errorf("ParallelTaskIDs = %v, want %v", action.ParallelTaskIDs, want)
	}
}

// TestNextAction_Phase5ThreeParallel verifies Decision 22: three parallel tasks sets all IDs.
func TestNextAction_Phase5ThreeParallel(t *testing.T) {
	t.Parallel()

	sm := newTestStateManager(t, "phase-5", func(s *state.State) error {
		s.Tasks = map[string]state.Task{
			"1": {Title: "Task 1", ExecutionMode: "parallel", ImplStatus: "pending"},
			"2": {Title: "Task 2", ExecutionMode: "parallel", ImplStatus: "pending"},
			"3": {Title: "Task 3", ExecutionMode: "parallel", ImplStatus: "pending"},
		}
		return nil
	})

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

	want := []string{"1", "2", "3"}
	if !reflect.DeepEqual(action.ParallelTaskIDs, want) {
		t.Errorf("ParallelTaskIDs = %v, want %v", action.ParallelTaskIDs, want)
	}
}

// TestNextAction_Phase6ImplFail verifies Decision 23: FAIL verdict retries implementation.
func TestNextAction_Phase6ImplFail(t *testing.T) {
	t.Parallel()

	sm := newTestStateManager(t, "phase-6", func(s *state.State) error {
		s.Tasks = map[string]state.Task{
			"1": {Title: "Task 1", ExecutionMode: "sequential", ImplStatus: "completed", ReviewStatus: ""},
		}
		return nil
	})

	eng := &Engine{
		agentDir:         "/test/agents",
		specsDir:         "/test/specs",
		verdictReader:    stubVerdictReader(VerdictFail),
		sourceTypeReader: stubSourceTypeReader("text"),
	}

	// Create a review file so verdict is read (not just spawning reviewer)
	st, err := sm.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if err := writeFileForTest(st.Workspace+"/review-1.md", "## Verdict: FAIL\n"); err != nil {
		t.Fatalf("writeFileForTest: %v", err)
	}

	action, err := eng.NextAction(sm, "")
	if err != nil {
		t.Fatalf("NextAction: %v", err)
	}

	// Should retry implementation
	if action.Type != ActionSpawnAgent {
		t.Errorf("action.Type = %q, want %q (should retry impl)", action.Type, ActionSpawnAgent)
	}
	if action.Agent != agentImplementer {
		t.Errorf("action.Agent = %q, want %q", action.Agent, agentImplementer)
	}
}

// TestNextAction_Phase6ImplRetryLimit verifies Decision 23: retry >= 2 escalates to human.
func TestNextAction_Phase6ImplRetryLimit(t *testing.T) {
	t.Parallel()

	sm := newTestStateManager(t, "phase-6", func(s *state.State) error {
		s.Tasks = map[string]state.Task{
			"1": {Title: "Task 1", ExecutionMode: "sequential", ImplStatus: "completed", ReviewStatus: "", ImplRetries: 2},
		}
		return nil
	})

	eng := &Engine{
		agentDir:         "/test/agents",
		specsDir:         "/test/specs",
		verdictReader:    stubVerdictReader(VerdictFail),
		sourceTypeReader: stubSourceTypeReader("text"),
	}

	// Create a review file
	st, err := sm.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if err := writeFileForTest(st.Workspace+"/review-1.md", "## Verdict: FAIL\n"); err != nil {
		t.Fatalf("writeFileForTest: %v", err)
	}

	action, err := eng.NextAction(sm, "")
	if err != nil {
		t.Fatalf("NextAction: %v", err)
	}

	if action.Type != ActionCheckpoint {
		t.Errorf("action.Type = %q, want %q (should escalate)", action.Type, ActionCheckpoint)
	}
}

// TestNextAction_DocsStubSynthesis verifies Decision 16: docs task synthesizes stubs after phase-1.
func TestNextAction_DocsStubSynthesis(t *testing.T) {
	t.Parallel()

	docs := TaskTypeDocs
	sm := newTestStateManager(t, "phase-1", func(s *state.State) error {
		s.TaskType = &docs
		s.CompletedPhases = []string{"setup", PhaseOne}
		return nil
	})

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

	// First call should write design.md stub (file doesn't exist in temp dir)
	if action.Type != ActionWriteFile {
		t.Errorf("action.Type = %q, want %q", action.Type, ActionWriteFile)
	}
}

// TestNextAction_BugfixStubSynthesis verifies Decision 17: bugfix task synthesizes stubs after phase-3.
func TestNextAction_BugfixStubSynthesis(t *testing.T) {
	t.Parallel()

	bugfix := TaskTypeBugfix
	sm := newTestStateManager(t, "phase-3", func(s *state.State) error {
		s.TaskType = &bugfix
		s.CompletedPhases = []string{"setup", PhaseOne, PhaseTwo, PhaseThree}
		return nil
	})

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

	// First call should write design.md stub (file doesn't exist in temp dir)
	if action.Type != ActionWriteFile {
		t.Errorf("action.Type = %q, want %q", action.Type, ActionWriteFile)
	}
}

// TestNextAction_FinalSummary_Investigation verifies Decision 25 for investigation type.
func TestNextAction_FinalSummary_Investigation(t *testing.T) {
	t.Parallel()

	investigation := TaskTypeInvestigation
	sm := newTestStateManager(t, "final-summary", func(s *state.State) error {
		s.TaskType = &investigation
		return nil
	})

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

	if action.Type != ActionSpawnAgent {
		t.Errorf("action.Type = %q, want %q", action.Type, ActionSpawnAgent)
	}

	// Should use analysis.md and investigation.md as inputs
	found := false
	for _, f := range action.InputFiles {
		if f == "investigation.md" {
			found = true
		}
	}
	if !found {
		t.Errorf("InputFiles = %v; expected to contain investigation.md", action.InputFiles)
	}
}

// TestNextAction_FinalSummary_Feature verifies Decision 25 for feature type (comprehensive review).
func TestNextAction_FinalSummary_Feature(t *testing.T) {
	t.Parallel()

	feature := TaskTypeFeature
	sm := newTestStateManager(t, "final-summary", func(s *state.State) error {
		s.TaskType = &feature
		return nil
	})

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

	if action.Agent != agentComprehensiveReview {
		t.Errorf("action.Agent = %q, want %q", action.Agent, agentComprehensiveReview)
	}
}

// TestNextAction_Completed verifies that the completed phase returns done action.
func TestNextAction_Completed(t *testing.T) {
	t.Parallel()

	sm := newTestStateManager(t, "completed", nil)

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

	if action.Type != ActionDone {
		t.Errorf("action.Type = %q, want %q", action.Type, ActionDone)
	}
}

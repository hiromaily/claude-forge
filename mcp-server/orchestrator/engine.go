package orchestrator

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	// orchestrator → state (one-way; state must never import orchestrator)
	"github.com/hiromaily/claude-forge/mcp-server/state"
)

// Agent name constants — unexported; used only inside NextAction dispatch.
const (
	agentSituationAnalyst    = "situation-analyst"
	agentAnalyst             = "analyst"
	agentInvestigator        = "investigator"
	agentArchitect           = "architect"
	agentDesignReviewer      = "design-reviewer"
	agentTaskDecomposer      = "task-decomposer"
	agentTaskReviewer        = "task-reviewer"
	agentImplementer         = "implementer"
	agentImplReviewer        = "impl-reviewer"
	agentComprehensiveReview = "comprehensive-reviewer"
	agentVerifier            = "verifier"
)

// Engine computes the next pipeline action from the current state.
// verdictReader and sourceTypeReader are injectable for testing;
// NewEngine sets them to the production implementations.
type Engine struct {
	agentDir         string // reserved for future agent .md file resolution; not read by NextAction
	specsDir         string
	verdictReader    func(path string) (Verdict, []Finding, error)
	sourceTypeReader func(workspace string) string
}

// NewEngine constructs a ready-to-use Engine with production I/O implementations.
// agentDir and specsDir are stored as-is; no path existence validation is performed.
func NewEngine(agentDir, specsDir string) *Engine {
	return &Engine{
		agentDir:         agentDir,
		specsDir:         specsDir,
		verdictReader:    ParseVerdict,
		sourceTypeReader: readSourceType,
	}
}

// NextAction returns the next Action to execute given the current pipeline state.
// It does not mutate state. File I/O is performed only through e.verdictReader
// (decisions 18, 19, 23) and e.sourceTypeReader (decision 26).
//
// Caller contract for skip signals: when action.Type == ActionDone and
// strings.HasPrefix(action.Summary, SkipSummaryPrefix), the caller must call
// phase-complete for the current phase, then invoke NextAction again.
// A true pipeline completion returns ActionDone with a Summary that does NOT
// start with SkipSummaryPrefix.
//
//nolint:gocyclo // complexity is inherent in the dispatch table
func (e *Engine) NextAction(sm *state.StateManager, _ string) (Action, error) {
	st, err := sm.GetState()
	if err != nil {
		return Action{}, fmt.Errorf("NextAction: GetState: %w", err)
	}

	phase := st.CurrentPhase

	// Decision 14 — Phase skip gate
	// Fires before any per-phase handler.
	if slices.Contains(st.SkippedPhases, phase) {
		return NewDoneAction(SkipSummaryPrefix+phase, ""), nil
	}

	switch phase {
	case PhaseOne:
		return e.handlePhaseOne(st)
	case PhaseTwo:
		return e.handlePhaseTwo(st)
	case PhaseThree:
		return e.handlePhaseThree(st)
	case PhaseThreeB:
		return e.handlePhaseThreeB(st)
	case PhaseCheckpointA:
		return e.handleCheckpointA(st)
	case PhaseFour:
		return e.handlePhaseFour(st)
	case PhaseFourB:
		return e.handlePhaseFourB(st)
	case PhaseCheckpointB:
		return e.handleCheckpointB(st)
	case PhaseFive:
		return e.handlePhaseFive(st)
	case PhaseSix:
		return e.handlePhaseSix(st)
	case PhaseSeven:
		return e.handlePhaseSeven(st)
	case PhaseFinalVerification:
		return e.handleFinalVerification(st)
	case PhasePRCreation:
		return e.handlePRCreation(st)
	case PhaseFinalSummary:
		return e.handleFinalSummary(st)
	case PhasePostToSource:
		return e.handlePostToSource(st)
	case PhaseCompleted:
		return NewDoneAction("pipeline completed", ""), nil
	default:
		return Action{}, fmt.Errorf("NextAction: unknown phase %q", phase)
	}
}

// handlePhaseOne handles Phase 1 (situation analysis / analyst).
// Implements Decision 15 (lite → analyst) and Decision 16 (docs stub synthesis).
func (e *Engine) handlePhaseOne(st *state.State) (Action, error) {
	// Decision 16 — docs M/L stub synthesis (after Phase 1 completes)
	if st.TaskType != nil && *st.TaskType == TaskTypeDocs &&
		slices.Contains(st.CompletedPhases, PhaseOne) {
		return e.handleDocsStubSynthesis(st, PhaseOne)
	}

	// Decision 15 — lite → analyst agent
	if st.FlowTemplate != nil && *st.FlowTemplate == TemplateLite {
		return NewSpawnAgentAction(
			agentAnalyst,
			"Run Phase 1+2 analysis for the pipeline.",
			"sonnet",
			PhaseOne,
			[]string{"request.md"},
			"analysis.md",
		), nil
	}

	// Standard Phase 1: situation-analyst
	return NewSpawnAgentAction(
		agentSituationAnalyst,
		"Run Phase 1 situation analysis for the pipeline.",
		"sonnet",
		PhaseOne,
		[]string{"request.md"},
		"analysis.md",
	), nil
}

// handleDocsStubSynthesis implements Decision 16 — docs M/L stub synthesis.
// Returns stub write actions one at a time based on file existence.
// phase is the pipeline phase under which these actions are issued (always PhaseOne from caller).
func (*Engine) handleDocsStubSynthesis(st *state.State, phase string) (Action, error) {
	workspace := st.Workspace

	// Step 1: design.md absent → write stub
	designPath := filepath.Join(workspace, "design.md")
	if _, err := os.Stat(designPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return Action{}, fmt.Errorf("handleDocsStubSynthesis: stat %s: %w", designPath, err)
		}
		content := "# Design\n\n_Auto-generated stub for docs task type. " +
			"Fill in the design details or leave as-is._\n"
		return NewWriteFileAction(phase, designPath, content), nil
	}

	// Step 2: tasks.md absent → write stub
	tasksPath := filepath.Join(workspace, "tasks.md")
	if _, err := os.Stat(tasksPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return Action{}, fmt.Errorf("handleDocsStubSynthesis: stat %s: %w", tasksPath, err)
		}
		content := "## Task 1: Implement documentation [sequential]\n\n" +
			"**Depends on:** None\n**Files:** TBD\n\n" +
			"**Acceptance criteria:**\n- [ ] **AC-1:** Documentation is complete.\n"
		return NewWriteFileAction(phase, tasksPath, content), nil
	}

	// Step 3: both present but tasks map empty → run task_init
	if len(st.Tasks) == 0 {
		return NewExecAction(phase, []string{"task_init", workspace}), nil
	}

	// Step 4: all done → advance to Phase 3b
	return NewSpawnAgentAction(
		agentDesignReviewer,
		"Review the design document.",
		"sonnet",
		PhaseThreeB,
		[]string{"design.md"},
		"review-design.md",
	), nil
}

// handlePhaseTwo handles Phase 2 (investigator).
func (*Engine) handlePhaseTwo(_ *state.State) (Action, error) {
	return NewSpawnAgentAction(
		agentInvestigator,
		"Run Phase 2 deep-dive investigation.",
		"sonnet",
		PhaseTwo,
		[]string{"request.md", "analysis.md"},
		"investigation.md",
	), nil
}

// handlePhaseThree handles Phase 3 (architect) and Decision 17 (bugfix stub synthesis).
func (e *Engine) handlePhaseThree(st *state.State) (Action, error) {
	// Decision 17 — bugfix stub synthesis (after Phase 3 completes)
	if st.TaskType != nil && *st.TaskType == TaskTypeBugfix &&
		slices.Contains(st.CompletedPhases, PhaseThree) {
		return e.handleBugfixStubSynthesis(st, PhaseThree)
	}

	return NewSpawnAgentAction(
		agentArchitect,
		"Run Phase 3 architecture/design.",
		"sonnet",
		PhaseThree,
		[]string{"request.md", "analysis.md", "investigation.md"},
		"design.md",
	), nil
}

// handleBugfixStubSynthesis implements Decision 17 — bugfix stub synthesis.
// phase is the pipeline phase under which these actions are issued (always PhaseThree from caller).
func (*Engine) handleBugfixStubSynthesis(st *state.State, phase string) (Action, error) {
	workspace := st.Workspace

	// Step 1: design.md absent → write stub
	designPath := filepath.Join(workspace, "design.md")
	if _, err := os.Stat(designPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return Action{}, fmt.Errorf("handleBugfixStubSynthesis: stat %s: %w", designPath, err)
		}
		content := "# Design\n\n_Auto-generated stub for bugfix task type. " +
			"Fill in the fix description or leave as-is._\n"
		return NewWriteFileAction(phase, designPath, content), nil
	}

	// Step 2: tasks.md absent → write stub
	tasksPath := filepath.Join(workspace, "tasks.md")
	if _, err := os.Stat(tasksPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return Action{}, fmt.Errorf("handleBugfixStubSynthesis: stat %s: %w", tasksPath, err)
		}
		content := "## Task 1: Apply bugfix [sequential]\n\n" +
			"**Depends on:** None\n**Files:** TBD\n\n" +
			"**Acceptance criteria:**\n- [ ] **AC-1:** Bug is fixed.\n"
		return NewWriteFileAction(phase, tasksPath, content), nil
	}

	// Step 3: both present but tasks map empty → run task_init
	if len(st.Tasks) == 0 {
		return NewExecAction(phase, []string{"task_init", workspace}), nil
	}

	// Step 4: all done → advance to Phase 4b (task reviewer)
	return NewSpawnAgentAction(
		agentTaskReviewer,
		"Review the task decomposition.",
		"sonnet",
		PhaseFourB,
		[]string{"tasks.md"},
		"review-tasks.md",
	), nil
}

// handlePhaseThreeB handles Phase 3b (design reviewer) — Decision 18.
func (e *Engine) handlePhaseThreeB(st *state.State) (Action, error) {
	reviewPath := filepath.Join(st.Workspace, "review-design.md")

	// If review file doesn't exist yet, spawn the design reviewer.
	if _, err := os.Stat(reviewPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return Action{}, fmt.Errorf("handlePhaseThreeB: stat %s: %w", reviewPath, err)
		}
		return NewSpawnAgentAction(
			agentDesignReviewer,
			"Review the design document.",
			"sonnet",
			PhaseThreeB,
			[]string{"design.md"},
			"review-design.md",
		), nil
	}

	// Decision 18 — Design review verdict branch
	verdict, _, err := e.verdictReader(reviewPath)
	if err != nil {
		return Action{}, fmt.Errorf("handlePhaseThreeB: read verdict: %w", err)
	}

	// Decision 21 — Retry limit
	if st.Revisions.DesignRevisions >= 2 {
		return NewCheckpointAction(
			"design-retry-limit",
			"Design revision limit reached (2 retries). Human review required.",
			[]string{"approve", "abandon"},
		), nil
	}

	// Decision 20 — Auto-approve gate
	if st.AutoApprove && (verdict == VerdictApprove || verdict == VerdictApproveWithNotes) {
		return NewSpawnAgentAction(
			agentTaskDecomposer,
			"Decompose the approved design into tasks.",
			"sonnet",
			PhaseFour,
			[]string{"design.md"},
			"tasks.md",
		), nil
	}

	switch verdict {
	case VerdictApprove, VerdictApproveWithNotes:
		return NewCheckpointAction(
			"design-approved",
			"Design review complete. Verdict: "+string(verdict)+". Proceed to task decomposition?",
			[]string{"proceed", "revise"},
		), nil
	case VerdictRevise:
		// Re-spawn architect for revision
		return NewSpawnAgentAction(
			agentArchitect,
			"Revise the design based on review feedback.",
			"sonnet",
			PhaseThree,
			[]string{"request.md", "analysis.md", "review-design.md"},
			"design.md",
		), nil
	default:
		return NewCheckpointAction(
			"design-review-unknown",
			"Design review verdict is unknown or unrecognised: "+string(verdict)+". Human review required.",
			[]string{"approve", "revise", "abandon"},
		), nil
	}
}

// handleCheckpointA handles checkpoint-a (between design review and task decomposition).
func (*Engine) handleCheckpointA(_ *state.State) (Action, error) {
	return NewCheckpointAction(
		"checkpoint-a",
		"Checkpoint A reached. Design approved. Proceed to Phase 4 (task decomposition)?",
		[]string{"proceed", "revise", "abandon"},
	), nil
}

// handlePhaseFour handles Phase 4 (task decomposer).
func (*Engine) handlePhaseFour(_ *state.State) (Action, error) {
	return NewSpawnAgentAction(
		agentTaskDecomposer,
		"Decompose the design into implementation tasks.",
		"sonnet",
		PhaseFour,
		[]string{"design.md"},
		"tasks.md",
	), nil
}

// handlePhaseFourB handles Phase 4b (task reviewer) — Decision 19.
func (e *Engine) handlePhaseFourB(st *state.State) (Action, error) {
	reviewPath := filepath.Join(st.Workspace, "review-tasks.md")

	// If review file doesn't exist yet, spawn the task reviewer.
	if _, err := os.Stat(reviewPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return Action{}, fmt.Errorf("handlePhaseFourB: stat %s: %w", reviewPath, err)
		}
		return NewSpawnAgentAction(
			agentTaskReviewer,
			"Review the task decomposition.",
			"sonnet",
			PhaseFourB,
			[]string{"tasks.md"},
			"review-tasks.md",
		), nil
	}

	// Decision 19 — Task review verdict branch
	verdict, _, err := e.verdictReader(reviewPath)
	if err != nil {
		return Action{}, fmt.Errorf("handlePhaseFourB: read verdict: %w", err)
	}

	// Decision 21 — Retry limit
	if st.Revisions.TaskRevisions >= 2 {
		return NewCheckpointAction(
			"task-retry-limit",
			"Task revision limit reached (2 retries). Human review required.",
			[]string{"approve", "abandon"},
		), nil
	}

	// Decision 20 — Auto-approve gate
	if st.AutoApprove && (verdict == VerdictApprove || verdict == VerdictApproveWithNotes) {
		return NewSpawnAgentAction(
			agentImplementer,
			"Implement the tasks as decomposed.",
			"sonnet",
			PhaseFive,
			[]string{"tasks.md", "design.md"},
			"",
		), nil
	}

	switch verdict {
	case VerdictApprove, VerdictApproveWithNotes:
		return NewCheckpointAction(
			"tasks-approved",
			"Task review complete. Verdict: "+string(verdict)+". Proceed to implementation?",
			[]string{"proceed", "revise"},
		), nil
	case VerdictRevise:
		// Re-spawn task decomposer for revision
		return NewSpawnAgentAction(
			agentTaskDecomposer,
			"Revise task decomposition based on review feedback.",
			"sonnet",
			PhaseFour,
			[]string{"design.md", "review-tasks.md"},
			"tasks.md",
		), nil
	default:
		return NewCheckpointAction(
			"task-review-unknown",
			"Task review verdict is unknown or unrecognised: "+string(verdict)+". Human review required.",
			[]string{"approve", "revise", "abandon"},
		), nil
	}
}

// handleCheckpointB handles checkpoint-b (between task review and implementation).
func (*Engine) handleCheckpointB(_ *state.State) (Action, error) {
	return NewCheckpointAction(
		"checkpoint-b",
		"Checkpoint B reached. Tasks approved. Proceed to Phase 5 (implementation)?",
		[]string{"proceed", "revise", "abandon"},
	), nil
}

// handlePhaseFive handles Phase 5 (implementation) — Decisions 22, 27, 28.
// Pre-conditions are checked via setup exec actions (SetupOnly=true):
//   - Decision 27: if st.Tasks is empty, emit task_init setup action.
//   - Decision 28: if st.Branch is nil and not using current branch, emit create_branch setup action.
//
// Setup exec actions are reported with setup_only=true so pipeline_report_result
// records phase-log but does NOT call PhaseComplete. The engine re-enters on
// the next pipeline_next_action call to check the next pre-condition.
func (*Engine) handlePhaseFive(st *state.State) (Action, error) {
	// Decision 27 — task_init setup
	if len(st.Tasks) == 0 {
		return NewSetupExecAction(PhaseFive, []string{"task_init", st.Workspace}), nil
	}

	// Decision 28 — Branch creation setup
	if st.Branch == nil && !st.UseCurrentBranch {
		return NewSetupExecAction(PhaseFive, []string{"create_branch", deriveBranchName(st)}), nil
	}

	// Decision 22 — Phase 5 parallel/sequential ordering
	taskKeys := sortedTaskKeys(st.Tasks)
	if len(taskKeys) == 0 {
		// All tasks removed after init (edge case); advance
		return NewDoneAction(SkipSummaryPrefix+PhaseFive, ""), nil
	}

	// Find pending tasks (not yet completed)
	var pendingKeys []string
	for _, k := range taskKeys {
		task := st.Tasks[k]
		if task.ImplStatus != "completed" {
			pendingKeys = append(pendingKeys, k)
		}
	}

	if len(pendingKeys) == 0 {
		// All tasks completed; move on
		return NewDoneAction(SkipSummaryPrefix+PhaseFive, ""), nil
	}

	// Detect parallel groups: consecutive tasks with executionMode == "parallel"
	// Check if the first pending task is parallel
	firstKey := pendingKeys[0]
	firstTask := st.Tasks[firstKey]

	if firstTask.ExecutionMode == "parallel" {
		// Collect all consecutive parallel tasks from the start
		var parallelKeys []string
		for _, k := range pendingKeys {
			if st.Tasks[k].ExecutionMode == "parallel" {
				parallelKeys = append(parallelKeys, k)
			} else {
				break
			}
		}
		return NewParallelSpawnAction(
			agentImplementer,
			"Implement tasks in parallel.",
			"sonnet",
			PhaseFive,
			[]string{"tasks.md", "design.md"},
			parallelKeys,
		), nil
	}

	// Sequential: spawn first pending task
	return NewSpawnAgentAction(
		agentImplementer,
		"Implement task "+firstKey+".",
		"sonnet",
		PhaseFive,
		[]string{"tasks.md", "design.md"},
		"impl-"+firstKey+".md",
	), nil
}

// handlePhaseSix handles Phase 6 (impl reviewer) — Decision 23.
func (e *Engine) handlePhaseSix(st *state.State) (Action, error) {
	// Decision 23 — Phase 6 PASS/FAIL retry
	taskKeys := sortedTaskKeys(st.Tasks)

	for _, k := range taskKeys {
		task := st.Tasks[k]
		// Find tasks that have been implemented but not reviewed
		if task.ImplStatus == "completed" && task.ReviewStatus == "" {
			reviewFile := filepath.Join(st.Workspace, "review-"+k+".md")

			// If review file doesn't exist, spawn reviewer
			if _, err := os.Stat(reviewFile); err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					return Action{}, fmt.Errorf("handlePhaseSix: stat %s: %w", reviewFile, err)
				}
				return NewSpawnAgentAction(
					agentImplReviewer,
					"Review implementation for task "+k+".",
					"sonnet",
					PhaseSix,
					[]string{"impl-" + k + ".md", "tasks.md"},
					"review-"+k+".md",
				), nil
			}

			// Read verdict
			verdict, _, err := e.verdictReader(reviewFile)
			if err != nil {
				return Action{}, fmt.Errorf("handlePhaseSix: read verdict for task %s: %w", k, err)
			}

			if verdict == VerdictFail {
				// Check retry limit
				if task.ImplRetries >= 2 {
					return NewCheckpointAction(
						"impl-retry-limit-"+k,
						"Implementation retry limit reached for task "+k+" (2 retries). Human review required.",
						[]string{"approve", "abandon"},
					), nil
				}
				// Retry implementation
				return NewSpawnAgentAction(
					agentImplementer,
					"Retry implementation for task "+k+" after review failure.",
					"sonnet",
					PhaseFive,
					[]string{"tasks.md", "design.md", "review-" + k + ".md"},
					"impl-"+k+".md",
				), nil
			}
			// VerdictPass, VerdictPassWithNotes, or any other passing verdict:
			// task is considered reviewed; continue to next task in loop.
		}
	}

	// All tasks reviewed; proceed
	return NewSpawnAgentAction(
		agentVerifier,
		"Verify all implementations.",
		"sonnet",
		PhaseSeven,
		[]string{"tasks.md"},
		"verification.md",
	), nil
}

// handlePhaseSeven handles Phase 7 (comprehensive reviewer).
// Phase 7 performs holistic cross-task review (naming consistency, duplication,
// interface coherence). The final-verification phase handles build/test verification.
func (*Engine) handlePhaseSeven(_ *state.State) (Action, error) {
	return NewSpawnAgentAction(
		agentComprehensiveReview,
		"Run comprehensive cross-task review.",
		"sonnet",
		PhaseSeven,
		[]string{"design.md", "tasks.md"},
		"comprehensive-review.md",
	), nil
}

// handleFinalVerification handles the final-verification phase.
func (*Engine) handleFinalVerification(_ *state.State) (Action, error) {
	return NewSpawnAgentAction(
		agentVerifier,
		"Run final verification of the complete pipeline output.",
		"sonnet",
		PhaseFinalVerification,
		[]string{"tasks.md", "design.md"},
		"final-verification.md",
	), nil
}

// handlePRCreation handles the pr-creation phase — Decision 24.
func (*Engine) handlePRCreation(st *state.State) (Action, error) {
	// Decision 24 — PR skip (runtime SkipPr flag)
	// Note: Decision 14 already handles the case where pr-creation is in SkippedPhases.
	if st.SkipPr {
		return NewDoneAction(SkipSummaryPrefix+"pr-creation", ""), nil
	}

	title := derivePRTitle(st)
	bodyFile := filepath.Join(st.Workspace, "final-summary.md")

	return NewExecAction(PhasePRCreation, []string{
		"gh", "pr", "create",
		"--title", title,
		"--body-file", bodyFile,
	}), nil
}

// derivePRTitle generates a meaningful PR title from state context.
// It uses the task type prefix and spec name as a basis.
func derivePRTitle(st *state.State) string {
	prefix := "feat"
	if st.TaskType != nil {
		switch *st.TaskType {
		case TaskTypeBugfix:
			prefix = "fix"
		case TaskTypeDocs:
			prefix = "docs"
		case TaskTypeRefactor:
			prefix = "refactor"
		case TaskTypeInvestigation:
			prefix = "chore"
		}
	}

	name := stripDatePrefix(st.SpecName)
	// Replace hyphens with spaces for readability in PR titles.
	name = strings.ReplaceAll(name, "-", " ")

	return prefix + ": " + name
}

// handleFinalSummary handles the final-summary phase — Decision 25.
func (*Engine) handleFinalSummary(st *state.State) (Action, error) {
	// Decision 25 — Final Summary template
	taskType := TaskTypeFeature
	if st.TaskType != nil {
		taskType = *st.TaskType
	}

	switch taskType {
	case TaskTypeBugfix, TaskTypeDocs:
		// summary reads review-{N}.md files; no comprehensive-review.md
		taskKeys := sortedTaskKeys(st.Tasks)
		inputFiles := make([]string, 0, len(taskKeys))
		for _, k := range taskKeys {
			inputFiles = append(inputFiles, "review-"+k+".md")
		}
		return NewSpawnAgentAction(
			agentVerifier,
			"Generate final summary for "+taskType+" task.",
			"sonnet",
			PhaseFinalSummary,
			inputFiles,
			"final-summary.md",
		), nil
	case TaskTypeInvestigation:
		// summary reads analysis.md and investigation.md
		return NewSpawnAgentAction(
			agentVerifier,
			"Generate final summary for investigation task.",
			"sonnet",
			PhaseFinalSummary,
			[]string{"analysis.md", "investigation.md"},
			"final-summary.md",
		), nil
	default:
		// feature, refactor, default: phase-7 already ran comprehensive review,
		// so final-summary just generates a summary document from its output.
		return NewSpawnAgentAction(
			agentVerifier,
			"Generate final summary with pipeline statistics.",
			"sonnet",
			PhaseFinalSummary,
			[]string{"comprehensive-review.md", "design.md", "tasks.md"},
			"final-summary.md",
		), nil
	}
}

// handlePostToSource handles the post-to-source phase — Decision 26.
func (e *Engine) handlePostToSource(st *state.State) (Action, error) {
	// Decision 26 — Post-to-source dispatch
	sourceType := e.sourceTypeReader(st.Workspace)

	switch sourceType {
	case "github_issue":
		return NewExecAction(PhasePostToSource, []string{
			"gh", "issue", "comment",
			"--body-file", filepath.Join(st.Workspace, "final-summary.md"),
		}), nil
	case "jira_issue":
		return NewCheckpointAction(
			"post-to-jira",
			"Post the final summary to the Jira issue. Review final-summary.md and post manually.",
			[]string{"done"},
		), nil
	default: // "text" and anything else
		return NewDoneAction("no external posting needed (source_type="+sourceType+")", ""), nil
	}
}

// readSourceType reads the source_type field from {workspace}/request.md front matter.
// Returns "text" when the field is absent or the file is unreadable.
func readSourceType(workspace string) string {
	path := filepath.Join(workspace, "request.md")
	f, err := os.Open(path)
	if err != nil {
		return "text"
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	inFrontMatter := false

	for scanner.Scan() {
		line := scanner.Text()
		if line == "---" {
			if !inFrontMatter {
				inFrontMatter = true
				continue
			}
			// Second --- ends front matter
			break
		}
		if inFrontMatter {
			if val, ok := strings.CutPrefix(line, "source_type:"); ok {
				val = strings.TrimSpace(val)
				if val != "" {
					return val
				}
			}
		}
	}

	return "text"
}

// deriveBranchName generates a deterministic branch name from the spec name.
// It strips the date prefix (e.g., "20260330-") and truncates to 60 characters
// to produce readable branch names like "forge/soa-2899-task-status-options".
func deriveBranchName(st *state.State) string {
	name := stripDatePrefix(st.SpecName)
	name = strings.ToLower(strings.ReplaceAll(name, " ", "-"))

	// Truncate to 60 characters at a word boundary for readability.
	const maxLen = 60
	if len(name) > maxLen {
		name = name[:maxLen]
		if idx := strings.LastIndex(name, "-"); idx > 0 {
			name = name[:idx]
		}
	}

	return "forge/" + name
}

// stripDatePrefix removes a leading "YYYYMMDD-" date prefix from a spec name.
// Returns the input unchanged if no date prefix is found.
func stripDatePrefix(name string) string {
	if len(name) > 9 && name[8] == '-' {
		allDigits := true
		for _, c := range name[:8] {
			if c < '0' || c > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			return name[9:]
		}
	}
	return name
}

// sortedTaskKeys returns task keys from tasks sorted numerically ascending.
// Needed for deterministic Phase 5 ordering because Go map iteration is unordered.
// Keys that cannot be parsed as integers are sorted lexicographically after numeric keys.
func sortedTaskKeys(tasks map[string]state.Task) []string {
	keys := make([]string, 0, len(tasks))
	for k := range tasks {
		keys = append(keys, k)
	}

	sort.Slice(keys, func(i, j int) bool {
		ni, errI := strconv.Atoi(keys[i])
		nj, errJ := strconv.Atoi(keys[j])
		if errI == nil && errJ == nil {
			return ni < nj
		}
		// Non-numeric keys sort after numeric keys
		if errI == nil {
			return true
		}
		if errJ == nil {
			return false
		}
		// Both non-numeric: lexicographic
		return keys[i] < keys[j]
	})

	return keys
}

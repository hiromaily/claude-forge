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
	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
)

// Agent name constants — unexported; used only inside NextAction dispatch.
const (
	agentSituationAnalyst    = "situation-analyst"
	agentAnalystInvestigator = "situation-analyst-investigator"
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

// minimalTasksContent is written to tasks.md when Phase 4 (task decomposition) is
// skipped for the light/S effort template. It creates a single sequential task
// so that task_init can populate state and the implementer has a task to run.
const minimalTasksContent = "# Tasks\n\n## Task 1: Implement\n\nApply all changes described in design.md as a single implementation unit.\n\nmode: sequential\n"

// Engine computes the next pipeline action from the current state.
// verdictReader and sourceTypeReader are injectable for testing;
// NewEngine sets them to the production implementations.
type Engine struct {
	agentDir         string // reserved for future agent .md file resolution; not read by NextAction
	specsDir         string
	verdictReader    func(path string) (Verdict, []Finding, error)
	sourceTypeReader func(workspace string) string
	sourceURLReader  func(workspace string) string
}

// NewEngine constructs a ready-to-use Engine with production I/O implementations.
// agentDir and specsDir are stored as-is; no path existence validation is performed.
func NewEngine(agentDir, specsDir string) *Engine {
	return &Engine{
		agentDir:         agentDir,
		specsDir:         specsDir,
		verdictReader:    ParseVerdict,
		sourceTypeReader: readSourceType,
		sourceURLReader:  readSourceURL,
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
	case PhaseFinalCommit:
		return e.handleFinalCommit(st)
	case PhaseCompleted:
		return NewDoneAction("pipeline completed", ""), nil
	default:
		return Action{}, fmt.Errorf("NextAction: unknown phase %q", phase)
	}
}

// handlePhaseOne handles Phase 1 (situation analysis).
// When Phase 2 is in SkippedPhases (light/S template), dispatches the combined
// analyst-investigator agent so both analyses are written to analysis.md in one pass.
func (*Engine) handlePhaseOne(st *state.State) (Action, error) {
	if slices.Contains(st.SkippedPhases, PhaseTwo) {
		return NewSpawnAgentAction(
			agentAnalystInvestigator,
			"Run Phase 1 combined situation analysis and investigation.",
			state.DefaultModel,
			PhaseOne,
			[]string{state.ArtifactRequest},
			state.ArtifactAnalysis,
		), nil
	}
	return NewSpawnAgentAction(
		agentSituationAnalyst,
		"Run Phase 1 situation analysis for the pipeline.",
		state.DefaultModel,
		PhaseOne,
		[]string{state.ArtifactRequest},
		state.ArtifactAnalysis,
	), nil
}

// handlePhaseTwo handles Phase 2 (investigator).
func (*Engine) handlePhaseTwo(_ *state.State) (Action, error) {
	return NewSpawnAgentAction(
		agentInvestigator,
		"Run Phase 2 deep-dive investigation.",
		state.DefaultModel,
		PhaseTwo,
		[]string{state.ArtifactRequest, state.ArtifactAnalysis},
		state.ArtifactInvestigation,
	), nil
}

// handlePhaseThree handles Phase 3 (architect).
// investigation.md is included in inputs only when Phase 2 was not skipped
// (it is absent when Phase 2 is in SkippedPhases, e.g. light/S template).
func (*Engine) handlePhaseThree(st *state.State) (Action, error) {
	inputFiles := []string{state.ArtifactRequest, state.ArtifactAnalysis}
	if !slices.Contains(st.SkippedPhases, PhaseTwo) {
		inputFiles = append(inputFiles, state.ArtifactInvestigation)
	}
	return NewSpawnAgentAction(
		agentArchitect,
		"Run Phase 3 architecture/design.",
		state.DefaultModel,
		PhaseThree,
		inputFiles,
		state.ArtifactDesign,
	), nil
}

// handlePhaseThreeB handles Phase 3b (design reviewer) — Decision 18.
func (e *Engine) handlePhaseThreeB(st *state.State) (Action, error) {
	reviewPath := filepath.Join(st.Workspace, state.ArtifactReviewDesign)

	// If review file doesn't exist yet, spawn the design reviewer.
	if _, err := os.Stat(reviewPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return Action{}, fmt.Errorf("handlePhaseThreeB: stat %s: %w", reviewPath, err)
		}
		return NewSpawnAgentAction(
			agentDesignReviewer,
			"Review the design document.",
			state.DefaultModel,
			PhaseThreeB,
			[]string{state.ArtifactDesign},
			state.ArtifactReviewDesign,
		), nil
	}

	// Decision 18 — Design review verdict branch
	verdict, _, err := e.verdictReader(reviewPath)
	if err != nil {
		return Action{}, fmt.Errorf("handlePhaseThreeB: read verdict: %w", err)
	}

	// Decision 21 — Retry limit
	if st.Revisions.DesignRevisions >= state.MaxRevisionRetries {
		return NewCheckpointAction(
			"design-retry-limit",
			fmt.Sprintf("Design revision limit reached (%d retries). Human review required.", state.MaxRevisionRetries),
			[]string{"approve", "abandon"},
		), nil
	}

	// Decision 20 — Auto-approve gate
	if st.AutoApprove && (verdict == VerdictApprove || verdict == VerdictApproveWithNotes) {
		if slices.Contains(st.SkippedPhases, PhaseFour) {
			// Phase 4 is skipped; complete phase-3b and let checkpoint-a handle auto-approval.
			return NewDoneAction(SkipSummaryPrefix+PhaseThreeB, ""), nil
		}
		return NewSpawnAgentAction(
			agentTaskDecomposer,
			"Decompose the approved design into tasks.",
			state.DefaultModel,
			PhaseFour,
			[]string{state.ArtifactDesign},
			state.ArtifactTasks,
		), nil
	}

	switch verdict {
	case VerdictApprove, VerdictApproveWithNotes:
		nextStep := "task decomposition"
		if slices.Contains(st.SkippedPhases, PhaseFour) {
			nextStep = "implementation"
		}
		return NewCheckpointAction(
			"design-approved",
			"Design review complete. Verdict: "+string(verdict)+". Proceed to "+nextStep+"?",
			[]string{"proceed", "revise"},
		), nil
	case VerdictRevise:
		// Re-spawn architect for revision
		return NewSpawnAgentAction(
			agentArchitect,
			"Revise the design based on review feedback.",
			state.DefaultModel,
			PhaseThree,
			[]string{state.ArtifactRequest, state.ArtifactAnalysis, state.ArtifactReviewDesign},
			state.ArtifactDesign,
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
// When phase-4 is skipped and auto-approve is on, the checkpoint is auto-skipped.
// When phase-4 is skipped, the checkpoint message refers to Phase 5 instead of Phase 4.
func (*Engine) handleCheckpointA(st *state.State) (Action, error) {
	// When phase-4 is skipped and auto-approve is on, skip this checkpoint too.
	if slices.Contains(st.SkippedPhases, PhaseFour) && st.AutoApprove {
		return NewDoneAction(SkipSummaryPrefix+PhaseCheckpointA, ""), nil
	}
	msg := "Checkpoint A reached. Design approved. Proceed to Phase 4 (task decomposition)?"
	if slices.Contains(st.SkippedPhases, PhaseFour) {
		msg = "Checkpoint A reached. Design approved. Proceed to Phase 5 (implementation)?"
	}
	return NewCheckpointAction(
		PhaseCheckpointA,
		msg,
		[]string{"proceed", "revise", "abandon"},
	), nil
}

// handlePhaseFour handles Phase 4 (task decomposer).
func (*Engine) handlePhaseFour(_ *state.State) (Action, error) {
	return NewSpawnAgentAction(
		agentTaskDecomposer,
		"Decompose the design into implementation tasks.",
		state.DefaultModel,
		PhaseFour,
		[]string{state.ArtifactDesign},
		state.ArtifactTasks,
	), nil
}

// handlePhaseFourB handles Phase 4b (task reviewer) — Decision 19.
func (e *Engine) handlePhaseFourB(st *state.State) (Action, error) {
	reviewPath := filepath.Join(st.Workspace, state.ArtifactReviewTasks)

	// If review file doesn't exist yet, spawn the task reviewer.
	if _, err := os.Stat(reviewPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return Action{}, fmt.Errorf("handlePhaseFourB: stat %s: %w", reviewPath, err)
		}
		return NewSpawnAgentAction(
			agentTaskReviewer,
			"Review the task decomposition.",
			state.DefaultModel,
			PhaseFourB,
			[]string{state.ArtifactTasks},
			state.ArtifactReviewTasks,
		), nil
	}

	// Decision 19 — Task review verdict branch
	verdict, _, err := e.verdictReader(reviewPath)
	if err != nil {
		return Action{}, fmt.Errorf("handlePhaseFourB: read verdict: %w", err)
	}

	// Decision 21 — Retry limit
	if st.Revisions.TaskRevisions >= state.MaxRevisionRetries {
		return NewCheckpointAction(
			"task-retry-limit",
			fmt.Sprintf("Task revision limit reached (%d retries). Human review required.", state.MaxRevisionRetries),
			[]string{"approve", "abandon"},
		), nil
	}

	// Decision 20 — Auto-approve gate
	if st.AutoApprove && (verdict == VerdictApprove || verdict == VerdictApproveWithNotes) {
		return NewSpawnAgentAction(
			agentImplementer,
			"Implement the tasks as decomposed.",
			state.DefaultModel,
			PhaseFive,
			[]string{state.ArtifactTasks, state.ArtifactDesign},
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
			state.DefaultModel,
			PhaseFour,
			[]string{state.ArtifactDesign, state.ArtifactReviewTasks},
			state.ArtifactTasks,
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
		PhaseCheckpointB,
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
	// Decision 27 — task setup
	if len(st.Tasks) == 0 {
		// When phase-4 is skipped, write a minimal tasks.md before task_init.
		if slices.Contains(st.SkippedPhases, PhaseFour) {
			tasksPath := filepath.Join(st.Workspace, state.ArtifactTasks)
			if _, err := os.Stat(tasksPath); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return Action{
						Type:      ActionWriteFile,
						Phase:     PhaseFive,
						Path:      tasksPath,
						Content:   minimalTasksContent,
						SetupOnly: true,
					}, nil
				}
				return Action{}, fmt.Errorf("handlePhaseFive: stat %s: %w", tasksPath, err)
			}
		}
		return NewSetupExecAction(PhaseFive, []string{"task_init", st.Workspace}), nil
	}

	// Decision 28 — removed: branch creation now happens during initialisation
	// (pipeline_init_with_context returns the branch name for immediate creation).
	// If branch is still nil here (legacy state or UseCurrentBranch), it is not
	// an error — the orchestrator already handles the branch setup.

	// Decision 29 — Batch commit after parallel tasks complete
	if st.NeedsBatchCommit {
		return NewSetupExecAction(PhaseFive, []string{"batch_commit"}), nil
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
		if task.ImplStatus != state.TaskStatusCompleted {
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

	if firstTask.ExecutionMode == state.ExecModeParallel {
		// Collect all consecutive parallel tasks from the start
		var parallelKeys []string
		for _, k := range pendingKeys {
			if st.Tasks[k].ExecutionMode == state.ExecModeParallel {
				parallelKeys = append(parallelKeys, k)
			} else {
				break
			}
		}
		return NewParallelSpawnAction(
			agentImplementer,
			"Implement tasks in parallel.",
			state.DefaultModel,
			PhaseFive,
			[]string{state.ArtifactTasks, state.ArtifactDesign},
			parallelKeys,
		), nil
	}

	// Sequential: spawn first pending task
	return NewSpawnAgentAction(
		agentImplementer,
		"Implement task "+firstKey+".",
		state.DefaultModel,
		PhaseFive,
		[]string{state.ArtifactTasks, state.ArtifactDesign},
		"impl-"+firstKey+".md",
	), nil
}

// handlePhaseSix handles Phase 6 (impl reviewer) — Decision 23.
//
// Task lifecycle within Phase 6:
//   - ReviewStatus ""                     → no review file → spawn reviewer
//   - ReviewStatus ""       + file exists → read verdict (transitional state):
//   - PASS/PASS_WITH_NOTES → pipeline_report_result will set ReviewStatus
//   - FAIL → dispatch implementer retry with review file as context
//   - ReviewStatus "completed_fail"        → pipeline_report_result set this;
//     dispatch implementer retry using review file (idempotent via state guard)
//   - ReviewStatus "completed_pass"/"completed_pass_with_notes" → skip (done)
func (e *Engine) handlePhaseSix(st *state.State) (Action, error) {
	// Decision 23 — Phase 6 PASS/FAIL retry
	taskKeys := sortedTaskKeys(st.Tasks)

	for _, k := range taskKeys {
		task := st.Tasks[k]

		// Skip tasks that are not yet implemented.
		if task.ImplStatus != state.TaskStatusCompleted {
			continue
		}

		// Skip tasks that are already reviewed and passing.
		if task.ReviewStatus == state.TaskStatusCompletedPass ||
			task.ReviewStatus == state.TaskStatusCompletedPassNote {
			continue
		}

		reviewFile := filepath.Join(st.Workspace, "review-"+k+".md")

		// Task was reviewed and failed — pipeline_report_result has already
		// incremented ImplRetries and set ReviewStatus. Dispatch the retry
		// without re-reading the file (idempotent: state is the guard).
		if task.ReviewStatus == state.TaskStatusCompletedFail {
			if task.ImplRetries >= state.MaxRevisionRetries {
				return NewCheckpointAction(
					"impl-retry-limit-"+k,
					fmt.Sprintf("Implementation retry limit reached for task %s (%d retries). Human review required.", k, state.MaxRevisionRetries),
					[]string{"approve", "abandon"},
				), nil
			}
			// Include review-k.md so the implementer can read the reviewer's feedback.
			return NewSpawnAgentAction(
				agentImplementer,
				"Retry implementation for task "+k+" after review failure.",
				state.DefaultModel,
				PhaseFive,
				[]string{state.ArtifactTasks, state.ArtifactDesign, "review-" + k + ".md"},
				"impl-"+k+".md",
			), nil
		}

		// If review file doesn't exist, spawn reviewer.
		if _, err := os.Stat(reviewFile); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return Action{}, fmt.Errorf("handlePhaseSix: stat %s: %w", reviewFile, err)
			}
			return NewSpawnAgentAction(
				agentImplReviewer,
				"Review implementation for task "+k+".",
				state.DefaultModel,
				PhaseSix,
				[]string{"impl-" + k + ".md", state.ArtifactTasks},
				"review-"+k+".md",
			), nil
		}

		// Review file exists but ReviewStatus not yet set — transitional state
		// (pipeline_report_result not yet called after this reviewer run).
		// Read verdict as fallback so the orchestrator is never left waiting.
		verdict, _, err := e.verdictReader(reviewFile)
		if err != nil {
			return Action{}, fmt.Errorf("handlePhaseSix: read verdict for task %s: %w", k, err)
		}

		if verdict == VerdictFail {
			// pipeline_report_result will increment ImplRetries and set
			// ReviewStatus = "completed_fail" when called. Use the current
			// counter to guard the retry limit conservatively.
			if task.ImplRetries >= state.MaxRevisionRetries {
				return NewCheckpointAction(
					"impl-retry-limit-"+k,
					fmt.Sprintf("Implementation retry limit reached for task %s (%d retries). Human review required.", k, state.MaxRevisionRetries),
					[]string{"approve", "abandon"},
				), nil
			}
			// Include review-k.md so the implementer can read the feedback.
			return NewSpawnAgentAction(
				agentImplementer,
				"Retry implementation for task "+k+" after review failure.",
				state.DefaultModel,
				PhaseFive,
				[]string{state.ArtifactTasks, state.ArtifactDesign, "review-" + k + ".md"},
				"impl-"+k+".md",
			), nil
		}
		// VerdictPass, VerdictPassWithNotes, or any other passing verdict:
		// task is considered reviewed. This is recorded in state by
		// pipeline_report_result so the engine never re-processes this task.
	}

	// All tasks reviewed; proceed.
	return NewSpawnAgentAction(
		agentVerifier,
		"Verify all implementations.",
		state.DefaultModel,
		PhaseSeven,
		[]string{state.ArtifactTasks},
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
		state.DefaultModel,
		PhaseSeven,
		[]string{state.ArtifactDesign, state.ArtifactTasks},
		state.ArtifactComprehensiveReview,
	), nil
}

// handleFinalVerification handles the final-verification phase.
func (*Engine) handleFinalVerification(_ *state.State) (Action, error) {
	return NewSpawnAgentAction(
		agentVerifier,
		"Run final verification of the complete pipeline output.",
		state.DefaultModel,
		PhaseFinalVerification,
		[]string{state.ArtifactTasks, state.ArtifactDesign},
		state.ArtifactFinalVerification,
	), nil
}

// handlePRCreation handles the pr-creation phase — Decision 24.
func (*Engine) handlePRCreation(st *state.State) (Action, error) {
	// Decision 24 — PR skip (runtime SkipPr flag)
	// Note: Decision 14 already handles the case where pr-creation is in SkippedPhases.
	if st.SkipPr {
		return NewDoneAction(SkipSummaryPrefix+PhasePRCreation, ""), nil
	}

	title := derivePRTitle(st)
	bodyFile := filepath.Join(st.Workspace, state.ArtifactSummary)

	return NewExecAction(PhasePRCreation, []string{
		"gh", "pr", "create",
		"--title", title,
		"--body-file", bodyFile,
	}), nil
}

// derivePRTitle generates a meaningful PR title from state context.
// The commit-type prefix is derived from the branch name prefix
// (feature/ → feat, fix/ → fix, refactor/ → refactor, docs/ → docs, chore/ → chore).
// Falls back to "feat" for unrecognised or absent branch prefixes.
func derivePRTitle(st *state.State) string {
	prefix := "feat"
	if st.Branch != nil {
		branch := *st.Branch
		switch {
		case strings.HasPrefix(branch, "fix/"):
			prefix = "fix"
		case strings.HasPrefix(branch, "refactor/"):
			prefix = "refactor"
		case strings.HasPrefix(branch, "docs/"):
			prefix = "docs"
		case strings.HasPrefix(branch, "chore/"):
			prefix = "chore"
		case strings.HasPrefix(branch, "feature/"):
			prefix = "feat"
		}
	}

	name := stripDatePrefix(st.SpecName)
	// Replace hyphens with spaces for readability in PR titles.
	name = strings.ReplaceAll(name, "-", " ")

	return prefix + ": " + name
}

// handleFinalSummary handles the final-summary phase — Decision 25.
// comprehensive-review.md is included only when PhaseSeven was not skipped
// (i.e., the flow template is standard or full). For effort S (light template),
// PhaseSeven is skipped and the file does not exist.
// analysis.md and investigation.md are included (when present) to provide
// context for the Improvement Report epilogue.
func (*Engine) handleFinalSummary(st *state.State) (Action, error) {
	inputs := []string{state.ArtifactDesign, state.ArtifactTasks}
	if !slices.Contains(st.SkippedPhases, PhaseSeven) {
		inputs = append([]string{state.ArtifactComprehensiveReview}, inputs...)
	}

	// Include analysis.md and investigation.md for the Improvement Report.
	// These files document what information was hard to find and what
	// documentation gaps were encountered — essential for improvement proposals.
	inputs = append(inputs, state.ArtifactAnalysis, state.ArtifactInvestigation)

	return NewSpawnAgentAction(
		agentVerifier,
		"Generate final summary with pipeline statistics and improvement report.",
		state.DefaultModel,
		PhaseFinalSummary,
		inputs,
		state.ArtifactSummary,
	), nil
}

// handleFinalCommit handles the final-commit phase — Decision 27.
// This is the last phase before completed. The orchestrator calls pipeline_report_result
// FIRST (which transitions state.json to "completed"), then amends the last commit so
// that state.json is captured in its final state. Force-pushes the PR branch.
// Skipped when PR creation was skipped (--nopr or pr-creation in skippedPhases),
// since there is no commit to amend.
func (*Engine) handleFinalCommit(st *state.State) (Action, error) {
	if st.SkipPr || slices.Contains(st.SkippedPhases, PhasePRCreation) {
		return NewDoneAction(SkipSummaryPrefix+PhaseFinalCommit, ""), nil
	}

	return NewExecAction(PhaseFinalCommit, []string{
		"final_commit",
	}), nil
}

// handlePostToSource handles the post-to-source phase — Decision 26.
func (e *Engine) handlePostToSource(st *state.State) (Action, error) {
	// Decision 26 — Post-to-source dispatch
	sourceType := e.sourceTypeReader(st.Workspace)

	// Use the phase ID as checkpoint name so mcp__forge-state__checkpoint()
	// validation succeeds (it compares checkpoint name against CurrentPhase).
	// The source type (github/jira) is embedded in the message, not the name.
	var label string
	switch sourceType {
	case state.SourceTypeGitHub:
		label = "GitHub issue"
	case state.SourceTypeJira:
		label = "Jira issue"
	default: // "text" and anything else — skip this phase
		return NewDoneAction(SkipSummaryPrefix+PhasePostToSource, ""), nil
	}

	sourceURL := e.sourceURLReader(st.Workspace)
	if sourceURL == "" {
		return NewDoneAction(SkipSummaryPrefix+PhasePostToSource, ""), nil
	}

	msg := fmt.Sprintf(
		"Pipeline complete. Post the final summary as a comment to the %s?\n\nURL: %s\nSummary file: %s/%s",
		label, sourceURL, st.Workspace, state.ArtifactSummary,
	)
	return NewCheckpointAction(PhasePostToSource, msg, []string{"post", "skip"}), nil
}

// readFrontMatterField reads a named field from {workspace}/request.md YAML front matter.
// Returns fallback when the field is absent or the file is unreadable.
func readFrontMatterField(workspace, field, fallback string) string {
	path := filepath.Join(workspace, state.ArtifactRequest)
	f, err := os.Open(path)
	if err != nil {
		return fallback
	}
	defer func() { _ = f.Close() }()

	prefix := field + ":"
	scanner := bufio.NewScanner(f)
	inFrontMatter := false

	for scanner.Scan() {
		line := scanner.Text()
		if line == "---" {
			if !inFrontMatter {
				inFrontMatter = true
				continue
			}
			break
		}
		if inFrontMatter {
			if val, ok := strings.CutPrefix(line, prefix); ok {
				val = strings.TrimSpace(val)
				if val != "" {
					return val
				}
			}
		}
	}

	return fallback
}

// readSourceType reads the source_type field from {workspace}/request.md front matter.
// Returns "text" when the field is absent or the file is unreadable.
func readSourceType(workspace string) string {
	return readFrontMatterField(workspace, "source_type", state.SourceTypeText)
}

// readSourceURL reads the source_url field from {workspace}/request.md front matter.
// Returns "" when the field is absent or the file is unreadable.
func readSourceURL(workspace string) string {
	return readFrontMatterField(workspace, "source_url", "")
}

// DeriveBranchName generates a deterministic branch name from the spec name.
// It strips the date prefix (e.g., "20260330-") and truncates to 60 characters
// to produce readable branch names like "forge/soa-2899-task-status-options".
// Exported so pipeline_init_with_context can derive the branch name during
// initialisation (before Phase 5).
func DeriveBranchName(st *state.State) string {
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

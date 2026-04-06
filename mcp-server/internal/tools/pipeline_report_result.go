// Package tools — pipeline_report_result MCP handler.
// Records phase-log entry, validates artifacts, parses verdict, and advances state.
package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hiromaily/claude-forge/mcp-server/internal/history"
	"github.com/hiromaily/claude-forge/mcp-server/internal/orchestrator"
	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
	"github.com/hiromaily/claude-forge/mcp-server/internal/validation"
)

// phaseRevType maps review phases to their revision type passed to RevisionBump.
//
//nolint:gochecknoglobals // package-level lookup table for phase revision types
var phaseRevType = map[string]string{
	"phase-3b": "design",
	"phase-4b": "tasks",
}

// reviewArtifactFile maps review phases to the artifact file that contains the verdict.
//
//nolint:gochecknoglobals // package-level lookup table for review artifact files
var reviewArtifactFile = map[string]string{
	"phase-3b": "review-design.md",
	"phase-4b": "review-tasks.md",
}

// phaseAgentName maps review phases to the agent name used for pattern accumulation.
//
//nolint:gochecknoglobals // package-level lookup table for phase agent names
var phaseAgentName = map[string]string{
	"phase-3b": "design-reviewer",
	"phase-4b": "task-reviewer",
}

// reportResultResponse is the structured response returned by PipelineReportResultHandler.
type reportResultResponse struct {
	StateUpdated    bool                   `json:"state_updated"`
	ArtifactWritten string                 `json:"artifact_written"`
	VerdictParsed   string                 `json:"verdict_parsed"`
	Findings        []orchestrator.Finding `json:"findings"`
	NextActionHint  string                 `json:"next_action_hint"`
	Warning         string                 `json:"warning,omitempty"`
}

// reportResultInput collects parsed parameters from the MCP request.
type reportResultInput struct {
	workspace  string
	phase      string
	tokensUsed int
	durationMs int
	model      string
	setupOnly  bool // when true, record phase-log but skip PhaseComplete
}

// PipelineReportResultHandler handles the "pipeline_report_result" MCP tool.
// It records a phase-log entry, validates the artifact, parses any verdict,
// and advances pipeline state accordingly.
func PipelineReportResultHandler(sm *state.StateManager, kb *history.KnowledgeBase) server.ToolHandlerFunc {
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Step 1: Parse required parameters.
		workspace, phase, result, err := requireWorkspaceAndPhase(req)
		if result != nil {
			return result, err
		}

		in := reportResultInput{
			workspace:  workspace,
			phase:      phase,
			tokensUsed: req.GetInt("tokens_used", 0),
			durationMs: req.GetInt("duration_ms", 0),
			model:      req.GetString("model", ""),
			setupOnly:  req.GetBool("setup_only", false),
		}

		return handleReportResult(sm, kb, in)
	}
}

// handleReportResult performs the core logic of PipelineReportResultHandler.
// Extracted to a named function for testability.
func handleReportResult(sm *state.StateManager, kb *history.KnowledgeBase, in reportResultInput) (*mcp.CallToolResult, error) {
	var warnings []string

	// Step 2: Load state for duplicate-log check (before PhaseLog).
	s, err := loadState(in.workspace)
	if err != nil {
		return errorf("read state: %v", err)
	}
	if w := Warn3dPhaseLogDuplicate(in.phase, s); w != "" {
		warnings = append(warnings, w)
	}

	// Step 3: Record phase-log entry.
	if err := sm.PhaseLog(in.workspace, in.phase, in.tokensUsed, in.durationMs, in.model); err != nil {
		return errorf("phase_log: %v", err)
	}

	// Step 4: Validate artifacts for this phase.
	results := validation.ValidateArtifacts(in.workspace, in.phase)

	// Step 5: Process validation results.
	var artifactWritten string
	for i, result := range results {
		if strings.HasPrefix(result.Error, "unknown phase:") {
			warnings = append(warnings, "artifact validation skipped: "+result.Error)
			continue
		}
		// For phase-6, ArtifactResult.Valid=false may indicate a FAIL verdict (not a missing file).
		// Block only when there is an error string (file missing, no verdict token found).
		// ParseVerdict is the authoritative mechanism for PASS/FAIL decisions in phase-6.
		if !result.Valid && result.Error != "" {
			return errorf("artifact invalid for %s: %s", in.phase, result.Error)
		}
		// Step 6: Set artifactWritten from the first result with a File field.
		if i == 0 && result.File != "" {
			artifactWritten = result.File
		}
	}

	// Steps 7–9: Determine state transition based on phase.
	resp, err := determineTransition(sm, kb, in, results, artifactWritten, &warnings)
	if err != nil {
		return errorf("%v", err)
	}

	resp.Warning = strings.Join(warnings, "; ")
	return okJSON(resp)
}

// determineTransition decides the correct state transition and returns a partial response.
//
//nolint:gocyclo // complexity is inherent in the dispatch table
func determineTransition(
	sm *state.StateManager,
	kb *history.KnowledgeBase,
	in reportResultInput,
	results []validation.ArtifactResult,
	artifactWritten string,
	warnings *[]string,
) (reportResultResponse, error) {
	// Step 7: Review phases (phase-3b, phase-4b) — parse verdict and decide.
	if revType, ok := phaseRevType[in.phase]; ok {
		artifactFile, knownFile := reviewArtifactFile[in.phase]
		if !knownFile {
			// Fallback: complete the phase without verdict parsing.
			if err := sm.PhaseComplete(in.workspace, in.phase); err != nil {
				return reportResultResponse{}, err
			}
			return reportResultResponse{
				StateUpdated:    true,
				ArtifactWritten: artifactWritten,
				NextActionHint:  "proceed",
			}, nil
		}

		verdict, findings, err := orchestrator.ParseVerdict(filepath.Join(in.workspace, artifactFile))
		if err != nil {
			return reportResultResponse{}, err
		}

		findings = nonNilSlice(findings)

		// Accumulate review findings into the pattern knowledge base (fail-open).
		agentName := phaseAgentName[in.phase]
		if accumErr := kb.Patterns.Accumulate(findings, agentName, time.Now().UTC()); accumErr != nil {
			*warnings = append(*warnings, "pattern accumulation warning: "+accumErr.Error())
		}

		switch verdict {
		case orchestrator.VerdictRevise:
			if err := sm.RevisionBump(in.workspace, revType); err != nil {
				return reportResultResponse{}, err
			}
			return reportResultResponse{
				StateUpdated:    true,
				ArtifactWritten: artifactWritten,
				VerdictParsed:   string(verdict),
				Findings:        findings,
				NextActionHint:  "revision_required",
			}, nil
		default:
			// APPROVE, APPROVE_WITH_NOTES, or UNKNOWN — all advance the phase.
			if err := sm.PhaseComplete(in.workspace, in.phase); err != nil {
				return reportResultResponse{}, err
			}
			return reportResultResponse{
				StateUpdated:    true,
				ArtifactWritten: artifactWritten,
				VerdictParsed:   string(verdict),
				Findings:        findings,
				NextActionHint:  "proceed",
			}, nil
		}
	}

	// Step 8: Phase-6 — parse verdict from each impl-*.md.
	if in.phase == "phase-6" {
		return handlePhase6Transition(sm, in, results, artifactWritten)
	}

	// Step 9: All other phases — advance unless setup_only.
	if in.setupOnly {
		// Clear batch commit flag only after a batch_commit exec completes.
		// Other phase-5 setup actions (task_init, create_branch) must not clear it.
		s, stateErr := sm.GetState()
		if stateErr != nil {
			return reportResultResponse{}, stateErr
		}
		if in.phase == "phase-5" && s.NeedsBatchCommit {
			if updateErr := sm.Update(func(st *state.State) error {
				st.NeedsBatchCommit = false
				return nil
			}); updateErr != nil {
				return reportResultResponse{}, updateErr
			}
		}
		return reportResultResponse{
			StateUpdated:    true,
			ArtifactWritten: artifactWritten,
			NextActionHint:  "setup_continue",
		}, nil
	}

	// Step 9b: Phase-5 special handling — do not advance if pending tasks remain.
	// After a parallel batch completes, there may be sequential tasks still pending.
	// Re-enter handlePhaseFive by returning "setup_continue" instead of advancing.
	if in.phase == "phase-5" {
		// Auto-mark tasks as completed when their impl-N.md artifact exists.
		// The implementer agent writes impl-N.md but may not call task_update
		// explicitly, so we reconcile task status from artifact presence.
		// Batch all updates in a single transaction to avoid O(N) disk I/O.
		// Also detect parallel batch completion and set NeedsBatchCommit flag.
		if updateErr := sm.Update(func(st *state.State) error {
			newlyCompleted := 0
			for k, t := range st.Tasks {
				if t.ImplStatus == "completed" {
					continue
				}
				implFile := filepath.Join(in.workspace, "impl-"+k+".md")
				if _, statErr := os.Stat(implFile); statErr == nil {
					if t.ExecutionMode == "parallel" {
						newlyCompleted++
					}
					t.ImplStatus = "completed"
					st.Tasks[k] = t
				}
			}
			// If parallel tasks were just completed, signal batch commit needed.
			if newlyCompleted > 0 {
				st.NeedsBatchCommit = true
			}
			return nil
		}); updateErr != nil {
			return reportResultResponse{}, updateErr
		}

		// Re-read state after potential updates.
		s, err := sm.GetState()
		if err != nil {
			return reportResultResponse{}, err
		}
		hasPending := false
		for _, t := range s.Tasks {
			if t.ImplStatus != "completed" {
				hasPending = true
				break
			}
		}
		// Also hold in phase-5 if a batch commit is pending (e.g. last parallel
		// batch just completed — all tasks done but git commit not yet run).
		if hasPending || s.NeedsBatchCommit {
			return reportResultResponse{
				StateUpdated:    true,
				ArtifactWritten: artifactWritten,
				NextActionHint:  "setup_continue",
			}, nil
		}

		// All tasks complete — clear any completed_fail retry state so the
		// engine dispatches fresh reviewers after the retry implementer ran.
		clearCompletedFailTasks(sm, in.workspace)
	}

	if err := sm.PhaseComplete(in.workspace, in.phase); err != nil {
		return reportResultResponse{}, err
	}
	return reportResultResponse{
		StateUpdated:    true,
		ArtifactWritten: artifactWritten,
		NextActionHint:  "proceed",
	}, nil
}

// handlePhase6Transition processes phase-6 results, parsing verdicts from review-*.md
// files and deterministically updating task ReviewStatus in state.json.
//
// Key invariant: after this function returns, every task whose review-{k}.md
// contains a PASS/PASS_WITH_NOTES verdict will have its ReviewStatus set in
// state.json. This prevents the engine from re-dispatching reviewers for
// already-reviewed tasks.
//
//nolint:gocyclo // complexity is inherent in multi-task verdict reconciliation
func handlePhase6Transition(
	sm *state.StateManager,
	in reportResultInput,
	results []validation.ArtifactResult,
	artifactWritten string,
) (reportResultResponse, error) {
	allFindings := []orchestrator.Finding{}
	var verdictParsed string
	anyFail := false

	for _, result := range results {
		if result.File == "" {
			continue
		}

		verdict, findings, err := orchestrator.ParseVerdict(filepath.Join(in.workspace, result.File))
		if err != nil {
			// File I/O error — treat as fail.
			anyFail = true
			continue
		}

		if findings != nil {
			allFindings = append(allFindings, findings...)
		}

		if verdictParsed == "" {
			verdictParsed = string(verdict)
		}

		// Only VerdictFail triggers retry; VerdictPassWithNotes is treated as passing.
		if verdict == orchestrator.VerdictFail {
			anyFail = true
		}
	}

	if anyFail {
		// Deterministic: record the FAIL verdict in state so the engine can
		// dispatch the retry via ReviewStatus without re-reading the review file.
		// This also prevents double-increment if pipeline_next_action is called
		// multiple times before the orchestrator calls pipeline_report_result.
		if updateErr := sm.Update(func(st *state.State) error {
			for _, result := range results {
				if result.File == "" || result.VerdictFound != state.VerdictFail {
					continue
				}
				taskKey := reviewFileTaskKey(result.File)
				if taskKey == "" {
					continue
				}
				if t, ok := st.Tasks[taskKey]; ok {
					t.ImplRetries++
					t.ReviewStatus = state.TaskStatusCompletedFail
					st.Tasks[taskKey] = t
				}
			}
			return nil
		}); updateErr != nil {
			return reportResultResponse{}, updateErr
		}
		return reportResultResponse{
			StateUpdated:    true,
			ArtifactWritten: artifactWritten,
			VerdictParsed:   verdictParsed,
			Findings:        allFindings,
			NextActionHint:  "retry_impl",
		}, nil
	}

	// Deterministic: reconcile ReviewStatus from review file verdicts.
	// This ensures the engine (handlePhaseSix) never re-processes a task
	// whose review file already contains a passing verdict.
	if updateErr := sm.Update(func(st *state.State) error {
		for k, t := range st.Tasks {
			if t.ImplStatus != state.TaskStatusCompleted {
				continue
			}
			// Already marked as passing — skip.
			if t.ReviewStatus == state.TaskStatusCompletedPass ||
				t.ReviewStatus == state.TaskStatusCompletedPassNote {
				continue
			}
			reviewFile := filepath.Join(in.workspace, "review-"+k+".md")
			if _, statErr := os.Stat(reviewFile); statErr != nil {
				continue
			}
			verdict, _, vErr := orchestrator.ParseVerdict(reviewFile)
			if vErr != nil {
				continue
			}
			switch verdict {
			case orchestrator.VerdictPass:
				t.ReviewStatus = state.TaskStatusCompletedPass
				st.Tasks[k] = t
			case orchestrator.VerdictPassWithNotes:
				t.ReviewStatus = state.TaskStatusCompletedPassNote
				st.Tasks[k] = t
			}
		}
		return nil
	}); updateErr != nil {
		return reportResultResponse{}, updateErr
	}

	// Check whether any task still needs a review.
	s, err := sm.GetState()
	if err != nil {
		return reportResultResponse{}, err
	}
	for _, t := range s.Tasks {
		if t.ImplStatus != state.TaskStatusCompleted {
			continue
		}
		if t.ReviewStatus != state.TaskStatusCompletedPass &&
			t.ReviewStatus != state.TaskStatusCompletedPassNote {
			// Task needs review — hold in phase-6.
			return reportResultResponse{
				StateUpdated:    true,
				ArtifactWritten: artifactWritten,
				VerdictParsed:   verdictParsed,
				Findings:        allFindings,
				NextActionHint:  "setup_continue",
			}, nil
		}
	}

	if err := sm.PhaseComplete(in.workspace, in.phase); err != nil {
		return reportResultResponse{}, err
	}
	return reportResultResponse{
		StateUpdated:    true,
		ArtifactWritten: artifactWritten,
		VerdictParsed:   verdictParsed,
		Findings:        allFindings,
		NextActionHint:  "proceed",
	}, nil
}

// reviewFileTaskKey extracts the task key from a review file basename.
// e.g. "review-1.md" → "1", "review-task-abc.md" → "task-abc".
// Returns "" if the filename does not match the review-*.md or impl-*.md pattern.
func reviewFileTaskKey(filename string) string {
	base := filepath.Base(filename)
	switch {
	case strings.HasPrefix(base, "review-") && strings.HasSuffix(base, ".md"):
		return strings.TrimSuffix(strings.TrimPrefix(base, "review-"), ".md")
	case strings.HasPrefix(base, "impl-") && strings.HasSuffix(base, ".md"):
		return strings.TrimSuffix(strings.TrimPrefix(base, "impl-"), ".md")
	}
	return ""
}

// clearCompletedFailTasks resets ReviewStatus and removes stale review files for
// tasks in the "completed_fail" state. Called from the phase-5 handler after a
// retry implementer run so the engine dispatches a fresh reviewer on the next call.
// Fail-open: errors are suppressed so the phase-5 advance is not blocked.
func clearCompletedFailTasks(sm *state.StateManager, workspace string) {
	_ = sm.Update(func(st *state.State) error {
		for k, t := range st.Tasks {
			if t.ReviewStatus != state.TaskStatusCompletedFail {
				continue
			}
			// Delete the stale review file so the engine dispatches a fresh reviewer.
			reviewFile := filepath.Join(workspace, "review-"+k+".md")
			_ = os.Remove(reviewFile)
			t.ReviewStatus = ""
			st.Tasks[k] = t
		}
		return nil
	})
}

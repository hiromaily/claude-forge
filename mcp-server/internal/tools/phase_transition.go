// Package tools — phase transition helpers for pipeline_report_result.
// Contains phase-5 and phase-4 specific transition logic extracted from
// determineTransition for focused readability.
package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hiromaily/claude-forge/mcp-server/internal/orchestrator"
	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
	"github.com/hiromaily/claude-forge/mcp-server/internal/validation"
)

// handlePhase5Transition handles Step 9b and beyond in determineTransition:
// the phase-5 pending-task check, phase-4 workflow rule application, and
// the default PhaseComplete path for all other non-setup-only phases.
//
// It is called from determineTransition after the setup_only early return,
// covering phases that are not review phases and not phase-6.
//
//nolint:gocyclo // complexity is inherent in the dispatch table
func handlePhase5Transition(
	sm *state.StateManager,
	in reportResultInput,
	artifactWritten string,
) (reportResultOutcome, error) {
	// Step 9b: Phase-5 special handling — do not advance if pending tasks remain.
	// After a parallel batch completes, there may be sequential tasks still pending.
	// Re-enter handlePhaseFive by returning "setup_continue" instead of advancing.
	if in.phase == "phase-5" {
		// Auto-mark tasks as completed when their impl-N.md artifact exists.
		// The implementer agent writes impl-N.md but may not call task_update
		// explicitly, so we reconcile task status from artifact presence.
		// Pre-compute which tasks have impl files outside the lock to avoid
		// holding the state lock during disk I/O.
		preState, psErr := sm.GetState()
		if psErr != nil {
			return reportResultOutcome{}, psErr
		}
		implFileExists := make(map[string]bool, len(preState.Tasks))
		for k, t := range preState.Tasks {
			if t.ImplStatus == "completed" {
				continue
			}
			implFile := filepath.Join(in.workspace, "impl-"+k+".md")
			if _, statErr := os.Stat(implFile); statErr == nil {
				implFileExists[k] = true
			}
		}
		// Batch all updates in a single transaction; no I/O inside the lock.
		// Also detect parallel batch completion and set NeedsBatchCommit flag.
		if updateErr := sm.Update(func(st *state.State) error {
			newlyCompleted := 0
			for k, t := range st.Tasks {
				if t.ImplStatus == "completed" {
					continue
				}
				if implFileExists[k] {
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
			return reportResultOutcome{}, updateErr
		}

		// Re-read state after potential updates.
		s, err := sm.GetState()
		if err != nil {
			return reportResultOutcome{}, err
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
			return reportResultOutcome{
				StateUpdated:    true,
				ArtifactWritten: artifactWritten,
				NextActionHint:  "setup_continue",
			}, nil
		}

		// Phase 5 completion gate: verify impl file count matches task count.
		// This is a deterministic safety check — even if task status says all
		// completed, the actual impl-{N}.md files must exist for every task.
		// When missing files are found, reset ImplStatus so the engine
		// re-dispatches implementers on the next pipeline_next_action call.
		if missing := missingImplFiles(in.workspace, s.Tasks); len(missing) > 0 {
			if updateErr := sm.Update(func(st *state.State) error {
				for _, k := range missing {
					if t, ok := st.Tasks[k]; ok {
						t.ImplStatus = ""
						st.Tasks[k] = t
					}
				}
				return nil
			}); updateErr != nil {
				return reportResultOutcome{}, updateErr
			}
			return reportResultOutcome{
				StateUpdated:    true,
				ArtifactWritten: artifactWritten,
				NextActionHint:  "setup_continue",
				Warning: "phase-5 completion blocked: missing impl files for tasks: " +
					strings.Join(missing, ", ") + " — ImplStatus reset to pending",
			}, nil
		}

		// All tasks complete — clear any completed_fail retry state so the
		// engine dispatches fresh reviewers after the retry implementer ran.
		if err := clearCompletedFailTasks(sm, in.workspace); err != nil {
			return reportResultOutcome{}, err
		}
	}

	// Phase-4 (task-decomposer) completion gate: apply deterministic workflow
	// rules from .specs/instructions.md. If violations exist, write
	// review-tasks.md and emit revision_required so the engine re-dispatches
	// task-decomposer with the findings.
	if in.phase == "phase-4" {
		if resp, handled, err := applyWorkflowRules(sm, in.workspace, artifactWritten); err != nil {
			return reportResultOutcome{}, err
		} else if handled {
			return resp, nil
		}
	}

	if err := sm.PhaseComplete(in.workspace, in.phase); err != nil {
		return reportResultOutcome{}, err
	}
	return reportResultOutcome{
		StateUpdated:    true,
		ArtifactWritten: artifactWritten,
		NextActionHint:  "proceed",
	}, nil
}

// clearCompletedFailTasks resets ReviewStatus and removes stale review files for
// tasks in the "completed_fail" state. Called from the phase-5 handler after a
// retry implementer run so the engine dispatches a fresh reviewer on the next call.
func clearCompletedFailTasks(sm *state.StateManager, workspace string) error {
	// Identify stale review files before acquiring the lock.
	st, err := sm.GetState()
	if err != nil {
		return err
	}
	var filesToRemove []string
	for k, t := range st.Tasks {
		if t.ReviewStatus == state.TaskStatusCompletedFail {
			filesToRemove = append(filesToRemove, filepath.Join(workspace, "review-"+k+".md"))
		}
	}
	// Delete stale review files outside the lock so the engine dispatches fresh reviewers.
	for _, f := range filesToRemove {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	// Update state in a single transaction; no I/O inside the lock.
	return sm.Update(func(st *state.State) error {
		for k, t := range st.Tasks {
			if t.ReviewStatus != state.TaskStatusCompletedFail {
				continue
			}
			t.ReviewStatus = ""
			st.Tasks[k] = t
		}
		return nil
	})
}

// applyWorkflowRules runs .specs/instructions.md rules against the phase-4
// tasks.md output. Returns (handled=true, resp) when violations exist and
// the caller should return without calling PhaseComplete. Returns
// (handled=false, zero) in any of these pass-through cases:
//   - tasks.md cannot be read or parsed (the existing artifact validator
//     will surface the failure).
//   - workspace is not under a .specs/ directory (flat layout — workflow
//     rules do not apply).
//   - no violations were found. This also covers the "no rules file"
//     case: validation.LoadRules returns an empty rule set when
//     .specs/instructions.md is absent, so Validate yields zero
//     violations and we proceed unchanged. On the pass-through, any
//     stale review-tasks.md left behind by a previous workflow-rules
//     iteration is removed so the phase-4b task-reviewer can write a
//     fresh file.
//
// Why here and not in validation.ValidateArtifacts: that API only checks
// file presence and verdict tokens — it does not take the parsed tasks
// map or the repo root. This helper owns the specific phase-4 wiring.
//
// On the violation path, this helper bumps state.Revisions.TaskRevisions via
// sm.RevisionBump so the engine's retry-limit guard (handlePhaseFour) can
// enforce MaxRevisionRetries on the next pipeline_next_action call. This
// mirrors how VerdictRevise on phase-4b increments the same counter in
// determineTransition.
func applyWorkflowRules(sm *state.StateManager, workspace, artifactWritten string) (reportResultOutcome, bool, error) {
	tasks, rules, reviewPath, ok, err := loadPhase4Context(workspace)
	if !ok || err != nil {
		return reportResultOutcome{}, false, err
	}

	violations := validation.Validate(tasks, rules)
	if len(violations) == 0 {
		// Pass-through: ensure any stale review-tasks.md from an earlier
		// workflow-rules iteration in the same pipeline is removed so the
		// phase-4b task-reviewer (handlePhaseFourB) writes a fresh file
		// instead of reading a stale REVISE verdict and looping.
		if err := os.Remove(reviewPath); err != nil && !os.IsNotExist(err) {
			return reportResultOutcome{}, false, fmt.Errorf("remove stale %s: %w", state.ArtifactReviewTasks, err)
		}
		return reportResultOutcome{}, false, nil
	}

	resp, err := writeViolationResponse(sm, workspace, reviewPath, artifactWritten, violations)
	if err != nil {
		return reportResultOutcome{}, false, err
	}
	return resp, true, nil
}

// loadPhase4Context reads tasks.md, checks the workspace layout, and loads
// workflow rules. Returns ok=false (no error) for silent pass-through cases
// where workflow rules should not be applied.
func loadPhase4Context(workspace string) (map[string]state.Task, *validation.WorkflowRules, string, bool, error) {
	tasksData, err := os.ReadFile(filepath.Join(workspace, state.ArtifactTasks))
	if err != nil {
		// tasks.md missing: let the normal artifact validator handle it.
		return nil, nil, "", false, nil
	}

	tasks, err := ParseTasksMd(string(tasksData))
	if err != nil {
		// Let the caller fail via artifact validation or parser errors.
		return nil, nil, "", false, nil
	}

	// Workflow rules only apply when the workspace follows the
	// .specs/<spec>/ layout. Any other layout (e.g. flat test fixtures)
	// falls through silently — fail-open rather than mis-resolving the
	// repo root into an unrelated directory.
	if filepath.Base(filepath.Dir(workspace)) != ".specs" {
		return nil, nil, "", false, nil
	}
	// Repo root is two levels up from a .specs/<spec-name>/ workspace.
	repoRoot := filepath.Dir(filepath.Dir(workspace))
	rules, err := validation.LoadRules(repoRoot)
	if err != nil {
		// Rule file exists but is malformed. Surface as an error so the
		// user sees the parse failure instead of silently skipping.
		return nil, nil, "", false, fmt.Errorf("load workflow rules: %w", err)
	}

	reviewPath := filepath.Join(workspace, state.ArtifactReviewTasks)
	return tasks, rules, reviewPath, true, nil
}

// writeViolationResponse writes review-tasks.md, bumps TaskRevisions, and
// builds the reportResultOutcome for a workflow-rules violation.
//
// Order matters: write review-tasks.md FIRST, then bump TaskRevisions.
// If RevisionBump fails, the orchestrator still sees review-tasks.md on
// the next pipeline_next_action call and handlePhaseFour re-dispatches
// task-decomposer — we just lose one retry-limit tick, which is
// recoverable. Reversing the order would risk bumping the counter with
// no findings file on disk, which would waste a retry slot silently.
func writeViolationResponse(sm *state.StateManager, workspace, reviewPath, artifactWritten string, violations []validation.Violation) (reportResultOutcome, error) {
	if err := os.WriteFile(reviewPath, []byte(validation.FormatReviewFindings(violations)), 0o600); err != nil {
		return reportResultOutcome{}, fmt.Errorf("write %s: %w", state.ArtifactReviewTasks, err)
	}

	// Bump TaskRevisions so handlePhaseFour enforces MaxRevisionRetries on
	// the next pipeline_next_action call. This mirrors how determineTransition
	// bumps the same counter when phase-4b parses a REVISE verdict — keeping
	// the retry-limit enforcement in one place (the engine).
	if err := sm.RevisionBump(workspace, state.RevTypeTasks); err != nil {
		return reportResultOutcome{}, fmt.Errorf("revision bump (tasks): %w", err)
	}

	findings := make([]orchestrator.Finding, 0, len(violations))
	for _, v := range violations {
		findings = append(findings, orchestrator.Finding{
			Severity: orchestrator.SeverityCritical,
			Description: fmt.Sprintf("task %s (%s) violates rule %q: %s",
				v.TaskKey, v.TaskTitle, v.RuleID, v.Reason),
		})
	}

	return reportResultOutcome{
		StateUpdated:    true,
		ArtifactWritten: artifactWritten,
		VerdictParsed:   "REVISE",
		Findings:        findings,
		NextActionHint:  "revision_required",
		Warning: fmt.Sprintf("phase-4 workflow rules: %d violation(s) — see %s",
			len(violations), state.ArtifactReviewTasks),
	}, nil
}

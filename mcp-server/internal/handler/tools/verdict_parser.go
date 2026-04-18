// verdict parsing helpers for pipeline_report_result.
// Contains lookup maps and the functions that determine state transitions
// based on review phase verdicts.

package tools

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/orchestrator"
	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/state"
	"github.com/hiromaily/claude-forge/mcp-server/internal/handler/validation"
	"github.com/hiromaily/claude-forge/mcp-server/internal/intelligence/history"
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

// revisionPrimaryArtifact maps review phases to the primary artifact that
// the revision agent writes. Used to detect post-revision stale reviews
// via modification-time comparison.
//
//nolint:gochecknoglobals // package-level lookup table for revision primary artifacts
var revisionPrimaryArtifact = map[string]string{
	"phase-3b": state.ArtifactDesign,
	"phase-4b": state.ArtifactTasks,
}

// phaseAgentName maps review phases to the agent name used for pattern accumulation.
//
//nolint:gochecknoglobals // package-level lookup table for phase agent names
var phaseAgentName = map[string]string{
	"phase-3b": "design-reviewer",
	"phase-4b": "task-reviewer",
}

// determineTransition decides the correct state transition and returns a partial response.
func determineTransition(
	sm *state.StateManager,
	kb *history.KnowledgeBase,
	in reportResultInput,
	results []validation.ArtifactResult,
	artifactWritten string,
	warnings *[]string,
) (reportResultOutcome, error) {
	// Step 7: Review phases (phase-3b, phase-4b) — parse verdict and decide.
	if revType, ok := phaseRevType[in.phase]; ok {
		artifactFile, knownFile := reviewArtifactFile[in.phase]
		if !knownFile {
			// Fallback: complete the phase without verdict parsing.
			if err := sm.PhaseComplete(in.workspace, in.phase); err != nil {
				return reportResultOutcome{}, err
			}
			return reportResultOutcome{
				StateUpdated:    true,
				ArtifactWritten: artifactWritten,
				NextActionHint:  "proceed",
			}, nil
		}

		verdict, findings, err := orchestrator.ParseVerdict(filepath.Join(in.workspace, artifactFile))
		if err != nil {
			return reportResultOutcome{}, err
		}

		findings = nonNilSlice(findings)

		// Accumulate review findings into the pattern knowledge base (fail-open).
		agentName := phaseAgentName[in.phase]
		if accumErr := kb.Patterns.Accumulate(findings, agentName, time.Now().UTC()); accumErr != nil {
			*warnings = append(*warnings, "pattern accumulation warning: "+accumErr.Error())
		}

		switch verdict {
		case orchestrator.VerdictRevise:
			// Detect post-revision stale review: if the primary artifact
			// (design.md for phase-3b, tasks.md for phase-4b) was modified
			// more recently than the review artifact, the revision agent
			// (architect / task-decomposer) has already run. Delete the stale
			// review file and return "setup_continue" so the engine re-enters
			// handlePhaseThreeB/handlePhaseFourB and spawns the reviewer to
			// re-evaluate the revised artifact. Without this check, the engine
			// would read the stale REVISE verdict and re-dispatch the revision
			// agent in an infinite loop.
			if primaryFile, ok := revisionPrimaryArtifact[in.phase]; ok {
				reviewStat, _ := os.Stat(filepath.Join(in.workspace, artifactFile))
				primaryStat, primaryErr := os.Stat(filepath.Join(in.workspace, primaryFile))
				// The >= comparison (not strictly >) is intentional: when both
				// files share the same mtime (same-second writes on HFS+/FAT),
				// re-dispatching the reviewer (safe) is preferred over
				// re-dispatching the revision agent (infinite loop risk).
				if primaryErr == nil && reviewStat != nil && !primaryStat.ModTime().Before(reviewStat.ModTime()) {
					// Architect/decomposer already revised — delete stale review.
					reviewFilePath := filepath.Join(in.workspace, artifactFile)
					if rmErr := os.Remove(reviewFilePath); rmErr != nil {
						// Deletion failed — fall through to normal REVISE path
						// rather than returning setup_continue with the stale file
						// still on disk (which would reintroduce the infinite loop).
						*warnings = append(*warnings, "failed to remove stale review file: "+rmErr.Error())
					} else {
						return reportResultOutcome{
							StateUpdated:    true,
							ArtifactWritten: artifactWritten,
							NextActionHint:  "setup_continue",
						}, nil
					}
				}
			}

			if err := sm.RevisionBump(in.workspace, revType); err != nil {
				return reportResultOutcome{}, err
			}
			return reportResultOutcome{
				StateUpdated:    true,
				ArtifactWritten: artifactWritten,
				VerdictParsed:   string(verdict),
				Findings:        findings,
				NextActionHint:  "revision_required",
			}, nil
		default:
			// APPROVE, APPROVE_WITH_NOTES, or UNKNOWN — all advance the phase.
			if err := sm.PhaseComplete(in.workspace, in.phase); err != nil {
				return reportResultOutcome{}, err
			}
			return reportResultOutcome{
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
		return reportResultOutcome{
			StateUpdated:    true,
			ArtifactWritten: artifactWritten,
			NextActionHint:  "setup_continue",
		}, nil
	}

	return handlePhase5Transition(sm, in, artifactWritten)
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
) (reportResultOutcome, error) {
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
					// Guard against double-increment: only bump ImplRetries on first FAIL
					// transition. Subsequent pipeline_next_action calls before the phase
					// advances must not re-increment an already-failed task's counter.
					if t.ReviewStatus != state.TaskStatusCompletedFail {
						t.ImplRetries++
					}
					t.ReviewStatus = state.TaskStatusCompletedFail
					st.Tasks[taskKey] = t
				}
			}
			return nil
		}); updateErr != nil {
			return reportResultOutcome{}, updateErr
		}
		return reportResultOutcome{
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
	// Read verdicts outside the lock to avoid I/O inside a critical section.
	s, err := sm.GetState()
	if err != nil {
		return reportResultOutcome{}, err
	}
	type verdictUpdate struct {
		key    string
		status string
	}
	var verdictUpdates []verdictUpdate
	for k, t := range s.Tasks {
		if t.ImplStatus != state.TaskStatusCompleted {
			continue
		}
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
		switch verdict { //nolint:exhaustive // only PASS variants update state; FAIL/REVISE/UNKNOWN are intentionally ignored here
		case orchestrator.VerdictPass:
			verdictUpdates = append(verdictUpdates, verdictUpdate{key: k, status: state.TaskStatusCompletedPass})
		case orchestrator.VerdictPassWithNotes:
			verdictUpdates = append(verdictUpdates, verdictUpdate{key: k, status: state.TaskStatusCompletedPassNote})
		}
	}
	if len(verdictUpdates) > 0 {
		if updateErr := sm.Update(func(st *state.State) error {
			for _, u := range verdictUpdates {
				t := st.Tasks[u.key]
				t.ReviewStatus = u.status
				st.Tasks[u.key] = t
			}
			return nil
		}); updateErr != nil {
			return reportResultOutcome{}, updateErr
		}
	}

	// Check whether any task still needs a review.
	s, err = sm.GetState()
	if err != nil {
		return reportResultOutcome{}, err
	}
	for _, t := range s.Tasks {
		if t.ImplStatus != state.TaskStatusCompleted {
			continue
		}
		if t.ReviewStatus != state.TaskStatusCompletedPass &&
			t.ReviewStatus != state.TaskStatusCompletedPassNote {
			// Task needs review — hold in phase-6.
			return reportResultOutcome{
				StateUpdated:    true,
				ArtifactWritten: artifactWritten,
				VerdictParsed:   verdictParsed,
				Findings:        allFindings,
				NextActionHint:  "setup_continue",
			}, nil
		}
	}

	// Phase 6 completion gate: verify review file count matches task count.
	// When missing files are found, reset ReviewStatus so the engine
	// re-dispatches reviewers on the next pipeline_next_action call.
	if missing := missingReviewFiles(in.workspace, s.Tasks); len(missing) > 0 {
		if updateErr := sm.Update(func(st *state.State) error {
			for _, k := range missing {
				if t, ok := st.Tasks[k]; ok {
					t.ReviewStatus = ""
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
			VerdictParsed:   verdictParsed,
			Findings:        allFindings,
			NextActionHint:  "setup_continue",
			Warning: "phase-6 completion blocked: missing review files for tasks: " +
				strings.Join(missing, ", ") + " — ReviewStatus reset to pending",
		}, nil
	}

	if err := sm.PhaseComplete(in.workspace, in.phase); err != nil {
		return reportResultOutcome{}, err
	}
	return reportResultOutcome{
		StateUpdated:    true,
		ArtifactWritten: artifactWritten,
		VerdictParsed:   verdictParsed,
		Findings:        allFindings,
		NextActionHint:  "proceed",
	}, nil
}

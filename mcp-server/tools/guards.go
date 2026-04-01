// Package tools implements guard functions that re-implement the blocking and
// warning guards from scripts/pre-tool-hook.sh (Rules 3a–3j) inside the Go MCP
// tool handlers.  Blocking guards return a non-nil error; warning-only guards
// return a non-empty string message and never return an error.
package tools

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/hiromaily/claude-forge/mcp-server/state"
)

// phaseArtifacts maps phase identifiers to the artifact file that must exist in
// the workspace before phase-complete is accepted.  Phases absent from this map
// have no required artifact (e.g. checkpoint-a, phase-5, pr-creation).
var phaseArtifacts = map[string]string{
	"phase-1":       "analysis.md",
	"phase-2":       "investigation.md",
	"phase-3":       "design.md",
	"phase-3b":      "review-design.md",
	"phase-4":       "tasks.md",
	"phase-4b":      "review-tasks.md",
	"phase-7":       "comprehensive-review.md",
	"final-summary": "summary.md",
}

// phaseLogRequired lists phases that are expected to emit a phase-log entry
// before phase-complete.  Checkpoint and admin phases are excluded.
var phaseLogRequired = map[string]bool{
	"phase-1":            true,
	"phase-2":            true,
	"phase-3":            true,
	"phase-3b":           true,
	"phase-4":            true,
	"phase-4b":           true,
	"phase-7":            true,
	"final-verification": true,
}

// ---------- Blocking guards (return error) ----------

// Guard3aArtifactExists enforces Rule 3a: the artifact file for the given phase
// must exist in workspace before phase-complete is accepted.
// Returns nil when no artifact is required for the phase, or when the artifact exists.
// Returns a non-nil error when the required artifact is absent.
func Guard3aArtifactExists(workspace, phase string, _ *state.State) error {
	artifact, required := phaseArtifacts[phase]
	if !required {
		return nil
	}
	path := filepath.Join(workspace, artifact)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("BLOCKED: %s must exist before completing %s. Write the artifact file first", artifact, phase)
	}
	return nil
}

// Guard3bReviewFileExists enforces Rule 3b: review-{taskNum}.md must exist in
// workspace before a task-update sets reviewStatus to completed_pass.
// Returns nil for any reviewStatus other than "completed_pass".
func Guard3bReviewFileExists(workspace, taskNum, reviewStatus string, _ *state.State) error {
	if reviewStatus != "completed_pass" {
		return nil
	}
	reviewFile := fmt.Sprintf("review-%s.md", taskNum)
	path := filepath.Join(workspace, reviewFile)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("BLOCKED: %s must exist before marking task %s review as passed. Write the review file first", reviewFile, taskNum)
	}
	return nil
}

// Guard3cTasksNonEmpty enforces Rule 3c: state.Tasks must be non-empty before
// phase-start for phase-5 is accepted.
// Returns nil for any phase other than "phase-5".
func Guard3cTasksNonEmpty(phase string, s *state.State) error {
	if phase != "phase-5" {
		return nil
	}
	if len(s.Tasks) == 0 {
		return errors.New("BLOCKED: no tasks initialized in state.json. Run task-init before starting phase-5")
	}
	return nil
}

// Guard3eCheckpointAwaitingHuman enforces Rule 3e: phase-complete on checkpoint-a,
// checkpoint-b, or any phase currently in awaiting_human status requires
// currentPhaseStatus == "awaiting_human".
// Returns nil for phases that are not in awaiting_human status.
func Guard3eCheckpointAwaitingHuman(phase string, s *state.State) error {
	isCheckpoint := phase == "checkpoint-a" || phase == "checkpoint-b"
	isAwaitingPhase := s.CurrentPhase == phase && s.CurrentPhaseStatus == "awaiting_human"
	if !isCheckpoint && !isAwaitingPhase {
		return nil
	}
	if s.CurrentPhaseStatus != "awaiting_human" {
		return fmt.Errorf(
			"BLOCKED: phase-complete %s requires currentPhaseStatus == \"awaiting_human\". "+
				"Call checkpoint {workspace} %s first to register the human review pause before completing this checkpoint",
			phase, phase,
		)
	}
	return nil
}

// Guard3gCheckpointBDoneOrSkipped enforces Rule 3g: task-init requires checkpoint-b
// to be present in completedPhases or skippedPhases.
func Guard3gCheckpointBDoneOrSkipped(s *state.State) error {
	if slices.Contains(s.CompletedPhases, "checkpoint-b") {
		return nil
	}
	if slices.Contains(s.SkippedPhases, "checkpoint-b") {
		return nil
	}
	return errors.New("BLOCKED: task-init requires checkpoint-b to be completed or skipped first. " +
		"Complete Checkpoint B (human approval or auto-approve) before initializing tasks",
	)
}

// Guard3jCheckpointRevisionPending enforces Rule 3j: phase-complete on
// checkpoint-a or checkpoint-b is blocked when checkpointRevisionPending for
// that checkpoint is true.
// Returns nil for non-checkpoint phases and dynamic checkpoints (e.g., post-to-source).
func Guard3jCheckpointRevisionPending(phase string, s *state.State) error {
	if phase != "checkpoint-a" && phase != "checkpoint-b" {
		return nil
	}
	if s.CheckpointRevisionPending != nil && s.CheckpointRevisionPending[phase] {
		return fmt.Errorf(
			"BLOCKED: phase-complete %s requires 'clear-revision-pending' to be called first. "+
				"The user requested a revision — call clear-revision-pending {workspace} %s "+
				"after receiving explicit user approval, then call phase-complete",
			phase, phase,
		)
	}
	return nil
}

// GuardInitValidated enforces the init guard (AC-3): the MCP init tool must be
// called with validated=true (i.e. after mcp__forge-state__validate_input succeeded).
// Returns a non-nil error when validated is false or absent (false default).
func GuardInitValidated(validated bool) error {
	if !validated {
		return errors.New("BLOCKED: init requires validated=true. " +
			"Call mcp__forge-state__validate_input first and pass validated=true only when it succeeds",
		)
	}
	return nil
}

// ---------- Warning-only guards (return string, not error) ----------

// Warn3dPhaseLogDuplicate implements Rule 3d: warns when a phase-log entry for
// the given phase already exists in state.PhaseLog.
// Returns a non-empty warning string when a duplicate exists; empty string otherwise.
func Warn3dPhaseLogDuplicate(phase string, s *state.State) string {
	for _, entry := range s.PhaseLog {
		if entry.Phase == phase {
			return fmt.Sprintf(
				"WARNING: phase-log entry for '%s' already exists. "+
					"This may be a legitimate retry or an accidental duplicate",
				phase,
			)
		}
	}
	return ""
}

// Warn3fPhaseLogMissing implements Rule 3f: warns when phase-complete is called
// for a phase that should have emitted a phase-log entry but has none.
// Returns a non-empty warning string when the entry is missing; empty string otherwise.
// Checkpoint and admin phases are exempt.
func Warn3fPhaseLogMissing(phase string, s *state.State) string {
	if !phaseLogRequired[phase] {
		return ""
	}
	for _, entry := range s.PhaseLog {
		if entry.Phase == phase {
			return ""
		}
	}
	return fmt.Sprintf(
		"WARNING: no phase-log entry for '%s'. "+
			"Call phase-log <workspace> <phase> <tokens> <duration_ms> <model> before completing this phase",
		phase,
	)
}

// Warn3hTaskNotFound implements Rule 3h: warns when a task-update references a
// task number that does not exist in state.Tasks.
// Returns a non-empty warning string when the task is absent; empty string otherwise.
func Warn3hTaskNotFound(taskNum string, s *state.State) string {
	if _, ok := s.Tasks[taskNum]; !ok {
		return fmt.Sprintf(
			"WARNING: task %q not found in state.Tasks. "+
				"Verify the task number or run task-init first",
			taskNum,
		)
	}
	return ""
}

// Warn3iPhaseNotInProgress implements Rule 3i: warns when phase-complete is called
// while currentPhaseStatus is not "in_progress".
// Returns a non-empty warning string when status is unexpected; empty string otherwise.
func Warn3iPhaseNotInProgress(s *state.State) string {
	if s.CurrentPhaseStatus != "in_progress" {
		return fmt.Sprintf(
			"WARNING: phase-complete called but currentPhaseStatus is %q (expected \"in_progress\"). "+
				"Verify the pipeline state before completing this phase",
			s.CurrentPhaseStatus,
		)
	}
	return ""
}

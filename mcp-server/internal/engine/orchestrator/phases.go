// provides pure-logic building blocks for the pipeline engine.
// It may import the state/ package for state types used by the engine.

package orchestrator

import "github.com/hiromaily/claude-forge/mcp-server/internal/engine/state"

// Phase ID aliases — re-exported from the state package so that callers
// within orchestrator (engine.go, etc.) can reference them without a package prefix.
const (
	PhaseSetup             = state.PhaseSetup
	PhaseOne               = state.PhaseOne
	PhaseTwo               = state.PhaseTwo
	PhaseThree             = state.PhaseThree
	PhaseThreeB            = state.PhaseThreeB
	PhaseCheckpointA       = state.PhaseCheckpointA
	PhaseFour              = state.PhaseFour
	PhaseFourB             = state.PhaseFourB
	PhaseCheckpointB       = state.PhaseCheckpointB
	PhaseFive              = state.PhaseFive
	PhaseSix               = state.PhaseSix
	PhaseSeven             = state.PhaseSeven
	PhaseFinalVerification = state.PhaseFinalVerification
	PhasePRCreation        = state.PhasePRCreation
	PhaseFinalSummary      = state.PhaseFinalSummary
	PhaseFinalCommit       = state.PhaseFinalCommit
	PhasePostToSource      = state.PhasePostToSource
	PhaseCompleted         = state.PhaseCompleted
)

// AllPhases is the canonical ordering, derived from phaseRegistry by initRegistry().
// It is assigned at package init time and must not be mutated after that.
var AllPhases []string

// nonSkippable is the set of phases that can never be passed to skip-phase.
// Assigned by initRegistry() from phaseRegistry entries where Skippable == false.
var nonSkippable map[string]bool

// allPhasesSet is a fast-lookup set of all phase IDs.
// Assigned by initRegistry() from phaseRegistry entries.
var allPhasesSet map[string]bool

// IsSkippable returns true if phase is a valid skip target.
// A phase is skippable if it is a known phase and not in nonSkippable.
func IsSkippable(phase string) bool {
	if nonSkippable[phase] {
		return false
	}

	return allPhasesSet[phase]
}

// NextPhase returns the first phase after current that is not in skipped.
// If current is not found, or no non-skipped successor exists, returns "completed".
func NextPhase(current string, skipped []string) string {
	// Build a fast lookup set for skipped phases.
	skippedSet := make(map[string]bool, len(skipped))
	for _, s := range skipped {
		skippedSet[s] = true
	}

	// Find the index of current in AllPhases.
	currentIdx := -1

	for i, p := range AllPhases {
		if p == current {
			currentIdx = i

			break
		}
	}

	// If current is not found, return "completed".
	if currentIdx < 0 {
		return PhaseCompleted
	}

	// Walk forward from the phase after current.
	for i := currentIdx + 1; i < len(AllPhases); i++ {
		p := AllPhases[i]
		if !skippedSet[p] {
			return p
		}
	}

	return PhaseCompleted
}

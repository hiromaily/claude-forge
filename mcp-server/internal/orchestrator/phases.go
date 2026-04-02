// Package orchestrator provides pure-logic building blocks for the pipeline engine.
// It may import the state/ package for state types used by the engine.
package orchestrator

// Phase ID constants — must match state.ValidPhases string values exactly.
const (
	PhaseSetup             = "setup"
	PhaseOne               = "phase-1"
	PhaseTwo               = "phase-2"
	PhaseThree             = "phase-3"
	PhaseThreeB            = "phase-3b"
	PhaseCheckpointA       = "checkpoint-a"
	PhaseFour              = "phase-4"
	PhaseFourB             = "phase-4b"
	PhaseCheckpointB       = "checkpoint-b"
	PhaseFive              = "phase-5"
	PhaseSix               = "phase-6"
	PhaseSeven             = "phase-7"
	PhaseFinalVerification = "final-verification"
	PhasePRCreation        = "pr-creation"
	PhaseFinalSummary      = "final-summary"
	PhasePostToSource      = "post-to-source"
	PhaseCompleted         = "completed"
)

// AllPhases is the canonical ordering. It must remain in sync with
// state.ValidPhases. Both are string-equal; neither imports the other.
var AllPhases = []string{
	PhaseSetup, PhaseOne, PhaseTwo, PhaseThree, PhaseThreeB,
	PhaseCheckpointA, PhaseFour, PhaseFourB, PhaseCheckpointB,
	PhaseFive, PhaseSix, PhaseSeven, PhaseFinalVerification,
	PhaseFinalSummary, PhasePRCreation, PhasePostToSource, PhaseCompleted,
}

// nonSkippable is the set of phases that can never be passed to skip-phase.
var nonSkippable = map[string]bool{
	PhaseSetup:     true,
	PhaseCompleted: true,
}

// allPhasesSet is a fast-lookup set derived from AllPhases.
var allPhasesSet = func() map[string]bool {
	m := make(map[string]bool, len(AllPhases))
	for _, p := range AllPhases {
		m[p] = true
	}

	return m
}()

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

package orchestrator

import "github.com/hiromaily/claude-forge/mcp-server/internal/state"

// PhaseDescriptor holds the static metadata for one pipeline phase.
// The ordered slice of descriptors (phaseRegistry) is the single source of truth
// for phase sequence, skippability, display labels, and per-template skip behaviour.
type PhaseDescriptor struct {
	// ID is the canonical phase identifier string (e.g. "phase-1").
	// It must match the corresponding state.Phase* constant.
	ID string

	// Skippable is false only for "setup" and "completed".
	// All other phases may be passed to the skip-phase command.
	Skippable bool

	// Label is the human-readable display label used in effort_options output.
	// An empty string means the phase ID itself is used as the label.
	Label string

	// TemplateSkips maps flow template names to a boolean indicating whether
	// that template skips this phase. A true value for key "light" means the
	// "light" flow template skips this phase at workspace setup.
	// nil and an empty map are both treated as "no template skips this phase".
	TemplateSkips map[string]bool
}

// phaseRegistry is the ordered, read-only slice of phase descriptors that defines
// the canonical pipeline sequence. Order here determines pipeline execution order.
//
// Rules:
//   - Exactly 18 entries, one per phase in state.ValidPhases.
//   - IDs must match state.ValidPhases in the same order.
//   - Skippable must be false only for PhaseSetup and PhaseCompleted.
//   - TemplateSkips["full"] must be absent or false for every entry
//     (the full template skips nothing).
var phaseRegistry = []PhaseDescriptor{
	{ID: PhaseSetup, Skippable: false},
	{ID: PhaseOne, Skippable: true},
	{
		ID:            PhaseTwo,
		Skippable:     true,
		Label:         "Investigation",
		TemplateSkips: map[string]bool{state.TemplateLight: true},
	},
	{ID: PhaseThree, Skippable: true},
	{ID: PhaseThreeB, Skippable: true},
	{ID: PhaseCheckpointA, Skippable: true, Label: "Design Checkpoint"},
	{
		ID:            PhaseFour,
		Skippable:     true,
		Label:         "Task Decomposition",
		TemplateSkips: map[string]bool{state.TemplateLight: true},
	},
	{
		ID:        PhaseFourB,
		Skippable: true,
		Label:     "Tasks AI Review",
		TemplateSkips: map[string]bool{
			state.TemplateLight:    true,
			state.TemplateStandard: true,
		},
	},
	{
		ID:        PhaseCheckpointB,
		Skippable: true,
		Label:     "Tasks Checkpoint",
		TemplateSkips: map[string]bool{
			state.TemplateLight:    true,
			state.TemplateStandard: true,
		},
	},
	{ID: PhaseFive, Skippable: true},
	{ID: PhaseSix, Skippable: true},
	{
		ID:            PhaseSeven,
		Skippable:     true,
		Label:         "Comprehensive Review",
		TemplateSkips: map[string]bool{state.TemplateLight: true},
	},
	{ID: PhaseFinalVerification, Skippable: true},
	{ID: PhasePRCreation, Skippable: true},
	{ID: PhaseFinalSummary, Skippable: true},
	{ID: PhaseFinalCommit, Skippable: true},
	{ID: PhasePostToSource, Skippable: true},
	{ID: PhaseCompleted, Skippable: false},
}

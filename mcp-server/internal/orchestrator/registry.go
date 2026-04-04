package orchestrator

import (
	"fmt"

	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
)

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
	{ID: PhasePostToSource, Skippable: true},
	{ID: PhaseFinalCommit, Skippable: true},
	{ID: PhaseCompleted, Skippable: false},
}

// init calls initRegistry once at package load time to populate all derived vars.
func init() {
	initRegistry()
}

// initRegistry populates AllPhases, nonSkippable, allPhasesSet, skipTable, and phaseLabels
// from the phaseRegistry declaration. It panics if phaseRegistry is inconsistent with
// state.ValidPhases (different lengths or IDs not found in state.ValidPhases).
//
// All derived vars are zero-valued at declaration and assigned here, so they are
// always in sync with the single source of truth: phaseRegistry.
func initRegistry() { //nolint:gocyclo // complexity is inherent in the multi-pass validation and derivation logic
	// Consistency check: lengths must match.
	if len(phaseRegistry) != len(state.ValidPhases) {
		panic(fmt.Sprintf(
			"orchestrator: phaseRegistry has %d entries but state.ValidPhases has %d entries; "+
				"add or remove the corresponding entry in the other slice",
			len(phaseRegistry), len(state.ValidPhases),
		))
	}

	// Build a fast lookup set from state.ValidPhases for the ID check.
	validSet := make(map[string]bool, len(state.ValidPhases))
	for _, id := range state.ValidPhases {
		validSet[id] = true
	}

	// Verify every phaseRegistry ID is in state.ValidPhases (and in the same order).
	for i, d := range phaseRegistry {
		if !validSet[d.ID] {
			panic(fmt.Sprintf(
				"orchestrator: phaseRegistry[%d].ID = %q is not present in state.ValidPhases",
				i, d.ID,
			))
		}
		if d.ID != state.ValidPhases[i] {
			panic(fmt.Sprintf(
				"orchestrator: phaseRegistry[%d].ID = %q but state.ValidPhases[%d] = %q; "+
					"phaseRegistry and state.ValidPhases must be in the same order",
				i, d.ID, i, state.ValidPhases[i],
			))
		}
	}

	// Derive AllPhases: ordered slice of IDs.
	phases := make([]string, len(phaseRegistry))
	for i, d := range phaseRegistry {
		phases[i] = d.ID
	}
	AllPhases = phases

	// Derive nonSkippable: set of IDs where Skippable == false.
	ns := make(map[string]bool)
	for _, d := range phaseRegistry {
		if !d.Skippable {
			ns[d.ID] = true
		}
	}
	nonSkippable = ns

	// Derive allPhasesSet: set of all IDs.
	aps := make(map[string]bool, len(phaseRegistry))
	for _, d := range phaseRegistry {
		aps[d.ID] = true
	}
	allPhasesSet = aps

	// Derive skipTable: for each known template, collect IDs where TemplateSkips[template] is true.
	// Always initialize the slice for every known template (even if empty) so that
	// SkipsForTemplate("full") returns []string{} (non-nil).
	st := make(map[string][]string, len(state.ValidTemplates))
	for _, tmpl := range state.ValidTemplates {
		st[tmpl] = []string{}
	}
	for _, d := range phaseRegistry {
		for tmpl, skipped := range d.TemplateSkips {
			if _, valid := st[tmpl]; !valid {
				panic(fmt.Sprintf("orchestrator: phase %q has unknown template %q in TemplateSkips", d.ID, tmpl))
			}
			if skipped {
				st[tmpl] = append(st[tmpl], d.ID)
			}
		}
	}
	skipTable = st

	// Derive phaseLabels: map of ID → Label for non-empty labels.
	pl := make(map[string]string)
	for _, d := range phaseRegistry {
		if d.Label != "" {
			pl[d.ID] = d.Label
		}
	}
	phaseLabels = pl
}

// Package orchestrator provides pure-logic building blocks for the pipeline engine.
package orchestrator

import (
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/state"
)

// TestPhaseDescriptorFields verifies that PhaseDescriptor has the expected fields
// and that the type compiles correctly with the documented field set.
func TestPhaseDescriptorFields(t *testing.T) {
	t.Parallel()

	// Construct a PhaseDescriptor to confirm the struct compiles with all fields.
	d := PhaseDescriptor{
		ID:            "phase-1",
		Skippable:     true,
		Label:         "Test Label",
		TemplateSkips: map[string]bool{"light": true},
	}

	if d.ID != "phase-1" {
		t.Errorf("PhaseDescriptor.ID = %q, want %q", d.ID, "phase-1")
	}

	if !d.Skippable {
		t.Errorf("PhaseDescriptor.Skippable = false, want true")
	}

	if d.Label != "Test Label" {
		t.Errorf("PhaseDescriptor.Label = %q, want %q", d.Label, "Test Label")
	}

	if !d.TemplateSkips["light"] {
		t.Errorf("PhaseDescriptor.TemplateSkips[\"light\"] = false, want true")
	}
}

// TestPhaseRegistryLength verifies that phaseRegistry contains exactly 18 entries.
func TestPhaseRegistryLength(t *testing.T) {
	t.Parallel()

	const wantCount = 18
	if got := len(phaseRegistry); got != wantCount {
		t.Errorf("len(phaseRegistry) = %d, want %d", got, wantCount)
	}
}

// TestPhaseRegistryOrder verifies all phase IDs appear in canonical pipeline order
// as defined by state.ValidPhases (the single source of truth).
func TestPhaseRegistryOrder(t *testing.T) {
	t.Parallel()

	want := state.ValidPhases

	if len(phaseRegistry) != len(want) {
		t.Fatalf("len(phaseRegistry) = %d, want %d", len(phaseRegistry), len(want))
	}

	for i, d := range phaseRegistry {
		if d.ID != want[i] {
			t.Errorf("phaseRegistry[%d].ID = %q, want %q", i, d.ID, want[i])
		}
	}
}

// TestPhaseRegistrySkippable verifies that only "setup" and "completed" have Skippable: false.
func TestPhaseRegistrySkippable(t *testing.T) {
	t.Parallel()

	for _, d := range phaseRegistry {
		t.Run(d.ID, func(t *testing.T) {
			t.Parallel()

			wantSkippable := d.ID != state.PhaseSetup && d.ID != state.PhaseCompleted
			if d.Skippable != wantSkippable {
				t.Errorf("phaseRegistry entry %q: Skippable = %v, want %v",
					d.ID, d.Skippable, wantSkippable)
			}
		})
	}
}

// TestPhaseRegistryTemplateSkipsData verifies that TemplateSkips entries match
// the expected skip data from the current skipTable in flow_templates.go.
// This ensures the registry data is consistent with the existing source-of-truth.
func TestPhaseRegistryTemplateSkipsData(t *testing.T) {
	t.Parallel()

	// Expected: phases skipped by the "light" template.
	wantLightSkips := map[string]bool{
		state.PhaseTwo:         true,
		state.PhaseFour:        true,
		state.PhaseFourB:       true,
		state.PhaseCheckpointB: true,
		state.PhaseSix:         true,
	}

	// Expected: phases skipped by the "standard" template.
	wantStandardSkips := map[string]bool{
		state.PhaseFourB:       true,
		state.PhaseCheckpointB: true,
	}

	for _, d := range phaseRegistry {
		t.Run(d.ID, func(t *testing.T) {
			t.Parallel()

			gotLightSkip := d.TemplateSkips["light"]
			wantLightSkip := wantLightSkips[d.ID]
			if gotLightSkip != wantLightSkip {
				t.Errorf("phaseRegistry[%q].TemplateSkips[\"light\"] = %v, want %v",
					d.ID, gotLightSkip, wantLightSkip)
			}

			gotStandardSkip := d.TemplateSkips["standard"]
			wantStandardSkip := wantStandardSkips[d.ID]
			if gotStandardSkip != wantStandardSkip {
				t.Errorf("phaseRegistry[%q].TemplateSkips[\"standard\"] = %v, want %v",
					d.ID, gotStandardSkip, wantStandardSkip)
			}

			// "full" template skips nothing — every phase should have TemplateSkips["full"] == false.
			if d.TemplateSkips["full"] {
				t.Errorf("phaseRegistry[%q].TemplateSkips[\"full\"] = true, want false (full template skips nothing)",
					d.ID)
			}
		})
	}
}

// TestPhaseRegistryLabels verifies that Label values match the expected phaseLabels data.
func TestPhaseRegistryLabels(t *testing.T) {
	t.Parallel()

	wantLabels := map[string]string{
		state.PhaseOne:               "Situation Analysis",
		state.PhaseTwo:               "Investigation",
		state.PhaseThree:             "Design",
		state.PhaseThreeB:            "Design Review",
		state.PhaseCheckpointA:       "Design Checkpoint",
		state.PhaseFour:              "Task Decomposition",
		state.PhaseFourB:             "Tasks AI Review",
		state.PhaseCheckpointB:       "Tasks Checkpoint",
		state.PhaseFive:              "Implementation",
		state.PhaseSix:               "Code Review",
		state.PhaseSeven:             "Comprehensive Review",
		state.PhaseFinalVerification: "Final Verification",
		state.PhasePRCreation:        "PR Creation",
		state.PhaseFinalSummary:      "Final Summary",
		state.PhasePostToSource:      "Post to Source",
		state.PhaseFinalCommit:       "Final Commit",
	}

	for _, d := range phaseRegistry {
		t.Run(d.ID, func(t *testing.T) {
			t.Parallel()

			wantLabel := wantLabels[d.ID]
			if d.Label != wantLabel {
				t.Errorf("phaseRegistry[%q].Label = %q, want %q", d.ID, d.Label, wantLabel)
			}
		})
	}
}

// TestInitRegistryPopulatesAllPhases verifies that AllPhases is populated by initRegistry()
// and contains exactly the IDs from phaseRegistry in order.
func TestInitRegistryPopulatesAllPhases(t *testing.T) {
	t.Parallel()

	if len(AllPhases) != len(phaseRegistry) {
		t.Fatalf("AllPhases length = %d, want %d (len(phaseRegistry))", len(AllPhases), len(phaseRegistry))
	}

	for i, d := range phaseRegistry {
		if AllPhases[i] != d.ID {
			t.Errorf("AllPhases[%d] = %q, want %q (phaseRegistry[%d].ID)", i, AllPhases[i], d.ID, i)
		}
	}
}

// TestInitRegistryPopulatesNonSkippable verifies that nonSkippable is populated by initRegistry()
// and contains exactly the IDs where Skippable is false in phaseRegistry.
func TestInitRegistryPopulatesNonSkippable(t *testing.T) {
	t.Parallel()

	// Collect expected non-skippable phases from registry.
	wantNonSkippable := map[string]bool{}
	for _, d := range phaseRegistry {
		if !d.Skippable {
			wantNonSkippable[d.ID] = true
		}
	}

	// nonSkippable must contain exactly: setup and completed.
	for id := range wantNonSkippable {
		if !nonSkippable[id] {
			t.Errorf("nonSkippable[%q] = false, want true (Skippable=false in phaseRegistry)", id)
		}
	}

	// Verify no extra entries in nonSkippable.
	for id := range nonSkippable {
		if !wantNonSkippable[id] {
			t.Errorf("nonSkippable contains unexpected entry %q", id)
		}
	}
}

// TestInitRegistryPopulatesAllPhasesSet verifies that allPhasesSet is populated by initRegistry()
// and contains every ID from phaseRegistry.
func TestInitRegistryPopulatesAllPhasesSet(t *testing.T) {
	t.Parallel()

	for _, d := range phaseRegistry {
		if !allPhasesSet[d.ID] {
			t.Errorf("allPhasesSet[%q] = false, want true", d.ID)
		}
	}

	if len(allPhasesSet) != len(phaseRegistry) {
		t.Errorf("allPhasesSet length = %d, want %d", len(allPhasesSet), len(phaseRegistry))
	}
}

// TestInitRegistryPopulatesSkipTable verifies that skipTable is populated by initRegistry()
// and matches what TemplateSkips entries in phaseRegistry imply.
func TestInitRegistryPopulatesSkipTable(t *testing.T) {
	t.Parallel()

	templates := []string{state.TemplateLight, state.TemplateStandard, state.TemplateFull}

	for _, tmpl := range templates {
		t.Run(tmpl, func(t *testing.T) {
			t.Parallel()

			// Build expected skip list for this template from phaseRegistry.
			var wantSkips []string
			for _, d := range phaseRegistry {
				if d.TemplateSkips[tmpl] {
					wantSkips = append(wantSkips, d.ID)
				}
			}
			if wantSkips == nil {
				wantSkips = []string{}
			}

			got := skipTable[tmpl]
			if got == nil {
				t.Fatalf("skipTable[%q] = nil, want %v", tmpl, wantSkips)
			}

			if len(got) != len(wantSkips) {
				t.Fatalf("skipTable[%q] length = %d, want %d; got %v, want %v",
					tmpl, len(got), len(wantSkips), got, wantSkips)
			}

			for i := range got {
				if got[i] != wantSkips[i] {
					t.Errorf("skipTable[%q][%d] = %q, want %q", tmpl, i, got[i], wantSkips[i])
				}
			}
		})
	}
}

// TestInitRegistryPopulatesPhaseLabels verifies that phaseLabels is populated by initRegistry()
// and contains exactly the non-empty Label entries from phaseRegistry.
func TestInitRegistryPopulatesPhaseLabels(t *testing.T) {
	t.Parallel()

	// Build expected labels from phaseRegistry.
	wantLabels := map[string]string{}
	for _, d := range phaseRegistry {
		if d.Label != "" {
			wantLabels[d.ID] = d.Label
		}
	}

	for id, wantLabel := range wantLabels {
		gotLabel, ok := phaseLabels[id]
		if !ok {
			t.Errorf("phaseLabels[%q] is missing, want %q", id, wantLabel)
			continue
		}
		if gotLabel != wantLabel {
			t.Errorf("phaseLabels[%q] = %q, want %q", id, gotLabel, wantLabel)
		}
	}

	if len(phaseLabels) != len(wantLabels) {
		t.Errorf("phaseLabels length = %d, want %d", len(phaseLabels), len(wantLabels))
	}
}

// TestInitRegistryPanicOnLengthMismatch verifies that initRegistry panics
// when phaseRegistry and state.ValidPhases have different lengths.
// Must not call t.Parallel() — it temporarily mutates the package-level phaseRegistry.
func TestInitRegistryPanicOnLengthMismatch(t *testing.T) {
	orig := phaseRegistry
	defer func() { phaseRegistry = orig }()

	// Append a dummy entry to make lengths diverge.
	phaseRegistry = append(phaseRegistry, PhaseDescriptor{ID: "fake-phase", Skippable: true})

	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		initRegistry()
	}()

	if !panicked {
		t.Error("initRegistry() did not panic on phaseRegistry/state.ValidPhases length mismatch")
	}
}

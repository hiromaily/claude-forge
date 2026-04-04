// Package orchestrator provides pure-logic building blocks for the pipeline engine.
package orchestrator

import (
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
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

// TestPhaseRegistryOrder verifies all 18 phase IDs appear in canonical pipeline order.
func TestPhaseRegistryOrder(t *testing.T) {
	t.Parallel()

	want := []string{
		state.PhaseSetup,
		state.PhaseOne,
		state.PhaseTwo,
		state.PhaseThree,
		state.PhaseThreeB,
		state.PhaseCheckpointA,
		state.PhaseFour,
		state.PhaseFourB,
		state.PhaseCheckpointB,
		state.PhaseFive,
		state.PhaseSix,
		state.PhaseSeven,
		state.PhaseFinalVerification,
		state.PhasePRCreation,
		state.PhaseFinalSummary,
		state.PhaseFinalCommit,
		state.PhasePostToSource,
		state.PhaseCompleted,
	}

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
		state.PhaseSeven:       true,
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
		state.PhaseTwo:         "Investigation",
		state.PhaseFour:        "Task Decomposition",
		state.PhaseFourB:       "Tasks AI Review",
		state.PhaseCheckpointA: "Design Checkpoint",
		state.PhaseCheckpointB: "Tasks Checkpoint",
		state.PhaseSeven:       "Comprehensive Review",
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

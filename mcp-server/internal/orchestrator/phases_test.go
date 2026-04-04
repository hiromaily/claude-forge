// Package orchestrator provides pure-logic building blocks for the pipeline engine.
package orchestrator

import (
	"reflect"
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
)

func TestAllPhasesCount(t *testing.T) {
	t.Parallel()

	wantCount := len(phaseRegistry)

	if got := len(AllPhases); got != wantCount {
		t.Errorf("AllPhases length = %d, want %d (len(phaseRegistry))", got, wantCount)
	}
}

func TestAllPhasesOrder(t *testing.T) {
	t.Parallel()

	want := []string{
		"setup", "phase-1", "phase-2", "phase-3", "phase-3b",
		"checkpoint-a", "phase-4", "phase-4b", "checkpoint-b",
		"phase-5", "phase-6", "phase-7", "final-verification",
		"pr-creation", "final-summary", "final-commit", "post-to-source", "completed",
	}

	if len(AllPhases) != len(want) {
		t.Fatalf("AllPhases length = %d, want %d", len(AllPhases), len(want))
	}

	for i, phase := range AllPhases {
		if phase != want[i] {
			t.Errorf("AllPhases[%d] = %q, want %q", i, phase, want[i])
		}
	}
}

func TestIsSkippable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		phase string
		want  bool
	}{
		{phase: "setup", want: false},
		{phase: "completed", want: false},
		{phase: "phase-1", want: true},
		{phase: "phase-2", want: true},
		{phase: "phase-3", want: true},
		{phase: "phase-3b", want: true},
		{phase: "checkpoint-a", want: true},
		{phase: "phase-4", want: true},
		{phase: "phase-4b", want: true},
		{phase: "checkpoint-b", want: true},
		{phase: "phase-5", want: true},
		{phase: "phase-6", want: true},
		{phase: "phase-7", want: true},
		{phase: "final-verification", want: true},
		{phase: "pr-creation", want: true},
		{phase: "final-summary", want: true},
		{phase: "final-commit", want: true},
		{phase: "post-to-source", want: true},
		{phase: "unknown-phase", want: false},
		{phase: "", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.phase, func(t *testing.T) {
			t.Parallel()

			got := IsSkippable(tc.phase)
			if got != tc.want {
				t.Errorf("IsSkippable(%q) = %v, want %v", tc.phase, got, tc.want)
			}
		})
	}
}

func TestNextPhase(t *testing.T) {
	t.Parallel()

	// Build the list of all phases after phase-1 (for the all-skipped test).
	allAfterPhaseOne := AllPhases[2:] // phases from phase-2 onwards

	tests := []struct {
		name    string
		current string
		skipped []string
		want    string
	}{
		{
			name:    "linear progression from phase-1",
			current: PhaseOne,
			skipped: nil,
			want:    "phase-2",
		},
		{
			name:    "skip phase-2 returns phase-3",
			current: PhaseOne,
			skipped: []string{"phase-2"},
			want:    "phase-3",
		},
		{
			name:    "all remaining phases skipped returns completed",
			current: PhaseOne,
			skipped: allAfterPhaseOne,
			want:    "completed",
		},
		{
			name:    "unknown current returns completed",
			current: "unknown",
			skipped: nil,
			want:    "completed",
		},
		{
			name:    "linear progression from setup",
			current: PhaseSetup,
			skipped: nil,
			want:    "phase-1",
		},
		{
			name:    "last phase before completed",
			current: PhasePostToSource,
			skipped: nil,
			want:    "completed",
		},
		{
			name:    "completed has no successor",
			current: PhaseCompleted,
			skipped: nil,
			want:    "completed",
		},
		{
			name:    "skip multiple phases in middle",
			current: PhaseOne,
			skipped: []string{"phase-2", "phase-3", "phase-3b"},
			want:    "checkpoint-a",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := NextPhase(tc.current, tc.skipped)
			if got != tc.want {
				t.Errorf("NextPhase(%q, %v) = %q, want %q", tc.current, tc.skipped, got, tc.want)
			}
		})
	}
}

// TestPhaseRegistryConsistency verifies that phaseRegistry and state.ValidPhases
// contain the same IDs in the same order. This is the primary regression guard
// for the two-edit-site rule (add a phase to both state.ValidPhases and phaseRegistry).
func TestPhaseRegistryConsistency(t *testing.T) {
	t.Parallel()

	t.Run("every_registry_id_in_valid_phases", func(t *testing.T) {
		t.Parallel()

		validSet := make(map[string]bool, len(state.ValidPhases))
		for _, id := range state.ValidPhases {
			validSet[id] = true
		}

		for i, d := range phaseRegistry {
			if !validSet[d.ID] {
				t.Errorf("phaseRegistry[%d].ID = %q not found in state.ValidPhases", i, d.ID)
			}
		}
	})

	t.Run("every_valid_phase_in_registry", func(t *testing.T) {
		t.Parallel()

		registrySet := make(map[string]bool, len(phaseRegistry))
		for _, d := range phaseRegistry {
			registrySet[d.ID] = true
		}

		for i, id := range state.ValidPhases {
			if !registrySet[id] {
				t.Errorf("state.ValidPhases[%d] = %q not found in phaseRegistry", i, id)
			}
		}
	})

	t.Run("same_order", func(t *testing.T) {
		t.Parallel()

		if len(phaseRegistry) != len(state.ValidPhases) {
			t.Fatalf("length mismatch: phaseRegistry has %d entries, state.ValidPhases has %d",
				len(phaseRegistry), len(state.ValidPhases))
		}

		for i, d := range phaseRegistry {
			if d.ID != state.ValidPhases[i] {
				t.Errorf("position %d: phaseRegistry[%d].ID = %q, state.ValidPhases[%d] = %q",
					i, i, d.ID, i, state.ValidPhases[i])
			}
		}
	})
}

// TestSkipTableDerivedFromRegistry verifies that SkipsForTemplate(state.TemplateLight)
// returns exactly the phase IDs whose TemplateSkips["light"] is true in phaseRegistry,
// in declaration order.
func TestSkipTableDerivedFromRegistry(t *testing.T) {
	t.Parallel()

	// Compute expected light skips from phaseRegistry directly (declaration order).
	var wantLightSkips []string
	for _, d := range phaseRegistry {
		if d.TemplateSkips[state.TemplateLight] {
			wantLightSkips = append(wantLightSkips, d.ID)
		}
	}

	got := SkipsForTemplate(state.TemplateLight)

	if !reflect.DeepEqual(got, wantLightSkips) {
		t.Errorf("SkipsForTemplate(%q) = %v, want %v (derived from phaseRegistry)", state.TemplateLight, got, wantLightSkips)
	}
}

// Package orchestrator provides pure-logic building blocks for the pipeline engine.
package orchestrator

import (
	"testing"
)

func TestAllPhasesCount(t *testing.T) {
	t.Parallel()

	const wantCount = 17

	if got := len(AllPhases); got != wantCount {
		t.Errorf("AllPhases length = %d, want %d", got, wantCount)
	}
}

func TestAllPhasesOrder(t *testing.T) {
	t.Parallel()

	want := []string{
		"setup", "phase-1", "phase-2", "phase-3", "phase-3b",
		"checkpoint-a", "phase-4", "phase-4b", "checkpoint-b",
		"phase-5", "phase-6", "phase-7", "final-verification",
		"final-summary", "pr-creation", "post-to-source", "completed",
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

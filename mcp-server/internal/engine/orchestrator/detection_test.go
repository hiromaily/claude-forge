// Package orchestrator provides pure-logic pipeline orchestration for the forge-state MCP server.
package orchestrator

import (
	"testing"
)

func TestDetectEffort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		flagEffort  string
		storyPoints int
		text        string
		want        string
	}{
		{name: "flag_override_wins", flagEffort: "L", storyPoints: 1, want: "L"},
		{name: "story_points_1_s", storyPoints: 1, want: "S"},
		{name: "story_points_2_s", storyPoints: 2, want: "S"},
		{name: "story_points_4_s_boundary", storyPoints: 4, want: "S"},
		{name: "story_points_5_m", storyPoints: 5, want: "M"},
		{name: "story_points_12_m_boundary", storyPoints: 12, want: "M"},
		{name: "story_points_13_l", storyPoints: 13, want: "L"},
		{name: "story_points_100_l", storyPoints: 100, want: "L"},
		{name: "story_points_zero_default", storyPoints: 0, want: "M"},
		{name: "story_points_negative_default", storyPoints: -1, want: "M"},
		{name: "default_empty_inputs", want: "M"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := DetectEffort(tc.flagEffort, tc.storyPoints, tc.text)
			if got != tc.want {
				t.Errorf("DetectEffort(%q, %d, %q) = %q, want %q",
					tc.flagEffort, tc.storyPoints, tc.text, got, tc.want)
			}
		})
	}
}

func TestEffortToTemplate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		effort string
		want   string
	}{
		{name: "s_maps_to_light", effort: "S", want: "light"},
		{name: "m_maps_to_standard", effort: "M", want: "standard"},
		{name: "l_maps_to_full", effort: "L", want: "full"},
		{name: "unknown_maps_to_standard", effort: "unknown", want: "standard"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := EffortToTemplate(tc.effort)
			if got != tc.want {
				t.Errorf("EffortToTemplate(%q) = %q, want %q", tc.effort, got, tc.want)
			}
		})
	}
}

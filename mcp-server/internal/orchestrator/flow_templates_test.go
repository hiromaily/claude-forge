package orchestrator

import (
	"reflect"
	"testing"
)

func TestSkipsForTemplate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		template string
		want     []string
	}{
		{
			name:     "standard template",
			template: TemplateStandard,
			want:     []string{PhaseFourB, PhaseCheckpointB},
		},
		{
			name:     "light template",
			template: TemplateLight,
			want:     []string{PhaseTwo, PhaseFour, PhaseFourB, PhaseCheckpointB, PhaseSeven},
		},
		{
			name:     "full template",
			template: TemplateFull,
			want:     []string{},
		},
		{
			name:     "unknown template returns nil",
			template: "unknown",
			want:     nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := SkipsForTemplate(tc.template)
			if tc.want == nil {
				if got != nil {
					t.Errorf("SkipsForTemplate(%q) = %v, want nil", tc.template, got)
				}

				return
			}

			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("SkipsForTemplate(%q) = %v, want %v", tc.template, got, tc.want)
			}
		})
	}
}

func TestSkipsForEffort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		effort string
		want   []string
	}{
		{
			name:   "S returns light skips",
			effort: "S",
			want:   []string{PhaseTwo, PhaseFour, PhaseFourB, PhaseCheckpointB, PhaseSeven},
		},
		{
			name:   "M returns standard skips",
			effort: "M",
			want:   []string{PhaseFourB, PhaseCheckpointB},
		},
		{
			name:   "L returns empty skips",
			effort: "L",
			want:   []string{},
		},
		{
			name:   "unknown falls back to standard skips",
			effort: "unknown",
			want:   []string{PhaseFourB, PhaseCheckpointB},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := SkipsForEffort(tc.effort)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("SkipsForEffort(%q) = %v, want %v", tc.effort, got, tc.want)
			}
		})
	}
}

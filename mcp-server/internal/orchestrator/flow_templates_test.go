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
			name:     "lite template",
			template: TemplateLite,
			want:     []string{PhaseFourB, PhaseCheckpointB, PhaseSix, PhaseSeven},
		},
		{
			name:     "direct template",
			template: TemplateDirect,
			want:     []string{PhaseOne, PhaseTwo, PhaseThree, PhaseFour, PhaseFourB, PhaseCheckpointB, PhaseSix, PhaseSeven},
		},
		{
			name:     "standard template",
			template: TemplateStandard,
			want:     []string{},
		},
		{
			name:     "light template",
			template: TemplateLight,
			want:     []string{PhaseFourB, PhaseCheckpointB, PhaseSeven},
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

func TestShouldSynthesizeStubs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		template string
		want     bool
	}{
		{
			name:     "direct returns true",
			template: TemplateDirect,
			want:     true,
		},
		{
			name:     "standard returns false",
			template: TemplateStandard,
			want:     false,
		},
		{
			name:     "lite returns false",
			template: TemplateLite,
			want:     false,
		},
		{
			name:     "light returns false",
			template: TemplateLight,
			want:     false,
		},
		{
			name:     "full returns false",
			template: TemplateFull,
			want:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := ShouldSynthesizeStubs(tc.template)
			if got != tc.want {
				t.Errorf("ShouldSynthesizeStubs(%q) = %v, want %v", tc.template, got, tc.want)
			}
		})
	}
}

// TestSkipsForCell covers all 20 (taskType, effort) cells from design.md.
func TestSkipsForCell(t *testing.T) {
	t.Parallel()

	tests := []struct {
		taskType string
		effort   string
		want     []string
	}{
		// feature row: XS→lite, S→light, M→standard, L→full
		{
			taskType: "feature",
			effort:   "XS",
			want:     []string{PhaseFourB, PhaseCheckpointB, PhaseSix, PhaseSeven},
		},
		{
			taskType: "feature",
			effort:   "S",
			want:     []string{PhaseFourB, PhaseCheckpointB, PhaseSeven},
		},
		{
			taskType: "feature",
			effort:   "M",
			want:     []string{},
		},
		{
			taskType: "feature",
			effort:   "L",
			want:     []string{},
		},

		// bugfix row: XS→direct, S→lite+phase-4, M→light+phase-4, L→standard+phase-4
		{
			taskType: "bugfix",
			effort:   "XS",
			want:     []string{PhaseOne, PhaseTwo, PhaseThree, PhaseFour, PhaseFourB, PhaseCheckpointB, PhaseSix, PhaseSeven},
		},
		{
			taskType: "bugfix",
			effort:   "S",
			want:     []string{PhaseFour, PhaseFourB, PhaseCheckpointB, PhaseSix, PhaseSeven},
		},
		{
			taskType: "bugfix",
			effort:   "M",
			want:     []string{PhaseFour, PhaseFourB, PhaseCheckpointB, PhaseSeven},
		},
		{
			taskType: "bugfix",
			effort:   "L",
			want:     []string{PhaseFour, PhaseFourB, PhaseCheckpointB, PhaseSeven},
		},

		// refactor row: XS→lite, S→light, M→standard, L→full
		{
			taskType: "refactor",
			effort:   "XS",
			want:     []string{PhaseFourB, PhaseCheckpointB, PhaseSix, PhaseSeven},
		},
		{
			taskType: "refactor",
			effort:   "S",
			want:     []string{PhaseFourB, PhaseCheckpointB, PhaseSeven},
		},
		{
			taskType: "refactor",
			effort:   "M",
			want:     []string{},
		},
		{
			taskType: "refactor",
			effort:   "L",
			want:     []string{},
		},

		// docs row: XS→direct, S→direct, M→lite+phase-2/3/4, L→light+phase-2/3/4
		{
			taskType: "docs",
			effort:   "XS",
			want:     []string{PhaseOne, PhaseTwo, PhaseThree, PhaseFour, PhaseFourB, PhaseCheckpointB, PhaseSix, PhaseSeven},
		},
		{
			taskType: "docs",
			effort:   "S",
			want:     []string{PhaseOne, PhaseTwo, PhaseThree, PhaseFour, PhaseFourB, PhaseCheckpointB, PhaseSix, PhaseSeven},
		},
		{
			taskType: "docs",
			effort:   "M",
			want:     []string{PhaseTwo, PhaseThree, PhaseFour, PhaseFourB, PhaseCheckpointB, PhaseSix, PhaseSeven},
		},
		{
			taskType: "docs",
			effort:   "L",
			want:     []string{PhaseTwo, PhaseThree, PhaseFour, PhaseFourB, PhaseCheckpointB, PhaseSeven},
		},

		// investigation row: XS/S/M/L all have same 11-element skip list
		{
			taskType: "investigation",
			effort:   "XS",
			want: []string{
				PhaseThree, PhaseThreeB, PhaseCheckpointA,
				PhaseFour, PhaseFourB, PhaseCheckpointB,
				PhaseFive, PhaseSix, PhaseSeven,
				PhaseFinalVerification, PhasePRCreation,
			},
		},
		{
			taskType: "investigation",
			effort:   "S",
			want: []string{
				PhaseThree, PhaseThreeB, PhaseCheckpointA,
				PhaseFour, PhaseFourB, PhaseCheckpointB,
				PhaseFive, PhaseSix, PhaseSeven,
				PhaseFinalVerification, PhasePRCreation,
			},
		},
		{
			taskType: "investigation",
			effort:   "M",
			want: []string{
				PhaseThree, PhaseThreeB, PhaseCheckpointA,
				PhaseFour, PhaseFourB, PhaseCheckpointB,
				PhaseFive, PhaseSix, PhaseSeven,
				PhaseFinalVerification, PhasePRCreation,
			},
		},
		{
			taskType: "investigation",
			effort:   "L",
			want: []string{
				PhaseThree, PhaseThreeB, PhaseCheckpointA,
				PhaseFour, PhaseFourB, PhaseCheckpointB,
				PhaseFive, PhaseSix, PhaseSeven,
				PhaseFinalVerification, PhasePRCreation,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.taskType+"/"+tc.effort, func(t *testing.T) {
			t.Parallel()

			got := SkipsForCell(tc.taskType, tc.effort)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("SkipsForCell(%q, %q) = %v, want %v", tc.taskType, tc.effort, got, tc.want)
			}
		})
	}
}

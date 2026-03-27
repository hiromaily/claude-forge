package orchestrator

// Template name constants — must match state.ValidTemplates values exactly.
const (
	TemplateDirect   = "direct"
	TemplateLite     = "lite"
	TemplateLight    = "light"
	TemplateStandard = "standard"
	TemplateFull     = "full"
)

// skipTable maps flow template name → ordered list of phases to skip at workspace setup.
var skipTable = map[string][]string{
	TemplateDirect:   {PhaseOne, PhaseTwo, PhaseThree, PhaseFour, PhaseFourB, PhaseCheckpointB, PhaseSix, PhaseSeven},
	TemplateLite:     {PhaseFourB, PhaseCheckpointB, PhaseSix, PhaseSeven},
	TemplateLight:    {PhaseFourB, PhaseCheckpointB, PhaseSeven},
	TemplateStandard: {},
	TemplateFull:     {},
}

// CellKey identifies a (taskType, effort) combination.
type CellKey struct {
	TaskType string
	Effort   string
}

// cellOverrides stores the complete per-cell skip lists for cells where task type
// modifies the template base. All 20 cells are covered between skipTable + cellOverrides.
// Source: SKILL.md lines 528–549 "Workspace Setup skip-phase calls" column.
var cellOverrides = map[CellKey][]string{
	// bugfix S: lite base + phase-4 supplement
	{TaskType: "bugfix", Effort: "S"}: {PhaseFour, PhaseFourB, PhaseCheckpointB, PhaseSix, PhaseSeven},
	// bugfix M: light base + phase-4 supplement
	{TaskType: "bugfix", Effort: "M"}: {PhaseFour, PhaseFourB, PhaseCheckpointB, PhaseSeven},
	// bugfix L: standard base + phase-4 supplement
	{TaskType: "bugfix", Effort: "L"}: {PhaseFour, PhaseFourB, PhaseCheckpointB, PhaseSeven},
	// docs M: lite base + phase-2/3/4 supplements
	{TaskType: "docs", Effort: "M"}: {PhaseTwo, PhaseThree, PhaseFour, PhaseFourB, PhaseCheckpointB, PhaseSix, PhaseSeven},
	// docs L: light base + phase-2/3/4 supplements
	{TaskType: "docs", Effort: "L"}: {PhaseTwo, PhaseThree, PhaseFour, PhaseFourB, PhaseCheckpointB, PhaseSeven},
	// investigation XS: lite base + all impl/delivery phases (11 skips total)
	{TaskType: "investigation", Effort: "XS"}: {
		PhaseThree, PhaseThreeB, PhaseCheckpointA,
		PhaseFour, PhaseFourB, PhaseCheckpointB,
		PhaseFive, PhaseSix, PhaseSeven,
		PhaseFinalVerification, PhasePRCreation,
	},
	// investigation S: same as XS (same template, same skips — SKILL.md lines 546–547)
	{TaskType: "investigation", Effort: "S"}: {
		PhaseThree, PhaseThreeB, PhaseCheckpointA,
		PhaseFour, PhaseFourB, PhaseCheckpointB,
		PhaseFive, PhaseSix, PhaseSeven,
		PhaseFinalVerification, PhasePRCreation,
	},
	// investigation M: light base + all impl/delivery phases
	{TaskType: "investigation", Effort: "M"}: {
		PhaseThree, PhaseThreeB, PhaseCheckpointA,
		PhaseFour, PhaseFourB, PhaseCheckpointB,
		PhaseFive, PhaseSix, PhaseSeven,
		PhaseFinalVerification, PhasePRCreation,
	},
	// investigation L: standard base + all impl/delivery phases
	{TaskType: "investigation", Effort: "L"}: {
		PhaseThree, PhaseThreeB, PhaseCheckpointA,
		PhaseFour, PhaseFourB, PhaseCheckpointB,
		PhaseFive, PhaseSix, PhaseSeven,
		PhaseFinalVerification, PhasePRCreation,
	},
}

// SkipsForCell returns the complete ordered skip list for a (taskType, effort) cell
// as used during Workspace Setup phase-skipping only. Phase 1 conditional skips
// (e.g., skip phase-2 after phase-1 completes for lite template) are NOT included —
// those are A3 engine responsibility. It prefers cellOverrides when present; falls
// back to the template base skipTable via DeriveFlowTemplate.
func SkipsForCell(taskType, effort string) []string {
	key := CellKey{TaskType: taskType, Effort: effort}
	if skips, ok := cellOverrides[key]; ok {
		return skips
	}

	// Fall back to the base skip list for the derived template.
	template := DeriveFlowTemplate(taskType, effort)

	return SkipsForTemplate(template)
}

// SkipsForTemplate returns the base skip list for a template name (ignores per-cell overrides).
// Returns nil for unknown templates.
func SkipsForTemplate(template string) []string {
	skips, ok := skipTable[template]
	if !ok {
		return nil
	}

	return skips
}

// ShouldSynthesizeStubs returns true when the flow template is "direct".
func ShouldSynthesizeStubs(template string) bool {
	return template == TemplateDirect
}

package orchestrator

// Template name constants — must match state.ValidTemplates values exactly.
const (
	TemplateLight    = "light"
	TemplateStandard = "standard"
	TemplateFull     = "full"
)

// skipTable maps flow template name → ordered list of phases to skip at workspace setup.
var skipTable = map[string][]string{
	TemplateLight:    {PhaseFourB, PhaseCheckpointB, PhaseSeven},
	TemplateStandard: {PhaseFourB, PhaseCheckpointB},
	TemplateFull:     {},
}

// SkipsForTemplate returns the base skip list for a template name.
// Returns nil for unknown templates.
func SkipsForTemplate(template string) []string {
	skips, ok := skipTable[template]
	if !ok {
		return nil
	}

	return skips
}

// SkipsForEffort returns the phase skip list for a given effort level.
// Delegates to EffortToTemplate to resolve the template, then SkipsForTemplate.
// Unknown effort values fall through to EffortToTemplate's default ("standard").
func SkipsForEffort(effort string) []string {
	return SkipsForTemplate(EffortToTemplate(effort))
}

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

// validEfforts are the known effort values recognised by SkipsForEffort.
var validEfforts = map[string]bool{
	"S": true,
	"M": true,
	"L": true,
}

// SkipsForEffort returns the phase skip list for a given effort level.
// Delegates to EffortToTemplate to resolve the template, then SkipsForTemplate.
// Returns an empty slice for unknown effort values (safe default: skip nothing).
func SkipsForEffort(effort string) []string {
	if !validEfforts[effort] {
		return []string{}
	}

	return SkipsForTemplate(EffortToTemplate(effort))
}

package orchestrator

// Template name constants — must match state.ValidTemplates values exactly.
const (
	TemplateLight    = "light"
	TemplateStandard = "standard"
	TemplateFull     = "full"
)

// skipTable maps flow template name → ordered list of phases to skip at workspace setup.
var skipTable = map[string][]string{
	TemplateLight:    {PhaseTwo, PhaseFour, PhaseFourB, PhaseCheckpointB, PhaseSeven},
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

// phaseLabels maps phase IDs to human-readable labels for display in effort_options.
var phaseLabels = map[string]string{
	PhaseTwo:         "Investigation",
	PhaseFour:        "Task Decomposition",
	PhaseFourB:       "Tasks AI Review",
	PhaseCheckpointA: "Design Checkpoint",
	PhaseCheckpointB: "Tasks Checkpoint",
	PhaseSeven:       "Comprehensive Review",
}

// PhaseLabel returns a human-readable label for a phase ID.
func PhaseLabel(phaseID string) string {
	if label, ok := phaseLabels[phaseID]; ok {
		return label
	}
	return phaseID
}

// SkipLabel represents a skipped phase with both its ID and human-readable label.
type SkipLabel struct {
	PhaseID string `json:"phase_id"`
	Label   string `json:"label"`
}

// SkipsWithLabelsForEffort returns the skip list with human-readable labels for an effort level.
func SkipsWithLabelsForEffort(effort string) []SkipLabel {
	ids := SkipsForEffort(effort)
	result := make([]SkipLabel, len(ids))
	for i, id := range ids {
		result[i] = SkipLabel{PhaseID: id, Label: PhaseLabel(id)}
	}
	return result
}

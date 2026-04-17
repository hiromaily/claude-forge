package orchestrator

import "github.com/hiromaily/claude-forge/mcp-server/internal/state"

// Template name constants — aliased from the state package to prevent silent divergence.
const (
	TemplateLight    = state.TemplateLight
	TemplateStandard = state.TemplateStandard
	TemplateFull     = state.TemplateFull
)

// skipTable maps flow template name → ordered list of phases to skip at workspace setup.
// Assigned by initRegistry() in registry.go from phaseRegistry TemplateSkips fields.
//
// Invariant: every valid template key is always present with a non-nil slice.
// The "full" template skips nothing, so its slice is []string{} (non-nil, length 0).
// SkipsForTemplate("full") therefore returns an empty slice, never nil.
// This invariant is established by initRegistry() pre-populating each template key
// before iterating phaseRegistry; a missing-or-nil entry for any valid template is a bug.
var skipTable map[string][]string

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
// Assigned by initRegistry() in registry.go from phaseRegistry Label fields.
var phaseLabels map[string]string

// PhaseLabel returns a human-readable label for a phase ID.
func PhaseLabel(phaseID string) string {
	if label, ok := phaseLabels[phaseID]; ok {
		return label
	}
	return phaseID
}

// PhaseLabels returns a copy of the phase ID → human-readable label map.
// Used by the dashboard to resolve labels on the client side.
func PhaseLabels() map[string]string {
	out := make(map[string]string, len(phaseLabels))
	for k, v := range phaseLabels {
		out[k] = v
	}
	return out
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

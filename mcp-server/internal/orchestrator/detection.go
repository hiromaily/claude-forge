// Package orchestrator provides pure-logic pipeline orchestration for the forge-state MCP server.
package orchestrator

import "strings"

// effortSmallKeywords are text patterns that suggest a small (S) effort task.
// These are checked case-insensitively against the combined summary+description.
var effortSmallKeywords = []string{
	"validation",
	"required",
	"optional",
	"rename",
	"typo",
	"label",
	"message",
	"toggle",
	"flag",
	"visibility",
	"hide",
	"show",
	"enable",
	"disable",
}

// effortLargeKeywords are text patterns that suggest a large (L) effort task.
var effortLargeKeywords = []string{
	"migration",
	"new service",
	"new api",
	"new endpoint",
	"redesign",
	"architecture",
	"rewrite",
}

// DetectEffort resolves effort from highest to lowest precedence:
//  1. flagEffort (non-empty string from --effort= flag)
//  2. storyPoints (int; <=0 means not provided)
//  3. text heuristic (keyword scoring over text)
//  4. default: "M"
func DetectEffort(flagEffort string, storyPoints int, text string) string {
	// 1. Flag override wins.
	if flagEffort != "" {
		return flagEffort
	}

	// 2. Story points mapping: SP ≤ 4 → S, SP ≤ 12 → M, SP > 12 → L.
	// <=0 means not provided; skip.
	if storyPoints >= 1 {
		switch {
		case storyPoints <= 4:
			return "S"
		case storyPoints <= 12:
			return "M"
		default:
			return "L"
		}
	}

	// 3. Text heuristic — keyword-based estimation.
	if text != "" {
		lower := strings.ToLower(text)

		// Check large keywords first (higher priority).
		for _, kw := range effortLargeKeywords {
			if strings.Contains(lower, kw) {
				return "L"
			}
		}

		// Check small keywords.
		for _, kw := range effortSmallKeywords {
			if strings.Contains(lower, kw) {
				return "S"
			}
		}
	}

	// 4. Default.
	return "M"
}

// EffortToTemplate maps an effort label to a flow template name.
// "S" → "light", "L" → "full", all other values (including "M" and unknown) → "standard".
func EffortToTemplate(effort string) string {
	switch effort {
	case "S":
		return TemplateLight
	case "L":
		return TemplateFull
	default: // "M" and unknown
		return TemplateStandard
	}
}

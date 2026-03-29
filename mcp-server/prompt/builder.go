package prompt

import (
	"fmt"
	"strings"

	"github.com/hiromaily/claude-forge/mcp-server/history"
)

const tokenBudget = 8_000

// estimateTokens returns a rough token estimate for a string.
// Approximation: 1 token ≈ 4 characters.
func estimateTokens(s string) int {
	return (len(s) + 3) / 4
}

// BuildPrompt assembles a 4-layer prompt for the given agent.
//
// Layer 1: agentInstructions (pre-read by caller from agents/{agentName}.md)
// Layer 2: artifactsSection (pre-built by caller)
// Layer 3: repository profile context (omitted if profile == "")
// Layer 4: data flywheel history context (omitted if ctx has no data for the agent)
//
// Token budget guard: truncates Layer 4 first (friction items from the end, then
// similar-pipeline items from the end, then pattern items from the end), then removes
// Layer 3 entirely. Layer 1 and Layer 2 are never truncated.
//
//nolint:gocyclo // complexity is inherent in the multi-layer assembly and budget guard
func BuildPrompt(agentName, agentInstructions, artifactsSection, profile string, ctx HistoryContext) string {
	rule, hasRule := agentRules[agentName]

	// Collect Layer 4 items based on agent rules.
	var frictionItems []history.FrictionPoint
	var patItems []history.PatternEntry
	var pipelines []history.SearchResult

	if hasRule {
		// Layer 4: Similar pipelines.
		if rule.Similar {
			pipelines = ctx.SimilarPipelines
		}

		// Layer 4: Patterns (CRITICAL only or all).
		switch rule.Patterns {
		case patternCriticalOnly:
			patItems = ctx.CriticalPatterns
		case patternAll:
			patItems = ctx.AllPatterns
		case patternNone:
			// no patterns included for this agent
		}

		// Layer 4: Friction points.
		if rule.Friction {
			frictionItems = ctx.FrictionPoints
		}
	}

	// Assemble the base (Layer 1 + Layer 2).
	base := agentInstructions
	if artifactsSection != "" {
		if base != "" {
			base += "\n\n"
		}

		base += artifactsSection
	}

	// If the base already exceeds the budget, return it as-is (Layer 1 and Layer 2
	// are never truncated).
	if estimateTokens(base) >= tokenBudget {
		return base
	}

	// Build Layer 3 string.
	layer3 := ""
	if profile != "" {
		layer3 = "## Repository Context\n\n" + profile
	}

	// Iteratively remove items from Layer 4 (friction first, then similar pipelines,
	// then patterns), then Layer 3, until the assembled prompt fits within tokenBudget.
	for {
		layer4 := buildLayer4(patItems, frictionItems, pipelines)
		combined := assembleLayers(base, layer3, layer4)

		if estimateTokens(combined) <= tokenBudget {
			return combined
		}

		// Remove from the end: friction first.
		if len(frictionItems) > 0 {
			frictionItems = frictionItems[:len(frictionItems)-1]
			continue
		}

		// Then similar pipelines.
		if len(pipelines) > 0 {
			pipelines = pipelines[:len(pipelines)-1]
			continue
		}

		// Then patterns.
		if len(patItems) > 0 {
			patItems = patItems[:len(patItems)-1]
			continue
		}

		// Layer 4 is now empty. Remove Layer 3.
		if layer3 != "" {
			layer3 = ""
			continue
		}

		// Nothing more to remove — return the base only.
		return base
	}
}

// assembleLayers joins the base with optional Layer 3 and Layer 4 strings.
func assembleLayers(base, layer3, layer4 string) string {
	var sb strings.Builder

	sb.WriteString(base)

	if layer3 != "" {
		sb.WriteString("\n\n")
		sb.WriteString(layer3)
	}

	if layer4 != "" {
		sb.WriteString("\n\n")
		sb.WriteString(layer4)
	}

	return sb.String()
}

// buildLayer4 assembles the Layer 4 string from the current item slices.
// Returns "" if all slices are empty. Section headers are only emitted when the
// corresponding slice is non-empty.
func buildLayer4(patItems []history.PatternEntry, frictionItems []history.FrictionPoint, pipelines []history.SearchResult) string {
	var sb strings.Builder

	if len(pipelines) > 0 {
		sb.WriteString("## Past Similar Pipelines\n\n")

		for _, r := range pipelines {
			fmt.Fprintf(&sb, "- %s: %s (similarity: %.2f)\n", r.SpecName, r.OneLiner, r.Similarity)
		}
	}

	if len(patItems) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}

		sb.WriteString("## Common Review Findings\n\n")

		for _, p := range patItems {
			fmt.Fprintf(&sb, "- %s: %q (seen %d times, %s)\n",
				p.Severity, p.Pattern, p.Frequency, p.Agent)
		}
	}

	if len(frictionItems) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}

		sb.WriteString("## Known AI Friction Points\n\n")

		for _, f := range frictionItems {
			fmt.Fprintf(&sb, "- %s: %s\n  Mitigation: %s\n",
				f.Category, f.Description, f.Mitigation)
		}
	}

	return sb.String()
}

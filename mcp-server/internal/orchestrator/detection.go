// Package orchestrator provides pure-logic pipeline orchestration for the forge-state MCP server.
package orchestrator

import "strings"

// Task type constants — exported; no equivalent in state/ package today.
const (
	TaskTypeFeature       = "feature"
	TaskTypeBugfix        = "bugfix"
	TaskTypeDocs          = "docs"
	TaskTypeRefactor      = "refactor"
	TaskTypeInvestigation = "investigation"
)

// ValidTaskTypes is the canonical set used by DetectTaskType's default fallback.
var ValidTaskTypes = []string{
	TaskTypeFeature,
	TaskTypeBugfix,
	TaskTypeDocs,
	TaskTypeRefactor,
	TaskTypeInvestigation,
}

// jiraTypeMap maps Jira issue type strings to internal task type constants.
var jiraTypeMap = map[string]string{
	"Bug":           TaskTypeBugfix,
	"Story":         TaskTypeFeature,
	"Epic":          TaskTypeFeature,
	"New Feature":   TaskTypeFeature,
	"Task":          TaskTypeFeature,
	"Sub-task":      TaskTypeFeature,
	"Documentation": TaskTypeDocs,
	"Improvement":   TaskTypeFeature,
}

// jiraTypeFallbackRules maps keyword substrings (case-insensitive) in Jira
// issue type names to task types. Used when the exact type name is not in
// jiraTypeMap, enabling support for localized issue type names without
// hardcoding every language variant.
var jiraTypeFallbackRules = []struct {
	substring string
	taskType  string
}{
	{"bug", TaskTypeBugfix},
	{"story", TaskTypeFeature},
	{"task", TaskTypeFeature},
	{"feature", TaskTypeFeature},
	{"improvement", TaskTypeFeature},
	{"doc", TaskTypeDocs},
}

// githubLabelRules maps label substrings (case-insensitive) to task types.
// Entries are checked in order; first match wins.
var githubLabelRules = []struct {
	substring string
	taskType  string
}{
	{"bug", TaskTypeBugfix},
	{"fix", TaskTypeBugfix},
	{"enhancement", TaskTypeFeature},
	{"feature", TaskTypeFeature},
	{"documentation", TaskTypeDocs},
	{"docs", TaskTypeDocs},
	{"refactor", TaskTypeRefactor},
	{"investigation", TaskTypeInvestigation},
	{"research", TaskTypeInvestigation},
}

// textHeuristicRules maps keyword substrings (case-insensitive) in text to task types.
// Higher specificity rules are listed first.
var textHeuristicRules = []struct {
	keyword  string
	taskType string
}{
	{"bug", TaskTypeBugfix},
	{"fix", TaskTypeBugfix},
	{"crash", TaskTypeBugfix},
	{"regression", TaskTypeBugfix},
	{"documentation", TaskTypeDocs},
	{"readme", TaskTypeDocs},
	{"refactor", TaskTypeRefactor},
	{"investigation", TaskTypeInvestigation},
	{"research", TaskTypeInvestigation},
}

// DetectTaskType resolves task type from highest to lowest precedence:
//  1. flagTaskType (non-empty string from --type= flag)
//  2. jiraType (Jira issue type, e.g. "Bug", "Story", "Task")
//  3. githubLabels ([]string of GitHub label names)
//  4. text heuristic (keyword scoring over text)
//  5. default: "feature"
func DetectTaskType(flagTaskType, jiraType string, githubLabels []string, text string) string {
	// 1. Flag override wins.
	if flagTaskType != "" {
		return flagTaskType
	}

	// 2. Jira type mapping (exact match, then substring fallback).
	if jiraType != "" {
		if mapped, ok := jiraTypeMap[jiraType]; ok {
			return mapped
		}
		// Substring fallback for localized issue type names.
		lower := strings.ToLower(jiraType)
		for _, rule := range jiraTypeFallbackRules {
			if strings.Contains(lower, rule.substring) {
				return rule.taskType
			}
		}
	}

	// 3. GitHub label substring matching (case-insensitive).
	for _, label := range githubLabels {
		lower := strings.ToLower(label)
		for _, rule := range githubLabelRules {
			if strings.Contains(lower, rule.substring) {
				return rule.taskType
			}
		}
	}

	// 4. Text heuristic keyword scoring.
	if text != "" {
		lower := strings.ToLower(text)
		for _, rule := range textHeuristicRules {
			if strings.Contains(lower, rule.keyword) {
				return rule.taskType
			}
		}
	}

	// 5. Default.
	return TaskTypeFeature
}

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

	// 2. Story points mapping.
	// <=0 means not provided; skip.
	if storyPoints >= 1 {
		switch {
		case storyPoints == 1:
			return "XS"
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

// deriveFlowTemplateMatrix is a map[taskType]map[effort]template for the 20-cell matrix.
// Source: design.md Section 3, authoritative SKILL.md 20-cell table.
// Template name string literals are used here to avoid a compile-time dependency on
// flow_templates.go (which may be implemented in a parallel task).
// These values must remain consistent with the TemplateXxx constants in flow_templates.go.
var deriveFlowTemplateMatrix = map[string]map[string]string{
	TaskTypeFeature: {
		"XS": "lite",
		"S":  "light",
		"M":  "standard",
		"L":  "full",
	},
	TaskTypeBugfix: {
		"XS": "direct",
		"S":  "lite",
		"M":  "light",
		"L":  "standard",
	},
	TaskTypeRefactor: {
		"XS": "lite",
		"S":  "light",
		"M":  "standard",
		"L":  "full",
	},
	TaskTypeDocs: {
		"XS": "direct",
		"S":  "direct",
		"M":  "lite",
		"L":  "light",
	},
	TaskTypeInvestigation: {
		"XS": "lite",
		"S":  "lite",
		"M":  "light",
		"L":  "standard",
	},
}

// DeriveFlowTemplate maps (taskType, effort) to a flow template name.
// Uses a map[string]map[string]string to avoid cyclomatic complexity violations.
// Unknown combinations default to "standard".
func DeriveFlowTemplate(taskType, effort string) string {
	if effortMap, ok := deriveFlowTemplateMatrix[taskType]; ok {
		if template, ok := effortMap[effort]; ok {
			return template
		}
	}
	return "standard"
}

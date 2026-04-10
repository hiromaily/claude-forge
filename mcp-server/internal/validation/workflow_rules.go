// Package validation — workflow_rules.go implements .specs/instructions.md
// loading and validation for phase-4 task-decomposer output.
//
// See docs/superpowers/specs/2026-04-10-workflow-instructions-design.md for
// the design rationale. The evaluator runs at phase-4 completion (before
// PhaseComplete) and returns Violations for tasks that match a rule's
// `when` conditions but do not have mode: human_gate set.
package validation

import (
	"regexp"
)

// WorkflowRules is the root YAML schema of .specs/instructions.md.
type WorkflowRules struct {
	Rules []Rule `yaml:"rules"`
}

// Rule is a single workflow enforcement rule.
type Rule struct {
	ID      string     `yaml:"id"`
	When    Conditions `yaml:"when"`
	Require string     `yaml:"require"`
	Reason  string     `yaml:"reason"`

	// compiledTitleRegex is set by LoadRules after YAML parse. Not serialised.
	compiledTitleRegex *regexp.Regexp `yaml:"-"`
}

// Conditions specifies when a rule matches a task.
type Conditions struct {
	FilesMatch   []string `yaml:"files_match,omitempty"`
	TitleMatches string   `yaml:"title_matches,omitempty"`
}

// Violation describes a single rule violation produced by Validate.
type Violation struct {
	TaskKey   string
	TaskTitle string
	RuleID    string
	Reason    string
	MatchedBy string
}

// WorkflowRulesFileName is the repo-relative path of the instructions file.
const WorkflowRulesFileName = ".specs/instructions.md"

// requireHumanGate is the only permitted value of Rule.Require in MVP.
const requireHumanGate = "human_gate"

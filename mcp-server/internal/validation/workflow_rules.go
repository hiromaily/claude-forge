// Package validation — workflow_rules.go implements .specs/instructions.md
// loading and validation for phase-4 task-decomposer output.
//
// See docs/superpowers/specs/2026-04-10-workflow-instructions-design.md for
// the design rationale. The evaluator runs at phase-4 completion (before
// PhaseComplete) and returns Violations for tasks that match a rule's
// `when` conditions but do not have mode: human_gate set.
package validation

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
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

// LoadRules reads and parses {repoRoot}/.specs/instructions.md.
//
// Behaviour:
//   - Returns (&WorkflowRules{}, nil) if the file does not exist (optional feature).
//   - Returns an error if the file exists but cannot be read, parsed,
//     or validated. Errors include field/line context where possible.
//   - Pre-compiles every rule's title_matches regex and stores the
//     compiled value in Rule.compiledTitleRegex.
//
// The markdown body after the closing `---` is ignored.
func LoadRules(repoRoot string) (*WorkflowRules, error) {
	path := filepath.Join(repoRoot, WorkflowRulesFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &WorkflowRules{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", WorkflowRulesFileName, err)
	}

	yamlBytes, err := extractFrontmatter(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", WorkflowRulesFileName, err)
	}

	var rules WorkflowRules
	dec := yaml.NewDecoder(strings.NewReader(string(yamlBytes)))
	dec.KnownFields(true) // reject unknown fields
	if err := dec.Decode(&rules); err != nil {
		return nil, fmt.Errorf("%s: parse YAML: %w", WorkflowRulesFileName, err)
	}

	for i := range rules.Rules {
		if err := validateAndCompileRule(&rules.Rules[i]); err != nil {
			return nil, fmt.Errorf("%s: rule[%d] (%q): %w",
				WorkflowRulesFileName, i, rules.Rules[i].ID, err)
		}
	}

	return &rules, nil
}

// extractFrontmatter returns the YAML payload between the opening and
// closing `---` fences. Returns an error if no frontmatter is present.
func extractFrontmatter(data []byte) ([]byte, error) {
	content := string(data)
	// Tolerate an optional leading UTF-8 BOM and blank lines.
	content = strings.TrimPrefix(content, "\ufeff")
	content = strings.TrimLeft(content, "\n\r\t ")

	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return nil, fmt.Errorf("missing YAML frontmatter: file must start with '---'")
	}

	// Find the closing fence. It must appear on its own line.
	// Walk lines starting after the opening fence.
	lines := strings.Split(content, "\n")
	if len(lines) < 2 {
		return nil, fmt.Errorf("unterminated YAML frontmatter")
	}
	// lines[0] is the opening "---"
	var yamlLines []string
	closed := false
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == "---" {
			closed = true
			break
		}
		yamlLines = append(yamlLines, lines[i])
	}
	if !closed {
		return nil, fmt.Errorf("unterminated YAML frontmatter (no closing '---' line)")
	}
	return []byte(strings.Join(yamlLines, "\n")), nil
}

// validateAndCompileRule checks structural invariants and pre-compiles regexes.
func validateAndCompileRule(r *Rule) error {
	if r.ID == "" {
		return fmt.Errorf("missing 'id'")
	}
	if r.Reason == "" {
		return fmt.Errorf("missing 'reason'")
	}
	if r.Require != requireHumanGate {
		return fmt.Errorf("require: %q not supported (only %q in MVP)", r.Require, requireHumanGate)
	}
	if len(r.When.FilesMatch) == 0 && r.When.TitleMatches == "" {
		return fmt.Errorf("when: must specify at least one of files_match, title_matches")
	}
	if r.When.TitleMatches != "" {
		re, err := regexp.Compile(r.When.TitleMatches)
		if err != nil {
			return fmt.Errorf("invalid title_matches regex: %w", err)
		}
		r.compiledTitleRegex = re
	}
	return nil
}

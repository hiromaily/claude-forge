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
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"gopkg.in/yaml.v3"

	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
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

// matchFiles returns the first pattern in patterns that matches any file
// in files, or "" if no combination matches. Patterns use doublestar
// syntax (supports `**` for recursive directory matching).
//
// Pattern iteration order is preserved (first-win) so stable error
// messages can reference the pattern that triggered the match.
func matchFiles(patterns, files []string) string {
	for _, p := range patterns {
		for _, f := range files {
			ok, err := doublestar.Match(p, f)
			if err != nil {
				// doublestar.Match only returns ErrBadPattern, which LoadRules
				// could in principle catch earlier. Here we swallow it and
				// move on to the next pattern rather than panicking on
				// unvalidated input.
				continue
			}
			if ok {
				return p
			}
		}
	}
	return ""
}

// matchTitle reports whether the task title matches the pre-compiled regex.
// A nil regex never matches (used by Conditions with no title_matches).
func matchTitle(re *regexp.Regexp, title string) bool {
	if re == nil {
		return false
	}
	return re.MatchString(title)
}

// Validate walks tasks and returns violations: tasks that match any rule's
// `when` conditions but do not have ExecutionMode == "human_gate".
//
// Semantics:
//   - Within a rule's `when`, all specified conditions must match (AND).
//   - A task may violate multiple rules; every violation is reported.
//   - Returns nil (empty slice) if rules is nil or has zero rules.
//   - Violations are sorted by (TaskKey asc, RuleID asc) for deterministic
//     error messages.
func Validate(tasks map[string]state.Task, rules *WorkflowRules) []Violation {
	if rules == nil || len(rules.Rules) == 0 {
		return nil
	}

	// Collect task keys in deterministic order.
	keys := make([]string, 0, len(tasks))
	for k := range tasks {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var violations []Violation
	for _, k := range keys {
		task := tasks[k]
		if task.ExecutionMode == state.ExecModeHumanGate {
			continue // already correctly marked; never a violation
		}
		for _, r := range rules.Rules {
			ok, matchedBy := ruleMatches(&r, task)
			if ok {
				violations = append(violations, Violation{
					TaskKey:   k,
					TaskTitle: task.Title,
					RuleID:    r.ID,
					Reason:    r.Reason,
					MatchedBy: matchedBy,
				})
			}
		}
	}
	return violations
}

// ruleMatches applies AND semantics across conditions. Returns (matched,
// matchedBy). matchedBy is a short human-readable tag indicating which
// condition(s) triggered the match — used in error messages.
func ruleMatches(r *Rule, task state.Task) (bool, string) {
	hasFiles := len(r.When.FilesMatch) > 0
	hasTitle := r.When.TitleMatches != ""

	var parts []string
	if hasFiles {
		matched := matchFiles(r.When.FilesMatch, task.Files)
		if matched == "" {
			return false, ""
		}
		parts = append(parts, "files_match:"+matched)
	}
	if hasTitle {
		if !matchTitle(r.compiledTitleRegex, task.Title) {
			return false, ""
		}
		parts = append(parts, "title_matches")
	}
	if len(parts) == 0 {
		// No conditions at all — should have been caught by LoadRules.
		// Treat as non-match to avoid false positives.
		return false, ""
	}
	return true, strings.Join(parts, ",")
}

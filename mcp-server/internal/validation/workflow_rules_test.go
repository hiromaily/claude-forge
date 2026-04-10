// Package validation_test contains external tests for the workflow_rules
// loader, Validate entrypoint, and FormatReviewFindings formatter — i.e.
// everything that can be covered via the exported API only.
package validation_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
	"github.com/hiromaily/claude-forge/mcp-server/internal/validation"
)

func TestLoadRules_HappyPath(t *testing.T) {
	t.Parallel()

	// LoadRules reads from {repoRoot}/.specs/instructions.md. We fake the
	// repo root by pointing at a temp dir that mimics the expected layout.
	tmp := t.TempDir()
	if err := copyFixture(t, "testdata/workflow_rules/rules_ok.md", filepath.Join(tmp, ".specs", "instructions.md")); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}

	rules, err := validation.LoadRules(tmp)
	if err != nil {
		t.Fatalf("LoadRules: %v", err)
	}
	if rules == nil {
		t.Fatal("LoadRules returned nil rules")
	}
	if got, want := len(rules.Rules), 2; got != want {
		t.Fatalf("rule count = %d, want %d", got, want)
	}

	r0 := rules.Rules[0]
	if r0.ID != "akupara-proto" {
		t.Errorf("rule[0].ID = %q, want %q", r0.ID, "akupara-proto")
	}
	if len(r0.When.FilesMatch) != 2 {
		t.Errorf("rule[0].When.FilesMatch len = %d, want 2", len(r0.When.FilesMatch))
	}
	if r0.Require != "human_gate" {
		t.Errorf("rule[0].Require = %q, want %q", r0.Require, "human_gate")
	}

	r1 := rules.Rules[1]
	if r1.When.TitleMatches != `(?i)drop\s+(table|column)` {
		t.Errorf("rule[1].When.TitleMatches = %q", r1.When.TitleMatches)
	}
}

// copyFixture copies a fixture file into dst, creating parent directories
// as needed. dst must be absolute (e.g. under t.TempDir()).
func copyFixture(t *testing.T, src, dst string) error {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func TestLoadRules_ParseErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		fixture     string
		wantErrSubs string
	}{
		{"unknown_field", "testdata/workflow_rules/rules_unknown_field.md", "field requires not found"},
		{"bad_regex", "testdata/workflow_rules/rules_bad_regex.md", "invalid title_matches regex"},
		{"bad_require", "testdata/workflow_rules/rules_bad_require.md", "not supported"},
		{"missing_id", "testdata/workflow_rules/rules_missing_id.md", "missing 'id'"},
		{"no_frontmatter", "testdata/workflow_rules/rules_no_frontmatter.md", "missing YAML frontmatter"},
		{"bad_glob", "testdata/workflow_rules/rules_bad_glob.md", "invalid files_match pattern"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tmp := t.TempDir()
			if err := copyFixture(t, tc.fixture, filepath.Join(tmp, ".specs", "instructions.md")); err != nil {
				t.Fatalf("copy fixture: %v", err)
			}
			_, err := validation.LoadRules(tmp)
			if err == nil {
				t.Fatalf("LoadRules(%s): expected error, got nil", tc.fixture)
			}
			if !strings.Contains(err.Error(), tc.wantErrSubs) {
				t.Errorf("LoadRules(%s): error %q does not contain %q",
					tc.fixture, err.Error(), tc.wantErrSubs)
			}
		})
	}
}

func TestLoadRules_Empty(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	if err := copyFixture(t, "testdata/workflow_rules/rules_empty.md", filepath.Join(tmp, ".specs", "instructions.md")); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	rules, err := validation.LoadRules(tmp)
	if err != nil {
		t.Fatalf("LoadRules: %v", err)
	}
	if len(rules.Rules) != 0 {
		t.Errorf("rule count = %d, want 0", len(rules.Rules))
	}
}

func TestLoadRules_MissingFile(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	// No .specs/instructions.md created.
	rules, err := validation.LoadRules(tmp)
	if err != nil {
		t.Fatalf("LoadRules on missing file: unexpected error %v", err)
	}
	if rules == nil {
		t.Fatal("LoadRules returned nil, want empty WorkflowRules{}")
	}
	if len(rules.Rules) != 0 {
		t.Errorf("rule count = %d, want 0", len(rules.Rules))
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	// Load rules through the exported LoadRules entrypoint so the
	// compiledTitleRegex is populated. Using a fixture file keeps this
	// test honest about the public API.
	tmp := t.TempDir()
	if err := copyFixture(t, "testdata/workflow_rules/rules_validate.md", filepath.Join(tmp, ".specs", "instructions.md")); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	rules, err := validation.LoadRules(tmp)
	if err != nil {
		t.Fatalf("LoadRules: %v", err)
	}

	tasks := map[string]state.Task{
		"1": {
			Title:         "Update deal proto",
			Files:         []string{"backend/pkg/api/deal.proto"},
			ExecutionMode: "sequential",
		},
		"2": {
			Title:         "Refactor helper",
			Files:         []string{"backend/pkg/util/helper.go"},
			ExecutionMode: "sequential",
		},
		"3": {
			Title:         "Drop column users.legacy_token",
			Files:         []string{"backend/migrations/003.sql"},
			ExecutionMode: "human_gate", // already correct — no violation
		},
		"4": {
			Title:         "Drop column users.old",
			Files:         []string{"backend/migrations/004.sql"},
			ExecutionMode: "sequential", // violates drop-col rule
		},
	}

	violations := validation.Validate(tasks, rules)

	// Task 1 violates akupara-proto. Task 4 violates drop-col.
	// Task 2 is clean. Task 3 is already human_gate.
	if got := len(violations); got != 2 {
		t.Fatalf("len(violations) = %d, want 2: %+v", got, violations)
	}

	// Validate returns violations in deterministic TaskKey order.
	if violations[0].TaskKey != "1" || violations[0].RuleID != "akupara-proto" {
		t.Errorf("violations[0] = %+v, want task 1 / akupara-proto", violations[0])
	}
	if violations[1].TaskKey != "4" || violations[1].RuleID != "drop-col" {
		t.Errorf("violations[1] = %+v, want task 4 / drop-col", violations[1])
	}
}

func TestValidate_AndSemantics(t *testing.T) {
	t.Parallel()

	// Rule requires BOTH files_match AND title_matches.
	tmp := t.TempDir()
	if err := copyFixture(t, "testdata/workflow_rules/rules_and_semantics.md", filepath.Join(tmp, ".specs", "instructions.md")); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	rules, err := validation.LoadRules(tmp)
	if err != nil {
		t.Fatalf("LoadRules: %v", err)
	}

	tasks := map[string]state.Task{
		// files match but title does not
		"1": {Title: "Add index", Files: []string{"backend/migrations/010.sql"}, ExecutionMode: "sequential"},
		// title matches but files do not
		"2": {Title: "Drop unused flag", Files: []string{"frontend/app/flags.ts"}, ExecutionMode: "sequential"},
		// both match
		"3": {Title: "Drop column", Files: []string{"backend/migrations/011.sql"}, ExecutionMode: "sequential"},
	}

	v := validation.Validate(tasks, rules)
	if len(v) != 1 || v[0].TaskKey != "3" {
		t.Errorf("Validate = %+v, want exactly task 3 violating", v)
	}
}

func TestValidate_EmptyRules(t *testing.T) {
	t.Parallel()

	tasks := map[string]state.Task{
		"1": {Title: "Anything", Files: []string{"a.go"}, ExecutionMode: "sequential"},
	}
	if got := validation.Validate(tasks, &validation.WorkflowRules{}); len(got) != 0 {
		t.Errorf("Validate with empty rules = %+v, want []", got)
	}
	if got := validation.Validate(tasks, nil); len(got) != 0 {
		t.Errorf("Validate with nil rules = %+v, want []", got)
	}
}

func TestFormatReviewFindings(t *testing.T) {
	t.Parallel()

	violations := []validation.Violation{
		{
			TaskKey:   "1",
			TaskTitle: "Update deal proto",
			RuleID:    "akupara-proto",
			Reason:    "akupara-proto coordination required",
			MatchedBy: "files_match:backend/**/*.proto",
		},
		{
			TaskKey:   "4",
			TaskTitle: "Drop column users.old",
			RuleID:    "drop-col",
			Reason:    "stakeholder approval required",
			MatchedBy: "title_matches",
		},
	}
	got := validation.FormatReviewFindings(violations)

	wantSubstrings := []string{
		"REVISE",
		"Task 1: Update deal proto",
		"rule: akupara-proto",
		"akupara-proto coordination required",
		"mode: human_gate",
		"Task 4: Drop column users.old",
		"rule: drop-col",
		"stakeholder approval required",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(got, want) {
			t.Errorf("FormatReviewFindings output missing %q\n---\n%s\n---", want, got)
		}
	}
}

func TestFormatReviewFindings_Empty(t *testing.T) {
	t.Parallel()

	got := validation.FormatReviewFindings(nil)
	if got != "" {
		t.Errorf("FormatReviewFindings(nil) = %q, want empty", got)
	}
}

package validation

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
)

var (
	osReadFile  = os.ReadFile
	osMkdirAll  = os.MkdirAll
	osWriteFile = os.WriteFile
)

func TestLoadRules_HappyPath(t *testing.T) {
	// LoadRules reads from {repoRoot}/.specs/instructions.md. We fake the
	// repo root by pointing at a fixture dir that already contains
	// .specs/instructions.md — except our fixtures live in
	// testdata/workflow_rules/, so we construct an equivalent path below.
	//
	// Strategy: copy the fixture into a temp directory that mimics the
	// expected layout (tmpdir/.specs/instructions.md), then call LoadRules.
	tmp := t.TempDir()
	if err := copyFixture(t, "testdata/workflow_rules/rules_ok.md", filepath.Join(tmp, ".specs", "instructions.md")); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}

	rules, err := LoadRules(tmp)
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
	if r1.compiledTitleRegex == nil {
		t.Error("rule[1].compiledTitleRegex is nil, want compiled")
	}
}

// copyFixture copies a fixture file into dst, creating parent directories
// as needed. dst must be absolute (e.g. under t.TempDir()).
func copyFixture(t *testing.T, src, dst string) error {
	t.Helper()
	data, err := readFile(src)
	if err != nil {
		return err
	}
	if err := mkdirAll(filepath.Dir(dst)); err != nil {
		return err
	}
	return writeFile(dst, data)
}

// readFile / mkdirAll / writeFile are trivial wrappers around os/io.
// They exist to keep the test body focused. Implementations go below.

func readFile(p string) ([]byte, error)  { return osReadFile(p) }
func mkdirAll(p string) error            { return osMkdirAll(p, 0o755) }
func writeFile(p string, b []byte) error { return osWriteFile(p, b, 0o644) }

func TestLoadRules_ParseErrors(t *testing.T) {
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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			if err := copyFixture(t, tc.fixture, filepath.Join(tmp, ".specs", "instructions.md")); err != nil {
				t.Fatalf("copy fixture: %v", err)
			}
			_, err := LoadRules(tmp)
			if err == nil {
				t.Fatalf("LoadRules(%s): expected error, got nil", tc.fixture)
			}
			if !contains(err.Error(), tc.wantErrSubs) {
				t.Errorf("LoadRules(%s): error %q does not contain %q",
					tc.fixture, err.Error(), tc.wantErrSubs)
			}
		})
	}
}

func TestLoadRules_Empty(t *testing.T) {
	tmp := t.TempDir()
	if err := copyFixture(t, "testdata/workflow_rules/rules_empty.md", filepath.Join(tmp, ".specs", "instructions.md")); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	rules, err := LoadRules(tmp)
	if err != nil {
		t.Fatalf("LoadRules: %v", err)
	}
	if len(rules.Rules) != 0 {
		t.Errorf("rule count = %d, want 0", len(rules.Rules))
	}
}

// contains returns true iff substr appears in s. Local helper to avoid
// pulling strings into the test file's top-of-file import block twice.
func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestMatchFiles(t *testing.T) {
	cases := []struct {
		name     string
		patterns []string
		files    []string
		want     string // matched pattern, or "" if no match
	}{
		{
			name:     "recursive_proto",
			patterns: []string{"backend/**/*.proto"},
			files:    []string{"backend/pkg/api/deal.proto"},
			want:     "backend/**/*.proto",
		},
		{
			name:     "any_file_matches_wins",
			patterns: []string{"**/*.sql"},
			files:    []string{"README.md", "backend/migrations/001.sql"},
			want:     "**/*.sql",
		},
		{
			name:     "no_match",
			patterns: []string{"**/*.proto"},
			files:    []string{"backend/pkg/api/deal.go"},
			want:     "",
		},
		{
			name:     "multiple_patterns_first_wins",
			patterns: []string{"**/*.proto", "**/*.sql"},
			files:    []string{"m.sql"},
			want:     "**/*.sql",
		},
		{
			name:     "empty_files",
			patterns: []string{"**/*"},
			files:    nil,
			want:     "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchFiles(tc.patterns, tc.files)
			if got != tc.want {
				t.Errorf("matchFiles() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLoadRules_MissingFile(t *testing.T) {
	tmp := t.TempDir()
	// No .specs/instructions.md created.
	rules, err := LoadRules(tmp)
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

func TestMatchTitle(t *testing.T) {
	re := mustCompile(t, `(?i)drop\s+(table|column)`)
	cases := []struct {
		name  string
		title string
		want  bool
	}{
		{"match_upper", "DROP COLUMN users.legacy", true},
		{"match_lower", "drop table old_events", true},
		{"match_with_prefix", "[task 3] drop column", true},
		{"no_match", "add column users.new", false},
		{"nil_regex_no_match", "anything", false}, // see case below
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var useRegex *regexp.Regexp
			if tc.name != "nil_regex_no_match" {
				useRegex = re
			}
			got := matchTitle(useRegex, tc.title)
			if got != tc.want {
				t.Errorf("matchTitle() = %v, want %v", got, tc.want)
			}
		})
	}
}

func mustCompile(t *testing.T, pat string) *regexp.Regexp {
	t.Helper()
	re, err := regexp.Compile(pat)
	if err != nil {
		t.Fatalf("compile %q: %v", pat, err)
	}
	return re
}

func TestValidate(t *testing.T) {
	// Build a rules struct directly instead of going through LoadRules
	// so this test isolates Validate behaviour.
	rules := &WorkflowRules{
		Rules: []Rule{
			{
				ID:      "akupara-proto",
				When:    Conditions{FilesMatch: []string{"backend/**/*.proto"}},
				Require: "human_gate",
				Reason:  "akupara-proto coordination required",
			},
			{
				ID:                 "drop-col",
				When:               Conditions{TitleMatches: `(?i)drop column`},
				Require:            "human_gate",
				Reason:             "stakeholder approval required",
				compiledTitleRegex: mustCompile(t, `(?i)drop column`),
			},
		},
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

	violations := Validate(tasks, rules)

	// Task 1 violates akupara-proto. Task 4 violates drop-col.
	// Task 2 is clean. Task 3 is already human_gate.
	if got := len(violations); got != 2 {
		t.Fatalf("len(violations) = %d, want 2: %+v", got, violations)
	}

	// Violations should be sorted by TaskKey for determinism.
	sort.SliceStable(violations, func(i, j int) bool {
		return violations[i].TaskKey < violations[j].TaskKey
	})

	if violations[0].TaskKey != "1" || violations[0].RuleID != "akupara-proto" {
		t.Errorf("violations[0] = %+v, want task 1 / akupara-proto", violations[0])
	}
	if violations[1].TaskKey != "4" || violations[1].RuleID != "drop-col" {
		t.Errorf("violations[1] = %+v, want task 4 / drop-col", violations[1])
	}
}

func TestValidate_AndSemantics(t *testing.T) {
	// Rule requires BOTH files_match AND title_matches.
	rules := &WorkflowRules{
		Rules: []Rule{
			{
				ID: "both",
				When: Conditions{
					FilesMatch:   []string{"backend/migrations/**/*.sql"},
					TitleMatches: `(?i)drop`,
				},
				Require:            "human_gate",
				Reason:             "combined",
				compiledTitleRegex: mustCompile(t, `(?i)drop`),
			},
		},
	}

	tasks := map[string]state.Task{
		// files match but title does not
		"1": {Title: "Add index", Files: []string{"backend/migrations/010.sql"}, ExecutionMode: "sequential"},
		// title matches but files do not
		"2": {Title: "Drop unused flag", Files: []string{"frontend/app/flags.ts"}, ExecutionMode: "sequential"},
		// both match
		"3": {Title: "Drop column", Files: []string{"backend/migrations/011.sql"}, ExecutionMode: "sequential"},
	}

	v := Validate(tasks, rules)
	if len(v) != 1 || v[0].TaskKey != "3" {
		t.Errorf("Validate = %+v, want exactly task 3 violating", v)
	}
}

func TestValidate_EmptyRules(t *testing.T) {
	tasks := map[string]state.Task{
		"1": {Title: "Anything", Files: []string{"a.go"}, ExecutionMode: "sequential"},
	}
	if got := Validate(tasks, &WorkflowRules{}); len(got) != 0 {
		t.Errorf("Validate with empty rules = %+v, want []", got)
	}
	if got := Validate(tasks, nil); len(got) != 0 {
		t.Errorf("Validate with nil rules = %+v, want []", got)
	}
}

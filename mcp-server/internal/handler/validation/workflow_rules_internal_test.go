// Package validation contains internal tests for unexported workflow_rules
// helpers (matchFiles, matchTitle). These tests need access to package-private
// symbols and therefore declare `package validation` rather than the external
// test package used by workflow_rules_test.go.
package validation

import (
	"regexp"
	"testing"
)

func TestMatchFiles(t *testing.T) {
	t.Parallel()

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
			t.Parallel()
			got := matchFiles(tc.patterns, tc.files)
			if got != tc.want {
				t.Errorf("matchFiles() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMatchTitle(t *testing.T) {
	t.Parallel()

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
			t.Parallel()
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

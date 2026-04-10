package validation

import (
	"os"
	"path/filepath"
	"testing"
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

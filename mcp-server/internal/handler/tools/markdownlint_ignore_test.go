package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newSpecsWorkspace creates a repo-root/.specs/<spec> layout under t.TempDir and
// returns (repoRoot, workspace).
func newSpecsWorkspace(t *testing.T) (string, string) {
	t.Helper()
	repoRoot := t.TempDir()
	workspace := filepath.Join(repoRoot, ".specs", "20260601-some-spec")
	if err := os.MkdirAll(workspace, 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	return repoRoot, workspace
}

func TestEnsureMarkdownlintIgnoreCreatesFile(t *testing.T) {
	t.Parallel()
	repoRoot, workspace := newSpecsWorkspace(t)

	ensureMarkdownlintIgnore(workspace)

	data, err := os.ReadFile(filepath.Join(repoRoot, ".markdownlintignore"))
	if err != nil {
		t.Fatalf("expected .markdownlintignore to be created: %v", err)
	}
	if !markdownlintIgnoreCoversSpecs(string(data)) {
		t.Errorf(".markdownlintignore does not cover .specs; content:\n%s", data)
	}
}

func TestEnsureMarkdownlintIgnoreAppendsToExisting(t *testing.T) {
	t.Parallel()
	repoRoot, workspace := newSpecsWorkspace(t)
	ignorePath := filepath.Join(repoRoot, ".markdownlintignore")
	existing := "node_modules/\nCHANGELOG.md\n"
	if err := os.WriteFile(ignorePath, []byte(existing), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	ensureMarkdownlintIgnore(workspace)

	data, err := os.ReadFile(ignorePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := string(data)
	if !markdownlintIgnoreCoversSpecs(got) {
		t.Errorf("expected .specs entry to be appended; content:\n%s", got)
	}
	// Pre-existing user content must be preserved.
	for _, want := range []string{"node_modules/", "CHANGELOG.md"} {
		if !strings.Contains(got, want) {
			t.Errorf("existing entry %q was lost; content:\n%s", want, got)
		}
	}
}

func TestEnsureMarkdownlintIgnoreIdempotent(t *testing.T) {
	t.Parallel()
	repoRoot, workspace := newSpecsWorkspace(t)
	ignorePath := filepath.Join(repoRoot, ".markdownlintignore")

	ensureMarkdownlintIgnore(workspace)
	first, err := os.ReadFile(ignorePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	ensureMarkdownlintIgnore(workspace)
	second, err := os.ReadFile(ignorePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(first) != string(second) {
		t.Errorf("second call changed the file:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestEnsureMarkdownlintIgnoreSkipsFlatLayout(t *testing.T) {
	t.Parallel()
	// Flat layout: workspace is a bare temp dir not under .specs/.
	flat := t.TempDir()
	ensureMarkdownlintIgnore(flat)
	// No .markdownlintignore should be created next to or above a flat workspace.
	if _, err := os.Stat(filepath.Join(filepath.Dir(flat), ".markdownlintignore")); err == nil {
		t.Errorf("flat layout should not create a .markdownlintignore")
	}
}

// Package tools — unit tests for git_ops.go (repoRoot and executeBatchCommit).
package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/state"
	"github.com/hiromaily/claude-forge/mcp-server/internal/intelligence/history"
)

// TestPRBodyFileWithClosingRef verifies that prBodyFileWithClosingRef returns
// the original path when closingRef is empty, and creates a combined temp file
// with the closing reference appended when closingRef is non-empty.
func TestPRBodyFileWithClosingRef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		summaryContent string
		closingRef     string
		wantSamePath   bool   // true = returned path must equal summaryPath
		wantContains   string // substring that must appear in file content
	}{
		{
			name:           "no_closing_ref",
			summaryContent: "# Summary\n\nSome content.\n",
			closingRef:     "",
			wantSamePath:   true,
			wantContains:   "# Summary",
		},
		{
			name:           "with_closing_ref",
			summaryContent: "# Summary\n\nSome content.\n",
			closingRef:     "\n\nCloses #42",
			wantSamePath:   false,
			wantContains:   "Closes #42",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			summaryPath := filepath.Join(dir, "summary.md")
			if err := os.WriteFile(summaryPath, []byte(tc.summaryContent), 0o600); err != nil {
				t.Fatalf("write summary.md: %v", err)
			}

			got, err := prBodyFileWithClosingRef(summaryPath, tc.closingRef)
			if err != nil {
				t.Fatalf("prBodyFileWithClosingRef: %v", err)
			}
			if got != summaryPath {
				defer func() { _ = os.Remove(got) }()
			}

			if tc.wantSamePath && got != summaryPath {
				t.Errorf("expected same path %q, got %q", summaryPath, got)
			}
			if !tc.wantSamePath && got == summaryPath {
				t.Errorf("expected temp file path, got original summary path %q", got)
			}

			content, err := os.ReadFile(got)
			if err != nil {
				t.Fatalf("ReadFile(%q): %v", got, err)
			}
			if !strings.Contains(string(content), tc.wantContains) {
				t.Errorf("file content %q does not contain %q", string(content), tc.wantContains)
			}
		})
	}
}

// initGitRepo initialises a bare git repository in dir so that repoRoot and
// git commands work correctly.  A dummy initial commit is created so that
// `git diff --name-only HEAD` does not fail with "no commits yet".
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")

	// Create and commit a placeholder file so HEAD exists.
	placeholder := filepath.Join(dir, "README.md")
	if err := os.WriteFile(placeholder, []byte("placeholder\n"), 0o600); err != nil {
		t.Fatalf("write placeholder: %v", err)
	}
	run("add", "README.md")
	run("commit", "-m", "chore: initial commit")
}

// TestRepoRoot verifies that repoRoot returns the canonical absolute path of a
// git repository root when called with a workspace directory inside that repo.
func TestRepoRoot(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	initGitRepo(t, dir)

	got, err := repoRoot(dir)
	if err != nil {
		t.Fatalf("repoRoot(%q) returned error: %v", dir, err)
	}

	// git rev-parse --show-toplevel returns a resolved, canonical path.
	// filepath.EvalSymlinks resolves any OS-level symlinks in t.TempDir() (e.g.
	// /var -> /private/var on macOS) so the comparison is stable across platforms.
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", dir, err)
	}
	want = filepath.Clean(want)

	if got != want {
		t.Errorf("repoRoot(%q) = %q, want %q", dir, got, want)
	}
}

// TestRepoRoot_NonRepo verifies that repoRoot returns a wrapped error when the
// directory is not inside a git repository.
func TestRepoRoot_NonRepo(t *testing.T) {
	t.Parallel()

	// Use a plain temp dir — not git-initialised.
	dir := t.TempDir()

	_, err := repoRoot(dir)
	if err == nil {
		t.Errorf("repoRoot(%q) expected error for non-repo dir, got nil", dir)
	}
	if !strings.Contains(err.Error(), "git rev-parse --show-toplevel") {
		t.Errorf("error %q does not mention 'git rev-parse --show-toplevel'", err.Error())
	}
}

// TestExecuteBatchCommit_EmptyFiles verifies that executeBatchCommit exercises
// the fallback `git diff --name-only HEAD` path when Tasks have empty Files
// lists, and returns no error (empty diff → no-op warning, no panic).
func TestExecuteBatchCommit_EmptyFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	initGitRepo(t, dir)

	// Create a StateManager with the workspace pointing at the git repo.
	sm := state.NewStateManager("dev")
	if err := sm.Init(dir, "test-spec"); err != nil {
		t.Fatalf("sm.Init: %v", err)
	}

	// Set up a parallel task with an empty Files list and completed status.
	// This forces executeBatchCommit to fall back to `git diff --name-only HEAD`.
	if err := sm.Update(func(s *state.State) error {
		s.Tasks = map[string]state.Task{
			"1": {
				Title:         "Task 1",
				ExecutionMode: state.ExecModeParallel,
				ImplStatus:    state.TaskStatusCompleted,
				Files:         []string{}, // intentionally empty — triggers fallback
			},
		}
		s.NeedsBatchCommit = true
		return nil
	}); err != nil {
		t.Fatalf("sm.Update: %v", err)
	}

	// After the initial commit there are no staged/modified tracked files, so
	// `git diff --name-only HEAD` returns empty output.  The function must
	// return a non-empty warning (the no-op message) and nil error.
	warning, err := executeBatchCommit(dir, sm)
	if err != nil {
		t.Fatalf("executeBatchCommit returned unexpected error: %v", err)
	}

	// The fallback no-op path must produce a warning explaining the situation.
	if warning == "" {
		t.Errorf("executeBatchCommit with no changed files: expected non-empty warning, got empty string")
	}
}

// initGitRepoWithRemote initialises a working git repository in dir with a
// local bare repository as its "origin" remote.  The initial commit is pushed
// so that --force-with-lease succeeds in executeFinalCommit.  Returns the path
// of the bare repository.
func initGitRepoWithRemote(t *testing.T, dir string) {
	t.Helper()

	bareDir := t.TempDir()

	runIn := func(wd string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = wd
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v in %s: %v\n%s", args, wd, err, out)
		}
	}

	// Create the bare remote.
	runIn(bareDir, "init", "--bare")

	// Initialise the working repo and push to it.
	initGitRepo(t, dir)
	runIn(dir, "remote", "add", "origin", bareDir)
	// push.default=current means `git push --force-with-lease` (no args) pushes
	// the current branch to a remote branch of the same name — works regardless
	// of whether the local default branch is "main" or "master".
	runIn(dir, "config", "push.default", "current")
	runIn(dir, "push", "-u", "origin", "HEAD")
}

// TestExecuteFinalCommit_Success verifies that executeFinalCommit advances
// state to "completed", force-adds workspace artifacts, amends the last
// commit, and pushes successfully when a remote is configured.
func TestExecuteFinalCommit_Success(t *testing.T) {
	// Not parallel — modifies git working tree state.
	dir := t.TempDir()
	initGitRepoWithRemote(t, dir)

	sm := state.NewStateManager("dev")
	if err := sm.Init(dir, "test-spec"); err != nil {
		t.Fatalf("sm.Init: %v", err)
	}

	// Create summary.md so git add -f succeeds.
	summaryPath := filepath.Join(dir, "summary.md")
	if err := os.WriteFile(summaryPath, []byte("# Summary\n"), 0o600); err != nil {
		t.Fatalf("write summary.md: %v", err)
	}

	kb := history.NewKnowledgeBase("")
	if err := executeFinalCommit(dir, sm, kb); err != nil {
		t.Fatalf("executeFinalCommit returned unexpected error: %v", err)
	}

	// Verify state.json on disk reflects completed status.
	s, err := sm.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if s.CurrentPhase != state.PhaseCompleted {
		t.Errorf("CurrentPhase = %q, want %q", s.CurrentPhase, state.PhaseCompleted)
	}
	if s.CurrentPhaseStatus != state.StatusCompleted {
		t.Errorf("CurrentPhaseStatus = %q, want %q", s.CurrentPhaseStatus, state.StatusCompleted)
	}
}

// TestExecuteFinalCommit_PushFails verifies that executeFinalCommit returns an
// error when the git push step fails (no remote configured).  State is still
// advanced to "completed" in step 1 before the push is attempted.
func TestExecuteFinalCommit_PushFails(t *testing.T) {
	// Not parallel — modifies git working tree state.
	dir := t.TempDir()
	initGitRepo(t, dir) // no remote added

	sm := state.NewStateManager("dev")
	if err := sm.Init(dir, "test-spec"); err != nil {
		t.Fatalf("sm.Init: %v", err)
	}

	summaryPath := filepath.Join(dir, "summary.md")
	if err := os.WriteFile(summaryPath, []byte("# Summary\n"), 0o600); err != nil {
		t.Fatalf("write summary.md: %v", err)
	}

	kb := history.NewKnowledgeBase("")
	err := executeFinalCommit(dir, sm, kb)
	if err == nil {
		t.Fatal("executeFinalCommit expected error when push has no remote, got nil")
	}
	if !strings.Contains(err.Error(), "executeFinalCommit push") {
		t.Errorf("error %q does not mention 'executeFinalCommit push'", err.Error())
	}
}

// TestIsGitIgnored verifies the isGitIgnored helper against actual .gitignore rules.
func TestIsGitIgnored(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	initGitRepo(t, dir)

	// Write a .gitignore that ignores the .specs directory.
	gitignore := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(gitignore, []byte(".specs/\n"), 0o600); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "ignored_file", path: ".specs/state.json", want: true},
		{name: "ignored_nested", path: ".specs/20260408-test/summary.md", want: true},
		{name: "not_ignored", path: "src/main.go", want: false},
		{name: "readme", path: "README.md", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isGitIgnored(dir, tc.path)
			if got != tc.want {
				t.Errorf("isGitIgnored(%q, %q) = %v, want %v", dir, tc.path, got, tc.want)
			}
		})
	}
}

// TestIsGitIgnored_NoGitignore verifies that isGitIgnored returns false (fail-open)
// when no .gitignore exists.
func TestIsGitIgnored_NoGitignore(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	initGitRepo(t, dir)

	if got := isGitIgnored(dir, ".specs/state.json"); got {
		t.Errorf("isGitIgnored with no .gitignore = true, want false")
	}
}

// TestIsGitIgnored_NegationPattern verifies that .gitignore negation patterns
// (e.g. !.specs/**/state.json) correctly mark files as not-ignored.
func TestIsGitIgnored_NegationPattern(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	initGitRepo(t, dir)

	// Ignore .specs/ but exclude state.json and summary.md via negation.
	// The !.specs/*/ line is required to unignore intermediate directories;
	// without it, git ignores the parent dir and negation for children has no effect.
	gitignore := filepath.Join(dir, ".gitignore")
	content := ".specs/**\n!.specs/*/\n!.specs/**/state.json\n!.specs/**/summary.md\n"
	if err := os.WriteFile(gitignore, []byte(content), 0o600); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "state_json_not_ignored", path: ".specs/20260408-test/state.json", want: false},
		{name: "summary_md_not_ignored", path: ".specs/20260408-test/summary.md", want: false},
		{name: "design_md_ignored", path: ".specs/20260408-test/design.md", want: true},
		{name: "analysis_md_ignored", path: ".specs/20260408-test/analysis.md", want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isGitIgnored(dir, tc.path)
			if got != tc.want {
				t.Errorf("isGitIgnored(%q, %q) = %v, want %v", dir, tc.path, got, tc.want)
			}
		})
	}
}

// TestExecuteFinalCommit_GitIgnored verifies that executeFinalCommit skips the
// artifact commit (git add -f + amend) when the workspace directory is gitignored.
func TestExecuteFinalCommit_GitIgnored(t *testing.T) {
	dir := t.TempDir()
	initGitRepoWithRemote(t, dir)

	// Write a .gitignore that excludes the workspace entirely.
	gitignore := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(gitignore, []byte(".specs/\n"), 0o600); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	// Commit the .gitignore so git knows about it.
	runIn := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runIn("add", ".gitignore")
	runIn("commit", "-m", "chore: add .gitignore")
	runIn("push")

	sm := state.NewStateManager("dev")
	// The workspace is inside .specs/ which is gitignored.
	wsDir := filepath.Join(dir, ".specs", "20260408-test")
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := sm.Init(wsDir, "test-spec"); err != nil {
		t.Fatalf("sm.Init: %v", err)
	}

	summaryPath := filepath.Join(wsDir, "summary.md")
	if err := os.WriteFile(summaryPath, []byte("# Summary\n"), 0o600); err != nil {
		t.Fatalf("write summary.md: %v", err)
	}

	kb := history.NewKnowledgeBase("")
	if err := executeFinalCommit(wsDir, sm, kb); err != nil {
		t.Fatalf("executeFinalCommit returned unexpected error: %v", err)
	}

	// Verify state.json reflects completed status (handleReportResult still runs).
	s, err := sm.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if s.CurrentPhase != state.PhaseCompleted {
		t.Errorf("CurrentPhase = %q, want %q", s.CurrentPhase, state.PhaseCompleted)
	}

	// Verify state.json was NOT added to the last commit.
	cmd := exec.Command("git", "-C", dir, "log", "--oneline", "-1", "--name-only")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "state.json") {
		t.Errorf("state.json was committed despite being gitignored: %s", out)
	}
}

// TestExecuteBatchCommit_EmptyFiles_MultiTask verifies that the fallback path
// is taken even when multiple parallel tasks are present but all have empty
// Files lists.
func TestExecuteBatchCommit_EmptyFiles_MultiTask(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	initGitRepo(t, dir)

	sm := state.NewStateManager("dev")
	if err := sm.Init(dir, "test-spec"); err != nil {
		t.Fatalf("sm.Init: %v", err)
	}

	if err := sm.Update(func(s *state.State) error {
		s.Tasks = map[string]state.Task{
			"1": {
				Title:         "Task 1",
				ExecutionMode: state.ExecModeParallel,
				ImplStatus:    state.TaskStatusCompleted,
				Files:         nil,
			},
			"2": {
				Title:         "Task 2",
				ExecutionMode: state.ExecModeParallel,
				ImplStatus:    state.TaskStatusCompleted,
				Files:         []string{},
			},
			"3": {
				// Sequential task — should not contribute files regardless.
				Title:         "Task 3",
				ExecutionMode: state.ExecModeSequential,
				ImplStatus:    state.TaskStatusCompleted,
				Files:         []string{"some/file.go"},
			},
		}
		s.NeedsBatchCommit = true
		return nil
	}); err != nil {
		t.Fatalf("sm.Update: %v", err)
	}

	// Sequential task files must not be included; parallel tasks have no files →
	// fallback path; no changed tracked files → no-op warning, nil error.
	warning, err := executeBatchCommit(dir, sm)
	if err != nil {
		t.Fatalf("executeBatchCommit returned unexpected error: %v", err)
	}
	if warning == "" {
		t.Errorf("executeBatchCommit with no changed files: expected non-empty warning, got empty string")
	}
}

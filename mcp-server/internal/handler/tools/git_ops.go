// git operations for batch and final commit absorption.
// These functions are called internally by PipelineNextActionHandler to execute
// git operations that were previously delegated to the orchestrator via exec actions.

package tools

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/orchestrator"
	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/state"
	"github.com/hiromaily/claude-forge/mcp-server/internal/intelligence/history"
)

// repoRoot returns the absolute path of the git repository root for the given
// workspace directory. It runs `git -C <workspace> rev-parse --show-toplevel`
// and returns the trimmed stdout. On failure, the error includes full stdout and
// stderr for diagnostics.
func repoRoot(workspace string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("git", "-C", workspace, "rev-parse", "--show-toplevel")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git rev-parse --show-toplevel: %w\nstdout: %s\nstderr: %s",
			err, stdout.String(), stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}

// currentGitBranch returns the current branch name (e.g. "feature/foo")
// for the given working directory. Returns empty string on error (detached HEAD,
// not a git repo, etc.).
func currentGitBranch(dir string) string {
	var stdout bytes.Buffer
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(stdout.String())
}

// runGit executes a git command and returns a wrapped error with stdout/stderr
// included when the command fails. The first argument is the working-directory
// flag value (used with `git -C <dir>`); remaining args are the git sub-command
// and its arguments.
func runGit(dir string, args ...string) error {
	fullArgs := append([]string{"-C", dir}, args...)
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("git", fullArgs...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w\nstdout: %s\nstderr: %s",
			strings.Join(args, " "), err, stdout.String(), stderr.String())
	}
	return nil
}

// isGitIgnored checks whether path is ignored by .gitignore rules in the
// repository rooted at repo. It uses `git check-ignore -q` which exits 0 when
// the path IS ignored and 1 when it is NOT ignored. Any other error (e.g. repo
// not found) is treated as "not ignored" (fail-open) to avoid blocking the
// pipeline on unexpected git failures.
func isGitIgnored(repo, path string) bool {
	cmd := exec.Command("git", "-C", repo, "check-ignore", "-q", path)
	return cmd.Run() == nil // exit 0 → ignored
}

// executeBatchCommit stages and commits all files changed by completed parallel tasks.
//
// Algorithm:
//  1. Load state to read Tasks.
//  2. Collect Files from parallel+completed tasks. Fall back to `git diff --name-only HEAD`
//     when the collected list is empty (covers implementers that modified files
//     without populating Tasks[k].Files).
//  3. Detect untracked files via `git status --short --porcelain`. Untracked files
//     are NOT staged — they are surfaced in the returned warning string so the
//     operator can see what was omitted.
//  4. Run `git add -- <files>` then `git commit -m "chore: batch commit parallel tasks"`.
//
// The warning return value (when non-empty) is surfaced in nextActionResponse.Warning
// by the PipelineNextActionHandler caller; it is not logged internally.
//
//nolint:gocyclo // complexity is inherent in the multi-step fallback and detection logic
func executeBatchCommit(workspace string, sm *state.StateManager) (warning string, err error) {
	s, err := sm.GetState()
	if err != nil {
		return "", fmt.Errorf("executeBatchCommit load state: %w", err)
	}

	repo, err := repoRoot(workspace)
	if err != nil {
		return "", err
	}

	// Step 1: Collect files from parallel+completed tasks.
	var files []string
	for _, t := range s.Tasks {
		if t.ExecutionMode == state.ExecModeParallel && t.ImplStatus == state.TaskStatusCompleted {
			files = append(files, t.Files...)
		}
	}

	// Step 2: Fall back to `git diff --name-only HEAD` when file list is empty.
	if len(files) == 0 {
		var stdout bytes.Buffer
		cmd := exec.Command("git", "-C", repo, "diff", "--name-only", "HEAD")
		cmd.Stdout = &stdout
		if runErr := cmd.Run(); runErr != nil {
			// Fail-open: if the diff command fails (e.g. no commits yet), return
			// a warning instead of blocking the pipeline.
			return fmt.Sprintf("git diff fallback failed: %v", runErr), nil
		}
		raw := strings.TrimSpace(stdout.String())
		if raw == "" {
			// No changed tracked files — nothing to commit; this is a no-op.
			return "batch commit: no changed files detected (git diff --name-only HEAD returned empty)", nil
		}
		for line := range strings.SplitSeq(raw, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				files = append(files, line)
			}
		}
	}

	// Step 3: Detect untracked files and include in warning (do NOT stage them).
	var warnings []string
	{
		var stdout bytes.Buffer
		cmd := exec.Command("git", "-C", repo, "status", "--short", "--porcelain")
		cmd.Stdout = &stdout
		if runErr := cmd.Run(); runErr == nil {
			var untracked []string
			for line := range strings.SplitSeq(stdout.String(), "\n") {
				if after, ok := strings.CutPrefix(line, "?? "); ok {
					path := strings.TrimSpace(after)
					if path != "" {
						untracked = append(untracked, path)
					}
				}
			}
			if len(untracked) > 0 {
				warnings = append(warnings, "untracked files not committed (add manually if needed): "+
					strings.Join(untracked, ", "))
			}
		}
	}

	// Step 4: Filter out non-existent paths to prevent "pathspec did not match" errors.
	// Task decomposers may list paths that don't exist on disk (e.g., guessed filenames).
	var validFiles []string
	var skippedFiles []string
	for _, f := range files {
		absPath := filepath.Join(repo, f)
		if _, statErr := os.Stat(absPath); statErr == nil {
			validFiles = append(validFiles, f)
		} else {
			skippedFiles = append(skippedFiles, f)
		}
	}
	if len(skippedFiles) > 0 {
		warnings = append(warnings, fmt.Sprintf("skipped %d non-existent paths: %s",
			len(skippedFiles), strings.Join(skippedFiles, ", ")))
	}
	if len(validFiles) == 0 {
		// All paths were invalid — fall back to git diff.
		var stdout bytes.Buffer
		cmd := exec.Command("git", "-C", repo, "diff", "--name-only", "HEAD")
		cmd.Stdout = &stdout
		if runErr := cmd.Run(); runErr == nil {
			raw := strings.TrimSpace(stdout.String())
			for line := range strings.SplitSeq(raw, "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					validFiles = append(validFiles, line)
				}
			}
		}
		if len(validFiles) == 0 {
			return strings.Join(warnings, "; "), nil
		}
	}
	files = validFiles

	// Step 5: Stage files.
	addArgs := append([]string{"add", "--"}, files...)
	if err := runGit(repo, addArgs...); err != nil {
		return strings.Join(warnings, "; "), fmt.Errorf("batch commit stage: %w", err)
	}

	// Step 6: Commit.
	if err := runGit(repo, "commit", "-m", "chore: batch commit parallel tasks"); err != nil {
		return strings.Join(warnings, "; "), fmt.Errorf("batch commit: %w", err)
	}

	return strings.Join(warnings, "; "), nil
}

// executeFinalCommit finalises the pipeline by:
//  1. Calling handleReportResult with the final-commit phase to write state.json as completed.
//  2. Determining the repository root directory.
//  3. Updating the PR body with the final summary.md (best-effort, non-fatal).
//     This is necessary because pr-creation runs BEFORE final-summary —
//     summary.md does not exist at PR creation time.
//     See handlePRCreation in engine.go for the design rationale.
//  4. Force-adding workspace/summary.md and workspace/state.json — but only files
//     that are NOT gitignored. Each file is checked individually via isGitIgnored
//     so that negation patterns (e.g. !.specs/**/state.json) are respected.
//     When all artifact files are gitignored, step 5 (amend) is skipped entirely.
//  5. Amending the last commit (--no-edit) to include the staged state files.
//  6. Force-pushing with --force-with-lease.
//
// Ordering invariant: handleReportResult must be called first so that state.json
// on disk reflects the completed status before it is staged by `git add -f`.
//
// On any git failure the full stdout/stderr is included in the returned error.
// No rollback is attempted — state is already written as completed.
func executeFinalCommit(workspace string, sm *state.StateManager, kb *history.KnowledgeBase) error {
	// Step 1: Advance state to completed via handleReportResult.
	in := reportResultInput{
		workspace: workspace,
		phase:     state.PhaseFinalCommit,
	}
	if _, err := handleReportResult(sm, kb, in); err != nil {
		return fmt.Errorf("executeFinalCommit handleReportResult: %w", err)
	}

	// Step 2: Determine repo root.
	repo, err := repoRoot(workspace)
	if err != nil {
		return err
	}

	// Step 3: Update PR body with the final summary.md.
	// pr-creation phase used a placeholder body because summary.md is generated
	// later in the final-summary phase. Now that summary.md exists, replace the
	// PR body with the complete version. `gh pr edit` targets the PR on the
	// current branch — no explicit PR number needed.
	// If this is a GitHub issue pipeline, append "Closes #N" so that the reference
	// persists in the final PR body (the initial placeholder body added it, but
	// --body-file would overwrite it without this step).
	// Best-effort: if `gh` is not available or the edit fails (e.g. CI, test env),
	// log a warning and continue — the PR body will retain the placeholder but
	// the commit and push must still succeed.
	summaryPath := filepath.Join(workspace, "summary.md")
	if _, statErr := os.Stat(summaryPath); statErr != nil {
		fmt.Fprintf(os.Stderr, "warning: executeFinalCommit: summary.md not found, skipping PR body update\n")
	} else {
		bodyFile, closingRefErr := prBodyFileWithClosingRef(summaryPath, orchestrator.ClosingRef(workspace))
		if closingRefErr != nil {
			fmt.Fprintf(os.Stderr, "warning: executeFinalCommit: failed to build PR body: %v\n", closingRefErr)
		} else {
			defer func() {
				if bodyFile != summaryPath {
					_ = os.Remove(bodyFile)
				}
			}()
			if err := runCommand(repo, "gh", "pr", "edit", "--body-file", bodyFile); err != nil {
				fmt.Fprintf(os.Stderr, "warning: executeFinalCommit: gh pr edit failed (non-fatal): %v\n", err)
			}
		}
	}

	// Step 4: Force-add workspace artifacts (state.json and summary.md if it exists).
	// Each file is checked individually against .gitignore so that negation
	// patterns are respected per-file. When all candidates are gitignored the
	// amend step is skipped entirely.
	statePath := filepath.Join(workspace, "state.json")
	candidates := []string{statePath}
	if _, statErr := os.Stat(summaryPath); statErr == nil {
		candidates = append(candidates, summaryPath)
	}

	var addFiles []string
	for _, f := range candidates {
		if isGitIgnored(repo, f) {
			fmt.Fprintf(os.Stderr, "info: executeFinalCommit: %s is gitignored, skipping\n", filepath.Base(f))
		} else {
			addFiles = append(addFiles, f)
		}
	}

	if len(addFiles) > 0 {
		addArgs := append([]string{"add", "-f"}, addFiles...)
		if err := runGit(repo, addArgs...); err != nil {
			return fmt.Errorf("executeFinalCommit add: %w", err)
		}

		// Step 5: Amend the last commit to include the staged state files.
		if err := runGit(repo, "commit", "--amend", "--no-edit"); err != nil {
			return fmt.Errorf("executeFinalCommit amend: %w", err)
		}
	}

	// Step 6: Push with --force-with-lease to protect against concurrent pushes.
	if err := runGit(repo, "push", "--force-with-lease"); err != nil {
		return fmt.Errorf("executeFinalCommit push: %w", err)
	}

	return nil
}

// prBodyFileWithClosingRef returns the path to a file suitable for `gh pr edit --body-file`.
// When closingRef is empty the original summaryPath is returned unchanged.
// When closingRef is non-empty the summary content is combined with the closing
// reference into a temp file and that temp file path is returned; the caller is
// responsible for deleting it.
func prBodyFileWithClosingRef(summaryPath, closingRef string) (string, error) {
	if closingRef == "" {
		return summaryPath, nil
	}
	content, err := os.ReadFile(summaryPath)
	if err != nil {
		return "", fmt.Errorf("prBodyFileWithClosingRef read: %w", err)
	}
	content = append(content, []byte(closingRef)...)
	tmp, err := os.CreateTemp("", "forge-pr-body-*.md")
	if err != nil {
		return "", fmt.Errorf("prBodyFileWithClosingRef tempfile: %w", err)
	}
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", fmt.Errorf("prBodyFileWithClosingRef write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return "", fmt.Errorf("prBodyFileWithClosingRef close: %w", err)
	}
	return tmp.Name(), nil
}

// runCommand executes a non-git command with the given working directory.
// Unlike runGit (which uses `git -C <dir>`), this sets cmd.Dir because
// non-git commands like `gh` do not have a `-C` equivalent.
// Returns a wrapped error with stdout/stderr on failure.
func runCommand(dir string, name string, args ...string) error {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w\nstdout: %s\nstderr: %s",
			name, strings.Join(args, " "), err, stdout.String(), stderr.String())
	}
	return nil
}

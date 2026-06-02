// markdownlint ignore management for forge spec artifacts.

package tools

import (
	"os"
	"path/filepath"
	"strings"
)

// markdownlintIgnoreEntry is the pattern forge adds so host-project markdownlint
// runs skip the internal spec artifacts (request.md, design.md, tasks.md, impl-*.md,
// summary.md, review-*.md) that forge writes under .specs/.
const markdownlintIgnoreEntry = ".specs/"

// markdownlintIgnoreHeader documents the generated entry so a human editing the file
// understands why it is there.
const markdownlintIgnoreHeader = "# Added by claude-forge: internal spec artifacts are not product docs."

// ensureMarkdownlintIgnore writes or updates a .markdownlintignore at the repo root so
// that forge's internal spec artifacts under .specs/ do not generate markdownlint noise
// (MD032/MD036/MD034 etc.) in the host project (improvement #10).
//
// It is best-effort and idempotent:
//   - only acts on the canonical .specs/<spec-name>/ workspace layout (flat/test layouts
//     are skipped so no file is created in unexpected locations);
//   - if .markdownlintignore is absent it is created with the .specs/ entry;
//   - if it exists, the entry is appended only when no equivalent .specs ignore is present,
//     preserving any user-authored content;
//   - all I/O errors are swallowed: failing to write a lint-ignore must never break init.
func ensureMarkdownlintIgnore(workspace string) {
	if filepath.Base(filepath.Dir(workspace)) != ".specs" {
		return // flat layout (e.g. test fixtures) — nothing to anchor the repo root on.
	}
	repoRoot := filepath.Dir(filepath.Dir(workspace))
	ignorePath := filepath.Join(repoRoot, ".markdownlintignore")

	data, err := os.ReadFile(ignorePath)
	switch {
	case err == nil:
		if markdownlintIgnoreCoversSpecs(string(data)) {
			return
		}
		updated := strings.TrimRight(string(data), "\n") + "\n" + markdownlintIgnoreEntry + "\n"
		_ = os.WriteFile(ignorePath, []byte(updated), 0o600) //nolint:gosec // fixed repo-root .markdownlintignore path, not user input
	case os.IsNotExist(err):
		content := markdownlintIgnoreHeader + "\n" + markdownlintIgnoreEntry + "\n"
		_ = os.WriteFile(ignorePath, []byte(content), 0o600)
	default:
		// Unreadable for some other reason — leave it untouched rather than clobber.
	}
}

// markdownlintIgnoreCoversSpecs reports whether the given .markdownlintignore content
// already ignores the .specs directory in any of its common spellings.
func markdownlintIgnoreCoversSpecs(content string) bool {
	for line := range strings.SplitSeq(content, "\n") {
		switch strings.TrimRight(strings.TrimSpace(line), "/") {
		case ".specs", ".specs/**", "/.specs":
			return true
		}
	}
	return false
}

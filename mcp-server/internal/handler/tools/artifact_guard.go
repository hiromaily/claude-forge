// artifact presence guards for pipeline_report_result.
// Contains functions that check for missing impl/review artifact files
// to enforce deterministic phase completion gates.

package tools

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/state"
)

// reviewFileTaskKey extracts the task key from a review file basename.
// e.g. "review-1.md" → "1", "review-task-abc.md" → "task-abc".
// Returns "" if the filename does not match the review-*.md or impl-*.md pattern.
func reviewFileTaskKey(filename string) string {
	base := filepath.Base(filename)
	switch {
	case strings.HasPrefix(base, "review-") && strings.HasSuffix(base, ".md"):
		return strings.TrimSuffix(strings.TrimPrefix(base, "review-"), ".md")
	case strings.HasPrefix(base, "impl-") && strings.HasSuffix(base, ".md"):
		return strings.TrimSuffix(strings.TrimPrefix(base, "impl-"), ".md")
	}
	return ""
}

// missingArtifactFiles returns task keys whose {prefix}{key}.md file does not exist on disk.
// Human-gate tasks are excluded because they produce no artifact file.
func missingArtifactFiles(workspace, prefix string, tasks map[string]state.Task) []string {
	var missing []string
	for k, t := range tasks {
		if t.ExecutionMode == state.ExecModeHumanGate {
			continue
		}
		if _, err := os.Stat(filepath.Join(workspace, prefix+k+".md")); err != nil {
			missing = append(missing, k)
		}
	}
	sort.Strings(missing)
	return missing
}

// missingReviewFiles returns task keys whose review-{key}.md file does not exist on disk.
// Used as a deterministic completion gate for Phase 6 — prevents the phase from
// advancing when some tasks lack review artifacts.
// Human-gate tasks are excluded because they skip review and produce no review file.
func missingReviewFiles(workspace string, tasks map[string]state.Task) []string {
	return missingArtifactFiles(workspace, "review-", tasks)
}

// Package tools — task_ops.go implements ParseTasksMd and executeTaskInit
// for absorbing the task_init action type inside PipelineNextActionHandler (P2).
package tools

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
)

// taskHeadingRE matches lines of the form "## Task N: <title>"
// where N is one or more digits. The title capture group is optional.
var taskHeadingRE = regexp.MustCompile(`^##\s+Task\s+(\d+):\s*(.*)$`)

// ParseTasksMd parses the canonical tasks.md format into a map of task number
// strings to state.Task values.
//
// The canonical format for each task section is:
//
//	## Task N: <title>
//	<free-form description paragraphs>
//	mode: sequential|parallel
//	files:
//	- path/to/file.go
//	- path/to/other.go
//	depends_on: [1, 2]   (optional; comma-separated integers inside brackets)
//
// Parsing rules (lenient heuristic — see design.md Section 3, A3 decision):
//   - Task sections are delimited by "## Task N:" headings (N is an integer).
//   - mode: is matched case-insensitively; defaults to "sequential" when absent.
//   - files: introduces a bullet list; each line starting with "- " is a file path.
//   - depends_on: is a bracketed or plain comma-separated integer list on the same line.
//   - Unknown lines within a task section are ignored (not an error).
//   - Returns an error when no "## Task N:" heading is found in content.
//
//nolint:gocyclo // complexity is inherent in the lenient line-by-line parser
func ParseTasksMd(content string) (map[string]state.Task, error) {
	lines := strings.Split(content, "\n")

	type taskEntry struct {
		num     string
		task    state.Task
		inFiles bool
	}

	var entries []*taskEntry
	var current *taskEntry

	for _, rawLine := range lines {
		line := strings.TrimRight(rawLine, "\r")

		// Check for a new task heading.
		if m := taskHeadingRE.FindStringSubmatch(line); m != nil {
			// Flush previous task into slice.
			if current != nil {
				entries = append(entries, current)
			}
			title := strings.TrimSpace(m[2])
			current = &taskEntry{
				num: m[1],
				task: state.Task{
					Title:         title,
					ExecutionMode: "sequential", // default per spec
					ImplStatus:    "pending",
					ReviewStatus:  "pending",
				},
			}
			continue
		}

		// Skip lines before the first task heading.
		if current == nil {
			continue
		}

		trimmed := strings.TrimSpace(line)

		// File bullet list entry.
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "  - ") {
			// Only accumulate if we are currently inside a "files:" block.
			if current.inFiles {
				filePath := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "  - "), "- "))
				if filePath != "" {
					current.task.Files = append(current.task.Files, filePath)
				}
				continue
			}
		}

		lower := strings.ToLower(trimmed)

		// mode: field.
		if strings.HasPrefix(lower, "mode:") {
			val := strings.TrimSpace(trimmed[len("mode:"):])
			if strings.EqualFold(val, "parallel") {
				current.task.ExecutionMode = "parallel"
			} else {
				current.task.ExecutionMode = "sequential"
			}
			current.inFiles = false
			continue
		}

		// files: header — subsequent "- " lines are file paths.
		if strings.ToLower(trimmed) == "files:" {
			current.inFiles = true
			continue
		}

		// depends_on: field.
		if strings.HasPrefix(lower, "depends_on:") {
			current.inFiles = false
			raw := strings.TrimSpace(trimmed[len("depends_on:"):])
			// Strip surrounding brackets if present.
			raw = strings.TrimPrefix(raw, "[")
			raw = strings.TrimSuffix(raw, "]")
			for part := range strings.SplitSeq(raw, ",") {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				n, err := strconv.Atoi(part)
				if err == nil {
					current.task.DependsOn = append(current.task.DependsOn, n)
				}
			}
			continue
		}

		// Any non-bullet, non-blank line outside a files block ends the files block.
		if trimmed != "" && !strings.HasPrefix(line, "- ") && !strings.HasPrefix(line, "  - ") {
			current.inFiles = false
		}
	}

	// Flush the last task.
	if current != nil {
		entries = append(entries, current)
	}

	if len(entries) == 0 {
		return nil, errors.New("ParseTasksMd: no '## Task N:' heading found in content")
	}

	result := make(map[string]state.Task, len(entries))
	for _, e := range entries {
		result[e.num] = e.task
	}
	return result, nil
}

// executeTaskInit reads tasks.md from the workspace, parses it, and calls
// sm.TaskInit to store the tasks in state.json.
//
// Invariant: This function does NOT call Guard3gCheckpointBDoneOrSkipped.
// This bypass is intentional and safe: the engine only emits ActionTaskInit
// from handlePhaseFive, which is reached only after checkpoint-b is already
// complete. The guard is therefore redundant at this call site and would
// incorrectly block execution if checkpoint-b's status is "completed" rather
// than the "awaiting_human" state the guard checks for.
func executeTaskInit(phase string, sm *state.StateManager) error {
	// Load state to get the workspace path.
	s, err := sm.GetState()
	if err != nil {
		return fmt.Errorf("executeTaskInit(%s): load state: %w", phase, err)
	}
	workspace := s.Workspace

	// Read workspace/tasks.md.
	tasksPath := filepath.Join(workspace, "tasks.md")
	data, err := os.ReadFile(tasksPath)
	if err != nil {
		return fmt.Errorf("executeTaskInit(%s): read %s: %w", phase, tasksPath, err)
	}

	// Parse the tasks.md content.
	tasks, err := ParseTasksMd(string(data))
	if err != nil {
		return fmt.Errorf("executeTaskInit(%s): parse tasks.md: %w", phase, err)
	}

	// Store tasks in state.json directly via TaskInit.
	// Guard3gCheckpointBDoneOrSkipped is intentionally NOT called here —
	// see the invariant note in the function doc above.
	if err := sm.TaskInit(workspace, tasks); err != nil {
		return fmt.Errorf("executeTaskInit(%s): TaskInit: %w", phase, err)
	}

	return nil
}

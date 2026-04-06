// Package tools — task_ops_test.go: table-driven tests for ParseTasksMd.
package tools

import (
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
)

// TestParseTasksMd exercises the canonical tasks.md parsing logic across a
// variety of inputs including well-formed, edge-case, and malformed documents.
func TestParseTasksMd(t *testing.T) {
	t.Parallel()

	type tc struct {
		name    string
		content string
		// expected values — checked only when wantErr is false
		wantKeys  []string
		wantTasks map[string]state.Task
		wantErr   bool
	}

	cases := []tc{
		{
			name: "sequential mode",
			content: `## Task 1: Do the thing
Some description text.
mode: sequential
files:
- cmd/main.go
- internal/foo.go
`,
			wantKeys: []string{"1"},
			wantTasks: map[string]state.Task{
				"1": {
					Title:         "Do the thing",
					ExecutionMode: "sequential",
					Files:         []string{"cmd/main.go", "internal/foo.go"},
					ImplStatus:    "pending",
					ReviewStatus:  "pending",
				},
			},
		},
		{
			name: "parallel mode",
			content: `## Task 2: Parallel task
mode: parallel
files:
- pkg/bar.go
`,
			wantKeys: []string{"2"},
			wantTasks: map[string]state.Task{
				"2": {
					Title:         "Parallel task",
					ExecutionMode: "parallel",
					Files:         []string{"pkg/bar.go"},
					ImplStatus:    "pending",
					ReviewStatus:  "pending",
				},
			},
		},
		{
			name: "multi-task document",
			content: `## Task 1: First task
mode: sequential
files:
- a.go

## Task 2: Second task
mode: parallel
files:
- b.go
- c.go
`,
			wantKeys: []string{"1", "2"},
			wantTasks: map[string]state.Task{
				"1": {
					Title:         "First task",
					ExecutionMode: "sequential",
					Files:         []string{"a.go"},
					ImplStatus:    "pending",
					ReviewStatus:  "pending",
				},
				"2": {
					Title:         "Second task",
					ExecutionMode: "parallel",
					Files:         []string{"b.go", "c.go"},
					ImplStatus:    "pending",
					ReviewStatus:  "pending",
				},
			},
		},
		{
			name: "missing mode defaults to sequential",
			content: `## Task 3: No mode field
files:
- x.go
`,
			wantKeys: []string{"3"},
			wantTasks: map[string]state.Task{
				"3": {
					Title:         "No mode field",
					ExecutionMode: "sequential",
					Files:         []string{"x.go"},
					ImplStatus:    "pending",
					ReviewStatus:  "pending",
				},
			},
		},
		{
			name: "empty files list",
			content: `## Task 4: Task with no files
mode: sequential
`,
			wantKeys: []string{"4"},
			wantTasks: map[string]state.Task{
				"4": {
					Title:         "Task with no files",
					ExecutionMode: "sequential",
					Files:         nil,
					ImplStatus:    "pending",
					ReviewStatus:  "pending",
				},
			},
		},
		{
			name: "depends_on parsing",
			content: `## Task 5: Dependent task
mode: sequential
files:
- dep.go
depends_on: [1, 2]
`,
			wantKeys: []string{"5"},
			wantTasks: map[string]state.Task{
				"5": {
					Title:         "Dependent task",
					ExecutionMode: "sequential",
					Files:         []string{"dep.go"},
					DependsOn:     []int{1, 2},
					ImplStatus:    "pending",
					ReviewStatus:  "pending",
				},
			},
		},
		{
			name:    "malformed task — no heading — error",
			content: `This is just some text without any Task heading.`,
			wantErr: true,
		},
		{
			name:    "empty content — error",
			content: "",
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseTasksMd(c.content)

			if c.wantErr {
				if err == nil {
					t.Fatalf("ParseTasksMd(%q): expected error, got nil", c.content)
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseTasksMd(%q): unexpected error: %v", c.content, err)
			}

			// Check that all expected keys are present.
			for _, k := range c.wantKeys {
				gotTask, ok := got[k]
				if !ok {
					t.Errorf("ParseTasksMd: missing key %q in result %v", k, got)
					continue
				}
				wantTask := c.wantTasks[k]
				assertTaskEqual(t, k, wantTask, gotTask)
			}

			// Check that no unexpected keys are present.
			if len(got) != len(c.wantKeys) {
				t.Errorf("ParseTasksMd: result has %d keys, want %d; keys=%v", len(got), len(c.wantKeys), mapKeys(got))
			}
		})
	}
}

// assertTaskEqual compares two state.Task values field by field.
func assertTaskEqual(t *testing.T, taskKey string, want, got state.Task) {
	t.Helper()

	if got.Title != want.Title {
		t.Errorf("task[%s].Title = %q, want %q", taskKey, got.Title, want.Title)
	}
	if got.ExecutionMode != want.ExecutionMode {
		t.Errorf("task[%s].ExecutionMode = %q, want %q", taskKey, got.ExecutionMode, want.ExecutionMode)
	}
	if got.ImplStatus != want.ImplStatus {
		t.Errorf("task[%s].ImplStatus = %q, want %q", taskKey, got.ImplStatus, want.ImplStatus)
	}
	if got.ReviewStatus != want.ReviewStatus {
		t.Errorf("task[%s].ReviewStatus = %q, want %q", taskKey, got.ReviewStatus, want.ReviewStatus)
	}

	// Compare Files slices.
	if len(got.Files) != len(want.Files) {
		t.Errorf("task[%s].Files = %v (len %d), want %v (len %d)", taskKey, got.Files, len(got.Files), want.Files, len(want.Files))
	} else {
		for i, f := range want.Files {
			if got.Files[i] != f {
				t.Errorf("task[%s].Files[%d] = %q, want %q", taskKey, i, got.Files[i], f)
			}
		}
	}

	// Compare DependsOn slices.
	if len(got.DependsOn) != len(want.DependsOn) {
		t.Errorf("task[%s].DependsOn = %v (len %d), want %v (len %d)", taskKey, got.DependsOn, len(got.DependsOn), want.DependsOn, len(want.DependsOn))
	} else {
		for i, d := range want.DependsOn {
			if got.DependsOn[i] != d {
				t.Errorf("task[%s].DependsOn[%d] = %d, want %d", taskKey, i, got.DependsOn[i], d)
			}
		}
	}
}

// mapKeys returns the keys of a map[string]state.Task for diagnostic output.
func mapKeys(m map[string]state.Task) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

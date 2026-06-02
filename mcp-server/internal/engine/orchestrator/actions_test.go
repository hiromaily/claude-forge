package orchestrator

import (
	"reflect"
	"strings"
	"testing"
)

// TestNewHumanGateActionCrossRepo verifies improvement #5: a human gate whose task
// involves an external repository carries cross-repo guidance (PR → CI → preview pin
// and a skill pointer), while a plain in-repo gate stays concise.
func TestNewHumanGateActionCrossRepo(t *testing.T) {
	t.Parallel()

	t.Run("external_repo_task_gets_guidance", func(t *testing.T) {
		t.Parallel()
		a := NewHumanGateAction(PhaseFive, "1", "Merge akupara-proto PR and pin preview")
		for _, want := range []string{"cross-repository", "preview", "update-proto", "CI", "Choose 'done' only after"} {
			if !strings.Contains(a.PresentToUser, want) {
				t.Errorf("cross-repo gate message missing %q; got:\n%s", want, a.PresentToUser)
			}
		}
	})

	t.Run("in_repo_task_stays_concise", func(t *testing.T) {
		t.Parallel()
		a := NewHumanGateAction(PhaseFive, "2", "Rename the local handler function")
		if strings.Contains(a.PresentToUser, "cross-repository") {
			t.Errorf("in-repo gate should not include cross-repo guidance; got:\n%s", a.PresentToUser)
		}
		if !strings.Contains(a.PresentToUser, "requires human action") {
			t.Errorf("in-repo gate missing base message; got:\n%s", a.PresentToUser)
		}
	})
}

func TestSkipSummaryPrefix(t *testing.T) {
	t.Parallel()

	if SkipSummaryPrefix != "skip:" {
		t.Errorf("SkipSummaryPrefix = %q; want %q", SkipSummaryPrefix, "skip:")
	}
}

func TestNewParallelSpawnAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		agent      string
		prompt     string
		model      string
		phase      string
		inputFiles []string
		tasks      []ParallelTask
		want       Action
	}{
		{
			name:       "parallel_two_tasks",
			agent:      "implementer",
			prompt:     "implement tasks",
			model:      "sonnet",
			phase:      "phase-5",
			inputFiles: []string{"design.md", "tasks.md"},
			tasks: []ParallelTask{
				{ID: "1", OutputFile: "impl-1.md"},
				{ID: "2", OutputFile: "impl-2.md"},
			},
			want: Action{
				Type:       ActionSpawnAgent,
				Agent:      "implementer",
				Prompt:     "implement tasks",
				Model:      "sonnet",
				Phase:      "phase-5",
				InputFiles: []string{"design.md", "tasks.md"},
				ParallelTasks: []ParallelTask{
					{ID: "1", OutputFile: "impl-1.md"},
					{ID: "2", OutputFile: "impl-2.md"},
				},
				ParallelTaskIDs: []string{"1", "2"},
				OutputFile:      "",
			},
		},
		{
			name:       "parallel_reviewers_with_per_task_inputs",
			agent:      "impl-reviewer",
			prompt:     "review in parallel",
			model:      "sonnet",
			phase:      "phase-6",
			inputFiles: []string{"tasks.md"},
			tasks: []ParallelTask{
				{ID: "1", InputFiles: []string{"impl-1.md"}, OutputFile: "review-1.md"},
				{ID: "3", InputFiles: []string{"impl-3.md"}, OutputFile: "review-3.md"},
			},
			want: Action{
				Type:       ActionSpawnAgent,
				Agent:      "impl-reviewer",
				Prompt:     "review in parallel",
				Model:      "sonnet",
				Phase:      "phase-6",
				InputFiles: []string{"tasks.md"},
				ParallelTasks: []ParallelTask{
					{ID: "1", InputFiles: []string{"impl-1.md"}, OutputFile: "review-1.md"},
					{ID: "3", InputFiles: []string{"impl-3.md"}, OutputFile: "review-3.md"},
				},
				ParallelTaskIDs: []string{"1", "3"},
				OutputFile:      "",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := NewParallelSpawnAction(tc.agent, tc.prompt, tc.model, tc.phase, tc.inputFiles, tc.tasks)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("NewParallelSpawnAction() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestNewSpawnAgentActionHasNilParallelTaskIDs(t *testing.T) {
	t.Parallel()

	got := NewSpawnAgentAction("implementer", "do the task", "sonnet", "phase-5", []string{"design.md"}, "impl-1.md")
	if got.ParallelTaskIDs != nil {
		t.Errorf("NewSpawnAgentAction().ParallelTaskIDs = %v; want nil", got.ParallelTaskIDs)
	}
}

func TestActionTypeConstants(t *testing.T) {
	t.Parallel()

	if ActionSpawnAgent != "spawn_agent" {
		t.Errorf("ActionSpawnAgent = %q; want %q", ActionSpawnAgent, "spawn_agent")
	}

	if ActionCheckpoint != "checkpoint" {
		t.Errorf("ActionCheckpoint = %q; want %q", ActionCheckpoint, "checkpoint")
	}

	if ActionExec != "exec" {
		t.Errorf("ActionExec = %q; want %q", ActionExec, "exec")
	}

	if ActionWriteFile != "write_file" {
		t.Errorf("ActionWriteFile = %q; want %q", ActionWriteFile, "write_file")
	}

	if ActionDone != "done" {
		t.Errorf("ActionDone = %q; want %q", ActionDone, "done")
	}

	if ActionTaskInit != "task_init" {
		t.Errorf("ActionTaskInit = %q; want %q", ActionTaskInit, "task_init")
	}

	if ActionBatchCommit != "batch_commit" {
		t.Errorf("ActionBatchCommit = %q; want %q", ActionBatchCommit, "batch_commit")
	}
}

func TestNewActions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		build func() Action
		want  Action
	}{
		{
			name: "SpawnAgent",
			build: func() Action {
				return NewSpawnAgentAction("implementer", "do the task", "sonnet", "phase-5", []string{"design.md"}, "impl-1.md")
			},
			want: Action{
				Type:       ActionSpawnAgent,
				Agent:      "implementer",
				Prompt:     "do the task",
				Model:      "sonnet",
				Phase:      "phase-5",
				InputFiles: []string{"design.md"},
				OutputFile: "impl-1.md",
			},
		},
		{
			name: "Checkpoint",
			build: func() Action {
				return NewCheckpointAction("design-review", "Please review the design", []string{"Approve", "Revise"})
			},
			want: Action{
				Type:          ActionCheckpoint,
				Name:          "design-review",
				PresentToUser: "Please review the design",
				Options:       []string{"Approve", "Revise"},
			},
		},
		{
			name: "Exec",
			build: func() Action {
				return NewExecAction("phase-5", []string{"go build ./...", "go test ./..."})
			},
			want: Action{
				Type:     ActionExec,
				Phase:    "phase-5",
				Commands: []string{"go build ./...", "go test ./..."},
			},
		},
		{
			name: "WriteFile",
			build: func() Action {
				return NewWriteFileAction("phase-1", "/workspace/output.md", "# Output\nHello world")
			},
			want: Action{
				Type:    ActionWriteFile,
				Phase:   "phase-1",
				Path:    "/workspace/output.md",
				Content: "# Output\nHello world",
			},
		},
		{
			name: "Done",
			build: func() Action {
				return NewDoneAction("Pipeline complete", "/workspace/summary.md")
			},
			want: Action{
				Type:        ActionDone,
				Summary:     "Pipeline complete",
				SummaryPath: "/workspace/summary.md",
			},
		},
		{
			name: "TaskInit",
			build: func() Action {
				return NewTaskInitAction("phase-5")
			},
			want: Action{
				Type:      ActionTaskInit,
				Phase:     "phase-5",
				SetupOnly: true,
			},
		},
		{
			name: "BatchCommit",
			build: func() Action {
				return NewBatchCommitAction("phase-5")
			},
			want: Action{
				Type:      ActionBatchCommit,
				Phase:     "phase-5",
				SetupOnly: true,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := tc.build()
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

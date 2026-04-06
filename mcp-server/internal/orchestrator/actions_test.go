package orchestrator

import (
	"reflect"
	"testing"
)

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
		taskIDs    []string
		want       Action
	}{
		{
			name:       "parallel_two_tasks",
			agent:      "implementer",
			prompt:     "implement tasks",
			model:      "sonnet",
			phase:      "phase-5",
			inputFiles: []string{"design.md", "tasks.md"},
			taskIDs:    []string{"1", "2"},
			want: Action{
				Type:            ActionSpawnAgent,
				Agent:           "implementer",
				Prompt:          "implement tasks",
				Model:           "sonnet",
				Phase:           "phase-5",
				InputFiles:      []string{"design.md", "tasks.md"},
				ParallelTaskIDs: []string{"1", "2"},
				OutputFile:      "",
			},
		},
		{
			name:       "parallel_three_tasks",
			agent:      "implementer",
			prompt:     "implement all",
			model:      "sonnet",
			phase:      "phase-5",
			inputFiles: []string{"design.md"},
			taskIDs:    []string{"1", "2", "3"},
			want: Action{
				Type:            ActionSpawnAgent,
				Agent:           "implementer",
				Prompt:          "implement all",
				Model:           "sonnet",
				Phase:           "phase-5",
				InputFiles:      []string{"design.md"},
				ParallelTaskIDs: []string{"1", "2", "3"},
				OutputFile:      "",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := NewParallelSpawnAction(tc.agent, tc.prompt, tc.model, tc.phase, tc.inputFiles, tc.taskIDs)
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

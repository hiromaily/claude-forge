package orchestrator

import (
	"testing"
)

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
}

func TestNewSpawnAgentAction(t *testing.T) {
	t.Parallel()

	a := NewSpawnAgentAction("implementer", "do the task", "sonnet", "phase-5", []string{"design.md"}, "impl-1.md")

	if a.Type != ActionSpawnAgent {
		t.Errorf("Type = %q; want %q", a.Type, ActionSpawnAgent)
	}

	if a.Agent != "implementer" {
		t.Errorf("Agent = %q; want %q", a.Agent, "implementer")
	}

	if a.Prompt != "do the task" {
		t.Errorf("Prompt = %q; want %q", a.Prompt, "do the task")
	}

	if a.Model != "sonnet" {
		t.Errorf("Model = %q; want %q", a.Model, "sonnet")
	}

	if a.Phase != "phase-5" {
		t.Errorf("Phase = %q; want %q", a.Phase, "phase-5")
	}

	if len(a.InputFiles) != 1 || a.InputFiles[0] != "design.md" {
		t.Errorf("InputFiles = %v; want [design.md]", a.InputFiles)
	}

	if a.OutputFile != "impl-1.md" {
		t.Errorf("OutputFile = %q; want %q", a.OutputFile, "impl-1.md")
	}

	// cross-variant fields must be zero values
	if a.Commands != nil {
		t.Errorf("Commands should be nil for SpawnAgent action; got %v", a.Commands)
	}

	if a.Path != "" {
		t.Errorf("Path should be empty for SpawnAgent action; got %q", a.Path)
	}

	if a.Name != "" {
		t.Errorf("Name should be empty for SpawnAgent action; got %q", a.Name)
	}

	if a.Summary != "" {
		t.Errorf("Summary should be empty for SpawnAgent action; got %q", a.Summary)
	}
}

func TestNewCheckpointAction(t *testing.T) {
	t.Parallel()

	opts := []string{"Approve", "Revise"}
	a := NewCheckpointAction("design-review", "Please review the design", opts)

	if a.Type != ActionCheckpoint {
		t.Errorf("Type = %q; want %q", a.Type, ActionCheckpoint)
	}

	if a.Name != "design-review" {
		t.Errorf("Name = %q; want %q", a.Name, "design-review")
	}

	if a.PresentToUser != "Please review the design" {
		t.Errorf("PresentToUser = %q; want %q", a.PresentToUser, "Please review the design")
	}

	if len(a.Options) != 2 || a.Options[0] != "Approve" || a.Options[1] != "Revise" {
		t.Errorf("Options = %v; want [Approve Revise]", a.Options)
	}

	// cross-variant fields must be zero values
	if a.Agent != "" {
		t.Errorf("Agent should be empty for Checkpoint action; got %q", a.Agent)
	}

	if a.Commands != nil {
		t.Errorf("Commands should be nil for Checkpoint action; got %v", a.Commands)
	}

	if a.Path != "" {
		t.Errorf("Path should be empty for Checkpoint action; got %q", a.Path)
	}

	if a.Summary != "" {
		t.Errorf("Summary should be empty for Checkpoint action; got %q", a.Summary)
	}
}

func TestNewExecAction(t *testing.T) {
	t.Parallel()

	cmds := []string{"go build ./...", "go test ./..."}
	a := NewExecAction(cmds)

	if a.Type != ActionExec {
		t.Errorf("Type = %q; want %q", a.Type, ActionExec)
	}

	if len(a.Commands) != 2 || a.Commands[0] != "go build ./..." || a.Commands[1] != "go test ./..." {
		t.Errorf("Commands = %v; want %v", a.Commands, cmds)
	}

	// cross-variant fields must be zero values
	if a.Agent != "" {
		t.Errorf("Agent should be empty for Exec action; got %q", a.Agent)
	}

	if a.Path != "" {
		t.Errorf("Path should be empty for Exec action; got %q", a.Path)
	}

	if a.Name != "" {
		t.Errorf("Name should be empty for Exec action; got %q", a.Name)
	}

	if a.Summary != "" {
		t.Errorf("Summary should be empty for Exec action; got %q", a.Summary)
	}
}

func TestNewWriteFileAction(t *testing.T) {
	t.Parallel()

	a := NewWriteFileAction("/workspace/output.md", "# Output\nHello world")

	if a.Type != ActionWriteFile {
		t.Errorf("Type = %q; want %q", a.Type, ActionWriteFile)
	}

	if a.Path != "/workspace/output.md" {
		t.Errorf("Path = %q; want %q", a.Path, "/workspace/output.md")
	}

	if a.Content != "# Output\nHello world" {
		t.Errorf("Content = %q; want %q", a.Content, "# Output\nHello world")
	}

	// cross-variant fields must be zero values
	if a.Agent != "" {
		t.Errorf("Agent should be empty for WriteFile action; got %q", a.Agent)
	}

	if a.Commands != nil {
		t.Errorf("Commands should be nil for WriteFile action; got %v", a.Commands)
	}

	if a.Name != "" {
		t.Errorf("Name should be empty for WriteFile action; got %q", a.Name)
	}

	if a.Summary != "" {
		t.Errorf("Summary should be empty for WriteFile action; got %q", a.Summary)
	}
}

func TestNewDoneAction(t *testing.T) {
	t.Parallel()

	a := NewDoneAction("Pipeline complete", "/workspace/final-summary.md")

	if a.Type != ActionDone {
		t.Errorf("Type = %q; want %q", a.Type, ActionDone)
	}

	if a.Summary != "Pipeline complete" {
		t.Errorf("Summary = %q; want %q", a.Summary, "Pipeline complete")
	}

	if a.SummaryPath != "/workspace/final-summary.md" {
		t.Errorf("SummaryPath = %q; want %q", a.SummaryPath, "/workspace/final-summary.md")
	}

	// cross-variant fields must be zero values
	if a.Agent != "" {
		t.Errorf("Agent should be empty for Done action; got %q", a.Agent)
	}

	if a.Commands != nil {
		t.Errorf("Commands should be nil for Done action; got %v", a.Commands)
	}

	if a.Path != "" {
		t.Errorf("Path should be empty for Done action; got %q", a.Path)
	}

	if a.Name != "" {
		t.Errorf("Name should be empty for Done action; got %q", a.Name)
	}
}

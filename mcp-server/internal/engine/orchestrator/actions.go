package orchestrator

import (
	"fmt"

	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/sourcetype"
)

// Action type constants — match design-mcp-v2.md JSON "type" values.
const (
	ActionSpawnAgent   = "spawn_agent"
	ActionCheckpoint   = "checkpoint"
	ActionExec         = "exec"
	ActionWriteFile    = "write_file"
	ActionDone         = "done"
	ActionTaskInit     = "task_init"     // engine dispatches task_init internally; never surfaced to orchestrator
	ActionBatchCommit  = "batch_commit"  // engine dispatches batch commit internally; never surfaced to orchestrator
	ActionHumanGate    = "human_gate"    // engine dispatches human gate; handler presents to orchestrator as-is
	ActionRenameBranch = "rename_branch" // engine dispatches branch rename when design content suggests a different type
	ActionPushBranch   = "push_branch"   // engine dispatches branch push before pr-creation; absorbed internally by pipeline_next_action
)

// SkipSummaryPrefix is the prefix placed in Action.Summary for per-phase skip signals.
// The engine emits ActionDone{Summary: SkipSummaryPrefix + phaseID} when a phase
// should be skipped; PipelineNextActionHandler detects this prefix, calls
// StateManager.PhaseCompleteSkipped(workspace, phaseID), and loops to compute the next action —
// so the orchestrator never sees a skip signal directly.
//
// Callers outside the handler that need to distinguish a skip from true pipeline
// completion can check:
//
//	strings.HasPrefix(action.Summary, orchestrator.SkipSummaryPrefix)
const SkipSummaryPrefix = "skip:"

// Action is the discriminated union returned by the engine's next-action step.
// The Type field selects which optional fields are populated.
type Action struct {
	Type string `json:"type"`

	// spawn_agent fields
	Agent           string   `json:"agent,omitempty"`
	Prompt          string   `json:"prompt,omitempty"`
	Model           string   `json:"model,omitempty"`
	Phase           string   `json:"phase,omitempty"`
	InputFiles      []string `json:"input_files,omitempty"`
	OutputFile      string   `json:"output_file,omitempty"`
	ParallelTaskIDs []string `json:"parallel_task_ids,omitempty"` // non-nil iff this is a parallel fanout

	// checkpoint fields
	Name          string   `json:"name,omitempty"`
	PresentToUser string   `json:"present_to_user,omitempty"`
	Options       []string `json:"options,omitempty"`

	// exec fields
	Commands []string `json:"commands,omitempty"`

	// write_file fields
	Path    string `json:"path,omitempty"`
	Content string `json:"content,omitempty"`

	// rename_branch fields
	OldBranch string `json:"old_branch,omitempty"`
	NewBranch string `json:"new_branch,omitempty"`

	// setup flag — when true, pipeline_report_result records phase-log but skips PhaseComplete
	SetupOnly bool `json:"setup_only,omitempty"`

	// done fields
	Summary     string `json:"summary,omitempty"`
	SummaryPath string `json:"summary_path,omitempty"`

	// post-to-source metadata — populated only for post-to-source checkpoints
	PostMethod *sourcetype.PostConfig `json:"post_method,omitempty"`
}

// NewSpawnAgentAction constructs an Action of type ActionSpawnAgent.
func NewSpawnAgentAction(agent, prompt, model, phase string, inputFiles []string, outputFile string) Action {
	return Action{
		Type:       ActionSpawnAgent,
		Agent:      agent,
		Prompt:     prompt,
		Model:      model,
		Phase:      phase,
		InputFiles: inputFiles,
		OutputFile: outputFile,
	}
}

// NewParallelSpawnAction constructs an ActionSpawnAgent that signals parallel fanout.
// taskIDs must be sorted numerically ascending by the caller (engine uses sortedTaskKeys).
// Prompt contains the shared prompt for all parallel tasks.
func NewParallelSpawnAction(agent, prompt, model, phase string, inputFiles []string, taskIDs []string) Action {
	return Action{
		Type:            ActionSpawnAgent,
		Agent:           agent,
		Prompt:          prompt,
		Model:           model,
		Phase:           phase,
		InputFiles:      inputFiles,
		ParallelTaskIDs: taskIDs,
	}
}

// NewCheckpointAction constructs an Action of type ActionCheckpoint.
func NewCheckpointAction(name, presentToUser string, options []string) Action {
	return Action{
		Type:          ActionCheckpoint,
		Name:          name,
		PresentToUser: presentToUser,
		Options:       options,
	}
}

// NewExecAction constructs an Action of type ActionExec.
// phase must be the pipeline phase ID under which this exec is issued (e.g., "pr-creation").
func NewExecAction(phase string, commands []string) Action {
	return Action{
		Type:     ActionExec,
		Phase:    phase,
		Commands: commands,
	}
}

// NewWriteFileAction constructs an Action of type ActionWriteFile.
// phase must be the pipeline phase ID under which this write is issued (e.g., "phase-1").
func NewWriteFileAction(phase, path, content string) Action {
	return Action{
		Type:    ActionWriteFile,
		Phase:   phase,
		Path:    path,
		Content: content,
	}
}

// NewSetupExecAction constructs a setup exec action that should NOT advance the phase.
// The orchestrator must pass setup_only=true when calling pipeline_report_result.
func NewSetupExecAction(phase string, commands []string) Action {
	return Action{
		Type:      ActionExec,
		Phase:     phase,
		Commands:  commands,
		SetupOnly: true,
	}
}

// NewDoneAction constructs an Action of type ActionDone.
func NewDoneAction(summary, summaryPath string) Action {
	return Action{
		Type:        ActionDone,
		Summary:     summary,
		SummaryPath: summaryPath,
	}
}

// NewTaskInitAction constructs an Action of type ActionTaskInit.
// SetupOnly is true so pipeline_report_result records phase-log but skips PhaseComplete.
func NewTaskInitAction(phase string) Action {
	return Action{
		Type:      ActionTaskInit,
		Phase:     phase,
		SetupOnly: true,
	}
}

// NewBatchCommitAction constructs an Action of type ActionBatchCommit.
// SetupOnly is true so pipeline_report_result records phase-log but skips PhaseComplete.
func NewBatchCommitAction(phase string) Action {
	return Action{
		Type:      ActionBatchCommit,
		Phase:     phase,
		SetupOnly: true,
	}
}

// NewRenameBranchAction constructs an Action of type ActionRenameBranch.
// SetupOnly is true so pipeline_report_result records phase-log but skips PhaseComplete.
func NewRenameBranchAction(phase, oldBranch, newBranch string) Action {
	return Action{
		Type:      ActionRenameBranch,
		Phase:     phase,
		OldBranch: oldBranch,
		NewBranch: newBranch,
		SetupOnly: true,
	}
}

// NewPushBranchAction constructs an Action of type ActionPushBranch.
// It is absorbed internally by pipeline_next_action (never surfaced to the orchestrator),
// so pipeline_report_result is never called for this action type; SetupOnly is set for
// consistency with other internally-absorbed action constructors but has no effect.
func NewPushBranchAction(phase string) Action {
	return Action{
		Type:      ActionPushBranch,
		Phase:     phase,
		SetupOnly: true,
	}
}

// NewHumanGateAction constructs an Action of type ActionHumanGate.
// TaskKey is the numeric task key (e.g. "3") that requires human action.
// The pipeline_next_action handler converts this to a checkpoint-like response
// for the orchestrator and stores the task key in PendingHumanGate.
func NewHumanGateAction(phase, taskKey, title string) Action {
	return Action{
		Type:          ActionHumanGate,
		Phase:         phase,
		Name:          taskKey,
		PresentToUser: fmt.Sprintf("Task %s requires human action: %s\n\nComplete the action and choose 'done' to continue, or 'skip' to mark it without action.", taskKey, title),
		Options:       []string{"done", "skip", "abandon"},
	}
}

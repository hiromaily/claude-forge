package orchestrator

// Action type constants — match design-mcp-v2.md JSON "type" values.
const (
	ActionSpawnAgent = "spawn_agent"
	ActionCheckpoint = "checkpoint"
	ActionExec       = "exec"
	ActionWriteFile  = "write_file"
	ActionDone       = "done"
)

// SkipSummaryPrefix is the prefix placed in Action.Summary for per-phase skip signals.
// Callers distinguish a skip from true pipeline completion by checking:
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

	// done fields
	Summary     string `json:"summary,omitempty"`
	SummaryPath string `json:"summary_path,omitempty"`
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
func NewExecAction(commands []string) Action {
	return Action{
		Type:     ActionExec,
		Commands: commands,
	}
}

// NewWriteFileAction constructs an Action of type ActionWriteFile.
func NewWriteFileAction(path, content string) Action {
	return Action{
		Type:    ActionWriteFile,
		Path:    path,
		Content: content,
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

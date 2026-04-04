// Package state defines the State data model for the forge-state MCP server.
// The Go struct field json tags must remain in 1:1 correspondence with the
// state.json schema managed by the Go MCP server state package.
package state

// ValidPhases enumerates all legal phase identifiers.
var ValidPhases = []string{
	PhaseSetup, PhaseOne, PhaseTwo, PhaseThree, PhaseThreeB,
	PhaseCheckpointA, PhaseFour, PhaseFourB, PhaseCheckpointB,
	PhaseFive, PhaseSix, PhaseSeven, PhaseFinalVerification,
	PhasePRCreation, PhaseFinalSummary, PhasePostToSource, PhaseFinalCommit, PhaseCompleted,
}

// ValidEfforts enumerates legal effort labels.
var ValidEfforts = []string{EffortS, EffortM, EffortL}

// ValidTemplates enumerates legal flow template names.
var ValidTemplates = []string{TemplateLight, TemplateStandard, TemplateFull}

// ValidRevTypes enumerates legal revision type identifiers.
var ValidRevTypes = []string{RevTypeDesign, RevTypeTasks}

// State mirrors the top-level state.json object written by the Go MCP server state package.
type State struct {
	Version                   int             `json:"version"`
	MCPVersion                string          `json:"forge-state-mcp-version,omitempty"`
	SpecName                  string          `json:"specName"`
	Workspace                 string          `json:"workspace"`
	Branch                    *string         `json:"branch"`
	Effort                    *string         `json:"effort"`
	FlowTemplate              *string         `json:"flowTemplate"`
	AutoApprove               bool            `json:"autoApprove"`
	SkipPr                    bool            `json:"skipPr"`
	UseCurrentBranch          bool            `json:"useCurrentBranch"`
	Debug                     bool            `json:"debug"`
	SkippedPhases             []string        `json:"skippedPhases"`
	CurrentPhase              string          `json:"currentPhase"`
	CurrentPhaseStatus        string          `json:"currentPhaseStatus"`
	CompletedPhases           []string        `json:"completedPhases"`
	Revisions                 Revisions       `json:"revisions"`
	CheckpointRevisionPending map[string]bool `json:"checkpointRevisionPending"`
	NeedsBatchCommit          bool            `json:"needsBatchCommit"`
	Tasks                     map[string]Task `json:"tasks"`
	PhaseLog                  []PhaseLogEntry `json:"phaseLog"`
	Timestamps                Timestamps      `json:"timestamps"`
	Error                     *PhaseError     `json:"error"`
}

// Revisions holds counters for design/task review revision cycles.
type Revisions struct {
	DesignRevisions       int `json:"designRevisions"`
	TaskRevisions         int `json:"taskRevisions"`
	DesignInlineRevisions int `json:"designInlineRevisions"`
	TaskInlineRevisions   int `json:"taskInlineRevisions"`
}

// Task represents a single implementation task entry inside state.Tasks.
// ImplRetries and ReviewRetries are JSON numbers (int), not strings.
type Task struct {
	Title         string   `json:"title"`
	ExecutionMode string   `json:"executionMode"`
	DependsOn     []int    `json:"depends_on"`
	Files         []string `json:"files"`
	ImplStatus    string   `json:"implStatus"`
	ReviewStatus  string   `json:"reviewStatus"`
	ImplRetries   int      `json:"implRetries"`
	ReviewRetries int      `json:"reviewRetries"`
}

// PhaseLogEntry records token/duration metrics for a completed phase.
// DurationMs maps to the "duration_ms" JSON key to match shell-script output.
type PhaseLogEntry struct {
	Phase      string `json:"phase"`
	Tokens     int    `json:"tokens"`
	DurationMs int    `json:"duration_ms"`
	Model      string `json:"model"`
	Timestamp  string `json:"timestamp"`
}

// Timestamps holds ISO-8601/RFC-3339 wall-clock timestamps for the pipeline.
type Timestamps struct {
	Created      string  `json:"created"`
	LastUpdated  string  `json:"lastUpdated"`
	PhaseStarted *string `json:"phaseStarted"`
}

// PhaseError captures error details when a phase fails.
type PhaseError struct {
	Phase     string `json:"phase"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

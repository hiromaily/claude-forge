// Package state defines centralized constants for the forge-state pipeline.
// All phase identifiers, status values, task fields, artifact filenames, and
// other magic strings are defined here to prevent typo-induced bugs and to
// make rename operations safe (change once, compile-check everywhere).
package state

// ---------- Phase identifiers ----------

const (
	PhaseSetup             = "setup"
	PhaseOne               = "phase-1"
	PhaseTwo               = "phase-2"
	PhaseThree             = "phase-3"
	PhaseThreeB            = "phase-3b"
	PhaseCheckpointA       = "checkpoint-a"
	PhaseFour              = "phase-4"
	PhaseFourB             = "phase-4b"
	PhaseCheckpointB       = "checkpoint-b"
	PhaseFive              = "phase-5"
	PhaseSix               = "phase-6"
	PhaseSeven             = "phase-7"
	PhaseFinalVerification = "final-verification"
	PhasePRCreation        = "pr-creation"
	PhaseFinalSummary      = "final-summary"
	PhaseFinalCommit       = "final-commit"
	PhasePostToSource      = "post-to-source"
	PhaseCompleted         = "completed"
)

// ---------- Phase status values ----------

const (
	StatusPending       = "pending"
	StatusInProgress    = "in_progress"
	StatusCompleted     = "completed"
	StatusFailed        = "failed"
	StatusAwaitingHuman = "awaiting_human"
	StatusAbandoned     = "abandoned"
)

// ---------- Task status values ----------

const (
	TaskStatusCompleted         = "completed"
	TaskStatusCompletedPass     = "completed_pass"
	TaskStatusCompletedFail     = "completed_fail"
	TaskStatusCompletedPassNote = "completed_pass_with_notes"
)

// ---------- Task field names ----------

const (
	TaskFieldImplStatus    = "implStatus"
	TaskFieldReviewStatus  = "reviewStatus"
	TaskFieldExecutionMode = "executionMode"
	TaskFieldTitle         = "title"
	TaskFieldImplRetries   = "implRetries"
	TaskFieldReviewRetries = "reviewRetries"
)

// ---------- Execution modes ----------

const (
	ExecModeParallel   = "parallel"
	ExecModeSequential = "sequential"
	ExecModeHumanGate  = "human_gate"
)

// ---------- Revision types ----------

const (
	RevTypeDesign = "design"
	RevTypeTasks  = "tasks"
)

// ---------- Effort labels ----------

const (
	EffortS = "S"
	EffortM = "M"
	EffortL = "L"
)

// ---------- Flow templates ----------

const (
	TemplateLight    = "light"
	TemplateStandard = "standard"
	TemplateFull     = "full"
)

// ---------- Artifact filenames ----------

const (
	ArtifactRequest             = "request.md"
	ArtifactAnalysis            = "analysis.md"
	ArtifactInvestigation       = "investigation.md"
	ArtifactDesign              = "design.md"
	ArtifactReviewDesign        = "review-design.md"
	ArtifactTasks               = "tasks.md"
	ArtifactReviewTasks         = "review-tasks.md"
	ArtifactComprehensiveReview = "comprehensive-review.md"
	ArtifactSummary             = "summary.md"
	ArtifactFinalVerification   = "final-verification.md"
)

// ---------- Default model ----------

const (
	DefaultModel = "sonnet"
)

// ---------- Retry limits ----------

const (
	MaxRevisionRetries = 2
)

// ---------- Verdict values ----------
// These are string constants (not the orchestrator.Verdict type) so that
// packages that do not import orchestrator can still reference them for
// content matching (e.g. validation/artifact.go).

const (
	VerdictApprove          = "APPROVE"
	VerdictApproveWithNotes = "APPROVE_WITH_NOTES"
	VerdictRevise           = "REVISE"
	VerdictPass             = "PASS"
	VerdictPassWithNotes    = "PASS_WITH_NOTES"
	VerdictFail             = "FAIL"
	VerdictUnknown          = "UNKNOWN"
)

// ---------- Severity markers ----------

const (
	SeverityCritical    = "CRITICAL"
	SeverityMinor       = "MINOR"
	SeverityCriticalTag = "[CRITICAL]"
	SeverityMinorTag    = "[MINOR]"
)

// ---------- File management ----------

const (
	StateFileName = "state.json"
	TempSuffix    = ".tmp"
	FilePermRW    = 0o600
)

// ---------- Source types ----------

const (
	SourceTypeGitHub = "github_issue"
	SourceTypeJira   = "jira_issue"
	SourceTypeText   = "text"
)

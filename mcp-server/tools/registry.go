// Package tools registers all 32 MCP tool handlers with the MCP server.
// Tool names use underscores (hyphens from state-manager.sh commands are converted).
package tools

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hiromaily/claude-forge/mcp-server/events"
	"github.com/hiromaily/claude-forge/mcp-server/state"
)

// RegisterAll registers all 32 tool handlers with srv, delegating to sm.
// bus receives published events from the five state-mutation handlers.
// slack sends Slack webhook notifications for phase-complete, phase-fail, and abandon.
// eventsPort is the port the SSE HTTP server is listening on (from FORGE_EVENTS_PORT).
// This is the single entry point called from main.go.
func RegisterAll(srv *server.MCPServer, sm *state.StateManager, bus *events.EventBus, slack *events.SlackNotifier, eventsPort string) {
	srv.AddTool(
		mcp.NewTool("init",
			mcp.WithDescription("Initialise a new pipeline workspace (state.json). Requires validated=true after validate-input.sh succeeds."),
			mcp.WithString("workspace", mcp.Required(), mcp.Description("Absolute path to the workspace directory")),
			mcp.WithString("spec_name", mcp.Required(), mcp.Description("Spec/project identifier")),
			mcp.WithBoolean("validated", mcp.Required(), mcp.Description("Must be true — pass only after validate-input.sh exits 0")),
		),
		InitHandler(sm),
	)

	srv.AddTool(
		mcp.NewTool("get",
			mcp.WithDescription("Read a single field from state.json. Supports top-level and dot-notation sub-fields."),
			mcp.WithString("workspace", mcp.Required(), mcp.Description("Absolute path to the workspace directory")),
			mcp.WithString("field", mcp.Required(), mcp.Description("Field name, e.g. specName, currentPhase, revisions.designRevisions")),
		),
		GetHandler(sm),
	)

	srv.AddTool(
		mcp.NewTool("phase_start",
			mcp.WithDescription("Mark a phase as in_progress. Blocked when tasks are empty for phase-5."),
			mcp.WithString("workspace", mcp.Required(), mcp.Description("Absolute path to the workspace directory")),
			mcp.WithString("phase", mcp.Required(), mcp.Description("Phase identifier, e.g. phase-1, phase-2")),
		),
		PhaseStartHandler(sm, bus),
	)

	srv.AddTool(
		mcp.NewTool("phase_complete",
			mcp.WithDescription("Mark a phase as completed and advance to the next phase. Enforces artifact, checkpoint, and revision-pending guards."),
			mcp.WithString("workspace", mcp.Required(), mcp.Description("Absolute path to the workspace directory")),
			mcp.WithString("phase", mcp.Required(), mcp.Description("Phase identifier")),
		),
		PhaseCompleteHandler(sm, bus, slack),
	)

	srv.AddTool(
		mcp.NewTool("phase_fail",
			mcp.WithDescription("Record a phase failure with an error message."),
			mcp.WithString("workspace", mcp.Required(), mcp.Description("Absolute path to the workspace directory")),
			mcp.WithString("phase", mcp.Required(), mcp.Description("Phase identifier")),
			mcp.WithString("message", mcp.Description("Human-readable failure reason")),
		),
		PhaseFailHandler(sm, bus, slack),
	)

	srv.AddTool(
		mcp.NewTool("checkpoint",
			mcp.WithDescription("Register a human-review pause by setting currentPhaseStatus to awaiting_human."),
			mcp.WithString("workspace", mcp.Required(), mcp.Description("Absolute path to the workspace directory")),
			mcp.WithString("phase", mcp.Required(), mcp.Description("checkpoint-a or checkpoint-b")),
		),
		CheckpointHandler(sm, bus),
	)

	srv.AddTool(
		mcp.NewTool("task_init",
			mcp.WithDescription("Populate state.Tasks with the task map. Requires checkpoint-b to be completed or skipped."),
			mcp.WithString("workspace", mcp.Required(), mcp.Description("Absolute path to the workspace directory")),
			mcp.WithObject("tasks", mcp.Required(), mcp.Description("Map of task number → task object")),
		),
		TaskInitHandler(sm),
	)

	srv.AddTool(
		mcp.NewTool("task_update",
			mcp.WithDescription("Update a single field in a task entry. Enforces review-file guard when setting reviewStatus=completed_pass."),
			mcp.WithString("workspace", mcp.Required(), mcp.Description("Absolute path to the workspace directory")),
			mcp.WithString("task_num", mcp.Required(), mcp.Description("Task number key, e.g. 1, 2")),
			mcp.WithString("field", mcp.Required(), mcp.Description("Field to update: implStatus, reviewStatus, implRetries, reviewRetries")),
			mcp.WithString("value", mcp.Required(), mcp.Description("New value for the field")),
		),
		TaskUpdateHandler(sm),
	)

	srv.AddTool(
		mcp.NewTool("revision_bump",
			mcp.WithDescription("Increment the design or tasks revision counter."),
			mcp.WithString("workspace", mcp.Required(), mcp.Description("Absolute path to the workspace directory")),
			mcp.WithString("rev_type", mcp.Required(), mcp.Description("design or tasks")),
		),
		RevisionBumpHandler(sm),
	)

	srv.AddTool(
		mcp.NewTool("inline_revision_bump",
			mcp.WithDescription("Increment the design or tasks inline revision counter."),
			mcp.WithString("workspace", mcp.Required(), mcp.Description("Absolute path to the workspace directory")),
			mcp.WithString("rev_type", mcp.Required(), mcp.Description("design or tasks")),
		),
		InlineRevisionBumpHandler(sm),
	)

	srv.AddTool(
		mcp.NewTool("set_branch",
			mcp.WithDescription("Set the branch field in state.json."),
			mcp.WithString("workspace", mcp.Required(), mcp.Description("Absolute path to the workspace directory")),
			mcp.WithString("branch", mcp.Required(), mcp.Description("Git branch name")),
		),
		SetBranchHandler(sm),
	)

	srv.AddTool(
		mcp.NewTool("set_task_type",
			mcp.WithDescription("Set the taskType field in state.json."),
			mcp.WithString("workspace", mcp.Required(), mcp.Description("Absolute path to the workspace directory")),
			mcp.WithString("task_type", mcp.Required(), mcp.Description("Task type identifier, e.g. feature, bugfix")),
		),
		SetTaskTypeHandler(sm),
	)

	srv.AddTool(
		mcp.NewTool("set_effort",
			mcp.WithDescription("Set the effort field. Valid values: XS, S, M, L."),
			mcp.WithString("workspace", mcp.Required(), mcp.Description("Absolute path to the workspace directory")),
			mcp.WithString("effort", mcp.Required(), mcp.Description("XS, S, M, or L")),
		),
		SetEffortHandler(sm),
	)

	srv.AddTool(
		mcp.NewTool("set_flow_template",
			mcp.WithDescription("Set the flowTemplate field. Valid values: direct, lite, light, standard, full."),
			mcp.WithString("workspace", mcp.Required(), mcp.Description("Absolute path to the workspace directory")),
			mcp.WithString("flow_template", mcp.Required(), mcp.Description("direct, lite, light, standard, or full")),
		),
		SetFlowTemplateHandler(sm),
	)

	srv.AddTool(
		mcp.NewTool("set_auto_approve",
			mcp.WithDescription("Set autoApprove = true in state.json."),
			mcp.WithString("workspace", mcp.Required(), mcp.Description("Absolute path to the workspace directory")),
		),
		SetAutoApproveHandler(sm),
	)

	srv.AddTool(
		mcp.NewTool("set_skip_pr",
			mcp.WithDescription("Set skipPr = true in state.json."),
			mcp.WithString("workspace", mcp.Required(), mcp.Description("Absolute path to the workspace directory")),
		),
		SetSkipPrHandler(sm),
	)

	srv.AddTool(
		mcp.NewTool("set_debug",
			mcp.WithDescription("Set debug = true in state.json."),
			mcp.WithString("workspace", mcp.Required(), mcp.Description("Absolute path to the workspace directory")),
		),
		SetDebugHandler(sm),
	)

	srv.AddTool(
		mcp.NewTool("set_use_current_branch",
			mcp.WithDescription("Set useCurrentBranch = true and branch = the provided value."),
			mcp.WithString("workspace", mcp.Required(), mcp.Description("Absolute path to the workspace directory")),
			mcp.WithString("branch", mcp.Required(), mcp.Description("Existing git branch name")),
		),
		SetUseCurrentBranchHandler(sm),
	)

	srv.AddTool(
		mcp.NewTool("set_revision_pending",
			mcp.WithDescription("Set checkpointRevisionPending[checkpoint] = true to indicate a user-requested revision."),
			mcp.WithString("workspace", mcp.Required(), mcp.Description("Absolute path to the workspace directory")),
			mcp.WithString("checkpoint", mcp.Required(), mcp.Description("checkpoint-a or checkpoint-b")),
		),
		SetRevisionPendingHandler(sm),
	)

	srv.AddTool(
		mcp.NewTool("clear_revision_pending",
			mcp.WithDescription("Clear checkpointRevisionPending[checkpoint] after the user approves the revision."),
			mcp.WithString("workspace", mcp.Required(), mcp.Description("Absolute path to the workspace directory")),
			mcp.WithString("checkpoint", mcp.Required(), mcp.Description("checkpoint-a or checkpoint-b")),
		),
		ClearRevisionPendingHandler(sm),
	)

	srv.AddTool(
		mcp.NewTool("skip_phase",
			mcp.WithDescription("Add a phase to skippedPhases and advance currentPhase."),
			mcp.WithString("workspace", mcp.Required(), mcp.Description("Absolute path to the workspace directory")),
			mcp.WithString("phase", mcp.Required(), mcp.Description("Phase identifier to skip")),
		),
		SkipPhaseHandler(sm),
	)

	srv.AddTool(
		mcp.NewTool("phase_log",
			mcp.WithDescription("Append a token/duration metrics entry to phaseLog. Issues a warning when a duplicate entry for the same phase already exists."),
			mcp.WithString("workspace", mcp.Required(), mcp.Description("Absolute path to the workspace directory")),
			mcp.WithString("phase", mcp.Required(), mcp.Description("Phase identifier")),
			mcp.WithNumber("tokens", mcp.Required(), mcp.Description("Token count for this phase")),
			mcp.WithNumber("duration_ms", mcp.Required(), mcp.Description("Wall-clock duration in milliseconds")),
			mcp.WithString("model", mcp.Required(), mcp.Description("Model identifier, e.g. sonnet")),
		),
		PhaseLogHandler(sm),
	)

	srv.AddTool(
		mcp.NewTool("phase_stats",
			mcp.WithDescription("Return aggregated token and duration statistics from phaseLog."),
			mcp.WithString("workspace", mcp.Required(), mcp.Description("Absolute path to the workspace directory")),
		),
		PhaseStatsHandler(sm),
	)

	srv.AddTool(
		mcp.NewTool("abandon",
			mcp.WithDescription("Mark the pipeline as abandoned."),
			mcp.WithString("workspace", mcp.Required(), mcp.Description("Absolute path to the workspace directory")),
		),
		AbandonHandler(sm, bus, slack),
	)

	srv.AddTool(
		mcp.NewTool("resume_info",
			mcp.WithDescription("Return a structured summary of pipeline state for orchestrator resume logic."),
			mcp.WithString("workspace", mcp.Required(), mcp.Description("Absolute path to the workspace directory")),
		),
		ResumeInfoHandler(sm),
	)

	srv.AddTool(
		mcp.NewTool("refresh_index",
			mcp.WithDescription("Execute build-specs-index.sh via os/exec to rebuild .specs/index.json. Returns an error if the script exits non-zero."),
			mcp.WithString("workspace", mcp.Required(), mcp.Description("Absolute path to the workspace directory")),
		),
		RefreshIndexHandler(sm),
	)

	srv.AddTool(
		mcp.NewTool("search_patterns",
			mcp.WithDescription("Score .specs/index.json entries against request.md using BM25. Returns ranked markdown output (review-feedback or impl patterns)."),
			mcp.WithString("workspace", mcp.Required(), mcp.Description("Absolute path to the workspace directory")),
			mcp.WithString("task_type", mcp.Description("Task type used for BM25 taskType boost, e.g. feature, bugfix")),
			mcp.WithNumber("top_k", mcp.Description("Maximum number of results (0 = mode-specific default: 3 for review-feedback, 2 for impl)")),
			mcp.WithString("mode", mcp.Description("Output mode: \"impl\" for implementation patterns; any other value means review-feedback mode")),
		),
		SearchPatternsHandler(sm),
	)

	srv.AddTool(
		mcp.NewTool("subscribe_events",
			mcp.WithDescription("Return the SSE endpoint URL for real-time phase transition events. "+
				"Returns {\"endpoint\":\"http://localhost:<port>/events\"} when FORGE_EVENTS_PORT is set, "+
				"or an informational message when SSE is not configured."),
		),
		SubscribeEventsHandler(eventsPort),
	)

	srv.AddTool(
		mcp.NewTool("ast_summary",
			mcp.WithDescription("Parse a source file with tree-sitter and return a compact markdown summary of exported function "+
				"signatures, type definitions, and constants. Supports Go, TypeScript, Python, and Bash."),
			mcp.WithString("file_path", mcp.Required(), mcp.Description("Absolute path to the source file to summarize")),
			mcp.WithString("language", mcp.Description("Language override: go, typescript, python, bash. When omitted, language is auto-detected from the file extension.")),
		),
		AstSummaryHandler(),
	)

	srv.AddTool(
		mcp.NewTool("ast_find_definition",
			mcp.WithDescription("Search a source file for a named symbol declaration using tree-sitter AST parsing. Returns the definition text; multiple matches are prefixed with a count header."),
			mcp.WithString("file_path", mcp.Required(), mcp.Description("Absolute path to the source file to search")),
			mcp.WithString("symbol", mcp.Required(), mcp.Description("Symbol name to look up (function name, type name, etc.)")),
			mcp.WithString("language", mcp.Description("Language override: go, typescript, python, bash. When omitted, language is auto-detected from the file extension.")),
		),
		AstFindDefinitionHandler(),
	)

	depGraphTool, depGraphHandler := AstDependencyGraphHandler()
	srv.AddTool(depGraphTool, depGraphHandler)

	srv.AddTool(
		mcp.NewTool("impact_scope",
			mcp.WithDescription("Identify files that call a given symbol via a two-pass import+call-site scan. Returns a ranked list of affected files with BFS distance (distance=-1 for TypeScript/Python)."),
			mcp.WithString("root_path", mcp.Required(), mcp.Description("Absolute path to the root directory of the source tree")),
			mcp.WithString("file_path", mcp.Required(), mcp.Description("Absolute path to the file containing the changed symbol")),
			mcp.WithString("symbol_name", mcp.Required(), mcp.Description("Function, type, or constant name to search for callers of")),
			mcp.WithString("language", mcp.Required(), mcp.Description("Language: go, typescript, python, or bash")),
		),
		AstImpactScopeHandler(),
	)
}

// pipeline_init_with_context MCP handler.
// Implements the three-call confirmation flow.
// First call (neither user_confirmation nor discussion_answers present): detects effort and returns
//
//	needs_user_confirmation (or needs_discussion when --discuss is active, source is text, and not --auto).
//
// Discussion call (discussion_answers non-empty, user_confirmation absent): enriches the task body,
//
//	returns needs_user_confirmation with enriched_request_body set. No filesystem writes.
//
// Confirmation call (user_confirmation present, discussion_answers absent): validates, initialises
//
//	workspace, writes state.json + request.md.
//
// DISCRIMINATOR ORDER: discussion_answers is checked BEFORE user_confirmation to prevent shadowing.

package tools

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/orchestrator"
	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/sourcetype"
	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/state"
	"github.com/hiromaily/claude-forge/mcp-server/pkg/events"
	"github.com/hiromaily/claude-forge/mcp-server/pkg/maputil"
)

// DiscussionPrompt is returned by handleFirstCall when --discuss is active
// and source type is "text". The orchestrator presents Questions to the user,
// collects answers, and calls back with discussion_answers set.
type DiscussionPrompt struct {
	Questions []string `json:"questions"`
	Message   string   `json:"message"`
}

// PipelineInitWithContextResult is the response shape for pipeline_init_with_context.
type PipelineInitWithContextResult struct {
	Ready                 bool                    `json:"ready,omitempty"`
	Workspace             string                  `json:"workspace,omitempty"`
	Effort                string                  `json:"effort,omitempty"`
	FlowTemplate          string                  `json:"flow_template,omitempty"`
	SkippedPhases         []string                `json:"skipped_phases,omitempty"`
	RequestMDContent      string                  `json:"request_md_content,omitempty"`
	Branch                string                  `json:"branch,omitempty"`
	CreateBranch          bool                    `json:"create_branch,omitempty"`
	Warning               string                  `json:"warning,omitempty"`
	NeedsUserConfirmation *UserConfirmationPrompt `json:"needs_user_confirmation,omitempty"`
	NeedsDiscussion       *DiscussionPrompt       `json:"needs_discussion,omitempty"`
}

// EffortOption describes a single effort level with its skip list and recommended flag.
type EffortOption struct {
	SkippedPhases []orchestrator.SkipLabel `json:"skipped_phases"`
	Recommended   bool                     `json:"recommended"`
}

// UserConfirmationPrompt holds the detected values to present to the user.
type UserConfirmationPrompt struct {
	DetectedEffort      string                  `json:"detected_effort"`
	EffortOptions       map[string]EffortOption `json:"effort_options"`
	CurrentBranch       string                  `json:"current_branch"`
	IsMainBranch        bool                    `json:"is_main_branch"`
	Message             string                  `json:"message"`
	EnrichedRequestBody string                  `json:"enriched_request_body,omitempty"`
}

// pipelineFlags holds parsed flag fields from the flags parameter.
type pipelineFlags struct {
	Auto           bool
	SkipPR         bool
	Debug          bool
	Discuss        bool
	EffortOverride string
	CurrentBranch  string
}

// userConfirmation holds confirmed effort, branch decision, and optional workspace slug from the second call.
type userConfirmation struct {
	Effort              string
	WorkspaceSlug       string // optional LLM-generated ASCII slug; overrides the auto-derived slug
	UseCurrentBranch    bool   // true = stay on current branch; false = create new branch from slug
	EnrichedRequestBody string // carries enriched body from discussion call; "" means use defaults
}

// PipelineInitWithContextHandler handles the "pipeline_init_with_context" MCP tool.
// First call (neither user_confirmation nor discussion_answers present): detects effort and returns
//
//	needs_user_confirmation (or needs_discussion when --discuss+text+non-auto).
//
// Discussion call (discussion_answers non-empty, user_confirmation absent): builds enriched body,
//
//	returns needs_user_confirmation with enriched_request_body. No filesystem writes.
//
// Confirmation call (user_confirmation present, discussion_answers absent): finalizes workspace —
//
//	creates directory, initialises state, applies all setters, skips phases, writes request.md.
//
// DISCRIMINATOR ORDER: discussion_answers checked BEFORE user_confirmation to prevent shadowing.
func PipelineInitWithContextHandler(sm *state.StateManager, bus *events.EventBus) server.ToolHandlerFunc { //nolint:gocyclo // complexity is inherent in the dispatch table
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace, err := req.RequireString("workspace")
		if err != nil {
			return errorf("%v", err)
		}

		args := req.GetArguments()

		// Derive source type from source_url for handler lookup.
		// Check top-level source_url first, then source_url inside external_context.
		sourceURL := maputil.StringField(args, "source_url")
		if sourceURL == "" {
			if ec, ok := args["external_context"].(map[string]any); ok {
				sourceURL = maputil.StringField(ec, "source_url")
			}
		}
		detectedSourceType := ""
		if sourceURL != "" {
			detectedSourceType, _ = sourcetype.ClassifyURL(sourceURL)
		}
		// Fallback: detect source type from external_context field prefixes
		// when no source_url is provided (backward compatibility).
		if detectedSourceType == "" {
			detectedSourceType = detectSourceTypeFromFields(args)
		}

		// Parse external_context object.
		extCtx, err := parseExternalContext(args, detectedSourceType)
		if err != nil {
			return errorf("parse external_context: %v", err)
		}

		// source_id and source_url are returned by pipeline_init but aren't fetched
		// fields, so the orchestrator passes them as top-level parameters rather than
		// embedding them inside external_context alongside the fetched GitHub/Jira fields.
		if topSourceID := maputil.StringField(args, "source_id"); topSourceID != "" && extCtx.SourceID == "" {
			extCtx.SourceID = topSourceID
		}
		if topSourceURL := maputil.StringField(args, "source_url"); topSourceURL != "" && extCtx.SourceURL == "" {
			extCtx.SourceURL = topSourceURL
		}

		// task_text carries the original task text for text source type pipelines.
		// It enables --discuss enrichment and fixes the pre-existing gap where request.md
		// had an empty body for text source pipelines.
		taskText := req.GetString("task_text", "")
		extCtx.TaskText = taskText

		// Parse flags object.
		flags, err := parseFlags(args)
		if err != nil {
			return errorf("parse flags: %v", err)
		}

		// Three-call discriminator.
		// IMPORTANT: discussion_answers must be checked BEFORE user_confirmation so the
		// discussion path cannot be shadowed by the existing confirmation branch.
		discussionAnswers := req.GetString("discussion_answers", "")
		ucRaw, hasConfirmation := args["user_confirmation"]

		// Guard: both fields present is ambiguous — return an error.
		if discussionAnswers != "" && hasConfirmation && ucRaw != nil {
			return errorf("discussion_answers and user_confirmation must not both be present")
		}

		switch {
		case discussionAnswers != "":
			// Discussion call: build enriched body, return needs_user_confirmation.
			return handleDiscussionCall(workspace, extCtx, flags, discussionAnswers)
		case hasConfirmation && ucRaw != nil:
			// Confirmation call: validate effort, initialise workspace, write files.
			uc, err := parseUserConfirmation(ucRaw)
			if err != nil {
				return errorf("parse user_confirmation: %v", err)
			}
			return handleSecondCall(sm, bus, workspace, extCtx, flags, uc)
		default:
			// First call: detect effort, return needs_user_confirmation (or needs_discussion).
			return handleFirstCall(workspace, extCtx, flags)
		}
	}
}

// ---------- first call ----------

func handleFirstCall(workspace string, extCtx externalContext, flags pipelineFlags) (*mcp.CallToolResult, error) {
	if flags.Discuss && !flags.Auto && extCtx.IsTextSource() {
		result := PipelineInitWithContextResult{
			NeedsDiscussion: &DiscussionPrompt{
				Questions: defaultDiscussionQuestions(),
				Message:   "Please answer the following questions to help the pipeline understand your intent.",
			},
		}
		_ = workspace // no I/O on first call
		return okJSON(result)
	}

	// Standard path: detect effort and return needs_user_confirmation.
	return buildUserConfirmationPrompt(workspace, extCtx, flags, "")
}

// buildUserConfirmationPrompt constructs a needs_user_confirmation response.
// enrichedBody is set when called from handleDiscussionCall; "" otherwise.
func buildUserConfirmationPrompt(workspace string, extCtx externalContext, flags pipelineFlags, enrichedBody string) (*mcp.CallToolResult, error) {
	// Detect effort.
	combinedText := strings.TrimSpace(extCtx.Fields.CombinedText() + " " + extCtx.TaskText)
	effort := orchestrator.DetectEffort(flags.EffortOverride, extCtx.Fields.StoryPoints, combinedText)

	// Build EffortOptions for all three valid efforts with human-readable labels.
	// The detected effort is marked as recommended so the orchestrator renders it deterministically.
	effortOptions := map[string]EffortOption{
		"S": {SkippedPhases: orchestrator.SkipsWithLabelsForEffort("S"), Recommended: effort == "S"},
		"M": {SkippedPhases: orchestrator.SkipsWithLabelsForEffort("M"), Recommended: effort == "M"},
		"L": {SkippedPhases: orchestrator.SkipsWithLabelsForEffort("L"), Recommended: effort == "L"},
	}

	// Determine branch state for the user confirmation prompt.
	currentBranch := flags.CurrentBranch
	isMain := currentBranch == "" || currentBranch == "main" || currentBranch == "master"

	// Must be non-empty: the orchestrator echoes this back in user_confirmation.enriched_request_body
	// so initWorkspace can write a non-empty request.md (task_text is not re-sent on the second call).
	echoBody := enrichedBody
	if echoBody == "" {
		echoBody = extCtx.TaskText
	}

	nuc := &UserConfirmationPrompt{
		DetectedEffort:      effort,
		EffortOptions:       effortOptions,
		CurrentBranch:       currentBranch,
		IsMainBranch:        isMain,
		EnrichedRequestBody: echoBody,
		Message: fmt.Sprintf(
			"Detected effort=%q. Current branch=%q (is_main=%v). "+
				"Please confirm by calling pipeline_init_with_context again with "+
				"user_confirmation={effort:..., use_current_branch:...}. "+
				"Available effort options: S (light flow), M (standard flow), L (full flow).",
			effort, currentBranch, isMain,
		),
	}

	result := PipelineInitWithContextResult{
		NeedsUserConfirmation: nuc,
	}
	_ = workspace // workspace echoed only; no I/O on first call
	return okJSON(result)
}

// ---------- discussion call ----------

// handleDiscussionCall handles the second call when discussion_answers is present.
// It builds the enriched request body and returns needs_user_confirmation with
// EnrichedRequestBody set. No filesystem writes are performed.
func handleDiscussionCall(
	workspace string,
	extCtx externalContext,
	flags pipelineFlags,
	discussionAnswers string,
) (*mcp.CallToolResult, error) {
	enrichedBody := buildEnrichedRequestBody(extCtx.TaskText, discussionAnswers)
	return buildUserConfirmationPrompt(workspace, extCtx, flags, enrichedBody)
}

// ---------- second call ----------

func handleSecondCall(
	sm *state.StateManager,
	bus *events.EventBus,
	workspace string,
	extCtx externalContext,
	flags pipelineFlags,
	uc userConfirmation,
) (*mcp.CallToolResult, error) {
	// Validate effort.
	if !slices.Contains(state.ValidEfforts, uc.Effort) {
		return errorf("invalid effort %q: must be one of %v", uc.Effort, state.ValidEfforts)
	}

	// Derive flow template and skip phases directly from effort.
	flowTemplate := orchestrator.EffortToTemplate(uc.Effort)
	skippedPhases := orchestrator.SkipsForEffort(uc.Effort)

	// Derive specName and optionally rename workspace for better readability.
	// Priority order: external context (Jira/GitHub) > LLM-provided slug > auto-derived slug.
	// When LLM slug is provided alongside a source_id (GitHub issue number or Jira key),
	// prepend the source_id to the slug so it appears in the branch name and PR title.
	workspace = refineWorkspacePath(workspace, extCtx)
	if uc.WorkspaceSlug != "" {
		slug := uc.WorkspaceSlug
		if extCtx.SourceID != "" {
			slug = extCtx.SourceID + "-" + slug
		}
		workspace = applyWorkspaceSlug(workspace, slug)
	}
	specName := deriveSpecName(workspace)

	// Derive branch name before initWorkspace so it can be set in Configure
	// (single state.json write instead of Configure + SetBranch).
	var branchName string
	createBranch := false
	if uc.UseCurrentBranch {
		branchName = flags.CurrentBranch
	} else {
		st := &state.State{SpecName: specName}
		branchName = orchestrator.DeriveBranchName(st)
		createBranch = true
	}

	// Create directory, initialise state, write request.md.
	requestMD, err := initWorkspace(sm, workspace, specName, flags, uc, branchName, flowTemplate, skippedPhases, extCtx, uc.EnrichedRequestBody)
	if err != nil {
		return errorf("%v", err)
	}

	publishEventWithDetail(bus, nil, "pipeline-init", "setup", specName, workspace, "in_progress", "effort="+uc.Effort)

	return okJSON(PipelineInitWithContextResult{
		Ready:            true,
		Workspace:        workspace,
		Effort:           uc.Effort,
		FlowTemplate:     flowTemplate,
		SkippedPhases:    skippedPhases,
		RequestMDContent: requestMD,
		Branch:           branchName,
		CreateBranch:     createBranch,
	})
}

// ---------- flag and confirmation parsers ----------

// parseFlags extracts flag fields from the args map.
func parseFlags(args map[string]any) (pipelineFlags, error) {
	var flags pipelineFlags

	raw, ok := args["flags"]
	if !ok || raw == nil {
		return flags, nil
	}

	m, err := maputil.ToMap(raw)
	if err != nil {
		return flags, fmt.Errorf("parse flags: %w", err)
	}

	flags.Auto = maputil.BoolField(m, "auto")
	flags.SkipPR = maputil.BoolField(m, "skip_pr")
	flags.Debug = maputil.BoolField(m, "debug")
	flags.Discuss = maputil.BoolField(m, "discuss")
	flags.EffortOverride = maputil.StringField(m, "effort_override")
	flags.CurrentBranch = maputil.StringField(m, "current_branch")

	return flags, nil
}

// parseUserConfirmation extracts effort and optional workspace_slug from user_confirmation raw value.
func parseUserConfirmation(raw any) (userConfirmation, error) {
	var uc userConfirmation

	m, err := maputil.ToMap(raw)
	if err != nil {
		return uc, fmt.Errorf("parse user_confirmation: %w", err)
	}

	uc.Effort = maputil.StringField(m, "effort")
	uc.WorkspaceSlug = maputil.StringField(m, "workspace_slug")
	uc.UseCurrentBranch = maputil.BoolField(m, "use_current_branch")
	uc.EnrichedRequestBody = maputil.StringField(m, "enriched_request_body")
	return uc, nil
}

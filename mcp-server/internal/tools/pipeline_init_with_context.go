// Package tools — pipeline_init_with_context MCP handler.
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
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hiromaily/claude-forge/mcp-server/internal/orchestrator"
	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
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

// UserConfirmationPrompt holds the detected values to present to the user.
type UserConfirmationPrompt struct {
	DetectedEffort      string                              `json:"detected_effort"`
	EffortOptions       map[string][]orchestrator.SkipLabel `json:"effort_options"`
	CurrentBranch       string                              `json:"current_branch"`
	IsMainBranch        bool                                `json:"is_main_branch"`
	Message             string                              `json:"message"`
	EnrichedRequestBody string                              `json:"enriched_request_body,omitempty"`
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
func PipelineInitWithContextHandler(sm *state.StateManager) server.ToolHandlerFunc {
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace, err := req.RequireString("workspace")
		if err != nil {
			return errorf("%v", err)
		}

		args := req.GetArguments()

		// Parse external_context object.
		extCtx, err := parseExternalContext(args)
		if err != nil {
			return errorf("parse external_context: %v", err)
		}

		// source_id and source_url are returned by pipeline_init but aren't fetched
		// fields, so the orchestrator passes them as top-level parameters rather than
		// embedding them inside external_context alongside the fetched GitHub/Jira fields.
		if topSourceID := stringField(args, "source_id"); topSourceID != "" && extCtx.SourceID == "" {
			extCtx.SourceID = topSourceID
		}
		if topSourceURL := stringField(args, "source_url"); topSourceURL != "" && extCtx.SourceURL == "" {
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
			return handleSecondCall(sm, workspace, extCtx, flags, uc)
		default:
			// First call: detect effort, return needs_user_confirmation (or needs_discussion).
			return handleFirstCall(workspace, extCtx, flags)
		}
	}
}

// ---------- first call ----------

func handleFirstCall(workspace string, extCtx externalContext, flags pipelineFlags) (*mcp.CallToolResult, error) {
	isTextSource := extCtx.GitHubTitle == "" && extCtx.GitHubBody == "" &&
		extCtx.JiraIssueType == "" && extCtx.JiraSummary == "" && extCtx.JiraDescription == ""
	if flags.Discuss && !flags.Auto && isTextSource {
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
	combinedText := strings.TrimSpace(extCtx.GitHubTitle + " " + extCtx.GitHubBody + " " +
		extCtx.JiraSummary + " " + extCtx.JiraDescription + " " + extCtx.TaskText)
	effort := orchestrator.DetectEffort(flags.EffortOverride, extCtx.JiraStoryPoints, combinedText)

	// Build EffortOptions for all three valid efforts with human-readable labels.
	effortOptions := map[string][]orchestrator.SkipLabel{
		"S": orchestrator.SkipsWithLabelsForEffort("S"),
		"M": orchestrator.SkipsWithLabelsForEffort("M"),
		"L": orchestrator.SkipsWithLabelsForEffort("L"),
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
	workspace = refineWorkspacePath(workspace, extCtx)
	if uc.WorkspaceSlug != "" {
		workspace = applyWorkspaceSlug(workspace, uc.WorkspaceSlug)
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

	data, err := json.Marshal(raw)
	if err != nil {
		return flags, fmt.Errorf("marshal flags: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return flags, fmt.Errorf("unmarshal flags: %w", err)
	}

	flags.Auto = boolField(m, "auto")
	flags.SkipPR = boolField(m, "skip_pr")
	flags.Debug = boolField(m, "debug")
	flags.Discuss = boolField(m, "discuss")
	flags.EffortOverride = stringField(m, "effort_override")
	flags.CurrentBranch = stringField(m, "current_branch")

	return flags, nil
}

// parseUserConfirmation extracts effort and optional workspace_slug from user_confirmation raw value.
func parseUserConfirmation(raw any) (userConfirmation, error) {
	var uc userConfirmation

	data, err := json.Marshal(raw)
	if err != nil {
		return uc, fmt.Errorf("marshal user_confirmation: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return uc, fmt.Errorf("unmarshal user_confirmation: %w", err)
	}

	uc.Effort = stringField(m, "effort")
	uc.WorkspaceSlug = stringField(m, "workspace_slug")
	uc.UseCurrentBranch = boolField(m, "use_current_branch")
	uc.EnrichedRequestBody = stringField(m, "enriched_request_body")
	return uc, nil
}

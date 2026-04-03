// Package tools — pipeline_init_with_context MCP handler.
// Implements the two-call confirmation flow.
// First call (user_confirmation absent): detects effort and returns needs_user_confirmation.
// Second call (user_confirmation present): validates, initialises workspace, writes state.json + request.md.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hiromaily/claude-forge/mcp-server/internal/orchestrator"
	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
)

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
}

// UserConfirmationPrompt holds the detected values to present to the user.
type UserConfirmationPrompt struct {
	DetectedEffort string                              `json:"detected_effort"`
	EffortOptions  map[string][]orchestrator.SkipLabel `json:"effort_options"`
	CurrentBranch  string                              `json:"current_branch"`
	IsMainBranch   bool                                `json:"is_main_branch"`
	Message        string                              `json:"message"`
}

// externalContext holds parsed GitHub/Jira context fields.
type externalContext struct {
	// Source identifiers from pipeline_init result — used in request.md front matter.
	SourceURL string
	SourceID  string
	// GitHub fields
	GitHubLabels []string
	GitHubTitle  string
	GitHubBody   string
	// Jira fields
	JiraIssueType   string
	JiraStoryPoints int
	JiraSummary     string
	JiraDescription string
}

// pipelineFlags holds parsed flag fields from the flags parameter.
type pipelineFlags struct {
	Auto           bool
	SkipPR         bool
	Debug          bool
	EffortOverride string
	CurrentBranch  string
}

// userConfirmation holds confirmed effort, branch decision, and optional workspace slug from the second call.
type userConfirmation struct {
	Effort           string
	WorkspaceSlug    string // optional LLM-generated ASCII slug; overrides the auto-derived slug
	UseCurrentBranch bool   // true = stay on current branch; false = create new branch from slug
}

// PipelineInitWithContextHandler handles the "pipeline_init_with_context" MCP tool.
// First call (user_confirmation absent): detects effort and returns needs_user_confirmation.
// Second call (user_confirmation present): finalizes workspace — creates directory, initialises
// state, applies all setters, skips phases, writes request.md.
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

		// Parse flags object.
		flags, err := parseFlags(args)
		if err != nil {
			return errorf("parse flags: %v", err)
		}

		// Check if user_confirmation is present.
		ucRaw, hasConfirmation := args["user_confirmation"]
		if !hasConfirmation || ucRaw == nil {
			return handleFirstCall(workspace, extCtx, flags)
		}

		// Second call: extract user_confirmation.
		uc, err := parseUserConfirmation(ucRaw)
		if err != nil {
			return errorf("parse user_confirmation: %v", err)
		}

		return handleSecondCall(sm, workspace, extCtx, flags, uc)
	}
}

// ---------- first call ----------

func handleFirstCall(workspace string, extCtx externalContext, flags pipelineFlags) (*mcp.CallToolResult, error) {
	// Detect effort.
	combinedText := strings.TrimSpace(extCtx.GitHubTitle + " " + extCtx.GitHubBody + " " +
		extCtx.JiraSummary + " " + extCtx.JiraDescription)
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

	nuc := &UserConfirmationPrompt{
		DetectedEffort: effort,
		EffortOptions:  effortOptions,
		CurrentBranch:  currentBranch,
		IsMainBranch:   isMain,
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

	// Create directory, initialise state, write request.md.
	requestMD, err := initWorkspace(sm, workspace, specName, flags, uc, flowTemplate, skippedPhases, extCtx)
	if err != nil {
		return errorf("%v", err)
	}

	// Derive branch name and determine if branch creation is needed.
	result := PipelineInitWithContextResult{
		Ready:            true,
		Workspace:        workspace,
		Effort:           uc.Effort,
		FlowTemplate:     flowTemplate,
		SkippedPhases:    skippedPhases,
		RequestMDContent: requestMD,
	}

	if uc.UseCurrentBranch {
		// User chose to stay on current branch — no creation needed.
		result.Branch = flags.CurrentBranch
	} else {
		// Derive branch name from spec name (deterministic).
		st := &state.State{SpecName: specName}
		branchName := orchestrator.DeriveBranchName(st)
		result.Branch = branchName
		result.CreateBranch = true

		// Record branch in state so Phase 5 doesn't try to create it again.
		if setErr := sm.SetBranch(workspace, branchName); setErr != nil {
			result.Warning = fmt.Sprintf("set_branch: %v", setErr)
		}
	}

	return okJSON(result)
}

// initWorkspace executes the 8-step I/O sequence for the second call.
// It creates the workspace directory, initialises state, applies all configuration in
// a single write via sm.Configure, and writes request.md.
// Returns the request.md content on success.
func initWorkspace(
	sm *state.StateManager,
	workspace, specName string,
	flags pipelineFlags,
	uc userConfirmation,
	flowTemplate string,
	skippedPhases []string,
	extCtx externalContext,
) (string, error) {
	// Validate workspace path — reject non-ASCII characters so that
	// multibyte input (e.g. Japanese) never produces an unreadable directory name.
	if hasNonASCII(workspace) {
		return "", fmt.Errorf("workspace path %q contains non-ASCII characters; use only ASCII in directory names", workspace)
	}

	// Create workspace directory.
	if err := os.MkdirAll(workspace, 0o750); err != nil {
		return "", fmt.Errorf("MkdirAll %q: %w", workspace, err)
	}

	// Check state.json doesn't already exist.
	stateFile := filepath.Join(workspace, "state.json")
	if _, err := os.Stat(stateFile); err == nil {
		return "", fmt.Errorf("workspace %q already initialised: state.json exists", workspace)
	}

	// sm.Init.
	if err := sm.Init(workspace, specName); err != nil {
		return "", fmt.Errorf("sm.Init: %w", err)
	}

	// Apply all configuration in a single write to state.json.
	cfg := state.PipelineConfig{
		Effort:           uc.Effort,
		FlowTemplate:     flowTemplate,
		AutoApprove:      flags.Auto,
		SkipPR:           flags.SkipPR,
		Debug:            flags.Debug,
		SkippedPhases:    skippedPhases,
		UseCurrentBranch: uc.UseCurrentBranch,
	}
	if uc.UseCurrentBranch && flags.CurrentBranch != "" {
		cfg.Branch = flags.CurrentBranch
	}
	if err := sm.Configure(workspace, cfg); err != nil {
		return "", fmt.Errorf("configure: %w", err)
	}

	// Write request.md.
	requestMD := buildRequestMD(extCtx)
	reqPath := filepath.Join(workspace, "request.md")
	if err := os.WriteFile(reqPath, []byte(requestMD), 0o600); err != nil {
		return "", fmt.Errorf("write request.md: %w", err)
	}

	return requestMD, nil
}

// ---------- helpers ----------

// parseExternalContext extracts GitHub/Jira context fields from the args map.
func parseExternalContext(args map[string]any) (externalContext, error) {
	var extCtx externalContext

	raw, ok := args["external_context"]
	if !ok || raw == nil {
		return extCtx, nil
	}

	// Round-trip through JSON to normalize types.
	data, err := json.Marshal(raw)
	if err != nil {
		return extCtx, fmt.Errorf("marshal external_context: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return extCtx, fmt.Errorf("unmarshal external_context: %w", err)
	}

	extCtx.SourceURL = stringField(m, "source_url")
	extCtx.SourceID = stringField(m, "source_id")
	extCtx.GitHubTitle = stringField(m, "github_title")
	extCtx.GitHubBody = stringField(m, "github_body")
	extCtx.JiraIssueType = stringFieldAlt(m, "jira_issue_type", "issue_type")
	extCtx.JiraSummary = stringFieldAlt(m, "jira_summary", "summary")
	extCtx.JiraDescription = stringFieldAlt(m, "jira_description", "description")

	// Parse github_labels (array of strings).
	if labelsRaw, ok := m["github_labels"]; ok {
		switch v := labelsRaw.(type) {
		case []any:
			for _, l := range v {
				if s, ok := l.(string); ok {
					extCtx.GitHubLabels = append(extCtx.GitHubLabels, s)
				}
			}
		case []string:
			extCtx.GitHubLabels = v
		}
	}

	// Parse jira_story_points (number), with "story_points" as fallback alias.
	if _, ok := m["jira_story_points"]; !ok {
		if sp, ok2 := m["story_points"]; ok2 {
			m["jira_story_points"] = sp
		}
	}
	if spRaw, ok := m["jira_story_points"]; ok {
		switch v := spRaw.(type) {
		case float64:
			extCtx.JiraStoryPoints = int(v)
		case int:
			extCtx.JiraStoryPoints = v
		case json.Number:
			n, _ := v.Int64()
			extCtx.JiraStoryPoints = int(n)
		}
	}

	return extCtx, nil
}

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
	return uc, nil
}

// buildRequestMD constructs the request.md content.
func buildRequestMD(extCtx externalContext) string {
	var sb strings.Builder

	// Determine source_type and body.
	sourceType := "text"
	var body string

	if extCtx.GitHubTitle != "" || extCtx.GitHubBody != "" {
		sourceType = "github_issue"
		body = strings.TrimSpace(extCtx.GitHubTitle + "\n\n" + extCtx.GitHubBody)
	} else if extCtx.JiraIssueType != "" || extCtx.JiraSummary != "" || extCtx.JiraDescription != "" {
		sourceType = "jira_issue"
		body = strings.TrimSpace(extCtx.JiraSummary + "\n\n" + extCtx.JiraDescription)
	}

	sb.WriteString("---\n")
	sb.WriteString("source_type: ")
	sb.WriteString(sourceType)
	sb.WriteString("\n")
	if extCtx.SourceURL != "" {
		sb.WriteString("source_url: ")
		sb.WriteString(extCtx.SourceURL)
		sb.WriteString("\n")
	}
	if extCtx.SourceID != "" {
		sb.WriteString("source_id: ")
		sb.WriteString(extCtx.SourceID)
		sb.WriteString("\n")
	}
	sb.WriteString("---\n")

	if body != "" {
		sb.WriteString("\n")
		sb.WriteString(body)
		sb.WriteString("\n")
	}

	return sb.String()
}

// ---------- field extraction helpers ----------

func stringField(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

// stringFieldAlt tries the primary key first, then falls back to the alt key.
// This allows callers to pass either "jira_summary" or "summary" as the field name.
func stringFieldAlt(m map[string]any, primary, alt string) string {
	if s := stringField(m, primary); s != "" {
		return s
	}
	return stringField(m, alt)
}

func boolField(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok || v == nil {
		return false
	}
	b, _ := v.(bool)
	return b
}

// applyWorkspaceSlug replaces the slug portion of a workspace path with the
// LLM-generated slug. If slugify produces an empty result (e.g. pure Japanese input),
// the original workspace path is returned unchanged.
func applyWorkspaceSlug(workspace, rawSlug string) string {
	cleaned := slugify(rawSlug)
	if cleaned == "" {
		return workspace
	}
	return replaceWorkspaceSlug(workspace, cleaned)
}

// hasNonASCII guards workspace paths against unreadable multibyte characters (e.g. Japanese).
func hasNonASCII(s string) bool {
	for _, r := range s {
		if r > 0x7F {
			return true
		}
	}
	return false
}

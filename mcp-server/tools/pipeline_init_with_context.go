// Package tools — pipeline_init_with_context MCP handler.
// Implements the two-call confirmation flow for decisions 6–13.
// First call (user_confirmation absent): runs decisions and returns needs_user_confirmation.
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

	"github.com/hiromaily/claude-forge/mcp-server/orchestrator"
	"github.com/hiromaily/claude-forge/mcp-server/state"
)

// PipelineInitWithContextResult is the response shape for pipeline_init_with_context.
type PipelineInitWithContextResult struct {
	Ready                 bool                    `json:"ready,omitempty"`
	Workspace             string                  `json:"workspace,omitempty"`
	TaskType              string                  `json:"task_type,omitempty"`
	Effort                string                  `json:"effort,omitempty"`
	FlowTemplate          string                  `json:"flow_template,omitempty"`
	SkippedPhases         []string                `json:"skipped_phases,omitempty"`
	SynthesizeStubs       bool                    `json:"synthesize_stubs,omitempty"`
	RequestMDContent      string                  `json:"request_md_content,omitempty"`
	Warning               string                  `json:"warning,omitempty"`
	NeedsUserConfirmation *UserConfirmationPrompt `json:"needs_user_confirmation,omitempty"`
}

// UserConfirmationPrompt holds the detected values to present to the user.
type UserConfirmationPrompt struct {
	DetectedTaskType string   `json:"detected_task_type"`
	DetectedEffort   string   `json:"detected_effort"`
	FlowTemplate     string   `json:"flow_template"`
	SkippedPhases    []string `json:"skipped_phases"`
	Message          string   `json:"message"`
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
	TypeOverride   string
	EffortOverride string
	CurrentBranch  string
}

// userConfirmation holds confirmed task_type and effort from the second call.
type userConfirmation struct {
	TaskType string
	Effort   string
}

// PipelineInitWithContextHandler handles the "pipeline_init_with_context" MCP tool.
// First call (user_confirmation absent): runs decisions 6–13 and returns needs_user_confirmation.
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
	taskType, effort, flowTemplate, skippedPhases, warning := runDecisions(extCtx, flags)

	nuc := &UserConfirmationPrompt{
		DetectedTaskType: taskType,
		DetectedEffort:   effort,
		FlowTemplate:     flowTemplate,
		SkippedPhases:    skippedPhases,
		Message: fmt.Sprintf(
			"Detected task_type=%q, effort=%q, flow_template=%q. "+
				"Please confirm or override these values by calling pipeline_init_with_context "+
				"again with user_confirmation={task_type:..., effort:...}.",
			taskType, effort, flowTemplate,
		),
	}

	result := PipelineInitWithContextResult{
		NeedsUserConfirmation: nuc,
	}
	if warning != "" {
		result.Warning = warning
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
	// Step 1: Validate task_type.
	if !slices.Contains(orchestrator.ValidTaskTypes, uc.TaskType) {
		return errorf("invalid task_type %q: must be one of %v", uc.TaskType, orchestrator.ValidTaskTypes)
	}

	// Step 2: Validate effort.
	if !slices.Contains(state.ValidEfforts, uc.Effort) {
		return errorf("invalid effort %q: must be one of %v", uc.Effort, state.ValidEfforts)
	}

	// Steps 3–5: Re-derive flow template and skip phases; apply decision 12.
	flowTemplate, skippedPhases, warning := deriveFlowDecisions(uc.TaskType, uc.Effort, flags.Auto)

	// Step 6: Derive specName.
	specName := deriveSpecName(workspace)

	// Steps 7a–7l: Create directory, initialise state, write request.md.
	requestMD, err := initWorkspace(sm, workspace, specName, flags, uc, flowTemplate, skippedPhases, extCtx)
	if err != nil {
		return errorf("%v", err)
	}

	synthesizeStubs := orchestrator.ShouldSynthesizeStubs(flowTemplate)

	return okJSON(PipelineInitWithContextResult{
		Ready:            true,
		Workspace:        workspace,
		TaskType:         uc.TaskType,
		Effort:           uc.Effort,
		FlowTemplate:     flowTemplate,
		SkippedPhases:    skippedPhases,
		SynthesizeStubs:  synthesizeStubs,
		RequestMDContent: requestMD,
		Warning:          warning,
	})
}

// ---------- decisions 6–13 ----------

// runDecisions runs decisions 6–13 and returns taskType, effort, flowTemplate, skippedPhases, warning.
func runDecisions(extCtx externalContext, flags pipelineFlags) (taskType, effort, flowTemplate string, skippedPhases []string, warning string) {
	// Decision 6–8: detect task type (single call handles all precedences).
	combinedText := strings.TrimSpace(extCtx.GitHubTitle + " " + extCtx.GitHubBody + " " +
		extCtx.JiraSummary + " " + extCtx.JiraDescription)
	taskType = orchestrator.DetectTaskType(
		flags.TypeOverride,
		extCtx.JiraIssueType,
		extCtx.GitHubLabels,
		combinedText,
	)

	// Decision 9: detect effort.
	effort = orchestrator.DetectEffort(flags.EffortOverride, extCtx.JiraStoryPoints, combinedText)

	// Decisions 10–12: flow template, skip sequence, auto conflict.
	flowTemplate, skippedPhases, warning = deriveFlowDecisions(taskType, effort, flags.Auto)

	// Decision 13: stub synthesis (reflected in flowTemplate; result conveyed in response).
	return taskType, effort, flowTemplate, skippedPhases, warning
}

// deriveFlowDecisions derives flowTemplate, skippedPhases, and warning for decisions 10–12.
// Decision 12: if flowTemplate is "full" and autoFlag=true, downgrade to "standard".
func deriveFlowDecisions(taskType, effort string, autoFlag bool) (flowTemplate string, skippedPhases []string, warning string) {
	flowTemplate = orchestrator.DeriveFlowTemplate(taskType, effort)
	skippedPhases = orchestrator.SkipsForCell(taskType, effort)
	if flowTemplate == orchestrator.TemplateFull && autoFlag {
		warning = fmt.Sprintf(
			"flow_template %q conflicts with auto=true; downgrading to %q",
			orchestrator.TemplateFull, orchestrator.TemplateStandard,
		)
		flowTemplate = orchestrator.TemplateStandard
		skippedPhases = orchestrator.SkipsForTemplate(orchestrator.TemplateStandard)
	}
	return flowTemplate, skippedPhases, warning
}

// initWorkspace executes the 8-step I/O sequence for the second call (steps 7a–7l).
// It creates the workspace directory, initialises state, applies all setters, skips phases,
// and writes request.md. Returns the request.md content on success.
//
//nolint:gocyclo // complexity is inherent in the 8-step I/O sequence with flag branches
func initWorkspace(
	sm *state.StateManager,
	workspace, specName string,
	flags pipelineFlags,
	uc userConfirmation,
	flowTemplate string,
	skippedPhases []string,
	extCtx externalContext,
) (string, error) {
	// Step 7a: Create workspace directory.
	if err := os.MkdirAll(workspace, 0o750); err != nil {
		return "", fmt.Errorf("MkdirAll %q: %w", workspace, err)
	}

	// Step 7b: Check state.json doesn't already exist.
	stateFile := filepath.Join(workspace, "state.json")
	if _, err := os.Stat(stateFile); err == nil {
		return "", fmt.Errorf("workspace %q already initialised: state.json exists", workspace)
	}

	// Step 7c: sm.Init.
	if err := sm.Init(workspace, specName); err != nil {
		return "", fmt.Errorf("sm.Init: %w", err)
	}

	// Step 7d: SetTaskType.
	if err := sm.SetTaskType(workspace, uc.TaskType); err != nil {
		return "", fmt.Errorf("SetTaskType: %w", err)
	}

	// Step 7e: SetEffort.
	if err := sm.SetEffort(workspace, uc.Effort); err != nil {
		return "", fmt.Errorf("SetEffort: %w", err)
	}

	// Step 7f: SetFlowTemplate.
	if err := sm.SetFlowTemplate(workspace, flowTemplate); err != nil {
		return "", fmt.Errorf("SetFlowTemplate: %w", err)
	}

	// Step 7g: auto flag.
	if flags.Auto {
		if err := sm.SetAutoApprove(workspace); err != nil {
			return "", fmt.Errorf("SetAutoApprove: %w", err)
		}
	}

	// Step 7h: skip_pr flag.
	if flags.SkipPR {
		if err := sm.SetSkipPr(workspace); err != nil {
			return "", fmt.Errorf("SetSkipPr: %w", err)
		}
	}

	// Step 7i: debug flag.
	if flags.Debug {
		if err := sm.SetDebug(workspace); err != nil {
			return "", fmt.Errorf("SetDebug: %w", err)
		}
	}

	// Step 7j: current_branch.
	if flags.CurrentBranch != "" && flags.CurrentBranch != "main" && flags.CurrentBranch != "master" {
		if err := sm.SetUseCurrentBranch(workspace, flags.CurrentBranch); err != nil {
			return "", fmt.Errorf("SetUseCurrentBranch: %w", err)
		}
	}

	// Step 7k: skip phases in order.
	for _, phase := range skippedPhases {
		if err := sm.SkipPhase(workspace, phase); err != nil {
			return "", fmt.Errorf("SkipPhase %q: %w", phase, err)
		}
	}

	// Step 7l: Write request.md.
	requestMD := buildRequestMD(extCtx, uc.TaskType)
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
	extCtx.JiraIssueType = stringField(m, "jira_issue_type")
	extCtx.JiraSummary = stringField(m, "jira_summary")
	extCtx.JiraDescription = stringField(m, "jira_description")

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

	// Parse jira_story_points (number).
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
	flags.TypeOverride = stringField(m, "type_override")
	flags.EffortOverride = stringField(m, "effort_override")
	flags.CurrentBranch = stringField(m, "current_branch")

	return flags, nil
}

// parseUserConfirmation extracts task_type and effort from user_confirmation raw value.
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

	uc.TaskType = stringField(m, "task_type")
	uc.Effort = stringField(m, "effort")
	return uc, nil
}

// buildRequestMD constructs the request.md content.
func buildRequestMD(extCtx externalContext, taskType string) string {
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
	sb.WriteString("task_type: ")
	sb.WriteString(taskType)
	sb.WriteString("\n")
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

func boolField(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok || v == nil {
		return false
	}
	b, _ := v.(bool)
	return b
}

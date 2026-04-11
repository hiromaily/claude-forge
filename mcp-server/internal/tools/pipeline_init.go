// Package tools — pipeline_init MCP handler.
// PipelineInitHandler is a pure detection tool: it parses the raw arguments string
// and returns structured data about the pipeline to initialize. It has no side effects
// on StateManager — no sm.Init or setter calls are made.
package tools

import (
	"context"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
	"github.com/hiromaily/claude-forge/mcp-server/internal/validation"
)

// ResumeMode describes how a pipeline resume was triggered.
// It is absent ("") for new pipelines.
type ResumeMode = string

const (
	// ResumeModeNone is the zero value; indicates a new pipeline (not a resume).
	ResumeModeNone ResumeMode = ""
	// ResumeModeAuto is set when the input matches an existing spec directory
	// in .specs/. The orchestrator proceeds directly without confirmation.
	ResumeModeAuto ResumeMode = "auto"
)

// PipelineInitResult is the structured result returned by PipelineInitHandler.
// On resume path: ResumeMode is "auto", Workspace and Instruction are set.
//
//	The orchestrator proceeds directly without confirmation.
//
// On new pipeline path: ResumeMode is absent, all detection fields are populated.
// On error (invalid input or resume with missing state.json): Errors is non-empty.
type PipelineInitResult struct {
	ResumeMode  ResumeMode         `json:"resume_mode,omitempty"`
	Workspace   string             `json:"workspace,omitempty"`
	Instruction string             `json:"instruction,omitempty"`
	SpecName    string             `json:"spec_name,omitempty"`
	SourceType  string             `json:"source_type,omitempty"`
	SourceURL   string             `json:"source_url,omitempty"`
	SourceID    string             `json:"source_id,omitempty"`
	CoreText    string             `json:"core_text,omitempty"`
	Flags       *PipelineInitFlags `json:"flags,omitempty"`
	FetchNeeded *FetchNeeded       `json:"fetch_needed,omitempty"`
	Errors      []string           `json:"errors,omitempty"`
}

// PipelineInitFlags holds the parsed flag values from the arguments string.
// All fields are always present in the Flags object (even if zero/nil).
type PipelineInitFlags struct {
	Auto           bool    `json:"auto"`
	SkipPR         bool    `json:"skip_pr"`
	Debug          bool    `json:"debug"`
	Discuss        bool    `json:"discuss"`
	EffortOverride *string `json:"effort_override"`
	CurrentBranch  string  `json:"current_branch,omitempty"`
}

// FetchNeeded describes the external data that must be fetched before
// pipeline_init_with_context can run decisions 6–13.
// Only populated for github_issue and jira_issue source types.
type FetchNeeded struct {
	Type        string   `json:"type"`
	Fields      []string `json:"fields"`
	Instruction string   `json:"instruction"`
}

// PipelineInitHandler handles the "pipeline_init" MCP tool.
// Accepts: arguments (string, required), current_branch (string, optional).
// Returns: PipelineInitResult serialised as JSON via okJSON.
// sm is accepted for uniform registration but is NOT used by this handler.
//

func PipelineInitHandler(sm *state.StateManager) server.ToolHandlerFunc {
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		arguments := req.GetString("arguments", "")
		currentBranch := req.GetString("current_branch", "")

		// Decision 1–4: Validate input and parse flags first so that resume
		// detection operates on stripped CoreText rather than the raw string.
		// This fixes the bug where ".specs/my-dir --debug" would pass the old
		// HasPrefix check but then fail state.json lookup due to the trailing flags.
		result := validation.ValidateInput(arguments)
		if !result.Valid {
			return okJSON(PipelineInitResult{
				Errors: result.Errors,
			})
		}

		// Decision 1: Resume detection — auto-detect from .specs/ directory.
		// 1a: CoreText starts with ".specs/" (user typed the full path).
		// 1b: CoreText matches a dirname under .specs/ (e.g. "20260320-fix-auth-timeout").
		// In both cases, if state.json exists, it's a resume.
		coreText := result.Parsed.CoreText
		if strings.HasPrefix(coreText, ".specs/") {
			return handleResumePath(coreText)
		}
		// Check if .specs/<coreText>/state.json exists — auto-resume detection.
		candidateWorkspace := path.Join(".specs", coreText)
		candidateStateJSON := filepath.Join(candidateWorkspace, "state.json")
		if _, err := os.Stat(candidateStateJSON); err == nil {
			return handleResumePath(candidateWorkspace)
		}

		// Build flags from parsed validation result.
		flags := buildFlags(result.Parsed, currentBranch)

		// Decision 5: Source type and workspace path.
		sourceType := result.Parsed.SourceType

		// Extract source_id for GitHub/Jira.
		sourceID := extractSourceID(sourceType, coreText)

		// Generate workspace path.
		workspace := makeWorkspacePath(time.Now(), coreText)

		// Derive spec_name from workspace base.
		specName := deriveSpecName(workspace)

		// Build fetch_needed block.
		fetchNeeded := makeFetchNeeded(sourceType)

		// source_url is only meaningful for URL-based source types; omit for text/workspace.
		var sourceURL string
		if sourceType == "github_issue" || sourceType == "jira_issue" {
			sourceURL = coreText
		}

		return okJSON(PipelineInitResult{
			Workspace:   workspace,
			SpecName:    specName,
			SourceType:  sourceType,
			SourceURL:   sourceURL,
			SourceID:    sourceID,
			CoreText:    result.Parsed.CoreText,
			Flags:       flags,
			FetchNeeded: fetchNeeded,
		})
	}
}

// handleResumePath handles the resume detection path.
// Returns a result with ResumeMode set if state.json exists, or an error result if not.
func handleResumePath(workspace string) (*mcp.CallToolResult, error) {
	stateJSONPath := filepath.Join(workspace, "state.json")
	if _, err := os.Stat(stateJSONPath); err != nil {
		// state.json absent — return error result (not MCP error).
		return okJSON(PipelineInitResult{
			Errors: []string{"workspace path looks like a resume candidate but state.json not found: " + workspace},
		})
	}
	return okJSON(PipelineInitResult{
		ResumeMode:  ResumeModeAuto,
		Workspace:   workspace,
		Instruction: "call state_resume_info",
	})
}

// buildFlags constructs PipelineInitFlags from the parsed validation result.
func buildFlags(parsed validation.ParsedInput, currentBranch string) *PipelineInitFlags {
	flags := &PipelineInitFlags{
		CurrentBranch: currentBranch,
	}

	// Bare flags from validation.
	for _, bf := range parsed.BareFlags {
		switch bf {
		case "auto":
			flags.Auto = true
		case "nopr":
			flags.SkipPR = true
		case "debug":
			flags.Debug = true
		case "discuss":
			flags.Discuss = true
		}
	}

	// Key-value flags: effort override.
	if v, ok := parsed.Flags["effort"]; ok {
		s := v
		flags.EffortOverride = &s
	}

	return flags
}

// extractSourceID extracts the source identifier from the core text for GitHub/Jira URLs.
// For GitHub: the issue number (e.g., "42" from .../issues/42).
// For Jira: the issue key (e.g., "PROJ-123" from .../browse/PROJ-123).
// Returns empty string for text/workspace source types.
func extractSourceID(sourceType, coreText string) string {
	switch sourceType {
	case "github_issue", "jira_issue":
		u, err := url.Parse(coreText)
		if err != nil {
			return ""
		}
		return path.Base(u.Path)
	default:
		return ""
	}
}

// slugifyOrDefault returns slugify(text), falling back to "task" when the result
// would be empty (e.g., all-Japanese input or an empty string).
func slugifyOrDefault(text string) string {
	if s := slugify(text); s != "" {
		return s
	}
	return "task"
}

// makeWorkspacePath generates the workspace path in the format .specs/YYYYMMDD-<slug>.
// Falls back to "task" when slugify produces an empty result (e.g., all-Japanese input).
func makeWorkspacePath(date time.Time, text string) string {
	dateStr := date.Format("20060102")
	return ".specs/" + dateStr + "-" + slugifyOrDefault(text)
}

// maxSlugLen is the maximum length of a slug produced by slugify.
// Kept in sync with the branch-name limit in orchestrator.deriveBranchName.
const maxSlugLen = 60

// slugify converts text to a lowercase, hyphen-separated slug:
//  1. Lowercase the full string
//  2. Replace all runs of non-alphanumeric characters with a single hyphen
//  3. Strip leading and trailing hyphens
//  4. Truncate to maxSlugLen characters, then strip any trailing hyphen
func slugify(text string) string {
	// Step 1: Lowercase.
	s := strings.ToLower(text)

	// Step 2: Replace all runs of non-alphanumeric characters with a single hyphen.
	var b strings.Builder
	inSep := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			inSep = false
		} else if !inSep {
			b.WriteRune('-')
			inSep = true
		}
	}
	result := b.String()

	// Step 3: Strip leading and trailing hyphens.
	result = strings.Trim(result, "-")

	// Step 4: Truncate to maxSlugLen characters, then strip any trailing hyphen.
	if len(result) > maxSlugLen {
		result = result[:maxSlugLen]
		result = strings.TrimRight(result, "-")
	}

	return result
}

// deriveSpecName extracts the spec name from the workspace path.
// It takes the base name and strips the YYYYMMDD- prefix.
// If no hyphen is found, it returns the full base name.
func deriveSpecName(workspace string) string {
	base := filepath.Base(workspace)
	_, after, ok := strings.Cut(base, "-")
	if !ok {
		return base
	}
	return after
}

// refineWorkspacePath replaces a URL-derived workspace path with a meaningful one
// when external context provides a source ID and/or summary (Jira or GitHub).
// For Jira with ID: ".specs/20260330-soa-2883-skip-minutes-job-without-integration"
// For GitHub with ID: ".specs/20260330-42-fix-auth-timeout"
// For title only (source_id absent): ".specs/20260330-fix-auth-timeout"
// Returns the original workspace path if no refinement is possible.
func refineWorkspacePath(workspace string, extCtx externalContext) string {
	var id, summary string

	switch {
	case extCtx.SourceID != "" && extCtx.JiraSummary != "":
		id = extCtx.SourceID
		summary = extCtx.JiraSummary
	case extCtx.SourceID != "" && extCtx.GitHubTitle != "":
		id = extCtx.SourceID
		summary = extCtx.GitHubTitle
	case extCtx.JiraSummary != "":
		summary = extCtx.JiraSummary
	case extCtx.GitHubTitle != "":
		summary = extCtx.GitHubTitle
	default:
		return workspace
	}

	combined := summary
	if id != "" {
		combined = id + " " + summary
	}
	return replaceWorkspaceSlug(workspace, slugifyOrDefault(combined))
}

// replaceWorkspaceSlug replaces the slug portion of a workspace path with newSlug,
// preserving the YYYYMMDD date prefix when present.
// Example: replaceWorkspaceSlug(".specs/20260330-old", "new-slug") → ".specs/20260330-new-slug"
func replaceWorkspaceSlug(workspace, newSlug string) string {
	base := filepath.Base(workspace)
	if idx := strings.IndexByte(base, '-'); idx > 0 {
		return filepath.Join(filepath.Dir(workspace), base[:idx]+"-"+newSlug)
	}
	return filepath.Join(filepath.Dir(workspace), newSlug)
}

// makeFetchNeeded constructs the FetchNeeded block for the given source type.
// Returns nil for text and workspace source types.
func makeFetchNeeded(sourceType string) *FetchNeeded {
	switch sourceType {
	case "github_issue":
		return &FetchNeeded{
			Type:        "github",
			Fields:      []string{"labels", "title", "body"},
			Instruction: "fetch github issue fields before calling pipeline_init_with_context",
		}
	case "jira_issue":
		return &FetchNeeded{
			Type:        "jira",
			Fields:      []string{"issue_type", "story_points", "summary", "description"},
			Instruction: "fetch jira issue fields before calling pipeline_init_with_context",
		}
	default:
		return nil
	}
}

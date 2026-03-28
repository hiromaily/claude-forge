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
	"unicode"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hiromaily/claude-forge/mcp-server/state"
	"github.com/hiromaily/claude-forge/mcp-server/validation"
)

// PipelineInitResult is the structured result returned by PipelineInitHandler.
// On resume path: Resume=true, Workspace and Instruction are set, all other fields zero.
// On new pipeline path: Resume=false, all detection fields are populated.
// On error (invalid input or resume with missing state.json): Errors is non-empty.
type PipelineInitResult struct {
	Resume      bool               `json:"resume,omitempty"`
	Workspace   string             `json:"workspace,omitempty"`
	Instruction string             `json:"instruction,omitempty"`
	SpecName    string             `json:"spec_name,omitempty"`
	SourceType  string             `json:"source_type,omitempty"`
	SourceURL   string             `json:"source_url,omitempty"`
	SourceID    string             `json:"source_id,omitempty"`
	Flags       *PipelineInitFlags `json:"flags,omitempty"`
	FetchNeeded *FetchNeeded       `json:"fetch_needed,omitempty"`
	Errors      []string           `json:"errors,omitempty"`
}

// PipelineInitFlags holds the parsed flag values from the arguments string.
// All five fields are always present in the Flags object (even if zero/nil).
type PipelineInitFlags struct {
	Auto           bool    `json:"auto"`
	SkipPR         bool    `json:"skip_pr"`
	Debug          bool    `json:"debug"`
	TypeOverride   *string `json:"type_override"`
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

		trimmed := strings.TrimSpace(arguments)

		// Decision 1: Resume detection.
		// Use HasPrefix check per design spec. If trimmed starts with ".specs/",
		// verify state.json exists before confirming resume.
		if strings.HasPrefix(trimmed, ".specs/") {
			return handleResumePath(trimmed)
		}

		// Decision 2–4: Validate input and parse flags.
		result := validation.ValidateInput(arguments)
		if !result.Valid {
			return okJSON(PipelineInitResult{
				Errors: result.Errors,
			})
		}

		// Build flags from parsed validation result.
		flags := buildFlags(result.Parsed, currentBranch)

		// Decision 5: Source type and workspace path.
		sourceType := result.Parsed.SourceType
		coreText := result.Parsed.CoreText

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
			Flags:       flags,
			FetchNeeded: fetchNeeded,
		})
	}
}

// handleResumePath handles the resume detection path.
// Returns resume:true if state.json exists, or an error result if it doesn't.
func handleResumePath(workspace string) (*mcp.CallToolResult, error) {
	stateJSONPath := filepath.Join(workspace, "state.json")
	if _, err := os.Stat(stateJSONPath); err != nil {
		// state.json absent — return error result (not MCP error).
		return okJSON(PipelineInitResult{
			Errors: []string{"workspace path looks like a resume candidate but state.json not found: " + workspace},
		})
	}
	return okJSON(PipelineInitResult{
		Resume:      true,
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
		}
	}

	// Key-value flags: type and effort overrides.
	if v, ok := parsed.Flags["type"]; ok {
		s := v
		flags.TypeOverride = &s
	}
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

// makeWorkspacePath generates the workspace path in the format .specs/YYYYMMDD-<slug>.
func makeWorkspacePath(date time.Time, text string) string {
	dateStr := date.Format("20060102")
	slug := slugify(text)
	return ".specs/" + dateStr + "-" + slug
}

// slugify converts text to a lowercase, hyphen-separated slug:
//  1. Lowercase the full string
//  2. Replace all runs of non-alphanumeric characters with a single hyphen
//  3. Strip leading and trailing hyphens
//  4. Truncate to 40 characters, then strip any trailing hyphen
func slugify(text string) string {
	// Step 1: Lowercase.
	s := strings.ToLower(text)

	// Step 2: Replace all runs of non-alphanumeric characters with a single hyphen.
	var b strings.Builder
	inSep := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
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

	// Step 4: Truncate to 40 characters, then strip any trailing hyphen.
	if len(result) > 40 {
		result = result[:40]
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

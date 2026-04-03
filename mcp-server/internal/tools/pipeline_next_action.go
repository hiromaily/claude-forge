// Package tools — pipeline_next_action MCP handler.
// Delegates to Engine.NextAction() and enriches spawn_agent prompts
// with agent .md file contents and input artifact file paths (not contents).
package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hiromaily/claude-forge/mcp-server/internal/history"
	"github.com/hiromaily/claude-forge/mcp-server/internal/orchestrator"
	"github.com/hiromaily/claude-forge/mcp-server/internal/profile"
	"github.com/hiromaily/claude-forge/mcp-server/internal/prompt"
	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
)

const similarPipelinesSearchLimit = 3

// outputVerdictHints maps output artifact filenames to a validation hint that
// tells the agent which verdict token(s) must appear in the file. These hints
// mirror the verdictSet requirements in validation/artifact.go so agents
// produce structurally valid artifacts on the first attempt.
//
//nolint:gochecknoglobals // package-level lookup table for verdict hints
var outputVerdictHints = map[string]string{
	state.ArtifactReviewDesign: "The file MUST contain exactly one verdict line: `## Verdict: APPROVE`, `APPROVE_WITH_NOTES`, or `REVISE`.",
	state.ArtifactReviewTasks:  "The file MUST contain exactly one verdict line: `## Verdict: APPROVE`, `APPROVE_WITH_NOTES`, or `REVISE`.",
}

// nextActionResponse wraps orchestrator.Action to add an optional Warning field.
// The warning is set fail-open when enrichPrompt cannot find the agent .md file.
type nextActionResponse struct {
	orchestrator.Action
	Warning string `json:"warning,omitempty"`
}

// PipelineNextActionHandler returns the next pipeline action for the orchestrator
// to execute, given the current workspace state.
//
// Parameters:
//   - workspace (required): absolute path to the workspace directory
//   - user_response (optional): response from the user to a checkpoint (forward-compatibility)
//
// The handler creates a per-call StateManager to avoid workspace-binding conflicts,
// delegates to eng.NextAction, and — for spawn_agent actions — enriches the prompt
// with the agent .md file contents and input artifact file paths.
func PipelineNextActionHandler(
	sm *state.StateManager,
	eng *orchestrator.Engine,
	agentDir string,
	histIdx *history.HistoryIndex,
	kb *history.KnowledgeBase,
	profiler *profile.RepoProfiler,
) server.ToolHandlerFunc {
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace, result, err := requireWorkspace(req)
		if result != nil {
			return result, err
		}
		userResponse := req.GetString("user_response", "")

		// Per-call StateManager: create a fresh instance to avoid workspace-mismatch errors.
		sm2 := state.NewStateManager(sm.Version())
		if err := sm2.LoadFromFile(workspace); err != nil {
			return errorf("load state: %v", err)
		}

		action, err := eng.NextAction(sm2, userResponse)
		if err != nil {
			return errorf("next_action: %v", err)
		}

		resp := nextActionResponse{Action: action}

		// Eliminate the window between pipeline_next_action returning a checkpoint action
		// and the orchestrator calling mcp__forge-state__checkpoint().
		// Set currentPhaseStatus to "awaiting_human" immediately so the stop hook permits
		// session exit even if the conversation ends before the orchestrator calls checkpoint().
		if action.Type == orchestrator.ActionCheckpoint {
			if updateErr := sm2.Update(func(s *state.State) error {
				s.CurrentPhaseStatus = "awaiting_human"
				return nil
			}); updateErr != nil {
				// Fail-open: warn but still return the action.
				resp.Warning = fmt.Sprintf("set awaiting_human: %v", updateErr)
			}
		}

		if action.Type == orchestrator.ActionSpawnAgent && agentDir != "" {
			if enrichErr := enrichPrompt(&resp, agentDir, workspace, sm2, histIdx, kb, profiler); enrichErr != nil {
				// Fail-open: return the action with a warning, not an error.
				resp.Warning = fmt.Sprintf("enrichPrompt: %v", enrichErr)
			}
		}

		return okJSON(resp)
	}
}

// enrichPrompt builds the 4-layer agent prompt by assembling:
//   - Layer 1: agent .md file contents
//   - Layer 2: input artifact file paths (agents read files themselves via Read tool)
//   - Layer 3: repository profile (from profiler.FormatForPrompt(); empty when profiler is nil)
//   - Layer 4: data flywheel history context (when histIdx is non-nil)
//
// The history query uses state.SpecName (falling back to filepath.Base(workspace)
// when SpecName is empty). On any error from history.Search, resp.Warning is set
// and the empty HistoryContext is used (fail-open pattern).
//
// Returns an error if the agent .md file is missing (caller should treat as warning).
func enrichPrompt(
	resp *nextActionResponse,
	agentDir, workspace string,
	sm *state.StateManager,
	histIdx *history.HistoryIndex,
	kb *history.KnowledgeBase,
	profiler *profile.RepoProfiler,
) error {
	action := &resp.Action

	agentFile := filepath.Join(agentDir, action.Agent+".md")
	agentData, err := os.ReadFile(agentFile)
	if err != nil {
		return fmt.Errorf("read agent file %q: %w", agentFile, err)
	}

	agentInstructions := string(agentData)

	// Build Layer 2 artifacts section — file paths only (no content inlining).
	// Agents read artifacts themselves via the Read tool. This keeps the MCP
	// response small (~1–2 KB) and avoids the "Large MCP response" error that
	// occurs when tasks.md/design.md contents are embedded (~50 KB+).
	var artifactSB strings.Builder
	if len(action.InputFiles) > 0 {
		artifactSB.WriteString("## Input Artifacts\n\n")
		artifactSB.WriteString("Read the following files from the workspace before starting:\n")
		for _, inputFile := range action.InputFiles {
			artifactSB.WriteString("- ")
			artifactSB.WriteString(filepath.Join(workspace, inputFile))
			artifactSB.WriteString("\n")
		}
	}

	// Output artifact instruction — deterministic enforcement that agents write
	// their output file. Without this, agents non-deterministically return results
	// as text without writing the file, causing pipeline_report_result to fail
	// artifact validation.
	if action.OutputFile != "" {
		if artifactSB.Len() > 0 {
			artifactSB.WriteString("\n")
		}
		outputPath := filepath.Join(workspace, action.OutputFile)
		artifactSB.WriteString("## Output Artifact\n\n")
		artifactSB.WriteString("**MANDATORY**: When you have finished your work, write your complete output to this file using the Write tool:\n")
		artifactSB.WriteString("- `")
		artifactSB.WriteString(outputPath)
		artifactSB.WriteString("`\n\n")
		artifactSB.WriteString("Do NOT return the output as response text only — the pipeline requires the file to exist on disk.\n")

		// Add verdict requirement hints for review-phase agents so they
		// include the required verdict token in their output file.
		if hint, ok := outputVerdictHints[action.OutputFile]; ok {
			artifactSB.WriteString("\n")
			artifactSB.WriteString(hint)
			artifactSB.WriteString("\n")
		}
	}

	artifactsSection := artifactSB.String()

	// Determine history search query from state.SpecName, falling back to workspace base.
	var histCtx prompt.HistoryContext

	if histIdx != nil {
		oneLiner := ""
		if st, stErr := sm.GetState(); stErr == nil {
			oneLiner = st.SpecName
		}

		if oneLiner == "" {
			oneLiner = filepath.Base(workspace)
		}

		results, searchErr := history.Search(histIdx, oneLiner, similarPipelinesSearchLimit)
		if searchErr != nil {
			// Fail-open: record warning and proceed with empty history context.
			if resp.Warning != "" {
				resp.Warning += "; "
			}

			resp.Warning += fmt.Sprintf("history.Search: %v", searchErr)
		} else {
			histCtx = prompt.BuildContextFromResults(results, kb)
		}
	}

	var profileStr string
	if profiler != nil {
		profileStr = profiler.FormatForPrompt()
	}

	action.Prompt = prompt.BuildPrompt(action.Agent, agentInstructions, artifactsSection, profileStr, histCtx)

	return nil
}

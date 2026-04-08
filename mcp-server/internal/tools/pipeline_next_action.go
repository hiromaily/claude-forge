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

const verdictHintApproveRevise = "The file MUST contain exactly one verdict line: `## Verdict: APPROVE`, `APPROVE_WITH_NOTES`, or `REVISE`."

// outputVerdictHints maps output artifact filenames to a validation hint that
// tells the agent which verdict token(s) must appear in the file. These hints
// mirror the verdictSet requirements in validation/artifact.go so agents
// produce structurally valid artifacts on the first attempt.
//
//nolint:gochecknoglobals // package-level lookup table for verdict hints
var outputVerdictHints = map[string]string{
	state.ArtifactReviewDesign: verdictHintApproveRevise,
	state.ArtifactReviewTasks:  verdictHintApproveRevise,
}

// nextActionResponse wraps orchestrator.Action to add an optional Warning field.
// The warning is set fail-open when enrichPrompt cannot find the agent .md file.
type nextActionResponse struct {
	orchestrator.Action
	Warning string `json:"warning,omitempty"`
}

// maxDispatchIter is the maximum number of iterations for the P1 skip loop and the
// P2/P3/P4 dispatch loop. The pipeline has 18 phases; 20 provides a safe margin.
const maxDispatchIter = 20

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
//
// Internal absorptions (not returned to the orchestrator):
//   - P1: skip-completion loop — ActionDone with SkipSummaryPrefix triggers
//     sm2.PhaseCompleteSkipped and re-invokes eng.NextAction until a non-skip action.
//   - P2: ActionTaskInit — calls executeTaskInit then re-invokes eng.NextAction.
//   - P3: ActionBatchCommit — calls executeBatchCommit, clears NeedsBatchCommit, re-invokes.
//   - P4: ActionExec with commands[0]=="final_commit" — calls executeFinalCommit, returns done.
//
//nolint:gocyclo // complexity is inherent in the P1-P4 dispatch logic; splitting would obscure the flow
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

		// P0: Resolve pending human gate from a previous call.
		// If PendingHumanGate is set, the user has acknowledged the gate
		// (by calling pipeline_next_action again). Mark the task completed
		// and clear the flag before computing the next action.
		if st, stErr := sm2.GetState(); stErr == nil && st.PendingHumanGate != nil {
			taskKey := *st.PendingHumanGate
			if updateErr := sm2.Update(func(s *state.State) error {
				if t, ok := s.Tasks[taskKey]; ok {
					t.ImplStatus = state.TaskStatusCompleted
					t.ReviewStatus = state.TaskStatusCompletedPass // skip review for human gates
					s.Tasks[taskKey] = t
				}
				s.PendingHumanGate = nil
				s.CurrentPhaseStatus = state.StatusInProgress
				return nil
			}); updateErr != nil {
				return errorf("resolve human gate task %s: %v", taskKey, updateErr)
			}
		}

		action, err := eng.NextAction(sm2, userResponse)
		if err != nil {
			return errorf("next_action: %v", err)
		}

		// P1: absorb skip-completion loop internally.
		// Each ActionDone with a SkipSummaryPrefix triggers PhaseCompleteSkipped and
		// re-invokes eng.NextAction. The loop is bounded to 20 iterations (pipeline has
		// 18 phases; 20 provides a safe margin against infinite cycles).
		for iter := range maxDispatchIter {
			if action.Type != orchestrator.ActionDone ||
				!strings.HasPrefix(action.Summary, orchestrator.SkipSummaryPrefix) {
				break
			}
			skipPhase := strings.TrimPrefix(action.Summary, orchestrator.SkipSummaryPrefix)
			if skipErr := sm2.PhaseCompleteSkipped(workspace, skipPhase); skipErr != nil {
				return errorf("skip phase_complete %s: %v", skipPhase, skipErr)
			}
			action, err = eng.NextAction(sm2, "")
			if err != nil {
				return errorf("next_action (after skip %s): %v", skipPhase, err)
			}
			if iter == maxDispatchIter-1 {
				// We've used all iterations and the loop condition would be rechecked.
				// Check if we'd loop again (i.e., still in skip mode).
				if action.Type == orchestrator.ActionDone &&
					strings.HasPrefix(action.Summary, orchestrator.SkipSummaryPrefix) {
					return errorf("skip loop exceeded %d iterations — possible engine cycle; last skip: %s",
						maxDispatchIter, strings.TrimPrefix(action.Summary, orchestrator.SkipSummaryPrefix))
				}
			}
		}

		resp := nextActionResponse{Action: action}

		// appendWarning accumulates warnings into resp.Warning, semicolon-separated.
		appendWarning := func(msg string) {
			if resp.Warning != "" {
				resp.Warning += "; "
			}
			resp.Warning += msg
		}

		// P2/P3/P4 dispatch loop: absorb ActionTaskInit, ActionBatchCommit, and the
		// final_commit exec interception. The loop is bounded by maxDispatchIter to guard
		// against infinite cycles.
		for iter := range maxDispatchIter {
			switch action.Type {
			case orchestrator.ActionTaskInit:
				// P2: execute task_init internally and re-enter the loop.
				if taskErr := executeTaskInit(action.Phase, sm2); taskErr != nil {
					return errorf("task_init: %v", taskErr)
				}
				action, err = eng.NextAction(sm2, "")
				if err != nil {
					return errorf("next_action (after task_init): %v", err)
				}
				if iter == maxDispatchIter-1 {
					return errorf("dispatch loop exceeded %d iterations — possible engine cycle; last action: %s",
						maxDispatchIter, action.Type)
				}
				continue

			case orchestrator.ActionBatchCommit:
				// P3: execute batch commit internally, clear NeedsBatchCommit, re-enter.
				warning, batchErr := executeBatchCommit(workspace, sm2)
				if warning != "" {
					appendWarning(warning)
				}
				if batchErr != nil {
					return errorf("batch_commit: %v", batchErr)
				}
				// Clear NeedsBatchCommit flag in state.
				if updateErr := sm2.Update(func(s *state.State) error {
					s.NeedsBatchCommit = false
					return nil
				}); updateErr != nil {
					return errorf("clear NeedsBatchCommit: %v", updateErr)
				}
				action, err = eng.NextAction(sm2, "")
				if err != nil {
					return errorf("next_action (after batch_commit): %v", err)
				}
				if iter == maxDispatchIter-1 {
					return errorf("dispatch loop exceeded %d iterations — possible engine cycle; last action: %s",
						maxDispatchIter, action.Type)
				}
				continue

			case orchestrator.ActionExec:
				// P4: intercept final_commit exec — handle entirely in Go.
				if len(action.Commands) > 0 && action.Commands[0] == "final_commit" {
					if finalErr := executeFinalCommit(workspace, sm2, kb); finalErr != nil {
						return errorf("final_commit: %v", finalErr)
					}
					return okJSON(nextActionResponse{Action: orchestrator.NewDoneAction("pipeline completed", "")})
				}
				// Non-final_commit exec: fall through to return the action to the orchestrator.

			case orchestrator.ActionHumanGate:
				// P5: Human gate — store the pending task key in state so P0 can
				// resolve it on the next call. The action is returned as-is
				// (type "human_gate") to the orchestrator, which follows SKILL.md
				// human_gate rules (present to user, then call pipeline_next_action).
				// Do NOT convert to ActionCheckpoint — the orchestrator must NOT
				// call checkpoint() or phase_complete() for human gates.
				taskKey := action.Name
				if updateErr := sm2.Update(func(s *state.State) error {
					s.PendingHumanGate = &taskKey
					s.CurrentPhaseStatus = state.StatusAwaitingHuman
					return nil
				}); updateErr != nil {
					return errorf("set PendingHumanGate: %v", updateErr)
				}

			default:
				// ActionSpawnAgent, ActionCheckpoint, ActionWriteFile, ActionDone — return as-is.
			}

			// Action is ready to be returned to the orchestrator.
			break
		}

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
				appendWarning(fmt.Sprintf("set awaiting_human: %v", updateErr))
			}
		}

		resp.Action = action

		if action.Type == orchestrator.ActionSpawnAgent && agentDir != "" {
			if enrichErr := enrichPrompt(&resp, agentDir, workspace, sm2, histIdx, kb, profiler); enrichErr != nil {
				// Fail-open: return the action with a warning, not an error.
				appendWarning(fmt.Sprintf("enrichPrompt: %v", enrichErr))
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

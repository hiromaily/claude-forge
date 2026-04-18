// pipeline_next_action MCP handler.
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

	"github.com/hiromaily/claude-forge/mcp-server/internal/events"
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

// isCheckpointPhase returns true if the phase is a human-review checkpoint.
func isCheckpointPhase(phase string) bool {
	return phase == state.PhaseCheckpointA || phase == state.PhaseCheckpointB
}

// previousResult captures optional metrics from the action the orchestrator just completed.
// The P5 report block fires when actionComplete is true OR when any metric is non-zero
// (tokensUsed > 0, model != "", or durationMs > 0). actionComplete is the preferred
// signal — it handles fast exec/write_file actions where all numeric metrics may be zero.
type previousResult struct {
	tokensUsed     int
	durationMs     int
	model          string
	setupOnly      bool
	actionComplete bool
}

// reportResultEmbedded carries the report-result outcome inside nextActionResponse
// when previous_* parameters triggered a non-proceed transition.
type reportResultEmbedded struct {
	NextActionHint string                 `json:"next_action_hint"`
	VerdictParsed  string                 `json:"verdict_parsed,omitempty"`
	Findings       []orchestrator.Finding `json:"findings,omitempty"`
	Warning        string                 `json:"warning,omitempty"`
	DisplayMessage string                 `json:"display_message,omitempty"`
}

// nextActionResponse wraps orchestrator.Action with optional Warning and DisplayMessage fields.
// Warning is set fail-open when enrichPrompt cannot find the agent .md file.
// DisplayMessage is a pre-formatted progress line the orchestrator should output verbatim
// before executing the action (e.g. "▶ Phase 1 — Situation Analysis  ·  spawning …").
// ReportResult is present when previous_* params triggered a non-proceed transition.
type nextActionResponse struct {
	orchestrator.Action
	Warning            string                `json:"warning,omitempty"`
	DisplayMessage     string                `json:"display_message,omitempty"`
	ReportResult       *reportResultEmbedded `json:"report_result,omitempty"`
	CurrentPhase       string                `json:"current_phase,omitempty"`
	CurrentPhaseStatus string                `json:"current_phase_status,omitempty"`
}

// parsePreviousResult extracts the optional previous_* parameters from the MCP request.
func parsePreviousResult(req mcp.CallToolRequest) previousResult {
	return previousResult{
		tokensUsed:     req.GetInt("previous_tokens", 0),
		durationMs:     req.GetInt("previous_duration_ms", 0),
		model:          req.GetString("previous_model", ""),
		setupOnly:      req.GetBool("previous_setup_only", false),
		actionComplete: req.GetBool("previous_action_complete", false),
	}
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
	bus *events.EventBus,
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

		// P5: If previous_* parameters are supplied, run reportResultCore before computing
		// the next action. This merges pipeline_report_result into the pipeline_next_action
		// call, reducing the main loop from 3 calls to 2 calls per cycle.
		prev := parsePreviousResult(req)
		if prev.actionComplete || prev.tokensUsed > 0 || prev.model != "" || prev.durationMs > 0 {
			st, stErr := sm2.GetState()
			if stErr != nil {
				return errorf("get state for report: %v", stErr)
			}
			// Guard: skip reportResultCore for non-reportable phases.
			if st.CurrentPhase != "setup" && st.CurrentPhase != "completed" {
				rIn := reportResultInput{
					workspace:  workspace,
					phase:      st.CurrentPhase,
					tokensUsed: prev.tokensUsed,
					durationMs: prev.durationMs,
					model:      prev.model,
					setupOnly:  prev.setupOnly,
				}
				outcome, rErr := reportResultCore(sm2, kb, rIn)
				if rErr != nil {
					return errorf("report_result: %v", rErr)
				}
				// Publish action-complete for the finished agent/exec step.
				publishEventWithDetail(bus, nil, "action-complete", st.CurrentPhase, st.SpecName, workspace, "completed", prev.model)

				switch outcome.NextActionHint {
				case "revision_required":
					publishEvent(bus, nil, "revision-required", st.CurrentPhase, st.SpecName, workspace, "failed")
					// Surface to orchestrator; do NOT call eng.NextAction.
					return okJSON(nextActionResponse{
						ReportResult: &reportResultEmbedded{
							NextActionHint: outcome.NextActionHint,
							VerdictParsed:  outcome.VerdictParsed,
							Findings:       outcome.Findings,
							Warning:        outcome.Warning,
						},
					})
				case "setup_continue":
					// Absorbed internally; fall through to eng.NextAction.
				default:
					// "proceed" — phase completed successfully; emit phase-complete event.
					publishEvent(bus, nil, "phase-complete", st.CurrentPhase, st.SpecName, workspace, "completed")
				}
			}
		}

		// P8: Checkpoint response handler — the orchestrator passes the user's
		// response and the engine handles all state transitions deterministically.
		// The orchestrator MUST NOT call phase_complete for checkpoints; this
		// block owns the full lifecycle (proceed → advance, revise → rewind,
		// abandon → mark abandoned).
		if userResponse != "" {
			if st, stErr := sm2.GetState(); stErr == nil && isCheckpointPhase(st.CurrentPhase) {
				switch userResponse {
				case "proceed":
					if completeErr := sm2.PhaseComplete(workspace, st.CurrentPhase); completeErr != nil {
						return errorf("checkpoint proceed %s: %v", st.CurrentPhase, completeErr)
					}
				case "revise":
					var targetPhase, reviewPhase, reviewArtifact string
					switch st.CurrentPhase {
					case state.PhaseCheckpointA:
						targetPhase = state.PhaseThree
						reviewPhase = state.PhaseThreeB
						reviewArtifact = state.ArtifactReviewDesign
					case state.PhaseCheckpointB:
						targetPhase = state.PhaseFour
						reviewPhase = state.PhaseFourB
						reviewArtifact = state.ArtifactReviewTasks
					}
					// Delete stale review file so the review phase re-runs
					// the reviewer after the revision agent produces updated output.
					if reviewArtifact != "" {
						_ = os.Remove(filepath.Join(workspace, reviewArtifact))
					}
					if targetPhase != "" {
						if updateErr := sm2.Update(func(s *state.State) error {
							s.CurrentPhase = targetPhase
							s.CurrentPhaseStatus = state.StatusInProgress
							// Remove the review phase from CompletedPhases so the
							// agent handler doesn't include the deleted review file as input.
							if reviewPhase != "" {
								filtered := make([]string, 0, len(s.CompletedPhases))
								for _, p := range s.CompletedPhases {
									if p != reviewPhase {
										filtered = append(filtered, p)
									}
								}
								s.CompletedPhases = filtered
							}
							return nil
						}); updateErr != nil {
							return errorf("rewind %s to %s: %v", st.CurrentPhase, targetPhase, updateErr)
						}
					}
				case "abandon":
					if abandonErr := sm2.Abandon(workspace); abandonErr != nil {
						return errorf("checkpoint abandon: %v", abandonErr)
					}
					return okJSON(nextActionResponse{
						Action: orchestrator.NewDoneAction("pipeline abandoned at "+st.CurrentPhase, ""),
					})
				}
			}
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

			case orchestrator.ActionRenameBranch:
				// P6: execute git branch -m internally, update state, re-enter.
				newBranch := action.NewBranch // capture before action is reassigned
				if renameErr := runGit(workspace, "branch", "-m", action.OldBranch, newBranch); renameErr != nil {
					return errorf("rename_branch git: %v", renameErr)
				}
				if updateErr := sm2.Update(func(s *state.State) error {
					s.Branch = &newBranch
					s.BranchClassified = true
					return nil
				}); updateErr != nil {
					return errorf("rename_branch state update: %v", updateErr)
				}
				action, err = eng.NextAction(sm2, "")
				if err != nil {
					return errorf("next_action (after rename_branch): %v", err)
				}
				if iter == maxDispatchIter-1 {
					return errorf("dispatch loop exceeded %d iterations — possible engine cycle; last action: %s",
						maxDispatchIter, action.Type)
				}
				continue

			case orchestrator.ActionPushBranch:
				// P7: push feature branch to remote before pr-creation, update state, re-enter.
				// Absorbed internally so the orchestrator never sees push_branch as an action.
				// git push -u origin HEAD works on any checked-out branch name and is idempotent.
				if pushErr := runGit(workspace, "push", "-u", "origin", "HEAD"); pushErr != nil {
					return errorf("push_branch git: %v", pushErr)
				}
				if updateErr := sm2.Update(func(s *state.State) error {
					s.BranchPushed = true
					return nil
				}); updateErr != nil {
					return errorf("push_branch state update: %v", updateErr)
				}
				action, err = eng.NextAction(sm2, "")
				if err != nil {
					return errorf("next_action (after push_branch): %v", err)
				}
				if iter == maxDispatchIter-1 {
					return errorf("dispatch loop exceeded %d iterations — possible engine cycle; last action: %s",
						maxDispatchIter, action.Type)
				}
				continue

			default:
				// ActionSpawnAgent, ActionCheckpoint, ActionWriteFile, ActionDone — return as-is.
			}

			// Action is ready to be returned to the orchestrator.
			break
		}

		// When reaching a checkpoint without a rename, mark BranchClassified so the engine
		// does not re-evaluate branch type on subsequent calls.
		if action.Type == orchestrator.ActionCheckpoint {
			if st2, stErr := sm2.GetState(); stErr == nil && !st2.BranchClassified {
				if updateErr := sm2.Update(func(s *state.State) error {
					s.BranchClassified = true
					return nil
				}); updateErr != nil {
					appendWarning(fmt.Sprintf("set BranchClassified: %v", updateErr))
				}
			}
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
			if st, stErr := sm2.GetState(); stErr == nil {
				publishEvent(bus, nil, "checkpoint", action.Phase, st.SpecName, workspace, "awaiting_human")
			}
		}

		resp.Action = action
		resp.DisplayMessage = buildSpawnMessage(action)

		if action.Type == orchestrator.ActionSpawnAgent && agentDir != "" {
			if enrichErr := enrichPrompt(&resp, agentDir, workspace, sm2, histIdx, kb, profiler); enrichErr != nil {
				// Fail-open: return the action with a warning, not an error.
				appendWarning(fmt.Sprintf("enrichPrompt: %v", enrichErr))
			}
		}

		// Transition phase state to in_progress and emit dashboard events.
		// PhaseStart sets CurrentPhaseStatus="in_progress" and Timestamps.PhaseStarted.
		// Checkpoint and done actions are excluded — checkpoints set awaiting_human above,
		// and done signals pipeline completion (no phase to start).
		if action.Type != orchestrator.ActionCheckpoint && action.Type != orchestrator.ActionDone && action.Type != orchestrator.ActionHumanGate {
			if startErr := sm2.PhaseStart(workspace, action.Phase); startErr != nil {
				appendWarning(fmt.Sprintf("PhaseStart: %v", startErr))
			}
		}

		// Include current phase state in response for debugging and publish
		// fine-grained dispatch events for the dashboard.
		if st, stErr := sm2.GetState(); stErr == nil {
			resp.CurrentPhase = st.CurrentPhase
			resp.CurrentPhaseStatus = st.CurrentPhaseStatus
			switch action.Type {
			case orchestrator.ActionSpawnAgent:
				publishEvent(bus, nil, "phase-start", action.Phase, st.SpecName, workspace, "in_progress")
				publishEventWithDetail(bus, nil, "agent-dispatch", action.Phase, st.SpecName, workspace, "dispatched", action.Agent)
			case orchestrator.ActionExec, orchestrator.ActionWriteFile:
				publishEvent(bus, nil, "phase-start", action.Phase, st.SpecName, workspace, "in_progress")
			case orchestrator.ActionDone:
				publishEvent(bus, nil, "pipeline-complete", st.CurrentPhase, st.SpecName, workspace, "completed")
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
func enrichPrompt( //nolint:gocyclo // complexity is inherent in the dispatch table
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

	// Substitute agent instruction template variables with runtime values.
	// Agent .md files use {workspace}, {branch}, {spec-name}, and {N} as placeholders
	// that must be resolved before the prompt is sent, so agents receive concrete paths
	// and identifiers rather than literal brace-tokens.
	agentInstructions = strings.ReplaceAll(agentInstructions, "{workspace}", workspace)
	if taskN := extractTaskNumber(action.OutputFile); taskN != "" {
		agentInstructions = strings.ReplaceAll(agentInstructions, "{N}", taskN)
	}
	if st, stErr := sm.GetState(); stErr == nil {
		branch := ""
		if st.Branch != nil {
			branch = *st.Branch
		}
		agentInstructions = strings.ReplaceAll(agentInstructions, "{branch}", branch)
		agentInstructions = strings.ReplaceAll(agentInstructions, "{spec-name}", st.SpecName)
	}

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

	// Filter out patterns matching the current review's findings from the architect's
	// prompt. This prevents "Common Review Findings" from showing stale/already-addressed
	// findings that duplicate what's in review-design.md (which the architect reads directly).
	// Scope: architect only (reads review-design.md). If task-decomposer needs similar
	// filtering for review-tasks.md, extend this block.
	if action.Agent == "architect" {
		histCtx = filterCurrentReviewFindings(workspace, histCtx)
	}

	action.Prompt = prompt.BuildPrompt(action.Agent, agentInstructions, artifactsSection, profileStr, histCtx)

	// Layer 5: checkpoint feedback injection.
	// When a user approves a checkpoint with a message via the dashboard,
	// the message is written to checkpoint-message.txt. Read and consume it
	// here so it is deterministically injected into the agent prompt —
	// no reliance on SKILL.md or LLM interpretation.
	//
	// Invariant: this code only runs for ActionSpawnAgent (enrichPrompt is
	// gated on that type). If the next action after a checkpoint were
	// ActionExec/ActionWriteFile/ActionDone, the file would persist until
	// the next spawn_agent call. In practice this does not happen: after
	// checkpoint-a the next phase is always phase-4 (task-decomposer,
	// spawn_agent) and after checkpoint-b it is phase-5 (implementer,
	// spawn_agent after task_init absorption).
	//
	// Note: the ReadFile+Remove sequence is not atomic (TOCTOU). This is
	// acceptable because pipeline_next_action is called sequentially by the
	// orchestrator — concurrent calls for the same workspace do not occur
	// in normal operation.
	msgFile := filepath.Join(workspace, "checkpoint-message.txt")
	if msgData, readErr := os.ReadFile(msgFile); readErr == nil {
		msg := strings.TrimSpace(string(msgData))
		if msg != "" {
			action.Prompt += "\n\n## Human Feedback\n\n" +
				"The reviewer provided the following instructions during checkpoint approval. " +
				"Incorporate this feedback into your work:\n\n" + msg + "\n"
		}
		_ = os.Remove(msgFile)
	}

	return nil
}

// filterCurrentReviewFindings removes pattern entries that match findings in
// the current pipeline's review-design.md. This prevents the architect from
// seeing stale "Common Review Findings" that duplicate the review feedback
// already available as an input artifact.
func filterCurrentReviewFindings(workspace string, ctx prompt.HistoryContext) prompt.HistoryContext {
	reviewPath := filepath.Join(workspace, state.ArtifactReviewDesign)
	_, findings, err := orchestrator.ParseVerdict(reviewPath)
	if err != nil || len(findings) == 0 {
		return ctx
	}

	// Build a set of normalized finding descriptions to exclude.
	exclude := make(map[string]bool, len(findings))
	for _, f := range findings {
		exclude[strings.ToLower(f.Description)] = true
	}

	// Filter AllPatterns.
	filtered := make([]history.PatternEntry, 0, len(ctx.AllPatterns))
	for _, p := range ctx.AllPatterns {
		if !exclude[strings.ToLower(p.Pattern)] {
			filtered = append(filtered, p)
		}
	}
	ctx.AllPatterns = filtered

	// Filter CriticalPatterns.
	filteredCritical := make([]history.PatternEntry, 0, len(ctx.CriticalPatterns))
	for _, p := range ctx.CriticalPatterns {
		if !exclude[strings.ToLower(p.Pattern)] {
			filteredCritical = append(filteredCritical, p)
		}
	}
	ctx.CriticalPatterns = filteredCritical

	return ctx
}

// extractTaskNumber parses the task number from an output artifact filename.
// "impl-1.md" → "1", "review-2.md" → "2". Returns "" when the filename does
// not match either pattern (e.g. "analysis.md", "design.md").
func extractTaskNumber(outputFile string) string {
	for _, prefix := range []string{"impl-", "review-"} {
		if rest, ok := strings.CutPrefix(outputFile, prefix); ok {
			if n, ok2 := strings.CutSuffix(rest, ".md"); ok2 {
				return n
			}
		}
	}
	return ""
}

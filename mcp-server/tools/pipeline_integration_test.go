// Package tools — Go integration tests for pipeline round-trip flows.
// These tests exercise the full handler chain:
//
//	PipelineNextActionHandler → PipelineReportResultHandler
//
// using a real state.json in a temp directory.
package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/history"
	"github.com/hiromaily/claude-forge/mcp-server/orchestrator"
	"github.com/hiromaily/claude-forge/mcp-server/state"
)

// TestPipelineRoundTrip_Phase1ToPhase2 verifies that:
//  1. PipelineNextActionHandler at phase-1 returns a spawn_agent action.
//  2. After writing the analysis.md fixture, PipelineReportResultHandler
//     advances currentPhase to phase-2.
func TestPipelineRoundTrip_Phase1ToPhase2(t *testing.T) {
	t.Parallel()

	workspace, sm := initWorkspaceForNextAction(t, "phase-1", nil)
	eng := orchestrator.NewEngine("", "")
	nextActionH := PipelineNextActionHandler(sm, eng, "", nil, nil)
	reportResultH := PipelineReportResultHandler(state.NewStateManager(), history.NewKnowledgeBase(""))

	// Step 1: call pipeline_next_action at phase-1.
	result, err := callNextAction(t, nextActionH, workspace)
	if err != nil {
		t.Fatalf("PipelineNextActionHandler returned Go error: %v", err)
	}
	if result.IsError {
		t.Fatalf("PipelineNextActionHandler returned MCP error: %s", textContent(result))
	}

	var action orchestrator.Action
	if err := json.Unmarshal([]byte(textContent(result)), &action); err != nil {
		t.Fatalf("unmarshal action: %v (raw: %s)", err, textContent(result))
	}

	// Assert: spawn_agent for phase-1.
	if action.Type != orchestrator.ActionSpawnAgent {
		t.Fatalf("action.Type = %q, want %q", action.Type, orchestrator.ActionSpawnAgent)
	}
	if action.Phase != orchestrator.PhaseOne {
		t.Errorf("action.Phase = %q, want %q", action.Phase, orchestrator.PhaseOne)
	}

	// Step 2: write analysis.md fixture so artifact validation in report_result passes.
	analysisMD := "# Analysis\n\nThis is the situation analysis output.\n"
	if err := os.WriteFile(filepath.Join(workspace, "analysis.md"), []byte(analysisMD), 0o600); err != nil {
		t.Fatalf("write analysis.md: %v", err)
	}

	// Step 3: call pipeline_report_result for phase-1.
	reportRes := callTool(t, reportResultH, map[string]any{
		"workspace":   workspace,
		"phase":       "phase-1",
		"tokens_used": 1000,
		"duration_ms": 500,
		"model":       "sonnet",
	})
	if reportRes.IsError {
		t.Fatalf("PipelineReportResultHandler returned MCP error: %s", textContent(reportRes))
	}

	// Assert: currentPhase advanced to phase-2.
	s, err := state.ReadState(workspace)
	if err != nil {
		t.Fatalf("ReadState after report_result: %v", err)
	}
	if s.CurrentPhase != orchestrator.PhaseTwo {
		t.Errorf("currentPhase = %q after phase-1 report_result, want %q", s.CurrentPhase, orchestrator.PhaseTwo)
	}
}

// TestPipelineRoundTrip_SkipSignal verifies that:
//  1. PipelineNextActionHandler at phase-2 (with phase-2 in skippedPhases)
//     returns a done action whose summary starts with "skip:".
//  2. After calling phase_complete for phase-2, the next call returns
//     the first non-skipped phase (phase-3).
func TestPipelineRoundTrip_SkipSignal(t *testing.T) {
	t.Parallel()

	workspace, sm := initWorkspaceForNextAction(t, "phase-2", func(s *state.State) error {
		s.SkippedPhases = []string{"phase-2"}
		return nil
	})
	eng := orchestrator.NewEngine("", "")
	nextActionH := PipelineNextActionHandler(sm, eng, "", nil, nil)

	// Step 1: call pipeline_next_action at phase-2 which is skipped.
	result, err := callNextAction(t, nextActionH, workspace)
	if err != nil {
		t.Fatalf("PipelineNextActionHandler (skip) returned Go error: %v", err)
	}
	if result.IsError {
		t.Fatalf("PipelineNextActionHandler (skip) returned MCP error: %s", textContent(result))
	}

	var skipAction orchestrator.Action
	if err := json.Unmarshal([]byte(textContent(result)), &skipAction); err != nil {
		t.Fatalf("unmarshal skip action: %v (raw: %s)", err, textContent(result))
	}

	// Assert: done with "skip:" prefix.
	if skipAction.Type != orchestrator.ActionDone {
		t.Fatalf("skip action.Type = %q, want %q", skipAction.Type, orchestrator.ActionDone)
	}
	if !strings.HasPrefix(skipAction.Summary, orchestrator.SkipSummaryPrefix) {
		t.Fatalf("skip action.Summary = %q, want prefix %q", skipAction.Summary, orchestrator.SkipSummaryPrefix)
	}

	// Step 2: simulate SKILL.md loop — call phase_complete for the skipped phase,
	// then call pipeline_next_action again.
	// Use a fresh StateManager to avoid workspace-binding conflicts.
	smPhaseComplete := state.NewStateManager()
	if err := smPhaseComplete.PhaseComplete(workspace, "phase-2"); err != nil {
		t.Fatalf("PhaseComplete(phase-2): %v", err)
	}

	// Step 3: call pipeline_next_action again — should return the next non-skipped phase.
	// Create a new handler bound to the updated workspace.
	smNext := state.NewStateManager()
	if err := smNext.LoadFromFile(workspace); err != nil {
		t.Fatalf("LoadFromFile after phase_complete: %v", err)
	}
	nextActionH2 := PipelineNextActionHandler(smNext, eng, "", nil, nil)
	result2, err := callNextAction(t, nextActionH2, workspace)
	if err != nil {
		t.Fatalf("PipelineNextActionHandler (after skip) returned Go error: %v", err)
	}
	if result2.IsError {
		t.Fatalf("PipelineNextActionHandler (after skip) returned MCP error: %s", textContent(result2))
	}

	var nextAction orchestrator.Action
	if err := json.Unmarshal([]byte(textContent(result2)), &nextAction); err != nil {
		t.Fatalf("unmarshal next action: %v (raw: %s)", err, textContent(result2))
	}

	// Assert: the returned action is for phase-3 (first non-skipped phase after phase-2).
	if nextAction.Type != orchestrator.ActionSpawnAgent {
		t.Errorf("nextAction.Type = %q, want %q", nextAction.Type, orchestrator.ActionSpawnAgent)
	}
	if nextAction.Phase != orchestrator.PhaseThree {
		t.Errorf("nextAction.Phase = %q, want %q", nextAction.Phase, orchestrator.PhaseThree)
	}
}

// TestPipelineRoundTrip_ExecPhase verifies that:
//  1. PipelineNextActionHandler at pr-creation returns an exec action
//     with Phase == "pr-creation".
//  2. PipelineReportResultHandler records the phase-log and advances state.
func TestPipelineRoundTrip_ExecPhase(t *testing.T) {
	t.Parallel()

	// Set up workspace at pr-creation phase.
	workspace, sm := initWorkspaceForNextAction(t, "pr-creation", nil)
	eng := orchestrator.NewEngine("", "")
	nextActionH := PipelineNextActionHandler(sm, eng, "", nil, nil)
	reportResultH := PipelineReportResultHandler(state.NewStateManager(), history.NewKnowledgeBase(""))

	// Step 1: call pipeline_next_action at pr-creation.
	result, err := callNextAction(t, nextActionH, workspace)
	if err != nil {
		t.Fatalf("PipelineNextActionHandler (pr-creation) returned Go error: %v", err)
	}
	if result.IsError {
		t.Fatalf("PipelineNextActionHandler (pr-creation) returned MCP error: %s", textContent(result))
	}

	var action orchestrator.Action
	if err := json.Unmarshal([]byte(textContent(result)), &action); err != nil {
		t.Fatalf("unmarshal exec action: %v (raw: %s)", err, textContent(result))
	}

	// Assert: exec action with Phase == "pr-creation".
	if action.Type != orchestrator.ActionExec {
		t.Fatalf("action.Type = %q, want %q", action.Type, orchestrator.ActionExec)
	}
	if action.Phase != orchestrator.PhasePRCreation {
		t.Errorf("action.Phase = %q, want %q", action.Phase, orchestrator.PhasePRCreation)
	}
	if len(action.Commands) == 0 {
		t.Errorf("action.Commands is empty for exec action at pr-creation")
	}

	// Step 2: call pipeline_report_result for pr-creation.
	reportRes := callTool(t, reportResultH, map[string]any{
		"workspace":   workspace,
		"phase":       "pr-creation",
		"tokens_used": 0,
		"duration_ms": 200,
		"model":       "sonnet",
	})
	if reportRes.IsError {
		t.Fatalf("PipelineReportResultHandler (pr-creation) returned MCP error: %s", textContent(reportRes))
	}

	// Parse report result response.
	resp := parsePRRResponse(t, textContent(reportRes))
	if !resp.StateUpdated {
		t.Errorf("StateUpdated = false after pr-creation report_result, want true")
	}
	if resp.NextActionHint != "proceed" {
		t.Errorf("NextActionHint = %q, want %q", resp.NextActionHint, "proceed")
	}

	// Assert: state advanced past pr-creation.
	s, err := state.ReadState(workspace)
	if err != nil {
		t.Fatalf("ReadState after pr-creation report_result: %v", err)
	}
	if s.CurrentPhase == orchestrator.PhasePRCreation {
		t.Errorf("currentPhase still %q after report_result; expected it to advance", orchestrator.PhasePRCreation)
	}
}

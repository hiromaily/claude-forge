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

	"github.com/hiromaily/claude-forge/mcp-server/internal/history"
	"github.com/hiromaily/claude-forge/mcp-server/internal/orchestrator"
	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
)

// TestPipelineRoundTrip_Phase1ToPhase2 verifies that:
//  1. PipelineNextActionHandler at phase-1 returns a spawn_agent action.
//  2. After writing the analysis.md fixture, PipelineReportResultHandler
//     advances currentPhase to phase-2.
func TestPipelineRoundTrip_Phase1ToPhase2(t *testing.T) {
	t.Parallel()

	workspace, sm := initWorkspaceForNextAction(t, "phase-1", nil)
	eng := orchestrator.NewEngine("", "")
	nextActionH := PipelineNextActionHandler(sm, eng, "", nil, nil, nil)
	reportResultH := PipelineReportResultHandler(state.NewStateManager("dev"), history.NewKnowledgeBase(""))

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

// TestPipelineRoundTrip_SkipSignal verifies that the P1 skip-absorption loop is
// fully internal: when phase-2 is in skippedPhases, PipelineNextActionHandler
// absorbs the skip signal and returns the first non-skipped actionable phase
// (phase-3 spawn_agent) rather than exposing the done+skip: signal to the caller.
func TestPipelineRoundTrip_SkipSignal(t *testing.T) {
	t.Parallel()

	workspace, sm := initWorkspaceForNextAction(t, "phase-2", func(s *state.State) error {
		s.SkippedPhases = []string{"phase-2"}
		return nil
	})
	eng := orchestrator.NewEngine("", "")
	nextActionH := PipelineNextActionHandler(sm, eng, "", nil, nil, nil)

	// Call pipeline_next_action at phase-2 which is skipped.
	// The handler MUST absorb the skip internally (P1) and return the next
	// actionable phase (phase-3 spawn_agent) directly — no done+skip: passthrough.
	result, err := callNextAction(t, nextActionH, workspace)
	if err != nil {
		t.Fatalf("PipelineNextActionHandler (skip) returned Go error: %v", err)
	}
	if result.IsError {
		t.Fatalf("PipelineNextActionHandler (skip) returned MCP error: %s", textContent(result))
	}

	var action orchestrator.Action
	if err := json.Unmarshal([]byte(textContent(result)), &action); err != nil {
		t.Fatalf("unmarshal action: %v (raw: %s)", err, textContent(result))
	}

	// Assert: the returned action is NOT a skip signal (P1 absorbs it internally).
	if action.Type == orchestrator.ActionDone && strings.HasPrefix(action.Summary, orchestrator.SkipSummaryPrefix) {
		t.Fatalf("handler returned skip signal to caller (type=%q, summary=%q); P1 should absorb this",
			action.Type, action.Summary)
	}

	// Assert: the returned action is for phase-3 (first non-skipped phase after phase-2).
	if action.Type != orchestrator.ActionSpawnAgent {
		t.Errorf("action.Type = %q, want %q", action.Type, orchestrator.ActionSpawnAgent)
	}
	if action.Phase != orchestrator.PhaseThree {
		t.Errorf("action.Phase = %q, want %q", action.Phase, orchestrator.PhaseThree)
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
	nextActionH := PipelineNextActionHandler(sm, eng, "", nil, nil, nil)
	reportResultH := PipelineReportResultHandler(state.NewStateManager("dev"), history.NewKnowledgeBase(""))

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

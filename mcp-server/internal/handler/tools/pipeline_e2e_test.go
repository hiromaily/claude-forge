// Package tools — end-to-end integration tests for the full pipeline flow.
// These tests drive PipelineNextActionHandler → mock artifact writes →
// PipelineReportResultHandler through every phase until ActionDone, using
// a real state.json in a temp directory and no external services.
package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hiromaily/claude-forge/mcp-server/pkg/events"
	"github.com/hiromaily/claude-forge/mcp-server/internal/intelligence/history"
	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/orchestrator"
	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/state"
)

// e2eConfig holds per-test pipeline configuration for E2E tests.
type e2eConfig struct {
	effort              string
	template            string
	reviewDesignVerdict string
	autoApprove         bool
	skipPR              bool
	onAction            func(t *testing.T, action orchestrator.Action, s *state.State)
}

// setupE2EWorkspace initialises a workspace with the given config and returns
// handler closures for pipeline_next_action and pipeline_report_result.
func setupE2EWorkspace(
	t *testing.T,
	cfg e2eConfig,
) (workspace string, nextActionH server.ToolHandlerFunc, reportResultH server.ToolHandlerFunc) {
	t.Helper()

	dir := t.TempDir()

	sm := state.NewStateManager("dev")
	if err := sm.Init(dir, "e2e-test"); err != nil {
		t.Fatalf("sm.Init: %v", err)
	}
	if err := sm.Configure(dir, state.PipelineConfig{
		Effort:        cfg.effort,
		FlowTemplate:  cfg.template,
		AutoApprove:   cfg.autoApprove,
		SkipPR:        cfg.skipPR,
		SkippedPhases: orchestrator.SkipsForTemplate(cfg.template),
	}); err != nil {
		t.Fatalf("sm.Configure: %v", err)
	}
	if err := sm.Update(func(s *state.State) error {
		s.BranchClassified = true
		return nil
	}); err != nil {
		t.Fatalf("sm.Update (BranchClassified): %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, state.ArtifactRequest),
		[]byte("# Request\n\ntest task\n"),
		0o600,
	); err != nil {
		t.Fatalf("write request.md: %v", err)
	}

	eng := orchestrator.NewEngine("", "")
	kb := history.NewKnowledgeBase("")
	nextActionH = PipelineNextActionHandler(sm, events.NewEventBus(), eng, "", nil, kb, nil)
	reportResultH = PipelineReportResultHandler(sm, events.NewEventBus(), kb)

	return dir, nextActionH, reportResultH
}

// mockAgentExecute writes the minimum artifact content satisfying artifact validation
// for the given action phase. When approveOverride is non-nil and true, phase-3b
// always writes an APPROVE verdict regardless of cfg.reviewDesignVerdict.
func mockAgentExecute(
	t *testing.T,
	workspace string,
	action orchestrator.Action,
	cfg e2eConfig,
	approveOverride *bool,
) {
	t.Helper()

	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(workspace, name), []byte(content), 0o600); err != nil {
			t.Fatalf("mockAgentExecute write %s: %v", name, err)
		}
	}

	switch action.Phase {
	case state.PhaseOne:
		write(state.ArtifactAnalysis, "# Analysis\n\nSituation analysis.\n")
	case state.PhaseTwo:
		write(state.ArtifactInvestigation, "# Investigation\n\nFindings.\n")
	case state.PhaseThree:
		write(state.ArtifactDesign, "# Design\n\n## Approach\n\nDetails.\n")
		// Remove the previous review-design.md so handlePhaseThreeB dispatches
		// the design reviewer (not the architect again) on the next phase-3b call.
		_ = os.Remove(filepath.Join(workspace, state.ArtifactReviewDesign))
	case state.PhaseThreeB:
		switch {
		case approveOverride != nil && *approveOverride:
			write(state.ArtifactReviewDesign, "# Review\n\n## Verdict: APPROVE\n")
		case cfg.reviewDesignVerdict == "" || cfg.reviewDesignVerdict == "APPROVE":
			write(state.ArtifactReviewDesign, "# Review\n\n## Verdict: APPROVE\n")
		default:
			write(state.ArtifactReviewDesign,
				"# Review\n\n## Verdict: REVISE\n\n### Findings\n\n**1. [CRITICAL] Design flaw.**\n")
		}
	case state.PhaseFour:
		write(state.ArtifactTasks, "# Tasks\n\n## Task 1: Implement\n\nApply design.\n\nmode: sequential\n")
	case state.PhaseFourB:
		write(state.ArtifactReviewTasks, "# Review\n\n## Verdict: APPROVE\n")
	case state.PhaseFive:
		write("impl-1.md", "# Implementation\n\nDone.\n")
	case state.PhaseSix:
		write("review-1.md", "# Review\n\nPASS\n")
	case state.PhaseSeven:
		write(state.ArtifactComprehensiveReview, "# Comprehensive Review\n\nAll good.\n")
	case state.PhaseFinalVerification:
		write(state.ArtifactFinalVerification, "# Final Verification\n\nPassed.\n")
	case state.PhaseFinalSummary:
		write(state.ArtifactSummary, "# Summary\n\nCompleted.\n")
	default:
		t.Logf("mockAgentExecute: no artifact rule for phase %q; skipping", action.Phase)
	}
}

// runE2EPipeline drives the full pipeline loop until ActionDone or 60 iterations.
// Returns the collected actions (including the final ActionDone) and whether a
// revision cycle was detected (phase-3b returned revision_required).
func runE2EPipeline(
	t *testing.T,
	cfg e2eConfig,
	workspace string,
	nextActionH server.ToolHandlerFunc,
	reportResultH server.ToolHandlerFunc,
) (actions []orchestrator.Action, revisionCycleDetected bool) {
	t.Helper()

	approveOverride := new(bool) // *bool pointing to false
	revisionCycleDetected = false

	for range 60 {
		result, err := callNextAction(t, nextActionH, workspace)
		if err != nil {
			t.Fatalf("runE2EPipeline: callNextAction returned Go error: %v", err)
		}
		if result.IsError {
			t.Fatalf("runE2EPipeline: callNextAction returned MCP error: %s", textContent(result))
		}

		var resp nextActionResponse
		if err := json.Unmarshal([]byte(textContent(result)), &resp); err != nil {
			t.Fatalf("runE2EPipeline: unmarshal nextActionResponse: %v (raw: %s)", err, textContent(result))
		}

		actions = append(actions, resp.Action)

		if resp.Action.Type == orchestrator.ActionDone {
			return actions, revisionCycleDetected
		}

		// Determine the phase to report. For checkpoint actions, the phase is
		// stored in Action.Name (e.g. "checkpoint-a"), not Action.Phase (which is empty).
		reportPhase := resp.Action.Phase
		if resp.Action.Type == orchestrator.ActionCheckpoint && reportPhase == "" {
			reportPhase = resp.Action.Name
		}

		switch resp.Action.Type {
		case orchestrator.ActionWriteFile:
			if err := os.WriteFile(resp.Action.Path, []byte(resp.Action.Content), 0o600); err != nil {
				t.Fatalf("runE2EPipeline: write_file %s: %v", resp.Action.Path, err)
			}
		case orchestrator.ActionSpawnAgent:
			mockAgentExecute(t, workspace, resp.Action, cfg, approveOverride)
		case orchestrator.ActionCheckpoint:
			// no artifact write needed; reportResult call below advances state
		case orchestrator.ActionExec:
			// no mock artifact write needed; reportResult call below records phase-log
		default:
			t.Fatalf("runE2EPipeline: unhandled action type %q for phase %q", resp.Action.Type, resp.Action.Phase)
		}

		// Single post-switch reportResult call for ALL action types.
		reportRes := callTool(t, reportResultH, map[string]any{
			"workspace":   workspace,
			"phase":       reportPhase,
			"tokens_used": 500,
			"duration_ms": 1000,
			"model":       "sonnet",
		})
		if reportRes.IsError {
			t.Fatalf("runE2EPipeline: callReportResult for phase %q returned MCP error: %s",
				resp.Action.Phase, textContent(reportRes))
		}

		// Detect revision_required for phase-3b and set approveOverride for next phase-3b call.
		if resp.Action.Phase == state.PhaseThreeB && !*approveOverride {
			rro := parsePRRResponse(t, textContent(reportRes))
			if rro.NextActionHint == "revision_required" {
				revisionCycleDetected = true
				*approveOverride = true
			}
		}
	}

	t.Fatalf("runE2EPipeline: pipeline did not reach ActionDone within 60 iterations")
	return nil, false // unreachable; satisfies compiler
}

// phaseLogSet returns a set of phase IDs that appear in s.PhaseLog.
// phaseLogSet returns phases that were actually executed (agent spawned, exec run,
// or checkpoint reported). Skip-only entries (model == "skipped") are excluded so
// the set reflects phases that ran, not phases that were merely logged as skipped.
func phaseLogSet(s *state.State) map[string]bool {
	set := make(map[string]bool, len(s.PhaseLog))
	for _, entry := range s.PhaseLog {
		if entry.Model == "skipped" {
			continue
		}
		set[entry.Phase] = true
	}
	return set
}

// TestE2E_Templates is a table-driven test covering three template variants.
// Each subtest drives the full pipeline to completion and asserts:
//   - currentPhase == completed
//   - phase-6 (Code Review) ran only for standard and full templates
//   - phase-7 (Comprehensive Review) ran for all templates
func TestE2E_Templates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		template      string
		effort        string
		wantPhase6Run bool // Code Review expected to run
		wantPhase7Run bool // Comprehensive Review expected to run
	}{
		{name: "standard_template", template: state.TemplateStandard, effort: state.EffortM, wantPhase6Run: true, wantPhase7Run: true},
		{name: "light_template", template: state.TemplateLight, effort: state.EffortS, wantPhase6Run: false, wantPhase7Run: true},
		{name: "full_template", template: state.TemplateFull, effort: state.EffortL, wantPhase6Run: true, wantPhase7Run: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := e2eConfig{effort: tc.effort, template: tc.template, autoApprove: true, skipPR: true}
			workspace, nextActionH, reportResultH := setupE2EWorkspace(t, cfg)
			_, _ = runE2EPipeline(t, cfg, workspace, nextActionH, reportResultH)

			s, err := state.ReadState(workspace)
			if err != nil {
				t.Fatalf("ReadState: %v", err)
			}
			if s.CurrentPhase != state.PhaseCompleted {
				t.Errorf("currentPhase = %q, want %q", s.CurrentPhase, state.PhaseCompleted)
			}

			logged := phaseLogSet(s)
			if got := logged[state.PhaseSix]; got != tc.wantPhase6Run {
				t.Errorf("phase-6 (Code Review) logged = %v, want %v (template=%s)", got, tc.wantPhase6Run, tc.template)
			}
			if got := logged[state.PhaseSeven]; got != tc.wantPhase7Run {
				t.Errorf("phase-7 (Comprehensive Review) logged = %v, want %v (template=%s)", got, tc.wantPhase7Run, tc.template)
			}
		})
	}
}

// TestE2E_DesignRevisionCycle verifies that a REVISE verdict on phase-3b triggers
// a revision cycle, increments DesignRevisions to 1, and the pipeline still completes.
func TestE2E_DesignRevisionCycle(t *testing.T) {
	t.Parallel()
	cfg := e2eConfig{
		effort:              state.EffortM,
		template:            state.TemplateStandard,
		reviewDesignVerdict: "REVISE",
		autoApprove:         true,
		skipPR:              true,
	}
	workspace, nextActionH, reportResultH := setupE2EWorkspace(t, cfg)
	_, revisionDetected := runE2EPipeline(t, cfg, workspace, nextActionH, reportResultH)

	if !revisionDetected {
		t.Errorf("expected revision cycle to be detected, got revisionDetected=false")
	}

	s, err := state.ReadState(workspace)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if s.CurrentPhase != state.PhaseCompleted {
		t.Errorf("currentPhase = %q, want %q", s.CurrentPhase, state.PhaseCompleted)
	}
	if s.Revisions.DesignRevisions != 1 {
		t.Errorf("DesignRevisions = %d, want 1", s.Revisions.DesignRevisions)
	}
}

// TestE2E_CheckpointRevisionFlow exercises the P8 checkpoint revision flow end-to-end:
// 1. Pipeline runs through phase-1 → phase-2 → phase-3 → phase-3b → checkpoint-a
// 2. User responds with "revise" → pipeline rewinds to phase-3
// 3. Architect runs again, design reviewer runs again → checkpoint-a reached again
// 4. User responds with "proceed" → pipeline continues to phase-4 and beyond
// 5. Pipeline completes successfully.
func TestE2E_CheckpointRevisionFlow(t *testing.T) {
	t.Parallel()

	cfg := e2eConfig{
		effort:      state.EffortM,
		template:    state.TemplateStandard,
		autoApprove: false,
		skipPR:      true,
	}
	workspace, nextActionH, reportResultH := setupE2EWorkspace(t, cfg)

	// Track how many times checkpoint-a returned an ActionCheckpoint.
	checkpointACount := 0
	// Track observed phases to verify the revision cycle occurred.
	var phaseSequence []string
	// pendingCheckpoint is set when a checkpoint action is returned; on the next
	// iteration the test sends user_response instead of calling reportResult.
	pendingCheckpoint := ""

	for range 60 {
		var result *mcp.CallToolResult
		var err error

		// If a checkpoint is pending from the previous iteration, respond to it
		// via user_response instead of doing a normal callNextAction.
		switch {
		case pendingCheckpoint == state.PhaseCheckpointA:
			checkpointACount++
			if checkpointACount == 1 {
				// First time at checkpoint-a: respond "revise" to trigger rewind.
				result, err = callNextActionWithUserResponse(t, nextActionH, workspace, "revise")
			} else {
				// Second time at checkpoint-a: respond "proceed" to advance.
				result, err = callNextActionWithUserResponse(t, nextActionH, workspace, "proceed")
			}
			pendingCheckpoint = ""
		case pendingCheckpoint != "":
			// For other checkpoints (checkpoint-b), just proceed.
			result, err = callNextActionWithUserResponse(t, nextActionH, workspace, "proceed")
			pendingCheckpoint = ""
		default:
			result, err = callNextAction(t, nextActionH, workspace)
		}

		if err != nil {
			t.Fatalf("callNextAction returned Go error: %v", err)
		}
		if result.IsError {
			t.Fatalf("callNextAction returned MCP error: %s", textContent(result))
		}

		var resp nextActionResponse
		if err := json.Unmarshal([]byte(textContent(result)), &resp); err != nil {
			t.Fatalf("unmarshal nextActionResponse: %v (raw: %s)", err, textContent(result))
		}

		if resp.Action.Type == orchestrator.ActionDone {
			break
		}

		reportPhase := resp.Action.Phase
		if resp.Action.Type == orchestrator.ActionCheckpoint && reportPhase == "" {
			reportPhase = resp.Action.Name
		}
		phaseSequence = append(phaseSequence, reportPhase)

		// If this is a checkpoint action, set pendingCheckpoint and skip reportResult.
		// The P8 handler in pipeline_next_action owns the checkpoint lifecycle;
		// the test must NOT call reportResult for checkpoints — instead, on the next
		// iteration it sends user_response.
		if resp.Action.Type == orchestrator.ActionCheckpoint {
			pendingCheckpoint = reportPhase
			continue
		}

		switch resp.Action.Type {
		case orchestrator.ActionWriteFile:
			if err := os.WriteFile(resp.Action.Path, []byte(resp.Action.Content), 0o600); err != nil {
				t.Fatalf("write_file %s: %v", resp.Action.Path, err)
			}
		case orchestrator.ActionSpawnAgent:
			// Always write APPROVE verdicts — we don't want phase-3b revision_required
			// in this test (that's the old auto-revision path). The checkpoint revision
			// is driven by user_response="revise" at checkpoint-a.
			alwaysApprove := new(bool)
			*alwaysApprove = true
			mockAgentExecute(t, workspace, resp.Action, cfg, alwaysApprove)
		case orchestrator.ActionExec:
			// No mock artifact write needed.
		default:
			t.Fatalf("unhandled action type %q for phase %q", resp.Action.Type, resp.Action.Phase)
		}

		// Report result to advance state (for non-checkpoint actions).
		reportRes := callTool(t, reportResultH, map[string]any{
			"workspace":   workspace,
			"phase":       reportPhase,
			"tokens_used": 500,
			"duration_ms": 1000,
			"model":       "sonnet",
		})
		if reportRes.IsError {
			t.Fatalf("callReportResult for phase %q returned MCP error: %s",
				reportPhase, textContent(reportRes))
		}
	}

	// Verify checkpoint-a was reached exactly twice (once before revise, once after).
	if checkpointACount != 2 {
		t.Errorf("checkpoint-a was reached %d times, want 2", checkpointACount)
	}

	// Verify the phase sequence shows phase-3 appearing at least twice
	// (first run + revision run).
	phase3Count := 0
	for _, p := range phaseSequence {
		if p == state.PhaseThree {
			phase3Count++
		}
	}
	if phase3Count < 2 {
		t.Errorf("phase-3 appeared %d times in sequence, want >= 2 (revision should re-run it); sequence: %v",
			phase3Count, phaseSequence)
	}

	// Verify pipeline completed successfully.
	finalState, err := state.ReadState(workspace)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if finalState.CurrentPhase != state.PhaseCompleted {
		t.Errorf("currentPhase = %q, want %q", finalState.CurrentPhase, state.PhaseCompleted)
	}
}

// TestE2E_ActionSequenceComplete verifies that each template produces a minimum
// number of actions (catches silent skips), that the last action is ActionDone,
// and that no spawn_agent phase appears more than once outside revision cycles.
func TestE2E_ActionSequenceComplete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		template       string
		effort         string
		wantMinActions int
	}{
		{name: "standard", template: state.TemplateStandard, effort: state.EffortM, wantMinActions: 10},
		{name: "light", template: state.TemplateLight, effort: state.EffortS, wantMinActions: 6},
		{name: "full", template: state.TemplateFull, effort: state.EffortL, wantMinActions: 12},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := e2eConfig{effort: tc.effort, template: tc.template, autoApprove: true, skipPR: true}
			workspace, nextActionH, reportResultH := setupE2EWorkspace(t, cfg)
			actions, _ := runE2EPipeline(t, cfg, workspace, nextActionH, reportResultH)

			t.Logf("template=%s action_count=%d", tc.template, len(actions))
			for i, a := range actions {
				phase := a.Phase
				if phase == "" {
					phase = a.Name
				}
				t.Logf("  [%d] %s:%s", i, a.Type, phase)
			}

			if len(actions) < tc.wantMinActions {
				var phases []string
				for _, a := range actions {
					phase := a.Phase
					if phase == "" {
						phase = a.Name
					}
					phases = append(phases, a.Type+":"+phase)
				}
				t.Errorf("action count = %d, want >= %d; sequence: %v",
					len(actions), tc.wantMinActions, phases)
			}

			// Last action must be ActionDone.
			last := actions[len(actions)-1]
			if last.Type != orchestrator.ActionDone {
				t.Errorf("last action type = %q, want %q", last.Type, orchestrator.ActionDone)
			}

			// No duplicate spawn_agent phases (phase-3/3b may repeat in revision cycles).
			spawnPhases := make(map[string]int)
			for _, a := range actions {
				if a.Type == orchestrator.ActionSpawnAgent {
					spawnPhases[a.Phase]++
				}
			}
			for phase, count := range spawnPhases {
				if phase != state.PhaseThree && phase != state.PhaseThreeB && count > 1 {
					t.Errorf("spawn_agent for phase %q dispatched %d times, want 1", phase, count)
				}
			}
		})
	}
}

// TestE2E_SkippedPhasesInPhaseLog verifies that after the skip-logging fix,
// ALL phases (including skipped ones) have PhaseLog entries.
func TestE2E_SkippedPhasesInPhaseLog(t *testing.T) {
	t.Parallel()

	cfg := e2eConfig{effort: state.EffortS, template: state.TemplateLight, autoApprove: true, skipPR: true}
	workspace, nextActionH, reportResultH := setupE2EWorkspace(t, cfg)
	_, _ = runE2EPipeline(t, cfg, workspace, nextActionH, reportResultH)

	s, err := state.ReadState(workspace)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}

	// Build set of ALL phases that appear in PhaseLog (including skipped).
	allLogged := make(map[string]bool)
	for _, entry := range s.PhaseLog {
		allLogged[entry.Phase] = true
	}

	// Verify skipped phases appear in PhaseLog with model "skipped".
	expectedSkips := orchestrator.SkipsForTemplate(state.TemplateLight)
	for _, skip := range expectedSkips {
		if !allLogged[skip] {
			t.Errorf("skipped phase %q not found in PhaseLog; expected model=skipped entry", skip)
		}
	}

	// Verify ALL phases from phase-1 to final-commit have a PhaseLog entry.
	// setup and completed don't get entries.
	for _, phase := range orchestrator.AllPhases {
		if phase == state.PhaseSetup || phase == state.PhaseCompleted {
			continue
		}
		if !allLogged[phase] {
			t.Errorf("phase %q has no PhaseLog entry — action may have been silently consumed", phase)
		}
	}
}

func TestE2E_CheckpointPhaseLog(t *testing.T) {
	t.Parallel()

	cfg := e2eConfig{
		effort:      state.EffortM,
		template:    state.TemplateStandard,
		autoApprove: false,
		skipPR:      true,
	}
	workspace, nextActionH, reportResultH := setupE2EWorkspace(t, cfg)

	for range 60 {
		result, err := callNextAction(t, nextActionH, workspace)
		if err != nil {
			t.Fatalf("callNextAction: %v", err)
		}
		if result.IsError {
			t.Fatalf("MCP error: %s", textContent(result))
		}

		var resp nextActionResponse
		if err := json.Unmarshal([]byte(textContent(result)), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.Action.Type == orchestrator.ActionDone {
			break
		}

		reportPhase := resp.Action.Phase
		if resp.Action.Type == orchestrator.ActionCheckpoint && reportPhase == "" {
			reportPhase = resp.Action.Name
		}

		if resp.Action.Type == orchestrator.ActionCheckpoint {
			result, err = callNextActionWithUserResponse(t, nextActionH, workspace, "proceed")
			if err != nil {
				t.Fatalf("callNextActionWithUserResponse: %v", err)
			}
			if result.IsError {
				t.Fatalf("MCP error on proceed: %s", textContent(result))
			}
			if err := json.Unmarshal([]byte(textContent(result)), &resp); err != nil {
				t.Fatalf("unmarshal after proceed: %v", err)
			}
			if resp.Action.Type == orchestrator.ActionDone {
				break
			}
			reportPhase = resp.Action.Phase
			if resp.Action.Type == orchestrator.ActionCheckpoint && reportPhase == "" {
				reportPhase = resp.Action.Name
			}
		}

		switch resp.Action.Type {
		case orchestrator.ActionWriteFile:
			if err := os.WriteFile(resp.Action.Path, []byte(resp.Action.Content), 0o600); err != nil {
				t.Fatalf("write_file: %v", err)
			}
		case orchestrator.ActionSpawnAgent:
			approve := new(bool)
			*approve = true
			mockAgentExecute(t, workspace, resp.Action, cfg, approve)
		case orchestrator.ActionExec:
			// no-op
		}

		reportRes := callTool(t, reportResultH, map[string]any{
			"workspace":   workspace,
			"phase":       reportPhase,
			"tokens_used": 500,
			"duration_ms": 1000,
			"model":       "sonnet",
		})
		if reportRes.IsError {
			t.Fatalf("reportResult for %q: %s", reportPhase, textContent(reportRes))
		}
	}

	s, err := state.ReadState(workspace)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}

	logged := make(map[string]bool)
	for _, entry := range s.PhaseLog {
		logged[entry.Phase] = true
	}

	if !logged[state.PhaseCheckpointA] {
		t.Errorf("checkpoint-a not found in PhaseLog")
	}
	if !logged[state.PhaseCheckpointB] {
		t.Errorf("checkpoint-b not found in PhaseLog")
	}
}

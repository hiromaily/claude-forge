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

	"github.com/mark3labs/mcp-go/server"

	"github.com/hiromaily/claude-forge/mcp-server/internal/history"
	"github.com/hiromaily/claude-forge/mcp-server/internal/orchestrator"
	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
)

// e2eConfig holds per-test pipeline configuration for E2E tests.
type e2eConfig struct {
	effort              string // state.EffortM, state.EffortS, state.EffortL
	template            string // state.TemplateStandard, TemplateLight, TemplateFull
	reviewDesignVerdict string // verdict written to review-design.md on first phase-3b spawn; defaults to "APPROVE" if empty
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
		AutoApprove:   true,
		SkipPR:        true,
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
	nextActionH = PipelineNextActionHandler(sm, eng, "", nil, kb, nil)
	reportResultH = PipelineReportResultHandler(sm, kb)

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
// Returns true if a revision cycle was detected (phase-3b returned revision_required).
func runE2EPipeline(
	t *testing.T,
	cfg e2eConfig,
	workspace string,
	nextActionH server.ToolHandlerFunc,
	reportResultH server.ToolHandlerFunc,
) (revisionCycleDetected bool) {
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

		if resp.Action.Type == orchestrator.ActionDone {
			return revisionCycleDetected
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
	return false // unreachable; satisfies compiler
}

// TestE2E_Templates is a table-driven test covering three template variants.
// Each subtest drives the full pipeline to completion and asserts currentPhase == completed.
func TestE2E_Templates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		template string
		effort   string
	}{
		{name: "standard_template", template: state.TemplateStandard, effort: state.EffortM},
		{name: "light_template", template: state.TemplateLight, effort: state.EffortS},
		{name: "full_template", template: state.TemplateFull, effort: state.EffortL},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := e2eConfig{effort: tc.effort, template: tc.template}
			workspace, nextActionH, reportResultH := setupE2EWorkspace(t, cfg)
			runE2EPipeline(t, cfg, workspace, nextActionH, reportResultH)

			s, err := state.ReadState(workspace)
			if err != nil {
				t.Fatalf("ReadState: %v", err)
			}
			if s.CurrentPhase != state.PhaseCompleted {
				t.Errorf("currentPhase = %q, want %q", s.CurrentPhase, state.PhaseCompleted)
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
	}
	workspace, nextActionH, reportResultH := setupE2EWorkspace(t, cfg)
	revisionDetected := runE2EPipeline(t, cfg, workspace, nextActionH, reportResultH)

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

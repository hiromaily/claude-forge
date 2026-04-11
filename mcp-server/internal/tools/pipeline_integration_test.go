// Package tools — Go integration tests for pipeline round-trip flows.
// These tests exercise the full handler chain:
//
//	PipelineNextActionHandler → PipelineReportResultHandler
//
// using a real state.json in a temp directory.
// Also covers the full four-call --discuss mode sequence:
//
//	PipelineInitHandler → PipelineInitWithContextHandler (first) →
//	PipelineInitWithContextHandler (discussion) → PipelineInitWithContextHandler (confirmation)
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

// TestDiscussModeEndToEnd exercises the full four-call --discuss mode sequence:
//
//  1. PipelineInitHandler with "--discuss" input: assert flags.Discuss=true and
//     top-level core_text is the stripped task text.
//  2. First PipelineInitWithContextHandler call with discuss flag: assert
//     needs_discussion is non-null with three questions.
//  3. Discussion PipelineInitWithContextHandler call with discussion_answers: assert
//     needs_user_confirmation with non-empty enriched_request_body; no workspace created.
//  4. Confirmation PipelineInitWithContextHandler call with user_confirmation carrying
//     enriched_request_body: assert ready=true, request.md exists with correct content.
//  5. Error case: both discussion_answers and user_confirmation present returns error.
func TestDiscussModeEndToEnd(t *testing.T) {
	t.Parallel()

	const (
		taskInput         = "add login feature --discuss"
		taskText          = "add login feature"
		discussionAnswers = "Q1: definition of done\nQ2: no constraints\nQ3: use existing auth pkg"
	)

	sm := state.NewStateManager("dev")
	initH := PipelineInitHandler(sm)
	piwcH := PipelineInitWithContextHandler(sm)

	// ---- Step 1: pipeline_init with "--discuss" flag ----

	initRes := callTool(t, initH, map[string]any{
		"arguments": taskInput,
	})
	if initRes.IsError {
		t.Fatalf("step 1: PipelineInitHandler returned MCP error: %s", textContent(initRes))
	}

	var initResult PipelineInitResult
	if err := json.Unmarshal([]byte(textContent(initRes)), &initResult); err != nil {
		t.Fatalf("step 1: unmarshal PipelineInitResult: %v (raw: %s)", err, textContent(initRes))
	}

	if initResult.Flags == nil {
		t.Fatalf("step 1: flags is nil")
	}
	if !initResult.Flags.Discuss {
		t.Errorf("step 1: flags.Discuss = false, want true")
	}
	if initResult.CoreText != taskText {
		t.Errorf("step 1: core_text = %q, want %q", initResult.CoreText, taskText)
	}

	// Use a temp dir as the workspace root (PipelineInitHandler returns a .specs/-relative path;
	// we substitute a real temp dir so subsequent calls can create the workspace on disk).
	workspaceDir := t.TempDir()

	// ---- Step 2: first pipeline_init_with_context call (no discussion_answers, no user_confirmation) ----

	firstRes := callTool(t, piwcH, map[string]any{
		"workspace": workspaceDir,
		"flags":     map[string]any{"discuss": true},
		"task_text": taskText,
	})
	if firstRes.IsError {
		t.Fatalf("step 2: first call returned MCP error: %s", textContent(firstRes))
	}

	firstResult := parsePIWCResult(t, textContent(firstRes))

	if firstResult.NeedsDiscussion == nil {
		t.Fatalf("step 2: needs_discussion is nil, want non-null")
	}
	if firstResult.NeedsUserConfirmation != nil {
		t.Errorf("step 2: needs_user_confirmation should be nil when needs_discussion is returned")
	}
	if len(firstResult.NeedsDiscussion.Questions) != 3 {
		t.Errorf("step 2: needs_discussion.questions length = %d, want 3; questions = %v",
			len(firstResult.NeedsDiscussion.Questions), firstResult.NeedsDiscussion.Questions)
	}

	// ---- Step 3: discussion call (discussion_answers present, user_confirmation absent) ----

	discRes := callTool(t, piwcH, map[string]any{
		"workspace":          workspaceDir,
		"flags":              map[string]any{"discuss": true},
		"task_text":          taskText,
		"discussion_answers": discussionAnswers,
	})
	if discRes.IsError {
		t.Fatalf("step 3: discussion call returned MCP error: %s", textContent(discRes))
	}

	discResult := parsePIWCResult(t, textContent(discRes))

	if discResult.NeedsUserConfirmation == nil {
		t.Fatalf("step 3: needs_user_confirmation is nil, want non-null")
	}
	enrichedBody := discResult.NeedsUserConfirmation.EnrichedRequestBody
	if enrichedBody == "" {
		t.Fatalf("step 3: needs_user_confirmation.enriched_request_body is empty, want non-empty")
	}
	if !strings.Contains(enrichedBody, taskText) {
		t.Errorf("step 3: enriched_request_body does not contain task text %q; body: %s", taskText, enrichedBody)
	}
	if !strings.Contains(enrichedBody, discussionAnswers) {
		t.Errorf("step 3: enriched_request_body does not contain discussion answers; body: %s", enrichedBody)
	}

	// No workspace directory should have been created on disk by the discussion call.
	// workspaceDir itself was pre-created by t.TempDir(), but no state.json should exist.
	if _, err := os.Stat(filepath.Join(workspaceDir, "state.json")); err == nil {
		t.Errorf("step 3: state.json must not exist after discussion call (no filesystem writes)")
	}

	// ---- Step 4: confirmation call (user_confirmation with enriched_request_body) ----

	confirmRes := callTool(t, piwcH, map[string]any{
		"workspace": workspaceDir,
		"flags":     map[string]any{"discuss": true},
		"task_text": taskText,
		"user_confirmation": map[string]any{
			"effort":                "M",
			"use_current_branch":    true,
			"workspace_slug":        "test-discuss",
			"enriched_request_body": enrichedBody,
		},
	})
	if confirmRes.IsError {
		t.Fatalf("step 4: confirmation call returned MCP error: %s", textContent(confirmRes))
	}

	confirmResult := parsePIWCResult(t, textContent(confirmRes))
	if !confirmResult.Ready {
		t.Errorf("step 4: ready = false, want true")
	}

	// Determine actual workspace path — may be refined by workspace_slug.
	actualWorkspace := confirmResult.Workspace
	if actualWorkspace == "" {
		// Fall back to workspaceDir if result.Workspace is empty.
		actualWorkspace = workspaceDir
	}

	// Assertion 1: request.md exists.
	requestMDPath := filepath.Join(actualWorkspace, "request.md")
	requestMDBytes, err := os.ReadFile(requestMDPath)
	if err != nil {
		t.Fatalf("step 4 assertion 1: request.md does not exist at %s: %v", requestMDPath, err)
	}
	requestMD := string(requestMDBytes)

	// Assertion 2: front matter contains exactly "source_type: text".
	// Split into front matter and body.
	parts := strings.SplitN(requestMD, "---\n", 3)
	// parts[0] is empty (before first "---\n"), parts[1] is front matter, parts[2] is body.
	if len(parts) < 3 {
		t.Fatalf("step 4 assertion 2: request.md does not have expected front matter delimiters; content:\n%s", requestMD)
	}
	frontMatter := parts[1]
	body := parts[2]
	foundSourceType := false
	for _, line := range strings.Split(frontMatter, "\n") {
		if line == "source_type: text" {
			foundSourceType = true
			break
		}
	}
	if !foundSourceType {
		t.Errorf("step 4 assertion 2: front matter does not contain exact line \"source_type: text\"; front matter:\n%s", frontMatter)
	}

	// Assertion 3: body contains "## Discussion Answers" as a section header.
	if !strings.Contains(body, "## Discussion Answers") {
		t.Errorf("step 4 assertion 3: body does not contain \"## Discussion Answers\" section header; body:\n%s", body)
	}

	// Assertion 4: body contains the original task text.
	if !strings.Contains(body, taskText) {
		t.Errorf("step 4 assertion 4: body does not contain original task text %q; body:\n%s", taskText, body)
	}

	// Assertion 5: body contains the discussion answers.
	if !strings.Contains(body, discussionAnswers) {
		t.Errorf("step 4 assertion 5: body does not contain discussion answers; body:\n%s", body)
	}

	// ---- Step 5: error case — both discussion_answers and user_confirmation present ----

	errorRes := callTool(t, piwcH, map[string]any{
		"workspace":          workspaceDir,
		"flags":              map[string]any{"discuss": true},
		"task_text":          taskText,
		"discussion_answers": discussionAnswers,
		"user_confirmation": map[string]any{
			"effort":             "M",
			"use_current_branch": true,
		},
	})
	if !errorRes.IsError {
		t.Errorf("step 5: expected error when both discussion_answers and user_confirmation are present, got success")
	}
}

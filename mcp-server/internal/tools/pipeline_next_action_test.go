// Package tools — unit tests for PipelineNextActionHandler and enrichPrompt.
package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hiromaily/claude-forge/mcp-server/internal/orchestrator"
	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
)

// initWorkspaceForNextAction sets up a minimal workspace for pipeline_next_action tests.
func initWorkspaceForNextAction(t *testing.T, phase string, modify func(*state.State) error) (string, *state.StateManager) {
	t.Helper()
	dir := t.TempDir()
	sm := state.NewStateManager("dev")
	if err := sm.Init(dir, "test-spec"); err != nil {
		t.Fatalf("sm.Init: %v", err)
	}
	if phase != "" && phase != "phase-1" {
		if err := sm.Update(func(s *state.State) error {
			s.CurrentPhase = phase
			return nil
		}); err != nil {
			t.Fatalf("sm.Update (set phase): %v", err)
		}
	}
	if modify != nil {
		if err := sm.Update(modify); err != nil {
			t.Fatalf("sm.Update (modify): %v", err)
		}
	}
	return dir, sm
}

// callNextAction invokes PipelineNextActionHandler and returns the result.
func callNextAction(t *testing.T, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), workspace string) (*mcp.CallToolResult, error) {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"workspace": workspace,
	}
	return handler(t.Context(), req)
}

func TestPipelineNextAction(t *testing.T) {
	t.Parallel()

	t.Run("phase1_spawn", func(t *testing.T) {
		t.Parallel()
		workspace, sm := initWorkspaceForNextAction(t, "phase-1", nil)
		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, eng, "", nil, nil, nil)

		result, err := callNextAction(t, handler, workspace)
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if result.IsError {
			t.Fatalf("handler returned MCP error: %s", result.Content)
		}

		var action orchestrator.Action
		if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &action); err != nil {
			t.Fatalf("unmarshal action: %v", err)
		}
		if action.Type != orchestrator.ActionSpawnAgent {
			t.Errorf("action.Type = %q, want %q", action.Type, orchestrator.ActionSpawnAgent)
		}
		if action.Agent != "situation-analyst" {
			t.Errorf("action.Agent = %q, want %q", action.Agent, "situation-analyst")
		}
	})

	t.Run("phase2_spawn", func(t *testing.T) {
		t.Parallel()
		workspace, sm := initWorkspaceForNextAction(t, "phase-2", nil)
		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, eng, "", nil, nil, nil)

		result, err := callNextAction(t, handler, workspace)
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if result.IsError {
			t.Fatalf("handler returned MCP error: %s", result.Content)
		}

		var action orchestrator.Action
		if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &action); err != nil {
			t.Fatalf("unmarshal action: %v", err)
		}
		if action.Type != orchestrator.ActionSpawnAgent {
			t.Errorf("action.Type = %q, want %q", action.Type, orchestrator.ActionSpawnAgent)
		}
		if action.Agent != "investigator" {
			t.Errorf("action.Agent = %q, want %q", action.Agent, "investigator")
		}
	})

	t.Run("checkpoint_a", func(t *testing.T) {
		t.Parallel()
		workspace, sm := initWorkspaceForNextAction(t, "checkpoint-a", func(s *state.State) error {
			s.CurrentPhaseStatus = "awaiting_human"
			return nil
		})
		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, eng, "", nil, nil, nil)

		result, err := callNextAction(t, handler, workspace)
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if result.IsError {
			t.Fatalf("handler returned MCP error: %s", result.Content)
		}

		var action orchestrator.Action
		if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &action); err != nil {
			t.Fatalf("unmarshal action: %v", err)
		}
		if action.Type != orchestrator.ActionCheckpoint {
			t.Errorf("action.Type = %q, want %q", action.Type, orchestrator.ActionCheckpoint)
		}
	})

	t.Run("checkpoint_sets_awaiting_human", func(t *testing.T) {
		t.Parallel()
		// When pipeline_next_action returns a checkpoint action, it must immediately
		// set currentPhaseStatus = "awaiting_human" in state so the stop hook permits
		// session exit even if the orchestrator hasn't called checkpoint() yet.
		workspace, sm := initWorkspaceForNextAction(t, "checkpoint-b", nil)
		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, eng, "", nil, nil, nil)

		result, err := callNextAction(t, handler, workspace)
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if result.IsError {
			t.Fatalf("handler returned MCP error: %s", result.Content)
		}

		var action orchestrator.Action
		if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &action); err != nil {
			t.Fatalf("unmarshal action: %v", err)
		}
		if action.Type != orchestrator.ActionCheckpoint {
			t.Fatalf("action.Type = %q, want %q", action.Type, orchestrator.ActionCheckpoint)
		}

		after, err := loadState(workspace)
		if err != nil {
			t.Fatalf("loadState: %v", err)
		}
		if after.CurrentPhaseStatus != "awaiting_human" {
			t.Errorf("currentPhaseStatus after checkpoint action = %q, want %q", after.CurrentPhaseStatus, "awaiting_human")
		}
	})

	t.Run("skip_prefix_passthrough", func(t *testing.T) {
		t.Parallel()
		workspace, sm := initWorkspaceForNextAction(t, "phase-3b", func(s *state.State) error {
			s.SkippedPhases = []string{"phase-3b"}
			return nil
		})
		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, eng, "", nil, nil, nil)

		result, err := callNextAction(t, handler, workspace)
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if result.IsError {
			t.Fatalf("handler returned MCP error: %s", result.Content)
		}

		var action orchestrator.Action
		if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &action); err != nil {
			t.Fatalf("unmarshal action: %v", err)
		}
		if action.Type != orchestrator.ActionDone {
			t.Errorf("action.Type = %q, want %q", action.Type, orchestrator.ActionDone)
		}
		if !strings.HasPrefix(action.Summary, orchestrator.SkipSummaryPrefix) {
			t.Errorf("action.Summary = %q, want prefix %q", action.Summary, orchestrator.SkipSummaryPrefix)
		}
	})

	t.Run("prompt_enrichment", func(t *testing.T) {
		t.Parallel()
		workspace, sm := initWorkspaceForNextAction(t, "phase-1", nil)

		// Create a real agentDir with a situation-analyst.md file
		agentDir := t.TempDir()
		agentContent := "# Situation Analyst\nYou are a situation analyst agent."
		if err := os.WriteFile(filepath.Join(agentDir, "situation-analyst.md"), []byte(agentContent), 0o600); err != nil {
			t.Fatalf("write agent file: %v", err)
		}

		// Write input artifacts into the workspace (contents should NOT be inlined)
		artifactContent := "# Request\nThis is the test request."
		if err := os.WriteFile(filepath.Join(workspace, "request.md"), []byte(artifactContent), 0o600); err != nil {
			t.Fatalf("write request.md: %v", err)
		}

		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, eng, agentDir, nil, nil, nil)

		result, err := callNextAction(t, handler, workspace)
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if result.IsError {
			t.Fatalf("handler returned MCP error: %s", result.Content)
		}

		var action orchestrator.Action
		if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &action); err != nil {
			t.Fatalf("unmarshal action: %v", err)
		}
		if action.Type != orchestrator.ActionSpawnAgent {
			t.Fatalf("action.Type = %q, want %q", action.Type, orchestrator.ActionSpawnAgent)
		}

		// Agent instructions (Layer 1) must still be present.
		if !strings.Contains(action.Prompt, agentContent) {
			t.Errorf("Prompt does not contain agent file contents\nPrompt: %s", action.Prompt)
		}
		// Artifacts section must contain file paths, not inlined content.
		if !strings.Contains(action.Prompt, "## Input Artifacts") {
			t.Errorf("Prompt does not contain '## Input Artifacts'\nPrompt: %s", action.Prompt)
		}
		requestPath := filepath.Join(workspace, "request.md")
		if !strings.Contains(action.Prompt, requestPath) {
			t.Errorf("Prompt does not contain artifact path %q\nPrompt: %s", requestPath, action.Prompt)
		}
		// Artifact file content must NOT be inlined in the prompt.
		if strings.Contains(action.Prompt, artifactContent) {
			t.Errorf("Prompt should not inline artifact content, but found %q\nPrompt: %s", artifactContent, action.Prompt)
		}
	})

	t.Run("missing_agent_file", func(t *testing.T) {
		t.Parallel()
		workspace, sm := initWorkspaceForNextAction(t, "phase-1", nil)

		// agentDir exists but has no agent files
		agentDir := t.TempDir()

		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, eng, agentDir, nil, nil, nil)

		result, err := callNextAction(t, handler, workspace)
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		// Fail-open: no MCP error, just action returned with warning
		if result.IsError {
			t.Fatalf("handler should not return MCP error when agent file missing; got error: %s", result.Content)
		}

		var resp struct {
			orchestrator.Action
			Warning string `json:"warning,omitempty"`
		}
		if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if resp.Type != orchestrator.ActionSpawnAgent {
			t.Errorf("action.Type = %q, want %q", resp.Type, orchestrator.ActionSpawnAgent)
		}
		if resp.Warning == "" {
			t.Errorf("expected non-empty warning when agent file is missing, got empty warning")
		}
	})

	t.Run("workspace_not_found", func(t *testing.T) {
		t.Parallel()
		sm := state.NewStateManager("dev")
		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, eng, "", nil, nil, nil)

		result, err := callNextAction(t, handler, "/nonexistent/workspace/path")
		if err != nil {
			t.Fatalf("handler returned Go error: %v", err)
		}
		if !result.IsError {
			t.Errorf("handler should return MCP error for nonexistent workspace; got: %+v", result)
		}
	})
}

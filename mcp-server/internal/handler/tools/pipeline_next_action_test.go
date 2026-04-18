// Package tools — unit tests for PipelineNextActionHandler and enrichPrompt.
package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/orchestrator"
	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/state"
	"github.com/hiromaily/claude-forge/mcp-server/internal/intelligence/history"
	"github.com/hiromaily/claude-forge/mcp-server/pkg/events"
)

// runGitCmd runs a git command in the given directory and fails the test on error.
func runGitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

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
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, "", nil, nil, nil)

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
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, "", nil, nil, nil)

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
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, "", nil, nil, nil)

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
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, "", nil, nil, nil)

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

	t.Run("skip_absorption", func(t *testing.T) {
		// P1: verify that a skip signal is absorbed internally and the handler returns
		// the first non-skip action rather than returning done+skip: to the caller.
		// phase-3b is skipped; the next non-skipped phase is checkpoint-a → spawn_agent
		// (or checkpoint) depending on the engine's subsequent decision.
		t.Parallel()
		workspace, sm := initWorkspaceForNextAction(t, "phase-3b", func(s *state.State) error {
			s.SkippedPhases = []string{"phase-3b"}
			return nil
		})
		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, "", nil, nil, nil)

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
		// The handler must NOT return a done+skip: action to the caller (P1 absorption).
		if action.Type == orchestrator.ActionDone && strings.HasPrefix(action.Summary, orchestrator.SkipSummaryPrefix) {
			t.Errorf("handler returned skip signal to caller (action.Type=%q, Summary=%q); P1 should absorb this internally",
				action.Type, action.Summary)
		}
		// The action should be a non-skip, non-done type (spawn_agent or checkpoint).
		if action.Type == orchestrator.ActionDone && action.Summary == "" {
			t.Errorf("unexpected true done (empty summary) after skip absorption")
		}
	})

	t.Run("skip_phase_logged", func(t *testing.T) {
		// P1: verify that skipped phases get PhaseLog entries with Model == "skipped".
		// Before this fix, skipped phases appeared in CompletedPhases but not in PhaseLog,
		// making skip-related bugs invisible.
		t.Parallel()
		workspace, sm := initWorkspaceForNextAction(t, "phase-3b", func(s *state.State) error {
			s.SkippedPhases = []string{"phase-3b"}
			return nil
		})
		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, "", nil, nil, nil)

		result, err := callNextAction(t, handler, workspace)
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if result.IsError {
			t.Fatalf("handler returned MCP error: %s", result.Content)
		}

		// Verify PhaseLog contains an entry for the skipped phase.
		s, loadErr := loadState(workspace)
		if loadErr != nil {
			t.Fatalf("loadState: %v", loadErr)
		}
		found := false
		for _, entry := range s.PhaseLog {
			if entry.Phase == "phase-3b" && entry.Model == "skipped" {
				found = true
				if entry.Tokens != 0 {
					t.Errorf("skip PhaseLog entry tokens = %d, want 0", entry.Tokens)
				}
				if entry.DurationMs != 0 {
					t.Errorf("skip PhaseLog entry durationMs = %d, want 0", entry.DurationMs)
				}
				break
			}
		}
		if !found {
			t.Errorf("PhaseLog does not contain entry for skipped phase-3b with model=skipped; got %+v", s.PhaseLog)
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
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, agentDir, nil, nil, nil)

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

	t.Run("checkpoint_message_injected", func(t *testing.T) {
		t.Parallel()
		workspace, sm := initWorkspaceForNextAction(t, "phase-1", nil)

		agentDir := t.TempDir()
		agentContent := "# Situation Analyst\nYou are a situation analyst agent."
		if err := os.WriteFile(filepath.Join(agentDir, "situation-analyst.md"), []byte(agentContent), 0o600); err != nil {
			t.Fatalf("write agent file: %v", err)
		}

		// Write a checkpoint message file simulating dashboard approve with feedback.
		msgContent := "Focus on the auth module only."
		msgFile := filepath.Join(workspace, "checkpoint-message.txt")
		if err := os.WriteFile(msgFile, []byte(msgContent), 0o644); err != nil {
			t.Fatalf("write checkpoint-message.txt: %v", err)
		}

		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, agentDir, nil, nil, nil)

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
		// Human Feedback section must be present with the message content.
		if !strings.Contains(action.Prompt, "## Human Feedback") {
			t.Errorf("Prompt does not contain '## Human Feedback'\nPrompt: %s", action.Prompt)
		}
		if !strings.Contains(action.Prompt, msgContent) {
			t.Errorf("Prompt does not contain checkpoint message %q\nPrompt: %s", msgContent, action.Prompt)
		}
		// The file must be consumed (deleted) after injection.
		if _, statErr := os.Stat(msgFile); statErr == nil {
			t.Errorf("checkpoint-message.txt should be deleted after consumption")
		}
	})

	t.Run("no_checkpoint_message_no_feedback", func(t *testing.T) {
		t.Parallel()
		workspace, sm := initWorkspaceForNextAction(t, "phase-1", nil)

		agentDir := t.TempDir()
		agentContent := "# Situation Analyst\nYou are a situation analyst agent."
		if err := os.WriteFile(filepath.Join(agentDir, "situation-analyst.md"), []byte(agentContent), 0o600); err != nil {
			t.Fatalf("write agent file: %v", err)
		}

		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, agentDir, nil, nil, nil)

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
		// No checkpoint-message.txt → no Human Feedback section.
		if strings.Contains(action.Prompt, "## Human Feedback") {
			t.Errorf("Prompt should not contain '## Human Feedback' when no message file exists\nPrompt: %s", action.Prompt)
		}
	})

	t.Run("output_artifact_section", func(t *testing.T) {
		t.Parallel()
		workspace, sm := initWorkspaceForNextAction(t, "phase-1", nil)

		agentDir := t.TempDir()
		agentContent := "# Situation Analyst\nYou are a situation analyst agent."
		if err := os.WriteFile(filepath.Join(agentDir, "situation-analyst.md"), []byte(agentContent), 0o600); err != nil {
			t.Fatalf("write agent file: %v", err)
		}

		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, agentDir, nil, nil, nil)

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
		// Output Artifact section must be present since phase-1 has output_file = "analysis.md".
		if !strings.Contains(action.Prompt, "## Output Artifact") {
			t.Errorf("Prompt does not contain '## Output Artifact'\nPrompt: %s", action.Prompt)
		}
		expectedOutputPath := filepath.Join(workspace, "analysis.md")
		if !strings.Contains(action.Prompt, expectedOutputPath) {
			t.Errorf("Prompt does not contain output file path %q\nPrompt: %s", expectedOutputPath, action.Prompt)
		}
		if !strings.Contains(action.Prompt, "MANDATORY") {
			t.Errorf("Prompt does not contain 'MANDATORY' instruction\nPrompt: %s", action.Prompt)
		}
	})

	t.Run("missing_agent_file", func(t *testing.T) {
		t.Parallel()
		workspace, sm := initWorkspaceForNextAction(t, "phase-1", nil)

		// agentDir exists but has no agent files
		agentDir := t.TempDir()

		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, agentDir, nil, nil, nil)

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
		if resp.Action.Type != orchestrator.ActionSpawnAgent {
			t.Errorf("action.Type = %q, want %q", resp.Action.Type, orchestrator.ActionSpawnAgent)
		}
		if resp.Warning == "" {
			t.Errorf("expected non-empty warning when agent file is missing, got empty warning")
		}
	})

	t.Run("workspace_not_found", func(t *testing.T) {
		t.Parallel()
		sm := state.NewStateManager("dev")
		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, "", nil, nil, nil)

		result, err := callNextAction(t, handler, "/nonexistent/workspace/path")
		if err != nil {
			t.Fatalf("handler returned Go error: %v", err)
		}
		if !result.IsError {
			t.Errorf("handler should return MCP error for nonexistent workspace; got: %+v", result)
		}
	})

	t.Run("task_init_absorption", func(t *testing.T) {
		// P2: when the engine returns ActionTaskInit (st.Tasks is empty in phase-5),
		// the handler internally calls executeTaskInit and re-invokes eng.NextAction.
		// The result should be a spawn_agent action for the implementer (not ActionTaskInit).
		t.Parallel()

		workspace, sm := initWorkspaceForNextAction(t, "phase-5", nil)
		// workspace/tasks.md must exist for executeTaskInit to parse.
		tasksContent := "# Tasks\n\n## Task 1: Implement\n\nApply all changes.\n\nmode: sequential\n"
		if err := os.WriteFile(filepath.Join(workspace, "tasks.md"), []byte(tasksContent), 0o600); err != nil {
			t.Fatalf("write tasks.md: %v", err)
		}

		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, "", nil, nil, nil)

		result, err := callNextAction(t, handler, workspace)
		if err != nil {
			t.Fatalf("handler returned Go error: %v", err)
		}
		if result.IsError {
			t.Fatalf("handler returned MCP error: %s", result.Content)
		}

		var action orchestrator.Action
		if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &action); err != nil {
			t.Fatalf("unmarshal action: %v", err)
		}

		// The handler must NOT return ActionTaskInit to the caller (P2 absorption).
		if action.Type == orchestrator.ActionTaskInit {
			t.Errorf("handler returned ActionTaskInit to caller; P2 should absorb this internally")
		}
		// After task_init, the engine should dispatch the implementer (spawn_agent).
		if action.Type != orchestrator.ActionSpawnAgent {
			t.Errorf("action.Type = %q after task_init absorption, want %q", action.Type, orchestrator.ActionSpawnAgent)
		}
		if action.Agent != "implementer" {
			t.Errorf("action.Agent = %q after task_init absorption, want %q", action.Agent, "implementer")
		}

		// Confirm that tasks are now stored in state (executeTaskInit side effect).
		s, loadErr := loadState(workspace)
		if loadErr != nil {
			t.Fatalf("loadState after task_init_absorption: %v", loadErr)
		}
		if len(s.Tasks) == 0 {
			t.Errorf("state.Tasks is empty after task_init_absorption; expected tasks to be stored")
		}
	})

	t.Run("batch_commit_absorption", func(t *testing.T) {
		// P3: when NeedsBatchCommit=true, the handler internally calls executeBatchCommit,
		// clears the flag, and re-invokes eng.NextAction. The result is the next non-batch
		// action. Also verifies that state.NeedsBatchCommit == false on disk after the call.
		t.Parallel()

		// A real git repo is required because executeBatchCommit calls git commands.
		repoDir := t.TempDir()
		runGitCmd := func(args ...string) {
			t.Helper()
			cmd := exec.Command("git", args...)
			cmd.Dir = repoDir
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
		runGitCmd("init")
		runGitCmd("config", "user.email", "test@example.com")
		runGitCmd("config", "user.name", "Test")
		// Create initial commit so HEAD exists.
		if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("placeholder\n"), 0o600); err != nil {
			t.Fatalf("write README.md: %v", err)
		}
		runGitCmd("add", "README.md")
		runGitCmd("commit", "-m", "chore: initial commit")

		// Create workspace inside the git repo so repoRoot works.
		workspace := filepath.Join(repoDir, ".specs", "test-batch")
		if err := os.MkdirAll(workspace, 0o755); err != nil {
			t.Fatalf("mkdir workspace: %v", err)
		}

		sm := state.NewStateManager("dev")
		if err := sm.Init(workspace, "test-batch"); err != nil {
			t.Fatalf("sm.Init: %v", err)
		}

		// Set up phase-5 with tasks loaded (so engine skips task_init), with
		// NeedsBatchCommit=true (triggers ActionBatchCommit from engine).
		if err := sm.Update(func(s *state.State) error {
			s.CurrentPhase = state.PhaseFive
			s.Tasks = map[string]state.Task{
				"1": {
					Title:         "Task 1",
					ExecutionMode: state.ExecModeParallel,
					ImplStatus:    state.TaskStatusCompleted,
					ReviewStatus:  "pending",
					Files:         []string{}, // empty → executeBatchCommit uses git diff fallback (no-op warning)
				},
			}
			s.NeedsBatchCommit = true
			return nil
		}); err != nil {
			t.Fatalf("sm.Update: %v", err)
		}

		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, "", nil, nil, nil)

		result, err := callNextAction(t, handler, workspace)
		if err != nil {
			t.Fatalf("handler returned Go error: %v", err)
		}
		if result.IsError {
			t.Fatalf("handler returned MCP error: %s", result.Content)
		}

		var resp struct {
			orchestrator.Action
			Warning string `json:"warning,omitempty"`
		}
		if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}

		// The handler must NOT return ActionBatchCommit to the caller (P3 absorption).
		if resp.Action.Type == orchestrator.ActionBatchCommit {
			t.Errorf("handler returned ActionBatchCommit to caller; P3 should absorb this internally")
		}

		// Assert NeedsBatchCommit is false on disk after the call.
		s, loadErr := loadState(workspace)
		if loadErr != nil {
			t.Fatalf("loadState after batch_commit_absorption: %v", loadErr)
		}
		if s.NeedsBatchCommit {
			t.Errorf("state.NeedsBatchCommit = true after batch_commit_absorption; expected false")
		}
	})

	t.Run("final_commit_absorption", func(t *testing.T) {
		// P4: when the engine returns ActionExec with commands[0]=="final_commit",
		// the handler internally calls executeFinalCommit and returns ActionDone.
		// After the call, state.json must show currentPhase == "completed".
		t.Parallel()

		// Set up a git repo with a remote so git push --force-with-lease works.
		bareDir := t.TempDir()
		runBareGit := func(dir string, args ...string) {
			t.Helper()
			cmd := exec.Command("git", args...)
			cmd.Dir = dir
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
			}
		}

		// Create a bare remote repository.
		runBareGit(bareDir, "init", "--bare")

		// Clone from bare into a working repo.
		repoDir := t.TempDir()
		runBareGit(repoDir, "clone", bareDir, ".")
		runBareGit(repoDir, "config", "user.email", "test@example.com")
		runBareGit(repoDir, "config", "user.name", "Test")

		// Create initial commit and push to remote.
		if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("placeholder\n"), 0o600); err != nil {
			t.Fatalf("write README.md: %v", err)
		}
		runBareGit(repoDir, "add", "README.md")
		runBareGit(repoDir, "commit", "-m", "chore: initial commit")
		runBareGit(repoDir, "push", "origin", "HEAD")

		// Create workspace inside the repo.
		workspace := filepath.Join(repoDir, ".specs", "test-final-commit")
		if err := os.MkdirAll(workspace, 0o755); err != nil {
			t.Fatalf("mkdir workspace: %v", err)
		}

		sm := state.NewStateManager("dev")
		if err := sm.Init(workspace, "test-final-commit"); err != nil {
			t.Fatalf("sm.Init: %v", err)
		}

		// Set phase to final-commit with SkipPr=false so the engine returns
		// ActionExec with commands[0]=="final_commit".
		if err := sm.Update(func(s *state.State) error {
			s.CurrentPhase = state.PhaseFinalCommit
			s.SkipPr = false
			return nil
		}); err != nil {
			t.Fatalf("sm.Update: %v", err)
		}

		// Write a summary.md so executeFinalCommit's git add -f succeeds.
		summaryContent := "# Summary\n\nPipeline complete.\n"
		if err := os.WriteFile(filepath.Join(workspace, "summary.md"), []byte(summaryContent), 0o600); err != nil {
			t.Fatalf("write summary.md: %v", err)
		}

		kb := history.NewKnowledgeBase("")
		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, "", nil, kb, nil)

		result, err := callNextAction(t, handler, workspace)
		if err != nil {
			t.Fatalf("handler returned Go error: %v", err)
		}
		if result.IsError {
			t.Fatalf("handler returned MCP error: %s", result.Content)
		}

		var action orchestrator.Action
		if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &action); err != nil {
			t.Fatalf("unmarshal action: %v", err)
		}

		// P4 absorption: the handler must return ActionDone (not ActionExec with final_commit).
		if action.Type != orchestrator.ActionDone {
			t.Errorf("action.Type = %q after final_commit absorption, want %q", action.Type, orchestrator.ActionDone)
		}

		// State must be written as completed.
		s, loadErr := loadState(workspace)
		if loadErr != nil {
			t.Fatalf("loadState after final_commit_absorption: %v", loadErr)
		}
		if s.CurrentPhase != state.PhaseCompleted {
			t.Errorf("currentPhase = %q after final_commit_absorption, want %q", s.CurrentPhase, state.PhaseCompleted)
		}
	})

	t.Run("human_gate_returns_human_gate_type_and_sets_pending", func(t *testing.T) {
		t.Parallel()
		workspace, sm := initWorkspaceForNextAction(t, "phase-5", func(s *state.State) error {
			s.Tasks = map[string]state.Task{
				"1": {
					Title:         "Merge external PR",
					ExecutionMode: state.ExecModeHumanGate,
					ImplStatus:    "pending",
					ReviewStatus:  "pending",
				},
				"2": {
					Title:         "Update deps",
					ExecutionMode: state.ExecModeSequential,
					ImplStatus:    "pending",
					ReviewStatus:  "pending",
				},
			}
			return nil
		})
		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, "", nil, nil, nil)

		// First call: engine sees human_gate → handler returns human_gate action.
		result, err := callNextAction(t, handler, workspace)
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if result.IsError {
			t.Fatalf("handler returned MCP error: %s", result.Content)
		}

		var resp nextActionResponse
		if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.Action.Type != orchestrator.ActionHumanGate {
			t.Errorf("action.Type = %q, want %q", resp.Action.Type, orchestrator.ActionHumanGate)
		}
		if resp.Action.Name != "1" {
			t.Errorf("action.Name = %q, want %q", resp.Action.Name, "1")
		}

		// Verify PendingHumanGate is set in state.
		s, loadErr := loadState(workspace)
		if loadErr != nil {
			t.Fatalf("loadState: %v", loadErr)
		}
		if s.PendingHumanGate == nil || *s.PendingHumanGate != "1" {
			t.Errorf("PendingHumanGate = %v, want ptr to \"1\"", s.PendingHumanGate)
		}
		if s.CurrentPhaseStatus != state.StatusAwaitingHuman {
			t.Errorf("CurrentPhaseStatus = %q, want %q", s.CurrentPhaseStatus, state.StatusAwaitingHuman)
		}
	})

	t.Run("rename_branch_absorbed_and_state_updated", func(t *testing.T) {
		// When the engine returns ActionRenameBranch at checkpoint-a, the handler must:
		// 1. Execute git branch -m internally.
		// 2. Update state (Branch + BranchClassified).
		// 3. Re-enter the engine loop (the orchestrator never sees rename_branch).
		t.Parallel()

		branch := "feature/test-slug"
		workspace, sm := initWorkspaceForNextAction(t, "checkpoint-a", func(s *state.State) error {
			s.Branch = &branch
			s.BranchClassified = false
			return nil
		})

		// Set up a git repo with the feature/test-slug branch so git branch -m works.
		initGitRepo(t, workspace)
		runGitCmd(t, workspace, "checkout", "-b", "feature/test-slug")

		// Write design.md with fix-related content to trigger ClassifyBranchType → "fix".
		designContent := "# Design\n\nFix the broken endpoint that returns 500 errors.\n"
		if err := os.WriteFile(filepath.Join(workspace, "design.md"), []byte(designContent), 0o600); err != nil {
			t.Fatalf("write design.md: %v", err)
		}

		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, "", nil, nil, nil)

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

		// rename_branch must be absorbed — the returned action should be checkpoint.
		if action.Type == orchestrator.ActionRenameBranch {
			t.Fatalf("rename_branch was not absorbed — leaked to orchestrator")
		}
		if action.Type != orchestrator.ActionCheckpoint {
			t.Fatalf("action.Type = %q, want %q (checkpoint-a after absorbed rename)", action.Type, orchestrator.ActionCheckpoint)
		}

		// Verify state was updated: branch renamed and classified.
		after, loadErr := loadState(workspace)
		if loadErr != nil {
			t.Fatalf("loadState: %v", loadErr)
		}
		if after.Branch == nil {
			t.Fatalf("state.Branch is nil, want ptr to %q", "fix/test-slug")
		}
		if *after.Branch != "fix/test-slug" {
			t.Errorf("state.Branch = %q, want %q", *after.Branch, "fix/test-slug")
		}
		if !after.BranchClassified {
			t.Errorf("state.BranchClassified = false, want true")
		}
	})

	t.Run("human_gate_resolved_on_next_call", func(t *testing.T) {
		t.Parallel()
		taskKey := "1"
		workspace, sm := initWorkspaceForNextAction(t, "phase-5", func(s *state.State) error {
			s.PendingHumanGate = &taskKey
			s.CurrentPhaseStatus = state.StatusAwaitingHuman
			s.Tasks = map[string]state.Task{
				"1": {
					Title:         "Merge external PR",
					ExecutionMode: state.ExecModeHumanGate,
					ImplStatus:    "pending",
					ReviewStatus:  "pending",
				},
				"2": {
					Title:         "Update deps",
					ExecutionMode: state.ExecModeSequential,
					ImplStatus:    "pending",
					ReviewStatus:  "pending",
				},
			}
			return nil
		})
		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, "", nil, nil, nil)

		// Call pipeline_next_action: P0 should resolve the gate and advance.
		result, err := callNextAction(t, handler, workspace)
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if result.IsError {
			t.Fatalf("handler returned MCP error: %s", result.Content)
		}

		var resp nextActionResponse
		if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		// Should advance to task 2 (spawn implementer), not emit human_gate again.
		if resp.Action.Type != orchestrator.ActionSpawnAgent {
			t.Errorf("action.Type = %q, want %q (should advance past resolved gate)", resp.Action.Type, orchestrator.ActionSpawnAgent)
		}

		// Verify task 1 is completed and PendingHumanGate is cleared.
		s, loadErr := loadState(workspace)
		if loadErr != nil {
			t.Fatalf("loadState: %v", loadErr)
		}
		if s.PendingHumanGate != nil {
			t.Errorf("PendingHumanGate should be nil, got %v", s.PendingHumanGate)
		}
		task1 := s.Tasks["1"]
		if task1.ImplStatus != state.TaskStatusCompleted {
			t.Errorf("task 1 ImplStatus = %q, want %q", task1.ImplStatus, state.TaskStatusCompleted)
		}
		if task1.ReviewStatus != state.TaskStatusCompletedPass {
			t.Errorf("task 1 ReviewStatus = %q, want %q", task1.ReviewStatus, state.TaskStatusCompletedPass)
		}
	})
}

// callNextActionWithPrev invokes PipelineNextActionHandler with previous_* parameters set.
func callNextActionWithPrev(
	t *testing.T,
	handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error),
	workspace string,
	tokensUsed int,
	durationMs int,
	model string,
	setupOnly bool,
	actionComplete bool,
) (*mcp.CallToolResult, error) {
	t.Helper()
	args := map[string]any{
		"workspace":            workspace,
		"previous_tokens":      float64(tokensUsed),
		"previous_duration_ms": float64(durationMs),
		"previous_model":       model,
		"previous_setup_only":  setupOnly,
	}
	if actionComplete {
		args["previous_action_complete"] = true
	}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	return handler(t.Context(), req)
}

func TestPipelineNextAction_P5(t *testing.T) {
	t.Parallel()

	t.Run("proceed_variant_falls_through_to_next_action", func(t *testing.T) {
		// When previous_tokens > 0 and phase is phase-1 (proceed hint),
		// the P5 block should run reportResultCore, receive "proceed", fall through,
		// and return a real next action (spawn_agent for phase-2 investigator).
		t.Parallel()

		// Set up workspace in phase-1 (not a review phase, so outcome will be "proceed").
		workspace, sm := initWorkspaceForNextAction(t, "phase-1", nil)
		kb := history.NewKnowledgeBase("")
		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, "", nil, kb, nil)

		// Call with previous_tokens=500 to trigger P5 block.
		result, err := callNextActionWithPrev(t, handler, workspace, 500, 1000, "claude-sonnet-4-6", false, false)
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if result.IsError {
			t.Fatalf("handler returned MCP error: %s", result.Content)
		}

		var resp nextActionResponse
		if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		// The P5 block should have run reportResultCore (phase logged, phase-1 completed),
		// received "proceed", and fallen through to eng.NextAction — which should return
		// the spawn_agent action for phase-2 (investigator).
		if resp.Action.Type != orchestrator.ActionSpawnAgent {
			t.Errorf("action.Type = %q, want %q (proceed should fall through to NextAction)", resp.Action.Type, orchestrator.ActionSpawnAgent)
		}
		if resp.Action.Agent != "investigator" {
			t.Errorf("action.Agent = %q, want %q", resp.Action.Agent, "investigator")
		}
		// ReportResult should be nil when outcome is "proceed" (not surfaced to orchestrator).
		if resp.ReportResult != nil {
			t.Errorf("ReportResult should be nil for proceed outcome, got %+v", resp.ReportResult)
		}

		// Verify phase-1 was logged and completed in state.
		s, loadErr := loadState(workspace)
		if loadErr != nil {
			t.Fatalf("loadState: %v", loadErr)
		}
		if len(s.PhaseLog) == 0 {
			t.Errorf("PhaseLog should have at least one entry after P5 block ran")
		}
	})

	t.Run("previous_model_only_triggers_p5", func(t *testing.T) {
		// When previous_tokens == 0 but previous_model != "", P5 block should also trigger.
		t.Parallel()

		workspace, sm := initWorkspaceForNextAction(t, "phase-1", nil)
		kb := history.NewKnowledgeBase("")
		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, "", nil, kb, nil)

		// Use previous_model="" to confirm it does NOT trigger P5.
		// Then call with previous_model set to confirm it does.
		result, err := callNextActionWithPrev(t, handler, workspace, 0, 0, "claude-sonnet-4-6", false, false)
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if result.IsError {
			t.Fatalf("handler returned MCP error: %s", result.Content)
		}

		var resp nextActionResponse
		if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		// P5 triggered via model, phase-1 completed, should return phase-2 action.
		if resp.Action.Type != orchestrator.ActionSpawnAgent {
			t.Errorf("action.Type = %q, want %q", resp.Action.Type, orchestrator.ActionSpawnAgent)
		}
		if resp.Action.Agent != "investigator" {
			t.Errorf("action.Agent = %q, want %q", resp.Action.Agent, "investigator")
		}
	})

	t.Run("no_prev_params_skips_p5", func(t *testing.T) {
		// When both previous_tokens == 0 and previous_model == "", P5 block is skipped.
		// The handler proceeds directly to eng.NextAction and returns phase-1 action.
		t.Parallel()

		workspace, sm := initWorkspaceForNextAction(t, "phase-1", nil)
		kb := history.NewKnowledgeBase("")
		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, "", nil, kb, nil)

		// Call with no previous_* params (defaults to zero/empty).
		result, err := callNextAction(t, handler, workspace)
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if result.IsError {
			t.Fatalf("handler returned MCP error: %s", result.Content)
		}

		var resp nextActionResponse
		if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		// P5 skipped — still on phase-1, should return situation-analyst action.
		if resp.Action.Type != orchestrator.ActionSpawnAgent {
			t.Errorf("action.Type = %q, want %q", resp.Action.Type, orchestrator.ActionSpawnAgent)
		}
		if resp.Action.Agent != "situation-analyst" {
			t.Errorf("action.Agent = %q, want %q (P5 should be skipped, still on phase-1)", resp.Action.Agent, "situation-analyst")
		}

		// PhaseLog should be empty (P5 not triggered, no phase logged).
		s, loadErr := loadState(workspace)
		if loadErr != nil {
			t.Fatalf("loadState: %v", loadErr)
		}
		if len(s.PhaseLog) != 0 {
			t.Errorf("PhaseLog should be empty when P5 is skipped, got %d entries", len(s.PhaseLog))
		}
	})

	t.Run("revision_required_returns_early_no_next_action", func(t *testing.T) {
		// When phase is phase-3b with a REVISE verdict and previous_tokens > 0,
		// P5 block should call reportResultCore, receive "revision_required",
		// and return early WITHOUT calling eng.NextAction.
		// The response must have a non-nil ReportResult with correct fields.
		t.Parallel()

		workspace, sm := initWorkspaceForNextAction(t, "phase-3b", nil)
		// Write review-design.md with REVISE verdict and a [CRITICAL] finding.
		// ParseVerdict extracts findings from [CRITICAL] and [MINOR] labelled lines.
		// The format must match: "**N. [CRITICAL] <description>**"
		content := "# Design Review\n\n## Verdict: REVISE\n\n### Findings\n\n**1. [CRITICAL] The design is missing error handling for edge cases.**\n\nNeeds more work.\n"
		if err := os.WriteFile(filepath.Join(workspace, "review-design.md"), []byte(content), 0o600); err != nil {
			t.Fatalf("write review-design.md: %v", err)
		}

		kb := history.NewKnowledgeBase("")
		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, "", nil, kb, nil)

		result, err := callNextActionWithPrev(t, handler, workspace, 800, 2000, "claude-sonnet-4-6", false, false)
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if result.IsError {
			t.Fatalf("handler returned MCP error: %s", result.Content)
		}

		var resp nextActionResponse
		if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		// ReportResult must be non-nil and have correct NextActionHint.
		if resp.ReportResult == nil {
			t.Fatalf("ReportResult should be non-nil for revision_required outcome")
		}
		if resp.ReportResult.NextActionHint != "revision_required" {
			t.Errorf("ReportResult.NextActionHint = %q, want %q", resp.ReportResult.NextActionHint, "revision_required")
		}
		if resp.ReportResult.VerdictParsed != "REVISE" {
			t.Errorf("ReportResult.VerdictParsed = %q, want %q", resp.ReportResult.VerdictParsed, "REVISE")
		}
		// Findings must be non-empty for REVISE verdict.
		if len(resp.ReportResult.Findings) == 0 {
			t.Errorf("ReportResult.Findings should be non-empty for REVISE verdict")
		}

		// When revision_required, the handler returns early — action type should be zero value
		// (eng.NextAction was NOT called, so Action is default zero value).
		// The embedded action (from orchestrator.Action embedded in nextActionResponse) will be empty.
		if resp.Action.Type != "" {
			t.Errorf("Action.Type should be empty string (no NextAction call) for revision_required, got %q", resp.Action.Type)
		}

		// Verify state: DesignRevisions should be incremented.
		s, loadErr := loadState(workspace)
		if loadErr != nil {
			t.Fatalf("loadState: %v", loadErr)
		}
		if s.Revisions.DesignRevisions != 1 {
			t.Errorf("DesignRevisions = %d, want 1", s.Revisions.DesignRevisions)
		}
	})

	t.Run("setup_continue_falls_through_to_next_action", func(t *testing.T) {
		// When reportResultCore returns "setup_continue" (e.g. setup_only=true),
		// the P5 block should absorb the hint and fall through to eng.NextAction.
		t.Parallel()

		// Use phase-1 with setup_only=true to get setup_continue hint.
		workspace, sm := initWorkspaceForNextAction(t, "phase-1", nil)
		kb := history.NewKnowledgeBase("")
		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, "", nil, kb, nil)

		// setup_only=true should produce "setup_continue" from reportResultCore
		// for a non-review phase like phase-1.
		result, err := callNextActionWithPrev(t, handler, workspace, 100, 500, "claude-sonnet-4-6", true, false)
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if result.IsError {
			t.Fatalf("handler returned MCP error: %s", result.Content)
		}

		var resp nextActionResponse
		if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		// setup_continue is absorbed server-side; ReportResult should be nil.
		if resp.ReportResult != nil {
			t.Errorf("ReportResult should be nil for setup_continue outcome (absorbed internally)")
		}

		// Should fall through to eng.NextAction — still on phase-1, returns situation-analyst.
		if resp.Action.Type != orchestrator.ActionSpawnAgent {
			t.Errorf("action.Type = %q, want %q (setup_continue should fall through)", resp.Action.Type, orchestrator.ActionSpawnAgent)
		}
		if resp.Action.Agent != "situation-analyst" {
			t.Errorf("action.Agent = %q, want %q", resp.Action.Agent, "situation-analyst")
		}
	})

	t.Run("guard_skips_p5_for_setup_phase", func(t *testing.T) {
		// When CurrentPhase == "setup", the P5 guard should skip reportResultCore
		// even when previous_tokens > 0. The handler may still error from eng.NextAction
		// (setup is not a dispatchable phase), but PhaseLog must remain empty.
		t.Parallel()

		workspace, sm := initWorkspaceForNextAction(t, "setup", nil)
		kb := history.NewKnowledgeBase("")
		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, "", nil, kb, nil)

		result, err := callNextActionWithPrev(t, handler, workspace, 500, 1000, "claude-sonnet-4-6", false, false)
		if err != nil {
			t.Fatalf("handler returned Go error: %v", err)
		}
		// The handler may return an MCP error from eng.NextAction (setup is not dispatchable),
		// but if it does, it must NOT be a report_result error (P5 guard must have fired).
		if result.IsError {
			text := result.Content[0].(mcp.TextContent).Text
			if strings.Contains(text, "report_result:") {
				t.Errorf("error is from report_result path, P5 guard should have skipped it: %s", text)
			}
			// Error from eng.NextAction is expected — the guard fired correctly.
		}

		// PhaseLog should be empty — P5 guard skipped reportResultCore for "setup" phase.
		s, loadErr := loadState(workspace)
		if loadErr != nil {
			t.Fatalf("loadState: %v", loadErr)
		}
		if len(s.PhaseLog) != 0 {
			t.Errorf("PhaseLog should be empty when P5 guard skips 'setup' phase, got %d entries", len(s.PhaseLog))
		}
	})

	t.Run("guard_skips_p5_for_completed_phase", func(t *testing.T) {
		// When CurrentPhase == "completed", the P5 guard should skip reportResultCore.
		// The handler may succeed with a "done" action (completed is a valid terminal state).
		t.Parallel()

		workspace, sm := initWorkspaceForNextAction(t, "completed", nil)
		kb := history.NewKnowledgeBase("")
		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, "", nil, kb, nil)

		result, err := callNextActionWithPrev(t, handler, workspace, 500, 1000, "claude-sonnet-4-6", false, false)
		if err != nil {
			t.Fatalf("handler returned Go error: %v", err)
		}
		// If the handler errors, verify it's NOT a report_result error (P5 guard fired).
		if result.IsError {
			text := result.Content[0].(mcp.TextContent).Text
			if strings.Contains(text, "report_result:") {
				t.Errorf("error is from report_result path, P5 guard should have skipped it: %s", text)
			}
		}

		// PhaseLog should be empty — P5 guard skipped reportResultCore for "completed" phase.
		s, loadErr := loadState(workspace)
		if loadErr != nil {
			t.Fatalf("loadState: %v", loadErr)
		}
		if len(s.PhaseLog) != 0 {
			t.Errorf("PhaseLog should be empty when P5 guard skips 'completed' phase, got %d entries", len(s.PhaseLog))
		}
	})

	t.Run("action_complete_triggers_p5_for_exec_phase", func(t *testing.T) {
		// When previous_action_complete=true and tokens=0, model="", P5 block should still
		// trigger (exec/write_file actions complete without spending tokens).
		// This exercises the scenario where an exec or write_file phase completes with
		// durationMs=0 and tokensUsed=0 but the orchestrator correctly passes
		// previous_action_complete=true.
		t.Parallel()

		// Use phase-1 (non-review, non-setup) so reportResultCore returns "proceed".
		workspace, sm := initWorkspaceForNextAction(t, "phase-1", nil)
		kb := history.NewKnowledgeBase("")
		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, "", nil, kb, nil)

		// Call with actionComplete=true, tokens=0, model="" — P5 must fire via actionComplete flag.
		result, err := callNextActionWithPrev(t, handler, workspace, 0, 0, "", false, true)
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if result.IsError {
			t.Fatalf("handler returned MCP error: %s", result.Content)
		}

		var resp nextActionResponse
		if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		// P5 should have run reportResultCore (phase-1 logged and completed),
		// received "proceed", and fallen through to eng.NextAction — returning
		// the spawn_agent action for phase-2 (investigator).
		// This is not a stuck/infinite loop result (which would re-return phase-1 action).
		if resp.Action.Type != orchestrator.ActionSpawnAgent {
			t.Errorf("action.Type = %q, want %q (actionComplete=true should trigger P5 and advance phase)",
				resp.Action.Type, orchestrator.ActionSpawnAgent)
		}
		if resp.Action.Agent != "investigator" {
			t.Errorf("action.Agent = %q, want %q (should have advanced past phase-1)", resp.Action.Agent, "investigator")
		}
		// ReportResult should be nil when outcome is "proceed".
		if resp.ReportResult != nil {
			t.Errorf("ReportResult should be nil for proceed outcome, got %+v", resp.ReportResult)
		}

		// Verify phase-1 was logged and completed in state (P5 fired).
		s, loadErr := loadState(workspace)
		if loadErr != nil {
			t.Fatalf("loadState: %v", loadErr)
		}
		if len(s.PhaseLog) == 0 {
			t.Errorf("PhaseLog should have at least one entry after P5 fired via actionComplete=true")
		}
	})
}

// callNextActionWithUserResponse invokes PipelineNextActionHandler with user_response set.
func callNextActionWithUserResponse(
	t *testing.T,
	handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error),
	workspace string,
	userResponse string,
) (*mcp.CallToolResult, error) {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"workspace":     workspace,
		"user_response": userResponse,
	}
	return handler(t.Context(), req)
}

func TestPipelineNextAction_P8_CheckpointRevision(t *testing.T) {
	t.Parallel()

	t.Run("checkpoint_a_revise_rewinds_to_phase3", func(t *testing.T) {
		t.Parallel()

		// Set up workspace at checkpoint-a with phase-3b already completed.
		workspace, sm := initWorkspaceForNextAction(t, state.PhaseCheckpointA, func(s *state.State) error {
			s.CompletedPhases = []string{state.PhaseSetup, state.PhaseOne, state.PhaseTwo, state.PhaseThree, state.PhaseThreeB}
			s.CurrentPhaseStatus = state.StatusAwaitingHuman
			return nil
		})
		// Write design.md and review-design.md so they exist before revision.
		_ = os.WriteFile(filepath.Join(workspace, state.ArtifactDesign), []byte("# Design"), 0o600)
		_ = os.WriteFile(filepath.Join(workspace, state.ArtifactReviewDesign), []byte("## Verdict: APPROVE_WITH_NOTES\n\nfindings"), 0o600)
		_ = os.WriteFile(filepath.Join(workspace, state.ArtifactRequest), []byte("# Request"), 0o600)
		_ = os.WriteFile(filepath.Join(workspace, state.ArtifactAnalysis), []byte("# Analysis"), 0o600)

		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, "", nil, nil, nil)

		// Call with user_response="revise" — should rewind to phase-3.
		result, err := callNextActionWithUserResponse(t, handler, workspace, "revise")
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if result.IsError {
			t.Fatalf("handler returned MCP error: %s", result.Content)
		}

		var resp nextActionResponse
		if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		// Should return spawn_agent for the architect (phase-3).
		if resp.Action.Type != orchestrator.ActionSpawnAgent {
			t.Errorf("action.Type = %q, want %q", resp.Action.Type, orchestrator.ActionSpawnAgent)
		}
		if resp.Action.Agent != "architect" {
			t.Errorf("action.Agent = %q, want %q", resp.Action.Agent, "architect")
		}
		if resp.Action.Phase != state.PhaseThree {
			t.Errorf("action.Phase = %q, want %q", resp.Action.Phase, state.PhaseThree)
		}

		// Verify state was rewound to phase-3.
		s, loadErr := loadState(workspace)
		if loadErr != nil {
			t.Fatalf("loadState: %v", loadErr)
		}
		if s.CurrentPhase != state.PhaseThree {
			t.Errorf("CurrentPhase = %q, want %q", s.CurrentPhase, state.PhaseThree)
		}

		// review-design.md should be deleted — P8 removes it on rewind to prevent
		// the stale verdict from causing an infinite REVISE loop when phase-3b
		// re-enters. CompletedPhases is also cleaned so the architect is not
		// dispatched in "revision" mode (which would try to read the deleted file).
		if _, err := os.Stat(filepath.Join(workspace, state.ArtifactReviewDesign)); err == nil {
			t.Errorf("review-design.md should be deleted after checkpoint-a revision (prevents stale verdict loop)")
		}

		// Verify phase-3b was removed from CompletedPhases.
		for _, p := range s.CompletedPhases {
			if p == state.PhaseThreeB {
				t.Errorf("CompletedPhases should not contain %q after checkpoint-a revision", state.PhaseThreeB)
			}
		}
	})

	t.Run("checkpoint_b_revise_rewinds_to_phase4", func(t *testing.T) {
		t.Parallel()

		// Set up workspace at checkpoint-b with phase-4b already completed.
		workspace, sm := initWorkspaceForNextAction(t, state.PhaseCheckpointB, func(s *state.State) error {
			s.CompletedPhases = []string{
				state.PhaseSetup, state.PhaseOne, state.PhaseTwo,
				state.PhaseThree, state.PhaseThreeB, state.PhaseCheckpointA,
				state.PhaseFour, state.PhaseFourB,
			}
			s.CurrentPhaseStatus = state.StatusAwaitingHuman
			return nil
		})
		_ = os.WriteFile(filepath.Join(workspace, state.ArtifactDesign), []byte("# Design"), 0o600)
		_ = os.WriteFile(filepath.Join(workspace, state.ArtifactTasks), []byte("# Tasks\n\n## Task 1: Implement\nmode: sequential\n"), 0o600)
		_ = os.WriteFile(filepath.Join(workspace, state.ArtifactReviewTasks), []byte("## Verdict: APPROVE\n"), 0o600)

		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, "", nil, nil, nil)

		result, err := callNextActionWithUserResponse(t, handler, workspace, "revise")
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if result.IsError {
			t.Fatalf("handler returned MCP error: %s", result.Content)
		}

		var resp nextActionResponse
		if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		// Should return spawn_agent for the task-decomposer (phase-4).
		if resp.Action.Type != orchestrator.ActionSpawnAgent {
			t.Errorf("action.Type = %q, want %q", resp.Action.Type, orchestrator.ActionSpawnAgent)
		}
		if resp.Action.Agent != "task-decomposer" {
			t.Errorf("action.Agent = %q, want %q", resp.Action.Agent, "task-decomposer")
		}
		if resp.Action.Phase != state.PhaseFour {
			t.Errorf("action.Phase = %q, want %q", resp.Action.Phase, state.PhaseFour)
		}

		// Verify state was rewound to phase-4.
		s, loadErr := loadState(workspace)
		if loadErr != nil {
			t.Fatalf("loadState: %v", loadErr)
		}
		if s.CurrentPhase != state.PhaseFour {
			t.Errorf("CurrentPhase = %q, want %q", s.CurrentPhase, state.PhaseFour)
		}

		// review-tasks.md should be deleted — P8 removes it on rewind to prevent
		// the stale verdict from causing an infinite REVISE loop when phase-4b
		// re-enters. CompletedPhases is also cleaned so the task-decomposer is
		// not dispatched in "revision" mode (which would try to read the deleted file).
		if _, err := os.Stat(filepath.Join(workspace, state.ArtifactReviewTasks)); err == nil {
			t.Errorf("review-tasks.md should be deleted after checkpoint-b revision (prevents stale verdict loop)")
		}

		// Verify phase-4b was removed from CompletedPhases.
		for _, p := range s.CompletedPhases {
			if p == state.PhaseFourB {
				t.Errorf("CompletedPhases should not contain %q after checkpoint-b revision", state.PhaseFourB)
			}
		}
	})

	t.Run("checkpoint_a_proceed_advances_phase", func(t *testing.T) {
		t.Parallel()

		workspace, sm := initWorkspaceForNextAction(t, state.PhaseCheckpointA, func(s *state.State) error {
			s.CompletedPhases = []string{state.PhaseSetup, state.PhaseOne, state.PhaseTwo, state.PhaseThree, state.PhaseThreeB}
			s.CurrentPhaseStatus = state.StatusAwaitingHuman
			return nil
		})
		_ = os.WriteFile(filepath.Join(workspace, state.ArtifactDesign), []byte("# Design"), 0o600)

		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, "", nil, nil, nil)

		result, err := callNextActionWithUserResponse(t, handler, workspace, "proceed")
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if result.IsError {
			t.Fatalf("handler returned MCP error: %s", result.Content)
		}

		var resp nextActionResponse
		if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		// Should advance to phase-4 (task-decomposer).
		if resp.Action.Type != orchestrator.ActionSpawnAgent {
			t.Errorf("action.Type = %q, want %q", resp.Action.Type, orchestrator.ActionSpawnAgent)
		}
		if resp.Action.Agent != "task-decomposer" {
			t.Errorf("action.Agent = %q, want %q", resp.Action.Agent, "task-decomposer")
		}

		s, loadErr := loadState(workspace)
		if loadErr != nil {
			t.Fatalf("loadState: %v", loadErr)
		}
		if s.CurrentPhase != state.PhaseFour {
			t.Errorf("CurrentPhase = %q, want %q", s.CurrentPhase, state.PhaseFour)
		}
	})

	t.Run("checkpoint_a_abandon_returns_done", func(t *testing.T) {
		t.Parallel()

		workspace, sm := initWorkspaceForNextAction(t, state.PhaseCheckpointA, func(s *state.State) error {
			s.CurrentPhaseStatus = state.StatusAwaitingHuman
			return nil
		})

		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, "", nil, nil, nil)

		result, err := callNextActionWithUserResponse(t, handler, workspace, "abandon")
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if result.IsError {
			t.Fatalf("handler returned MCP error: %s", result.Content)
		}

		var resp nextActionResponse
		if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		if resp.Action.Type != orchestrator.ActionDone {
			t.Errorf("action.Type = %q, want %q", resp.Action.Type, orchestrator.ActionDone)
		}

		s, loadErr := loadState(workspace)
		if loadErr != nil {
			t.Fatalf("loadState: %v", loadErr)
		}
		if s.CurrentPhaseStatus != state.StatusAbandoned {
			t.Errorf("CurrentPhaseStatus = %q, want %q", s.CurrentPhaseStatus, state.StatusAbandoned)
		}
	})

	t.Run("non_checkpoint_revise_is_noop", func(t *testing.T) {
		t.Parallel()

		// user_response="revise" at a non-checkpoint phase should be a no-op.
		workspace, sm := initWorkspaceForNextAction(t, "phase-1", nil)
		eng := orchestrator.NewEngine("", "")
		handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, "", nil, nil, nil)

		result, err := callNextActionWithUserResponse(t, handler, workspace, "revise")
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if result.IsError {
			t.Fatalf("handler returned MCP error: %s", result.Content)
		}

		var resp nextActionResponse
		if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		// Should still return the phase-1 action (no rewind happened).
		if resp.Action.Type != orchestrator.ActionSpawnAgent {
			t.Errorf("action.Type = %q, want %q", resp.Action.Type, orchestrator.ActionSpawnAgent)
		}
		if resp.Action.Phase != state.PhaseOne {
			t.Errorf("action.Phase = %q, want %q", resp.Action.Phase, state.PhaseOne)
		}
	})
}

// TestExtractTaskNumber verifies the extractTaskNumber helper that is used to
// resolve the {N} template variable in agent .md files.
func TestExtractTaskNumber(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		outputFile string
		want       string
	}{
		{name: "impl_1", outputFile: "impl-1.md", want: "1"},
		{name: "impl_2", outputFile: "impl-2.md", want: "2"},
		{name: "impl_10", outputFile: "impl-10.md", want: "10"},
		{name: "review_1", outputFile: "review-1.md", want: "1"},
		{name: "review_3", outputFile: "review-3.md", want: "3"},
		{name: "analysis", outputFile: "analysis.md", want: ""},
		{name: "design", outputFile: "design.md", want: ""},
		{name: "tasks", outputFile: "tasks.md", want: ""},
		{name: "empty", outputFile: "", want: ""},
		{name: "impl_no_suffix", outputFile: "impl-1", want: ""},
		{name: "impl_prefix_only", outputFile: "impl-", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := extractTaskNumber(tc.outputFile)
			if got != tc.want {
				t.Errorf("extractTaskNumber(%q) = %q, want %q", tc.outputFile, got, tc.want)
			}
		})
	}
}

// TestEnrichPrompt_TemplateSubstitution verifies that {workspace}, {branch},
// {spec-name}, and {N} placeholders in agent .md files are replaced with
// runtime values before the prompt is returned.
func TestEnrichPrompt_TemplateSubstitution(t *testing.T) {
	t.Parallel()

	specName := "my-spec"
	branch := "fix/999-my-fix"

	// Phase-5 with a single sequential task so the engine dispatches a single-task
	// implementer (NewSpawnAgentAction with OutputFile="impl-1.md"), which populates
	// action.OutputFile and allows extractTaskNumber to resolve {N}.
	workspace, sm := initWorkspaceForNextAction(t, state.PhaseFive, func(s *state.State) error {
		s.SpecName = specName
		s.Branch = &branch
		s.Tasks = map[string]state.Task{
			"1": {
				Title:         "Task 1",
				ExecutionMode: state.ExecModeSequential,
				ImplStatus:    "",
				ReviewStatus:  "",
			},
		}
		return nil
	})

	// Write tasks.md so the engine skips ActionTaskInit.
	if err := os.WriteFile(filepath.Join(workspace, "tasks.md"), []byte("# Tasks\n"), 0o600); err != nil {
		t.Fatalf("write tasks.md: %v", err)
	}

	agentDir := t.TempDir()
	// The agent file for the implementer uses all four placeholders.
	agentContent := "workspace={workspace} branch={branch} spec={spec-name} task={N}"
	if err := os.WriteFile(filepath.Join(agentDir, "implementer.md"), []byte(agentContent), 0o600); err != nil {
		t.Fatalf("write implementer.md: %v", err)
	}

	eng := orchestrator.NewEngine(agentDir, "")
	handler := PipelineNextActionHandler(sm, events.NewEventBus(), eng, agentDir, nil, nil, nil)

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

	// All placeholders must be resolved; no literal brace-tokens should remain.
	if strings.Contains(action.Prompt, "{workspace}") {
		t.Errorf("Prompt still contains literal {workspace}\nPrompt: %s", action.Prompt)
	}
	if strings.Contains(action.Prompt, "{branch}") {
		t.Errorf("Prompt still contains literal {branch}\nPrompt: %s", action.Prompt)
	}
	if strings.Contains(action.Prompt, "{spec-name}") {
		t.Errorf("Prompt still contains literal {spec-name}\nPrompt: %s", action.Prompt)
	}
	if strings.Contains(action.Prompt, "{N}") {
		t.Errorf("Prompt still contains literal {N}\nPrompt: %s", action.Prompt)
	}

	// Verify the resolved values appear in the prompt.
	if !strings.Contains(action.Prompt, workspace) {
		t.Errorf("Prompt does not contain resolved workspace %q\nPrompt: %s", workspace, action.Prompt)
	}
	if !strings.Contains(action.Prompt, branch) {
		t.Errorf("Prompt does not contain resolved branch %q\nPrompt: %s", branch, action.Prompt)
	}
	if !strings.Contains(action.Prompt, specName) {
		t.Errorf("Prompt does not contain resolved spec-name %q\nPrompt: %s", specName, action.Prompt)
	}
	// Task number "1" should appear (from impl-1.md output file).
	if !strings.Contains(action.Prompt, "task=1") {
		t.Errorf("Prompt does not contain resolved task number (want 'task=1')\nPrompt: %s", action.Prompt)
	}
}

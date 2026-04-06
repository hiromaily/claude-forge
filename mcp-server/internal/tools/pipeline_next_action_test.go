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

	"github.com/hiromaily/claude-forge/mcp-server/internal/history"
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

	t.Run("output_artifact_section", func(t *testing.T) {
		t.Parallel()
		workspace, sm := initWorkspaceForNextAction(t, "phase-1", nil)

		agentDir := t.TempDir()
		agentContent := "# Situation Analyst\nYou are a situation analyst agent."
		if err := os.WriteFile(filepath.Join(agentDir, "situation-analyst.md"), []byte(agentContent), 0o600); err != nil {
			t.Fatalf("write agent file: %v", err)
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
		handler := PipelineNextActionHandler(sm, eng, "", nil, nil, nil)

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
		handler := PipelineNextActionHandler(sm, eng, "", nil, nil, nil)

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
		if resp.Type == orchestrator.ActionBatchCommit {
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
		handler := PipelineNextActionHandler(sm, eng, "", nil, kb, nil)

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
}

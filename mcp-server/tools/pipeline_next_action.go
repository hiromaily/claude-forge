// Package tools — pipeline_next_action MCP handler.
// Delegates to Engine.NextAction() and enriches spawn_agent prompts
// with agent .md file contents and input artifact file contents.
package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hiromaily/claude-forge/mcp-server/orchestrator"
	"github.com/hiromaily/claude-forge/mcp-server/state"
)

// nextActionResponse wraps orchestrator.Action to add an optional Warning field.
// The warning is set fail-open when enrichPrompt cannot find the agent .md file.
type nextActionResponse struct {
	orchestrator.Action
	Warning string `json:"warning,omitempty"`
}

// PipelineNextActionHandler returns the next pipeline action for the orchestrator
// to execute, given the current workspace state.
//
// Parameters:
//   - workspace (required): absolute path to the workspace directory
//   - user_response (optional): response from the user to a checkpoint (forward-compatibility)
//
// The handler creates a per-call StateManager to avoid workspace-binding conflicts,
// delegates to eng.NextAction, and — for spawn_agent actions — enriches the prompt
// with the agent .md file contents and each input artifact's contents.
func PipelineNextActionHandler(
	_ *state.StateManager,
	eng *orchestrator.Engine,
	agentDir string,
) server.ToolHandlerFunc {
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspace, err := req.RequireString("workspace")
		if err != nil {
			return errorf("%v", err)
		}
		userResponse := req.GetString("user_response", "")

		// Per-call StateManager: create a fresh instance to avoid workspace-mismatch errors.
		sm2 := state.NewStateManager()
		if err := sm2.LoadFromFile(workspace); err != nil {
			return errorf("load state: %v", err)
		}

		action, err := eng.NextAction(sm2, userResponse)
		if err != nil {
			return errorf("next_action: %v", err)
		}

		resp := nextActionResponse{Action: action}

		if action.Type == orchestrator.ActionSpawnAgent && agentDir != "" {
			if enrichErr := enrichPrompt(&action, agentDir, workspace); enrichErr != nil {
				// Fail-open: return the action with a warning, not an error.
				resp.Warning = fmt.Sprintf("enrichPrompt: %v", enrichErr)
			}
			resp.Action = action
		}

		return okJSON(resp)
	}
}

// enrichPrompt builds the agent prompt by concatenating the agent .md file contents
// with the workspace input artifact contents.
//
// The resulting prompt format is:
//
//	[agent .md file contents]
//
//	## Input Artifacts
//
//	### {filename}
//	[file contents]
//	...
//
// Returns an error if the agent .md file is missing (caller should treat as warning).
// Missing input artifact files are noted inline; they do not cause an error.
func enrichPrompt(action *orchestrator.Action, agentDir, workspace string) error {
	agentFile := filepath.Join(agentDir, action.Agent+".md")
	agentData, err := os.ReadFile(agentFile)
	if err != nil {
		return fmt.Errorf("read agent file %q: %w", agentFile, err)
	}

	var sb strings.Builder
	sb.Write(agentData)
	sb.WriteString("\n\n## Input Artifacts\n")

	for _, inputFile := range action.InputFiles {
		absPath := filepath.Join(workspace, inputFile)
		fileData, readErr := os.ReadFile(absPath)
		sb.WriteString("\n### ")
		sb.WriteString(filepath.Base(inputFile))
		sb.WriteString("\n")
		if readErr != nil {
			sb.WriteString("(file not found: ")
			sb.WriteString(inputFile)
			sb.WriteString(")\n")
		} else {
			sb.Write(fileData)
			sb.WriteString("\n")
		}
	}

	action.Prompt = sb.String()
	return nil
}

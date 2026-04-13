// workspace initialisation helpers.

package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
)

// initWorkspace executes the 8-step I/O sequence for the second call.
// It creates the workspace directory, initialises state, applies all configuration in
// a single write via sm.Configure, and writes request.md.
// enrichedBody is the pre-built request.md body from the discussion call; "" means use
// buildRequestMDWithBody defaults (extCtx.TaskText for text source, GitHub/Jira content otherwise).
// Returns the request.md content on success.
func initWorkspace(
	sm *state.StateManager,
	workspace, specName string,
	flags pipelineFlags,
	uc userConfirmation,
	branchName string,
	flowTemplate string,
	skippedPhases []string,
	extCtx externalContext,
	enrichedBody string,
) (string, error) {
	// Validate workspace path — reject non-ASCII characters so that
	// multibyte input (e.g. Japanese) never produces an unreadable directory name.
	if hasNonASCII(workspace) {
		return "", fmt.Errorf("workspace path %q contains non-ASCII characters; use only ASCII in directory names", workspace)
	}

	// Create workspace directory.
	if err := os.MkdirAll(workspace, 0o750); err != nil {
		return "", fmt.Errorf("MkdirAll %q: %w", workspace, err)
	}

	// Check state.json doesn't already exist.
	stateFile := filepath.Join(workspace, "state.json")
	if _, err := os.Stat(stateFile); err == nil {
		return "", fmt.Errorf("workspace %q already initialised: state.json exists", workspace)
	}

	// sm.Init.
	if err := sm.Init(workspace, specName); err != nil {
		return "", fmt.Errorf("sm.Init: %w", err)
	}

	// Apply all configuration in a single write to state.json.
	// Branch name is pre-derived by the caller (either current branch or
	// DeriveBranchName), avoiding a separate SetBranch call.
	cfg := state.PipelineConfig{
		Effort:           uc.Effort,
		FlowTemplate:     flowTemplate,
		AutoApprove:      flags.Auto,
		SkipPR:           flags.SkipPR,
		Debug:            flags.Debug,
		SkippedPhases:    skippedPhases,
		UseCurrentBranch: uc.UseCurrentBranch,
		Branch:           branchName,
	}
	if err := sm.Configure(workspace, cfg); err != nil {
		return "", fmt.Errorf("configure: %w", err)
	}

	// Write request.md.
	body := enrichedBody
	if body == "" {
		body = extCtx.TaskText
	}
	requestMD := buildRequestMDWithBody(extCtx, body)
	reqPath := filepath.Join(workspace, "request.md")
	if err := os.WriteFile(reqPath, []byte(requestMD), 0o600); err != nil {
		return "", fmt.Errorf("write request.md: %w", err)
	}

	return requestMD, nil
}

// applyWorkspaceSlug replaces the slug portion of a workspace path with the
// LLM-generated slug. If slugify produces an empty result (e.g. pure Japanese input),
// the original workspace path is returned unchanged.
func applyWorkspaceSlug(workspace, rawSlug string) string {
	cleaned := slugify(rawSlug)
	if cleaned == "" {
		return workspace
	}
	return replaceWorkspaceSlug(workspace, cleaned)
}

// hasNonASCII guards workspace paths against unreadable multibyte characters (e.g. Japanese).
func hasNonASCII(s string) bool {
	for _, r := range s {
		if r > 0x7F {
			return true
		}
	}
	return false
}

// deriveSpecName extracts the spec name from the workspace path.
// It takes the base name and strips the YYYYMMDD- prefix.
// If no hyphen is found, it returns the full base name.
func deriveSpecName(workspace string) string {
	base := filepath.Base(workspace)
	_, after, ok := strings.Cut(base, "-")
	if !ok {
		return base
	}
	return after
}

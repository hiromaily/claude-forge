package tools

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hiromaily/claude-forge/mcp-server/internal/orchestrator"
)

// buildSpawnMessage returns the progress line to display before spawning an agent.
// Format (sequential): "▶ Phase N — Label  ·  spawning agent-name…"
// Format (parallel):   "▶ Phase N — Label  ·  spawning agent-name  (parallel · N tasks)…"
// Returns empty string for non-spawn-agent actions.
func buildSpawnMessage(action orchestrator.Action) string {
	if action.Type != orchestrator.ActionSpawnAgent {
		return ""
	}
	label := phaseDisplayLabel(action.Phase)
	if len(action.ParallelTaskIDs) > 0 {
		return fmt.Sprintf("▶ %s  ·  spawning %s  (parallel · %d tasks)…",
			label, action.Agent, len(action.ParallelTaskIDs))
	}
	return fmt.Sprintf("▶ %s  ·  spawning %s…", label, action.Agent)
}

// buildCompleteMessage returns the completion line displayed after a phase finishes.
// Format: "  ✓ Complete  ·  1,847 tokens · 0:23"
func buildCompleteMessage(tokensUsed, durationMs int) string {
	return fmt.Sprintf("  ✓ Complete  ·  %s tokens · %s",
		formatTokens(tokensUsed), formatDuration(durationMs))
}

// phaseDisplayLabel builds a human-readable label for a phase ID.
// For phase-N IDs returns "Phase N — Label" when a label exists in the registry,
// or just "Phase N" when the registry falls back to the ID itself.
// For other IDs (checkpoints, final phases) returns the PhaseLabel value directly.
func phaseDisplayLabel(id string) string {
	if after, ok := strings.CutPrefix(id, "phase-"); ok {
		label := orchestrator.PhaseLabel(id)
		if label != id {
			return "Phase " + after + " — " + label
		}
		return "Phase " + after
	}
	return orchestrator.PhaseLabel(id)
}

// formatTokens formats n as a comma-separated integer string (e.g. 12345 → "12,345").
func formatTokens(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	cut := len(s) % 3
	if cut == 0 {
		cut = 3
	}
	var b strings.Builder
	b.WriteString(s[:cut])
	for i := cut; i < len(s); i += 3 {
		b.WriteByte(',')
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

// formatDuration formats milliseconds as "M:SS".
func formatDuration(ms int) string {
	total := ms / 1000
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}

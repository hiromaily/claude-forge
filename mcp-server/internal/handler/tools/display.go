package tools

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/orchestrator"
	"github.com/hiromaily/claude-forge/mcp-server/internal/intelligence/analytics"
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

// formatCheckpointCostLine returns a one-line running-cost summary for display at a
// human checkpoint, e.g. "  💰 So far: 1,234,567 tokens · ~$7.41 · 6 phases · 2 retries".
// Returns "" for a nil summary or a fresh pipeline that has logged no work yet, so the
// line is only shown once there is something meaningful to report (improvement #8).
func formatCheckpointCostLine(s *analytics.PipelineSummary) string {
	if s == nil || s.TotalTokens == 0 {
		return ""
	}
	return fmt.Sprintf("  💰 So far: %s tokens · ~$%.2f · %d phases · %d retries",
		formatTokens(s.TotalTokens), s.EstimatedCostUSD, s.PhasesExecuted, s.Retries)
}

// formatEstimateLine returns a one-line, pre-formatted upfront cost/token forecast for
// the detected effort, e.g.
// "  📊 Estimate (effort=L, 4 past run(s)): ~1,234,567 tokens / ~$7.41 (P50) · up to ~2,345,678 tokens / ~$14.08 (P90)".
// Returns "" when no estimate is available (nil) or there is no history (sample_size 0).
// The orchestrator displays this verbatim, so the P50/P90 figures are formatted here in
// the server rather than reconstructed by the LLM from the raw estimate struct — keeping
// the presentation deterministic (improvement #8).
func formatEstimateLine(effort string, est *analytics.EstimateResult) string {
	if est == nil || est.SampleSize == 0 {
		return ""
	}
	return fmt.Sprintf(
		"  📊 Estimate (effort=%s, %d past run(s)): ~%s tokens / ~$%.2f (P50) · up to ~%s tokens / ~$%.2f (P90)",
		effort, est.SampleSize,
		formatTokens(int(est.Tokens.P50)), est.CostUSD.P50,
		formatTokens(int(est.Tokens.P90)), est.CostUSD.P90,
	)
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
// Negative values are formatted as "-" followed by the formatted absolute value.
func formatTokens(n int) string {
	if n < 0 {
		return "-" + formatTokens(-n)
	}
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

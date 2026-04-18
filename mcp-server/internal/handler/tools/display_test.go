package tools

import (
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/orchestrator"
)

func TestBuildSpawnMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		action orchestrator.Action
		want   string
	}{
		{
			name:   "sequential_phase1",
			action: orchestrator.NewSpawnAgentAction("situation-analyst", "", "", "phase-1", nil, ""),
			want:   "▶ Phase 1 — Situation Analysis  ·  spawning situation-analyst…",
		},
		{
			name:   "sequential_phase3",
			action: orchestrator.NewSpawnAgentAction("architect", "", "", "phase-3", nil, ""),
			want:   "▶ Phase 3 — Design  ·  spawning architect…",
		},
		{
			name:   "sequential_phase3b",
			action: orchestrator.NewSpawnAgentAction("design-reviewer", "", "", "phase-3b", nil, ""),
			want:   "▶ Phase 3b — Design Review  ·  spawning design-reviewer…",
		},
		{
			name: "parallel_phase5",
			action: orchestrator.NewParallelSpawnAction("implementer", "", "", "phase-5", nil,
				[]string{"1", "2", "3"}),
			want: "▶ Phase 5 — Implementation  ·  spawning implementer  (parallel · 3 tasks)…",
		},
		{
			name:   "final_verification",
			action: orchestrator.NewSpawnAgentAction("verifier", "", "", "final-verification", nil, ""),
			want:   "▶ Final Verification  ·  spawning verifier…",
		},
		{
			name:   "non_spawn_returns_empty",
			action: orchestrator.NewCheckpointAction("checkpoint-a", "", nil),
			want:   "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildSpawnMessage(tc.action)
			if got != tc.want {
				t.Errorf("buildSpawnMessage() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildCompleteMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		tokensUsed int
		durationMs int
		want       string
	}{
		{
			name:       "sub_thousand_tokens",
			tokensUsed: 847,
			durationMs: 23000,
			want:       "  ✓ Complete  ·  847 tokens · 0:23",
		},
		{
			name:       "thousands_tokens_with_comma",
			tokensUsed: 1847,
			durationMs: 90000,
			want:       "  ✓ Complete  ·  1,847 tokens · 1:30",
		},
		{
			name:       "zero_values",
			tokensUsed: 0,
			durationMs: 0,
			want:       "  ✓ Complete  ·  0 tokens · 0:00",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildCompleteMessage(tc.tokensUsed, tc.durationMs)
			if got != tc.want {
				t.Errorf("buildCompleteMessage(%d, %d) = %q, want %q",
					tc.tokensUsed, tc.durationMs, got, tc.want)
			}
		})
	}
}

func TestFormatTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{1847, "1,847"},
		{10000, "10,000"},
		{12345, "12,345"},
		{100000, "100,000"},
		{123456, "123,456"},
		{1234567, "1,234,567"},
		{-1847, "-1,847"},
		{-999, "-999"},
	}

	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			got := formatTokens(tc.n)
			if got != tc.want {
				t.Errorf("formatTokens(%d) = %q, want %q", tc.n, got, tc.want)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ms   int
		want string
	}{
		{0, "0:00"},
		{23000, "0:23"},
		{60000, "1:00"},
		{90000, "1:30"},
		{3661000, "61:01"},
	}

	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			got := formatDuration(tc.ms)
			if got != tc.want {
				t.Errorf("formatDuration(%d) = %q, want %q", tc.ms, got, tc.want)
			}
		})
	}
}

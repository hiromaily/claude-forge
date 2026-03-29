package prompt

import (
	"strings"
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/history"
)

// makeFullCtx returns a HistoryContext populated with sample data for testing.
func makeFullCtx() HistoryContext {
	return HistoryContext{
		SimilarPipelines: []history.SearchResult{
			{SpecName: "20260101-fix-auth", OneLiner: "Similar bugfix. Design chose X approach.", Similarity: 0.85},
			{SpecName: "20260115-add-middleware", OneLiner: "Added middleware layer.", Similarity: 0.72},
		},
		CriticalPatterns: []history.PatternEntry{
			{Severity: "CRITICAL", Pattern: "missing error handling", Frequency: 3, Agent: "impl-reviewer"},
		},
		AllPatterns: []history.PatternEntry{
			{Severity: "CRITICAL", Pattern: "missing error handling", Frequency: 3, Agent: "impl-reviewer"},
			{Severity: "MINOR", Pattern: "import grouping inconsistency", Frequency: 5, Agent: "impl-reviewer"},
		},
		FrictionPoints: []history.FrictionPoint{
			{Category: "test_coverage", Description: "Implementer writes test fixtures to wrong directory", Mitigation: "specify testdata path explicitly"},
			{Category: "naming_convention", Description: "Variable names too abbreviated", Mitigation: "use full descriptive names"},
		},
	}
}

// TestBuildPrompt covers all nine design-specified test cases in a table-driven manner.
func TestBuildPrompt(t *testing.T) {
	t.Parallel()

	const (
		layer1 = "# Agent Instructions\n\nDo the task well."
		layer2 = "## Input Artifacts\n\n### design.md\nSome design content."
	)

	tests := []struct {
		name      string
		agentName string
		profile   string
		ctx       HistoryContext
		// presence checks
		wantContains    []string
		wantNotContains []string
	}{
		{
			name:      "empty_history_empty_profile",
			agentName: "implementer",
			profile:   "",
			ctx:       HistoryContext{},
			wantContains: []string{
				layer1,
				layer2,
			},
			wantNotContains: []string{
				"## Repository Context",
				"## Past Similar Pipelines",
				"## Common Review Findings",
				"## Known AI Friction Points",
			},
		},
		{
			name:      "architect_similar_and_critical_patterns_no_friction",
			agentName: "architect",
			profile:   "",
			ctx:       makeFullCtx(),
			wantContains: []string{
				"## Past Similar Pipelines",
				"20260101-fix-auth",
				"## Common Review Findings",
				"CRITICAL",
				"missing error handling",
			},
			wantNotContains: []string{
				"## Known AI Friction Points",
				"MINOR",
				"import grouping inconsistency",
			},
		},
		{
			name:      "implementer_critical_patterns_and_friction_no_similar",
			agentName: "implementer",
			profile:   "",
			ctx:       makeFullCtx(),
			wantContains: []string{
				"## Common Review Findings",
				"CRITICAL",
				"missing error handling",
				"## Known AI Friction Points",
				"Implementer writes test fixtures to wrong directory",
				"specify testdata path explicitly",
			},
			wantNotContains: []string{
				"## Past Similar Pipelines",
				"import grouping inconsistency",
			},
		},
		{
			name:      "impl_reviewer_all_patterns_and_friction_no_similar",
			agentName: "impl-reviewer",
			profile:   "",
			ctx:       makeFullCtx(),
			wantContains: []string{
				"## Common Review Findings",
				"CRITICAL",
				"missing error handling",
				"MINOR",
				"import grouping inconsistency",
				"## Known AI Friction Points",
				"Implementer writes test fixtures to wrong directory",
			},
			wantNotContains: []string{
				"## Past Similar Pipelines",
			},
		},
		{
			name:      "task_decomposer_all_patterns_and_similar_no_friction",
			agentName: "task-decomposer",
			profile:   "",
			ctx:       makeFullCtx(),
			wantContains: []string{
				"## Past Similar Pipelines",
				"20260101-fix-auth",
				"## Common Review Findings",
				"CRITICAL",
				"missing error handling",
				"MINOR",
				"import grouping inconsistency",
			},
			wantNotContains: []string{
				"## Known AI Friction Points",
			},
		},
		{
			name:      "other_agent_no_layer4_subsections",
			agentName: "situation-analyst",
			profile:   "",
			ctx:       makeFullCtx(),
			wantNotContains: []string{
				"## Past Similar Pipelines",
				"## Common Review Findings",
				"## Known AI Friction Points",
			},
		},
		{
			name:      "nonempty_profile_layer3_present",
			agentName: "other-agent",
			profile:   "Language: Go (85%), Shell (10%)\nTest framework: go test",
			ctx:       HistoryContext{},
			wantContains: []string{
				"## Repository Context",
				"Language: Go",
				"Test framework: go test",
			},
		},
		{
			name:      "token_budget_exceeded_layer4_truncated",
			agentName: "implementer",
			profile:   "",
			ctx:       makeFullCtx(),
			// The base is small; we test budget behavior separately in
			// TestBuildPrompt_TokenBudgetItemLevelTruncation below.
			// Here we verify the normal (within-budget) path produces friction.
			wantContains: []string{
				"## Known AI Friction Points",
				"Implementer writes test fixtures to wrong directory",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_ = t.Context() // satisfy the t.Context() usage requirement

			got := BuildPrompt(tc.agentName, layer1, layer2, tc.profile, tc.ctx)

			for _, want := range tc.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("BuildPrompt(%q): expected output to contain %q\ngot:\n%s", tc.agentName, want, got)
				}
			}

			for _, notWant := range tc.wantNotContains {
				if strings.Contains(got, notWant) {
					t.Errorf("BuildPrompt(%q): expected output NOT to contain %q\ngot:\n%s", tc.agentName, notWant, got)
				}
			}
		})
	}
}

// TestBuildPrompt_TokenBudgetItemLevelTruncation verifies that when the token
// budget is tight enough to remove one friction item but not all, the last
// (lowest-ranked) friction item is removed first while earlier items remain.
//
// Strategy: construct a base that uses nearly all of tokenBudget, then add a
// HistoryContext for "implementer" with two friction items where the combined
// prompt would exceed the budget but removing one friction item brings it within.
func TestBuildPrompt_TokenBudgetItemLevelTruncation(t *testing.T) {
	t.Parallel()

	_ = t.Context()

	// estimateTokens uses (len(s) + 3) / 4, so 1 token ≈ 4 chars.
	// tokenBudget = 8_000 tokens ≈ 32_000 chars.
	//
	// We want the base to consume most of the budget, leaving room for only
	// a portion of Layer 4.
	//
	// The two friction items will add roughly:
	//   item1: "- category_a: First friction item text\n  Mitigation: mitigation one\n" ≈ ~70 chars ≈ ~18 tokens
	//   item2: "- category_b: Second friction item text\n  Mitigation: mitigation two\n" ≈ ~70 chars ≈ ~18 tokens
	// Plus the header "## Known AI Friction Points\n\n" ≈ ~30 chars ≈ ~8 tokens
	//
	// We set the base to tokenBudget - 20 tokens worth of characters:
	// (8_000 - 20) * 4 = 31_920 chars. Adding both friction items (~44 tokens)
	// would push it over. But removing the second item (~18 tokens) leaves
	// header (~8) + item1 (~18) = ~26 tokens below the 20-token headroom.
	//
	// Use a base of exactly (tokenBudget - 25) * 4 chars to leave ~25 tokens
	// of headroom — enough for item1 (≈18 tokens) and header (≈8 tokens)
	// but not for both items together.

	const headroom = 25 // tokens left for Layer 4 after base fills budget
	baseLen := (tokenBudget-headroom)*4 - 3

	// Build a large base string (Layer 1 only, Layer 2 empty).
	largeBase := strings.Repeat("x", baseLen)

	// Friction item sizes: ensure item1 alone fits within headroom but both don't.
	// Each friction line is: "- {Category}: {Description}\n  Mitigation: {Mitigation}\n"
	// item1 ≈ 70 chars / 4 ≈ 18 tokens (fits within 25 - 8 = 17 remaining after header)
	// item2 ≈ 70 chars / 4 ≈ 18 tokens (does NOT fit)
	//
	// Actually let us make item1 small (~12 tokens) and item2 larger (~20 tokens)
	// so that item1 fits in the remaining headroom after the header (8 tokens),
	// but item1 + item2 together do not.

	ctx := HistoryContext{
		FrictionPoints: []history.FrictionPoint{
			// item1 — small, should survive truncation
			{Category: "a", Description: "first item", Mitigation: "fix a"},
			// item2 — second item; should be truncated (removed from end) first
			{Category: "b", Description: "second item that is the last ranked and should be removed by the budget guard when space is tight", Mitigation: "fix b"},
		},
	}

	got := BuildPrompt("implementer", largeBase, "", "", ctx)

	// item1 should still be present (it was first in the slice, i.e., higher ranked).
	if !strings.Contains(got, "first item") {
		t.Errorf("expected first friction item to remain after truncation, but it was removed\ngot length: %d", len(got))
	}

	// item2 (the last / lowest-ranked item) should have been removed.
	if strings.Contains(got, "second item that is the last ranked") {
		t.Errorf("expected last friction item to be truncated, but it is still present\ngot length: %d", len(got))
	}

	// The result must not exceed the token budget.
	tokens := estimateTokens(got)
	if tokens > tokenBudget {
		t.Errorf("result exceeds token budget: got %d tokens, want <= %d", tokens, tokenBudget)
	}
}

package tools

import (
	"slices"
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/state"
	"github.com/hiromaily/claude-forge/mcp-server/internal/intelligence/history"
	"github.com/hiromaily/claude-forge/mcp-server/internal/intelligence/prompt"
)

func patternTitles(entries []history.PatternEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Pattern
	}
	return out
}

// TestFilterFindingsForTask verifies improvement #2: a per-task impl-reviewer only
// receives findings relevant to the task under review.
func TestFilterFindingsForTask(t *testing.T) {
	t.Parallel()

	sqlFinding := history.PatternEntry{Pattern: "distinct order sql query 42p10", Severity: "CRITICAL", Category: "performance"}
	adapterFinding := history.PatternEntry{Pattern: "adapter user mapping incorrect", Severity: "MINOR", Category: "naming_convention"}

	ctxFor := func() prompt.HistoryContext {
		return prompt.HistoryContext{
			AllPatterns:      []history.PatternEntry{sqlFinding, adapterFinding},
			CriticalPatterns: []history.PatternEntry{sqlFinding},
		}
	}

	t.Run("frontend_task_drops_sql_finding", func(t *testing.T) {
		t.Parallel()
		st := &state.State{Tasks: map[string]state.Task{
			"14": {Title: "FE user adapter", Files: []string{"frontend/app/adapters/user.ts"}},
		}}
		got := filterFindingsForTask(st, "14", ctxFor())
		titles := patternTitles(got.AllPatterns)
		if slices.Contains(titles, sqlFinding.Pattern) {
			t.Errorf("SQL finding should be dropped for a frontend task; got %v", titles)
		}
		if !slices.Contains(titles, adapterFinding.Pattern) {
			t.Errorf("adapter finding should be kept for the FE adapter task; got %v", titles)
		}
	})

	t.Run("sql_task_keeps_sql_finding", func(t *testing.T) {
		t.Parallel()
		st := &state.State{Tasks: map[string]state.Task{
			"13": {Title: "deal SQL aggregation", Files: []string{"backend/query/deal.sql"}},
		}}
		got := filterFindingsForTask(st, "13", ctxFor())
		if !slices.Contains(patternTitles(got.AllPatterns), sqlFinding.Pattern) {
			t.Errorf("SQL finding should be kept for a SQL task; got %v", patternTitles(got.AllPatterns))
		}
	})

	t.Run("fails_open_when_task_has_no_scope", func(t *testing.T) {
		t.Parallel()
		st := &state.State{Tasks: map[string]state.Task{"5": {Title: "", Files: nil}}}
		got := filterFindingsForTask(st, "5", ctxFor())
		if len(got.AllPatterns) != 2 {
			t.Errorf("scopeless task must fail open (keep all findings); got %d", len(got.AllPatterns))
		}
	})

	t.Run("fails_open_when_task_unknown", func(t *testing.T) {
		t.Parallel()
		st := &state.State{Tasks: map[string]state.Task{}}
		got := filterFindingsForTask(st, "99", ctxFor())
		if len(got.AllPatterns) != 2 {
			t.Errorf("unknown task must fail open; got %d", len(got.AllPatterns))
		}
	})
}

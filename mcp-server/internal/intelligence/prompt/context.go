// assembles the 4-layer prompt passed to each pipeline agent.
// It integrates agent base instructions, input artifacts, repository profile
// context (Layer 3), and data flywheel history context (Layer 4).

package prompt

import (
	"github.com/hiromaily/claude-forge/mcp-server/internal/intelligence/history"
)

const defaultPatternQueryLimit = 20

// HistoryContext holds the pre-fetched data flywheel context for Layer 4.
// All fields are nil/empty when no history data is available.
type HistoryContext struct {
	SimilarPipelines []history.SearchResult
	CriticalPatterns []history.PatternEntry
	AllPatterns      []history.PatternEntry
	FrictionPoints   []history.FrictionPoint
}

// BuildContextFromResults assembles a HistoryContext from pre-fetched search
// results and an in-memory KnowledgeBase. Either argument may be nil/empty;
// missing data degrades gracefully to empty slices.
func BuildContextFromResults(results []history.SearchResult, kb *history.KnowledgeBase) HistoryContext {
	if kb == nil {
		return HistoryContext{
			SimilarPipelines: results,
		}
	}

	return HistoryContext{
		SimilarPipelines: results,
		CriticalPatterns: kb.Patterns.Query("", "CRITICAL", defaultPatternQueryLimit),
		AllPatterns:      kb.Patterns.Query("", "", defaultPatternQueryLimit),
		FrictionPoints:   kb.Friction.Points(),
	}
}

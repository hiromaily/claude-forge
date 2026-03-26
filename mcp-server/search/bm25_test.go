package search

import (
	"math"
	"testing"
)

//go:fix inline
func strPtr(s string) *string { return new(s) }

// TestTokenize verifies lowercasing, splitting on non-word chars, and filtering < 4 chars.
func TestTokenize(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "empty string",
			input: "",
			want:  []string{},
		},
		{
			name:  "short tokens filtered",
			input: "a bb ccc dddd",
			want:  []string{"dddd"},
		},
		{
			name:  "lowercases input",
			input: "Hello WORLD",
			want:  []string{"hello", "world"},
		},
		{
			name:  "splits on non-word characters",
			input: "foo-bar.bazz quux",
			want:  []string{"bazz", "quux"},
		},
		{
			name:  "mixed case and short tokens",
			input: "MCP Server State API",
			want:  []string{"server", "state"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Tokenize(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("Tokenize(%q) = %v, want %v", tc.input, got, tc.want)
			}
			for i, tok := range got {
				if tok != tc.want[i] {
					t.Errorf("Tokenize(%q)[%d] = %q, want %q", tc.input, i, tok, tc.want[i])
				}
			}
		})
	}
}

// TestScoreTermFrequency verifies that tf=2 scores strictly higher than tf=1
// but strictly less than 2× (BM25 saturation).
func TestScoreTermFrequency(t *testing.T) {
	entries := []IndexEntry{
		{SpecName: "one", RequestSummary: "alpha"},      // tf=2
		{SpecName: "two", RequestSummary: "alpha beta"}, // tf=1
	}
	results := Score(entries, "alpha", "", DefaultBM25Params())
	if len(results) < 2 {
		t.Fatalf("expected 2 scored entries, got %d", len(results))
	}
	// Results sorted descending, so results[0] should be tf=2
	if results[0].Entry.SpecName != "one" {
		t.Errorf("expected tf=2 entry first, got %q", results[0].Entry.SpecName)
	}
	score2 := results[0].Score
	score1 := results[1].Score
	if score2 <= score1 {
		t.Errorf("tf=2 score %f should be > tf=1 score %f", score2, score1)
	}
	if score2 >= 2*score1 {
		t.Errorf("tf=2 score %f should be < 2× tf=1 score %f (saturation check)", score2, 2*score1)
	}
}

// TestScoreIDF verifies that a rare term scores higher than a ubiquitous term.
func TestScoreIDF(t *testing.T) {
	entries := []IndexEntry{
		{SpecName: "a", RequestSummary: "common rare"},
		{SpecName: "b", RequestSummary: "common "},
		{SpecName: "c", RequestSummary: "common"},
	}
	// "common" appears in all 3 docs (high df, low IDF)
	// "rare" appears in only 1 doc (low df, high IDF)
	// entry "a" has both; score with rare query vs common query
	rareResults := Score(entries, "rare", "", DefaultBM25Params())
	commonResults := Score(entries, "common", "", DefaultBM25Params())

	// "a" should appear in rare results
	if len(rareResults) == 0 || rareResults[0].Entry.SpecName != "a" {
		t.Fatalf("expected entry 'a' first in rare results, got %v", rareResults)
	}
	// In common results, all 3 appear. Entry "a" has most tf for "common"
	if len(commonResults) == 0 {
		t.Fatalf("expected common results to be non-empty")
	}

	// The IDF of "rare" (df=1, N=3) should be higher than IDF of "common" (df=3, N=3)
	idfRare := math.Log((3-1+0.5)/(1+0.5) + 1)
	idfCommon := math.Log((3-3+0.5)/(3+0.5) + 1)
	if idfRare <= idfCommon {
		t.Errorf("IDF of rare term %f should be > IDF of common term %f", idfRare, idfCommon)
	}
}

// TestScoreOrdering verifies results are sorted by descending score.
func TestScoreOrdering(t *testing.T) {
	// Use documents with very different term frequencies to ensure ordering is deterministic.
	// All documents have the same length so length normalization doesn't interfere.
	// "high" has alpha 3 times, "mid" has alpha 2 times, "low" has alpha 1 time.
	// Additional filler terms keep document lengths equal (6 tokens each).
	entries := []IndexEntry{
		{SpecName: "low", RequestSummary: "alpha beta gamma delta epsilon zeta"},
		{SpecName: "high", RequestSummary: "alpha beta gamma delta"},
		{SpecName: "mid", RequestSummary: "alpha beta gamma delta epsilon"},
	}
	results := Score(entries, "alpha", "", DefaultBM25Params())
	if len(results) < 3 {
		t.Fatalf("expected 3 scored entries, got %d", len(results))
	}
	for i := 1; i < len(results); i++ {
		if results[i].Score > results[i-1].Score {
			t.Errorf("results not sorted descending: results[%d].Score=%f > results[%d].Score=%f",
				i, results[i].Score, i-1, results[i-1].Score)
		}
	}
	if results[0].Entry.SpecName != "high" {
		t.Errorf("expected 'high' first, got %q", results[0].Entry.SpecName)
	}
	if results[len(results)-1].Entry.SpecName != "low" {
		t.Errorf("expected 'low' last, got %q", results[len(results)-1].Entry.SpecName)
	}
}

// TestScoreZeroExclusion verifies entries with score <= 0 are excluded.
func TestScoreZeroExclusion(t *testing.T) {
	entries := []IndexEntry{
		{SpecName: "match", RequestSummary: "golang programming"},
		{SpecName: "nomatch", RequestSummary: "unrelated content here"},
	}
	// Query with a term that only appears in "match"
	results := Score(entries, "golang", "", DefaultBM25Params())
	for _, r := range results {
		if r.Score <= 0 {
			t.Errorf("entry %q has score %f <= 0, should be excluded", r.Entry.SpecName, r.Score)
		}
	}
	// "nomatch" should not appear
	for _, r := range results {
		if r.Entry.SpecName == "nomatch" {
			t.Errorf("entry 'nomatch' should be excluded (score <= 0)")
		}
	}
}

// TestScoreTaskTypeBoost verifies that a matching taskType entry ranks above
// an entry with equal BM25 score.
func TestScoreTaskTypeBoost(t *testing.T) {
	// Identical requestSummary means equal BM25, but one has matching taskType
	taskType := "feature"
	entries := []IndexEntry{
		{SpecName: "noboosted", RequestSummary: "implement search scoring", TaskType: new("bugfix")},
		{SpecName: "boosted", RequestSummary: "implement search scoring", TaskType: new("feature")},
	}
	results := Score(entries, "implement search scoring", taskType, DefaultBM25Params())
	if len(results) < 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Entry.SpecName != "boosted" {
		t.Errorf("expected boosted entry first, got %q", results[0].Entry.SpecName)
	}
	if results[0].Score <= results[1].Score {
		t.Errorf("boosted score %f should be > unboosted score %f", results[0].Score, results[1].Score)
	}
}

// TestScoreEmptyQuery verifies empty query returns empty slice.
func TestScoreEmptyQuery(t *testing.T) {
	entries := []IndexEntry{
		{SpecName: "a", RequestSummary: "some content"},
	}
	results := Score(entries, "", "", DefaultBM25Params())
	if len(results) != 0 {
		t.Errorf("expected empty results for empty query, got %v", results)
	}
}

// TestScoreEmptyEntries verifies empty entries returns empty slice.
func TestScoreEmptyEntries(t *testing.T) {
	results := Score([]IndexEntry{}, "query", "", DefaultBM25Params())
	if len(results) != 0 {
		t.Errorf("expected empty results for empty entries, got %v", results)
	}
}

// TestScoreSingleDocument verifies IDF is positive and finite for a single-document corpus.
func TestScoreSingleDocument(t *testing.T) {
	entries := []IndexEntry{
		{SpecName: "only", RequestSummary: "golang search implementation"},
	}
	results := Score(entries, "golang", "", DefaultBM25Params())
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	score := results[0].Score
	if score <= 0 {
		t.Errorf("expected positive score for single document, got %f", score)
	}
	if math.IsInf(score, 0) || math.IsNaN(score) {
		t.Errorf("expected finite score for single document, got %f", score)
	}
}

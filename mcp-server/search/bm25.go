// Package search implements BM25 scoring for pattern index entries.
// No external imports beyond Go stdlib (math, sort, strings, unicode).
package search

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// IndexEntry mirrors a single entry in .specs/index.json.
type IndexEntry struct {
	SpecName       string           `json:"specName"`
	Timestamp      string           `json:"timestamp"`
	TaskType       *string          `json:"taskType"`
	RequestSummary string           `json:"requestSummary"`
	ReviewFeedback []ReviewFeedback `json:"reviewFeedback"`
	ImplPatterns   []ImplPattern    `json:"implPatterns"`
	ImplOutcomes   []ImplOutcome    `json:"implOutcomes"`
	Outcome        string           `json:"outcome"`
}

// ImplOutcome mirrors the implOutcomes array in index.json.
// It is not used for BM25 scoring or output formatting (corpus is requestSummary only).
// Included for schema completeness so IndexEntry fully mirrors the canonical index schema
// produced by indexer.BuildSpecsIndex. Fields match the schema produced by indexer.BuildSpecsIndex:
// {reviewFile, verdict} — NOT {taskTitle, verdict} (taskTitle belongs to ImplPattern).
type ImplOutcome struct {
	ReviewFile string `json:"reviewFile"`
	Verdict    string `json:"verdict"`
}

// ReviewFeedback represents review feedback associated with an index entry.
type ReviewFeedback struct {
	Source   string   `json:"source"`
	Verdict  string   `json:"verdict"`
	Findings []string `json:"findings"`
}

// ImplPattern represents an implementation pattern associated with an index entry.
type ImplPattern struct {
	TaskTitle     string   `json:"taskTitle"`
	FilesModified []string `json:"filesModified"`
}

// ScoredEntry pairs an IndexEntry with its computed BM25 final score.
type ScoredEntry struct {
	Entry IndexEntry
	Score float64
}

// BM25Params holds the BM25 hyperparameters.
type BM25Params struct {
	K1 float64 // term-frequency saturation, default 1.5
	B  float64 // length normalisation, default 0.75
}

// DefaultBM25Params returns the standard BM25 parameters used by this package.
func DefaultBM25Params() BM25Params {
	return BM25Params{K1: 1.5, B: 0.75}
}

// Tokenize lowercases text, splits on non-word characters, and filters tokens
// shorter than 4 characters. Returns an empty (non-nil) slice for empty input.
func Tokenize(text string) []string {
	tokens := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	result := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		if len(tok) >= 4 {
			result = append(result, tok)
		}
	}
	return result
}

// Score runs BM25 over entries using query tokens derived from queryText,
// applies the taskType multiplicative boost, and returns entries sorted by
// descending score. Entries with score <= 0 are excluded from the result.
// An empty query or empty entries slice returns an empty slice.
func Score(entries []IndexEntry, queryText string, taskType string, params BM25Params) []ScoredEntry {
	if len(entries) == 0 {
		return []ScoredEntry{}
	}

	queryTokens := Tokenize(queryText)
	if len(queryTokens) == 0 {
		return []ScoredEntry{}
	}

	// Tokenize all documents and compute their lengths.
	docTokens := make([][]string, len(entries))
	totalLen := 0
	for i, e := range entries {
		toks := Tokenize(e.RequestSummary)
		docTokens[i] = toks
		totalLen += len(toks)
	}

	N := len(entries)
	avgdl := float64(totalLen) / float64(N)

	// Build document frequency map: df[term] = number of docs containing term.
	df := make(map[string]int)
	for _, toks := range docTokens {
		seen := make(map[string]bool)
		for _, tok := range toks {
			if !seen[tok] {
				df[tok]++
				seen[tok] = true
			}
		}
	}

	// Score each document.
	scored := make([]ScoredEntry, 0, len(entries))
	for i, e := range entries {
		toks := docTokens[i]
		docLen := float64(len(toks))

		// Build term frequency map for this document.
		tf := make(map[string]int)
		for _, tok := range toks {
			tf[tok]++
		}

		// Compute BM25 score.
		var bm25 float64
		for _, qt := range queryTokens {
			dfQt := df[qt] // 0 if term not in corpus
			tfQt := float64(tf[qt])

			// IDF: log((N - df + 0.5) / (df + 0.5) + 1)
			idf := math.Log((float64(N)-float64(dfQt)+0.5)/(float64(dfQt)+0.5) + 1)

			// TF normalisation with length normalization.
			denominator := tfQt + params.K1*(1-params.B+params.B*docLen/avgdl)
			var tfNorm float64
			if denominator > 0 {
				tfNorm = tfQt * (params.K1 + 1) / denominator
			}

			bm25 += idf * tfNorm
		}

		if bm25 <= 0 {
			continue
		}

		// Apply taskType post-score multiplicative boost.
		typeBoost := 0.0
		if taskType != "" && e.TaskType != nil && *e.TaskType == taskType {
			typeBoost = 1.0
		}
		finalScore := bm25 * (1 + typeBoost)

		scored = append(scored, ScoredEntry{Entry: e, Score: finalScore})
	}

	// Sort descending by final score.
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	return scored
}

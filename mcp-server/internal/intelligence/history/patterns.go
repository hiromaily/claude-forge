package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/orchestrator"
)

// PatternEntry is one accumulated finding pattern.
type PatternEntry struct {
	Pattern   string    `json:"pattern"`  // normalised description text
	Severity  string    `json:"severity"` // "CRITICAL" or "MINOR"
	Frequency int       `json:"frequency"`
	Agent     string    `json:"agent"` // reviewer agent name, or "" for historical scans
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	Category  string    `json:"category"` // one of the 8 fixed categories or "other"
}

// PatternsFile is the on-disk JSON shape for .specs/patterns.json.
type PatternsFile struct {
	UpdatedAt            time.Time      `json:"updatedAt"`
	TotalReviewsAnalyzed int            `json:"totalReviewsAnalyzed"`
	Patterns             []PatternEntry `json:"patterns"`
}

// PatternAccumulator holds in-memory patterns and persists to specsDir/patterns.json.
type PatternAccumulator struct {
	mu                   sync.RWMutex
	specsDir             string
	patterns             []PatternEntry
	patternIdx           map[string][]int // "category|severity" → indices into patterns
	totalReviewsAnalyzed int
}

// patternCategories maps the eight fixed categories to their trigger keywords.
// A finding description that contains any keyword is classified into that category.
// The first matching category wins.
//
//nolint:gochecknoglobals // package-level lookup table; immutable after init
var patternCategories = map[string][]string{
	"error_handling": {
		"error handling", "error return", "unchecked error", "ignore error",
		"missing error", "error check", "silent fail", "panic", "exception",
		"err ", "error",
	},
	"import_order": {
		"import order", "import group", "import sort", "import cycle",
		"circular import",
	},
	"test_coverage": {
		"test coverage", "unit test", "missing test", "untested", "edge case",
		"table-driven", "coverage",
	},
	"naming_convention": {
		"naming convention", "inconsistent name", "variable name", "naming guide",
		"camelcase", "snake_case", "abbreviat", "naming",
	},
	"type_safety": {
		"type assertion", "type cast", "type mismatch", "type safety",
		"interface{}", "unsafe",
	},
	"security": {
		"security vulnerability", "security", "credential", "password",
		"injection", "sanitize", "sanitise", "xss", "authentication",
	},
	"performance": {
		"performance", "inefficien", "benchmark", "memory", "cpu",
		"string concat", "allocat",
	},
	"documentation": {
		"missing documentation", "godoc", "undocumented", "exported function",
		"exported type", "public api", "docstring", "documentation",
	},
}

// patternCategoryOrder defines the evaluation order for category classification.
// More specific categories are checked before more general ones.
//
//nolint:gochecknoglobals // package-level lookup table; immutable after init
var patternCategoryOrder = []string{
	"import_order",
	"test_coverage",
	"naming_convention",
	"type_safety",
	"security",
	"performance",
	"documentation",
	"error_handling",
}

// patternStopwords are common English words removed during normalisation.
// This is a minimal set focused on reducing noise in finding text comparisons.
//
//nolint:gochecknoglobals // package-level lookup table; immutable after init
var patternStopwords = map[string]struct{}{
	"a": {}, "an": {}, "the": {}, "is": {}, "are": {}, "was": {}, "were": {},
	"be": {}, "been": {}, "being": {}, "have": {}, "has": {}, "had": {},
	"do": {}, "does": {}, "did": {}, "will": {}, "would": {}, "could": {},
	"should": {}, "may": {}, "might": {}, "shall": {}, "can": {},
	"of": {}, "in": {}, "on": {}, "at": {}, "by": {}, "for": {}, "with": {},
	"to": {}, "from": {}, "that": {}, "this": {}, "it": {}, "its": {},
	"not": {}, "no": {}, "and": {}, "or": {}, "but": {}, "if": {}, "when": {},
	"where": {}, "how": {}, "what": {}, "which": {}, "who": {},
}

// NewPatternAccumulator creates a new PatternAccumulator with the given specsDir.
// It does not call Load automatically.
func NewPatternAccumulator(specsDir string) *PatternAccumulator {
	return &PatternAccumulator{
		specsDir:   specsDir,
		patterns:   []PatternEntry{},
		patternIdx: make(map[string][]int),
	}
}

// Entries returns a copy of the in-memory pattern entries.
func (a *PatternAccumulator) Entries() []PatternEntry {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make([]PatternEntry, len(a.patterns))
	copy(result, a.patterns)

	return result
}

// TotalReviewsAnalyzed returns the number of Accumulate calls made.
func (a *PatternAccumulator) TotalReviewsAnalyzed() int {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.totalReviewsAnalyzed
}

// normalise lowercases the description and removes stopwords, returning a
// space-joined token string for Levenshtein comparison.
func normalise(desc string) string {
	lower := strings.ToLower(desc)
	words := strings.FieldsFunc(lower, func(r rune) bool {
		return r == ' ' || r == '\t' || r == ',' || r == '.' || r == ':' ||
			r == ';' || r == '!' || r == '?' || r == '"' || r == '\''
	})

	tokens := make([]string, 0, len(words))

	for _, w := range words {
		if _, isStop := patternStopwords[w]; !isStop && w != "" {
			tokens = append(tokens, w)
		}
	}

	return strings.Join(tokens, " ")
}

// classifyPattern returns the category for the normalised description text.
// Returns "other" when no category matches.
func classifyPattern(normDesc string) string {
	lower := strings.ToLower(normDesc)

	for _, cat := range patternCategoryOrder {
		keywords, ok := patternCategories[cat]
		if !ok {
			continue
		}

		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				return cat
			}
		}
	}

	return "other"
}

// Accumulate processes a slice of findings, merges near-duplicate entries
// (same category, Levenshtein ratio < 0.3), and persists the result.
// Each call increments TotalReviewsAnalyzed by one regardless of findings count.
func (a *PatternAccumulator) Accumulate(findings []orchestrator.Finding, agent string, ts time.Time) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.totalReviewsAnalyzed++

	for _, f := range findings {
		norm := normalise(f.Description)
		if norm == "" {
			continue
		}

		cat := classifyPattern(norm)
		sev := string(f.Severity)
		bucketKey := cat + "|" + sev

		// Search for an existing pattern in the same category+severity bucket
		// that is near-identical. Using the index map limits comparisons to
		// candidates in the same bucket instead of scanning all patterns.
		merged := false

		for _, idx := range a.patternIdx[bucketKey] {
			existing := &a.patterns[idx]

			if levenshteinRatio(norm, existing.Pattern) < 0.3 {
				existing.Frequency++

				if ts.After(existing.LastSeen) {
					existing.LastSeen = ts
				}

				if ts.Before(existing.FirstSeen) {
					existing.FirstSeen = ts
				}

				merged = true

				break
			}
		}

		if !merged {
			newIdx := len(a.patterns)
			a.patterns = append(a.patterns, PatternEntry{
				Pattern:   norm,
				Severity:  sev,
				Frequency: 1,
				Agent:     agent,
				FirstSeen: ts,
				LastSeen:  ts,
				Category:  cat,
			})
			a.patternIdx[bucketKey] = append(a.patternIdx[bucketKey], newIdx)
		}
	}

	return a.persist()
}

// Query returns pattern entries filtered by agentFilter and severityFilter, capped to limit.
// Empty filter strings match all values.
func (a *PatternAccumulator) Query(agentFilter, severityFilter string, limit int) []PatternEntry {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make([]PatternEntry, 0, len(a.patterns))

	for _, e := range a.patterns {
		if agentFilter != "" && e.Agent != agentFilter {
			continue
		}

		if severityFilter != "" && e.Severity != severityFilter {
			continue
		}

		result = append(result, e)

		if limit > 0 && len(result) >= limit {
			break
		}
	}

	return result
}

// Load reads patterns.json from specsDir and restores in-memory state.
// If the file is absent, Load returns nil (empty state, fail-open).
// If the file is corrupted, Load returns the parse error.
func (a *PatternAccumulator) Load() error {
	patternsPath := filepath.Join(a.specsDir, "patterns.json")

	data, err := os.ReadFile(patternsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("read patterns.json: %w", err)
	}

	var pf PatternsFile
	if err := json.Unmarshal(data, &pf); err != nil {
		return fmt.Errorf("parse patterns.json: %w", err)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if pf.Patterns == nil {
		a.patterns = []PatternEntry{}
	} else {
		a.patterns = pf.Patterns
	}

	a.totalReviewsAnalyzed = pf.TotalReviewsAnalyzed

	// Rebuild the bucket index from the loaded patterns.
	a.patternIdx = make(map[string][]int, len(a.patterns))
	for i, p := range a.patterns {
		key := p.Category + "|" + p.Severity
		a.patternIdx[key] = append(a.patternIdx[key], i)
	}

	return nil
}

// persist writes the current in-memory state to specsDir/patterns.json.
// Must be called with a.mu held (write lock).
func (a *PatternAccumulator) persist() error {
	pf := PatternsFile{
		UpdatedAt:            time.Now().UTC(),
		TotalReviewsAnalyzed: a.totalReviewsAnalyzed,
		Patterns:             a.patterns,
	}

	if pf.Patterns == nil {
		pf.Patterns = []PatternEntry{}
	}

	data, err := json.MarshalIndent(pf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal patterns.json: %w", err)
	}

	patternsPath := filepath.Join(a.specsDir, "patterns.json")
	if err := os.WriteFile(patternsPath, data, 0o600); err != nil {
		return fmt.Errorf("write patterns.json: %w", err)
	}

	return nil
}

package history

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// FrictionPoint is one extracted friction item.
type FrictionPoint struct {
	Category    string `json:"category"`
	Description string `json:"description"`
	Frequency   int    `json:"frequency"`
	Mitigation  string `json:"mitigation"`
}

// FrictionFile is the on-disk JSON shape for .specs/friction.json.
type FrictionFile struct {
	UpdatedAt            time.Time       `json:"updatedAt"`
	TotalReportsAnalyzed int             `json:"totalReportsAnalyzed"`
	FrictionPoints       []FrictionPoint `json:"frictionPoints"`
}

// FrictionMap holds in-memory friction points and persists to specsDir/friction.json.
type FrictionMap struct {
	mu                   sync.RWMutex
	specsDir             string
	points               []FrictionPoint
	totalReportsAnalyzed int
}

// frictionKeywords maps the eight fixed categories to their trigger phrases.
// A line/bullet containing any of these phrases is classified into that category.
//
//nolint:gochecknoglobals // package-level lookup table; immutable after init
var frictionKeywords = map[string][]string{
	"error_handling": {
		"error", "err", "exception", "panic", "fail", "failure",
		"ignore error", "unchecked", "missing error", "error return",
		"silent fail",
	},
	"import_order": {
		"import order", "import group", "import sort", "import organisation",
		"import organization", "import cycle", "circular import",
	},
	"test_coverage": {
		"test coverage", "unit test", "missing test", "untested", "edge case",
		"test case", "coverage", "happy path", "table-driven",
	},
	"naming_convention": {
		"naming", "name", "variable name", "abbreviat", "camelcase", "snake_case",
		"inconsistent name", "naming guide", "naming convention",
	},
	"type_safety": {
		"type assert", "type cast", "interface{}", "any type", "unsafe",
		"type mismatch", "type safety", "reflection",
	},
	"security": {
		"security", "secret", "credential", "token", "password", "injection",
		"sanitize", "sanitise", "xss", "sql injection", "auth",
	},
	"performance": {
		"performance", "slow", "allocat", "o(n", "o(n^", "inefficien",
		"profile", "benchmark", "memory", "cpu", "string concat", "loop",
	},
	"documentation": {
		"document", "godoc", "comment", "docstring", "readme",
		"missing doc", "undocumented", "exported function", "public api",
	},
}

// NewFrictionMap creates a new FrictionMap with the given specsDir.
// It does not call Build or Load.
func NewFrictionMap(specsDir string) *FrictionMap {
	return &FrictionMap{
		specsDir: specsDir,
		points:   []FrictionPoint{},
	}
}

// Points returns a copy of the in-memory friction points.
func (m *FrictionMap) Points() []FrictionPoint {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]FrictionPoint, len(m.points))
	copy(result, m.points)

	return result
}

// TotalReportsAnalyzed returns the count of improvement reports analyzed.
func (m *FrictionMap) TotalReportsAnalyzed() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.totalReportsAnalyzed
}

// Build scans specsDir for improvement reports in subdirectories, extracts
// friction points from each, merges them into the in-memory store, and
// persists to friction.json. It is tolerant of absent directories and files.
//
// Two sources are scanned per spec directory (first match wins):
//  1. improvement.md — dedicated improvement report file
//  2. summary.md — the "## Improvement Report" section is extracted
func (m *FrictionMap) Build() error { //nolint:cyclop // complexity inherent in multi-category scan
	dirEntries, err := os.ReadDir(m.specsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("read specsDir: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Reset state for a fresh build.
	m.points = []FrictionPoint{}
	m.totalReportsAnalyzed = 0

	// Accumulate points across all reports; use a map to merge duplicates by key.
	pointMap := make(map[string]*FrictionPoint)

	for _, dirEntry := range dirEntries {
		if !dirEntry.IsDir() {
			continue
		}

		specDir := filepath.Join(m.specsDir, dirEntry.Name())
		text := readImprovementContent(specDir)
		if text == "" {
			continue
		}

		m.totalReportsAnalyzed++

		extractFrictionPoints(text, pointMap)
	}

	// Flatten pointMap to slice.
	result := make([]FrictionPoint, 0, len(pointMap))
	for _, fp := range pointMap {
		result = append(result, *fp)
	}

	m.points = result

	return m.persist()
}

// Load reads friction.json from specsDir and restores the in-memory state.
// If the file is absent, Load returns nil (empty state, fail-open).
// If the file is corrupted, Load returns the parse error.
func (m *FrictionMap) Load() error {
	frictionPath := filepath.Join(m.specsDir, "friction.json")

	data, err := os.ReadFile(frictionPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("read friction.json: %w", err)
	}

	var ff FrictionFile
	if err := json.Unmarshal(data, &ff); err != nil {
		return fmt.Errorf("parse friction.json: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if ff.FrictionPoints == nil {
		m.points = []FrictionPoint{}
	} else {
		m.points = ff.FrictionPoints
	}

	m.totalReportsAnalyzed = ff.TotalReportsAnalyzed

	return nil
}

// persist writes the current in-memory state to specsDir/friction.json.
// Must be called with m.mu held (write lock).
func (m *FrictionMap) persist() error {
	ff := FrictionFile{
		UpdatedAt:            time.Now().UTC(),
		TotalReportsAnalyzed: m.totalReportsAnalyzed,
		FrictionPoints:       m.points,
	}

	if ff.FrictionPoints == nil {
		ff.FrictionPoints = []FrictionPoint{}
	}

	data, err := json.MarshalIndent(ff, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal friction.json: %w", err)
	}

	frictionPath := filepath.Join(m.specsDir, "friction.json")
	if err := os.WriteFile(frictionPath, data, 0o600); err != nil {
		return fmt.Errorf("write friction.json: %w", err)
	}

	return nil
}

// readImprovementContent returns the improvement report text from a spec directory.
// It first looks for a dedicated improvement.md file. If absent, it falls back
// to extracting the "## Improvement Report" section from summary.md.
// Returns "" if neither source contains improvement content.
func readImprovementContent(specDir string) string {
	// Primary: dedicated improvement.md file.
	improvementPath := filepath.Join(specDir, "improvement.md")
	if data, err := os.ReadFile(improvementPath); err == nil {
		return string(data)
	}

	// Fallback: extract the ## Improvement Report section from summary.md.
	summaryPath := filepath.Join(specDir, "summary.md")
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		return ""
	}

	return extractSection(string(data), "## Improvement Report")
}

// extractSection returns the content of a markdown section starting with the
// given heading, up to (but not including) the next heading of equal or higher
// level or end of file. Returns "" if the heading is not found.
func extractSection(text, heading string) string {
	headingLevel := countLeadingHashes(heading)
	idx := strings.Index(text, heading)
	if idx < 0 {
		return ""
	}

	// Start after the heading line.
	start := idx + len(heading)
	if nlIdx := strings.Index(text[start:], "\n"); nlIdx >= 0 {
		start += nlIdx + 1
	}

	// Collect lines until a heading of equal or higher level is found.
	rest := text[start:]
	scanner := bufio.NewScanner(strings.NewReader(rest))
	var sb strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimLeft(line, " ")
		if strings.HasPrefix(trimmed, "#") {
			level := countLeadingHashes(trimmed)
			if level > 0 && level <= headingLevel {
				break
			}
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	return sb.String()
}

// countLeadingHashes returns the number of leading '#' characters in s.
func countLeadingHashes(s string) int {
	n := 0
	for _, c := range s {
		if c == '#' {
			n++
		} else {
			break
		}
	}
	return n
}

// extractFrictionPoints scans the text of an improvement report and adds
// detected friction points into pointMap (keyed by "category|description").
// Multiple occurrences of the same key increment Frequency.
func extractFrictionPoints(text string, pointMap map[string]*FrictionPoint) { //nolint:cyclop // multi-category classification is inherently complex
	scanner := bufio.NewScanner(strings.NewReader(text))

	var (
		mitigation         string // pending mitigation text for next friction point
		nextLineMitigation bool   // true when the next scanned line is a mitigation continuation
	)

	for scanner.Scan() {
		line := scanner.Text()
		lower := strings.ToLower(strings.TrimSpace(line))

		// Consume a deferred mitigation continuation line.
		if nextLineMitigation {
			mitigation = strings.TrimSpace(line)
			nextLineMitigation = false

			continue
		}

		// Reset mitigation on new section headings.
		if strings.HasPrefix(lower, "#") {
			mitigation = ""

			continue
		}

		// Detect mitigation hints on the current line or deferred to next line.
		if strings.Contains(lower, "mitigation:") || strings.HasPrefix(lower, "- mitigation") {
			after := strings.TrimPrefix(lower, "- mitigation:")
			after = strings.TrimPrefix(after, "mitigation:")
			after = strings.TrimSpace(after)

			if after != "" {
				mitigation = after
			} else {
				nextLineMitigation = true
			}

			continue
		}

		// Skip empty lines.
		if lower == "" {
			continue
		}

		// Skip lines that are purely bullet headers (e.g., "### Documentation").
		if strings.HasPrefix(line, "###") || strings.HasPrefix(line, "##") {
			continue
		}

		// Classify the line into a category based on keyword matching.
		category := classifyLine(lower)
		if category == "" {
			continue
		}

		// Build a concise description from the raw line.
		desc := buildDescription(line)
		if desc == "" {
			continue
		}

		key := category + "|" + desc

		if existing, ok := pointMap[key]; ok {
			existing.Frequency++
		} else {
			pointMap[key] = &FrictionPoint{
				Category:    category,
				Description: desc,
				Frequency:   1,
				Mitigation:  mitigation,
			}
		}
	}
}

// classifyLine returns the friction category for the given lowercased line,
// or "" if no category matches.
func classifyLine(lower string) string {
	for category, keywords := range frictionKeywords {
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				return category
			}
		}
	}

	return ""
}

// buildDescription trims bullet markers and leading/trailing whitespace from a line.
func buildDescription(line string) string {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimPrefix(trimmed, "- ")
	trimmed = strings.TrimPrefix(trimmed, "* ")
	trimmed = strings.TrimSpace(trimmed)

	if len(trimmed) > 200 {
		trimmed = trimmed[:200]
	}

	return trimmed
}

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

// Build scans specsDir for improvement.md files in subdirectories, extracts
// friction points from each file, merges them into the in-memory store, and
// persists to friction.json. It is tolerant of absent directories and absent
// improvement.md files.
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

		improvementPath := filepath.Join(m.specsDir, dirEntry.Name(), "improvement.md")

		data, readErr := os.ReadFile(improvementPath)
		if readErr != nil {
			// No improvement.md in this spec dir — skip silently.
			continue
		}

		m.totalReportsAnalyzed++

		extractFrictionPoints(string(data), pointMap)
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

// extractFrictionPoints scans the text of an improvement.md file and adds
// detected friction points into pointMap (keyed by "category|description").
// Multiple occurrences of the same key increment Frequency.
func extractFrictionPoints(text string, pointMap map[string]*FrictionPoint) { //nolint:cyclop // multi-category classification is inherently complex
	scanner := bufio.NewScanner(strings.NewReader(text))

	var (
		currentSection string // tracks the current markdown section heading
		mitigation     string // pending mitigation line
	)

	var lines []string

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	for i, line := range lines {
		lower := strings.ToLower(strings.TrimSpace(line))

		// Track section headings to detect "mitigation" sections.
		if strings.HasPrefix(lower, "#") {
			currentSection = strings.TrimLeft(lower, "# ")
			mitigation = ""
			_ = currentSection

			continue
		}

		// Detect mitigation hints on the current line or nearby lines.
		if strings.Contains(lower, "mitigation:") || strings.HasPrefix(lower, "- mitigation") {
			// Look ahead for the mitigation text on the same line or next line.
			after := strings.TrimPrefix(lower, "- mitigation:")
			after = strings.TrimPrefix(after, "mitigation:")
			after = strings.TrimSpace(after)

			if after != "" {
				mitigation = after
			} else if i+1 < len(lines) {
				mitigation = strings.TrimSpace(lines[i+1])
			}

			continue
		}

		// Skip empty lines and pure headings.
		if lower == "" || strings.HasPrefix(lower, "#") {
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

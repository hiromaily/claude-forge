// Package history provides a history index over completed/abandoned pipeline runs
// stored in .specs/ directories. It scans state.json and request.md files,
// builds IndexEntry records, persists them to .specs/history-index.json, and
// supports incremental (differential) updates using an indexedAt watermark.
package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	bm25 "github.com/hiromaily/claude-forge/mcp-server/internal/search"
)

// errSkip is a sentinel error returned by parseSpec when a spec should be skipped
// (non-terminal phase, missing state.json, or corrupted data).
var errSkip = errors.New("skip")

const maxTags = 50

// IndexEntry holds metadata for a single completed or abandoned pipeline run.
type IndexEntry struct {
	SpecName     string    `json:"specName"`
	OneLiner     string    `json:"oneLiner"`
	Effort       string    `json:"effort"`
	FlowTemplate string    `json:"flowTemplate"`
	Tags         []string  `json:"tags"`
	Outcome      string    `json:"outcome"`
	TokensTotal  int       `json:"tokensTotal"`
	DurationMs   int       `json:"durationMs"`
	CreatedAt    time.Time `json:"createdAt"`
}

// IndexFile is the on-disk JSON shape for history-index.json.
type IndexFile struct {
	IndexedAt time.Time    `json:"indexedAt"`
	Entries   []IndexEntry `json:"entries"`
}

// HistoryIndex holds the in-memory index of historical pipeline entries
// and the path to the specs directory.
type HistoryIndex struct {
	specsDir string
	entries  []IndexEntry
}

// New creates a new HistoryIndex with the given specsDir. It does not call Build.
// h.Size() returns 0 and h.Entries() returns an empty slice until Build is called.
func New(specsDir string) *HistoryIndex {
	return &HistoryIndex{
		specsDir: specsDir,
		entries:  []IndexEntry{},
	}
}

// Size returns the number of in-memory index entries.
func (h *HistoryIndex) Size() int {
	return len(h.entries)
}

// Entries returns a copy of the in-memory index entries.
func (h *HistoryIndex) Entries() []IndexEntry {
	result := make([]IndexEntry, len(h.entries))
	copy(result, h.entries)
	return result
}

// SpecsDir returns the resolved specs directory path used by this index.
func (h *HistoryIndex) SpecsDir() string {
	return h.specsDir
}

// Build scans specsDir, loads the existing history-index.json (if any), adds
// new terminal specs (completed/abandoned) that are newer than the indexedAt
// watermark, and writes the updated index back to history-index.json.
// It is idempotent: calling Build twice does not produce duplicate SpecName entries.
func (h *HistoryIndex) Build() error {
	resolvedDir := resolveSpecsDir(h.specsDir)
	h.specsDir = resolvedDir

	indexPath := filepath.Join(resolvedDir, "history-index.json")

	// Load existing index file (empty IndexFile if absent).
	existing, err := loadExisting(indexPath)
	if err != nil {
		return fmt.Errorf("load existing index: %w", err)
	}

	// Build a set of already-indexed spec names to avoid duplicates.
	alreadyIndexed := make(map[string]bool, len(existing.Entries))
	for _, e := range existing.Entries {
		alreadyIndexed[e.SpecName] = true
	}

	// Scan specsDir for subdirectories (each subdirectory may be a spec).
	dirEntries, err := os.ReadDir(resolvedDir)
	if err != nil {
		if os.IsNotExist(err) {
			// specsDir does not exist yet -- nothing to index.
			return nil
		}

		return fmt.Errorf("read specsDir: %w", err)
	}

	newEntries := make([]IndexEntry, 0)

	for _, dirEntry := range dirEntries {
		if !dirEntry.IsDir() {
			continue
		}

		specDir := filepath.Join(resolvedDir, dirEntry.Name())
		entry, parseErr := parseSpec(specDir, existing.IndexedAt)

		if parseErr != nil {
			if errors.Is(parseErr, errSkip) {
				continue
			}

			// Non-skip errors are non-fatal; continue processing other specs.
			continue
		}

		// Skip specs already in the index.
		if alreadyIndexed[entry.SpecName] {
			continue
		}

		newEntries = append(newEntries, entry)
	}

	// Merge new entries into existing.
	merged := make([]IndexEntry, 0, len(existing.Entries)+len(newEntries))
	merged = append(merged, existing.Entries...)
	merged = append(merged, newEntries...)

	// Write updated index file.
	updated := IndexFile{
		IndexedAt: time.Now().UTC(),
		Entries:   merged,
	}

	data, err := json.MarshalIndent(updated, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal index: %w", err)
	}

	if err := os.WriteFile(indexPath, data, 0o600); err != nil {
		return fmt.Errorf("write index: %w", err)
	}

	// Update in-memory entries.
	h.entries = merged

	return nil
}

// resolveSpecsDir applies 3-stage resolution for the specs directory:
// Stage 1: use specsDir if non-empty.
// Stage 2: derive from runtime.Caller(0) source path if derived .specs/ exists.
// Stage 3: fall back to literal ".specs".
func resolveSpecsDir(specsDir string) string {
	if specsDir != "" {
		return specsDir
	}

	// Stage 2: derive from the source file location.
	_, filename, _, ok := runtime.Caller(0)
	if ok {
		// filename is .../mcp-server/history/index.go; go up two levels to repo root.
		repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(filename)))
		candidate := filepath.Join(repoRoot, ".specs")

		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// Stage 3: literal fallback.
	return ".specs"
}

// loadExisting reads and parses history-index.json from path.
// Returns an empty IndexFile (with zero-value IndexedAt) if the file is absent.
func loadExisting(path string) (IndexFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return IndexFile{}, nil
		}

		return IndexFile{}, fmt.Errorf("read index file: %w", err)
	}

	var idx IndexFile
	if err := json.Unmarshal(data, &idx); err != nil {
		// Corrupted index file -- treat as empty to allow rebuild.
		return IndexFile{}, nil
	}

	return idx, nil
}

// stateJSON mirrors the relevant fields from state.json for parsing.
type stateJSON struct {
	SpecName     string     `json:"specName"`
	CurrentPhase string     `json:"currentPhase"`
	Effort       *string    `json:"effort"`
	FlowTemplate *string    `json:"flowTemplate"`
	PhaseLog     []phaseLog `json:"phaseLog"`
	Timestamps   timestamps `json:"timestamps"`
}

type phaseLog struct {
	Tokens     int `json:"tokens"`
	DurationMs int `json:"duration_ms"`
}

type timestamps struct {
	Created     string `json:"created"`
	LastUpdated string `json:"lastUpdated"`
}

// parseSpec reads state.json and request.md for the given specDir.
// Returns errSkip if:
//   - state.json is absent
//   - currentPhase is not "completed" or "abandoned"
//   - state.json is corrupted
//   - the spec's lastUpdated timestamp is before or equal to indexedAt (watermark)
//
// The indexedAt parameter is the watermark from the existing index; if zero,
// all terminal specs are parsed.
func parseSpec(specDir string, indexedAt time.Time) (IndexEntry, error) { //nolint:cyclop // complexity is inherent in multi-field parsing
	stateData, err := os.ReadFile(filepath.Join(specDir, "state.json"))
	if err != nil {
		return IndexEntry{}, errSkip
	}

	var st stateJSON
	if err := json.Unmarshal(stateData, &st); err != nil {
		return IndexEntry{}, errSkip
	}

	// Only index terminal states.
	if st.CurrentPhase != "completed" && st.CurrentPhase != "abandoned" {
		return IndexEntry{}, errSkip
	}

	// Differential update: skip specs whose lastUpdated is before the watermark.
	if !indexedAt.IsZero() {
		lastUpdated, parseErr := time.Parse(time.RFC3339, st.Timestamps.LastUpdated)
		if parseErr != nil {
			// Fall back to created timestamp.
			lastUpdated, parseErr = time.Parse(time.RFC3339, st.Timestamps.Created)
			if parseErr != nil {
				// Cannot determine timestamp; skip to be safe.
				return IndexEntry{}, errSkip
			}
		}

		if !lastUpdated.After(indexedAt) {
			return IndexEntry{}, errSkip
		}
	}

	// Parse request.md (optional).
	var oneLiner string

	var tags []string

	requestData, err := os.ReadFile(filepath.Join(specDir, "request.md"))
	if err == nil {
		body := stripFrontmatter(string(requestData))
		oneLiner = extractOneLiner(body)
		tags = extractTags(body)
	}

	// Sum token/duration from phaseLog.
	var tokensTotal, durationMs int
	for _, pl := range st.PhaseLog {
		tokensTotal += pl.Tokens
		durationMs += pl.DurationMs
	}

	// Parse createdAt.
	var createdAt time.Time

	if st.Timestamps.Created != "" {
		parsedTime, parseErr := time.Parse(time.RFC3339, st.Timestamps.Created)
		if parseErr != nil {
			return IndexEntry{}, errSkip
		}
		createdAt = parsedTime
	}

	// Use specName from state.json or fall back to directory name.
	specName := st.SpecName
	if specName == "" {
		specName = filepath.Base(specDir)
	}

	// Dereference pointer fields.
	effort := ""
	if st.Effort != nil {
		effort = *st.Effort
	}

	flowTemplate := ""
	if st.FlowTemplate != nil {
		flowTemplate = *st.FlowTemplate
	}

	return IndexEntry{
		SpecName:     specName,
		OneLiner:     oneLiner,
		Effort:       effort,
		FlowTemplate: flowTemplate,
		Tags:         tags,
		Outcome:      st.CurrentPhase,
		TokensTotal:  tokensTotal,
		DurationMs:   durationMs,
		CreatedAt:    createdAt,
	}, nil
}

// stripFrontmatter removes YAML frontmatter delimited by leading --- lines.
func stripFrontmatter(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || lines[0] != "---" {
		return content
	}

	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			endIdx = i
			break
		}
	}

	if endIdx == -1 {
		return content
	}

	if endIdx+1 >= len(lines) {
		return ""
	}

	return strings.Join(lines[endIdx+1:], "\n")
}

// extractOneLiner returns the first non-empty line from content after stripping
// Markdown heading markers.
func extractOneLiner(content string) string {
	for line := range strings.SplitSeq(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Strip Markdown heading markers.
		trimmed = strings.TrimLeft(trimmed, "#")
		trimmed = strings.TrimSpace(trimmed)

		if trimmed != "" {
			return trimmed
		}
	}

	return ""
}

// extractTags tokenizes content using search.Tokenize, deduplicates, and caps at maxTags.
func extractTags(content string) []string {
	tokens := bm25.Tokenize(content)

	seen := make(map[string]bool, len(tokens))
	result := make([]string, 0, len(tokens))

	for _, tok := range tokens {
		if seen[tok] {
			continue
		}

		seen[tok] = true
		result = append(result, tok)

		if len(result) >= maxTags {
			break
		}
	}

	return result
}

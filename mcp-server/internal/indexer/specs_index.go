// Package indexer implements the pure-Go replacement for build-specs-index.sh.
// It scans a .specs/ directory, extracts metadata from each workspace, and writes
// an index.json file used by the BM25 search_patterns MCP tool.
package indexer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hiromaily/claude-forge/mcp-server/internal/search"
)

// Package-level compiled regexes — compiled once at startup.
var (
	spaceRe         = regexp.MustCompile(`\s{2,}`)
	findingRe       = regexp.MustCompile(`\*\*\d+\. \[(?:CRITICAL|MINOR)\][^*]+\*\*`)
	reviewVerdictRe = regexp.MustCompile(`APPROVE(?:_WITH_NOTES)?|REVISE`)
	implVerdictRe   = regexp.MustCompile(`PASS_WITH_NOTES|PASS|FAIL`)
	backtickRe      = regexp.MustCompile("`([^`]+)`")
	sectionRe       = regexp.MustCompile(`(?i)^## [Ff]iles ([Mm]odified|[Cc]reated( or [Mm]odified)?)`)
)

// BuildSpecsIndex scans specsDir for workspace subdirectories, extracts fields
// from each workspace's state.json and artifact .md files, and writes
// specsDir/index.json. Returns the number of entries written and any error.
func BuildSpecsIndex(specsDir string) (int, error) {
	dirEntries, err := os.ReadDir(specsDir)
	if err != nil {
		return 0, err
	}

	entries := make([]search.IndexEntry, 0)

	for _, de := range dirEntries {
		if !de.IsDir() {
			continue
		}

		workspaceDir := filepath.Join(specsDir, de.Name())
		entries = append(entries, buildEntry(workspaceDir))
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return 0, err
	}

	outputFile := filepath.Join(specsDir, "index.json")
	if err := os.WriteFile(outputFile, append(data, '\n'), 0o600); err != nil {
		return 0, err
	}

	return len(entries), nil
}

// buildEntry assembles one IndexEntry for a workspace directory.
func buildEntry(workspaceDir string) search.IndexEntry {
	dirBasename := filepath.Base(workspaceDir)

	// Read state.json if present.
	stateFile := filepath.Join(workspaceDir, "state.json")
	specName := dirBasename
	timestamp := "unknown"
	var taskType *string

	if data, err := os.ReadFile(stateFile); err == nil {
		var state struct {
			SpecName   string  `json:"specName"`
			TaskType   *string `json:"taskType"`
			Timestamps *struct {
				Created string `json:"created"`
			} `json:"timestamps"`
		}

		if jsonErr := json.Unmarshal(data, &state); jsonErr == nil {
			if state.SpecName != "" {
				specName = state.SpecName
			}

			if state.Timestamps != nil && state.Timestamps.Created != "" {
				timestamp = state.Timestamps.Created
			}

			taskType = state.TaskType
		}
	}

	outcome := deriveOutcome(stateFile)
	requestSummary := extractRequestSummary(workspaceDir)
	reviewFeedback := extractReviewFeedback(workspaceDir)
	implOutcomes := extractImplOutcomes(workspaceDir)
	implPatterns := extractImplPatterns(workspaceDir)

	return search.IndexEntry{
		SpecName:       specName,
		Timestamp:      timestamp,
		TaskType:       taskType,
		RequestSummary: requestSummary,
		ReviewFeedback: reviewFeedback,
		ImplOutcomes:   implOutcomes,
		ImplPatterns:   implPatterns,
		Outcome:        outcome,
	}
}

// extractRequestSummary reads request.md, strips YAML frontmatter delimited by
// "---" lines, and returns the first 200 characters of the body.
func extractRequestSummary(workspaceDir string) string {
	reqFile := filepath.Join(workspaceDir, "request.md")

	data, err := os.ReadFile(reqFile)
	if err != nil {
		return ""
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	// Strip YAML frontmatter only when the file starts with "---".
	// A "---" appearing later in the document is a markdown horizontal rule, not frontmatter.
	bodyStartIndex := 0
	if len(lines) > 0 && strings.TrimRight(lines[0], " \t\r") == "---" {
		endFound := false
		for i := 1; i < len(lines); i++ {
			if strings.TrimRight(lines[i], " \t\r") == "---" {
				bodyStartIndex = i + 1
				endFound = true
				break
			}
		}
		if !endFound {
			bodyStartIndex = len(lines) // no body if closing delimiter is absent
		}
	}

	body := strings.Join(lines[bodyStartIndex:], " ")
	// Normalize multiple whitespace runs (including those from newline→space conversion).
	body = normalizeWhitespace(body)

	if len(body) > 200 {
		return body[:200]
	}

	return body
}

// normalizeWhitespace trims leading/trailing whitespace and collapses internal
// runs of whitespace to a single space, mirroring the shell sed/tr pipeline.
func normalizeWhitespace(s string) string {
	s = strings.TrimSpace(s)
	return spaceRe.ReplaceAllString(s, " ")
}

// extractReviewFeedback returns a slice of REVISE-verdict feedback objects
// from review-design.md and review-tasks.md.
func extractReviewFeedback(workspaceDir string) []search.ReviewFeedback {
	result := make([]search.ReviewFeedback, 0)

	for _, sourceKey := range []string{"review-design", "review-tasks"} {
		reviewFile := filepath.Join(workspaceDir, sourceKey+".md")

		data, err := os.ReadFile(reviewFile)
		if err != nil {
			continue
		}

		content := string(data)

		// Detect verdict: APPROVE(_WITH_NOTES)? or REVISE.
		verdict := extractVerdict(content, reviewVerdictRe)
		if verdict != "REVISE" {
			continue
		}

		// Extract findings: **N. [CRITICAL] Title** or **N. [MINOR] Title**
		findings := extractFindings(content)

		result = append(result, search.ReviewFeedback{
			Source:   sourceKey,
			Verdict:  "REVISE",
			Findings: findings,
		})
	}

	return result
}

// extractVerdict finds the first occurrence of a verdict pattern in content.
func extractVerdict(content string, re *regexp.Regexp) string {
	return re.FindString(content)
}

// extractFindings extracts finding titles matching the pattern **N. [CRITICAL|MINOR] Title**.
func extractFindings(content string) []string {
	matches := findingRe.FindAllString(content, -1)

	result := make([]string, 0, len(matches))

	for _, m := range matches {
		// Strip surrounding **...**
		trimmed := strings.TrimPrefix(m, "**")
		trimmed = strings.TrimSuffix(trimmed, "**")
		result = append(result, trimmed)
	}

	return result
}

// extractImplOutcomes returns a slice of impl outcome objects from review-[0-9]*.md files.
// The glob pattern "review-[0-9]*.md" excludes review-design.md and review-tasks.md.
func extractImplOutcomes(workspaceDir string) []search.ImplOutcome {
	result := make([]search.ImplOutcome, 0)

	pattern := filepath.Join(workspaceDir, "review-[0-9]*.md")

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return result
	}

	for _, reviewFile := range matches {
		data, err := os.ReadFile(reviewFile)
		if err != nil {
			continue
		}

		content := string(data)

		// PASS_WITH_NOTES must come before PASS — leftmost match wins in ERE.
		verdict := extractVerdict(content, implVerdictRe)
		if verdict == "" {
			continue
		}

		result = append(result, search.ImplOutcome{
			ReviewFile: filepath.Base(reviewFile),
			Verdict:    verdict,
		})
	}

	return result
}

// extractImplPatterns returns a slice of impl pattern objects from impl-[0-9]*.md files.
// Each object has a TaskTitle (from the first # heading) and FilesModified (capped at 5).
func extractImplPatterns(workspaceDir string) []search.ImplPattern {
	result := make([]search.ImplPattern, 0)

	pattern := filepath.Join(workspaceDir, "impl-[0-9]*.md")

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return result
	}

	for _, implFile := range matches {
		data, err := os.ReadFile(implFile)
		if err != nil {
			continue
		}

		content := string(data)
		taskTitle := extractTaskTitle(content)
		filesModified := extractFilesModified(content)

		result = append(result, search.ImplPattern{
			TaskTitle:     taskTitle,
			FilesModified: filesModified,
		})
	}

	return result
}

// extractTaskTitle returns the text of the first "# " heading in content.
func extractTaskTitle(content string) string {
	for line := range strings.SplitSeq(content, "\n") {
		if after, ok := strings.CutPrefix(line, "# "); ok {
			return after
		}
	}

	return ""
}

// extractFilesModified extracts file paths from ## Files Modified/Created/Created or Modified
// sections, capping the result at 5. Handles both backtick-enclosed and plain paths.
func extractFilesModified(content string) []string {
	result := make([]string, 0, 5) //nolint:mnd // max 5 files per impl entry

	lines := strings.Split(content, "\n")
	inSection := false

	for _, line := range lines {
		if sectionRe.MatchString(line) {
			inSection = true
			continue
		}

		if strings.HasPrefix(line, "## ") {
			inSection = false
			continue
		}

		if !inSection {
			continue
		}

		if len(result) >= 5 { //nolint:mnd // max 5 files per impl entry
			break
		}

		// Only process bullet lines.
		if !strings.HasPrefix(line, "- ") && !strings.HasPrefix(line, "* ") {
			continue
		}

		bulletContent := line[2:] // strip "- " or "* "

		extracted := extractPathFromBullet(bulletContent, backtickRe)
		if extracted == "" {
			continue
		}

		// Strip absolute path prefix up to and including "claude-forge/".
		if idx := strings.Index(extracted, "claude-forge/"); idx >= 0 {
			extracted = extracted[idx+len("claude-forge/"):]
		}

		extracted = strings.TrimSpace(extracted)
		if extracted != "" {
			result = append(result, extracted)
		}
	}

	return result
}

// extractPathFromBullet tries to extract a file path from a bullet line.
// Strategy: first try backtick-enclosed path, then plain path without bold markers.
func extractPathFromBullet(line string, backtickRe *regexp.Regexp) string {
	// Strategy 1: backtick-enclosed path.
	if m := backtickRe.FindStringSubmatch(line); m != nil {
		candidate := m[1]
		// Validate: must contain "/" or match a file-extension pattern.
		if strings.Contains(candidate, "/") || isFilePath(candidate) {
			return candidate
		}
	}

	// Strategy 2: plain path (not bold-marked).
	if strings.HasPrefix(line, "**") {
		return ""
	}

	if strings.Contains(line, "/") || isFilePath(line) {
		// Strip any trailing annotation after whitespace (e.g. "path/file.go — description").
		// Take only the first whitespace-delimited token.
		parts := strings.Fields(line)
		if len(parts) > 0 {
			return parts[0]
		}
	}

	return ""
}

// isFilePath returns true if s looks like a file name (has a common file extension).
var fileExtRe = regexp.MustCompile(`\.[a-zA-Z0-9]+$`)

func isFilePath(s string) bool {
	return fileExtRe.MatchString(s)
}

// deriveOutcome maps the state.json content at stateFile to a canonical outcome string.
// Returns: completed | in_progress | abandoned | failed | unknown.
func deriveOutcome(stateFile string) string {
	data, err := os.ReadFile(stateFile)
	if err != nil {
		// No state file — unknown outcome.
		return "unknown"
	}

	var state struct {
		CurrentPhase       string `json:"currentPhase"`
		CurrentPhaseStatus string `json:"currentPhaseStatus"`
	}

	if err := json.Unmarshal(data, &state); err != nil {
		return "unknown"
	}

	switch {
	case state.CurrentPhaseStatus == "abandoned":
		return "abandoned"
	case state.CurrentPhaseStatus == "failed":
		return "failed"
	case state.CurrentPhase == "post-to-source" && state.CurrentPhaseStatus == "completed":
		return "completed"
	case state.CurrentPhase == "completed" && state.CurrentPhaseStatus == "completed":
		return "completed"
	case state.CurrentPhase != "":
		return "in_progress"
	default:
		return "unknown"
	}
}

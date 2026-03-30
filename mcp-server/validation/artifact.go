// Package validation implements input and artifact validation logic for
// the forge-state MCP server.
package validation

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ArtifactResult is the structured result returned by ValidateArtifacts for a
// single artifact file. For phase-6, ValidateArtifacts returns one element per
// impl-*.md file found; for all other phases it returns exactly one element.
type ArtifactResult struct {
	Valid           bool           `json:"valid"`
	File            string         `json:"file"`
	VerdictFound    string         `json:"verdict_found"`
	FindingsCount   *FindingsCount `json:"findings_count"`
	MissingSections []string       `json:"missing_sections"`
	Error           string         `json:"error,omitempty"`
}

// FindingsCount holds counts of CRITICAL and MINOR findings in a review artifact.
type FindingsCount struct {
	Critical int `json:"CRITICAL"`
	Minor    int `json:"MINOR"`
}

// artifactRule describes the validation constraints for a phase artifact file.
type artifactRule struct {
	filename        string
	requiredHeading string   // non-empty: file must contain this substring
	verdictSet      []string // non-nil: one of these must appear in the file
	requireNonEmpty bool     // true: any non-empty content suffices
}

// artifactRules maps phase identifiers to their validation rules.
// phase-6 is handled separately by validateArtifactPhase6.
//
//nolint:gochecknoglobals // package-level lookup table for phase rules
var artifactRules = map[string]artifactRule{
	"phase-3":            {filename: "design.md", requiredHeading: "## "},
	"phase-3b":           {filename: "review-design.md", verdictSet: []string{"APPROVE_WITH_NOTES", "APPROVE", "REVISE"}},
	"phase-4":            {filename: "tasks.md", requiredHeading: "## Task"},
	"phase-4b":           {filename: "review-tasks.md", verdictSet: []string{"APPROVE_WITH_NOTES", "APPROVE", "REVISE"}},
	"phase-7":            {filename: "verification.md", requireNonEmpty: true},
	"final-summary":      {filename: "comprehensive-review.md", requireNonEmpty: true},
	"final-verification": {filename: "final-verification.md", requireNonEmpty: true},
}

// ValidateArtifacts checks that the expected artifact file exists in workspace
// for the given phase and that it meets the required content constraints.
//
// For all phases except phase-6 the returned slice contains exactly one element.
// For phase-6 the slice contains one element per impl-*.md file found in
// workspace, or one element with valid=false if no impl-*.md files exist.
func ValidateArtifacts(workspace, phase string) []ArtifactResult {
	if phase == "phase-6" {
		return validateArtifactPhase6(workspace)
	}

	rule, ok := artifactRules[phase]
	if !ok {
		return []ArtifactResult{
			{
				Valid: false,
				Error: "unknown phase: " + phase,
			},
		}
	}

	return []ArtifactResult{validateArtifactRule(workspace, rule)}
}

// validateArtifactRule validates a single artifact against its rule.
func validateArtifactRule(workspace string, rule artifactRule) ArtifactResult {
	filePath := filepath.Join(workspace, rule.filename)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return ArtifactResult{
			Valid: false,
			File:  rule.filename,
			Error: "file not found: " + rule.filename,
		}
	}

	content := string(data)

	// Check required heading.
	if rule.requiredHeading != "" {
		if !strings.Contains(content, rule.requiredHeading) {
			return ArtifactResult{
				Valid: false,
				File:  rule.filename,
				Error: "missing required heading " + rule.requiredHeading + " in " + rule.filename,
			}
		}

		return ArtifactResult{
			Valid: true,
			File:  rule.filename,
		}
	}

	// Check verdict set.
	if rule.verdictSet != nil {
		verdict := findVerdict(content, rule.verdictSet)
		if verdict == "" {
			return ArtifactResult{
				Valid: false,
				File:  rule.filename,
				Error: "no verdict keyword found in " + rule.filename + " (expected one of: " + strings.Join(rule.verdictSet, ", ") + ")",
			}
		}

		fc := countFindings(content)

		return ArtifactResult{
			Valid:         true,
			File:          rule.filename,
			VerdictFound:  verdict,
			FindingsCount: fc,
		}
	}

	// Check non-empty.
	if rule.requireNonEmpty {
		if strings.TrimSpace(content) == "" {
			return ArtifactResult{
				Valid: false,
				File:  rule.filename,
				Error: "file " + rule.filename + " is empty",
			}
		}

		return ArtifactResult{
			Valid: true,
			File:  rule.filename,
		}
	}

	return ArtifactResult{
		Valid: true,
		File:  rule.filename,
	}
}

// findVerdict scans content for the first matching verdict keyword from verdictSet.
// The verdictSet should list longer/more-specific verdicts before shorter ones
// (e.g., APPROVE_WITH_NOTES before APPROVE) to avoid premature matching.
func findVerdict(content string, verdictSet []string) string {
	for _, v := range verdictSet {
		if strings.Contains(content, v) {
			return v
		}
	}

	return ""
}

// countFindings counts occurrences of [CRITICAL] and [MINOR] patterns in content.
func countFindings(content string) *FindingsCount {
	return &FindingsCount{
		Critical: strings.Count(content, "[CRITICAL]"),
		Minor:    strings.Count(content, "[MINOR]"),
	}
}

// validateArtifactPhase6 validates impl-*.md files for phase-6.
// Returns one result per impl-*.md file, or one error result if no files found.
func validateArtifactPhase6(workspace string) []ArtifactResult {
	pattern := filepath.Join(workspace, "impl-*.md")

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return []ArtifactResult{{
			Valid: false,
			Error: "error searching for impl-*.md files: " + err.Error(),
		}}
	}
	if len(matches) == 0 {
		return []ArtifactResult{{
			Valid: false,
			Error: "no impl-*.md files found in workspace",
		}}
	}
	sort.Strings(matches)

	verdictSet := []string{"PASS_WITH_NOTES", "PASS", "FAIL"}
	results := make([]ArtifactResult, 0, len(matches))

	for _, match := range matches {
		filename := filepath.Base(match)

		data, err := os.ReadFile(match)
		if err != nil {
			results = append(results, ArtifactResult{
				Valid: false,
				File:  filename,
				Error: "could not read " + filename + ": " + err.Error(),
			})

			continue
		}

		content := string(data)
		verdict := findVerdict(content, verdictSet)

		if verdict == "" {
			results = append(results, ArtifactResult{
				Valid: false,
				File:  filename,
				Error: "no PASS/PASS_WITH_NOTES/FAIL verdict found in " + filename,
			})

			continue
		}

		// FAIL verdict means structurally valid but review result is a failure.
		results = append(results, ArtifactResult{
			Valid:        verdict != "FAIL",
			File:         filename,
			VerdictFound: verdict,
		})
	}

	return results
}

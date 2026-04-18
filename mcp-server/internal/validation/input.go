// provides input and artifact validation logic for the
// claude-forge MCP server. It replaces the shell-script validate-input.sh and
// the pre-tool-hook artifact checks with typed Go functions.

package validation

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/hiromaily/claude-forge/mcp-server/internal/sourcetype"
)

// InputResult is the structured result returned by ValidateInput.
type InputResult struct {
	Valid  bool        `json:"valid"`
	Errors []string    `json:"errors"`
	Parsed ParsedInput `json:"parsed"`
}

// ParsedInput holds the parsed components extracted from the raw arguments string.
type ParsedInput struct {
	Flags      map[string]string `json:"flags"`
	BareFlags  []string          `json:"bare_flags"`
	CoreText   string            `json:"core_text"`
	SourceType string            `json:"source_type"` // "github_issue","jira_issue","linear_issue","workspace","text"
}

// effortXS is the effort level that is explicitly not supported.
// Valid efforts are S, M, L; XS is rejected at input validation time.
const effortXS = "XS"

// Compiled regexps for flag stripping.
// Key-value flags: --type=<val> and --effort=<val>.
var reKeyValueFlag = regexp.MustCompile(`--(?:type|effort)=[^\s]+`)

// Bare flags (word-boundary aware): --auto, --nopr, --debug, --discuss.
// Each pattern matches the flag only when preceded by start-of-string or
// whitespace and followed by end-of-string or whitespace.
// --resume is stripped from input for backward compatibility but is NOT
// added to BareFlags — resume is now auto-detected from .specs/ directory existence.
var (
	reBareAuto    = regexp.MustCompile(`(?:^|\s)--auto(?:\s|$)`)
	reBareNopr    = regexp.MustCompile(`(?:^|\s)--nopr(?:\s|$)`)
	reBareDebug   = regexp.MustCompile(`(?:^|\s)--debug(?:\s|$)`)
	reBareDiscuss = regexp.MustCompile(`(?:^|\s)--discuss(?:\s|$)`)
	reBareResume  = regexp.MustCompile(`(?:^|\s)--resume(?:\s|$)`)
)

// Regexp for URL detection (not classification — classification is delegated to sourcetype.ClassifyURL).
var reHTTPS = regexp.MustCompile(`^https?://`)

// Regexps for parsing flags into the Flags map.
var (
	reTypeFlag   = regexp.MustCompile(`--type=([^\s]+)`)
	reEffortFlag = regexp.MustCompile(`--effort=([^\s]+)`)
)

// ValidateInput validates the raw arguments string passed to the forge pipeline.
// It replicates the logic of scripts/validate-input.sh (checks 1-8) without
// the marker-file write (step 9, which is dropped in the MCP-side flow).
//
// SourceType is set to "workspace" when CoreText contains ".specs/" (string-only
// detection; no filesystem stat call is performed).
func ValidateInput(arguments string) InputResult {
	// Check 1: empty input.
	trimmed := strings.TrimSpace(arguments)
	if trimmed == "" {
		return InputResult{
			Valid:  false,
			Errors: []string{"ERROR: No task description provided. Please provide a development task, GitHub Issue URL, Jira Issue URL, or Linear Issue URL. (empty input)"},
		}
	}

	// Parse flags.
	flags, bareFlags := parseFlags(trimmed)

	// Check: XS effort is not supported.
	if flags["effort"] == effortXS {
		return InputResult{
			Valid:  false,
			Errors: []string{`effort "XS" is not supported; valid efforts are: S, M, L`},
		}
	}

	// Strip flags to get the core task description.
	core := stripFlags(trimmed)

	// Check: only flags provided, no actual task.
	if core == "" {
		return InputResult{
			Valid:  false,
			Errors: []string{"ERROR: Only flags provided, no task description. Please provide a development task after the flags. (empty input)"},
		}
	}

	// Classify input type.
	isURL := reHTTPS.MatchString(core)
	isWorkspace := strings.Contains(core, ".specs/")

	// Check 2: minimum length (skip for URLs and workspace paths).
	if !isURL && !isWorkspace {
		if len([]rune(core)) < 5 {
			return InputResult{
				Valid:  false,
				Errors: []string{"ERROR: Task description too short (" + itoa(len([]rune(core))) + " chars). Please provide a more specific description (minimum 5 characters). (too short)"},
			}
		}
	}

	// Check 3: URL format validation.
	if isURL {
		return validateURL(core, flags, bareFlags)
	}

	// Determine source type for non-URL inputs.
	sourceType := "text"
	if isWorkspace {
		sourceType = "workspace"
	}

	return InputResult{
		Valid: true,
		Parsed: ParsedInput{
			Flags:      flags,
			BareFlags:  normalizeBareFlags(bareFlags),
			CoreText:   core,
			SourceType: sourceType,
		},
	}
}

// validateURL checks the URL format and returns the appropriate InputResult.
func validateURL(core string, flags map[string]string, bareFlags []string) InputResult {
	sourceType, err := sourcetype.ClassifyURL(core)
	if err != nil {
		return InputResult{
			Valid:  false,
			Errors: []string{err.Error()},
		}
	}
	return InputResult{
		Valid: true,
		Parsed: ParsedInput{
			Flags:      flags,
			BareFlags:  normalizeBareFlags(bareFlags),
			CoreText:   core,
			SourceType: sourceType,
		},
	}
}

// parseFlags extracts key-value flags and bare flags from the trimmed input.
func parseFlags(trimmed string) (map[string]string, []string) {
	flags := make(map[string]string)
	var bareFlags []string

	if m := reTypeFlag.FindStringSubmatch(trimmed); len(m) == 2 {
		flags["type"] = m[1]
	}
	if m := reEffortFlag.FindStringSubmatch(trimmed); len(m) == 2 {
		flags["effort"] = m[1]
	}

	// Check for bare flags using word-boundary aware patterns.
	padded := " " + trimmed + " "
	if reBareAuto.MatchString(padded) {
		bareFlags = append(bareFlags, "auto")
	}
	if reBareNopr.MatchString(padded) {
		bareFlags = append(bareFlags, "nopr")
	}
	if reBareDebug.MatchString(padded) {
		bareFlags = append(bareFlags, "debug")
	}
	if reBareDiscuss.MatchString(padded) {
		bareFlags = append(bareFlags, "discuss")
	}
	return flags, bareFlags
}

// stripFlags removes all known flag patterns from s and returns the trimmed result.
func stripFlags(s string) string {
	// Remove --type=<val> and --effort=<val>.
	s = reKeyValueFlag.ReplaceAllString(s, " ")

	// Remove bare flags (word-boundary aware).
	// Prepend and append a space so the boundary patterns work at the
	// beginning/end of the string, then trim the bookend spaces.
	s = " " + s + " "
	s = reBareAuto.ReplaceAllString(s, " ")
	s = reBareNopr.ReplaceAllString(s, " ")
	s = reBareDebug.ReplaceAllString(s, " ")
	s = reBareDiscuss.ReplaceAllString(s, " ")
	s = reBareResume.ReplaceAllString(s, " ") // strip for backward compat; not in BareFlags

	return strings.TrimSpace(s)
}

// normalizeBareFlags returns a non-nil slice; if input is nil it returns [].
func normalizeBareFlags(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

// itoa converts an int to a decimal string.
func itoa(n int) string {
	return strconv.Itoa(n)
}

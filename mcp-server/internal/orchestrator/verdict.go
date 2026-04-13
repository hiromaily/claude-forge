// provides pure-logic building blocks for the pipeline engine.

package orchestrator

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"

	// Aliased as stateconst to avoid shadowing orchestrator-local type names
	// (e.g. SeverityCritical Severity vs state.SeverityCritical string).
	stateconst "github.com/hiromaily/claude-forge/mcp-server/internal/state"
)

// Verdict represents the outcome token from a review file.
type Verdict string

// Known verdict constants matching the tokens written by review agents.
// Values are derived from state.constants to ensure a single source of truth.
const (
	VerdictApprove          Verdict = Verdict(stateconst.VerdictApprove)
	VerdictApproveWithNotes Verdict = Verdict(stateconst.VerdictApproveWithNotes)
	VerdictRevise           Verdict = Verdict(stateconst.VerdictRevise)
	VerdictFail             Verdict = Verdict(stateconst.VerdictFail)
	VerdictPass             Verdict = Verdict(stateconst.VerdictPass)
	VerdictPassWithNotes    Verdict = Verdict(stateconst.VerdictPassWithNotes)
	VerdictUnknown          Verdict = Verdict(stateconst.VerdictUnknown)
)

// Severity classifies the urgency of a review finding.
type Severity string

// Known severity constants.
const (
	SeverityCritical Severity = Severity(stateconst.SeverityCritical)
	SeverityMinor    Severity = Severity(stateconst.SeverityMinor)
)

// Finding represents a single labelled finding extracted from a review file.
type Finding struct {
	Severity    Severity `json:"severity"`
	Description string   `json:"description"`
}

// headingVerdictRe matches markdown heading lines of the form:
//
//	## Verdict: TOKEN
//	# Verdict: TOKEN
//
// Capture group 1 is the verdict token.
var headingVerdictRe = regexp.MustCompile(`^#{1,2}\s+Verdict:\s+(\S+)`)

// inlineVerdictRe matches a bare inline verdict line of the form:
//
//	Verdict: TOKEN
//
// Capture group 1 is the verdict token.
var inlineVerdictRe = regexp.MustCompile(`^Verdict:\s+(\S+)`)

// findingRe matches finding lines in two forms:
//
//	**1. [CRITICAL] description text**
//	**[MINOR] description text**
//
// Capture group 1 is the severity; capture group 2 is the description text.
var findingRe = regexp.MustCompile(`\*\*(?:\d+\.\s+)?\[(CRITICAL|MINOR)\]\s+(.+?)\*\*`)

// knownVerdicts is the set of recognised verdict tokens. Tokens not in this set
// are treated as unrecognised and will not set the verdict.
var knownVerdicts = map[string]Verdict{
	string(VerdictApprove):          VerdictApprove,
	string(VerdictApproveWithNotes): VerdictApproveWithNotes,
	string(VerdictRevise):           VerdictRevise,
	string(VerdictFail):             VerdictFail,
	string(VerdictPass):             VerdictPass,
	string(VerdictPassWithNotes):    VerdictPassWithNotes,
}

// ParseVerdict reads filePath, extracts the verdict and findings.
//
// Two-pass parsing rules:
//  1. Pass 1 (heading scan): scan all lines for "## Verdict:" or "# Verdict:" patterns.
//     The first match is authoritative.
//  2. Pass 2 (inline scan): only reached when pass 1 found no verdict. Scan for lines
//     matching "^Verdict: TOKEN".
//
// Finding extraction scans all lines for [CRITICAL] or [MINOR] labelled items.
//
// Returns (VerdictUnknown, nil, nil) when the file contains no recognisable verdict.
// Returns a non-nil error only for file I/O failures.
func ParseVerdict(filePath string) (Verdict, []Finding, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return VerdictUnknown, nil, fmt.Errorf("ParseVerdict: open %q: %w", filePath, err)
	}

	defer func() { _ = f.Close() }()

	var (
		headingVerdict Verdict
		inlineVerdict  Verdict
		findings       []Finding
	)

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()

		// Heading scan (pass 1).
		if headingVerdict == "" {
			if m := headingVerdictRe.FindStringSubmatch(line); m != nil {
				token := strings.TrimSpace(m[1])
				if v, ok := knownVerdicts[token]; ok {
					headingVerdict = v
				}
			}
		}

		// Inline scan (pass 2 — collect first match, used only when no heading found).
		if inlineVerdict == "" && headingVerdict == "" {
			if m := inlineVerdictRe.FindStringSubmatch(line); m != nil {
				token := strings.TrimSpace(m[1])
				if v, ok := knownVerdicts[token]; ok {
					inlineVerdict = v
				}
			}
		}

		// Finding extraction — runs unconditionally on every line.
		if m := findingRe.FindStringSubmatch(line); m != nil {
			sev := Severity(m[1])
			desc := strings.TrimSpace(m[2])
			findings = append(findings, Finding{Severity: sev, Description: desc})
		}
	}

	if err := scanner.Err(); err != nil {
		return VerdictUnknown, nil, fmt.Errorf("ParseVerdict: scan %q: %w", filePath, err)
	}

	// Determine final verdict: heading wins over inline; unknown if neither found.
	var result Verdict

	switch {
	case headingVerdict != "":
		result = headingVerdict
	case inlineVerdict != "":
		result = inlineVerdict
	default:
		// No recognisable verdict found.
		return VerdictUnknown, nil, nil
	}

	if findings == nil {
		findings = []Finding{}
	}

	return result, findings, nil
}

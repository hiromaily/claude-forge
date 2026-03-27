// Package orchestrator provides pure-logic building blocks for the pipeline engine.
package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFileForTest is a test helper that writes content to path.
func writeFileForTest(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}

// testdataPath returns the absolute path to a fixture file in testdata/.
func testdataPath(name string) string {
	return filepath.Join("testdata", name)
}

func TestParseVerdict_AllVerdictConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		file             string
		wantVerdict      Verdict
		wantFindingCount int
		wantSeverity     Severity // checked only when wantFindingCount > 0
	}{
		{name: "approve", file: "review-design-approve.md", wantVerdict: VerdictApprove, wantFindingCount: 0},
		{name: "approve_with_notes", file: "review-design-approve-with-notes.md", wantVerdict: VerdictApproveWithNotes, wantFindingCount: 2, wantSeverity: SeverityMinor},
		{name: "revise", file: "review-design-revise.md", wantVerdict: VerdictRevise, wantFindingCount: 1, wantSeverity: SeverityCritical},
		{name: "fail", file: "review-design-fail.md", wantVerdict: VerdictFail, wantFindingCount: 1, wantSeverity: SeverityCritical},
		{name: "pass", file: "review-design-pass.md", wantVerdict: VerdictPass, wantFindingCount: 0},
		{name: "pass_with_notes", file: "review-design-pass-with-notes.md", wantVerdict: VerdictPassWithNotes, wantFindingCount: 1, wantSeverity: SeverityMinor},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, findings, err := ParseVerdict(testdataPath(tc.file))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tc.wantVerdict {
				t.Errorf("verdict = %q, want %q", got, tc.wantVerdict)
			}

			if len(findings) != tc.wantFindingCount {
				t.Fatalf("findings count = %d, want %d; got %v", len(findings), tc.wantFindingCount, findings)
			}

			for i, f := range findings {
				if f.Severity != tc.wantSeverity {
					t.Errorf("findings[%d].Severity = %q, want %q", i, f.Severity, tc.wantSeverity)
				}
			}
		})
	}
}

func TestParseVerdict_InlineOnly(t *testing.T) {
	t.Parallel()

	verdict, _, err := ParseVerdict(testdataPath("review-design-inline-only.md"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if verdict != VerdictApprove {
		t.Errorf("verdict = %q, want %q (inline form should be used when no heading form present)", verdict, VerdictApprove)
	}
}

func TestParseVerdict_BothForms_HeadingWins(t *testing.T) {
	t.Parallel()

	verdict, _, err := ParseVerdict(testdataPath("review-design-both-forms.md"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if verdict != VerdictRevise {
		t.Errorf("verdict = %q, want %q (heading form should win over inline APPROVE)", verdict, VerdictRevise)
	}
}

func TestParseVerdict_FileNotFound(t *testing.T) {
	t.Parallel()

	_, _, err := ParseVerdict(testdataPath("nonexistent-file.md"))
	if err == nil {
		t.Error("expected non-nil error for non-existent file, got nil")
	}
}

func TestParseVerdict_NoVerdict(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	noVerdictPath := filepath.Join(tmpDir, "no-verdict.md")

	content := "# Review\n\nThis file has no verdict token at all.\n"
	if err := writeFileForTest(noVerdictPath, content); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	verdict, findings, err := ParseVerdict(noVerdictPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if verdict != VerdictUnknown {
		t.Errorf("verdict = %q, want %q", verdict, VerdictUnknown)
	}

	if findings != nil {
		t.Errorf("findings = %v, want nil", findings)
	}
}

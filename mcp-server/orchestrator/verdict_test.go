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

func TestParseVerdict_Approve(t *testing.T) {
	t.Parallel()

	verdict, findings, err := ParseVerdict(testdataPath("review-design-approve.md"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if verdict != VerdictApprove {
		t.Errorf("verdict = %q, want %q", verdict, VerdictApprove)
	}

	if len(findings) != 0 {
		t.Errorf("findings count = %d, want 0; got %v", len(findings), findings)
	}
}

func TestParseVerdict_ApproveWithNotes(t *testing.T) {
	t.Parallel()

	verdict, findings, err := ParseVerdict(testdataPath("review-design-approve-with-notes.md"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if verdict != VerdictApproveWithNotes {
		t.Errorf("verdict = %q, want %q", verdict, VerdictApproveWithNotes)
	}

	if len(findings) != 2 {
		t.Fatalf("findings count = %d, want 2; got %v", len(findings), findings)
	}

	for i, f := range findings {
		if f.Severity != SeverityMinor {
			t.Errorf("findings[%d].Severity = %q, want %q", i, f.Severity, SeverityMinor)
		}
	}
}

func TestParseVerdict_Revise(t *testing.T) {
	t.Parallel()

	verdict, findings, err := ParseVerdict(testdataPath("review-design-revise.md"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if verdict != VerdictRevise {
		t.Errorf("verdict = %q, want %q", verdict, VerdictRevise)
	}

	if len(findings) != 1 {
		t.Fatalf("findings count = %d, want 1; got %v", len(findings), findings)
	}

	if findings[0].Severity != SeverityCritical {
		t.Errorf("findings[0].Severity = %q, want %q", findings[0].Severity, SeverityCritical)
	}
}

func TestParseVerdict_Fail(t *testing.T) {
	t.Parallel()

	verdict, findings, err := ParseVerdict(testdataPath("review-design-fail.md"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if verdict != VerdictFail {
		t.Errorf("verdict = %q, want %q", verdict, VerdictFail)
	}

	if len(findings) != 1 {
		t.Fatalf("findings count = %d, want 1; got %v", len(findings), findings)
	}

	if findings[0].Severity != SeverityCritical {
		t.Errorf("findings[0].Severity = %q, want %q", findings[0].Severity, SeverityCritical)
	}
}

func TestParseVerdict_Pass(t *testing.T) {
	t.Parallel()

	verdict, findings, err := ParseVerdict(testdataPath("review-design-pass.md"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if verdict != VerdictPass {
		t.Errorf("verdict = %q, want %q", verdict, VerdictPass)
	}

	if len(findings) != 0 {
		t.Errorf("findings count = %d, want 0; got %v", len(findings), findings)
	}
}

func TestParseVerdict_PassWithNotes(t *testing.T) {
	t.Parallel()

	verdict, findings, err := ParseVerdict(testdataPath("review-design-pass-with-notes.md"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if verdict != VerdictPassWithNotes {
		t.Errorf("verdict = %q, want %q", verdict, VerdictPassWithNotes)
	}

	if len(findings) != 1 {
		t.Fatalf("findings count = %d, want 1; got %v", len(findings), findings)
	}

	if findings[0].Severity != SeverityMinor {
		t.Errorf("findings[0].Severity = %q, want %q", findings[0].Severity, SeverityMinor)
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

	// Write a temp file with no verdict token to a path we control.
	// We use t.TempDir() and create the file inline.
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

func TestParseVerdict_AllVerdictConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		file        string
		wantVerdict Verdict
	}{
		{name: "approve", file: "review-design-approve.md", wantVerdict: VerdictApprove},
		{name: "approve_with_notes", file: "review-design-approve-with-notes.md", wantVerdict: VerdictApproveWithNotes},
		{name: "revise", file: "review-design-revise.md", wantVerdict: VerdictRevise},
		{name: "fail", file: "review-design-fail.md", wantVerdict: VerdictFail},
		{name: "pass", file: "review-design-pass.md", wantVerdict: VerdictPass},
		{name: "pass_with_notes", file: "review-design-pass-with-notes.md", wantVerdict: VerdictPassWithNotes},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, _, err := ParseVerdict(testdataPath(tc.file))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tc.wantVerdict {
				t.Errorf("verdict = %q, want %q", got, tc.wantVerdict)
			}
		})
	}
}

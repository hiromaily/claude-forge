// Package validation_test contains tests for the validation package.
package validation_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/validation"
)

// writeFile is a helper to create a file with given content in a directory.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("writeFile %s: %v", name, err)
	}
}

func TestValidateArtifacts_Phase3(t *testing.T) {
	t.Parallel()

	t.Run("missing file", func(t *testing.T) {
		t.Parallel()

		workspace := t.TempDir()
		results := validation.ValidateArtifacts(workspace, "phase-3")

		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}

		r := results[0]

		if r.Valid {
			t.Error("expected valid=false for missing file")
		}

		if r.Error == "" {
			t.Error("expected non-empty error for missing file")
		}
	})

	t.Run("file with heading", func(t *testing.T) {
		t.Parallel()

		workspace := t.TempDir()
		writeFile(t, workspace, "design.md", "# Title\n\n## Introduction\n\nSome content here.\n")

		results := validation.ValidateArtifacts(workspace, "phase-3")

		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}

		r := results[0]

		if !r.Valid {
			t.Errorf("expected valid=true, got error: %s", r.Error)
		}

		if r.FindingsCount != nil {
			t.Error("expected findings_count to be nil for phase-3")
		}
	})

	t.Run("file without heading", func(t *testing.T) {
		t.Parallel()

		workspace := t.TempDir()
		writeFile(t, workspace, "design.md", "Just plain text with no headings.\n")

		results := validation.ValidateArtifacts(workspace, "phase-3")

		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}

		r := results[0]

		if r.Valid {
			t.Error("expected valid=false for file without heading")
		}
	})

	t.Run("findings_count is null in JSON", func(t *testing.T) {
		t.Parallel()

		workspace := t.TempDir()
		writeFile(t, workspace, "design.md", "## Overview\n\nContent.\n")

		results := validation.ValidateArtifacts(workspace, "phase-3")

		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}

		data, err := json.Marshal(results[0])
		if err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}

		var m map[string]any

		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}

		fc, exists := m["findings_count"]
		if !exists {
			t.Error("expected findings_count key in JSON output")
		}

		if fc != nil {
			t.Errorf("expected findings_count to be null in JSON, got %v", fc)
		}
	})
}

func TestValidateArtifacts_Phase3b(t *testing.T) {
	t.Parallel()

	t.Run("APPROVE verdict", func(t *testing.T) {
		t.Parallel()

		workspace := t.TempDir()
		writeFile(t, workspace, "review-design.md", "## Verdict\n\nAPPROVE\n")

		results := validation.ValidateArtifacts(workspace, "phase-3b")

		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}

		r := results[0]

		if !r.Valid {
			t.Errorf("expected valid=true for APPROVE verdict, error: %s", r.Error)
		}

		if r.VerdictFound != "APPROVE" {
			t.Errorf("expected verdict_found=APPROVE, got %q", r.VerdictFound)
		}

		if r.FindingsCount == nil {
			t.Error("expected findings_count to be non-nil for phase-3b")
		}
	})

	t.Run("APPROVE_WITH_NOTES verdict", func(t *testing.T) {
		t.Parallel()

		workspace := t.TempDir()
		writeFile(t, workspace, "review-design.md",
			"## Verdict\n\nAPPROVE_WITH_NOTES\n\n**1. [CRITICAL] Issue one**\n\n**2. [MINOR] Issue two**\n")

		results := validation.ValidateArtifacts(workspace, "phase-3b")

		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}

		r := results[0]

		if !r.Valid {
			t.Errorf("expected valid=true for APPROVE_WITH_NOTES, error: %s", r.Error)
		}

		if r.VerdictFound != "APPROVE_WITH_NOTES" {
			t.Errorf("expected verdict_found=APPROVE_WITH_NOTES, got %q", r.VerdictFound)
		}

		if r.FindingsCount == nil {
			t.Fatal("expected findings_count to be non-nil")
		}

		if r.FindingsCount.Critical != 1 {
			t.Errorf("expected 1 CRITICAL finding, got %d", r.FindingsCount.Critical)
		}

		if r.FindingsCount.Minor != 1 {
			t.Errorf("expected 1 MINOR finding, got %d", r.FindingsCount.Minor)
		}
	})

	t.Run("REVISE verdict", func(t *testing.T) {
		t.Parallel()

		workspace := t.TempDir()
		writeFile(t, workspace, "review-design.md", "## Verdict\n\nREVISE\n\nNeeds work.\n")

		results := validation.ValidateArtifacts(workspace, "phase-3b")

		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}

		r := results[0]

		if !r.Valid {
			t.Errorf("expected valid=true for REVISE verdict, error: %s", r.Error)
		}

		if r.VerdictFound != "REVISE" {
			t.Errorf("expected verdict_found=REVISE, got %q", r.VerdictFound)
		}
	})

	t.Run("no verdict", func(t *testing.T) {
		t.Parallel()

		workspace := t.TempDir()
		writeFile(t, workspace, "review-design.md", "## Review\n\nThis has no verdict keyword.\n")

		results := validation.ValidateArtifacts(workspace, "phase-3b")

		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}

		r := results[0]

		if r.Valid {
			t.Error("expected valid=false for no verdict")
		}
	})

	t.Run("findings_count non-nil", func(t *testing.T) {
		t.Parallel()

		workspace := t.TempDir()
		writeFile(t, workspace, "review-design.md", "APPROVE\n")

		results := validation.ValidateArtifacts(workspace, "phase-3b")

		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}

		if results[0].FindingsCount == nil {
			t.Error("expected findings_count to be non-nil for phase-3b")
		}
	})
}

func TestValidateArtifacts_Phase4(t *testing.T) {
	t.Parallel()

	t.Run("file with Task heading", func(t *testing.T) {
		t.Parallel()

		workspace := t.TempDir()
		writeFile(t, workspace, "tasks.md", "## Task 1: Do something\n\nDetails here.\n")

		results := validation.ValidateArtifacts(workspace, "phase-4")

		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}

		r := results[0]

		if !r.Valid {
			t.Errorf("expected valid=true, error: %s", r.Error)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		t.Parallel()

		workspace := t.TempDir()
		results := validation.ValidateArtifacts(workspace, "phase-4")

		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}

		if results[0].Valid {
			t.Error("expected valid=false for missing file")
		}
	})
}

func TestValidateArtifacts_Phase4b(t *testing.T) {
	t.Parallel()

	t.Run("APPROVE verdict", func(t *testing.T) {
		t.Parallel()

		workspace := t.TempDir()
		writeFile(t, workspace, "review-tasks.md", "APPROVE\n\nAll tasks look good.\n")

		results := validation.ValidateArtifacts(workspace, "phase-4b")

		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}

		r := results[0]

		if !r.Valid {
			t.Errorf("expected valid=true, error: %s", r.Error)
		}

		if r.FindingsCount == nil {
			t.Error("expected findings_count to be non-nil for phase-4b")
		}
	})
}

func TestValidateArtifacts_Phase7(t *testing.T) {
	t.Parallel()

	t.Run("non-empty file", func(t *testing.T) {
		t.Parallel()

		workspace := t.TempDir()
		writeFile(t, workspace, "verification.md", "Some content here.\n")

		results := validation.ValidateArtifacts(workspace, "phase-7")

		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}

		if !results[0].Valid {
			t.Errorf("expected valid=true, error: %s", results[0].Error)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		t.Parallel()

		workspace := t.TempDir()
		results := validation.ValidateArtifacts(workspace, "phase-7")

		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}

		if results[0].Valid {
			t.Error("expected valid=false for missing file")
		}
	})
}

func TestValidateArtifacts_Phase6(t *testing.T) {
	t.Parallel()

	t.Run("no impl files", func(t *testing.T) {
		t.Parallel()

		workspace := t.TempDir()
		results := validation.ValidateArtifacts(workspace, "phase-6")

		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}

		r := results[0]

		if r.Valid {
			t.Error("expected valid=false for no impl files")
		}

		if r.Error == "" {
			t.Error("expected non-empty error string for no impl files")
		}
	})

	t.Run("one PASS file", func(t *testing.T) {
		t.Parallel()

		workspace := t.TempDir()
		writeFile(t, workspace, "impl-1.md", "## Summary\n\nPASS\n")

		results := validation.ValidateArtifacts(workspace, "phase-6")

		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}

		r := results[0]

		if !r.Valid {
			t.Errorf("expected valid=true for PASS, error: %s", r.Error)
		}

		if r.VerdictFound != "PASS" {
			t.Errorf("expected verdict_found=PASS, got %q", r.VerdictFound)
		}
	})

	t.Run("PASS_WITH_NOTES file", func(t *testing.T) {
		t.Parallel()

		workspace := t.TempDir()
		writeFile(t, workspace, "impl-1.md", "## Summary\n\nPASS_WITH_NOTES\n")

		results := validation.ValidateArtifacts(workspace, "phase-6")

		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}

		r := results[0]

		if !r.Valid {
			t.Errorf("expected valid=true for PASS_WITH_NOTES, error: %s", r.Error)
		}

		if r.VerdictFound != "PASS_WITH_NOTES" {
			t.Errorf("expected verdict_found=PASS_WITH_NOTES, got %q", r.VerdictFound)
		}
	})

	t.Run("FAIL file", func(t *testing.T) {
		t.Parallel()

		workspace := t.TempDir()
		writeFile(t, workspace, "impl-1.md", "## Summary\n\nFAIL\n\nSome issues found.\n")

		results := validation.ValidateArtifacts(workspace, "phase-6")

		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}

		r := results[0]

		if r.Valid {
			t.Error("expected valid=false for FAIL verdict")
		}

		if r.VerdictFound != "FAIL" {
			t.Errorf("expected verdict_found=FAIL, got %q", r.VerdictFound)
		}
	})

	t.Run("multiple impl files", func(t *testing.T) {
		t.Parallel()

		workspace := t.TempDir()
		writeFile(t, workspace, "impl-1.md", "PASS\n")
		writeFile(t, workspace, "impl-2.md", "PASS_WITH_NOTES\n")
		writeFile(t, workspace, "impl-3.md", "FAIL\n")

		results := validation.ValidateArtifacts(workspace, "phase-6")

		if len(results) != 3 {
			t.Fatalf("expected 3 results for 3 impl files, got %d", len(results))
		}
	})
}

func TestValidateArtifacts_UnknownPhase(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	results := validation.ValidateArtifacts(workspace, "phase-unknown")

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]

	if r.Valid {
		t.Error("expected valid=false for unknown phase")
	}

	if r.Error == "" {
		t.Error("expected non-empty error for unknown phase")
	}
}

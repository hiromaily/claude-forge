// Package validation_test provides table-driven tests for the validation package.
package validation_test

import (
	"strings"
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/internal/validation"
)

func TestValidateInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		input          string
		wantValid      bool
		wantSourceType string
		wantErrContain string // substring that must appear in errors[0] when invalid
	}{
		// AC-1: empty / whitespace / too-short
		{
			name:           "empty string",
			input:          "",
			wantValid:      false,
			wantErrContain: "empty",
		},
		{
			name:           "whitespace only",
			input:          "   ",
			wantValid:      false,
			wantErrContain: "empty",
		},
		{
			name:           "2-char core text",
			input:          "ab",
			wantValid:      false,
			wantErrContain: "short",
		},
		{
			name:           "4-char core text",
			input:          "abcd",
			wantValid:      false,
			wantErrContain: "short",
		},
		{
			name:           "flags only no task",
			input:          "--auto --nopr",
			wantValid:      false,
			wantErrContain: "empty",
		},
		// AC-1: valid plain text
		{
			name:           "5-char plain text valid",
			input:          "hello",
			wantValid:      true,
			wantSourceType: "text",
		},
		{
			name:           "plain text with flags",
			input:          "--auto implement the feature",
			wantValid:      true,
			wantSourceType: "text",
		},
		// AC-2: flag stripping
		{
			name:           "strip --type flag",
			input:          "--type=feature implement login",
			wantValid:      true,
			wantSourceType: "text",
		},
		{
			name:           "strip --effort flag",
			input:          "--effort=M implement login",
			wantValid:      true,
			wantSourceType: "text",
		},
		{
			name:           "strip --debug flag",
			input:          "--debug implement login",
			wantValid:      true,
			wantSourceType: "text",
		},
		{
			name:           "strip --nopr flag",
			input:          "--nopr implement login",
			wantValid:      true,
			wantSourceType: "text",
		},
		{
			name:           "strip --resume flag leaving dirname as core",
			input:          "20260401-effort-only-flow --resume",
			wantValid:      true,
			wantSourceType: "text",
		},
		{
			name:           "strip all flags leaving valid core",
			input:          "--type=bugfix --effort=S --auto --nopr --debug fix the crash",
			wantValid:      true,
			wantSourceType: "text",
		},
		// AC-2: --automatic is NOT stripped (word boundary).
		// --automatic is 11 chars, so it survives as the core text and is valid.
		{
			name:           "--automatic not stripped as bare flag",
			input:          "--automatic",
			wantValid:      true,
			wantSourceType: "text",
		},
		{
			name:           "--automatic alongside text stays in core",
			input:          "--automatic testing",
			wantValid:      true,
			wantSourceType: "text",
		},
		// Contrast: --auto IS stripped, leaving empty core (flags only error).
		{
			name:           "--auto alone stripped to empty",
			input:          "--auto",
			wantValid:      false,
			wantErrContain: "empty",
		},
		// AC-3: GitHub URL valid
		{
			name:           "valid GitHub issue URL",
			input:          "https://github.com/owner/repo/issues/42",
			wantValid:      true,
			wantSourceType: "github_issue",
		},
		// AC-3: GitHub URL malformed
		{
			name:           "GitHub URL missing issues path",
			input:          "https://github.com/owner/repo",
			wantValid:      false,
			wantErrContain: "GitHub",
		},
		{
			name:           "GitHub URL with non-numeric issue number",
			input:          "https://github.com/owner/repo/issues/abc",
			wantValid:      false,
			wantErrContain: "GitHub",
		},
		// AC-3: Jira URL valid
		{
			name:           "valid Jira URL",
			input:          "https://example.atlassian.net/browse/PROJ-123",
			wantValid:      true,
			wantSourceType: "jira_issue",
		},
		// AC-3: Jira URL malformed
		{
			name:           "Jira URL missing browse path",
			input:          "https://example.atlassian.net/PROJ-123",
			wantValid:      false,
			wantErrContain: "Jira",
		},
		// AC-3: unknown https URL
		{
			name:           "arbitrary https URL",
			input:          "https://example.com/some/path",
			wantValid:      false,
			wantErrContain: "Unrecognised URL format",
		},
		// workspace path via .specs/ substring
		{
			name:           "workspace path via .specs/ substring",
			input:          ".specs/20260101-some-spec",
			wantValid:      true,
			wantSourceType: "workspace",
		},
		// XS effort rejection (AC-1)
		{
			name:           "effort XS rejected",
			input:          "--effort=XS implement login",
			wantValid:      false,
			wantErrContain: `effort "XS" is not supported; valid efforts are: S, M, L`,
		},
		{
			name:           "effort XS rejected with other flags",
			input:          "--auto --effort=XS implement the feature",
			wantValid:      false,
			wantErrContain: `effort "XS" is not supported; valid efforts are: S, M, L`,
		},
		// Valid efforts pass XS check (AC-2)
		{
			name:           "effort S passes validation",
			input:          "--effort=S implement login",
			wantValid:      true,
			wantSourceType: "text",
		},
		{
			name:           "effort M passes validation",
			input:          "--effort=M implement login",
			wantValid:      true,
			wantSourceType: "text",
		},
		{
			name:           "effort L passes validation",
			input:          "--effort=L implement login",
			wantValid:      true,
			wantSourceType: "text",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := validation.ValidateInput(tc.input)
			if result.Valid != tc.wantValid {
				t.Errorf("ValidateInput(%q).Valid = %v, want %v", tc.input, result.Valid, tc.wantValid)
			}
			if tc.wantValid {
				if result.Parsed.SourceType != tc.wantSourceType {
					t.Errorf("ValidateInput(%q).Parsed.SourceType = %q, want %q", tc.input, result.Parsed.SourceType, tc.wantSourceType)
				}
			} else {
				if len(result.Errors) == 0 {
					t.Errorf("ValidateInput(%q): expected errors but got none", tc.input)
				} else if tc.wantErrContain != "" && !strings.Contains(result.Errors[0], tc.wantErrContain) {
					t.Errorf("ValidateInput(%q).Errors[0] = %q, want substring %q", tc.input, result.Errors[0], tc.wantErrContain)
				}
			}
		})
	}
}

func TestValidateInputCoreText(t *testing.T) {
	t.Parallel()

	// After stripping flags, CoreText should not contain flag tokens.
	result := validation.ValidateInput("--type=feature --effort=S --auto --nopr --debug implement login")
	if !result.Valid {
		t.Fatalf("expected valid, got errors: %v", result.Errors)
	}
	core := result.Parsed.CoreText
	if strings.Contains(core, "--type") {
		t.Errorf("CoreText %q should not contain --type", core)
	}
	if strings.Contains(core, "--effort") {
		t.Errorf("CoreText %q should not contain --effort", core)
	}
	if strings.Contains(core, "--auto ") || core == "--auto" {
		t.Errorf("CoreText %q should not contain --auto", core)
	}
	if strings.Contains(core, "--nopr") {
		t.Errorf("CoreText %q should not contain --nopr", core)
	}
	if strings.Contains(core, "--debug") {
		t.Errorf("CoreText %q should not contain --debug", core)
	}
}

func TestValidateInputParsedFlags(t *testing.T) {
	t.Parallel()

	result := validation.ValidateInput("--type=feature --effort=M --auto --nopr implement something")
	if !result.Valid {
		t.Fatalf("expected valid, got errors: %v", result.Errors)
	}
	if result.Parsed.Flags["type"] != "feature" {
		t.Errorf("Flags[type] = %q, want %q", result.Parsed.Flags["type"], "feature")
	}
	if result.Parsed.Flags["effort"] != "M" {
		t.Errorf("Flags[effort] = %q, want %q", result.Parsed.Flags["effort"], "M")
	}
	// Bare flags should appear in BareFlags
	foundAuto := false
	foundNopr := false
	for _, f := range result.Parsed.BareFlags {
		if f == "auto" {
			foundAuto = true
		}
		if f == "nopr" {
			foundNopr = true
		}
	}
	if !foundAuto {
		t.Errorf("BareFlags should contain 'auto', got %v", result.Parsed.BareFlags)
	}
	if !foundNopr {
		t.Errorf("BareFlags should contain 'nopr', got %v", result.Parsed.BareFlags)
	}
}

func TestValidateInputResumeFlagInBareFlags(t *testing.T) {
	t.Parallel()

	// --resume must appear in BareFlags and be stripped from CoreText.
	result := validation.ValidateInput("20260401-effort-only-flow --resume")
	if !result.Valid {
		t.Fatalf("expected valid, got errors: %v", result.Errors)
	}
	foundResume := false
	for _, f := range result.Parsed.BareFlags {
		if f == "resume" {
			foundResume = true
		}
	}
	if !foundResume {
		t.Errorf("BareFlags should contain 'resume', got %v", result.Parsed.BareFlags)
	}
	if strings.Contains(result.Parsed.CoreText, "--resume") {
		t.Errorf("CoreText %q should not contain --resume after stripping", result.Parsed.CoreText)
	}
	if result.Parsed.CoreText != "20260401-effort-only-flow" {
		t.Errorf("CoreText = %q, want %q", result.Parsed.CoreText, "20260401-effort-only-flow")
	}
}

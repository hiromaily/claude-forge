// Package tools — unit tests for validate_input and validate_artifact MCP handlers.
// These tests verify that ValidateInputHandler and ValidateArtifactHandler behave
// correctly, including edge cases for empty inputs and missing parameters.
package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/internal/handler/validation"
)

// ---------- validate_input handler ----------

func TestValidateInputHandler_EmptyArguments(t *testing.T) {
	t.Parallel()

	h := ValidateInputHandler()
	res := callTool(t, h, map[string]any{
		"arguments": "",
	})
	// Empty string must NOT be an MCP-level error; instead valid:false in JSON content.
	if res.IsError {
		t.Errorf("ValidateInputHandler with empty arguments should not return MCP error, got: %v", textContent(res))
	}
	// Parse and verify valid:false.
	var result validation.InputResult
	if err := json.Unmarshal([]byte(textContent(res)), &result); err != nil {
		t.Fatalf("unmarshal InputResult: %v", err)
	}
	if result.Valid {
		t.Errorf("ValidateInputHandler empty arguments: got valid=true, want valid=false")
	}
	if len(result.Errors) == 0 {
		t.Errorf("ValidateInputHandler empty arguments: expected errors, got none")
	}
}

func TestValidateInputHandler_ValidText(t *testing.T) {
	t.Parallel()

	h := ValidateInputHandler()
	res := callTool(t, h, map[string]any{
		"arguments": "hello world",
	})
	if res.IsError {
		t.Errorf("ValidateInputHandler with valid text returned error: %v", textContent(res))
	}
	var result validation.InputResult
	if err := json.Unmarshal([]byte(textContent(res)), &result); err != nil {
		t.Fatalf("unmarshal InputResult: %v", err)
	}
	if !result.Valid {
		t.Errorf("ValidateInputHandler valid text: got valid=false, errors=%v", result.Errors)
	}
	if result.Parsed.SourceType != "text" {
		t.Errorf("ValidateInputHandler valid text: got source_type=%q, want %q", result.Parsed.SourceType, "text")
	}
}

func TestValidateInputHandler_GitHubURL(t *testing.T) {
	t.Parallel()

	h := ValidateInputHandler()
	res := callTool(t, h, map[string]any{
		"arguments": "https://github.com/owner/repo/issues/42",
	})
	if res.IsError {
		t.Errorf("ValidateInputHandler GitHub URL returned MCP error: %v", textContent(res))
	}
	var result validation.InputResult
	if err := json.Unmarshal([]byte(textContent(res)), &result); err != nil {
		t.Fatalf("unmarshal InputResult: %v", err)
	}
	if !result.Valid {
		t.Errorf("ValidateInputHandler GitHub URL: got valid=false, errors=%v", result.Errors)
	}
	if result.Parsed.SourceType != "github_issue" {
		t.Errorf("ValidateInputHandler GitHub URL: got source_type=%q, want %q", result.Parsed.SourceType, "github_issue")
	}
}

func TestValidateInputHandler_AbsentArgumentsParam(t *testing.T) {
	t.Parallel()

	// When the "arguments" param is absent it defaults to "" — same as empty.
	h := ValidateInputHandler()
	res := callTool(t, h, map[string]any{})
	// Should return valid:false in JSON, NOT an MCP error.
	if res.IsError {
		t.Errorf("ValidateInputHandler with absent arguments should not return MCP error, got: %v", textContent(res))
	}
	var result validation.InputResult
	if err := json.Unmarshal([]byte(textContent(res)), &result); err != nil {
		t.Fatalf("unmarshal InputResult: %v", err)
	}
	if result.Valid {
		t.Errorf("ValidateInputHandler absent arguments: got valid=true, want valid=false")
	}
}

// ---------- validate_artifact handler ----------

func TestValidateArtifactHandler_MissingWorkspace(t *testing.T) {
	t.Parallel()

	h := ValidateArtifactHandler()
	res := callTool(t, h, map[string]any{
		"phase": "phase-3",
	})
	if !res.IsError {
		t.Errorf("ValidateArtifactHandler missing workspace should return MCP error")
	}
}

func TestValidateArtifactHandler_MissingPhase(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	h := ValidateArtifactHandler()
	res := callTool(t, h, map[string]any{
		"workspace": dir,
	})
	if !res.IsError {
		t.Errorf("ValidateArtifactHandler missing phase should return MCP error")
	}
}

func TestValidateArtifactHandler_Phase3_Missing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	h := ValidateArtifactHandler()
	res := callTool(t, h, map[string]any{
		"workspace": dir,
		"phase":     "phase-3",
	})
	if res.IsError {
		t.Errorf("ValidateArtifactHandler phase-3 missing file should return JSON result, not MCP error: %v", textContent(res))
	}
	// Response must be a JSON array.
	var results []validation.ArtifactResult
	if err := json.Unmarshal([]byte(textContent(res)), &results); err != nil {
		t.Fatalf("unmarshal []ArtifactResult: %v (content: %s)", err, textContent(res))
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Valid {
		t.Errorf("ValidateArtifactHandler phase-3 missing file: got valid=true, want valid=false")
	}
}

func TestValidateArtifactHandler_Phase6_Pass(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Write one impl-1.md file containing PASS.
	implPath := filepath.Join(dir, "impl-1.md")
	if err := os.WriteFile(implPath, []byte("## Summary\n\nPASS\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	h := ValidateArtifactHandler()
	res := callTool(t, h, map[string]any{
		"workspace": dir,
		"phase":     "phase-6",
	})
	if res.IsError {
		t.Errorf("ValidateArtifactHandler phase-6 returned MCP error: %v", textContent(res))
	}
	// Response must be a JSON array.
	var results []validation.ArtifactResult
	if err := json.Unmarshal([]byte(textContent(res)), &results); err != nil {
		t.Fatalf("unmarshal []ArtifactResult: %v (content: %s)", err, textContent(res))
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Valid {
		t.Errorf("ValidateArtifactHandler phase-6 PASS: got valid=false, error=%q", results[0].Error)
	}
	if results[0].VerdictFound != "PASS" {
		t.Errorf("ValidateArtifactHandler phase-6 PASS: got verdict=%q, want %q", results[0].VerdictFound, "PASS")
	}
}

func TestValidateArtifactHandler_UnknownPhase(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	h := ValidateArtifactHandler()
	res := callTool(t, h, map[string]any{
		"workspace": dir,
		"phase":     "phase-99",
	})
	if res.IsError {
		t.Errorf("ValidateArtifactHandler unknown phase should return JSON result not MCP error: %v", textContent(res))
	}
	var results []validation.ArtifactResult
	if err := json.Unmarshal([]byte(textContent(res)), &results); err != nil {
		t.Fatalf("unmarshal []ArtifactResult: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Valid {
		t.Errorf("ValidateArtifactHandler unknown phase: got valid=true, want valid=false")
	}
}

func TestValidateArtifactHandler_ResponseIsAlwaysArray(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Write a valid design.md with a heading for phase-3.
	designPath := filepath.Join(dir, "design.md")
	if err := os.WriteFile(designPath, []byte("## Overview\n\nContent here.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	h := ValidateArtifactHandler()
	res := callTool(t, h, map[string]any{
		"workspace": dir,
		"phase":     "phase-3",
	})
	if res.IsError {
		t.Errorf("ValidateArtifactHandler returned MCP error: %v", textContent(res))
	}
	// Must always be a JSON array, even for non-phase-6 phases.
	raw := textContent(res)
	if len(raw) == 0 || raw[0] != '[' {
		t.Errorf("ValidateArtifactHandler: response must be JSON array, got: %s", raw)
	}
	var results []validation.ArtifactResult
	if err := json.Unmarshal([]byte(raw), &results); err != nil {
		t.Fatalf("unmarshal []ArtifactResult: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Valid {
		t.Errorf("ValidateArtifactHandler phase-3 valid file: got valid=false, error=%q", results[0].Error)
	}
}

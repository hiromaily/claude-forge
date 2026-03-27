// Package tools — handler tests for ast_impact_scope.
package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/ast"
)

// impactScopeResult is a partial unmarshal of the impact_scope JSON response,
// used for assertion purposes in handler tests.
type impactScopeResult struct {
	TargetFile    string            `json:"target_file"`
	Symbol        string            `json:"symbol"`
	Root          string            `json:"root"`
	Lang          string            `json:"lang"`
	AffectedFiles []ast.ImpactEntry `json:"affected_files"`
}

// writeImpactGoFile writes a .go file at relPath within dir with given content.
// It also creates parent directories as needed.
// Note: uses the same logic as writeTempGoFile; defined here to avoid a dependency
// between test files (both are in the same package so the shared helper in the
// ast_dependency_graph_test.go file is available, but for clarity we inline the
// setup calls using writeTempGoMod / writeTempGoFile which are already defined there).
func writeImpactGoFile(t *testing.T, dir, relPath, content string) {
	t.Helper()
	absPath := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(absPath), err)
	}
	if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", relPath, err)
	}
}

// TestAstImpactScopeFromRoot_GoCallerConfirmed verifies that a Go caller which
// imports the target package AND calls the target symbol appears in affected_files
// with distance == 1.
func TestAstImpactScopeFromRoot_GoCallerConfirmed(t *testing.T) {
	dir := t.TempDir()
	writeTempGoMod(t, dir, "example.com/test")

	// lib/target.go — defines TargetSymbol
	writeImpactGoFile(t, dir, "lib/target.go", `package lib

func TargetSymbol() {}
`)

	// caller/caller.go — imports lib and calls TargetSymbol
	writeImpactGoFile(t, dir, "caller/caller.go", `package caller

import "example.com/test/lib"

func run() {
	lib.TargetSymbol()
}
`)

	result, err := astImpactScopeFromRoot(
		context.Background(),
		dir,
		"lib/target.go",
		"TargetSymbol",
		"go",
	)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected IsError=false, got true: %s", textContent(result))
	}

	text := textContent(result)
	var resp impactScopeResult
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("unmarshal impactScopeResult: %v (text: %s)", err, text)
	}

	if len(resp.AffectedFiles) == 0 {
		t.Fatal("expected at least one entry in affected_files, got empty")
	}

	// Find the expected caller in affected_files.
	found := false
	for _, entry := range resp.AffectedFiles {
		if entry.File == "caller/caller.go" {
			if entry.Distance != 1 {
				t.Errorf("expected distance=1 for caller/caller.go, got %d", entry.Distance)
			}
			found = true
			break
		}
	}
	if !found {
		t.Errorf("caller/caller.go not found in affected_files; got: %+v", resp.AffectedFiles)
	}
}

// TestAstImpactScopeFromRoot_ImportsButNoCallNegative verifies that a Go file which
// imports the target package but does NOT call the target symbol is NOT present in
// affected_files.
func TestAstImpactScopeFromRoot_ImportsButNoCallNegative(t *testing.T) {
	dir := t.TempDir()
	writeTempGoMod(t, dir, "example.com/test")

	// lib/target.go — defines TargetSymbol
	writeImpactGoFile(t, dir, "lib/target.go", `package lib

func TargetSymbol() {}
func OtherFunc() {}
`)

	// caller/caller.go — imports lib but only calls OtherFunc, NOT TargetSymbol
	writeImpactGoFile(t, dir, "caller/caller.go", `package caller

import "example.com/test/lib"

func run() {
	lib.OtherFunc()
}
`)

	result, err := astImpactScopeFromRoot(
		context.Background(),
		dir,
		"lib/target.go",
		"TargetSymbol",
		"go",
	)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected IsError=false, got true: %s", textContent(result))
	}

	text := textContent(result)
	var resp impactScopeResult
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("unmarshal impactScopeResult: %v (text: %s)", err, text)
	}

	for _, entry := range resp.AffectedFiles {
		if entry.File == "caller/caller.go" {
			t.Errorf("caller/caller.go should NOT be in affected_files (imports but no call), but it appeared with distance=%d", entry.Distance)
		}
	}
}

// TestAstImpactScopeFromRoot_NoCallers verifies that when no file calls the target
// symbol, affected_files is an empty array (not nil, not an error).
func TestAstImpactScopeFromRoot_NoCallers(t *testing.T) {
	dir := t.TempDir()
	writeTempGoMod(t, dir, "example.com/test")

	// lib/target.go — defines TargetSymbol but nobody calls it
	writeImpactGoFile(t, dir, "lib/target.go", `package lib

func TargetSymbol() {}
`)

	result, err := astImpactScopeFromRoot(
		context.Background(),
		dir,
		"lib/target.go",
		"TargetSymbol",
		"go",
	)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected IsError=false, got true: %s", textContent(result))
	}

	text := textContent(result)
	var resp impactScopeResult
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("unmarshal impactScopeResult: %v (text: %s)", err, text)
	}

	if resp.AffectedFiles == nil {
		t.Error("expected non-nil affected_files array, got nil")
	}
	if len(resp.AffectedFiles) != 0 {
		t.Errorf("expected empty affected_files, got %+v", resp.AffectedFiles)
	}
}

// TestAstImpactScopeFromRoot_InvalidRoot verifies that an invalid root_path
// returns an error response (IsError=true).
func TestAstImpactScopeFromRoot_InvalidRoot(t *testing.T) {
	result, err := astImpactScopeFromRoot(
		context.Background(),
		"/nonexistent/path/that/does/not/exist",
		"lib/target.go",
		"TargetSymbol",
		"go",
	)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for invalid root_path, got false")
	}
	msg := textContent(result)
	if msg == "" {
		t.Error("expected non-empty error message, got empty")
	}
}

// TestAstImpactScopeFromRoot_TSCallerDistance verifies that a TypeScript caller
// that calls the target symbol is returned with distance == -1.
func TestAstImpactScopeFromRoot_TSCallerDistance(t *testing.T) {
	dir := t.TempDir()

	// target.ts — defines TargetSymbol (TypeScript, no imports needed)
	if err := os.WriteFile(filepath.Join(dir, "target.ts"), []byte(`
export function TargetSymbol() {}
`), 0o644); err != nil {
		t.Fatalf("write target.ts: %v", err)
	}

	// caller.ts — calls TargetSymbol
	if err := os.WriteFile(filepath.Join(dir, "caller.ts"), []byte(`
import { TargetSymbol } from './target';

TargetSymbol();
`), 0o644); err != nil {
		t.Fatalf("write caller.ts: %v", err)
	}

	result, err := astImpactScopeFromRoot(
		context.Background(),
		dir,
		"target.ts",
		"TargetSymbol",
		"typescript",
	)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected IsError=false, got true: %s", textContent(result))
	}

	text := textContent(result)
	var resp impactScopeResult
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("unmarshal impactScopeResult: %v (text: %s)", err, text)
	}

	if len(resp.AffectedFiles) == 0 {
		t.Fatal("expected at least one entry in affected_files for TS caller, got empty")
	}

	found := false
	for _, entry := range resp.AffectedFiles {
		if entry.File == "caller.ts" {
			if entry.Distance != -1 {
				t.Errorf("expected distance=-1 for TS caller, got %d", entry.Distance)
			}
			found = true
			break
		}
	}
	if !found {
		t.Errorf("caller.ts not found in affected_files; got: %+v", resp.AffectedFiles)
	}
}

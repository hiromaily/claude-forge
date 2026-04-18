// Package tools — handler tests for ast_dependency_graph.
package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/pkg/ast"
)

// writeTempGoMod writes a go.mod file with the given module name into dir.
//
//nolint:unparam // moduleName varies in future tests; keeping it parametric for flexibility
func writeTempGoMod(t *testing.T, dir, moduleName string) {
	t.Helper()
	content := "module " + moduleName + "\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(content), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
}

// writeTempGoFile writes a .go file at relPath within dir with the given content.
func writeTempGoFile(t *testing.T, dir, relPath, content string) {
	t.Helper()
	absPath := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(absPath), err)
	}
	if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", relPath, err)
	}
}

// TestAstDependencyGraphFromRoot_ValidGoDir verifies that a valid Go temp dir
// (with a go.mod and two .go files where one imports the other) produces a valid
// JSON response with non-empty nodes and at least one edge.
func TestAstDependencyGraphFromRoot_ValidGoDir(t *testing.T) {
	dir := t.TempDir()
	writeTempGoMod(t, dir, "example.com/test")

	// pkg/a.go — exported function, no imports
	writeTempGoFile(t, dir, "pkg/a.go", `package pkg

func DepAFunc() {}
`)

	// cmd/main.go — imports pkg
	writeTempGoFile(t, dir, "cmd/main.go", `package main

import "example.com/test/pkg"

func main() {
	pkg.DepAFunc()
}
`)

	result, err := astDependencyGraphFromRoot(context.Background(), dir, "go")
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected IsError=false, got true: %s", textContent(result))
	}

	text := textContent(result)
	var graph ast.DependencyGraph
	if err := json.Unmarshal([]byte(text), &graph); err != nil {
		t.Fatalf("unmarshal DependencyGraph: %v (text: %s)", err, text)
	}

	if len(graph.Nodes) == 0 {
		t.Error("expected non-empty nodes, got empty")
	}
	if len(graph.Edges) == 0 {
		t.Error("expected at least one edge, got none")
	}
	if graph.Root != dir {
		t.Errorf("graph.Root: got %q, want %q", graph.Root, dir)
	}
	if graph.Lang != "go" {
		t.Errorf("graph.Lang: got %q, want %q", graph.Lang, "go")
	}
}

// TestAstDependencyGraphFromRoot_InvalidRoot verifies that an invalid root_path
// returns an error response (IsError=true) with a non-empty message.
func TestAstDependencyGraphFromRoot_InvalidRoot(t *testing.T) {
	result, err := astDependencyGraphFromRoot(context.Background(), "/nonexistent/path/that/does/not/exist", "go")
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

// TestAstDependencyGraphFromRoot_UnsupportedLanguage verifies that an unsupported
// language returns an error response (IsError=true).
func TestAstDependencyGraphFromRoot_UnsupportedLanguage(t *testing.T) {
	dir := t.TempDir()
	result, err := astDependencyGraphFromRoot(context.Background(), dir, "cobol")
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for unsupported language, got false")
	}
	msg := textContent(result)
	if msg == "" {
		t.Error("expected non-empty error message, got empty")
	}
}

// TestAstDependencyGraphFromRoot_EmptyDir verifies that an empty directory
// produces valid JSON with empty nodes and edges slices — no error.
func TestAstDependencyGraphFromRoot_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	result, err := astDependencyGraphFromRoot(context.Background(), dir, "go")
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected IsError=false for empty dir, got true: %s", textContent(result))
	}

	text := textContent(result)
	var graph ast.DependencyGraph
	if err := json.Unmarshal([]byte(text), &graph); err != nil {
		t.Fatalf("unmarshal DependencyGraph: %v (text: %s)", err, text)
	}

	// nodes and edges should be empty (not nil — the marshalled form should use [])
	if len(graph.Nodes) != 0 {
		t.Errorf("expected empty nodes for empty dir, got %v", graph.Nodes)
	}
	if len(graph.Edges) != 0 {
		t.Errorf("expected empty edges for empty dir, got %v", graph.Edges)
	}
}

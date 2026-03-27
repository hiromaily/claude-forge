// Package ast provides unit tests for the graph domain functions.
package ast

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// ── ExtractImports ────────────────────────────────────────────────────────────

// TestExtractImports_DepB verifies that dep_b.go (which has `import "fmt"`)
// returns exactly ["fmt"].
func TestExtractImports_DepB(t *testing.T) {
	src := readTestdata(t, "dep_b.go")
	ctx := t.Context()

	imports, err := ExtractImports(ctx, src, Go, "dep_b.go")
	if err != nil {
		t.Fatalf("ExtractImports(dep_b.go): %v", err)
	}
	if len(imports) != 1 || imports[0] != "fmt" {
		t.Errorf("expected [\"fmt\"], got %v", imports)
	}
}

// TestExtractImports_DepA verifies that dep_a.go (no imports) returns an empty slice.
func TestExtractImports_DepA(t *testing.T) {
	src := readTestdata(t, "dep_a.go")
	ctx := t.Context()

	imports, err := ExtractImports(ctx, src, Go, "dep_a.go")
	if err != nil {
		t.Fatalf("ExtractImports(dep_a.go): %v", err)
	}
	if len(imports) != 0 {
		t.Errorf("expected empty slice, got %v", imports)
	}
}

// TestExtractImports_AliasedImport verifies that an aliased import still returns the import path.
func TestExtractImports_AliasedImport(t *testing.T) {
	src := []byte(`package example

import fmtalias "fmt"

func Use() {
	fmtalias.Println("hello")
}
`)
	ctx := t.Context()

	imports, err := ExtractImports(ctx, src, Go, "alias.go")
	if err != nil {
		t.Fatalf("ExtractImports(alias): %v", err)
	}
	if !slices.Contains(imports, "fmt") {
		t.Errorf("expected \"fmt\" in imports, got %v", imports)
	}
}

// TestExtractImports_TypeScript verifies ExtractImports on inline TypeScript source.
func TestExtractImports_TypeScript(t *testing.T) {
	src := []byte(`import { foo } from './foo';
import bar from './bar';
`)
	ctx := t.Context()

	imports, err := ExtractImports(ctx, src, TypeScript, "example.ts")
	if err != nil {
		t.Fatalf("ExtractImports(TypeScript): %v", err)
	}
	if len(imports) == 0 {
		t.Error("expected at least one import for TypeScript source, got none")
	}
}

// TestExtractImports_Python verifies ExtractImports on inline Python source.
func TestExtractImports_Python(t *testing.T) {
	src := []byte(`import os
import sys
from pathlib import Path
`)
	ctx := t.Context()

	imports, err := ExtractImports(ctx, src, Python, "example.py")
	if err != nil {
		t.Fatalf("ExtractImports(Python): %v", err)
	}
	if len(imports) == 0 {
		t.Error("expected at least one import for Python source, got none")
	}
	if !slices.Contains(imports, "os") {
		t.Errorf("expected \"os\" in imports, got %v", imports)
	}
}

// TestExtractImports_Bash verifies ExtractImports on inline Bash source.
func TestExtractImports_Bash(t *testing.T) {
	src := []byte(`#!/bin/bash
source ./utils.sh
echo "hello"
`)
	ctx := t.Context()

	imports, err := ExtractImports(ctx, src, Bash, "main.sh")
	if err != nil {
		t.Fatalf("ExtractImports(Bash): %v", err)
	}
	if len(imports) == 0 {
		t.Error("expected at least one import (source) for Bash source, got none")
	}
}

// TestExtractImports_UnsupportedLanguage verifies that an unsupported language returns non-nil error.
func TestExtractImports_UnsupportedLanguage(t *testing.T) {
	ctx := t.Context()
	_, err := ExtractImports(ctx, []byte("some code"), Language("ruby"), "file.rb")
	if err == nil {
		t.Error("expected non-nil error for unsupported language, got nil")
	}
	if !strings.Contains(err.Error(), "ruby") {
		t.Errorf("error message does not mention 'ruby': %q", err.Error())
	}
}

// ── ExtractCallSites ──────────────────────────────────────────────────────────

// TestExtractCallSites_DepC verifies that dep_c.go (contains fmt.Sprintf(...))
// returns a CallSite with Symbol == "Sprintf" and a valid line number.
func TestExtractCallSites_DepC(t *testing.T) {
	src := readTestdata(t, "dep_c.go")
	ctx := t.Context()

	callSites, err := ExtractCallSites(ctx, src, Go, "dep_c.go")
	if err != nil {
		t.Fatalf("ExtractCallSites(dep_c.go): %v", err)
	}

	found := false
	for _, cs := range callSites {
		if cs.Symbol == "Sprintf" {
			found = true
			if cs.Line <= 0 {
				t.Errorf("CallSite.Line should be > 0, got %d", cs.Line)
			}
			break
		}
	}
	if !found {
		t.Errorf("expected CallSite with Symbol==\"Sprintf\", got %v", callSites)
	}
}

// TestExtractCallSites_DepA verifies that dep_a.go (no calls) returns empty slice.
func TestExtractCallSites_DepA(t *testing.T) {
	src := readTestdata(t, "dep_a.go")
	ctx := t.Context()

	callSites, err := ExtractCallSites(ctx, src, Go, "dep_a.go")
	if err != nil {
		t.Fatalf("ExtractCallSites(dep_a.go): %v", err)
	}
	if len(callSites) != 0 {
		t.Errorf("expected empty slice for dep_a.go, got %v", callSites)
	}
}

// TestExtractCallSites_TypeScript verifies ExtractCallSites on inline TypeScript source.
func TestExtractCallSites_TypeScript(t *testing.T) {
	src := []byte(`function greet(name: string): string {
  return doGreet(name);
}
`)
	ctx := t.Context()

	callSites, err := ExtractCallSites(ctx, src, TypeScript, "example.ts")
	if err != nil {
		t.Fatalf("ExtractCallSites(TypeScript): %v", err)
	}
	found := false
	for _, cs := range callSites {
		if cs.Symbol == "doGreet" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected CallSite with Symbol==\"doGreet\" in TS, got %v", callSites)
	}
}

// TestExtractCallSites_Python verifies ExtractCallSites on inline Python source.
func TestExtractCallSites_Python(t *testing.T) {
	src := []byte(`def main():
    result = compute(42)
    return result
`)
	ctx := t.Context()

	callSites, err := ExtractCallSites(ctx, src, Python, "example.py")
	if err != nil {
		t.Fatalf("ExtractCallSites(Python): %v", err)
	}
	found := false
	for _, cs := range callSites {
		if cs.Symbol == "compute" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected CallSite with Symbol==\"compute\" in Python, got %v", callSites)
	}
}

// TestExtractCallSites_UnsupportedLanguage verifies that an unsupported language returns non-nil error.
func TestExtractCallSites_UnsupportedLanguage(t *testing.T) {
	ctx := t.Context()
	_, err := ExtractCallSites(ctx, []byte("some code"), Language("ruby"), "file.rb")
	if err == nil {
		t.Error("expected non-nil error for unsupported language, got nil")
	}
	if !strings.Contains(err.Error(), "ruby") {
		t.Errorf("error message does not mention 'ruby': %q", err.Error())
	}
}

// ── BuildDependencyGraph ──────────────────────────────────────────────────────

// TestBuildDependencyGraph_TwoFileGoDir verifies that a two-file Go temp dir with
// module "example.com/test" produces exactly one edge {From:"cmd/main.go", To:"pkg/a.go"}.
func TestBuildDependencyGraph_TwoFileGoDir(t *testing.T) {
	dir := t.TempDir()

	// Write go.mod
	goMod := []byte("module example.com/test\n\ngo 1.21\n")
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), goMod, 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	// Write pkg/a.go (no imports)
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o700); err != nil {
		t.Fatalf("mkdir pkg: %v", err)
	}
	pkgA := []byte("package pkg\n\nfunc AFunc() {}\n")
	if err := os.WriteFile(filepath.Join(dir, "pkg", "a.go"), pkgA, 0o600); err != nil {
		t.Fatalf("write pkg/a.go: %v", err)
	}

	// Write cmd/main.go (imports "example.com/test/pkg")
	if err := os.MkdirAll(filepath.Join(dir, "cmd"), 0o700); err != nil {
		t.Fatalf("mkdir cmd: %v", err)
	}
	cmdMain := []byte(`package main

import "example.com/test/pkg"

func main() {
	pkg.AFunc()
}
`)
	if err := os.WriteFile(filepath.Join(dir, "cmd", "main.go"), cmdMain, 0o600); err != nil {
		t.Fatalf("write cmd/main.go: %v", err)
	}

	ctx := t.Context()
	graph, err := BuildDependencyGraph(ctx, dir, Go)
	if err != nil {
		t.Fatalf("BuildDependencyGraph: %v", err)
	}

	// Both files must be in Nodes.
	nodeSet := make(map[string]bool, len(graph.Nodes))
	for _, n := range graph.Nodes {
		nodeSet[n] = true
	}
	if !nodeSet["pkg/a.go"] {
		t.Errorf("expected 'pkg/a.go' in Nodes, got %v", graph.Nodes)
	}
	if !nodeSet["cmd/main.go"] {
		t.Errorf("expected 'cmd/main.go' in Nodes, got %v", graph.Nodes)
	}

	// Exactly one edge: cmd/main.go → pkg/a.go
	if len(graph.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d: %v", len(graph.Edges), graph.Edges)
	}
	e := graph.Edges[0]
	if e.From != "cmd/main.go" || e.To != "pkg/a.go" {
		t.Errorf("expected edge {cmd/main.go -> pkg/a.go}, got {%s -> %s}", e.From, e.To)
	}
}

// TestBuildDependencyGraph_EmptyDir verifies that an empty directory returns empty Nodes and Edges.
func TestBuildDependencyGraph_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	ctx := t.Context()

	graph, err := BuildDependencyGraph(ctx, dir, Go)
	if err != nil {
		t.Fatalf("BuildDependencyGraph(empty dir): %v", err)
	}
	if len(graph.Nodes) != 0 {
		t.Errorf("expected empty Nodes, got %v", graph.Nodes)
	}
	if len(graph.Edges) != 0 {
		t.Errorf("expected empty Edges, got %v", graph.Edges)
	}
}

// TestBuildDependencyGraph_NonMatchingFiles verifies that a dir with only .txt files for
// language=Go returns an empty graph.
func TestBuildDependencyGraph_NonMatchingFiles(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatalf("write readme.txt: %v", err)
	}

	ctx := t.Context()
	graph, err := BuildDependencyGraph(ctx, dir, Go)
	if err != nil {
		t.Fatalf("BuildDependencyGraph(non-matching files): %v", err)
	}
	if len(graph.Nodes) != 0 {
		t.Errorf("expected empty Nodes, got %v", graph.Nodes)
	}
	if len(graph.Edges) != 0 {
		t.Errorf("expected empty Edges, got %v", graph.Edges)
	}
}

// ── ReverseDependencies ───────────────────────────────────────────────────────

// TestReverseDependencies_Direct verifies A→B; target=B → [{A,1}].
func TestReverseDependencies_Direct(t *testing.T) {
	graph := DependencyGraph{
		Nodes: []string{"a.go", "b.go"},
		Edges: []Edge{{From: "a.go", To: "b.go"}},
	}

	result := ReverseDependencies(graph, "b.go")
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d: %v", len(result), result)
	}
	if result[0].File != "a.go" || result[0].Distance != 1 {
		t.Errorf("expected {a.go, 1}, got %v", result[0])
	}
}

// TestReverseDependencies_Transitive verifies A→B, B→C; target=C → [{B,1},{A,2}].
func TestReverseDependencies_Transitive(t *testing.T) {
	graph := DependencyGraph{
		Nodes: []string{"a.go", "b.go", "c.go"},
		Edges: []Edge{
			{From: "a.go", To: "b.go"},
			{From: "b.go", To: "c.go"},
		},
	}

	result := ReverseDependencies(graph, "c.go")
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(result), result)
	}
	if result[0].File != "b.go" || result[0].Distance != 1 {
		t.Errorf("expected result[0]={b.go, 1}, got %v", result[0])
	}
	if result[1].File != "a.go" || result[1].Distance != 2 {
		t.Errorf("expected result[1]={a.go, 2}, got %v", result[1])
	}
}

// TestReverseDependencies_BashTwoHop verifies the Bash two-hop case:
// lib.sh→utils.sh, main.sh→lib.sh; target=utils.sh → [{lib.sh,1},{main.sh,2}].
func TestReverseDependencies_BashTwoHop(t *testing.T) {
	graph := DependencyGraph{
		Nodes: []string{"lib.sh", "main.sh", "utils.sh"},
		Edges: []Edge{
			{From: "lib.sh", To: "utils.sh"},
			{From: "main.sh", To: "lib.sh"},
		},
	}

	result := ReverseDependencies(graph, "utils.sh")
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(result), result)
	}
	if result[0].File != "lib.sh" || result[0].Distance != 1 {
		t.Errorf("expected result[0]={lib.sh, 1}, got %v", result[0])
	}
	if result[1].File != "main.sh" || result[1].Distance != 2 {
		t.Errorf("expected result[1]={main.sh, 2}, got %v", result[1])
	}
}

// TestReverseDependencies_TargetNotInGraph verifies that a target not in the graph
// returns an empty slice (len==0).
func TestReverseDependencies_TargetNotInGraph(t *testing.T) {
	graph := DependencyGraph{
		Nodes: []string{"a.go", "b.go"},
		Edges: []Edge{{From: "a.go", To: "b.go"}},
	}

	result := ReverseDependencies(graph, "nonexistent.go")
	if len(result) != 0 {
		t.Errorf("expected empty slice for non-existent target, got %v", result)
	}
}

// TestReverseDependencies_Circular verifies that A→B, B→A; target=A returns
// exactly [{B,1}] with len==1 and A absent.
func TestReverseDependencies_Circular(t *testing.T) {
	graph := DependencyGraph{
		Nodes: []string{"a.go", "b.go"},
		Edges: []Edge{
			{From: "a.go", To: "b.go"},
			{From: "b.go", To: "a.go"},
		},
	}

	result := ReverseDependencies(graph, "a.go")
	if len(result) != 1 {
		t.Fatalf("expected exactly 1 entry for circular import (no infinite loop), got %d: %v", len(result), result)
	}
	if result[0].File != "b.go" || result[0].Distance != 1 {
		t.Errorf("expected {b.go, 1}, got %v", result[0])
	}
	// Ensure "a.go" (the target) is absent from results.
	for _, entry := range result {
		if entry.File == "a.go" {
			t.Errorf("target 'a.go' should not appear in ReverseDependencies results, but found %v", result)
		}
	}
}

// ── FindCallers ───────────────────────────────────────────────────────────────

// writeTempGoModule creates a temporary Go module in dir with module name "example.com/test".
func writeTempGoModule(t *testing.T, dir string) {
	t.Helper()
	const moduleName = "example.com/test"
	goMod := []byte("module " + moduleName + "\n\ngo 1.21\n")
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), goMod, 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
}

// TestFindCallers_GoImportsAndCalls verifies: Go file imports target pkg AND calls Symbol()
// → returned with distance=1.
func TestFindCallers_GoImportsAndCalls(t *testing.T) {
	dir := t.TempDir()
	writeTempGoModule(t, dir)

	// Target file: pkg/target.go
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o700); err != nil {
		t.Fatalf("mkdir pkg: %v", err)
	}
	targetSrc := []byte("package pkg\n\nfunc Symbol() {}\n")
	if err := os.WriteFile(filepath.Join(dir, "pkg", "target.go"), targetSrc, 0o600); err != nil {
		t.Fatalf("write pkg/target.go: %v", err)
	}

	// Caller file: cmd/main.go — imports "example.com/test/pkg" and calls pkg.Symbol()
	if err := os.MkdirAll(filepath.Join(dir, "cmd"), 0o700); err != nil {
		t.Fatalf("mkdir cmd: %v", err)
	}
	callerSrc := []byte(`package main

import "example.com/test/pkg"

func main() {
	pkg.Symbol()
}
`)
	if err := os.WriteFile(filepath.Join(dir, "cmd", "main.go"), callerSrc, 0o600); err != nil {
		t.Fatalf("write cmd/main.go: %v", err)
	}

	ctx := t.Context()
	results, err := FindCallers(ctx, dir, Go, "pkg/target.go", "Symbol")
	if err != nil {
		t.Fatalf("FindCallers: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(results), results)
	}
	if results[0].File != "cmd/main.go" {
		t.Errorf("expected File==\"cmd/main.go\", got %q", results[0].File)
	}
	if results[0].Distance != 1 {
		t.Errorf("expected Distance==1, got %d", results[0].Distance)
	}
}

// TestFindCallers_GoImportsButWrongCall verifies: Go file imports target pkg but calls
// only OtherFunc() → NOT returned.
func TestFindCallers_GoImportsButWrongCall(t *testing.T) {
	dir := t.TempDir()
	writeTempGoModule(t, dir)

	// Target file: pkg/target.go
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o700); err != nil {
		t.Fatalf("mkdir pkg: %v", err)
	}
	targetSrc := []byte("package pkg\n\nfunc Symbol() {}\nfunc OtherFunc() {}\n")
	if err := os.WriteFile(filepath.Join(dir, "pkg", "target.go"), targetSrc, 0o600); err != nil {
		t.Fatalf("write pkg/target.go: %v", err)
	}

	// Caller file: cmd/main.go — imports "example.com/test/pkg" but calls OtherFunc() only
	if err := os.MkdirAll(filepath.Join(dir, "cmd"), 0o700); err != nil {
		t.Fatalf("mkdir cmd: %v", err)
	}
	callerSrc := []byte(`package main

import "example.com/test/pkg"

func main() {
	pkg.OtherFunc()
}
`)
	if err := os.WriteFile(filepath.Join(dir, "cmd", "main.go"), callerSrc, 0o600); err != nil {
		t.Fatalf("write cmd/main.go: %v", err)
	}

	ctx := t.Context()
	results, err := FindCallers(ctx, dir, Go, "pkg/target.go", "Symbol")
	if err != nil {
		t.Fatalf("FindCallers: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected no results when only OtherFunc is called, got %v", results)
	}
}

// TestFindCallers_GoNoImport verifies: Go file does not import target → not returned.
func TestFindCallers_GoNoImport(t *testing.T) {
	dir := t.TempDir()
	writeTempGoModule(t, dir)

	// Target file: pkg/target.go
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o700); err != nil {
		t.Fatalf("mkdir pkg: %v", err)
	}
	targetSrc := []byte("package pkg\n\nfunc Symbol() {}\n")
	if err := os.WriteFile(filepath.Join(dir, "pkg", "target.go"), targetSrc, 0o600); err != nil {
		t.Fatalf("write pkg/target.go: %v", err)
	}

	// Unrelated file: cmd/main.go — does NOT import "example.com/test/pkg"
	if err := os.MkdirAll(filepath.Join(dir, "cmd"), 0o700); err != nil {
		t.Fatalf("mkdir cmd: %v", err)
	}
	callerSrc := []byte(`package main

func main() {
}
`)
	if err := os.WriteFile(filepath.Join(dir, "cmd", "main.go"), callerSrc, 0o600); err != nil {
		t.Fatalf("write cmd/main.go: %v", err)
	}

	ctx := t.Context()
	results, err := FindCallers(ctx, dir, Go, "pkg/target.go", "Symbol")
	if err != nil {
		t.Fatalf("FindCallers: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected no results when target is not imported, got %v", results)
	}
}

// TestFindCallers_TargetNotFound verifies: target file not found → empty result, no error.
func TestFindCallers_TargetNotFound(t *testing.T) {
	dir := t.TempDir()
	writeTempGoModule(t, dir)

	// Write a Go file with no imports
	src := []byte("package main\n\nfunc main() {}\n")
	if err := os.WriteFile(filepath.Join(dir, "main.go"), src, 0o600); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	ctx := t.Context()
	results, err := FindCallers(ctx, dir, Go, "nonexistent/target.go", "Symbol")
	if err != nil {
		t.Fatalf("expected no error when target not found, got: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty results when target not found, got %v", results)
	}
}

// TestFindCallers_TSPositive verifies: TypeScript file calls Symbol() → returned with distance=-1.
func TestFindCallers_TSPositive(t *testing.T) {
	dir := t.TempDir()

	// Write target TS file (the target)
	targetSrc := []byte(`export function Symbol(): void {}
`)
	if err := os.WriteFile(filepath.Join(dir, "target.ts"), targetSrc, 0o600); err != nil {
		t.Fatalf("write target.ts: %v", err)
	}

	// Write caller TS file that calls Symbol()
	callerSrc := []byte(`import { Symbol } from './target';

function caller(): void {
  Symbol();
}
`)
	if err := os.WriteFile(filepath.Join(dir, "caller.ts"), callerSrc, 0o600); err != nil {
		t.Fatalf("write caller.ts: %v", err)
	}

	ctx := t.Context()
	results, err := FindCallers(ctx, dir, TypeScript, "target.ts", "Symbol")
	if err != nil {
		t.Fatalf("FindCallers(TypeScript): %v", err)
	}

	found := false
	for _, r := range results {
		if r.File == "caller.ts" {
			found = true
			if r.Distance != -1 {
				t.Errorf("expected Distance==-1 for TypeScript caller, got %d", r.Distance)
			}
			break
		}
	}
	if !found {
		t.Errorf("expected 'caller.ts' in results, got %v", results)
	}
}

// TestFindCallers_TSNegative verifies: TypeScript file present but calls DifferentFunc()
// only → NOT returned.
func TestFindCallers_TSNegative(t *testing.T) {
	dir := t.TempDir()

	// Write target TS file
	targetSrc := []byte(`export function Symbol(): void {}
`)
	if err := os.WriteFile(filepath.Join(dir, "target.ts"), targetSrc, 0o600); err != nil {
		t.Fatalf("write target.ts: %v", err)
	}

	// Write TS file that only calls DifferentFunc(), not Symbol()
	callerSrc := []byte(`function caller(): void {
  DifferentFunc();
}
`)
	if err := os.WriteFile(filepath.Join(dir, "other.ts"), callerSrc, 0o600); err != nil {
		t.Fatalf("write other.ts: %v", err)
	}

	ctx := t.Context()
	results, err := FindCallers(ctx, dir, TypeScript, "target.ts", "Symbol")
	if err != nil {
		t.Fatalf("FindCallers(TypeScript negative): %v", err)
	}

	for _, r := range results {
		if r.File == "other.ts" {
			t.Errorf("'other.ts' should NOT be returned (only calls DifferentFunc), got %v", results)
		}
	}
}

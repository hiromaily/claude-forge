// Package profile_test tests the profile package analyzer and cache.
package profile

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestAnalyzeOrUpdate_fresh_cache(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cachePath := filepath.Join(dir, "repo-profile.json")

	// Write a fresh cache (updated 1 hour ago).
	prof := &RepoProfile{
		Languages:     []Language{{Name: "Go", Percentage: 100}},
		TestFramework: "go test",
		CISystem:      "GitHub Actions",
		LinterConfigs: []string{"golangci-lint"},
		DirConventions: map[string]string{
			"scripts/": "shell scripts",
		},
		BranchNaming: "feature/{name}",
		BuildCommand: "make build",
		TestCommand:  "make test",
		Monorepo:     false,
		LastUpdated:  time.Now().Add(-1 * time.Hour),
		Staleness:    "fresh",
	}

	data, err := json.Marshal(prof)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if writeErr := os.WriteFile(cachePath, data, 0o600); writeErr != nil {
		t.Fatalf("write cache: %v", writeErr)
	}

	p := New(cachePath, dir)
	result, err := p.AnalyzeOrUpdate()
	if err != nil {
		t.Fatalf("AnalyzeOrUpdate: %v", err)
	}

	// Should return cached data without re-analysis.
	if result.TestFramework != "go test" {
		t.Errorf("expected cached TestFramework 'go test', got %q", result.TestFramework)
	}

	if result.Staleness != "fresh" {
		t.Errorf("expected Staleness 'fresh', got %q", result.Staleness)
	}
}

func TestAnalyzeOrUpdate_stale_cache(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cachePath := filepath.Join(dir, "repo-profile.json")

	// Write a stale cache (updated 8 days ago).
	prof := &RepoProfile{
		Languages:      []Language{{Name: "Go", Percentage: 100}},
		TestFramework:  "go test",
		CISystem:       "OldCI",
		LinterConfigs:  []string{},
		DirConventions: map[string]string{},
		LastUpdated:    time.Now().Add(-8 * 24 * time.Hour),
		Staleness:      "fresh",
	}

	data, err := json.Marshal(prof)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if writeErr := os.WriteFile(cachePath, data, 0o600); writeErr != nil {
		t.Fatalf("write cache: %v", writeErr)
	}

	p := New(cachePath, dir)
	result, err := p.AnalyzeOrUpdate()
	if err != nil {
		t.Fatalf("AnalyzeOrUpdate: %v", err)
	}

	// Re-analysis ran; CISystem should be re-detected (likely empty in temp dir),
	// not "OldCI".
	if result.CISystem == "OldCI" {
		t.Errorf("expected re-analysis after stale cache, but still got OldCI")
	}

	if result.Staleness != "fresh" {
		t.Errorf("expected Staleness 'fresh' after re-analysis, got %q", result.Staleness)
	}
}

func TestAnalyzeOrUpdate_no_cache(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cachePath := filepath.Join(dir, "repo-profile.json")

	p := New(cachePath, dir)
	result, err := p.AnalyzeOrUpdate()
	if err != nil {
		t.Fatalf("AnalyzeOrUpdate: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil profile")
	}

	if result.Staleness != "fresh" {
		t.Errorf("expected Staleness 'fresh', got %q", result.Staleness)
	}
}

func TestAnalyzeOrUpdate_this_repo(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make not on PATH")
	}

	// Derive repo root from this file's location.
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}

	// This file is at mcp-server/internal/intelligence/profile/analyzer_test.go
	// Repo root is four levels up.
	repoRoot := filepath.Join(filepath.Dir(filename), "..", "..", "..", "..")
	cachePath := filepath.Join(t.TempDir(), "repo-profile.json")

	p := New(cachePath, repoRoot)
	result, err := p.AnalyzeOrUpdate()
	if err != nil {
		t.Fatalf("AnalyzeOrUpdate: %v", err)
	}

	if result.CISystem != "GitHub Actions" {
		t.Errorf("expected CISystem 'GitHub Actions', got %q", result.CISystem)
	}

	if result.TestFramework != "go test" {
		t.Errorf("expected TestFramework 'go test', got %q", result.TestFramework)
	}

	if !slices.Contains(result.LinterConfigs, "golangci-lint") {
		t.Errorf("expected 'golangci-lint' in LinterConfigs, got %v", result.LinterConfigs)
	}

	if result.BuildCommand != "make build" {
		t.Errorf("expected BuildCommand 'make build', got %q", result.BuildCommand)
	}

	if result.TestCommand != "make test" {
		t.Errorf("expected TestCommand 'make test', got %q", result.TestCommand)
	}
}

func TestAnalyzeOrUpdate_graceful_degradation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cachePath := filepath.Join(dir, "repo-profile.json")

	// Use empty temp dir as repo root (no git, no make targets, no CI files).
	p := New(cachePath, dir)
	result, err := p.AnalyzeOrUpdate()
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil profile")
	}

	// All string fields should be safe (empty string is ok).
	_ = result.TestFramework
	_ = result.CISystem
	_ = result.BranchNaming
	_ = result.BuildCommand
	_ = result.TestCommand
}

func TestFormatForPrompt_nonempty(t *testing.T) {
	t.Parallel()

	p := New("", "")
	p.profile = &RepoProfile{
		Languages:     []Language{{Name: "Go", Percentage: 85}, {Name: "Shell", Percentage: 10}},
		TestFramework: "go test",
		CISystem:      "GitHub Actions",
		LinterConfigs: []string{"golangci-lint"},
		BuildCommand:  "make build",
		TestCommand:   "make test",
		BranchNaming:  "feature/{name}",
	}

	out := p.FormatForPrompt()
	if out == "" {
		t.Fatal("expected non-empty output")
	}

	checks := []string{"Go", "go test", "GitHub Actions"}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestFormatForPrompt_nil_profile(t *testing.T) {
	t.Parallel()

	p := New("", "")
	// profile is nil by default.

	out := p.FormatForPrompt()
	if out != "" {
		t.Errorf("expected empty string for nil profile, got %q", out)
	}
}

func TestSaveAndLoadCache_roundtrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cachePath := filepath.Join(dir, "repo-profile.json")

	now := time.Now().UTC().Truncate(time.Second)
	prof := &RepoProfile{
		Languages:     []Language{{Name: "Go", Percentage: 85}, {Name: "Shell", Percentage: 15}},
		TestFramework: "go test",
		CISystem:      "GitHub Actions",
		LinterConfigs: []string{"golangci-lint"},
		DirConventions: map[string]string{
			"scripts/": "shell scripts",
			"agents/":  "agent definitions",
		},
		BranchNaming: "feature/{name}",
		BuildCommand: "make build",
		TestCommand:  "make test",
		Monorepo:     true,
		LastUpdated:  now,
		Staleness:    "fresh",
	}

	p := New(cachePath, dir)

	if err := p.saveCache(prof); err != nil {
		t.Fatalf("saveCache: %v", err)
	}

	loaded := p.loadCache()
	if loaded == nil {
		t.Fatal("loadCache returned nil")
	}

	// Compare fields manually (time comparison needs special handling).
	if !reflect.DeepEqual(prof.Languages, loaded.Languages) {
		t.Errorf("Languages mismatch: want %v, got %v", prof.Languages, loaded.Languages)
	}

	if prof.TestFramework != loaded.TestFramework {
		t.Errorf("TestFramework mismatch")
	}

	if prof.CISystem != loaded.CISystem {
		t.Errorf("CISystem mismatch")
	}

	if !reflect.DeepEqual(prof.LinterConfigs, loaded.LinterConfigs) {
		t.Errorf("LinterConfigs mismatch")
	}

	if !reflect.DeepEqual(prof.DirConventions, loaded.DirConventions) {
		t.Errorf("DirConventions mismatch")
	}

	if prof.BranchNaming != loaded.BranchNaming {
		t.Errorf("BranchNaming mismatch")
	}

	if prof.BuildCommand != loaded.BuildCommand {
		t.Errorf("BuildCommand mismatch")
	}

	if prof.TestCommand != loaded.TestCommand {
		t.Errorf("TestCommand mismatch")
	}

	if prof.Monorepo != loaded.Monorepo {
		t.Errorf("Monorepo mismatch")
	}

	if !prof.LastUpdated.Equal(loaded.LastUpdated) {
		t.Errorf("LastUpdated mismatch: want %v, got %v", prof.LastUpdated, loaded.LastUpdated)
	}
}

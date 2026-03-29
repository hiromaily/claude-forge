// Package tools — tests for ProfileGetHandler.
package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hiromaily/claude-forge/mcp-server/profile"
)

// TestProfileGetHandler_nil_profiler verifies that a nil profiler returns
// an MCP error result with the message "profiler not available".
func TestProfileGetHandler_nil_profiler(t *testing.T) {
	t.Parallel()

	handler := ProfileGetHandler(nil)
	req := mcp.CallToolRequest{}

	result, err := handler(t.Context(), req)
	if err != nil {
		t.Fatalf("unexpected handler error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError == true for nil profiler, got false")
	}
	// Verify error text contains the expected message.
	var sb strings.Builder
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	errText := sb.String()
	if !strings.Contains(errText, "profiler not available") {
		t.Errorf("expected error text to contain \"profiler not available\", got: %q", errText)
	}
}

// cacheOnDisk is the on-disk JSON format matching the profile package's internal
// cacheJSON struct. Duplicated here to avoid reaching into unexported internals.
type cacheOnDisk struct {
	Languages      []profile.Language `json:"languages"`
	TestFramework  string             `json:"test_framework"`
	CISystem       string             `json:"ci_system"`
	LinterConfigs  []string           `json:"linter_configs"`
	DirConventions map[string]string  `json:"dir_conventions"`
	BranchNaming   string             `json:"branch_naming"`
	BuildCommand   string             `json:"build_command"`
	TestCommand    string             `json:"test_command"`
	Monorepo       bool               `json:"monorepo"`
	LastUpdated    string             `json:"last_updated"` // RFC3339
	Staleness      string             `json:"staleness"`
}

// TestProfileGetHandler_with_cache constructs a profiler with a pre-written
// cache in t.TempDir() and verifies the JSON response contains expected fields.
func TestProfileGetHandler_with_cache(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cachePath := filepath.Join(dir, "repo-profile.json")

	// Write a pre-computed cache in the format that loadCache expects.
	cached := cacheOnDisk{
		Languages: []profile.Language{
			{Name: "Go", Percentage: 85},
			{Name: "Shell", Percentage: 10},
		},
		TestFramework: "go test",
		CISystem:      "GitHub Actions",
		LinterConfigs: []string{"golangci-lint"},
		DirConventions: map[string]string{
			"agents/":  "agent definitions",
			"scripts/": "shell scripts",
		},
		BranchNaming: "feature/{name}",
		BuildCommand: "make build",
		TestCommand:  "make test",
		Monorepo:     false,
		// Recent LastUpdated so AnalyzeOrUpdate returns without re-analysis.
		LastUpdated: time.Now().UTC().Format(time.RFC3339),
		Staleness:   "fresh",
	}
	data, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal test cache: %v", err)
	}
	if err := os.WriteFile(cachePath, data, 0o600); err != nil {
		t.Fatalf("failed to write test cache: %v", err)
	}

	// Create a profiler pointing at the temp cache.
	profiler := profile.New(cachePath, dir)

	handler := ProfileGetHandler(profiler)
	req := mcp.CallToolRequest{}

	result, err := handler(t.Context(), req)
	if err != nil {
		t.Fatalf("unexpected handler error: %v", err)
	}
	if result.IsError {
		var eb strings.Builder
		for _, c := range result.Content {
			if tc, ok := c.(mcp.TextContent); ok {
				eb.WriteString(tc.Text)
			}
		}
		t.Fatalf("unexpected MCP error: %s", eb.String())
	}

	// Extract text content.
	var rb strings.Builder
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			rb.WriteString(tc.Text)
		}
	}
	responseText := rb.String()

	// Verify the response is valid JSON with expected fields.
	var parsed map[string]any
	if err := json.Unmarshal([]byte(responseText), &parsed); err != nil {
		t.Fatalf("response is not valid JSON: %v\nresponse: %s", err, responseText)
	}

	// Check required fields are present.
	for _, field := range []string{"languages", "test_framework", "ci_system"} {
		if _, ok := parsed[field]; !ok {
			t.Errorf("expected field %q in response, got keys: %v", field, profileMapKeys(parsed))
		}
	}

	// Verify specific values.
	if tf, ok := parsed["test_framework"].(string); !ok || tf != "go test" {
		t.Errorf("expected test_framework == \"go test\", got: %v", parsed["test_framework"])
	}
	if ci, ok := parsed["ci_system"].(string); !ok || ci != "GitHub Actions" {
		t.Errorf("expected ci_system == \"GitHub Actions\", got: %v", parsed["ci_system"])
	}
}

// profileMapKeys returns the keys of a map[string]any for test error messages.
func profileMapKeys(m map[string]any) []string {
	result := make([]string, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	return result
}

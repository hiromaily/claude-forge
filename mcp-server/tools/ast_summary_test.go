// Package tools — handler tests for ast_summary.
package tools

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// astTestdataDir returns the absolute path to mcp-server/ast/testdata/.
func astTestdataDir() string {
	_, file, _, _ := runtime.Caller(0)
	// file is .../mcp-server/tools/ast_summary_test.go
	return filepath.Join(filepath.Dir(file), "..", "ast", "testdata")
}

func TestAstSummaryFromPath_Go(t *testing.T) {
	path := filepath.Join(astTestdataDir(), "sample.go")
	result, err := astSummaryFromPath(context.Background(), path, "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got IsError=true: %s", textContent(result))
	}
	text := textContent(result)
	if !strings.Contains(text, "## Functions") {
		t.Errorf("expected ## Functions section, got:\n%s", text)
	}
}

func TestAstSummaryFromPath_AutoDetect(t *testing.T) {
	// language="" with a .go path — should auto-detect as Go.
	path := filepath.Join(astTestdataDir(), "sample.go")
	result, err := astSummaryFromPath(context.Background(), path, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got IsError=true: %s", textContent(result))
	}
	text := textContent(result)
	if !strings.Contains(text, "## Functions") {
		t.Errorf("expected ## Functions section, got:\n%s", text)
	}
}

func TestAstSummaryFromPath_FileNotFound(t *testing.T) {
	result, err := astSummaryFromPath(context.Background(), "/nonexistent/path/file.go", "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected IsError=true for non-existent file")
	}
}

func TestAstSummaryFromPath_UnsupportedLanguage(t *testing.T) {
	path := filepath.Join(astTestdataDir(), "sample.go")
	result, err := astSummaryFromPath(context.Background(), path, "cobol")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected IsError=true for unsupported language")
	}
}

func TestAstSummaryFromPath_UnknownExtension(t *testing.T) {
	// language="" with a .rb path — should return IsError=true (unrecognized extension).
	tmpFile := filepath.Join(t.TempDir(), "code.rb")
	if err := os.WriteFile(tmpFile, []byte("def foo; end"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	result, err := astSummaryFromPath(context.Background(), tmpFile, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected IsError=true for unrecognized extension")
	}
}

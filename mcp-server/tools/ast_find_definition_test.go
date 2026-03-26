// Package tools — handler-layer tests for ast_find_definition.
package tools

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestAstFindDefinitionFromPath_Found verifies that a known symbol is found and
// returned without a count header when there is exactly one match.
func TestAstFindDefinitionFromPath_Found(t *testing.T) {
	ctx := context.Background()
	// sample.go contains exactly one definition of "Greet".
	result, err := astFindDefinitionFromPath(ctx,
		"../ast/testdata/sample.go",
		"go",
		"Greet",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertNotError(t, result, "Greet lookup")
	text := toolResultText(result)
	if !strings.Contains(text, "Greet") {
		t.Errorf("result text %q does not contain symbol 'Greet'", text)
	}
	// Single match: must NOT contain a count header.
	if strings.Contains(text, "matches found") {
		t.Errorf("single match should not contain 'matches found', got: %q", text)
	}
}

// TestAstFindDefinitionFromPath_NotFound verifies that a missing symbol returns
// IsError=false with empty text.
func TestAstFindDefinitionFromPath_NotFound(t *testing.T) {
	ctx := context.Background()
	result, err := astFindDefinitionFromPath(ctx,
		"../ast/testdata/sample.go",
		"go",
		"NonExistentSymbolXYZ",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertNotError(t, result, "not-found symbol")
	text := toolResultText(result)
	if text != "" {
		t.Errorf("expected empty text for not-found symbol, got: %q", text)
	}
}

// TestAstFindDefinitionFromPath_MultipleMatches verifies that when multiple
// definitions share the same name, a count header is prepended.
func TestAstFindDefinitionFromPath_MultipleMatches(t *testing.T) {
	ctx := context.Background()
	// Write a TypeScript file with two method signatures named "add".
	tmpFile := t.TempDir() + "/multi.ts"
	content := `// Test file with two overloaded method signatures.
interface Calculator {
  add(a: number, b: number): number;
  add(a: string, b: string): string;
}
`
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	result, err := astFindDefinitionFromPath(ctx, tmpFile, "typescript", "add")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertNotError(t, result, "multiple matches")
	text := toolResultText(result)
	// With multiple matches, text should start with a count line.
	if !strings.HasPrefix(text, "2 matches found") {
		t.Errorf("expected '2 matches found' prefix, got: %q", text)
	}
}

// TestAstFindDefinitionFromPath_FileNotFound verifies that a non-existent file
// returns IsError=true.
func TestAstFindDefinitionFromPath_FileNotFound(t *testing.T) {
	ctx := context.Background()
	result, err := astFindDefinitionFromPath(ctx,
		"/nonexistent/path/to/file.go",
		"go",
		"Foo",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsError {
		t.Errorf("expected IsError=true for missing file, got IsError=false")
	}
}

// TestAstFindDefinitionFromPath_UnsupportedLanguage verifies that an unsupported
// language returns IsError=true.
func TestAstFindDefinitionFromPath_UnsupportedLanguage(t *testing.T) {
	ctx := context.Background()
	result, err := astFindDefinitionFromPath(ctx,
		"../ast/testdata/sample.go",
		"ruby",
		"Greet",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsError {
		t.Errorf("expected IsError=true for unsupported language, got IsError=false")
	}
}

// TestAstFindDefinitionFromPath_AutoDetect verifies that language="" auto-detects
// from the file extension.
func TestAstFindDefinitionFromPath_AutoDetect(t *testing.T) {
	ctx := context.Background()
	result, err := astFindDefinitionFromPath(ctx,
		"../ast/testdata/sample.go",
		"", // auto-detect from .go extension
		"Greet",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertNotError(t, result, "auto-detect language")
	if !strings.Contains(toolResultText(result), "Greet") {
		t.Errorf("expected result to contain 'Greet', got: %q", toolResultText(result))
	}
}

// TestAstFindDefinitionFromPath_UnknownExtension verifies that language="" with
// an unrecognized extension returns IsError=true.
func TestAstFindDefinitionFromPath_UnknownExtension(t *testing.T) {
	ctx := context.Background()
	tmpFile := t.TempDir() + "/code.rb"
	if err := os.WriteFile(tmpFile, []byte("def foo; end"), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	result, err := astFindDefinitionFromPath(ctx, tmpFile, "", "foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsError {
		t.Errorf("expected IsError=true for unknown extension, got IsError=false")
	}
}

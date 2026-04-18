// Package ast provides unit tests for the ast domain package.
package ast

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

// testdataPath returns the absolute path to a file in the testdata directory.
func testdataPath(name string) string {
	// runtime.Caller(0) gives us the current source file path.
	// Using filepath.Join relative to the test file location.
	dir, err := filepath.Abs("testdata")
	if err != nil {
		panic(fmt.Sprintf("cannot resolve testdata path: %v", err))
	}
	return filepath.Join(dir, name)
}

// readTestdata reads the contents of a testdata file.
func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(testdataPath(name))
	if err != nil {
		t.Fatalf("readTestdata(%q): %v", name, err)
	}
	return data
}

// wordCount returns the number of whitespace-separated words in s.
func wordCount(s string) int {
	return len(strings.Fields(s))
}

// formatMarkdown renders a Summary as markdown text (mirroring what the MCP handler produces).
// This is used by TestTokenReduction to count words in the output.
func formatMarkdown(s Summary) string {
	var b strings.Builder
	if len(s.Functions) > 0 {
		b.WriteString("## Functions\n")
		for _, f := range s.Functions {
			b.WriteString(f)
			b.WriteString("\n")
		}
	}
	if len(s.Types) > 0 {
		b.WriteString("## Types\n")
		for _, tp := range s.Types {
			b.WriteString(tp)
			b.WriteString("\n")
		}
	}
	if len(s.Constants) > 0 {
		b.WriteString("## Constants\n")
		for _, c := range s.Constants {
			b.WriteString(c)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// TestSummarizeGo parses testdata/sample.go and asserts that:
//   - exported function signatures are present
//   - unexported functions are absent
//   - function bodies are absent from every entry
func TestSummarizeGo(t *testing.T) {
	src := readTestdata(t, "sample.go")
	ctx := context.Background()

	summary, err := Summarize(ctx, src, Go)
	if err != nil {
		t.Fatalf("Summarize(Go): %v", err)
	}

	// Must contain at least the exported functions Greet and Add.
	exportedWant := []string{"Greet", "Add"}
	for _, name := range exportedWant {
		found := false
		for _, fn := range summary.Functions {
			if strings.Contains(fn, name) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("exported function %q not found in summary.Functions: %v", name, summary.Functions)
		}
	}

	// Unexported function "helper" must not appear.
	for _, fn := range summary.Functions {
		if strings.Contains(fn, "helper") {
			t.Errorf("unexported function 'helper' found in summary.Functions: %q", fn)
		}
	}

	// All function entries must start with an uppercase letter (exported).
	for _, fn := range summary.Functions {
		// Strip "func " prefix if present to get the name.
		name := strings.TrimPrefix(fn, "func ")
		if len(name) == 0 {
			t.Errorf("empty signature entry in summary.Functions")
			continue
		}
		if !unicode.IsUpper(rune(name[0])) {
			t.Errorf("unexported signature in summary.Functions: %q", fn)
		}
	}

	// Function bodies must not appear (no return statements from bodies).
	for _, fn := range summary.Functions {
		if strings.Contains(fn, "return fmt.Sprintf") {
			t.Errorf("function body text found in signature: %q", fn)
		}
		if strings.Contains(fn, "return a + b") {
			t.Errorf("function body text found in signature: %q", fn)
		}
	}

	// sample.go exports one type: StatusCode
	foundStatusCode := false
	for _, tp := range summary.Types {
		if strings.Contains(tp, "StatusCode") {
			foundStatusCode = true
			break
		}
	}
	if !foundStatusCode {
		t.Errorf("exported type 'StatusCode' not found in summary.Types: %v", summary.Types)
	}

	// sample.go has exported constant DefaultTimeout
	foundConst := false
	for _, c := range summary.Constants {
		if strings.Contains(c, "DefaultTimeout") {
			foundConst = true
			break
		}
	}
	if !foundConst {
		t.Errorf("exported constant 'DefaultTimeout' not found in summary.Constants: %v", summary.Constants)
	}
}

// TestSummarizeTypeScript parses testdata/sample.ts and asserts that:
//   - interface declarations are extracted into summary.Types
//   - function signatures are extracted into summary.Functions
func TestSummarizeTypeScript(t *testing.T) {
	src := readTestdata(t, "sample.ts")
	ctx := context.Background()

	summary, err := Summarize(ctx, src, TypeScript)
	if err != nil {
		t.Fatalf("Summarize(TypeScript): %v", err)
	}

	// sample.ts has interface User and type alias UserId.
	typesWant := []string{"User", "UserId"}
	for _, name := range typesWant {
		found := false
		for _, tp := range summary.Types {
			if strings.Contains(tp, name) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("type/interface %q not found in summary.Types: %v", name, summary.Types)
		}
	}

	// sample.ts has exported functions greetUser and getUserById.
	functionsWant := []string{"greetUser", "getUserById"}
	for _, name := range functionsWant {
		found := false
		for _, fn := range summary.Functions {
			if strings.Contains(fn, name) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("function %q not found in summary.Functions: %v", name, summary.Functions)
		}
	}

	// Function bodies must not appear.
	for _, fn := range summary.Functions {
		if strings.Contains(fn, "return `Hello") {
			t.Errorf("function body text found in TS signature: %q", fn)
		}
		if strings.Contains(fn, "return undefined") {
			t.Errorf("function body text found in TS signature: %q", fn)
		}
	}
}

// TestSummarizePython parses testdata/sample.py and asserts that:
//   - class definitions appear in summary.Types
//   - function definitions appear in summary.Functions
func TestSummarizePython(t *testing.T) {
	src := readTestdata(t, "sample.py")
	ctx := context.Background()

	summary, err := Summarize(ctx, src, Python)
	if err != nil {
		t.Fatalf("Summarize(Python): %v", err)
	}

	// sample.py has class Animal.
	foundAnimal := false
	for _, tp := range summary.Types {
		if strings.Contains(tp, "Animal") {
			foundAnimal = true
			break
		}
	}
	if !foundAnimal {
		t.Errorf("class 'Animal' not found in summary.Types: %v", summary.Types)
	}

	// sample.py has functions greet and add.
	functionsWant := []string{"greet", "add"}
	for _, name := range functionsWant {
		found := false
		for _, fn := range summary.Functions {
			if strings.Contains(fn, name) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("function %q not found in summary.Functions: %v", name, summary.Functions)
		}
	}

	// Function bodies must not appear.
	for _, fn := range summary.Functions {
		if strings.Contains(fn, "return f\"Hello") {
			t.Errorf("function body text found in Python signature: %q", fn)
		}
	}
}

// TestSummarizeBash parses testdata/sample.sh and asserts that:
//   - function definitions appear in summary.Functions
func TestSummarizeBash(t *testing.T) {
	src := readTestdata(t, "sample.sh")
	ctx := context.Background()

	summary, err := Summarize(ctx, src, Bash)
	if err != nil {
		t.Fatalf("Summarize(Bash): %v", err)
	}

	// sample.sh has functions greet and add.
	functionsWant := []string{"greet", "add"}
	for _, name := range functionsWant {
		found := false
		for _, fn := range summary.Functions {
			if strings.Contains(fn, name) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("bash function %q not found in summary.Functions: %v", name, summary.Functions)
		}
	}

	// Function bodies must not appear (no echo statements).
	for _, fn := range summary.Functions {
		if strings.Contains(fn, "echo \"Hello") {
			t.Errorf("function body text found in bash signature: %q", fn)
		}
		if strings.Contains(fn, "echo $(( a + b ))") {
			t.Errorf("function body text found in bash signature: %q", fn)
		}
	}
}

// TestSummarizeUnsupportedLanguage verifies that Summarize returns a non-nil error
// when passed an unsupported Language value.
func TestSummarizeUnsupportedLanguage(t *testing.T) {
	ctx := context.Background()
	_, err := Summarize(ctx, []byte("some code"), Language("ruby"))
	if err == nil {
		t.Error("Summarize with Language('ruby'): expected non-nil error, got nil")
	}
	// Error message should mention "unsupported" and the language name.
	if !strings.Contains(err.Error(), "ruby") {
		t.Errorf("error message does not mention 'ruby': %q", err.Error())
	}
}

// TestFindDefinitionSingleMatch verifies that FindDefinition returns exactly one
// result when a symbol is defined once in a Go source file.
func TestFindDefinitionSingleMatch(t *testing.T) {
	src := []byte(`package sample

func Multiply(a, b int) int {
	return a * b
}

func Divide(a, b int) (int, error) {
	if b == 0 {
		return 0, fmt.Errorf("divide by zero")
	}
	return a / b, nil
}
`)
	ctx := context.Background()
	results, err := FindDefinition(ctx, src, Go, "Multiply")
	if err != nil {
		t.Fatalf("FindDefinition: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected exactly 1 result, got %d: %v", len(results), results)
	}
	if !strings.Contains(results[0], "Multiply") {
		t.Errorf("result does not contain 'Multiply': %q", results[0])
	}
}

// TestFindDefinitionMultipleMatches verifies that FindDefinition returns multiple
// results when a symbol name appears more than once (TypeScript overload-style).
func TestFindDefinitionMultipleMatches(t *testing.T) {
	// TypeScript allows function_signature (overload declarations) followed by a
	// function_declaration (implementation). Both share the same name.
	src := []byte(`
function process(x: number): number;
function process(x: string): string;
function process(x: any): any {
  return x;
}
`)
	ctx := context.Background()
	results, err := FindDefinition(ctx, src, TypeScript, "process")
	if err != nil {
		t.Fatalf("FindDefinition: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results for 'process', got %d: %v", len(results), results)
	}
	for _, r := range results {
		if !strings.Contains(r, "process") {
			t.Errorf("result does not contain 'process': %q", r)
		}
	}
}

// TestFindDefinitionNotFound verifies that FindDefinition returns an empty slice
// and nil error when the symbol does not exist in the source.
func TestFindDefinitionNotFound(t *testing.T) {
	src := []byte(`package sample

func RealFunc() {}
`)
	ctx := context.Background()
	results, err := FindDefinition(ctx, src, Go, "NoSuchSymbol")
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty slice, got: %v", results)
	}
}

// TestTokenReduction verifies that the word count of the Summarize output for
// large_sample.go and large_sample.ts is ≤ 20% of the word count of the full
// file content, confirming meaningful token reduction.
func TestTokenReduction(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		filename string
		lang     Language
	}{
		{"large_sample.go", Go},
		{"large_sample.ts", TypeScript},
	}

	for _, tc := range cases {
		t.Run(tc.filename, func(t *testing.T) {
			src := readTestdata(t, tc.filename)
			fullWords := wordCount(string(src))
			if fullWords == 0 {
				t.Fatalf("%s: file is empty", tc.filename)
			}

			summary, err := Summarize(ctx, src, tc.lang)
			if err != nil {
				t.Fatalf("Summarize(%q): %v", tc.filename, err)
			}

			summaryText := formatMarkdown(summary)
			summaryWords := wordCount(summaryText)

			ratio := float64(summaryWords) / float64(fullWords)
			const maxRatio = 0.20

			if ratio > maxRatio {
				t.Errorf(
					"%s: summary word count is %.1f%% of full file (want ≤ %.0f%%); "+
						"full=%d words, summary=%d words, ratio=%.4f",
					tc.filename,
					ratio*100,
					maxRatio*100,
					fullWords,
					summaryWords,
					ratio,
				)
			} else {
				t.Logf(
					"%s: token reduction OK — full=%d words, summary=%d words, ratio=%.4f (%.1f%%)",
					tc.filename,
					fullWords,
					summaryWords,
					ratio,
					ratio*100,
				)
			}
		})
	}
}

// TestLangFromExtension is a table-driven test for all supported and unsupported
// file extensions.
func TestLangFromExtension(t *testing.T) {
	cases := []struct {
		ext      string
		wantLang Language
		wantOK   bool
	}{
		{".go", Go, true},
		{".ts", TypeScript, true},
		{".tsx", TypeScript, true},
		{".py", Python, true},
		{".sh", Bash, true},
		{".bash", Bash, true},
		// unsupported extensions
		{".rb", "", false},
		{".js", "", false},
		{".java", "", false},
		{".cpp", "", false},
		{".rs", "", false},
		{"", "", false},
		{".go.bak", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.ext, func(t *testing.T) {
			got, ok := LangFromExtension(tc.ext)
			if ok != tc.wantOK {
				t.Errorf("LangFromExtension(%q): ok=%v, want %v", tc.ext, ok, tc.wantOK)
			}
			if got != tc.wantLang {
				t.Errorf("LangFromExtension(%q): lang=%q, want %q", tc.ext, got, tc.wantLang)
			}
		})
	}
}

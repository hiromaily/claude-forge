// Package ast provides AST parsing and code summarization.
// When built with CGO enabled, tree-sitter is used for full language support.
// When built with CGO disabled (e.g. static release builds), all functions
// return errCGORequired.
package ast

// Language represents a supported source language for AST parsing.
type Language string

const (
	Go         Language = "go"
	TypeScript Language = "typescript"
	Python     Language = "python"
	Bash       Language = "bash"
)

// Summary holds the extracted symbols from an AST parse.
type Summary struct {
	Functions []string
	Types     []string
	Constants []string
}

// LangFromExtension maps a file extension (including dot) to a Language.
// Returns false if the extension is not recognized.
func LangFromExtension(ext string) (Language, bool) {
	switch ext {
	case ".go":
		return Go, true
	case ".ts", ".tsx":
		return TypeScript, true
	case ".py":
		return Python, true
	case ".sh", ".bash":
		return Bash, true
	default:
		return "", false
	}
}

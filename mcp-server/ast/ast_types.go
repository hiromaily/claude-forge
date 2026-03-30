// Package ast provides tree-sitter-based AST parsing and code summarization.
package ast

import "unicode"

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

type unsupportedLangError struct {
	lang string
}

func (e *unsupportedLangError) Error() string {
	return "unsupported language: " + e.lang + "; supported: go, typescript, python, bash"
}

// isExported reports whether a symbol name is exported (public) in the given language.
// For Go, exported means starting with an uppercase letter.
// For other languages, all top-level declarations are considered exported.
func isExported(name string, lang Language) bool {
	if name == "" {
		return false
	}
	if lang == Go {
		return unicode.IsUpper(rune(name[0]))
	}
	return true
}

// makeSet creates a string-set from a slice.
func makeSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[item] = true
	}
	return s
}

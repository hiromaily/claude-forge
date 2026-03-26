// Package ast provides tree-sitter-based AST parsing and code summarization.
package ast

import (
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/bash"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

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

// languageGrammar returns the tree-sitter grammar for the given Language.
func languageGrammar(lang Language) (*sitter.Language, error) {
	switch lang {
	case Go:
		return golang.GetLanguage(), nil
	case TypeScript:
		return typescript.GetLanguage(), nil
	case Python:
		return python.GetLanguage(), nil
	case Bash:
		return bash.GetLanguage(), nil
	default:
		return nil, &unsupportedLangError{lang: string(lang)}
	}
}

type unsupportedLangError struct {
	lang string
}

func (e *unsupportedLangError) Error() string {
	return "unsupported language: " + e.lang + "; supported: go, typescript, python, bash"
}

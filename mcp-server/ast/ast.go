//go:build cgo

package ast

import (
	"context"
	"io"
	"strings"
	"unicode"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/bash"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

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

// functionNodeTypes returns the node type names that represent function declarations
// for the given language.
func functionNodeTypes(lang Language) []string {
	switch lang {
	case Go:
		return []string{"function_declaration", "method_declaration"}
	case TypeScript:
		return []string{"function_declaration", "function_signature", "method_definition", "method_signature"}
	case Python:
		return []string{"function_definition", "decorated_definition"}
	case Bash:
		return []string{"function_definition"}
	default:
		return nil
	}
}

// typeNodeTypes returns the node type names that represent type declarations
// for the given language.
func typeNodeTypes(lang Language) []string {
	switch lang {
	case Go:
		return []string{"type_spec"}
	case TypeScript:
		return []string{"interface_declaration", "type_alias_declaration"}
	case Python:
		return []string{"class_definition"}
	default:
		return nil
	}
}

// constantNodeTypes returns the node type names that represent constant declarations
// for the given language.
func constantNodeTypes(lang Language) []string {
	switch lang {
	case Go:
		return []string{"const_declaration"}
	default:
		return nil
	}
}

// nodeNameForLang extracts the name of a declaration node for the given language.
// Returns empty string if name cannot be determined.
func nodeNameForLang(node *sitter.Node, src []byte, lang Language) string {
	// Try "name" field first
	nameNode := node.ChildByFieldName("name")
	if nameNode != nil {
		return string(src[nameNode.StartByte():nameNode.EndByte()])
	}
	// For Python decorated_definition, look inside for the function_definition
	if lang == Python && node.Type() == "decorated_definition" {
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child.Type() == "function_definition" || child.Type() == "class_definition" {
				inner := child.ChildByFieldName("name")
				if inner != nil {
					return string(src[inner.StartByte():inner.EndByte()])
				}
			}
		}
	}
	return ""
}

// extractSignature extracts a function/type/constant signature (without body) from src.
// For declarations with bodies (blocks), it returns only up to the opening brace.
//
//nolint:gocyclo // complexity is inherent in the multi-language dispatch table (one case per node type)
func extractSignature(src []byte, node *sitter.Node, lang Language) string {
	nodeType := node.Type()
	fullText := string(src[node.StartByte():node.EndByte()])

	// For Go function/method declarations and Python function/class, strip the body.
	switch lang {
	case Go:
		if nodeType == "function_declaration" || nodeType == "method_declaration" {
			// Find the block child (body) and return text before it.
			bodyNode := node.ChildByFieldName("body")
			if bodyNode != nil {
				sig := strings.TrimRight(string(src[node.StartByte():bodyNode.StartByte()]), " \t")
				return sig
			}
		}
		if nodeType == "type_spec" {
			// Return the full type_spec text (it's compact: just "Name Type")
			return fullText
		}
		if nodeType == "const_declaration" {
			// For const, take up to first newline after the declaration or just single-line
			lines := strings.Split(fullText, "\n")
			if len(lines) > 0 {
				return strings.TrimSpace(lines[0])
			}
			return strings.TrimSpace(fullText)
		}
	case TypeScript:
		if nodeType == "function_declaration" || nodeType == "method_definition" {
			bodyNode := node.ChildByFieldName("body")
			if bodyNode != nil {
				sig := strings.TrimRight(string(src[node.StartByte():bodyNode.StartByte()]), " \t")
				return sig
			}
		}
		// function_signature, method_signature — no body
	case Python:
		if nodeType == "function_definition" {
			bodyNode := node.ChildByFieldName("body")
			if bodyNode != nil {
				sig := strings.TrimRight(string(src[node.StartByte():bodyNode.StartByte()]), " \t")
				return strings.TrimRight(sig, ":")
			}
		}
		if nodeType == "class_definition" {
			bodyNode := node.ChildByFieldName("body")
			if bodyNode != nil {
				sig := strings.TrimRight(string(src[node.StartByte():bodyNode.StartByte()]), " \t")
				return strings.TrimRight(sig, ":")
			}
		}
		if nodeType == "decorated_definition" {
			// Return first line (decorator + next line with def/class)
			lines := strings.Split(fullText, "\n")
			var sigLines []string
			for _, line := range lines {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "@") {
					sigLines = append(sigLines, line)
				} else if strings.HasPrefix(trimmed, "def ") || strings.HasPrefix(trimmed, "class ") || strings.HasPrefix(trimmed, "async ") {
					sigLines = append(sigLines, strings.TrimRight(strings.TrimRight(line, ":"), " \t"))
					break
				}
			}
			if len(sigLines) > 0 {
				return strings.Join(sigLines, "\n")
			}
		}
	case Bash:
		if nodeType == "function_definition" {
			bodyNode := node.ChildByFieldName("body")
			if bodyNode != nil {
				sig := strings.TrimRight(string(src[node.StartByte():bodyNode.StartByte()]), " \t")
				return sig
			}
			// fallback: just first line
			lines := strings.Split(fullText, "\n")
			return strings.TrimSpace(lines[0])
		}
	}

	return strings.TrimSpace(fullText)
}

// Summarize parses src using the given language grammar and returns
// a Summary of exported functions, types, and constants.
func Summarize(ctx context.Context, src []byte, lang Language) (Summary, error) {
	grammar, err := languageGrammar(lang)
	if err != nil {
		return Summary{}, err
	}

	root, err := sitter.ParseCtx(ctx, src, grammar)
	if err != nil {
		return Summary{}, err
	}

	fnTypes := makeSet(functionNodeTypes(lang))
	typeTypes := makeSet(typeNodeTypes(lang))
	constTypes := makeSet(constantNodeTypes(lang))

	var summary Summary

	iter := sitter.NewNamedIterator(root, sitter.DFSMode)
	err = iter.ForEach(func(node *sitter.Node) error {
		nodeType := node.Type()

		if fnTypes[nodeType] || typeTypes[nodeType] {
			name := nodeNameForLang(node, src, lang)
			exported := isExported(name, lang)
			if lang == TypeScript {
				// TypeScript uses explicit `export` keyword. A declaration is exported
				// only when its immediate parent is an export_statement node.
				exported = node.Parent() != nil && node.Parent().Type() == "export_statement"
			}
			if exported {
				sig := extractSignature(src, node, lang)
				if sig != "" {
					if fnTypes[nodeType] {
						summary.Functions = append(summary.Functions, sig)
					} else {
						summary.Types = append(summary.Types, sig)
					}
				}
			}
		} else if constTypes[nodeType] {
			// For Go const declarations, check if any spec is exported.
			sig := extractGoExportedConst(src, node)
			if sig != "" {
				summary.Constants = append(summary.Constants, sig)
			}
		}

		return nil
	})
	if err != nil && err != io.EOF {
		return Summary{}, err
	}

	return summary, nil
}

// extractGoExportedConst returns the first line of a const declaration if it contains
// at least one exported name. Returns "" if none are exported.
func extractGoExportedConst(src []byte, node *sitter.Node) string {
	// Walk children of const_declaration to find const_spec nodes
	hasExported := false
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "const_spec" {
			nameNode := child.ChildByFieldName("name")
			if nameNode != nil {
				name := string(src[nameNode.StartByte():nameNode.EndByte()])
				if isExported(name, Go) {
					hasExported = true
					break
				}
			}
		}
	}
	if !hasExported {
		return ""
	}
	// Return first line of the const_declaration
	fullText := string(src[node.StartByte():node.EndByte()])
	lines := strings.Split(fullText, "\n")
	return strings.TrimSpace(lines[0])
}

// FindDefinition searches src for declarations of symbol and returns their
// text representations. Returns an empty slice (not an error) if not found.
func FindDefinition(ctx context.Context, src []byte, lang Language, symbol string) ([]string, error) {
	grammar, err := languageGrammar(lang)
	if err != nil {
		return nil, err
	}

	root, err := sitter.ParseCtx(ctx, src, grammar)
	if err != nil {
		return nil, err
	}

	// Collect all declaration node types for this language.
	allDeclTypes := makeSet(append(append(functionNodeTypes(lang), typeNodeTypes(lang)...), constantNodeTypes(lang)...))

	var results []string

	iter := sitter.NewNamedIterator(root, sitter.DFSMode)
	err = iter.ForEach(func(node *sitter.Node) error {
		if !allDeclTypes[node.Type()] {
			return nil
		}

		name := nodeNameForLang(node, src, lang)
		if name != symbol {
			return nil
		}

		text := string(src[node.StartByte():node.EndByte()])
		results = append(results, strings.TrimSpace(text))
		return nil
	})
	if err != nil && err != io.EOF {
		return nil, err
	}

	return results, nil
}

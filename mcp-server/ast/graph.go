//go:build cgo

package ast

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"golang.org/x/mod/modfile"
)

// langExtension returns the canonical file extension (with dot) for a Language.
func langExtension(lang Language) string {
	switch lang {
	case Go:
		return ".go"
	case TypeScript:
		return ".ts"
	case Python:
		return ".py"
	case Bash:
		return ".sh"
	default:
		return ""
	}
}

// readGoModuleName reads the module name from a go.mod file in rootPath.
// Returns an empty string if go.mod is absent or unreadable.
func readGoModuleName(rootPath string) string {
	data, err := os.ReadFile(filepath.Join(rootPath, "go.mod"))
	if err != nil {
		return ""
	}
	f, err := modfile.ParseLax("go.mod", data, nil)
	if err != nil || f.Module == nil {
		return ""
	}
	return f.Module.Mod.Path
}

// deduplicateEdges removes duplicate Edge entries (same From+To).
func deduplicateEdges(edges []Edge) []Edge {
	seen := make(map[string]bool, len(edges))
	result := make([]Edge, 0, len(edges))
	for _, e := range edges {
		key := e.From + "\x00" + e.To
		if !seen[key] {
			seen[key] = true
			result = append(result, e)
		}
	}
	return result
}

// ExtractCallSites parses src using tree-sitter and returns all call sites found
// in the source for the given language. filePath is stored in each CallSite and
// used for error context.
//
//nolint:gocyclo // complexity is inherent in the multi-language dispatch table
func ExtractCallSites(ctx context.Context, src []byte, lang Language, filePath string) ([]CallSite, error) {
	grammar, err := languageGrammar(lang)
	if err != nil {
		return nil, err
	}

	root, err := sitter.ParseCtx(ctx, src, grammar)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filePath, err)
	}

	var callSites []CallSite

	iter := sitter.NewNamedIterator(root, sitter.DFSMode)
	iterErr := iter.ForEach(func(node *sitter.Node) error {
		switch lang {
		case Go:
			// Go uses `call_expression` nodes.
			// For qualified calls like pkg.Symbol(), the function child is a
			// selector_expression whose field child is the unqualified symbol name.
			if node.Type() == "call_expression" {
				funcNode := node.ChildByFieldName("function")
				if funcNode == nil {
					break
				}
				var symbol string
				if funcNode.Type() == "selector_expression" {
					// Qualified call: pkg.Symbol()
					fieldNode := funcNode.ChildByFieldName("field")
					if fieldNode != nil {
						symbol = string(src[fieldNode.StartByte():fieldNode.EndByte()])
					}
				} else if funcNode.Type() == "identifier" {
					// Unqualified call: Symbol()
					symbol = string(src[funcNode.StartByte():funcNode.EndByte()])
				}
				if symbol != "" {
					callSites = append(callSites, CallSite{
						File:   filePath,
						Symbol: symbol,
						Line:   int(node.StartPoint().Row) + 1, // tree-sitter rows are 0-indexed
					})
				}
			}
		case TypeScript:
			// TypeScript uses `call_expression` nodes; unqualified function name matched.
			if node.Type() == "call_expression" {
				funcNode := node.ChildByFieldName("function")
				if funcNode == nil {
					break
				}
				var symbol string
				if funcNode.Type() == "identifier" {
					symbol = string(src[funcNode.StartByte():funcNode.EndByte()])
				} else if funcNode.Type() == "member_expression" {
					// Qualified call: obj.method() — use property name
					propNode := funcNode.ChildByFieldName("property")
					if propNode != nil {
						symbol = string(src[propNode.StartByte():propNode.EndByte()])
					}
				}
				if symbol != "" {
					callSites = append(callSites, CallSite{
						File:   filePath,
						Symbol: symbol,
						Line:   int(node.StartPoint().Row) + 1,
					})
				}
			}
		case Python:
			// Python uses `call` nodes.
			if node.Type() == "call" {
				funcNode := node.ChildByFieldName("function")
				if funcNode == nil {
					break
				}
				var symbol string
				if funcNode.Type() == "identifier" {
					symbol = string(src[funcNode.StartByte():funcNode.EndByte()])
				} else if funcNode.Type() == "attribute" {
					// Qualified call: obj.method() — use attribute name
					attrNode := funcNode.ChildByFieldName("attribute")
					if attrNode != nil {
						symbol = string(src[attrNode.StartByte():attrNode.EndByte()])
					}
				}
				if symbol != "" {
					callSites = append(callSites, CallSite{
						File:   filePath,
						Symbol: symbol,
						Line:   int(node.StartPoint().Row) + 1,
					})
				}
			}
		case Bash:
			// Bash uses `command_name` nodes.
			if node.Type() == "command_name" {
				symbol := string(src[node.StartByte():node.EndByte()])
				if symbol != "" {
					callSites = append(callSites, CallSite{
						File:   filePath,
						Symbol: symbol,
						Line:   int(node.StartPoint().Row) + 1,
					})
				}
			}
		}
		return nil
	})

	if iterErr != nil && iterErr != io.EOF {
		return nil, fmt.Errorf("iterate %s: %w", filePath, iterErr)
	}

	return callSites, nil
}

// ExtractImports parses src using tree-sitter and returns the list of imported
// package/module paths for the given language. filePath is used for error context.
//
//nolint:gocyclo // complexity is inherent in the multi-language dispatch table
func ExtractImports(ctx context.Context, src []byte, lang Language, filePath string) ([]string, error) {
	grammar, err := languageGrammar(lang)
	if err != nil {
		return nil, err
	}

	root, err := sitter.ParseCtx(ctx, src, grammar)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filePath, err)
	}

	var imports []string

	iter := sitter.NewNamedIterator(root, sitter.DFSMode)
	iterErr := iter.ForEach(func(node *sitter.Node) error {
		switch lang {
		case Go:
			if node.Type() == "import_spec" {
				// The path child is a interpreted_string_literal: `"pkg/path"`
				pathNode := node.ChildByFieldName("path")
				if pathNode != nil {
					raw := string(src[pathNode.StartByte():pathNode.EndByte()])
					// Strip surrounding quotes
					if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
						imports = append(imports, raw[1:len(raw)-1])
					}
				}
			}
		case TypeScript:
			if node.Type() == "import_statement" {
				// Look for the string child (module specifier)
				for i := 0; i < int(node.ChildCount()); i++ {
					child := node.Child(i)
					if child.Type() == "string" {
						raw := string(src[child.StartByte():child.EndByte()])
						if len(raw) >= 2 {
							imports = append(imports, raw[1:len(raw)-1])
						}
						break
					}
				}
			}
		case Python:
			if node.Type() == "import_statement" {
				// import foo, bar
				for i := 0; i < int(node.NamedChildCount()); i++ {
					child := node.NamedChild(i)
					if child.Type() == "dotted_name" || child.Type() == "aliased_import" {
						nameNode := child
						if child.Type() == "aliased_import" {
							nameNode = child.NamedChild(0)
						}
						if nameNode != nil {
							imports = append(imports, string(src[nameNode.StartByte():nameNode.EndByte()]))
						}
					}
				}
			} else if node.Type() == "import_from_statement" {
				// from foo import bar — record the module name
				moduleNode := node.ChildByFieldName("module_name")
				if moduleNode != nil {
					imports = append(imports, string(src[moduleNode.StartByte():moduleNode.EndByte()]))
				}
			}
		case Bash:
			if node.Type() == "command" {
				// Detect `source file` or `. file` patterns
				nameNode := node.ChildByFieldName("name")
				if nameNode == nil {
					break
				}
				cmdName := string(src[nameNode.StartByte():nameNode.EndByte()])
				if cmdName == "source" || cmdName == "." {
					// First argument is the sourced file.
					argNode := node.ChildByFieldName("argument")
					if argNode == nil {
						// Fallback: try named children after the command name.
						for i := 0; i < int(node.NamedChildCount()); i++ {
							child := node.NamedChild(i)
							if child != nameNode {
								argNode = child
								break
							}
						}
					}
					if argNode != nil {
						raw := string(src[argNode.StartByte():argNode.EndByte()])
						// Strip surrounding quotes if present (supports 'path' and "path").
						if len(raw) >= 2 && (raw[0] == '"' || raw[0] == '\'') && raw[0] == raw[len(raw)-1] {
							raw = raw[1 : len(raw)-1]
						}
						imports = append(imports, raw)
					}
				}
			}
		}
		return nil
	})

	if iterErr != nil && iterErr != io.EOF {
		return nil, fmt.Errorf("iterate %s: %w", filePath, iterErr)
	}

	return imports, nil
}

// BuildDependencyGraph walks rootPath recursively, collecting files for the
// given language, and returns a file-level import dependency graph.
// Nodes are relative paths from rootPath. For Go, module-prefix stripping is
// applied to resolve import paths to relative file paths.
func BuildDependencyGraph(ctx context.Context, rootPath string, lang Language) (DependencyGraph, error) { //nolint:gocyclo // complexity is inherent in the multi-language dispatch
	ext := langExtension(lang)
	if ext == "" {
		return DependencyGraph{}, &unsupportedLangError{lang: string(lang)}
	}

	// Walk rootPath and collect all matching source files.
	var files []string
	err := filepath.WalkDir(rootPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.EqualFold(filepath.Ext(path), ext) {
			rel, relErr := filepath.Rel(rootPath, path)
			if relErr != nil {
				return relErr
			}
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		return DependencyGraph{}, fmt.Errorf("walk %s: %w", rootPath, err)
	}

	// For Go, read the module name for import-path stripping.
	var moduleName string
	if lang == Go {
		moduleName = readGoModuleName(rootPath)
	}

	// Build a reverse-lookup: import path → relative file path.
	// For Go: strip module prefix from each file's package path.
	// For other languages: use the source path (import value) directly.
	//
	// We build this by mapping each relative file to its importable path.
	// For Go files: importable path = moduleName + "/" + dir(relFile)
	// We also build a set of all nodes for quick node existence checks.
	fileSet := make(map[string]bool, len(files))
	for _, f := range files {
		fileSet[f] = true
	}

	// importPathToFile maps a canonical import path → relative file path.
	// For Go: "example.com/pkg" → "pkg/a.go" (any .go file in that dir).
	// We build this per-directory for Go.
	importPathToFile := make(map[string]string)
	if lang == Go && moduleName != "" {
		// Group files by their directory to build the mapping.
		for _, f := range files {
			dir := filepath.Dir(f)
			var importPath string
			if dir == "." {
				importPath = moduleName
			} else {
				importPath = moduleName + "/" + filepath.ToSlash(dir)
			}
			// Map importPath → first file in that directory (deterministic: files are sorted by WalkDir).
			if _, exists := importPathToFile[importPath]; !exists {
				importPathToFile[importPath] = f
			}
		}
	}

	// Parse imports for each file and build edges.
	nodeSet := make(map[string]bool)
	var edges []Edge

	for _, relFile := range files {
		absPath := filepath.Join(rootPath, relFile)
		src, readErr := os.ReadFile(absPath)
		if readErr != nil {
			return DependencyGraph{}, fmt.Errorf("read %s: %w", relFile, readErr)
		}

		imports, importErr := ExtractImports(ctx, src, lang, relFile)
		if importErr != nil {
			return DependencyGraph{}, fmt.Errorf("extract imports %s: %w", relFile, importErr)
		}

		nodeSet[relFile] = true

		for _, imp := range imports {
			var toFile string
			switch {
			case lang == Go && moduleName != "":
				// Resolve module-qualified import path to relative file path.
				if f, ok := importPathToFile[imp]; ok {
					toFile = f
				}
			case lang == Bash:
				// Bash: import is a sourced file path, relative to the script's directory.
				// Resolve relative to rootPath.
				scriptDir := filepath.Dir(filepath.Join(rootPath, relFile))
				candidate := filepath.Clean(filepath.Join(scriptDir, imp))
				rel, relErr := filepath.Rel(rootPath, candidate)
				if relErr == nil && fileSet[rel] {
					toFile = rel
				}
			default:
				// TypeScript/Python: use import path directly if it matches a known file.
				if fileSet[imp] {
					toFile = imp
				}
			}

			if toFile != "" && toFile != relFile {
				nodeSet[toFile] = true
				edges = append(edges, Edge{From: relFile, To: toFile})
			}
		}
	}

	// Build sorted node list.
	nodes := make([]string, 0, len(nodeSet))
	for n := range nodeSet {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)

	// Deduplicate edges (same From+To may appear multiple times if a file imports the same pkg via multiple files).
	edges = deduplicateEdges(edges)

	return DependencyGraph{
		Root:  rootPath,
		Lang:  string(lang),
		Nodes: nodes,
		Edges: edges,
	}, nil
}

// FindCallers identifies all files in rootPath that call symbolName defined (or
// declared) in targetFile.
//
// For Go and Bash, a two-pass strategy is used:
//  1. BuildDependencyGraph to obtain the import graph.
//  2. ReverseDependencies to find files that (transitively) import targetFile —
//     these are the import-filter candidates.
//  3. For each candidate, ExtractCallSites is used to confirm that symbolName is
//     actually called; only confirmed callers are returned.
//
// For TypeScript and Python, the import filter is skipped and all source files are
// scanned with ExtractCallSites. Confirmed callers receive Distance == -1 (sentinel:
// confirmed caller, BFS not available).
//
// Results are sorted: BFS-ranked entries (Go/Bash) by distance then alphabetically;
// TS/Python entries (distance=-1) appended after.
// Returns an empty (non-nil) slice when no callers are found.
func FindCallers(ctx context.Context, rootPath string, lang Language, targetFile string, symbolName string) ([]ImpactEntry, error) {
	ext := langExtension(lang)
	if ext == "" {
		return nil, &unsupportedLangError{lang: string(lang)}
	}

	switch lang {
	case Go, Bash:
		return findCallersTwoPass(ctx, rootPath, lang, targetFile, symbolName)
	case TypeScript, Python:
		return findCallersSinglePass(ctx, rootPath, lang, ext, targetFile, symbolName)
	default:
		return nil, &unsupportedLangError{lang: string(lang)}
	}
}

// findCallersTwoPass implements the two-pass strategy for Go and Bash:
// import filter via BFS, then call-site confirmation.
func findCallersTwoPass(ctx context.Context, rootPath string, lang Language, targetFile string, symbolName string) ([]ImpactEntry, error) {
	graph, err := BuildDependencyGraph(ctx, rootPath, lang)
	if err != nil {
		return nil, fmt.Errorf("build dependency graph: %w", err)
	}

	// Get all files that transitively import targetFile, with BFS distance.
	candidates := ReverseDependencies(graph, targetFile)
	if len(candidates) == 0 {
		return []ImpactEntry{}, nil
	}

	// Build a distance lookup for candidates.
	distanceOf := make(map[string]int, len(candidates))
	for _, c := range candidates {
		distanceOf[c.File] = c.Distance
	}

	// Pass 2: for each candidate, check if symbolName is actually called.
	var confirmed []ImpactEntry
	for _, candidate := range candidates {
		absPath := filepath.Join(rootPath, candidate.File)
		src, readErr := os.ReadFile(absPath)
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", candidate.File, readErr)
		}

		callSites, csErr := ExtractCallSites(ctx, src, lang, candidate.File)
		if csErr != nil {
			return nil, fmt.Errorf("extract call sites %s: %w", candidate.File, csErr)
		}

		for _, cs := range callSites {
			if cs.Symbol == symbolName {
				confirmed = append(confirmed, ImpactEntry{
					File:     candidate.File,
					Distance: distanceOf[candidate.File],
				})
				break // one match is enough to confirm this file
			}
		}
	}

	// Sort by distance ascending, then alphabetically within the same distance.
	sort.Slice(confirmed, func(i, j int) bool {
		if confirmed[i].Distance != confirmed[j].Distance {
			return confirmed[i].Distance < confirmed[j].Distance
		}
		return confirmed[i].File < confirmed[j].File
	})

	if len(confirmed) == 0 {
		return []ImpactEntry{}, nil
	}
	return confirmed, nil
}

// findCallersSinglePass implements the single-pass strategy for TypeScript and Python.
// All source files are scanned; confirmed callers receive Distance == -1.
func findCallersSinglePass(ctx context.Context, rootPath string, lang Language, ext string, targetFile string, symbolName string) ([]ImpactEntry, error) {
	var results []ImpactEntry

	err := filepath.WalkDir(rootPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ext) {
			return nil
		}

		rel, relErr := filepath.Rel(rootPath, path)
		if relErr != nil {
			return relErr
		}

		// Skip the target file itself.
		if rel == targetFile {
			return nil
		}

		src, readErr := os.ReadFile(path) //nolint:gosec // path is constructed from WalkDir, not user input
		if readErr != nil {
			return fmt.Errorf("read %s: %w", rel, readErr)
		}

		callSites, csErr := ExtractCallSites(ctx, src, lang, rel)
		if csErr != nil {
			return fmt.Errorf("extract call sites %s: %w", rel, csErr)
		}

		for _, cs := range callSites {
			if cs.Symbol == symbolName {
				results = append(results, ImpactEntry{
					File:     rel,
					Distance: -1,
				})
				break // one match per file
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", rootPath, err)
	}

	// Sort alphabetically (distance is always -1 for this path).
	sort.Slice(results, func(i, j int) bool {
		return results[i].File < results[j].File
	})

	if len(results) == 0 {
		return []ImpactEntry{}, nil
	}
	return results, nil
}

// Package ast provides source code analysis using tree-sitter parsers.
//
// Key capabilities:
//   - [Summarize]: parse a source file and return a compact markdown summary
//     of exported signatures (functions, types, methods).
//   - [FindDefinition]: locate and return the full definition of a named
//     symbol in a source file.
//   - [BuildDependencyGraph]: walk a source tree and return a file-level
//     import graph as JSON (nodes = files, edges = imports).
//   - [FindCallSites]: identify files that call a given symbol via a
//     two-pass import + call-site scan.
//
// Supports Go, TypeScript, and Python via tree-sitter grammars.
//
// Import direction: ast has no internal dependencies (leaf package).
package ast

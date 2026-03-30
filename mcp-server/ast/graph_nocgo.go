//go:build !cgo

package ast

import "context"

// ExtractCallSites is a stub used when CGO is disabled.
// It returns an error indicating that AST features are unavailable.
func ExtractCallSites(_ context.Context, _ []byte, _ Language, _ string) ([]CallSite, error) {
	return nil, errCGORequired
}

// ExtractImports is a stub used when CGO is disabled.
// It returns an error indicating that AST features are unavailable.
func ExtractImports(_ context.Context, _ []byte, _ Language, _ string) ([]string, error) {
	return nil, errCGORequired
}

// BuildDependencyGraph is a stub used when CGO is disabled.
// It returns an error indicating that AST features are unavailable.
func BuildDependencyGraph(_ context.Context, _ string, _ Language) (DependencyGraph, error) {
	return DependencyGraph{}, errCGORequired
}

// FindCallers is a stub used when CGO is disabled.
// It returns an error indicating that AST features are unavailable.
func FindCallers(_ context.Context, _ string, _ Language, _ string, _ string) ([]ImpactEntry, error) {
	return nil, errCGORequired
}

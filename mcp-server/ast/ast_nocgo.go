//go:build !cgo

package ast

import (
	"context"
	"errors"
)

// errCGORequired is returned by AST functions when CGO is not available.
var errCGORequired = errors.New("AST features require CGO; rebuild with CGO_ENABLED=1")

// Summarize is a stub used when CGO is disabled.
// It returns an error indicating that AST features are unavailable.
func Summarize(_ context.Context, _ []byte, _ Language) (Summary, error) {
	return Summary{}, errCGORequired
}

// FindDefinition is a stub used when CGO is disabled.
// It returns an error indicating that AST features are unavailable.
func FindDefinition(_ context.Context, _ []byte, _ Language, _ string) ([]string, error) {
	return nil, errCGORequired
}

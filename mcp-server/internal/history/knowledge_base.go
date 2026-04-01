package history

import (
	"errors"
	"fmt"
)

// KnowledgeBase bundles the PatternAccumulator and FrictionMap under a single
// wrapper for convenient injection and startup loading.
type KnowledgeBase struct {
	Patterns *PatternAccumulator
	Friction *FrictionMap
}

// NewKnowledgeBase creates a KnowledgeBase backed by the given specsDir.
// Both accumulators are initialised in empty state; call Load to restore
// persisted data from disk.
func NewKnowledgeBase(specsDir string) *KnowledgeBase {
	return &KnowledgeBase{
		Patterns: NewPatternAccumulator(specsDir),
		Friction: NewFrictionMap(specsDir),
	}
}

// Load reads patterns.json and friction.json from specsDir into memory.
// If both files are absent the method returns nil (fail-open).
// If one or both files are present but unreadable or corrupted the method
// returns a combined error while keeping both accumulators in a usable
// (possibly empty) state — the caller should log but not abort.
func (kb *KnowledgeBase) Load() error {
	var errs []error

	if err := kb.Patterns.Load(); err != nil {
		errs = append(errs, fmt.Errorf("patterns: %w", err))
	}

	if err := kb.Friction.Load(); err != nil {
		errs = append(errs, fmt.Errorf("friction: %w", err))
	}

	return errors.Join(errs...)
}

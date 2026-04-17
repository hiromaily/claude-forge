package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const preferencesFileName = "preferences.json"

// Preferences holds per-repository default flag settings.
// All fields are pointers to distinguish "not set" from "set to zero value".
type Preferences struct {
	Auto    *bool   `json:"auto,omitempty"`
	Debug   *bool   `json:"debug,omitempty"`
	Effort  *string `json:"effort,omitempty"`
	NoPR    *bool   `json:"nopr,omitempty"`
	Discuss *bool   `json:"discuss,omitempty"`
}

// LoadPreferences reads preferences.json from specsDir.
// Returns zero Preferences{} if the file does not exist (not an error).
func LoadPreferences(specsDir string) (Preferences, error) {
	var p Preferences
	data, err := os.ReadFile(filepath.Join(specsDir, preferencesFileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return p, nil
		}
		return p, fmt.Errorf("LoadPreferences: %w", err)
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return Preferences{}, fmt.Errorf("LoadPreferences: %w", err)
	}
	return p, nil
}

// SavePreferences writes preferences.json to specsDir atomically.
// Creates specsDir if it does not exist.
func SavePreferences(specsDir string, p Preferences) error {
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		return fmt.Errorf("SavePreferences: mkdir: %w", err)
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("SavePreferences: marshal: %w", err)
	}
	data = append(data, '\n')
	target := filepath.Join(specsDir, preferencesFileName)
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("SavePreferences: write tmp: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		return fmt.Errorf("SavePreferences: rename: %w", err)
	}
	return nil
}

// Validate checks that all set fields contain valid values.
func (p Preferences) Validate() error {
	if p.Effort != nil && !slices.Contains(ValidEfforts, *p.Effort) {
		return fmt.Errorf("invalid effort %q (expected: %s)", *p.Effort, strings.Join(ValidEfforts, ", "))
	}
	return nil
}

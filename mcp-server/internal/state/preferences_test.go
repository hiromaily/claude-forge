package state_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
)

func boolPtr(b bool) *bool { return new(b) }

func strPtr(s string) *string { return new(s) }

func TestLoadPreferences_FileNotExists(t *testing.T) {
	t.Parallel()
	p, err := state.LoadPreferences(t.TempDir())
	if err != nil {
		t.Fatalf("LoadPreferences error: %v", err)
	}
	if p.Auto != nil || p.Debug != nil || p.Effort != nil || p.NoPR != nil || p.Discuss != nil {
		t.Errorf("expected zero Preferences, got %+v", p)
	}
}

func TestLoadPreferences_ValidFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	data := `{"auto":true,"debug":false,"effort":"M","nopr":true,"discuss":true}`
	if err := os.WriteFile(filepath.Join(dir, "preferences.json"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := state.LoadPreferences(dir)
	if err != nil {
		t.Fatalf("LoadPreferences: %v", err)
	}
	if p.Auto == nil || !*p.Auto {
		t.Errorf("Auto = %v, want true", p.Auto)
	}
	if p.Debug == nil || *p.Debug {
		t.Errorf("Debug = %v, want false", p.Debug)
	}
	if p.Effort == nil || *p.Effort != "M" {
		t.Errorf("Effort = %v, want M", p.Effort)
	}
	if p.NoPR == nil || !*p.NoPR {
		t.Errorf("NoPR = %v, want true", p.NoPR)
	}
	if p.Discuss == nil || !*p.Discuss {
		t.Errorf("Discuss = %v, want true", p.Discuss)
	}
}

func TestLoadPreferences_PartialFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	data := `{"auto":true}`
	if err := os.WriteFile(filepath.Join(dir, "preferences.json"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := state.LoadPreferences(dir)
	if err != nil {
		t.Fatalf("LoadPreferences: %v", err)
	}
	if p.Auto == nil || !*p.Auto {
		t.Errorf("Auto = %v, want true", p.Auto)
	}
	if p.Debug != nil {
		t.Errorf("Debug should be nil, got %v", p.Debug)
	}
}

func TestSavePreferences_CreatesDir(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "nonexistent")
	p := state.Preferences{Auto: new(true)}
	if err := state.SavePreferences(dir, p); err != nil {
		t.Fatalf("SavePreferences: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "preferences.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var loaded state.Preferences
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if loaded.Auto == nil || !*loaded.Auto {
		t.Errorf("saved Auto = %v, want true", loaded.Auto)
	}
}

func TestSavePreferences_AtomicWrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := state.Preferences{Effort: new("L")}
	if err := state.SavePreferences(dir, p); err != nil {
		t.Fatalf("SavePreferences: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "preferences.json.tmp*"))
	if len(matches) > 0 {
		t.Errorf("temp files remain after save: %v", matches)
	}
}

func TestPreferences_Validate_ValidEffort(t *testing.T) {
	t.Parallel()
	for _, e := range []string{"S", "M", "L"} {
		p := state.Preferences{Effort: new(e)}
		if err := p.Validate(); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", e, err)
		}
	}
}

func TestPreferences_Validate_InvalidEffort(t *testing.T) {
	t.Parallel()
	p := state.Preferences{Effort: new("XS")}
	if err := p.Validate(); err == nil {
		t.Error("Validate(XS) = nil, want error")
	}
}

func TestPreferences_Validate_NilEffort(t *testing.T) {
	t.Parallel()
	p := state.Preferences{}
	if err := p.Validate(); err != nil {
		t.Errorf("Validate(nil effort) = %v, want nil", err)
	}
}

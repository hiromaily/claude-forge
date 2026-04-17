package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
)

func TestPreferencesGet_NoFile(t *testing.T) {
	t.Parallel()

	sm := state.NewStateManager("dev")
	sm.SetSpecsDir(t.TempDir())
	h := PreferencesGetHandler(sm)
	res := callTool(t, h, map[string]any{})
	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(res))
	}
	text := textContent(res)
	var got map[string]any
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatalf("unmarshal response: %v (text=%q)", err, text)
	}
	if len(got) != 0 {
		t.Errorf("expected empty object, got %v", got)
	}
}

func TestPreferencesGet_WithPreferences(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "preferences.json"),
		[]byte(`{"auto":true,"effort":"M"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	sm := state.NewStateManager("dev")
	sm.SetSpecsDir(dir)
	h := PreferencesGetHandler(sm)
	res := callTool(t, h, map[string]any{})
	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(res))
	}
	var got state.Preferences
	if err := json.Unmarshal([]byte(textContent(res)), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Auto == nil || !*got.Auto {
		t.Errorf("Auto = %v, want true", got.Auto)
	}
	if got.Effort == nil || *got.Effort != "M" {
		t.Errorf("Effort = %v, want M", got.Effort)
	}
}

func TestPreferencesSet_Valid(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sm := state.NewStateManager("dev")
	sm.SetSpecsDir(dir)
	h := PreferencesSetHandler(sm)
	res := callTool(t, h, map[string]any{
		"preferences": map[string]any{"auto": true, "effort": "S", "debug": false},
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(res))
	}
	p, err := state.LoadPreferences(dir)
	if err != nil {
		t.Fatalf("LoadPreferences: %v", err)
	}
	if p.Auto == nil || !*p.Auto {
		t.Errorf("Auto = %v, want true", p.Auto)
	}
	if p.Effort == nil || *p.Effort != "S" {
		t.Errorf("Effort = %v, want S", p.Effort)
	}
}

func TestPreferencesSet_InvalidEffort(t *testing.T) {
	t.Parallel()

	sm := state.NewStateManager("dev")
	sm.SetSpecsDir(t.TempDir())
	h := PreferencesSetHandler(sm)
	res := callTool(t, h, map[string]any{
		"preferences": map[string]any{"effort": "XS"},
	})
	if !res.IsError {
		t.Fatal("expected error for invalid effort, got success")
	}
}

func TestPreferencesSet_UnknownFields(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sm := state.NewStateManager("dev")
	sm.SetSpecsDir(dir)
	h := PreferencesSetHandler(sm)
	res := callTool(t, h, map[string]any{
		"preferences": map[string]any{"auto": true, "unknown_field": "value"},
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", textContent(res))
	}
	data, _ := os.ReadFile(filepath.Join(dir, "preferences.json"))
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal file: %v", err)
	}
	if _, found := raw["unknown_field"]; found {
		t.Error("unknown_field should have been stripped")
	}
}

func TestPreferencesSet_InvalidBoolType(t *testing.T) {
	t.Parallel()

	sm := state.NewStateManager("dev")
	sm.SetSpecsDir(t.TempDir())
	h := PreferencesSetHandler(sm)
	res := callTool(t, h, map[string]any{
		"preferences": map[string]any{"auto": "yes"},
	})
	if !res.IsError {
		t.Fatal("expected error for non-bool auto, got success")
	}
}

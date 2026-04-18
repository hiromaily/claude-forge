package maputil_test

import (
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/internal/maputil"
)

func TestStringField(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		m    map[string]any
		key  string
		want string
	}{
		{"present", map[string]any{"k": "v"}, "k", "v"},
		{"missing", map[string]any{}, "k", ""},
		{"nil_value", map[string]any{"k": nil}, "k", ""},
		{"non_string", map[string]any{"k": 42}, "k", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := maputil.StringField(tc.m, tc.key)
			if got != tc.want {
				t.Errorf("StringField(%v, %q) = %q, want %q", tc.m, tc.key, got, tc.want)
			}
		})
	}
}

func TestStringFieldAlt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		m       map[string]any
		primary string
		alt     string
		want    string
	}{
		{"primary_wins", map[string]any{"a": "1", "b": "2"}, "a", "b", "1"},
		{"fallback_alt", map[string]any{"b": "2"}, "a", "b", "2"},
		{"neither", map[string]any{}, "a", "b", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := maputil.StringFieldAlt(tc.m, tc.primary, tc.alt)
			if got != tc.want {
				t.Errorf("StringFieldAlt() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBoolField(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		m    map[string]any
		key  string
		want bool
	}{
		{"true", map[string]any{"k": true}, "k", true},
		{"false", map[string]any{"k": false}, "k", false},
		{"missing", map[string]any{}, "k", false},
		{"nil", map[string]any{"k": nil}, "k", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := maputil.BoolField(tc.m, tc.key)
			if got != tc.want {
				t.Errorf("BoolField() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIntFieldAlt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		m       map[string]any
		primary string
		alt     string
		want    int
	}{
		{"float64", map[string]any{"a": float64(3)}, "a", "b", 3},
		{"int", map[string]any{"a": 5}, "a", "b", 5},
		{"fallback_alt", map[string]any{"b": float64(8)}, "a", "b", 8},
		{"missing", map[string]any{}, "a", "b", 0},
		{"nil", map[string]any{"a": nil}, "a", "b", 0},
		{"primary_wins", map[string]any{"a": float64(3), "b": float64(8)}, "a", "b", 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := maputil.IntFieldAlt(tc.m, tc.primary, tc.alt)
			if got != tc.want {
				t.Errorf("IntFieldAlt() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestStringArray(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		m    map[string]any
		key  string
		want []string
	}{
		{"any_slice", map[string]any{"k": []any{"a", "b"}}, "k", []string{"a", "b"}},
		{"string_slice", map[string]any{"k": []string{"x", "y"}}, "k", []string{"x", "y"}},
		{"missing", map[string]any{}, "k", nil},
		{"mixed_types", map[string]any{"k": []any{"a", 42, "b"}}, "k", []string{"a", "b"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := maputil.StringArray(tc.m, tc.key)
			if len(got) != len(tc.want) {
				t.Fatalf("StringArray() len = %d, want %d", len(got), len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("StringArray()[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestToMap(t *testing.T) {
	t.Parallel()

	t.Run("map_passthrough", func(t *testing.T) {
		t.Parallel()
		input := map[string]any{"key": "val"}
		got, err := maputil.ToMap(input)
		if err != nil {
			t.Fatalf("ToMap: %v", err)
		}
		if got["key"] != "val" {
			t.Errorf("got[key] = %v, want val", got["key"])
		}
	})

	t.Run("nil_returns_error", func(t *testing.T) {
		t.Parallel()
		_, err := maputil.ToMap(nil)
		if err == nil {
			t.Errorf("ToMap(nil) should return error")
		}
	})
}

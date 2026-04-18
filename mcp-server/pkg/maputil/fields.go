// Package maputil provides type-safe field extraction from map[string]any.
package maputil

import (
	"encoding/json"
	"errors"
	"fmt"
)

// StringField extracts a string value from a map by key.
// Returns "" when the key is absent, nil, or not a string.
func StringField(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

// StringFieldAlt tries the primary key first, then falls back to the alt key.
func StringFieldAlt(m map[string]any, primary, alt string) string {
	if s := StringField(m, primary); s != "" {
		return s
	}
	return StringField(m, alt)
}

// BoolField extracts a bool value from a map by key.
// Returns false when the key is absent, nil, or not a bool.
func BoolField(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok || v == nil {
		return false
	}
	b, _ := v.(bool)
	return b
}

// IntFieldAlt extracts an integer value, trying primary key first then alt.
// Handles float64, int, and json.Number types.
func IntFieldAlt(m map[string]any, primary, alt string) int {
	raw, ok := m[primary]
	if !ok || raw == nil {
		raw, ok = m[alt]
		if !ok || raw == nil {
			return 0
		}
	}
	return toInt(raw)
}

// toInt converts a numeric value to int.
func toInt(raw any) int {
	switch v := raw.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return int(n)
		}
		if f, err := v.Float64(); err == nil {
			return int(f)
		}
	}
	return 0
}

// StringArray extracts a string slice from a map by key.
// Handles both []any (with non-string elements skipped) and []string.
// Returns nil when the key is absent.
func StringArray(m map[string]any, key string) []string {
	raw, ok := m[key]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []any:
		var result []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case []string:
		return v
	}
	return nil
}

// ToMap converts an arbitrary value to map[string]any via JSON round-trip.
// Returns an error if the input is nil or cannot be converted.
func ToMap(raw any) (map[string]any, error) {
	if raw == nil {
		return nil, errors.New("cannot convert nil to map")
	}
	if m, ok := raw.(map[string]any); ok {
		return m, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return m, nil
}

// Package tools — unit tests for context_fetcher.go.
// Tests cover parseEstimate, parseExternalContext (Linear fields),
// buildRequestMDWithBody (Linear branch), and IsTextSource.
package tools

import (
	"strings"
	"testing"
)

// ---------- TestParseEstimate ----------

func TestParseEstimate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		m    map[string]any
		want int
	}{
		{
			name: "float64_value",
			m:    map[string]any{"linear_estimate": float64(3)},
			want: 3,
		},
		{
			name: "int_value",
			m:    map[string]any{"linear_estimate": 5},
			want: 5,
		},
		{
			name: "fallback_alias_estimate",
			m:    map[string]any{"estimate": float64(8)},
			want: 8,
		},
		{
			name: "missing_key",
			m:    map[string]any{},
			want: 0,
		},
		{
			name: "nil_value",
			m:    map[string]any{"linear_estimate": nil},
			want: 0,
		},
		{
			name: "primary_wins_over_alias",
			m:    map[string]any{"linear_estimate": float64(3), "estimate": float64(8)},
			want: 3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := parseEstimate(tc.m)
			if got != tc.want {
				t.Errorf("parseEstimate() = %d, want %d", got, tc.want)
			}
		})
	}
}

// ---------- TestParseExternalContextLinear ----------

func TestParseExternalContextLinear(t *testing.T) {
	t.Parallel()

	t.Run("prefixed_linear_fields", func(t *testing.T) {
		t.Parallel()
		args := map[string]any{
			"external_context": map[string]any{
				"linear_title":       "Fix naming convention",
				"linear_description": "Update API naming",
				"linear_priority":    "high",
				"linear_estimate":    float64(3),
				"linear_labels":      []any{"bug", "backend"},
			},
		}
		extCtx, err := parseExternalContext(args)
		if err != nil {
			t.Fatalf("parseExternalContext: %v", err)
		}
		if extCtx.LinearTitle != "Fix naming convention" {
			t.Errorf("LinearTitle = %q, want %q", extCtx.LinearTitle, "Fix naming convention")
		}
		if extCtx.LinearDescription != "Update API naming" {
			t.Errorf("LinearDescription = %q, want %q", extCtx.LinearDescription, "Update API naming")
		}
		if extCtx.LinearPriority != "high" {
			t.Errorf("LinearPriority = %q, want %q", extCtx.LinearPriority, "high")
		}
		if extCtx.LinearEstimate != 3 {
			t.Errorf("LinearEstimate = %d, want %d", extCtx.LinearEstimate, 3)
		}
		if len(extCtx.LinearLabels) != 2 || extCtx.LinearLabels[0] != "bug" || extCtx.LinearLabels[1] != "backend" {
			t.Errorf("LinearLabels = %v, want [bug backend]", extCtx.LinearLabels)
		}
	})

	t.Run("fallback_aliases", func(t *testing.T) {
		t.Parallel()
		args := map[string]any{
			"external_context": map[string]any{
				"title":       "Fix naming convention",
				"description": "Update API naming",
				"priority":    "high",
				"estimate":    float64(5),
			},
		}
		extCtx, err := parseExternalContext(args)
		if err != nil {
			t.Fatalf("parseExternalContext: %v", err)
		}
		if extCtx.LinearTitle != "Fix naming convention" {
			t.Errorf("LinearTitle = %q, want %q (via fallback alias)", extCtx.LinearTitle, "Fix naming convention")
		}
		if extCtx.LinearDescription != "Update API naming" {
			t.Errorf("LinearDescription = %q, want %q (via fallback alias)", extCtx.LinearDescription, "Update API naming")
		}
		if extCtx.LinearEstimate != 5 {
			t.Errorf("LinearEstimate = %d, want %d (via fallback alias)", extCtx.LinearEstimate, 5)
		}
	})

	t.Run("nil_external_context", func(t *testing.T) {
		t.Parallel()
		args := map[string]any{}
		extCtx, err := parseExternalContext(args)
		if err != nil {
			t.Fatalf("parseExternalContext: %v", err)
		}
		if extCtx.LinearTitle != "" {
			t.Errorf("LinearTitle = %q, want empty", extCtx.LinearTitle)
		}
	})
}

// ---------- TestBuildRequestMDWithBodyLinear ----------

func TestBuildRequestMDWithBodyLinear(t *testing.T) {
	t.Parallel()

	t.Run("linear_context_produces_linear_issue", func(t *testing.T) {
		t.Parallel()
		extCtx := externalContext{
			SourceURL:         "https://linear.app/dealon/issue/DEA-13",
			SourceID:          "DEA-13",
			LinearTitle:       "Fix naming convention",
			LinearDescription: "Update all API endpoints",
		}
		got := buildRequestMDWithBody(extCtx, "")
		if !strings.Contains(got, "source_type: linear_issue") {
			t.Errorf("expected source_type: linear_issue in output:\n%s", got)
		}
		if !strings.Contains(got, "source_url: https://linear.app/dealon/issue/DEA-13") {
			t.Errorf("expected source_url in output:\n%s", got)
		}
		if !strings.Contains(got, "source_id: DEA-13") {
			t.Errorf("expected source_id in output:\n%s", got)
		}
		if !strings.Contains(got, "Fix naming convention") {
			t.Errorf("expected title in body:\n%s", got)
		}
		if !strings.Contains(got, "Update all API endpoints") {
			t.Errorf("expected description in body:\n%s", got)
		}
	})

	t.Run("linear_title_only", func(t *testing.T) {
		t.Parallel()
		extCtx := externalContext{
			LinearTitle: "Fix naming convention",
		}
		got := buildRequestMDWithBody(extCtx, "")
		if !strings.Contains(got, "source_type: linear_issue") {
			t.Errorf("expected source_type: linear_issue even with title only:\n%s", got)
		}
	})
}

// ---------- TestIsTextSource ----------

func TestIsTextSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		extCtx externalContext
		want   bool
	}{
		{
			name:   "empty_context",
			extCtx: externalContext{},
			want:   true,
		},
		{
			name:   "task_text_only",
			extCtx: externalContext{TaskText: "implement feature"},
			want:   true,
		},
		{
			name:   "github_title",
			extCtx: externalContext{GitHubTitle: "Fix bug"},
			want:   false,
		},
		{
			name:   "jira_summary",
			extCtx: externalContext{JiraSummary: "Fix bug"},
			want:   false,
		},
		{
			name:   "linear_title",
			extCtx: externalContext{LinearTitle: "Fix bug"},
			want:   false,
		},
		{
			name:   "linear_description_only",
			extCtx: externalContext{LinearDescription: "Some description"},
			want:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.extCtx.IsTextSource()
			if got != tc.want {
				t.Errorf("IsTextSource() = %v, want %v", got, tc.want)
			}
		})
	}
}

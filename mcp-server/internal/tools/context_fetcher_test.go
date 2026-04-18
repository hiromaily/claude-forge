// Package tools — unit tests for context_fetcher.go.
// Tests cover parseExternalContext (Linear fields),
// buildRequestMDWithBody (Linear branch), and IsTextSource.
package tools

import (
	"strings"
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/internal/sourcetype"
)

// ---------- TestParseExternalContextLinear ----------

func TestParseExternalContextLinear(t *testing.T) {
	t.Parallel()

	t.Run("prefixed_linear_fields", func(t *testing.T) {
		t.Parallel()
		args := map[string]any{
			"external_context": map[string]any{
				"linear_title":       "Fix naming convention",
				"linear_description": "Update API naming",
				"linear_estimate":    float64(3),
				"linear_labels":      []any{"bug", "backend"},
			},
		}
		extCtx, err := parseExternalContext(args, "linear_issue")
		if err != nil {
			t.Fatalf("parseExternalContext: %v", err)
		}
		if extCtx.Fields.Title != "Fix naming convention" {
			t.Errorf("Fields.Title = %q, want %q", extCtx.Fields.Title, "Fix naming convention")
		}
		if extCtx.Fields.Body != "Update API naming" {
			t.Errorf("Fields.Body = %q, want %q", extCtx.Fields.Body, "Update API naming")
		}
		if extCtx.Fields.StoryPoints != 3 {
			t.Errorf("Fields.StoryPoints = %d, want %d", extCtx.Fields.StoryPoints, 3)
		}
		if len(extCtx.Fields.Labels) != 2 || extCtx.Fields.Labels[0] != "bug" || extCtx.Fields.Labels[1] != "backend" {
			t.Errorf("Fields.Labels = %v, want [bug backend]", extCtx.Fields.Labels)
		}
	})

	t.Run("fallback_aliases", func(t *testing.T) {
		t.Parallel()
		args := map[string]any{
			"external_context": map[string]any{
				"title":       "Fix naming convention",
				"description": "Update API naming",
				"estimate":    float64(5),
			},
		}
		extCtx, err := parseExternalContext(args, "linear_issue")
		if err != nil {
			t.Fatalf("parseExternalContext: %v", err)
		}
		if extCtx.Fields.Title != "Fix naming convention" {
			t.Errorf("Fields.Title = %q, want %q (via fallback alias)", extCtx.Fields.Title, "Fix naming convention")
		}
		if extCtx.Fields.Body != "Update API naming" {
			t.Errorf("Fields.Body = %q, want %q (via fallback alias)", extCtx.Fields.Body, "Update API naming")
		}
		if extCtx.Fields.StoryPoints != 5 {
			t.Errorf("Fields.StoryPoints = %d, want %d (via fallback alias)", extCtx.Fields.StoryPoints, 5)
		}
	})

	t.Run("nil_external_context", func(t *testing.T) {
		t.Parallel()
		args := map[string]any{}
		extCtx, err := parseExternalContext(args, "")
		if err != nil {
			t.Fatalf("parseExternalContext: %v", err)
		}
		if extCtx.Fields.Title != "" {
			t.Errorf("Fields.Title = %q, want empty", extCtx.Fields.Title)
		}
	})
}

// ---------- TestBuildRequestMDWithBodyLinear ----------

func TestBuildRequestMDWithBodyLinear(t *testing.T) {
	t.Parallel()

	t.Run("linear_context_produces_linear_issue", func(t *testing.T) {
		t.Parallel()
		extCtx := externalContext{
			SourceURL:  "https://linear.app/dealon/issue/DEA-13",
			SourceID:   "DEA-13",
			SourceType: "linear_issue",
			Fields: sourcetype.ExternalFields{
				Title: "Fix naming convention",
				Body:  "Update all API endpoints",
			},
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
			SourceType: "linear_issue",
			Fields: sourcetype.ExternalFields{
				Title: "Fix naming convention",
			},
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
			name: "github_title",
			extCtx: externalContext{
				Fields: sourcetype.ExternalFields{Title: "Fix bug"},
			},
			want: false,
		},
		{
			name: "jira_summary",
			extCtx: externalContext{
				Fields: sourcetype.ExternalFields{Title: "Fix bug"},
			},
			want: false,
		},
		{
			name: "linear_title",
			extCtx: externalContext{
				Fields: sourcetype.ExternalFields{Title: "Fix bug"},
			},
			want: false,
		},
		{
			name: "body_only",
			extCtx: externalContext{
				Fields: sourcetype.ExternalFields{Body: "Some description"},
			},
			want: false,
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

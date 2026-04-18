package sourcetype

import "testing"

func TestGet_known_types(t *testing.T) {
	t.Parallel()
	for _, st := range []string{"github_issue", "jira_issue", "linear_issue"} {
		t.Run(st, func(t *testing.T) {
			t.Parallel()
			h := Get(st)
			if h == nil {
				t.Fatalf("Get(%q) = nil, want handler", st)
			}
			if h.Type() != st {
				t.Errorf("handler.Type() = %q, want %q", h.Type(), st)
			}
			if h.Label() == "" {
				t.Errorf("handler.Label() is empty")
			}
		})
	}
}

func TestGet_unknown(t *testing.T) {
	t.Parallel()
	if h := Get("text"); h != nil {
		t.Errorf("Get(text) should return nil, got %v", h)
	}
}

func TestAll_returns_three(t *testing.T) {
	t.Parallel()
	if n := len(All()); n != 3 {
		t.Errorf("All() returned %d handlers, want 3", n)
	}
}

func TestClassifyURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{"github", "https://github.com/org/repo/issues/42", "github_issue", false},
		{"jira", "https://example.atlassian.net/browse/PROJ-123", "jira_issue", false},
		{"linear", "https://linear.app/dealon/issue/DEA-13", "linear_issue", false},
		{"linear_with_slug", "https://linear.app/dealon/issue/DEA-13/some-slug", "linear_issue", false},
		{"github_malformed", "https://github.com/org/repo", "", true},
		{"unknown", "https://example.com/foo", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ClassifyURL(tc.url)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ClassifyURL(%q) = %q, want error", tc.url, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ClassifyURL(%q) error: %v", tc.url, err)
			}
			if got != tc.want {
				t.Errorf("ClassifyURL(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}

func TestIsURLSource(t *testing.T) {
	t.Parallel()
	if !IsURLSource("github_issue") {
		t.Errorf("IsURLSource(github_issue) = false, want true")
	}
	if IsURLSource("text") {
		t.Errorf("IsURLSource(text) = true, want false")
	}
}

func TestExtractSourceID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		sourceType string
		url        string
		want       string
	}{
		{"github", "github_issue", "https://github.com/org/repo/issues/42", "42"},
		{"jira", "jira_issue", "https://example.atlassian.net/browse/PROJ-123", "PROJ-123"},
		{"linear", "linear_issue", "https://linear.app/dealon/issue/DEA-13", "DEA-13"},
		{"linear_trailing_slash", "linear_issue", "https://linear.app/dealon/issue/DEA-13/", "DEA-13"},
		{"linear_with_slug", "linear_issue", "https://linear.app/dealon/issue/DEA-13/some-title", "DEA-13"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := Get(tc.sourceType)
			if h == nil {
				t.Fatalf("Get(%q) = nil", tc.sourceType)
			}
			got := h.ExtractSourceID(tc.url)
			if got != tc.want {
				t.Errorf("ExtractSourceID(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}

func TestExternalFields_IsEmpty(t *testing.T) {
	t.Parallel()
	if !(ExternalFields{}).IsEmpty() {
		t.Errorf("zero ExternalFields should be empty")
	}
	if (ExternalFields{Title: "x"}).IsEmpty() {
		t.Errorf("ExternalFields with Title should not be empty")
	}
}

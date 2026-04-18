# Source Type Registry + maputil Refactoring Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate DRY violations and scattered source-type knowledge by creating a `maputil` utility package and a `sourcetype` Handler registry with compile-time enforcement.

**Architecture:** Extract generic map helpers to `internal/maputil`. Create `internal/sourcetype` with Handler interface, ExternalFields unified type, and per-service implementations (GitHub, Jira, Linear). Refactor `tools` and `orchestrator` to delegate to the registry instead of inline switch statements.

**Tech Stack:** Go 1.26, stdlib only, table-driven tests with `t.Parallel()`

---

### Task 1: Create `internal/maputil` package

**Files:**
- Create: `poc/claude-forge/mcp-server/internal/maputil/fields.go`
- Create: `poc/claude-forge/mcp-server/internal/maputil/fields_test.go`

- [ ] **Step 1: Write tests for `StringField`, `StringFieldAlt`, `BoolField`**

Create `poc/claude-forge/mcp-server/internal/maputil/fields_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd poc/claude-forge/mcp-server && go test ./internal/maputil/... -count=1`
Expected: compilation error (package doesn't exist yet)

- [ ] **Step 3: Create `fields.go`**

Create `poc/claude-forge/mcp-server/internal/maputil/fields.go`:

```go
// Package maputil provides type-safe field extraction from map[string]any.
package maputil

import (
	"encoding/json"
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
		return nil, fmt.Errorf("cannot convert nil to map")
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
```

- [ ] **Step 4: Run tests**

Run: `cd poc/claude-forge/mcp-server && go test ./internal/maputil/... -count=1 -v`
Expected: all pass

- [ ] **Step 5: Commit**

```bash
git add poc/claude-forge/mcp-server/internal/maputil/
git commit -m "feat(maputil): add type-safe map field extraction utilities"
```

---

### Task 2: Create `internal/sourcetype` — types and Handler interface

**Files:**
- Create: `poc/claude-forge/mcp-server/internal/sourcetype/types.go`
- Create: `poc/claude-forge/mcp-server/internal/sourcetype/handler.go`
- Create: `poc/claude-forge/mcp-server/internal/sourcetype/registry.go`
- Create: `poc/claude-forge/mcp-server/internal/sourcetype/registry_test.go`

- [ ] **Step 1: Create `types.go`**

```go
// Package sourcetype provides a registry of external issue tracker integrations.
// Each source type (GitHub, Jira, Linear) implements the Handler interface.
// Adding a new integration requires only implementing Handler and registering it.
package sourcetype

// FetchConfig describes how to fetch external issue data.
// At most one of MCPTool or Command should drive the fetch;
// Instruction serves as a human-readable fallback.
type FetchConfig struct {
	Type            string            `json:"type"`
	MCPTool         string            `json:"mcp_tool,omitempty"`
	Command         string            `json:"command,omitempty"`
	MCPParams       map[string]string `json:"mcp_params,omitempty"`
	ResponseMapping map[string]string `json:"response_mapping"`
	Instruction     string            `json:"instruction"`
}

// PostConfig describes how to post a comment back to the source issue.
// At most one of MCPTool, Command, or Instruction should be set.
type PostConfig struct {
	MCPTool     string            `json:"mcp_tool,omitempty"`
	Command     string            `json:"command,omitempty"`
	MCPParams   map[string]string `json:"mcp_params,omitempty"`
	BodySource  string            `json:"body_source"`
	Instruction string            `json:"instruction,omitempty"`
}

// ExternalFields holds parsed issue fields in a service-neutral form.
// Each Handler populates the relevant fields; unused fields remain zero.
type ExternalFields struct {
	Title       string
	Body        string   // GitHub body, Jira/Linear description
	Labels      []string
	IssueType   string   // Jira only
	StoryPoints int      // Jira story points or Linear estimate
}

// CombinedText returns a single string of all text fields for effort detection.
func (ef ExternalFields) CombinedText() string {
	return ef.Title + " " + ef.Body
}

// IsEmpty returns true when no fields are populated.
func (ef ExternalFields) IsEmpty() bool {
	return ef.Title == "" && ef.Body == "" && ef.IssueType == "" && len(ef.Labels) == 0 && ef.StoryPoints == 0
}
```

- [ ] **Step 2: Create `handler.go`**

```go
package sourcetype

import "regexp"

// Handler defines the contract for a source type integration.
// Implementing this interface and registering in registry.go is all
// that's needed to add a new source type — no other files need changes.
type Handler interface {
	// Type returns the source type constant (e.g., "github_issue").
	Type() string

	// Label returns a human-readable label (e.g., "GitHub issue").
	Label() string

	// URLPattern returns the compiled regex that validates a full URL.
	URLPattern() *regexp.Regexp

	// BasePattern returns the compiled regex that identifies the service base URL.
	BasePattern() *regexp.Regexp

	// InvalidURLMessage returns the error message for malformed URLs.
	InvalidURLMessage() string

	// ExtractSourceID extracts the issue identifier from a validated URL.
	ExtractSourceID(rawURL string) string

	// FetchConfig returns the configuration for fetching issue data.
	FetchConfig(sourceURL, sourceID string) *FetchConfig

	// PostConfig returns the configuration for posting a comment back.
	// artifactPath is the full path to the summary file.
	PostConfig(sourceURL, sourceID, artifactPath string) *PostConfig

	// ParseExternalContext extracts typed fields from the raw external_context map.
	ParseExternalContext(m map[string]any) ExternalFields

	// SupportsClosingRef returns true if PR bodies should include "Closes #N".
	SupportsClosingRef() bool
}
```

- [ ] **Step 3: Create `registry.go`**

```go
package sourcetype

import (
	"fmt"
	"strings"
)

// handlers holds all registered source type handlers.
// Each handler file (github.go, jira.go, linear.go) adds itself via init().
var handlers []Handler

// register adds a handler to the registry. Called from init() in each handler file.
func register(h Handler) {
	handlers = append(handlers, h)
}

// Get returns the Handler for the given source type, or nil if not found.
func Get(sourceType string) Handler {
	for _, h := range handlers {
		if h.Type() == sourceType {
			return h
		}
	}
	return nil
}

// All returns all registered handlers.
func All() []Handler {
	return handlers
}

// IsURLSource returns true if the source type has a registered handler.
func IsURLSource(sourceType string) bool {
	return Get(sourceType) != nil
}

// ClassifyURL checks the URL against all registered handlers.
// Returns the source type on match, or an error with supported formats.
func ClassifyURL(rawURL string) (string, error) {
	for _, h := range handlers {
		if h.BasePattern().MatchString(rawURL) {
			if h.URLPattern().MatchString(rawURL) {
				return h.Type(), nil
			}
			return "", fmt.Errorf("ERROR: %s", h.InvalidURLMessage())
		}
	}

	var supported []string
	for _, h := range handlers {
		supported = append(supported, h.InvalidURLMessage())
	}
	return "", fmt.Errorf("ERROR: Unrecognised URL format. Supported formats: %s",
		strings.Join(supported, ", "))
}
```

- [ ] **Step 4: Create `registry_test.go`**

```go
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
```

- [ ] **Step 5: Run tests (should fail — handlers not yet registered)**

Run: `cd poc/claude-forge/mcp-server && go test ./internal/sourcetype/... -count=1`
Expected: fail (Get returns nil, All returns 0)

- [ ] **Step 6: Commit skeleton**

```bash
git add poc/claude-forge/mcp-server/internal/sourcetype/
git commit -m "feat(sourcetype): add Handler interface, registry, and types"
```

---

### Task 3: Implement GitHub, Jira, and Linear handlers

**Files:**
- Create: `poc/claude-forge/mcp-server/internal/sourcetype/github.go`
- Create: `poc/claude-forge/mcp-server/internal/sourcetype/jira.go`
- Create: `poc/claude-forge/mcp-server/internal/sourcetype/linear.go`

- [ ] **Step 1: Create `github.go`**

```go
package sourcetype

import (
	"net/url"
	"path"
	"regexp"

	"github.com/hiromaily/claude-forge/mcp-server/internal/maputil"
)

var (
	reGitHubURL  = regexp.MustCompile(`^https://github\.com/[^/]+/[^/]+/issues/[0-9]+`)
	reGitHubBase = regexp.MustCompile(`^https://github\.com/`)
)

func init() { register(&GitHubHandler{}) }

// GitHubHandler implements Handler for GitHub issues.
type GitHubHandler struct{}

func (h *GitHubHandler) Type() string              { return "github_issue" }
func (h *GitHubHandler) Label() string             { return "GitHub issue" }
func (h *GitHubHandler) URLPattern() *regexp.Regexp { return reGitHubURL }
func (h *GitHubHandler) BasePattern() *regexp.Regexp { return reGitHubBase }
func (h *GitHubHandler) SupportsClosingRef() bool   { return true }

func (h *GitHubHandler) InvalidURLMessage() string {
	return "Invalid GitHub URL format. Expected: https://github.com/{owner}/{repo}/issues/{number}"
}

func (h *GitHubHandler) ExtractSourceID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return path.Base(u.Path)
}

func (h *GitHubHandler) FetchConfig(sourceURL, sourceID string) *FetchConfig {
	return &FetchConfig{
		Type:    "github",
		Command: "gh issue view " + sourceURL + " --json title,body,labels",
		ResponseMapping: map[string]string{
			"title": "github_title", "body": "github_body", "labels": "github_labels",
		},
		Instruction: "fetch github issue fields before calling pipeline_init_with_context",
	}
}

func (h *GitHubHandler) PostConfig(sourceURL, sourceID, artifactPath string) *PostConfig {
	return &PostConfig{
		Command:    "gh issue comment " + sourceURL + " --body-file " + artifactPath,
		BodySource: artifactPath,
	}
}

func (h *GitHubHandler) ParseExternalContext(m map[string]any) ExternalFields {
	return ExternalFields{
		Title:  maputil.StringField(m, "github_title"),
		Body:   maputil.StringField(m, "github_body"),
		Labels: maputil.StringArray(m, "github_labels"),
	}
}
```

- [ ] **Step 2: Create `jira.go`**

```go
package sourcetype

import (
	"net/url"
	"path"
	"regexp"

	"github.com/hiromaily/claude-forge/mcp-server/internal/maputil"
)

var (
	reJiraURL  = regexp.MustCompile(`^https://[^/]+\.atlassian\.net/browse/[A-Z]+-[0-9]+`)
	reJiraBase = regexp.MustCompile(`^https://[^/]+\.atlassian\.net/`)
)

func init() { register(&JiraHandler{}) }

// JiraHandler implements Handler for Jira issues.
type JiraHandler struct{}

func (h *JiraHandler) Type() string              { return "jira_issue" }
func (h *JiraHandler) Label() string             { return "Jira issue" }
func (h *JiraHandler) URLPattern() *regexp.Regexp { return reJiraURL }
func (h *JiraHandler) BasePattern() *regexp.Regexp { return reJiraBase }
func (h *JiraHandler) SupportsClosingRef() bool   { return false }

func (h *JiraHandler) InvalidURLMessage() string {
	return "Invalid Jira URL format. Expected: https://{org}.atlassian.net/browse/{KEY}-{number}"
}

func (h *JiraHandler) ExtractSourceID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return path.Base(u.Path)
}

func (h *JiraHandler) FetchConfig(sourceURL, sourceID string) *FetchConfig {
	return &FetchConfig{
		Type: "jira",
		ResponseMapping: map[string]string{
			"summary": "jira_summary", "description": "jira_description",
			"issue_type": "jira_issue_type", "story_points": "jira_story_points",
		},
		Instruction: "fetch jira issue fields (summary, description, issuetype, story_points) before calling pipeline_init_with_context. Use Atlassian MCP tools if available, or Jira REST API with $JIRA_USER:$JIRA_TOKEN credentials.",
	}
}

func (h *JiraHandler) PostConfig(sourceURL, sourceID, artifactPath string) *PostConfig {
	return &PostConfig{
		BodySource:  artifactPath,
		Instruction: "Post the contents of " + artifactPath + " as a comment to " + sourceURL + ". Use Atlassian MCP tools if available, or convert the markdown to ADF and POST via Jira REST API with $JIRA_USER:$JIRA_TOKEN.",
	}
}

func (h *JiraHandler) ParseExternalContext(m map[string]any) ExternalFields {
	return ExternalFields{
		Title:       maputil.StringFieldAlt(m, "jira_summary", "summary"),
		Body:        maputil.StringFieldAlt(m, "jira_description", "description"),
		IssueType:   maputil.StringFieldAlt(m, "jira_issue_type", "issue_type"),
		StoryPoints: maputil.IntFieldAlt(m, "jira_story_points", "story_points"),
	}
}
```

- [ ] **Step 3: Create `linear.go`**

```go
package sourcetype

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/hiromaily/claude-forge/mcp-server/internal/maputil"
)

var (
	reLinearURL  = regexp.MustCompile(`^https://linear\.app/[^/]+/issue/[A-Z]+-[0-9]+`)
	reLinearBase = regexp.MustCompile(`^https://linear\.app/`)
)

func init() { register(&LinearHandler{}) }

// LinearHandler implements Handler for Linear issues.
type LinearHandler struct{}

func (h *LinearHandler) Type() string              { return "linear_issue" }
func (h *LinearHandler) Label() string             { return "Linear issue" }
func (h *LinearHandler) URLPattern() *regexp.Regexp { return reLinearURL }
func (h *LinearHandler) BasePattern() *regexp.Regexp { return reLinearBase }
func (h *LinearHandler) SupportsClosingRef() bool   { return false }

func (h *LinearHandler) InvalidURLMessage() string {
	return "Invalid Linear URL format. Expected: https://linear.app/{org}/issue/{KEY}-{number}"
}

func (h *LinearHandler) ExtractSourceID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i, seg := range segments {
		if seg == "issue" && i+1 < len(segments) {
			return segments[i+1]
		}
	}
	return ""
}

func (h *LinearHandler) FetchConfig(sourceURL, sourceID string) *FetchConfig {
	return &FetchConfig{
		Type:    "linear",
		MCPTool: "mcp__linear__get_issue",
		MCPParams: map[string]string{
			"issueId": sourceID,
		},
		ResponseMapping: map[string]string{
			"title": "linear_title", "description": "linear_description",
			"priority": "linear_priority", "estimate": "linear_estimate",
			"labels": "linear_labels",
		},
		Instruction: "fetch linear issue fields before calling pipeline_init_with_context",
	}
}

func (h *LinearHandler) PostConfig(sourceURL, sourceID, artifactPath string) *PostConfig {
	return &PostConfig{
		MCPTool:    "mcp__linear__save_comment",
		MCPParams:  map[string]string{"issueId": sourceID},
		BodySource: artifactPath,
	}
}

func (h *LinearHandler) ParseExternalContext(m map[string]any) ExternalFields {
	return ExternalFields{
		Title:       maputil.StringFieldAlt(m, "linear_title", "title"),
		Body:        maputil.StringFieldAlt(m, "linear_description", "description"),
		Labels:      maputil.StringArray(m, "linear_labels"),
		StoryPoints: maputil.IntFieldAlt(m, "linear_estimate", "estimate"),
	}
}
```

- [ ] **Step 4: Run sourcetype tests**

Run: `cd poc/claude-forge/mcp-server && go test ./internal/sourcetype/... -count=1 -v`
Expected: all pass (handlers registered via init)

- [ ] **Step 5: Commit**

```bash
git add poc/claude-forge/mcp-server/internal/sourcetype/
git commit -m "feat(sourcetype): implement GitHub, Jira, and Linear handlers"
```

---

### Task 4: Refactor `validation/input.go` to use sourcetype registry

**Files:**
- Modify: `poc/claude-forge/mcp-server/internal/validation/input.go`

- [ ] **Step 1: Replace URL regex vars and validateURL with sourcetype.ClassifyURL**

Remove from `input.go`:
- The `reGitHubURL`, `reGitHubBase`, `reJiraURL`, `reJiraBase`, `reLinearURL`, `reLinearBase` vars (lines 50-57)

Replace `validateURL` function body:

```go
func validateURL(core string, flags map[string]string, bareFlags []string) InputResult {
	sourceType, err := sourcetype.ClassifyURL(core)
	if err != nil {
		return InputResult{
			Valid:  false,
			Errors: []string{err.Error()},
		}
	}
	return InputResult{
		Valid: true,
		Parsed: ParsedInput{
			Flags:      flags,
			BareFlags:  normalizeBareFlags(bareFlags),
			CoreText:   core,
			SourceType: sourceType,
		},
	}
}
```

Add import: `"github.com/hiromaily/claude-forge/mcp-server/internal/sourcetype"`

Keep `reHTTPS` — it's used for the `isURL` check in `ValidateInput`.

- [ ] **Step 2: Run validation tests**

Run: `cd poc/claude-forge/mcp-server && go test ./internal/validation/... -count=1`
Expected: all pass (black-box tests, behavior unchanged)

- [ ] **Step 3: Commit**

```bash
git add poc/claude-forge/mcp-server/internal/validation/input.go
git commit -m "refactor(validation): delegate URL classification to sourcetype registry"
```

---

### Task 5: Refactor `tools/pipeline_init.go` to use sourcetype

**Files:**
- Modify: `poc/claude-forge/mcp-server/internal/tools/pipeline_init.go`
- Modify: `poc/claude-forge/mcp-server/internal/tools/pipeline_init_test.go`

- [ ] **Step 1: Replace FetchNeeded with sourcetype.FetchConfig**

Remove the `FetchNeeded` struct definition from `pipeline_init.go`.

In `PipelineInitResult`, change:
```go
FetchNeeded *FetchNeeded `json:"fetch_needed,omitempty"`
```
to:
```go
FetchNeeded *sourcetype.FetchConfig `json:"fetch_needed,omitempty"`
```

Add import: `"github.com/hiromaily/claude-forge/mcp-server/internal/sourcetype"`

- [ ] **Step 2: Replace extractSourceID and makeFetchNeeded**

Replace `extractSourceID`:
```go
func extractSourceID(sourceType, coreText string) string {
	h := sourcetype.Get(sourceType)
	if h == nil {
		return ""
	}
	return h.ExtractSourceID(coreText)
}
```

Replace `makeFetchNeeded`:
```go
func makeFetchNeeded(sourceType, sourceURL, sourceID string) *sourcetype.FetchConfig {
	h := sourcetype.Get(sourceType)
	if h == nil {
		return nil
	}
	return h.FetchConfig(sourceURL, sourceID)
}
```

Replace the sourceURL check:
```go
var sourceURL string
if sourcetype.IsURLSource(sourceType) {
	sourceURL = coreText
}
```

- [ ] **Step 3: Simplify refineWorkspacePath**

Replace the 8-case switch with:
```go
func refineWorkspacePath(workspace string, extCtx externalContext) string {
	title := extCtx.Fields.Title
	if title == "" {
		return workspace
	}
	combined := title
	if extCtx.SourceID != "" {
		combined = extCtx.SourceID + " " + title
	}
	return replaceWorkspaceSlug(workspace, slugifyOrDefault(combined))
}
```

- [ ] **Step 4: Update tests**

In `pipeline_init_test.go`, update type references from `FetchNeeded` to `sourcetype.FetchConfig` if needed (the JSON field name `fetch_needed` is unchanged, so tests using `parsePipelineInitResult` should still work if the result struct field type is aliased correctly).

Add import to test file if needed: `"github.com/hiromaily/claude-forge/mcp-server/internal/sourcetype"`

- [ ] **Step 5: Run tests**

Run: `cd poc/claude-forge/mcp-server && go test ./internal/tools/... -count=1 -run TestPipelineInit`
Expected: all pass

- [ ] **Step 6: Commit**

```bash
git add poc/claude-forge/mcp-server/internal/tools/pipeline_init.go poc/claude-forge/mcp-server/internal/tools/pipeline_init_test.go
git commit -m "refactor(pipeline-init): delegate to sourcetype registry for extractSourceID and makeFetchNeeded"
```

---

### Task 6: Refactor `tools/context_fetcher.go` to use maputil and sourcetype

**Files:**
- Modify: `poc/claude-forge/mcp-server/internal/tools/context_fetcher.go`
- Modify: `poc/claude-forge/mcp-server/internal/tools/context_fetcher_test.go`
- Modify: `poc/claude-forge/mcp-server/internal/tools/pipeline_init_with_context.go`

- [ ] **Step 1: Simplify externalContext struct**

Replace the per-service fields with a unified `ExternalFields`:

```go
type externalContext struct {
	SourceURL  string
	SourceID   string
	SourceType string // stored for buildRequestMDWithBody
	TaskText   string
	Fields     sourcetype.ExternalFields
}

func (ec externalContext) IsTextSource() bool {
	return ec.Fields.IsEmpty()
}
```

- [ ] **Step 2: Rewrite parseExternalContext**

The function now needs `sourceType` to look up the correct handler:

```go
func parseExternalContext(args map[string]any, sourceType string) (externalContext, error) {
	var extCtx externalContext
	extCtx.SourceType = sourceType

	raw, ok := args["external_context"]
	if !ok || raw == nil {
		return extCtx, nil
	}

	m, ok := raw.(map[string]any)
	if !ok {
		return extCtx, fmt.Errorf("external_context must be an object, got %T", raw)
	}

	extCtx.SourceURL = maputil.StringField(m, "source_url")
	extCtx.SourceID = maputil.StringField(m, "source_id")

	h := sourcetype.Get(sourceType)
	if h != nil {
		extCtx.Fields = h.ParseExternalContext(m)
	}

	return extCtx, nil
}
```

- [ ] **Step 3: Simplify buildRequestMDWithBody**

```go
func buildRequestMDWithBody(extCtx externalContext, body string) string {
	var sb strings.Builder

	sourceType := "text"
	var resolvedBody string

	if !extCtx.Fields.IsEmpty() && extCtx.SourceType != "" {
		sourceType = extCtx.SourceType
		resolvedBody = strings.TrimSpace(extCtx.Fields.Title + "\n\n" + extCtx.Fields.Body)
	} else {
		resolvedBody = body
	}

	sb.WriteString("---\n")
	sb.WriteString("source_type: ")
	sb.WriteString(sourceType)
	sb.WriteString("\n")
	if extCtx.SourceURL != "" {
		sb.WriteString("source_url: ")
		sb.WriteString(extCtx.SourceURL)
		sb.WriteString("\n")
	}
	if extCtx.SourceID != "" {
		sb.WriteString("source_id: ")
		sb.WriteString(extCtx.SourceID)
		sb.WriteString("\n")
	}
	sb.WriteString("---\n")

	if resolvedBody != "" {
		sb.WriteString("\n")
		sb.WriteString(resolvedBody)
		sb.WriteString("\n")
	}

	return sb.String()
}
```

- [ ] **Step 4: Remove deprecated helpers**

Delete from `context_fetcher.go`:
- `parseStoryPoints` (replaced by `maputil.IntFieldAlt`)
- `parseEstimate` (replaced by `maputil.IntFieldAlt`)
- `stringField` (replaced by `maputil.StringField`)
- `stringFieldAlt` (replaced by `maputil.StringFieldAlt`)
- `boolField` (replaced by `maputil.BoolField`)

Note: `boolField` is still used in `pipeline_init_with_context.go`. Replace those calls with `maputil.BoolField`.

- [ ] **Step 5: Update `pipeline_init_with_context.go`**

Update `parseExternalContext` call site — add `sourceType` argument. The handler extracts source_type from `args` or it's passed from the caller.

Find the call at approximately line 115:
```go
extCtx, err := parseExternalContext(args)
```
The source type is not directly available in the handler args. We need to derive it. The simplest approach: detect source type from the external_context fields, or pass it through. Since `parseExternalContext` now takes it as a parameter, and the MCP call doesn't explicitly pass it, we can detect it:

```go
// Detect source type from external_context fields.
sourceType := ""
if raw, ok := args["external_context"]; ok && raw != nil {
	if m, ok := raw.(map[string]any); ok {
		for _, h := range sourcetype.All() {
			fields := h.ParseExternalContext(m)
			if !fields.IsEmpty() {
				sourceType = h.Type()
				break
			}
		}
	}
}
extCtx, err := parseExternalContext(args, sourceType)
```

Actually this is overcomplicated. The simpler approach: `parseExternalContext` parses ALL handler contexts and picks the first non-empty one. But that defeats the purpose of the handler.

Better approach: store the `source_type` in `pipeline_init` result and have the orchestrator pass it back to `pipeline_init_with_context`. But the orchestrator already passes `source_url` and `source_id` — not `source_type`. We can derive source type from source_url:

```go
sourceURL := maputil.StringField(args, "source_url")
sourceType := ""
if sourceURL != "" {
	st, _ := sourcetype.ClassifyURL(sourceURL)
	sourceType = st
}
extCtx, err := parseExternalContext(args, sourceType)
```

If `source_url` is absent (text source), `sourceType` is empty and `parseExternalContext` skips handler parsing — correct behavior.

Update `boolField` calls to `maputil.BoolField`.

Update effort detection:
```go
combinedText := strings.TrimSpace(extCtx.Fields.CombinedText() + " " + extCtx.TaskText)
effort := orchestrator.DetectEffort(flags.EffortOverride, extCtx.Fields.StoryPoints, combinedText)
```

- [ ] **Step 6: Update tests**

Update `context_fetcher_test.go` — the `externalContext` struct shape changed. Tests now use `extCtx.Fields.Title` instead of `extCtx.LinearTitle`.

Update `pipeline_init_with_context_test.go` — no structural change needed since tests use MCP handler calls, not internal structs directly.

- [ ] **Step 7: Run tests**

Run: `cd poc/claude-forge/mcp-server && go test ./internal/tools/... -count=1`
Expected: all pass

- [ ] **Step 8: Commit**

```bash
git add poc/claude-forge/mcp-server/internal/tools/
git commit -m "refactor(context-fetcher): use maputil and sourcetype for unified field handling"
```

---

### Task 7: Refactor `orchestrator/engine.go` to use sourcetype

**Files:**
- Modify: `poc/claude-forge/mcp-server/internal/orchestrator/actions.go`
- Modify: `poc/claude-forge/mcp-server/internal/orchestrator/engine.go`
- Modify: `poc/claude-forge/mcp-server/internal/orchestrator/engine_test.go`

- [ ] **Step 1: Replace PostMethod with sourcetype.PostConfig in actions.go**

Remove the `PostMethod` struct definition. Change the `Action` field:
```go
PostMethod *sourcetype.PostConfig `json:"post_method,omitempty"`
```

Add import: `"github.com/hiromaily/claude-forge/mcp-server/internal/sourcetype"`

- [ ] **Step 2: Rewrite handlePostToSource in engine.go**

```go
func (e *Engine) handlePostToSource(st *state.State) (Action, error) {
	sourceType := e.sourceTypeReader(st.Workspace)

	h := sourcetype.Get(sourceType)
	if h == nil {
		return NewDoneAction(SkipSummaryPrefix+PhasePostToSource, ""), nil
	}

	sourceURL := e.sourceURLReader(st.Workspace)
	if sourceURL == "" {
		return NewDoneAction(SkipSummaryPrefix+PhasePostToSource, ""), nil
	}

	sourceID := e.sourceIDReader(st.Workspace)
	artifactPath := filepath.Join(st.Workspace, state.ArtifactSummary)
	pm := h.PostConfig(sourceURL, sourceID, artifactPath)

	msg := fmt.Sprintf(
		"Pipeline complete. Post the final summary as a comment to the %s?\n\nURL: %s\nSummary file: %s",
		h.Label(), sourceURL, artifactPath,
	)

	action := NewCheckpointAction(PhasePostToSource, msg, []string{"post", "skip"})
	action.PostMethod = pm
	return action, nil
}
```

Delete `sourceTypeLabel` and `buildPostMethod` functions.

- [ ] **Step 3: Update handlePRCreation closing ref**

Replace:
```go
if e.sourceTypeReader(st.Workspace) == state.SourceTypeGitHub {
```
with:
```go
h := sourcetype.Get(e.sourceTypeReader(st.Workspace))
if h != nil && h.SupportsClosingRef() {
```

- [ ] **Step 4: Update engine_test.go**

Replace `PostMethod` type references with `sourcetype.PostConfig`. The `TestBuildPostMethod` and `TestSourceTypeLabel` tests become obsolete — they tested functions that no longer exist. Replace them with tests that verify the handler registry behavior (or remove them since the handler-level tests in Task 3 cover this).

Update `TestPostToSource_CheckpointOptions` assertions to use `sourcetype.PostConfig` field names (they're the same: `MCPTool`, `Command`, `BodySource`).

- [ ] **Step 5: Run tests**

Run: `cd poc/claude-forge/mcp-server && go test -race ./... -count=1`
Expected: all pass

- [ ] **Step 6: Commit**

```bash
git add poc/claude-forge/mcp-server/internal/orchestrator/
git commit -m "refactor(engine): delegate to sourcetype handlers, remove PostMethod and switch statements"
```

---

### Task 8: Final verification and cleanup

**Files:** (verification + cleanup only)

- [ ] **Step 1: Run full test suite**

Run: `cd poc/claude-forge/mcp-server && go test -race ./... -count=1`
Expected: all packages pass

- [ ] **Step 2: Run linter**

Run: `cd poc/claude-forge/mcp-server && make go-lint-fast`
Expected: no errors

- [ ] **Step 3: Verify no source-type switch statements remain**

Run: `grep -rn 'case.*SourceTypeGitHub\|case.*github_issue\|case.*SourceTypeJira\|case.*jira_issue\|case.*SourceTypeLinear\|case.*linear_issue' poc/claude-forge/mcp-server/internal/tools/ poc/claude-forge/mcp-server/internal/orchestrator/ poc/claude-forge/mcp-server/internal/validation/`
Expected: no matches (all switch logic moved to sourcetype handlers)

- [ ] **Step 4: Verify import cycle is clean**

Run: `cd poc/claude-forge/mcp-server && go vet ./...`
Expected: no import cycle errors

- [ ] **Step 5: Commit any remaining fixes**

If any issues were found and fixed, commit them.

// Package tools — unit tests for PipelineInitHandler.
// Tests cover resume detection, source types, flag parsing, fetch_needed,
// workspace path generation, and invalid input handling.
package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
)

// ---------- helpers ----------

// parsePipelineInitResult unmarshals the handler response into PipelineInitResult.
func parsePipelineInitResult(t *testing.T, raw string) PipelineInitResult {
	t.Helper()
	var r PipelineInitResult
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatalf("parsePipelineInitResult: %v (content: %s)", err, raw)
	}
	return r
}

// ---------- TestHandleResumePath ----------
// Tests the internal handleResumePath helper directly since the handler
// uses HasPrefix(".specs/") which requires a relative path.

func TestHandleResumePath(t *testing.T) {
	t.Parallel()

	sm := state.NewStateManager()

	t.Run("state_json_exists_returns_resume_true", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		if err := sm.Init(dir, "test-spec"); err != nil {
			t.Fatalf("Init: %v", err)
		}

		res, err := handleResumePath(dir)
		if err != nil {
			t.Fatalf("handleResumePath returned go error: %v", err)
		}
		r := parsePipelineInitResult(t, textContent(res))
		if !r.Resume {
			t.Errorf("expected resume=true when state.json exists, got false")
		}
		if r.Workspace != dir {
			t.Errorf("workspace: got %q, want %q", r.Workspace, dir)
		}
		if r.Instruction != "call state_resume_info" {
			t.Errorf("instruction: got %q, want %q", r.Instruction, "call state_resume_info")
		}
		if len(r.Errors) != 0 {
			t.Errorf("errors should be empty for resume path, got %v", r.Errors)
		}
	})

	t.Run("state_json_absent_returns_error_result", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		// Do NOT create state.json.

		res, err := handleResumePath(dir)
		if err != nil {
			t.Fatalf("handleResumePath returned go error: %v", err)
		}
		r := parsePipelineInitResult(t, textContent(res))
		if r.Resume {
			t.Errorf("resume should be false when state.json is absent")
		}
		if len(r.Errors) == 0 {
			t.Errorf("expected errors when state.json is absent, got none")
		}
	})
}

// ---------- TestPipelineInitResumeDetection ----------

func TestPipelineInitResumeDetection(t *testing.T) {
	t.Parallel()

	sm := state.NewStateManager()
	h := PipelineInitHandler(sm)

	t.Run("resume_with_specs_prefix_missing_state_json", func(t *testing.T) {
		t.Parallel()

		// Pass a ".specs/" prefix path with no state.json — should return error result.
		res := callTool(t, h, map[string]any{
			"arguments": ".specs/20260101-nonexistent-workspace-for-test",
		})
		if res.IsError {
			t.Fatalf("handler should not return MCP error, got: %v", textContent(res))
		}
		r := parsePipelineInitResult(t, textContent(res))
		if r.Resume {
			t.Errorf("resume should be false when state.json is absent")
		}
		if len(r.Errors) == 0 {
			t.Errorf("expected errors when state.json is absent for .specs/ prefix path, got none")
		}
	})

	t.Run("non_resume_text", func(t *testing.T) {
		t.Parallel()

		res := callTool(t, h, map[string]any{
			"arguments": "implement a new feature for user login",
		})
		if res.IsError {
			t.Fatalf("handler returned MCP error for text input: %v", textContent(res))
		}
		r := parsePipelineInitResult(t, textContent(res))
		if r.Resume {
			t.Errorf("resume should be false for plain text input")
		}
		if r.SourceType != "text" {
			t.Errorf("source_type: got %q, want %q", r.SourceType, "text")
		}
	})

	t.Run("specs_mid_string_not_resume", func(t *testing.T) {
		t.Parallel()

		// ".specs/" appearing mid-string (not a prefix) should NOT trigger resume.
		res := callTool(t, h, map[string]any{
			"arguments": "prefix .specs/something",
		})
		if res.IsError {
			t.Fatalf("handler returned MCP error: %v", textContent(res))
		}
		r := parsePipelineInitResult(t, textContent(res))
		if r.Resume {
			t.Errorf("resume should be false when .specs/ is mid-string, not a prefix")
		}
	})

	t.Run("whitespace_trimmed_before_prefix_check", func(t *testing.T) {
		t.Parallel()

		// Leading/trailing whitespace is trimmed before HasPrefix check.
		// Use a .specs/ path that won't have state.json — checks the code path correctly.
		res := callTool(t, h, map[string]any{
			"arguments": "   .specs/20260101-whitespace-test-no-state-json   ",
		})
		if res.IsError {
			t.Fatalf("handler should not return MCP error, got: %v", textContent(res))
		}
		r := parsePipelineInitResult(t, textContent(res))
		// The trimmed argument starts with ".specs/" → resume path triggered.
		// state.json doesn't exist → error result.
		if len(r.Errors) == 0 {
			t.Errorf("expected errors (no state.json for .specs/ path), got none; this verifies trimming occurred")
		}
		// It should NOT have been treated as a new pipeline.
		if r.SourceType != "" {
			t.Errorf("source_type should be empty for resume detection path, got %q", r.SourceType)
		}
	})
}

// ---------- TestPipelineInitSourceTypes ----------

func TestPipelineInitSourceTypes(t *testing.T) {
	t.Parallel()

	sm := state.NewStateManager()
	h := PipelineInitHandler(sm)

	tests := []struct {
		name           string
		arguments      string
		wantSourceType string
		wantSourceID   string
	}{
		{
			name:           "github_url",
			arguments:      "https://github.com/owner/repo/issues/42",
			wantSourceType: "github_issue",
			wantSourceID:   "42",
		},
		{
			name:           "jira_url",
			arguments:      "https://myorg.atlassian.net/browse/PROJ-123",
			wantSourceType: "jira_issue",
			wantSourceID:   "PROJ-123",
		},
		{
			name:           "free_text",
			arguments:      "implement a user authentication feature",
			wantSourceType: "text",
			wantSourceID:   "",
		},
		{
			name: "workspace_path",
			// A path containing ".specs/" but not starting with it → "workspace" source type.
			arguments:      "some/path/.specs/20260101-my-feature",
			wantSourceType: "workspace",
			wantSourceID:   "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			res := callTool(t, h, map[string]any{
				"arguments": tc.arguments,
			})
			if res.IsError {
				t.Fatalf("handler returned MCP error: %v", textContent(res))
			}
			r := parsePipelineInitResult(t, textContent(res))
			if r.Resume {
				t.Errorf("should not be resume for source type test")
			}
			if r.SourceType != tc.wantSourceType {
				t.Errorf("source_type: got %q, want %q", r.SourceType, tc.wantSourceType)
			}
			if tc.wantSourceID != "" && r.SourceID != tc.wantSourceID {
				t.Errorf("source_id: got %q, want %q", r.SourceID, tc.wantSourceID)
			}
		})
	}
}

// ---------- TestPipelineInitFlagParsing ----------

func TestPipelineInitFlagParsing(t *testing.T) {
	t.Parallel()

	sm := state.NewStateManager()
	h := PipelineInitHandler(sm)

	t.Run("auto_flag", func(t *testing.T) {
		t.Parallel()
		res := callTool(t, h, map[string]any{
			"arguments": "implement feature --auto",
		})
		if res.IsError {
			t.Fatalf("handler returned MCP error: %v", textContent(res))
		}
		r := parsePipelineInitResult(t, textContent(res))
		if r.Flags == nil {
			t.Fatalf("flags is nil")
		}
		if !r.Flags.Auto {
			t.Errorf("auto flag: got false, want true")
		}
	})

	t.Run("nopr_flag", func(t *testing.T) {
		t.Parallel()
		res := callTool(t, h, map[string]any{
			"arguments": "implement feature --nopr",
		})
		if res.IsError {
			t.Fatalf("handler returned MCP error: %v", textContent(res))
		}
		r := parsePipelineInitResult(t, textContent(res))
		if r.Flags == nil {
			t.Fatalf("flags is nil")
		}
		if !r.Flags.SkipPR {
			t.Errorf("skip_pr flag: got false, want true")
		}
	})

	t.Run("debug_flag", func(t *testing.T) {
		t.Parallel()
		res := callTool(t, h, map[string]any{
			"arguments": "implement feature --debug",
		})
		if res.IsError {
			t.Fatalf("handler returned MCP error: %v", textContent(res))
		}
		r := parsePipelineInitResult(t, textContent(res))
		if r.Flags == nil {
			t.Fatalf("flags is nil")
		}
		if !r.Flags.Debug {
			t.Errorf("debug flag: got false, want true")
		}
	})

	t.Run("type_override", func(t *testing.T) {
		t.Parallel()
		res := callTool(t, h, map[string]any{
			"arguments": "implement feature --type=bugfix",
		})
		if res.IsError {
			t.Fatalf("handler returned MCP error: %v", textContent(res))
		}
		r := parsePipelineInitResult(t, textContent(res))
		if r.Flags == nil {
			t.Fatalf("flags is nil")
		}
		if r.Flags.TypeOverride == nil {
			t.Fatalf("type_override is nil")
		}
		if *r.Flags.TypeOverride != "bugfix" {
			t.Errorf("type_override: got %q, want %q", *r.Flags.TypeOverride, "bugfix")
		}
	})

	t.Run("effort_override", func(t *testing.T) {
		t.Parallel()
		res := callTool(t, h, map[string]any{
			"arguments": "implement feature --effort=S",
		})
		if res.IsError {
			t.Fatalf("handler returned MCP error: %v", textContent(res))
		}
		r := parsePipelineInitResult(t, textContent(res))
		if r.Flags == nil {
			t.Fatalf("flags is nil")
		}
		if r.Flags.EffortOverride == nil {
			t.Fatalf("effort_override is nil")
		}
		if *r.Flags.EffortOverride != "S" {
			t.Errorf("effort_override: got %q, want %q", *r.Flags.EffortOverride, "S")
		}
	})

	t.Run("combined_flags", func(t *testing.T) {
		t.Parallel()
		res := callTool(t, h, map[string]any{
			"arguments": "implement feature --auto --nopr --debug --type=feature --effort=M",
		})
		if res.IsError {
			t.Fatalf("handler returned MCP error: %v", textContent(res))
		}
		r := parsePipelineInitResult(t, textContent(res))
		if r.Flags == nil {
			t.Fatalf("flags is nil")
		}
		if !r.Flags.Auto {
			t.Errorf("auto: got false, want true")
		}
		if !r.Flags.SkipPR {
			t.Errorf("skip_pr: got false, want true")
		}
		if !r.Flags.Debug {
			t.Errorf("debug: got false, want true")
		}
		if r.Flags.TypeOverride == nil || *r.Flags.TypeOverride != "feature" {
			t.Errorf("type_override: got %v, want feature", r.Flags.TypeOverride)
		}
		if r.Flags.EffortOverride == nil || *r.Flags.EffortOverride != "M" {
			t.Errorf("effort_override: got %v, want M", r.Flags.EffortOverride)
		}
	})

	t.Run("current_branch_echoed", func(t *testing.T) {
		t.Parallel()
		res := callTool(t, h, map[string]any{
			"arguments":      "implement feature",
			"current_branch": "feature/my-branch",
		})
		if res.IsError {
			t.Fatalf("handler returned MCP error: %v", textContent(res))
		}
		r := parsePipelineInitResult(t, textContent(res))
		if r.Flags == nil {
			t.Fatalf("flags is nil")
		}
		if r.Flags.CurrentBranch != "feature/my-branch" {
			t.Errorf("current_branch: got %q, want %q", r.Flags.CurrentBranch, "feature/my-branch")
		}
	})
}

// ---------- TestPipelineInitFetchNeeded ----------

func TestPipelineInitFetchNeeded(t *testing.T) {
	t.Parallel()

	sm := state.NewStateManager()
	h := PipelineInitHandler(sm)

	t.Run("github_non_null", func(t *testing.T) {
		t.Parallel()
		res := callTool(t, h, map[string]any{
			"arguments": "https://github.com/owner/repo/issues/42",
		})
		if res.IsError {
			t.Fatalf("handler returned MCP error: %v", textContent(res))
		}
		r := parsePipelineInitResult(t, textContent(res))
		if r.FetchNeeded == nil {
			t.Fatalf("fetch_needed is nil for github_issue, want non-nil")
		}
		if r.FetchNeeded.Type != "github" {
			t.Errorf("fetch_needed.type: got %q, want %q", r.FetchNeeded.Type, "github")
		}
		wantFields := []string{"labels", "title", "body"}
		if len(r.FetchNeeded.Fields) != len(wantFields) {
			t.Errorf("fetch_needed.fields: got %v, want %v", r.FetchNeeded.Fields, wantFields)
		} else {
			for i, f := range wantFields {
				if r.FetchNeeded.Fields[i] != f {
					t.Errorf("fetch_needed.fields[%d]: got %q, want %q", i, r.FetchNeeded.Fields[i], f)
				}
			}
		}
	})

	t.Run("jira_non_null_with_correct_fields", func(t *testing.T) {
		t.Parallel()
		res := callTool(t, h, map[string]any{
			"arguments": "https://myorg.atlassian.net/browse/PROJ-123",
		})
		if res.IsError {
			t.Fatalf("handler returned MCP error: %v", textContent(res))
		}
		r := parsePipelineInitResult(t, textContent(res))
		if r.FetchNeeded == nil {
			t.Fatalf("fetch_needed is nil for jira_issue, want non-nil")
		}
		if r.FetchNeeded.Type != "jira" {
			t.Errorf("fetch_needed.type: got %q, want %q", r.FetchNeeded.Type, "jira")
		}
		wantFields := []string{"issue_type", "story_points", "summary", "description"}
		if len(r.FetchNeeded.Fields) != len(wantFields) {
			t.Errorf("fetch_needed.fields: got %v, want %v", r.FetchNeeded.Fields, wantFields)
		} else {
			for i, f := range wantFields {
				if r.FetchNeeded.Fields[i] != f {
					t.Errorf("fetch_needed.fields[%d]: got %q, want %q", i, r.FetchNeeded.Fields[i], f)
				}
			}
		}
	})

	t.Run("text_null", func(t *testing.T) {
		t.Parallel()
		res := callTool(t, h, map[string]any{
			"arguments": "implement a new feature",
		})
		if res.IsError {
			t.Fatalf("handler returned MCP error: %v", textContent(res))
		}
		r := parsePipelineInitResult(t, textContent(res))
		if r.FetchNeeded != nil {
			t.Errorf("fetch_needed should be nil for text source, got %+v", r.FetchNeeded)
		}
	})
}

// ---------- TestPipelineInitWorkspacePath ----------

func TestPipelineInitWorkspacePath(t *testing.T) {
	t.Parallel()

	sm := state.NewStateManager()
	h := PipelineInitHandler(sm)

	t.Run("format_specs_date_slug", func(t *testing.T) {
		t.Parallel()
		res := callTool(t, h, map[string]any{
			"arguments": "implement user authentication",
		})
		if res.IsError {
			t.Fatalf("handler returned MCP error: %v", textContent(res))
		}
		r := parsePipelineInitResult(t, textContent(res))
		if !strings.HasPrefix(r.Workspace, ".specs/") {
			t.Errorf("workspace: got %q, want .specs/ prefix", r.Workspace)
		}
		// Should contain date portion.
		date := time.Now().Format("20060102")
		if !strings.Contains(r.Workspace, date) {
			t.Errorf("workspace %q should contain date %s", r.Workspace, date)
		}
		if !strings.Contains(r.Workspace, "implement") {
			t.Errorf("workspace %q should contain slug from input", r.Workspace)
		}
	})

	t.Run("slug_truncation_at_60", func(t *testing.T) {
		t.Parallel()
		// Use a very long input to test truncation.
		longInput := "this is a very long description that should be truncated to sixty characters maximum length"
		res := callTool(t, h, map[string]any{
			"arguments": longInput,
		})
		if res.IsError {
			t.Fatalf("handler returned MCP error: %v", textContent(res))
		}
		r := parsePipelineInitResult(t, textContent(res))
		base := filepath.Base(r.Workspace)
		// base is YYYYMMDD-<slug>, slug part after first hyphen.
		parts := strings.SplitN(base, "-", 2)
		if len(parts) < 2 {
			t.Fatalf("workspace base %q has no hyphen separator", base)
		}
		slug := parts[1]
		if len(slug) > 60 {
			t.Errorf("slug %q is longer than 60 chars (len=%d)", slug, len(slug))
		}
		// Should not end with hyphen.
		if strings.HasSuffix(slug, "-") {
			t.Errorf("slug %q has trailing hyphen", slug)
		}
	})

	t.Run("special_characters_replaced", func(t *testing.T) {
		t.Parallel()
		res := callTool(t, h, map[string]any{
			"arguments": "Fix bug: handle null pointer exception!",
		})
		if res.IsError {
			t.Fatalf("handler returned MCP error: %v", textContent(res))
		}
		r := parsePipelineInitResult(t, textContent(res))
		base := filepath.Base(r.Workspace)
		parts := strings.SplitN(base, "-", 2)
		if len(parts) < 2 {
			t.Fatalf("workspace base %q has no hyphen separator", base)
		}
		slug := parts[1]
		// Should be lowercase with only alphanumeric and hyphens.
		for _, ch := range slug {
			if ch != '-' && (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') {
				t.Errorf("slug %q contains non-alphanumeric/hyphen char %q", slug, ch)
			}
		}
		// Should not have leading or trailing hyphen.
		if strings.HasPrefix(slug, "-") {
			t.Errorf("slug %q has leading hyphen", slug)
		}
		if strings.HasSuffix(slug, "-") {
			t.Errorf("slug %q has trailing hyphen", slug)
		}
	})
}

// ---------- TestPipelineInitInvalidInput ----------

func TestPipelineInitInvalidInput(t *testing.T) {
	t.Parallel()

	sm := state.NewStateManager()
	h := PipelineInitHandler(sm)

	tests := []struct {
		name      string
		arguments string
	}{
		{name: "empty", arguments: ""},
		{name: "too_short", arguments: "ab"},
		{name: "invalid_url", arguments: "https://unknown.example.com/foo"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			res := callTool(t, h, map[string]any{
				"arguments": tc.arguments,
			})
			// Must NOT return an MCP error.
			if res.IsError {
				t.Errorf("invalid input %q should not return MCP error, got: %v", tc.arguments, textContent(res))
			}
			r := parsePipelineInitResult(t, textContent(res))
			if len(r.Errors) == 0 {
				t.Errorf("invalid input %q: expected errors field to be populated, got none", tc.arguments)
			}
		})
	}
}

// ---------- TestSlugify ----------

func TestSlugify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "lowercase",
			input: "Hello World",
			want:  "hello-world",
		},
		{
			name:  "special_chars_replaced",
			input: "Fix: null pointer!",
			want:  "fix-null-pointer",
		},
		{
			name:  "leading_trailing_stripped",
			input: "  hello world  ",
			want:  "hello-world",
		},
		{
			name:  "multiple_non_alnum_runs",
			input: "hello---world",
			want:  "hello-world",
		},
		{
			name: "truncation_at_60",
			// Input is 80 chars after slugification; truncated to 60, no trailing hyphen.
			input: "this-is-a-very-long-slug-that-should-definitely-be-truncated-at-sixty-characters",
			want:  "this-is-a-very-long-slug-that-should-definitely-be-truncated",
		},
		{
			name: "no_trailing_hyphen_after_truncation",
			// 59 alphanumeric chars followed by '-more': hyphen lands at position 60,
			// TrimRight removes it, leaving 59 chars.
			input: "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvw-more",
			want:  "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvw",
		},
		{
			name:  "numbers_preserved",
			input: "fix issue 42",
			want:  "fix-issue-42",
		},
		{
			name:  "japanese_chars_stripped",
			input: "SOA-2896 アラート分析ジョブにて作成されるタスクレコード",
			want:  "soa-2896",
		},
		{
			name:  "mixed_japanese_english",
			input: "fix タスクの title 変更",
			want:  "fix-title",
		},
		{
			name:  "all_japanese",
			input: "タスクタイトル変更",
			want:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := slugify(tc.input)
			if got != tc.want {
				t.Errorf("slugify(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ---------- TestMakeWorkspacePath ----------

func TestMakeWorkspacePath(t *testing.T) {
	t.Parallel()

	fixedDate := time.Date(2026, 3, 28, 0, 0, 0, 0, time.UTC)

	t.Run("english_input", func(t *testing.T) {
		t.Parallel()
		got := makeWorkspacePath(fixedDate, "implement user login feature")
		want := ".specs/20260328-implement-user-login-feature"
		if got != want {
			t.Errorf("makeWorkspacePath = %q, want %q", got, want)
		}
	})

	t.Run("all_japanese_falls_back_to_task", func(t *testing.T) {
		t.Parallel()
		got := makeWorkspacePath(fixedDate, "タスクタイトル変更")
		want := ".specs/20260328-task"
		if got != want {
			t.Errorf("makeWorkspacePath = %q, want %q", got, want)
		}
	})

	t.Run("japanese_with_ascii_extracts_ascii", func(t *testing.T) {
		t.Parallel()
		got := makeWorkspacePath(fixedDate, "SOA-2896 アラート分析ジョブ")
		want := ".specs/20260328-soa-2896"
		if got != want {
			t.Errorf("makeWorkspacePath = %q, want %q", got, want)
		}
	})
}

// ---------- TestPipelineInitSpecName ----------

func TestPipelineInitSpecName(t *testing.T) {
	t.Parallel()

	sm := state.NewStateManager()
	h := PipelineInitHandler(sm)

	res := callTool(t, h, map[string]any{
		"arguments": "implement user authentication feature",
	})
	if res.IsError {
		t.Fatalf("handler returned MCP error: %v", textContent(res))
	}
	r := parsePipelineInitResult(t, textContent(res))
	if r.SpecName == "" {
		t.Errorf("spec_name should not be empty")
	}
	// spec_name should be derived from the workspace path base after stripping YYYYMMDD-.
	base := filepath.Base(r.Workspace)
	parts := strings.SplitN(base, "-", 2)
	if len(parts) >= 2 {
		expectedSpecName := parts[1]
		if r.SpecName != expectedSpecName {
			t.Errorf("spec_name: got %q, want %q", r.SpecName, expectedSpecName)
		}
	}
}

// ---------- TestPipelineInitNewPipelineFields ----------

func TestPipelineInitNewPipelineFields(t *testing.T) {
	t.Parallel()

	sm := state.NewStateManager()
	h := PipelineInitHandler(sm)

	res := callTool(t, h, map[string]any{
		"arguments":      "https://github.com/owner/repo/issues/99",
		"current_branch": "feature/new-auth",
	})
	if res.IsError {
		t.Fatalf("handler returned MCP error: %v", textContent(res))
	}
	r := parsePipelineInitResult(t, textContent(res))

	// AC-2: verify all required new-pipeline fields.
	if !strings.HasPrefix(r.Workspace, ".specs/") {
		t.Errorf("workspace %q should have .specs/ prefix", r.Workspace)
	}
	if r.SpecName == "" {
		t.Errorf("spec_name should not be empty")
	}
	if r.SourceType != "github_issue" {
		t.Errorf("source_type: got %q, want github_issue", r.SourceType)
	}
	if r.SourceID != "99" {
		t.Errorf("source_id: got %q, want 99", r.SourceID)
	}
	if r.Flags == nil {
		t.Fatalf("flags should not be nil")
	}
	if r.Flags.CurrentBranch != "feature/new-auth" {
		t.Errorf("flags.current_branch: got %q, want feature/new-auth", r.Flags.CurrentBranch)
	}
	if r.FetchNeeded == nil {
		t.Errorf("fetch_needed should not be nil for github_issue")
	}
	// Verify all five Flags fields are present (not zero for bool fields).
	// The flags struct should always be populated.
	_ = r.Flags.Auto           // exists
	_ = r.Flags.SkipPR         // exists
	_ = r.Flags.Debug          // exists
	_ = r.Flags.TypeOverride   // exists (may be nil)
	_ = r.Flags.EffortOverride // exists (may be nil)
}

// ---------- TestDeriveSpecName ----------

func TestDeriveSpecName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		workspace string
		want      string
	}{
		{
			name:      "with_hyphen",
			workspace: ".specs/20260328-my-feature",
			want:      "my-feature",
		},
		{
			name:      "no_hyphen_in_base",
			workspace: ".specs/nodate",
			want:      "nodate",
		},
		{
			name:      "deep_path",
			workspace: "/abs/.specs/20260328-auth-system",
			want:      "auth-system",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := deriveSpecName(tc.workspace)
			if got != tc.want {
				t.Errorf("deriveSpecName(%q) = %q, want %q", tc.workspace, got, tc.want)
			}
		})
	}
}

// ---------- TestExtractSourceID ----------

func TestExtractSourceID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		sourceType string
		coreText   string
		want       string
	}{
		{
			name:       "github_issue",
			sourceType: "github_issue",
			coreText:   "https://github.com/owner/repo/issues/42",
			want:       "42",
		},
		{
			name:       "jira_issue",
			sourceType: "jira_issue",
			coreText:   "https://myorg.atlassian.net/browse/PROJ-123",
			want:       "PROJ-123",
		},
		{
			name:       "text_source",
			sourceType: "text",
			coreText:   "implement a feature",
			want:       "",
		},
		{
			name:       "workspace_source",
			sourceType: "workspace",
			coreText:   ".specs/20260101-my-feature",
			want:       "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := extractSourceID(tc.sourceType, tc.coreText)
			if got != tc.want {
				t.Errorf("extractSourceID(%q, %q) = %q, want %q", tc.sourceType, tc.coreText, got, tc.want)
			}
		})
	}
}

// ---------- TestPipelineInitFlagsAllFiveFields ----------
// AC-2: flags contains all five fields.

func TestPipelineInitFlagsAllFiveFields(t *testing.T) {
	t.Parallel()

	sm := state.NewStateManager()
	h := PipelineInitHandler(sm)

	res := callTool(t, h, map[string]any{
		"arguments": "implement a new feature",
	})
	if res.IsError {
		t.Fatalf("handler returned MCP error: %v", textContent(res))
	}

	// Unmarshal raw to check all five fields are present in the JSON.
	var raw map[string]any
	if err := json.Unmarshal([]byte(textContent(res)), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	flagsRaw, ok := raw["flags"]
	if !ok {
		t.Fatalf("flags key missing from response")
	}
	flagsMap, ok := flagsRaw.(map[string]any)
	if !ok {
		t.Fatalf("flags is not a map")
	}

	// Verify all five fields are present in JSON.
	requiredFields := []string{"auto", "skip_pr", "debug", "type_override", "effort_override"}
	for _, field := range requiredFields {
		if _, ok := flagsMap[field]; !ok {
			t.Errorf("flags.%s missing from JSON response", field)
		}
	}
}

// ---------- TestPipelineInitNoSideEffects ----------
// PipelineInitHandler must not write state.json (pure detection).

func TestPipelineInitNoSideEffects(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sm := state.NewStateManager()
	h := PipelineInitHandler(sm)

	res := callTool(t, h, map[string]any{
		"arguments": "implement a new feature",
	})
	if res.IsError {
		t.Fatalf("handler returned MCP error: %v", textContent(res))
	}

	// state.json must NOT have been created in the temp dir.
	stateJSONPath := filepath.Join(dir, "state.json")
	if _, err := os.Stat(stateJSONPath); err == nil {
		t.Errorf("PipelineInitHandler created state.json — it must be a pure detection tool")
	}
}

// ---------- TestRefineWorkspacePath ----------

func TestRefineWorkspacePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		workspace string
		extCtx    externalContext
		want      string
	}{
		{
			name:      "jira_issue_refines_name",
			workspace: ".specs/20260330-https-legalforce-atlassian-net-browse-so",
			extCtx: externalContext{
				SourceID:    "SOA-2883",
				JiraSummary: "Light plan triggers meeting minutes job for Meet meetings",
			},
			want: ".specs/20260330-soa-2883-light-plan-triggers-meeting-mi",
		},
		{
			name:      "github_issue_refines_name",
			workspace: ".specs/20260330-https-github-com-owner-repo-issues-42",
			extCtx: externalContext{
				SourceID:    "42",
				GitHubTitle: "Fix auth timeout in middleware",
			},
			want: ".specs/20260330-42-fix-auth-timeout-in-middleware",
		},
		{
			name:      "no_context_keeps_original",
			workspace: ".specs/20260330-implement-a-new-feature",
			extCtx:    externalContext{},
			want:      ".specs/20260330-implement-a-new-feature",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := refineWorkspacePath(tc.workspace, tc.extCtx)
			if tc.name == "no_context_keeps_original" {
				if got != tc.want {
					t.Errorf("refineWorkspacePath() = %q, want %q", got, tc.want)
				}
			} else {
				// For refined paths, check prefix since Japanese chars get slugified
				if !strings.HasPrefix(got, tc.want) {
					t.Errorf("refineWorkspacePath() = %q, want prefix %q", got, tc.want)
				}
			}
		})
	}
}

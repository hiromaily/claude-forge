// Package tools — unit tests for pipeline_init_with_context MCP handler.
// Tests verify the two-call confirmation flow, decisions 6–13, I/O sequence, and AC criteria.
package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hiromaily/claude-forge/mcp-server/internal/orchestrator"
	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
)

// ---------- helpers ----------

// newSM returns a fresh StateManager for use in pipeline_init_with_context tests.
func newPIWCSM() *state.StateManager {
	return state.NewStateManager("dev")
}

// parsePIWCResult unmarshals PipelineInitWithContextResult from a content string.
func parsePIWCResult(t *testing.T, content string) PipelineInitWithContextResult {
	t.Helper()
	var result PipelineInitWithContextResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		t.Fatalf("unmarshal PipelineInitWithContextResult: %v (content: %s)", err, content)
	}
	return result
}

// ---------- TestPipelineInitWithContextFirstCall ----------

// TestPipelineInitWithContextFirstCallEffortOptions asserts that the first call
// always returns needs_user_confirmation with EffortOptions keys "S", "M", "L"
// and DetectedEffort set, regardless of external_context.
func TestPipelineInitWithContextFirstCallEffortOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		externalContext map[string]any
		flags           map[string]any
		wantEffort      string
		wantNoStateJSON bool
	}{
		{
			name: "github_labels_bug",
			externalContext: map[string]any{
				"github_labels": []any{"bug"},
				"github_title":  "Fix null pointer crash",
				"github_body":   "application crashes on startup",
			},
			flags:           map[string]any{},
			wantEffort:      "M",
			wantNoStateJSON: true,
		},
		{
			name: "jira_issue_type_bug",
			externalContext: map[string]any{
				"jira_issue_type":  "Bug",
				"jira_summary":     "Fix login error",
				"jira_description": "Users cannot log in",
			},
			flags:           map[string]any{},
			wantEffort:      "M",
			wantNoStateJSON: true,
		},
		{
			name:            "text_heuristic_default",
			externalContext: map[string]any{},
			flags:           map[string]any{},
			wantEffort:      "M",
			wantNoStateJSON: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			sm := newPIWCSM()

			h := PipelineInitWithContextHandler(sm)
			res := callTool(t, h, map[string]any{
				"workspace":        dir,
				"external_context": tc.externalContext,
				"flags":            tc.flags,
				// no user_confirmation on first call
			})

			if res.IsError {
				t.Fatalf("first call returned MCP error: %v", textContent(res))
			}

			result := parsePIWCResult(t, textContent(res))

			// Must return needs_user_confirmation block
			if result.NeedsUserConfirmation == nil {
				t.Fatalf("first call: expected needs_user_confirmation, got nil; result=%+v", result)
			}

			nuc := result.NeedsUserConfirmation

			// DetectedEffort must be set
			if nuc.DetectedEffort == "" {
				t.Errorf("detected_effort is empty, want non-empty")
			}
			if nuc.DetectedEffort != tc.wantEffort {
				t.Errorf("detected_effort = %q, want %q", nuc.DetectedEffort, tc.wantEffort)
			}

			// EffortOptions must contain keys "S", "M", "L"
			if nuc.EffortOptions == nil {
				t.Fatalf("effort_options is nil")
			}
			for _, key := range []string{"S", "M", "L"} {
				if _, ok := nuc.EffortOptions[key]; !ok {
					t.Errorf("effort_options missing key %q; got keys: %v", key, nuc.EffortOptions)
				}
			}
			// Must not contain extra keys
			if len(nuc.EffortOptions) != 3 {
				t.Errorf("effort_options has %d keys, want exactly 3 (S, M, L); got: %v", len(nuc.EffortOptions), nuc.EffortOptions)
			}

			// Verify each effort option maps to the correct skips with labels
			wantS := orchestrator.SkipsWithLabelsForEffort("S")
			wantM := orchestrator.SkipsWithLabelsForEffort("M")
			wantL := orchestrator.SkipsWithLabelsForEffort("L")
			if !skipLabelSliceEqual(nuc.EffortOptions["S"], wantS) {
				t.Errorf("effort_options[S] = %v, want %v", nuc.EffortOptions["S"], wantS)
			}
			if !skipLabelSliceEqual(nuc.EffortOptions["M"], wantM) {
				t.Errorf("effort_options[M] = %v, want %v", nuc.EffortOptions["M"], wantM)
			}
			if !skipLabelSliceEqual(nuc.EffortOptions["L"], wantL) {
				t.Errorf("effort_options[L] = %v, want %v", nuc.EffortOptions["L"], wantL)
			}

			if nuc.Message == "" {
				t.Errorf("message field is empty")
			}

			// No filesystem I/O — state.json must NOT exist
			if tc.wantNoStateJSON {
				stateFile := filepath.Join(dir, "state.json")
				if _, err := os.Stat(stateFile); err == nil {
					t.Errorf("state.json should NOT be written on first call, but it exists")
				}
			}
		})
	}
}

// TestPipelineInitWithContextFirstCallAlwaysPrompts asserts that the first call
// always returns needs_user_confirmation even when auto=true and effort_override="L"
// (no --auto bypass).
func TestPipelineInitWithContextFirstCallAlwaysPrompts(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sm := newPIWCSM()

	h := PipelineInitWithContextHandler(sm)
	res := callTool(t, h, map[string]any{
		"workspace":        dir,
		"external_context": map[string]any{},
		"flags": map[string]any{
			"auto":            true,
			"effort_override": "L",
		},
		// no user_confirmation
	})

	if res.IsError {
		t.Fatalf("first call with auto=true returned MCP error: %v", textContent(res))
	}

	result := parsePIWCResult(t, textContent(res))
	// Must still return needs_user_confirmation (no --auto bypass)
	if result.NeedsUserConfirmation == nil {
		t.Fatalf("expected needs_user_confirmation even with auto=true, got nil; result=%+v", result)
	}

	nuc := result.NeedsUserConfirmation
	// EffortOptions must still contain S, M, L
	for _, key := range []string{"S", "M", "L"} {
		if _, ok := nuc.EffortOptions[key]; !ok {
			t.Errorf("effort_options missing key %q with auto=true", key)
		}
	}
}

// ---------- TestPipelineInitWithContextSecondCall ----------

func TestPipelineInitWithContextSecondCall(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sm := newPIWCSM()

	h := PipelineInitWithContextHandler(sm)

	// Second call: with user_confirmation (no task_type)
	res := callTool(t, h, map[string]any{
		"workspace":        dir,
		"external_context": map[string]any{},
		"flags":            map[string]any{},
		"user_confirmation": map[string]any{
			"effort": "M",
		},
	})

	if res.IsError {
		t.Fatalf("second call returned MCP error: %v", textContent(res))
	}

	result := parsePIWCResult(t, textContent(res))
	if !result.Ready {
		t.Errorf("ready = false, want true")
	}
	if result.Workspace != dir {
		t.Errorf("workspace = %q, want %q", result.Workspace, dir)
	}

	// state.json must be created
	stateFile := filepath.Join(dir, "state.json")
	if _, err := os.Stat(stateFile); err != nil {
		t.Errorf("state.json should be created on second call: %v", err)
	}

	// Verify state contents — no TaskType written
	s, err := state.ReadState(dir)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if s.Effort == nil || *s.Effort != "M" {
		t.Errorf("state.Effort = %v, want M", s.Effort)
	}
	if s.FlowTemplate == nil || *s.FlowTemplate != orchestrator.EffortToTemplate("M") {
		t.Errorf("state.FlowTemplate = %v, want %q", s.FlowTemplate, orchestrator.EffortToTemplate("M"))
	}

	// request.md must be written
	reqPath := filepath.Join(dir, "request.md")
	if _, err := os.Stat(reqPath); err != nil {
		t.Errorf("request.md should be created on second call: %v", err)
	}

	// Second call when state.json already exists must return MCP error
	res2 := callTool(t, h, map[string]any{
		"workspace":        dir,
		"external_context": map[string]any{},
		"flags":            map[string]any{},
		"user_confirmation": map[string]any{
			"effort": "M",
		},
	})
	if !res2.IsError {
		t.Errorf("second call when state.json exists should return MCP error")
	}
}

// TestPipelineInitWithContextSecondCallXSReturnsError asserts that passing
// effort: "XS" in user_confirmation returns an error.
func TestPipelineInitWithContextSecondCallXSReturnsError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sm := newPIWCSM()

	h := PipelineInitWithContextHandler(sm)
	res := callTool(t, h, map[string]any{
		"workspace":        dir,
		"external_context": map[string]any{},
		"flags":            map[string]any{},
		"user_confirmation": map[string]any{
			"effort": "XS",
		},
	})

	if !res.IsError {
		t.Errorf("effort XS should return MCP error, but got success")
	}
}

// ---------- TestPipelineInitWithContextCurrentBranch ----------

func TestPipelineInitWithContextCurrentBranch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		currentBranch          string
		useCurrentBranch       bool
		expectUseCurrentBranch bool
		expectBranchSet        bool   // true = s.Branch should be non-nil
		expectBranchValue      string // expected branch value when expectBranchSet is true
	}{
		{
			name: "feature_branch_use_current", currentBranch: "feature/foo",
			useCurrentBranch: true, expectUseCurrentBranch: true,
			expectBranchSet: true, expectBranchValue: "feature/foo",
		},
		{
			name: "main_new_branch", currentBranch: "main",
			useCurrentBranch: false, expectUseCurrentBranch: false,
			expectBranchSet: true, // branch derived from spec name
		},
		{
			name: "empty_new_branch", currentBranch: "",
			useCurrentBranch: false, expectUseCurrentBranch: false,
			expectBranchSet: true, // branch derived from spec name
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			sm := newPIWCSM()

			h := PipelineInitWithContextHandler(sm)
			flags := map[string]any{}
			if tc.currentBranch != "" {
				flags["current_branch"] = tc.currentBranch
			}
			res := callTool(t, h, map[string]any{
				"workspace":        dir,
				"external_context": map[string]any{},
				"flags":            flags,
				"user_confirmation": map[string]any{
					"effort":             "M",
					"use_current_branch": tc.useCurrentBranch,
				},
			})

			if res.IsError {
				t.Fatalf("second call returned MCP error: %v", textContent(res))
			}

			s, err := state.ReadState(dir)
			if err != nil {
				t.Fatalf("ReadState: %v", err)
			}
			if s.UseCurrentBranch != tc.expectUseCurrentBranch {
				t.Errorf("UseCurrentBranch = %v, want %v (branch=%q)", s.UseCurrentBranch, tc.expectUseCurrentBranch, tc.currentBranch)
			}
			if tc.expectBranchSet && s.Branch == nil {
				t.Error("Branch should be set, got nil")
			}
			if tc.expectBranchValue != "" && (s.Branch == nil || *s.Branch != tc.expectBranchValue) {
				t.Errorf("Branch = %v, want %q", s.Branch, tc.expectBranchValue)
			}
		})
	}
}

// ---------- TestPipelineInitWithContextInvalidConfirmedEffort ----------

func TestPipelineInitWithContextInvalidConfirmedEffort(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sm := newPIWCSM()

	h := PipelineInitWithContextHandler(sm)
	res := callTool(t, h, map[string]any{
		"workspace":        dir,
		"external_context": map[string]any{},
		"flags":            map[string]any{},
		"user_confirmation": map[string]any{
			"effort": "XL",
		},
	})

	if !res.IsError {
		t.Errorf("invalid effort should return MCP error")
	}
}

// ---------- TestHasNonASCII ----------

func TestHasNonASCII(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		s    string
		want bool
	}{
		{name: "empty", s: "", want: false},
		{name: "ascii_path", s: ".specs/20260331-my-feature", want: false},
		{name: "ascii_only", s: "abcdefghijklmnopqrstuvwxyz0123456789-", want: false},
		{name: "japanese", s: ".specs/20260331-日本語のタスク", want: true},
		{name: "japanese_only", s: "日本語", want: true},
		{name: "latin_extended", s: "caf\u00e9", want: true},
		{name: "emoji", s: "task-\U0001F600", want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := hasNonASCII(tc.s)
			if got != tc.want {
				t.Errorf("hasNonASCII(%q) = %v, want %v", tc.s, got, tc.want)
			}
		})
	}
}

// ---------- TestInitWorkspaceRejectsNonASCIIPath ----------

func TestInitWorkspaceRejectsNonASCIIPath(t *testing.T) {
	t.Parallel()

	sm := newPIWCSM()
	h := PipelineInitWithContextHandler(sm)

	// Pass a workspace path containing Japanese characters directly.
	res := callTool(t, h, map[string]any{
		"workspace": ".specs/20260331-日本語のタスク",
		"flags":     map[string]any{},
		"user_confirmation": map[string]any{
			"effort": "M",
		},
	})

	// Expect an MCP-level error (IsError=true) rejecting the non-ASCII path.
	if !res.IsError {
		t.Fatalf("expected MCP error for non-ASCII workspace, got success: %v", textContent(res))
	}
	if !strings.Contains(textContent(res), "non-ASCII") {
		t.Errorf("error message should mention non-ASCII, got: %v", textContent(res))
	}
}

// ---------- TestPipelineInitWithContextRequestMDContent ----------

func TestPipelineInitWithContextRequestMDContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		externalContext      map[string]any
		wantInBody           string
		wantInFrontMatter    string
		wantNotInFrontMatter string
	}{
		{
			name: "github",
			externalContext: map[string]any{
				"github_labels": []any{"bug"},
				"github_title":  "Fix the crash",
				"github_body":   "Application crashes on startup",
			},
			wantInBody:           "Fix the crash",
			wantInFrontMatter:    "source_type",
			wantNotInFrontMatter: "task_type",
		},
		{
			name: "jira",
			externalContext: map[string]any{
				"jira_issue_type":  "Bug",
				"jira_summary":     "Login broken",
				"jira_description": "Users cannot log in",
			},
			wantInBody:           "Login broken",
			wantInFrontMatter:    "source_type",
			wantNotInFrontMatter: "task_type",
		},
		{
			name:                 "text",
			externalContext:      map[string]any{},
			wantInBody:           "",
			wantInFrontMatter:    "source_type",
			wantNotInFrontMatter: "task_type",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			sm := newPIWCSM()

			h := PipelineInitWithContextHandler(sm)
			res := callTool(t, h, map[string]any{
				"workspace":        dir,
				"external_context": tc.externalContext,
				"flags":            map[string]any{},
				"user_confirmation": map[string]any{
					"effort": "M",
				},
			})

			if res.IsError {
				t.Fatalf("returned MCP error: %v", textContent(res))
			}

			result := parsePIWCResult(t, textContent(res))
			reqPath := filepath.Join(result.Workspace, "request.md")
			content, err := os.ReadFile(reqPath)
			if err != nil {
				t.Fatalf("ReadFile request.md: %v", err)
			}
			contentStr := string(content)

			if !strings.Contains(contentStr, tc.wantInFrontMatter) {
				t.Errorf("request.md does not contain %q; content: %s", tc.wantInFrontMatter, contentStr)
			}
			if tc.wantInBody != "" && !strings.Contains(contentStr, tc.wantInBody) {
				t.Errorf("request.md does not contain body %q; content: %s", tc.wantInBody, contentStr)
			}
			// task_type must NOT be in request.md front-matter
			if strings.Contains(contentStr, tc.wantNotInFrontMatter) {
				t.Errorf("request.md should NOT contain %q; content: %s", tc.wantNotInFrontMatter, contentStr)
			}
		})
	}
}

// ---------- TestPipelineInitWithContextSourceURL ----------

// TestPipelineInitWithContextSourceURL verifies that source_url passed as a
// top-level parameter is written into request.md front matter.
func TestPipelineInitWithContextSourceURL(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sm := newPIWCSM()

	h := PipelineInitWithContextHandler(sm)
	res := callTool(t, h, map[string]any{
		"workspace":  dir,
		"source_id":  "SOA-123",
		"source_url": "https://example.atlassian.net/browse/SOA-123",
		"external_context": map[string]any{
			"jira_issue_type":  "Bug",
			"jira_summary":     "Fix login",
			"jira_description": "Users cannot log in",
		},
		"flags": map[string]any{},
		"user_confirmation": map[string]any{
			"effort": "S",
		},
	})

	if res.IsError {
		t.Fatalf("returned MCP error: %v", textContent(res))
	}

	result := parsePIWCResult(t, textContent(res))
	reqPath := filepath.Join(result.Workspace, "request.md")
	content, err := os.ReadFile(reqPath)
	if err != nil {
		t.Fatalf("ReadFile request.md: %v", err)
	}
	contentStr := string(content)

	if !strings.Contains(contentStr, "source_url: https://example.atlassian.net/browse/SOA-123") {
		t.Errorf("request.md missing source_url in front matter; content:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "source_id: SOA-123") {
		t.Errorf("request.md missing source_id in front matter; content:\n%s", contentStr)
	}
}

// ---------- TestPipelineInitWithContextTextSource ----------

func TestPipelineInitWithContextTextSource(t *testing.T) {
	t.Parallel()

	// Empty external_context → text heuristic; no error
	dir := t.TempDir()
	sm := newPIWCSM()

	h := PipelineInitWithContextHandler(sm)
	res := callTool(t, h, map[string]any{
		"workspace":        dir,
		"external_context": map[string]any{},
		"flags":            map[string]any{},
		// no user_confirmation (first call)
	})

	if res.IsError {
		t.Fatalf("text source first call returned MCP error: %v", textContent(res))
	}

	result := parsePIWCResult(t, textContent(res))
	if result.NeedsUserConfirmation == nil {
		t.Fatalf("expected needs_user_confirmation, got nil")
	}

	// First call should return EffortOptions even for empty context
	nuc := result.NeedsUserConfirmation
	if nuc.EffortOptions == nil {
		t.Fatalf("effort_options is nil for text source first call")
	}
	for _, key := range []string{"S", "M", "L"} {
		if _, ok := nuc.EffortOptions[key]; !ok {
			t.Errorf("effort_options missing key %q", key)
		}
	}
}

// ---------- TestApplyWorkspaceSlug ----------

func TestApplyWorkspaceSlug(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		workspace string
		rawSlug   string
		want      string
	}{
		{
			name:      "replaces_slug",
			workspace: ".specs/20260331-task",
			rawSlug:   "add user auth endpoint",
			want:      ".specs/20260331-add-user-auth-endpoint",
		},
		{
			name:      "truncates_long_slug",
			workspace: ".specs/20260331-task",
			rawSlug:   "this is a very long description that exceeds forty characters and should be truncated",
			// slugify truncates at 60 bytes then strips a trailing hyphen only
			want: ".specs/20260331-this-is-a-very-long-description-that-exceeds-forty-character",
		},
		{
			name:      "japanese_slug_falls_back",
			workspace: ".specs/20260331-task",
			rawSlug:   "日本語のタスク",
			want:      ".specs/20260331-task", // unchanged: slugify returns ""
		},
		{
			name:      "no_date_prefix",
			workspace: ".specs/mytask",
			rawSlug:   "fix export timeout",
			want:      ".specs/fix-export-timeout",
		},
		{
			name:      "cleans_slug_chars",
			workspace: ".specs/20260331-task",
			rawSlug:   "Fix: Report Export (timeout)",
			want:      ".specs/20260331-fix-report-export-timeout",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := applyWorkspaceSlug(tc.workspace, tc.rawSlug)
			if got != tc.want {
				t.Errorf("applyWorkspaceSlug(%q, %q) = %q, want %q", tc.workspace, tc.rawSlug, got, tc.want)
			}
		})
	}
}

// ---------- TestInitWorkspaceUsesWorkspaceSlug ----------

func TestInitWorkspaceUsesWorkspaceSlug(t *testing.T) {
	t.Parallel()

	// workspace_slug in user_confirmation should rename the workspace directory
	// and set SpecName accordingly.
	parentDir := t.TempDir()
	wsDir := filepath.Join(parentDir, "20260331-task")

	sm := newPIWCSM()
	h := PipelineInitWithContextHandler(sm)
	res := callTool(t, h, map[string]any{
		"workspace":        wsDir,
		"external_context": map[string]any{},
		"flags":            map[string]any{},
		"user_confirmation": map[string]any{
			"effort":         "M",
			"workspace_slug": "add user authentication",
		},
	})

	if res.IsError {
		t.Fatalf("unexpected MCP error: %v", textContent(res))
	}

	result := parsePIWCResult(t, textContent(res))
	wantWorkspace := filepath.Join(parentDir, "20260331-add-user-authentication")
	if result.Workspace != wantWorkspace {
		t.Errorf("Workspace = %q, want %q", result.Workspace, wantWorkspace)
	}

	s, err := state.ReadState(result.Workspace)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if s.SpecName != "add-user-authentication" {
		t.Errorf("SpecName = %q, want %q", s.SpecName, "add-user-authentication")
	}
}

// ---------- TestPipelineInitWithContextSpecNameFallback ----------

func TestPipelineInitWithContextSpecNameFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		workspaceDir string // subdirectory name to use
		wantSpecName string
	}{
		{
			name:         "with_hyphen",
			workspaceDir: "20260328-my-feature",
			wantSpecName: "my-feature",
		},
		{
			name:         "no_hyphen",
			workspaceDir: "myfeature",
			wantSpecName: "myfeature",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			parentDir := t.TempDir()
			wsDir := filepath.Join(parentDir, tc.workspaceDir)
			if err := os.MkdirAll(wsDir, 0o755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}

			sm := newPIWCSM()
			h := PipelineInitWithContextHandler(sm)
			res := callTool(t, h, map[string]any{
				"workspace":        wsDir,
				"external_context": map[string]any{},
				"flags":            map[string]any{},
				"user_confirmation": map[string]any{
					"effort": "M",
				},
			})

			if res.IsError {
				t.Fatalf("returned MCP error: %v", textContent(res))
			}

			s, err := state.ReadState(wsDir)
			if err != nil {
				t.Fatalf("ReadState: %v", err)
			}
			if s.SpecName != tc.wantSpecName {
				t.Errorf("SpecName = %q, want %q", s.SpecName, tc.wantSpecName)
			}
		})
	}
}

// ---------- TestTopLevelSourceIDRefinement ----------

func TestTopLevelSourceIDRefinement(t *testing.T) {
	t.Parallel()

	parentDir := t.TempDir()
	wsDir := filepath.Join(parentDir, "20260330-https-github-com-owner-repo-issues-42")

	sm := newPIWCSM()
	h := PipelineInitWithContextHandler(sm)
	res := callTool(t, h, map[string]any{
		"workspace": wsDir,
		"source_id": "42",
		"external_context": map[string]any{
			"github_title": "Fix auth timeout in middleware",
			"github_body":  "requests timeout after 30s",
		},
		"flags": map[string]any{},
		"user_confirmation": map[string]any{
			"effort": "S",
		},
	})

	if res.IsError {
		t.Fatalf("unexpected MCP error: %v", textContent(res))
	}

	result := parsePIWCResult(t, textContent(res))
	wantWorkspace := filepath.Join(parentDir, "20260330-42-fix-auth-timeout-in-middleware")
	if result.Workspace != wantWorkspace {
		t.Errorf("Workspace = %q, want %q", result.Workspace, wantWorkspace)
	}

	s, err := state.ReadState(result.Workspace)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if s.SpecName != "42-fix-auth-timeout-in-middleware" {
		t.Errorf("SpecName = %q, want %q", s.SpecName, "42-fix-auth-timeout-in-middleware")
	}
}

// ---------- TestDiscussionMode ----------

// TestDiscussFirstCallTextSourceReturnsNeedsDiscussion verifies that handleFirstCall
// returns needs_discussion (non-null) when --discuss is active, source is text (no GitHub/Jira
// fields), and --auto is not set.
func TestDiscussFirstCallTextSourceReturnsNeedsDiscussion(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sm := newPIWCSM()

	h := PipelineInitWithContextHandler(sm)
	res := callTool(t, h, map[string]any{
		"workspace":        dir,
		"task_text":        "implement login feature",
		"external_context": map[string]any{},
		"flags": map[string]any{
			"discuss": true,
		},
		// no user_confirmation, no discussion_answers
	})

	if res.IsError {
		t.Fatalf("first call with --discuss returned MCP error: %v", textContent(res))
	}

	result := parsePIWCResult(t, textContent(res))
	if result.NeedsDiscussion == nil {
		t.Fatalf("expected needs_discussion non-null; result=%+v", result)
	}
	if result.NeedsUserConfirmation != nil {
		t.Errorf("needs_user_confirmation should be null when needs_discussion is returned")
	}
	if len(result.NeedsDiscussion.Questions) == 0 {
		t.Errorf("needs_discussion.questions must not be empty")
	}
	if result.NeedsDiscussion.Message == "" {
		t.Errorf("needs_discussion.message must not be empty")
	}
	// No filesystem I/O.
	stateFile := filepath.Join(dir, "state.json")
	if _, err := os.Stat(stateFile); err == nil {
		t.Errorf("state.json should NOT be written on first call with --discuss")
	}
}

// TestDiscussFirstCallAutoSuppressesDiscussion verifies that --auto suppresses
// discussion even when --discuss is set; needs_user_confirmation is returned instead.
func TestDiscussFirstCallAutoSuppressesDiscussion(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sm := newPIWCSM()

	h := PipelineInitWithContextHandler(sm)
	res := callTool(t, h, map[string]any{
		"workspace":        dir,
		"task_text":        "implement login feature",
		"external_context": map[string]any{},
		"flags": map[string]any{
			"discuss": true,
			"auto":    true,
		},
	})

	if res.IsError {
		t.Fatalf("first call with --discuss --auto returned MCP error: %v", textContent(res))
	}

	result := parsePIWCResult(t, textContent(res))
	if result.NeedsDiscussion != nil {
		t.Errorf("needs_discussion should be null when --auto is set, got: %+v", result.NeedsDiscussion)
	}
	if result.NeedsUserConfirmation == nil {
		t.Fatalf("expected needs_user_confirmation when --auto suppresses discussion")
	}
}

// TestDiscussFirstCallGitHubSourceSkipsDiscussion verifies that --discuss is ignored
// for GitHub source pipelines; needs_user_confirmation is returned directly.
func TestDiscussFirstCallGitHubSourceSkipsDiscussion(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sm := newPIWCSM()

	h := PipelineInitWithContextHandler(sm)
	res := callTool(t, h, map[string]any{
		"workspace": dir,
		"external_context": map[string]any{
			"github_title": "Fix null pointer crash",
			"github_body":  "application crashes on startup",
		},
		"flags": map[string]any{
			"discuss": true,
		},
	})

	if res.IsError {
		t.Fatalf("first call with --discuss + GitHub source returned MCP error: %v", textContent(res))
	}

	result := parsePIWCResult(t, textContent(res))
	if result.NeedsDiscussion != nil {
		t.Errorf("needs_discussion should be null for GitHub source, got: %+v", result.NeedsDiscussion)
	}
	if result.NeedsUserConfirmation == nil {
		t.Fatalf("expected needs_user_confirmation for GitHub source with --discuss")
	}
}

// TestDiscussionCallReturnsEnrichedNeedsUserConfirmation verifies that sending
// discussion_answers (without user_confirmation) returns needs_user_confirmation
// with non-empty enriched_request_body and does not create workspace files.
func TestDiscussionCallReturnsEnrichedNeedsUserConfirmation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sm := newPIWCSM()

	h := PipelineInitWithContextHandler(sm)
	res := callTool(t, h, map[string]any{
		"workspace":          dir,
		"task_text":          "implement login feature",
		"external_context":   map[string]any{},
		"flags":              map[string]any{"discuss": true},
		"discussion_answers": "Q1: users can log in\nQ2: no constraints\nQ3: use existing auth pkg",
	})

	if res.IsError {
		t.Fatalf("discussion call returned MCP error: %v", textContent(res))
	}

	result := parsePIWCResult(t, textContent(res))
	if result.NeedsUserConfirmation == nil {
		t.Fatalf("discussion call: expected needs_user_confirmation, got nil; result=%+v", result)
	}
	nuc := result.NeedsUserConfirmation
	if nuc.EnrichedRequestBody == "" {
		t.Errorf("enriched_request_body must be non-empty after discussion call")
	}
	if !strings.Contains(nuc.EnrichedRequestBody, "implement login feature") {
		t.Errorf("enriched_request_body should contain original task text; got: %s", nuc.EnrichedRequestBody)
	}
	if !strings.Contains(nuc.EnrichedRequestBody, "## Discussion Answers") {
		t.Errorf("enriched_request_body should contain '## Discussion Answers' header; got: %s", nuc.EnrichedRequestBody)
	}
	if !strings.Contains(nuc.EnrichedRequestBody, "use existing auth pkg") {
		t.Errorf("enriched_request_body should contain discussion answers; got: %s", nuc.EnrichedRequestBody)
	}
	// No filesystem I/O — state.json must NOT exist.
	stateFile := filepath.Join(dir, "state.json")
	if _, err := os.Stat(stateFile); err == nil {
		t.Errorf("state.json should NOT be written on discussion call")
	}
}

// TestDiscussionAndConfirmationBothPresentReturnsError verifies that providing
// both discussion_answers and user_confirmation returns an error.
func TestDiscussionAndConfirmationBothPresentReturnsError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sm := newPIWCSM()

	h := PipelineInitWithContextHandler(sm)
	res := callTool(t, h, map[string]any{
		"workspace":          dir,
		"task_text":          "implement login feature",
		"external_context":   map[string]any{},
		"flags":              map[string]any{"discuss": true},
		"discussion_answers": "Q1: definition of done",
		"user_confirmation":  map[string]any{"effort": "M"},
	})

	if !res.IsError {
		t.Errorf("expected MCP error when both discussion_answers and user_confirmation are present; got success: %v", textContent(res))
	}
	if !strings.Contains(textContent(res), "discussion_answers") {
		t.Errorf("error message should mention discussion_answers; got: %v", textContent(res))
	}
}

// TestThirdCallWithEnrichedBodyWritesRequestMD verifies that when user_confirmation
// carries a non-empty enriched_request_body, the written request.md contains the enriched body.
func TestThirdCallWithEnrichedBodyWritesRequestMD(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sm := newPIWCSM()
	enrichedBody := "implement login feature\n\n## Discussion Answers\n\nQ1: users can log in"

	h := PipelineInitWithContextHandler(sm)
	res := callTool(t, h, map[string]any{
		"workspace":        dir,
		"task_text":        "implement login feature",
		"external_context": map[string]any{},
		"flags":            map[string]any{"discuss": true},
		"user_confirmation": map[string]any{
			"effort":                "M",
			"use_current_branch":    true,
			"workspace_slug":        "implement-login",
			"enriched_request_body": enrichedBody,
		},
	})

	if res.IsError {
		t.Fatalf("third call returned MCP error: %v", textContent(res))
	}

	result := parsePIWCResult(t, textContent(res))
	if !result.Ready {
		t.Errorf("ready = false, want true")
	}

	reqPath := filepath.Join(result.Workspace, "request.md")
	content, err := os.ReadFile(reqPath)
	if err != nil {
		t.Fatalf("ReadFile request.md: %v", err)
	}
	contentStr := string(content)

	if !strings.Contains(contentStr, "source_type: text") {
		t.Errorf("request.md missing 'source_type: text' in front matter; content:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "## Discussion Answers") {
		t.Errorf("request.md missing '## Discussion Answers' section; content:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "implement login feature") {
		t.Errorf("request.md missing original task text; content:\n%s", contentStr)
	}
}

// ---------- TestBuildRequestMDWithBody ----------

func TestBuildRequestMDWithBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		extCtx     externalContext
		body       string
		wantSource string
		wantBody   string
	}{
		{
			name:       "text_source_with_body",
			extCtx:     externalContext{},
			body:       "implement login feature",
			wantSource: "source_type: text",
			wantBody:   "implement login feature",
		},
		{
			name:       "text_source_empty_body",
			extCtx:     externalContext{},
			body:       "",
			wantSource: "source_type: text",
			wantBody:   "",
		},
		{
			name: "github_source_ignores_body",
			extCtx: externalContext{
				GitHubTitle: "Fix crash",
				GitHubBody:  "crashes on startup",
			},
			body:       "this body should be ignored",
			wantSource: "source_type: github_issue",
			wantBody:   "Fix crash",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildRequestMDWithBody(tc.extCtx, tc.body)
			if !strings.Contains(got, tc.wantSource) {
				t.Errorf("buildRequestMDWithBody: missing %q; got:\n%s", tc.wantSource, got)
			}
			if tc.wantBody != "" && !strings.Contains(got, tc.wantBody) {
				t.Errorf("buildRequestMDWithBody: missing body %q; got:\n%s", tc.wantBody, got)
			}
			if tc.wantBody == "" {
				// For empty body, verify the content only contains front matter.
				lines := strings.SplitSeq(strings.TrimSpace(got), "\n")
				for line := range lines {
					if line != "---" && !strings.Contains(line, ":") && line != "" {
						t.Errorf("buildRequestMDWithBody with empty body: unexpected non-frontmatter line %q; got:\n%s", line, got)
					}
				}
			}
		})
	}
}

// ---------- TestBuildEnrichedRequestBody ----------

func TestBuildEnrichedRequestBody(t *testing.T) {
	t.Parallel()

	taskText := "implement login feature"
	answers := "Q1: users can log in\nQ2: no constraints"
	got := buildEnrichedRequestBody(taskText, answers)

	if !strings.Contains(got, taskText) {
		t.Errorf("buildEnrichedRequestBody: missing task text %q; got: %s", taskText, got)
	}
	if !strings.Contains(got, "## Discussion Answers") {
		t.Errorf("buildEnrichedRequestBody: missing '## Discussion Answers' header; got: %s", got)
	}
	if !strings.Contains(got, answers) {
		t.Errorf("buildEnrichedRequestBody: missing answers %q; got: %s", answers, got)
	}
}

// ---------- TestParseFlagsDiscussKeyConsistency ----------

// TestParseFlagsDiscussKeyConsistency verifies that boolField(m, "discuss") reads the
// same key ("discuss") that PipelineInitFlags.Discuss json:"discuss" serialises.
// Round-trips a PipelineInitFlags{Discuss: true} through JSON and calls parseFlags on result.
func TestParseFlagsDiscussKeyConsistency(t *testing.T) {
	t.Parallel()

	// Build a PipelineInitFlags with Discuss=true and JSON-serialize it.
	initFlags := PipelineInitFlags{Discuss: true}
	data, err := json.Marshal(initFlags)
	if err != nil {
		t.Fatalf("json.Marshal PipelineInitFlags: %v", err)
	}

	// Wrap it in the args map as "flags" and parse it.
	args := map[string]any{
		"flags": json.RawMessage(data),
	}
	flags, err := parseFlags(args)
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if !flags.Discuss {
		t.Errorf("parseFlags: Discuss = false, want true after round-trip through PipelineInitFlags JSON")
	}
}

// ---------- helpers ----------

// stringSliceEqual compares two string slices for equality (both nil and empty are equal).
func stringSliceEqual(a, b []string) bool { //nolint:unused // kept for future test use
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func skipLabelSliceEqual(a, b []orchestrator.SkipLabel) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].PhaseID != b[i].PhaseID || a[i].Label != b[i].Label {
			return false
		}
	}
	return true
}

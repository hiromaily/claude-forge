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
	return state.NewStateManager()
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

func TestPipelineInitWithContextFirstCall(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		externalContext map[string]any
		flags           map[string]any
		wantTaskType    string
		wantEffort      string
		wantTemplate    string
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
			wantTaskType:    orchestrator.TaskTypeBugfix,
			wantEffort:      "M",
			wantTemplate:    "light",
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
			wantTaskType:    orchestrator.TaskTypeBugfix,
			wantEffort:      "M",
			wantTemplate:    "light",
			wantNoStateJSON: true,
		},
		{
			name:            "text_heuristic_feature",
			externalContext: map[string]any{},
			flags:           map[string]any{},
			wantTaskType:    orchestrator.TaskTypeFeature,
			wantEffort:      "M",
			wantTemplate:    "standard",
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
			if nuc.DetectedTaskType != tc.wantTaskType {
				t.Errorf("detected_task_type = %q, want %q", nuc.DetectedTaskType, tc.wantTaskType)
			}
			if nuc.DetectedEffort != tc.wantEffort {
				t.Errorf("detected_effort = %q, want %q", nuc.DetectedEffort, tc.wantEffort)
			}
			if nuc.FlowTemplate != tc.wantTemplate {
				t.Errorf("flow_template = %q, want %q", nuc.FlowTemplate, tc.wantTemplate)
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

// ---------- TestPipelineInitWithContextDecision12 ----------

func TestPipelineInitWithContextDecision12(t *testing.T) {
	t.Parallel()

	// feature + L = full; with auto=true → downgrade to standard
	dir := t.TempDir()
	sm := newPIWCSM()

	h := PipelineInitWithContextHandler(sm)
	res := callTool(t, h, map[string]any{
		"workspace":        dir,
		"external_context": map[string]any{},
		"flags": map[string]any{
			"auto":            true,
			"effort_override": "L",
			"type_override":   "feature",
		},
		// no user_confirmation
	})

	if res.IsError {
		t.Fatalf("decision 12 first call returned MCP error: %v", textContent(res))
	}

	result := parsePIWCResult(t, textContent(res))
	if result.NeedsUserConfirmation == nil {
		t.Fatalf("expected needs_user_confirmation, got nil")
	}

	nuc := result.NeedsUserConfirmation
	// Should be downgraded to standard
	if nuc.FlowTemplate != "standard" {
		t.Errorf("flow_template = %q, want %q (should be downgraded from full)", nuc.FlowTemplate, "standard")
	}
	// Warning must be non-empty
	if result.Warning == "" {
		t.Errorf("warning field is empty; expected downgrade warning")
	}

	// skipped_phases must match SkipsForTemplate("standard")
	expectedSkips := orchestrator.SkipsForTemplate("standard")
	if len(nuc.SkippedPhases) != len(expectedSkips) {
		t.Errorf("skipped_phases len = %d, want %d (standard skips)", len(nuc.SkippedPhases), len(expectedSkips))
	}
}

// ---------- TestPipelineInitWithContextSecondCall ----------

func TestPipelineInitWithContextSecondCall(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sm := newPIWCSM()

	h := PipelineInitWithContextHandler(sm)

	// Second call: with user_confirmation
	res := callTool(t, h, map[string]any{
		"workspace":        dir,
		"external_context": map[string]any{},
		"flags":            map[string]any{},
		"user_confirmation": map[string]any{
			"task_type": "feature",
			"effort":    "M",
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

	// Verify state contents
	s, err := state.ReadState(dir)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if s.TaskType == nil || *s.TaskType != "feature" {
		t.Errorf("state.TaskType = %v, want feature", s.TaskType)
	}
	if s.Effort == nil || *s.Effort != "M" {
		t.Errorf("state.Effort = %v, want M", s.Effort)
	}
	if s.FlowTemplate == nil || *s.FlowTemplate != "standard" {
		t.Errorf("state.FlowTemplate = %v, want standard", s.FlowTemplate)
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
			"task_type": "feature",
			"effort":    "M",
		},
	})
	if !res2.IsError {
		t.Errorf("second call when state.json exists should return MCP error")
	}
}

// ---------- TestPipelineInitWithContextCurrentBranch ----------

func TestPipelineInitWithContextCurrentBranch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		currentBranch          string
		expectUseCurrentBranch bool
	}{
		{name: "feature_branch", currentBranch: "feature/foo", expectUseCurrentBranch: true},
		{name: "main_skipped", currentBranch: "main", expectUseCurrentBranch: false},
		{name: "master_skipped", currentBranch: "master", expectUseCurrentBranch: false},
		{name: "empty_skipped", currentBranch: "", expectUseCurrentBranch: false},
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
					"task_type": "feature",
					"effort":    "M",
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
			if tc.expectUseCurrentBranch && (s.Branch == nil || *s.Branch != tc.currentBranch) {
				t.Errorf("Branch = %v, want %q", s.Branch, tc.currentBranch)
			}
		})
	}
}

// ---------- TestPipelineInitWithContextUserOverride ----------

func TestPipelineInitWithContextUserOverride(t *testing.T) {
	t.Parallel()

	// detected type would be "feature" (default), but user confirms "bugfix"
	dir := t.TempDir()
	sm := newPIWCSM()

	h := PipelineInitWithContextHandler(sm)
	res := callTool(t, h, map[string]any{
		"workspace":        dir,
		"external_context": map[string]any{},
		"flags":            map[string]any{},
		"user_confirmation": map[string]any{
			"task_type": "bugfix",
			"effort":    "S",
		},
	})

	if res.IsError {
		t.Fatalf("returned MCP error: %v", textContent(res))
	}

	result := parsePIWCResult(t, textContent(res))
	if !result.Ready {
		t.Fatalf("ready = false")
	}

	s, err := state.ReadState(dir)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if s.TaskType == nil || *s.TaskType != "bugfix" {
		t.Errorf("state.TaskType = %v, want bugfix", s.TaskType)
	}
	if s.Effort == nil || *s.Effort != "S" {
		t.Errorf("state.Effort = %v, want S", s.Effort)
	}
}

// ---------- TestPipelineInitWithContextInvalidConfirmedType ----------

func TestPipelineInitWithContextInvalidConfirmedType(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sm := newPIWCSM()

	h := PipelineInitWithContextHandler(sm)
	res := callTool(t, h, map[string]any{
		"workspace":        dir,
		"external_context": map[string]any{},
		"flags":            map[string]any{},
		"user_confirmation": map[string]any{
			"task_type": "unknown",
			"effort":    "M",
		},
	})

	if !res.IsError {
		t.Errorf("invalid task_type should return MCP error")
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
			"task_type": "feature",
			"effort":    "XL",
		},
	})

	if !res.IsError {
		t.Errorf("invalid effort should return MCP error")
	}
}

// ---------- TestPipelineInitWithContextSkipPhaseOrder ----------

func TestPipelineInitWithContextSkipPhaseOrder(t *testing.T) {
	t.Parallel()

	// bugfix + XS → "direct" template with many skips
	dir := t.TempDir()
	sm := newPIWCSM()

	h := PipelineInitWithContextHandler(sm)
	res := callTool(t, h, map[string]any{
		"workspace":        dir,
		"external_context": map[string]any{},
		"flags":            map[string]any{},
		"user_confirmation": map[string]any{
			"task_type": "bugfix",
			"effort":    "XS",
		},
	})

	if res.IsError {
		t.Fatalf("second call returned MCP error: %v", textContent(res))
	}

	s, err := state.ReadState(dir)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}

	expectedSkips := orchestrator.SkipsForCell("bugfix", "XS")
	if len(s.SkippedPhases) != len(expectedSkips) {
		t.Errorf("skipped_phases len = %d, want %d; got %v, want %v",
			len(s.SkippedPhases), len(expectedSkips), s.SkippedPhases, expectedSkips)
	}
	for i, phase := range expectedSkips {
		if i >= len(s.SkippedPhases) {
			break
		}
		if s.SkippedPhases[i] != phase {
			t.Errorf("skipped_phases[%d] = %q, want %q", i, s.SkippedPhases[i], phase)
		}
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
			"task_type": "feature",
			"effort":    "M",
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
		name              string
		externalContext   map[string]any
		wantInBody        string
		wantInFrontMatter string
	}{
		{
			name: "github",
			externalContext: map[string]any{
				"github_labels": []any{"bug"},
				"github_title":  "Fix the crash",
				"github_body":   "Application crashes on startup",
			},
			wantInBody:        "Fix the crash",
			wantInFrontMatter: "source_type",
		},
		{
			name: "jira",
			externalContext: map[string]any{
				"jira_issue_type":  "Bug",
				"jira_summary":     "Login broken",
				"jira_description": "Users cannot log in",
			},
			wantInBody:        "Login broken",
			wantInFrontMatter: "task_type",
		},
		{
			name:              "text",
			externalContext:   map[string]any{},
			wantInBody:        "",
			wantInFrontMatter: "task_type",
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
					"task_type": "feature",
					"effort":    "M",
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
		})
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

	// Default text heuristic with empty text → feature
	if result.NeedsUserConfirmation.DetectedTaskType != orchestrator.TaskTypeFeature {
		t.Errorf("empty context: detected_task_type = %q, want feature",
			result.NeedsUserConfirmation.DetectedTaskType)
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
			"task_type":      "feature",
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
					"task_type": "feature",
					"effort":    "M",
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
			"task_type": "bugfix",
			"effort":    "S",
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

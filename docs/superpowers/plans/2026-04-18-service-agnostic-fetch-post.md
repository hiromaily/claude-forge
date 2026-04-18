# Service-Agnostic fetch_needed / post-to-source Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make SKILL.md service-agnostic by returning structured fetch/post metadata from the Go MCP server, so new service integrations (GitHub, Jira, Linear, future services) require only Go code changes.

**Architecture:** Replace `FetchNeeded.Fields` with `MCPTool`/`Command`/`MCPParams`/`ResponseMapping`. Add `PostMethod` struct to `Action` for post-to-source checkpoint metadata. Update SKILL.md to use generic dispatch logic.

**Tech Stack:** Go 1.26, mcp-go library, Markdown (SKILL.md)

---

### Task 1: Add `PostMethod` struct to `orchestrator/actions.go`

**Files:**
- Modify: `poc/claude-forge/mcp-server/internal/engine/orchestrator/actions.go`

- [ ] **Step 1: Write the test**

No separate test needed — `PostMethod` is a data struct. It will be tested via Task 4 (engine tests).

- [ ] **Step 2: Add `PostMethod` struct and extend `Action`**

In `poc/claude-forge/mcp-server/internal/engine/orchestrator/actions.go`, add after the `Action` struct definition (after line 67):

```go
// PostMethod describes how to post a comment back to the source issue.
// Populated only for post-to-source checkpoint actions.
// Exactly one of MCPTool or Command must be set.
type PostMethod struct {
	MCPTool    string            `json:"mcp_tool,omitempty"`
	Command    string            `json:"command,omitempty"`
	MCPParams  map[string]string `json:"mcp_params,omitempty"`
	BodySource string            `json:"body_source"`
}
```

Add `PostMethod` field to the `Action` struct, after the `SummaryPath` field (line 66):

```go
	// post-to-source metadata — populated only for post-to-source checkpoints
	PostMethod *PostMethod `json:"post_method,omitempty"`
```

- [ ] **Step 3: Verify compilation**

Run: `cd poc/claude-forge/mcp-server && go build ./...`
Expected: success, no errors

- [ ] **Step 4: Commit**

```bash
git add poc/claude-forge/mcp-server/internal/engine/orchestrator/actions.go
git commit -m "feat(orchestrator): add PostMethod struct to Action for post-to-source metadata"
```

---

### Task 2: Restructure `FetchNeeded` in `pipeline_init.go`

**Files:**
- Modify: `poc/claude-forge/mcp-server/internal/handler/tools/pipeline_init.go`
- Modify: `poc/claude-forge/mcp-server/internal/handler/tools/pipeline_init_test.go`

- [ ] **Step 1: Update `FetchNeeded` struct**

In `poc/claude-forge/mcp-server/internal/handler/tools/pipeline_init.go`, replace the `FetchNeeded` struct (lines 71-75):

```go
// FetchNeeded describes the external data that must be fetched before
// pipeline_init_with_context can run decisions 6–13.
// Only populated for github_issue, jira_issue, and linear_issue source types.
// Exactly one of MCPTool or Command must be set.
type FetchNeeded struct {
	Type            string            `json:"type"`
	MCPTool         string            `json:"mcp_tool,omitempty"`
	Command         string            `json:"command,omitempty"`
	MCPParams       map[string]string `json:"mcp_params,omitempty"`
	ResponseMapping map[string]string `json:"response_mapping"`
	Instruction     string            `json:"instruction"`
}
```

- [ ] **Step 2: Update `makeFetchNeeded` signature and implementation**

Change the signature to accept `sourceURL` and `sourceID`:

```go
func makeFetchNeeded(sourceType, sourceURL, sourceID string) *FetchNeeded
```

Replace the body:

```go
func makeFetchNeeded(sourceType, sourceURL, sourceID string) *FetchNeeded {
	switch sourceType {
	case "github_issue":
		return &FetchNeeded{
			Type:    "github",
			Command: "gh issue view " + sourceURL + " --json title,body,labels",
			ResponseMapping: map[string]string{
				"title":  "github_title",
				"body":   "github_body",
				"labels": "github_labels",
			},
			Instruction: "fetch github issue fields before calling pipeline_init_with_context",
		}
	case "jira_issue":
		return &FetchNeeded{
			Type:    "jira",
			Command: "gh issue view is not available for Jira",
			ResponseMapping: map[string]string{
				"summary":      "jira_summary",
				"description":  "jira_description",
				"issue_type":   "jira_issue_type",
				"story_points": "jira_story_points",
			},
			Instruction: "fetch jira issue fields (summary, description, issuetype, story_points) before calling pipeline_init_with_context. Use Atlassian MCP tools if available, or Jira REST API with $JIRA_USER:$JIRA_TOKEN credentials.",
		}
	case "linear_issue":
		return &FetchNeeded{
			Type:    "linear",
			MCPTool: "mcp__linear__get_issue",
			MCPParams: map[string]string{
				"issueId": sourceID,
			},
			ResponseMapping: map[string]string{
				"title":       "linear_title",
				"description": "linear_description",
				"priority":    "linear_priority",
				"estimate":    "linear_estimate",
				"labels":      "linear_labels",
			},
			Instruction: "fetch linear issue fields before calling pipeline_init_with_context",
		}
	default:
		return nil
	}
}
```

Note: Jira's `Command` field is intentionally set to a non-executable placeholder. Jira fetching is complex (auth tokens, nested JSON, optional Atlassian MCP tools), so the `Instruction` field provides the real guidance. The orchestrator's generic logic tries `mcp_tool` first, then `command`, then falls back to `instruction`.

- [ ] **Step 3: Update the call site in `PipelineInitHandler`**

Change line 139 from:

```go
fetchNeeded := makeFetchNeeded(sourceType)
```

to:

```go
fetchNeeded := makeFetchNeeded(sourceType, sourceURL, sourceID)
```

But `sourceURL` is computed after `makeFetchNeeded` — move the `sourceURL` computation before `makeFetchNeeded`:

```go
// source_url is only meaningful for URL-based source types; omit for text/workspace.
var sourceURL string
if sourceType == "github_issue" || sourceType == "jira_issue" || sourceType == "linear_issue" {
	sourceURL = coreText
}

// Build fetch_needed block.
fetchNeeded := makeFetchNeeded(sourceType, sourceURL, sourceID)
```

Remove the old `sourceURL` assignment block that was below `makeFetchNeeded`.

- [ ] **Step 4: Update tests in `pipeline_init_test.go`**

In `TestPipelineInitFetchNeeded`, update the three existing test cases and the Linear test case. The assertions change from checking `Fields` to checking `ResponseMapping`, `MCPTool`/`Command`, and `MCPParams`.

For the `github_non_null` test case, replace the field assertions:

```go
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
	if r.FetchNeeded.Command == "" {
		t.Errorf("fetch_needed.command should be non-empty for github")
	}
	if r.FetchNeeded.ResponseMapping == nil {
		t.Fatalf("fetch_needed.response_mapping is nil")
	}
	if r.FetchNeeded.ResponseMapping["title"] != "github_title" {
		t.Errorf("response_mapping[title] = %q, want github_title", r.FetchNeeded.ResponseMapping["title"])
	}
})
```

For `jira_non_null_with_correct_fields`:

```go
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
	if r.FetchNeeded.ResponseMapping == nil {
		t.Fatalf("fetch_needed.response_mapping is nil")
	}
	if r.FetchNeeded.ResponseMapping["summary"] != "jira_summary" {
		t.Errorf("response_mapping[summary] = %q, want jira_summary", r.FetchNeeded.ResponseMapping["summary"])
	}
})
```

For `linear_non_null_with_correct_fields`:

```go
t.Run("linear_non_null_with_correct_fields", func(t *testing.T) {
	t.Parallel()
	res := callTool(t, h, map[string]any{
		"arguments": "https://linear.app/dealon/issue/DEA-13",
	})
	if res.IsError {
		t.Fatalf("handler returned MCP error: %v", textContent(res))
	}
	r := parsePipelineInitResult(t, textContent(res))
	if r.FetchNeeded == nil {
		t.Fatalf("fetch_needed is nil for linear_issue, want non-nil")
	}
	if r.FetchNeeded.Type != "linear" {
		t.Errorf("fetch_needed.type: got %q, want %q", r.FetchNeeded.Type, "linear")
	}
	if r.FetchNeeded.MCPTool != "mcp__linear__get_issue" {
		t.Errorf("fetch_needed.mcp_tool: got %q, want mcp__linear__get_issue", r.FetchNeeded.MCPTool)
	}
	if r.FetchNeeded.MCPParams == nil || r.FetchNeeded.MCPParams["issueId"] != "DEA-13" {
		t.Errorf("fetch_needed.mcp_params[issueId]: got %v, want DEA-13", r.FetchNeeded.MCPParams)
	}
	if r.FetchNeeded.ResponseMapping == nil {
		t.Fatalf("fetch_needed.response_mapping is nil")
	}
	if r.FetchNeeded.ResponseMapping["title"] != "linear_title" {
		t.Errorf("response_mapping[title] = %q, want linear_title", r.FetchNeeded.ResponseMapping["title"])
	}
})
```

The `text_null` test case remains unchanged.

- [ ] **Step 5: Run tests**

Run: `cd poc/claude-forge/mcp-server && go test ./internal/handler/tools/... -count=1 -run TestPipelineInit`
Expected: all pass

- [ ] **Step 6: Commit**

```bash
git add poc/claude-forge/mcp-server/internal/handler/tools/pipeline_init.go poc/claude-forge/mcp-server/internal/handler/tools/pipeline_init_test.go
git commit -m "feat(pipeline-init): restructure FetchNeeded with mcp_tool/command/response_mapping"
```

---

### Task 3: Update `handlePostToSource` in `engine.go`

**Files:**
- Modify: `poc/claude-forge/mcp-server/internal/engine/orchestrator/engine.go`

- [ ] **Step 1: Add `sourceTypeLabel` helper**

Add after the `handlePostToSource` function:

```go
// sourceTypeLabel returns a human-readable label for a source type.
func sourceTypeLabel(sourceType string) string {
	switch sourceType {
	case state.SourceTypeGitHub:
		return "GitHub issue"
	case state.SourceTypeJira:
		return "Jira issue"
	case state.SourceTypeLinear:
		return "Linear issue"
	default:
		return ""
	}
}
```

- [ ] **Step 2: Add `buildPostMethod` helper**

Add after `sourceTypeLabel`:

```go
// buildPostMethod constructs a PostMethod for the given source type.
// Returns nil for text sources or when sourceURL is empty.
func buildPostMethod(sourceType, sourceURL, sourceID, workspace string) *PostMethod {
	bodySource := filepath.Join(workspace, state.ArtifactSummary)

	switch sourceType {
	case state.SourceTypeGitHub:
		return &PostMethod{
			Command:    "gh issue comment " + sourceURL + " --body-file " + bodySource,
			BodySource: bodySource,
		}
	case state.SourceTypeJira:
		return &PostMethod{
			Command:    "echo 'Jira comment posting requires manual ADF conversion or Atlassian MCP tools'",
			BodySource: bodySource,
			Instruction: "Post the contents of " + bodySource + " as a comment to the Jira issue at " + sourceURL + ". Use Atlassian MCP tools if available, or convert the markdown to ADF and use the Jira REST API.",
		}
	case state.SourceTypeLinear:
		return &PostMethod{
			MCPTool:    "mcp__linear__save_comment",
			MCPParams:  map[string]string{"issueId": sourceID},
			BodySource: bodySource,
		}
	default:
		return nil
	}
}
```

Wait — the spec's `PostMethod` doesn't have an `Instruction` field. For Jira we need it. Let me add it to the struct in Task 1.

Actually, let me reconsider. The `Instruction` field is already on `FetchNeeded`. For `PostMethod` Jira case, the `command` field can be the fallback echo, and the orchestrator's generic logic will run it. The `Instruction` can be added to `PostMethod` for Jira's special needs.

Update the `PostMethod` struct definition in Task 1 to also include `Instruction`:

```go
type PostMethod struct {
	MCPTool     string            `json:"mcp_tool,omitempty"`
	Command     string            `json:"command,omitempty"`
	MCPParams   map[string]string `json:"mcp_params,omitempty"`
	BodySource  string            `json:"body_source"`
	Instruction string            `json:"instruction,omitempty"`
}
```

Now `buildPostMethod` for Jira:

```go
	case state.SourceTypeJira:
		return &PostMethod{
			BodySource:  bodySource,
			Instruction: "Post the contents of " + bodySource + " as a comment to " + sourceURL + ". Use Atlassian MCP tools if available, or convert the markdown to ADF and POST via Jira REST API with $JIRA_USER:$JIRA_TOKEN.",
		}
```

- [ ] **Step 3: Rewrite `handlePostToSource`**

Replace the entire function:

```go
func (e *Engine) handlePostToSource(st *state.State) (Action, error) {
	sourceType := e.sourceTypeReader(st.Workspace)
	sourceURL := e.sourceURLReader(st.Workspace)
	sourceID := e.sourceIDReader(st.Workspace)

	label := sourceTypeLabel(sourceType)
	if label == "" {
		return NewDoneAction(SkipSummaryPrefix+PhasePostToSource, ""), nil
	}

	if sourceURL == "" {
		return NewDoneAction(SkipSummaryPrefix+PhasePostToSource, ""), nil
	}

	pm := buildPostMethod(sourceType, sourceURL, sourceID, st.Workspace)

	msg := fmt.Sprintf(
		"Pipeline complete. Post the final summary as a comment to the %s?\n\nURL: %s\nSummary file: %s/%s",
		label, sourceURL, st.Workspace, state.ArtifactSummary,
	)

	action := NewCheckpointAction(PhasePostToSource, msg, []string{"post", "skip"})
	action.PostMethod = pm
	return action, nil
}
```

Remove the now-unused comment `// The source type (github/jira/linear) is embedded in the message, not the name.` and the old `switch` block.

- [ ] **Step 4: Add `filepath` import if not present**

`engine.go` already imports `path/filepath` — verify by checking existing imports.

- [ ] **Step 5: Verify compilation**

Run: `cd poc/claude-forge/mcp-server && go build ./...`
Expected: success

- [ ] **Step 6: Commit**

```bash
git add poc/claude-forge/mcp-server/internal/engine/orchestrator/engine.go
git commit -m "feat(engine): handlePostToSource returns PostMethod metadata in checkpoint action"
```

---

### Task 4: Update engine tests for `PostMethod`

**Files:**
- Modify: `poc/claude-forge/mcp-server/internal/engine/orchestrator/engine_test.go`

- [ ] **Step 1: Update `TestPostToSource_CheckpointOptions`**

Add a `sourceID` field to the test table struct and add a `stubSourceIDReader`. Add `PostMethod` assertions. Add a Linear test case.

Replace the test function:

```go
func TestPostToSource_CheckpointOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		sourceType   string
		sourceURL    string
		sourceID     string
		wantName     string
		wantURLInMsg string
		wantMCPTool  string
		wantCommand  string
	}{
		{
			name:         "github_issue",
			sourceType:   "github_issue",
			sourceURL:    "https://github.com/org/repo/issues/42",
			sourceID:     "42",
			wantName:     "post-to-source",
			wantURLInMsg: "https://github.com/org/repo/issues/42",
			wantCommand:  "gh issue comment",
		},
		{
			name:         "jira_issue",
			sourceType:   "jira_issue",
			sourceURL:    "https://example.atlassian.net/browse/PROJ-123",
			sourceID:     "PROJ-123",
			wantName:     "post-to-source",
			wantURLInMsg: "https://example.atlassian.net/browse/PROJ-123",
		},
		{
			name:         "linear_issue",
			sourceType:   "linear_issue",
			sourceURL:    "https://linear.app/dealon/issue/DEA-13",
			sourceID:     "DEA-13",
			wantName:     "post-to-source",
			wantURLInMsg: "https://linear.app/dealon/issue/DEA-13",
			wantMCPTool:  "mcp__linear__save_comment",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sm := newTestStateManager(t, "post-to-source", nil)
			eng := &Engine{
				agentDir:         "/test/agents",
				specsDir:         "/test/specs",
				verdictReader:    stubVerdictReader(VerdictApprove),
				sourceTypeReader: stubSourceTypeReader(tc.sourceType),
				sourceURLReader:  func(_ string) string { return tc.sourceURL },
				sourceIDReader:   func(_ string) string { return tc.sourceID },
			}

			action, err := eng.NextAction(sm, "")
			if err != nil {
				t.Fatalf("NextAction: %v", err)
			}

			if action.Type != ActionCheckpoint {
				t.Fatalf("action.Type = %q, want %q", action.Type, ActionCheckpoint)
			}
			if action.Name != tc.wantName {
				t.Errorf("action.Name = %q, want %q", action.Name, tc.wantName)
			}

			wantOptions := []string{"post", "skip"}
			if !slices.Equal(action.Options, wantOptions) {
				t.Errorf("action.Options = %v, want %v", action.Options, wantOptions)
			}
			if !strings.Contains(action.PresentToUser, tc.wantURLInMsg) {
				t.Errorf("action.PresentToUser does not contain URL %q: %q", tc.wantURLInMsg, action.PresentToUser)
			}

			// Verify PostMethod is present
			if action.PostMethod == nil {
				t.Fatalf("action.PostMethod is nil, want non-nil")
			}
			if tc.wantMCPTool != "" && action.PostMethod.MCPTool != tc.wantMCPTool {
				t.Errorf("PostMethod.MCPTool = %q, want %q", action.PostMethod.MCPTool, tc.wantMCPTool)
			}
			if tc.wantCommand != "" && !strings.Contains(action.PostMethod.Command, tc.wantCommand) {
				t.Errorf("PostMethod.Command = %q, want substring %q", action.PostMethod.Command, tc.wantCommand)
			}
			if action.PostMethod.BodySource == "" {
				t.Errorf("PostMethod.BodySource should not be empty")
			}
		})
	}
}
```

- [ ] **Step 2: Update existing `post_to_source_github_issue` test in the NextAction table**

In the big `TestNextAction` table test, the `post_to_source_github_issue` entry constructs an `Engine` without `sourceIDReader`. Add the field:

```go
sourceIDReader:   func(_ string) string { return "42" },
```

Do the same for `post_to_source_jira_issue` and any other post-to-source entries.

- [ ] **Step 3: Run tests**

Run: `cd poc/claude-forge/mcp-server && go test -race ./internal/engine/orchestrator/... -count=1`
Expected: all pass

- [ ] **Step 4: Commit**

```bash
git add poc/claude-forge/mcp-server/internal/engine/orchestrator/engine_test.go
git commit -m "test(engine): verify PostMethod in post-to-source checkpoint actions"
```

---

### Task 5: Update SKILL.md — generic fetch_needed logic

**Files:**
- Modify: `poc/claude-forge/skills/forge/SKILL.md`

- [ ] **Step 1: Replace fetch_needed section**

Replace lines 19-26 (the `fetch_needed` handling in Step 1.4a) with:

```markdown
   a. If `result.fetch_needed` is non-null:
      1. Fetch the external data using the method specified in `fetch_needed`:
         - If `fetch_needed.mcp_tool` is set: call the MCP tool with `fetch_needed.mcp_params`.
         - Else if `fetch_needed.command` is set: execute the command via Bash and parse the JSON output.
         - Else: follow `fetch_needed.instruction` as a fallback guide.
      2. Map the response fields to `external_context` using `fetch_needed.response_mapping`:
         for each entry `(response_key → context_key)`, set `external_context[context_key] = response[response_key]`.
      3. Call `mcp__forge-state__pipeline_init_with_context(workspace=result.workspace, source_id=result.source_id, source_url=result.source_url, flags=result.flags, external_context=<mapped data>)`.
         (`task_text` is not applicable for external issue sources — do not pass it.)
```

- [ ] **Step 2: Replace post-to-source section**

Replace lines 116-134 (the service-specific post-to-source logic) with:

```markdown
     - **Special: `post-to-source` checkpoint** — when `action.name`
       is `"post-to-source"`:
       1. Ask the user whether to post the work report (use AskUserQuestion
          with options "post" / "skip").
       2. If the user chooses **"post"** and `action.post_method` is present:
          a. Read the body content from `action.post_method.body_source`.
          b. Post the comment using the method specified in `post_method`:
             - If `post_method.mcp_tool` is set: call it with `post_method.mcp_params`
               and the body content (pass body as the `body` parameter).
             - Else if `post_method.command` is set: execute the command via Bash.
             - Else if `post_method.instruction` is set: follow the instruction as a fallback guide.
          c. Report success or failure to the user.
       3. If the user chooses **"skip"**: do nothing.
```

- [ ] **Step 3: Update the skill description line**

Line 3 currently says:

```
description: Orchestrate a full development pipeline using MCP-driven subagents. Accepts plain text, GitHub Issue URLs, or Jira Issue URLs as input.
```

Replace with:

```
description: Orchestrate a full development pipeline using MCP-driven subagents. Accepts plain text or issue tracker URLs (GitHub, Jira, Linear, etc.) as input.
```

- [ ] **Step 4: Commit**

```bash
git add poc/claude-forge/skills/forge/SKILL.md
git commit -m "feat(skill): replace service-specific fetch/post logic with generic dispatch"
```

---

### Task 6: Run full test suite and verify

**Files:** (no changes — verification only)

- [ ] **Step 1: Run full Go test suite**

Run: `cd poc/claude-forge/mcp-server && go test -race ./... -count=1`
Expected: all packages pass

- [ ] **Step 2: Run hook tests**

Run: `cd poc/claude-forge && bash scripts/test-hooks.sh`
Expected: all tests pass

- [ ] **Step 3: Verify no service-specific logic remains in SKILL.md**

Search for hardcoded service references in the fetch/post sections:

Run: `grep -n "gh issue comment\|mcp__linear__\|atlassian\|jira_issue_type\|github_title\|linear_title" poc/claude-forge/skills/forge/SKILL.md`
Expected: no matches in the fetch_needed or post-to-source sections (may appear in other unrelated sections)

- [ ] **Step 4: Update testing checklist**

In `poc/claude-forge/.claude/rules/testing.md` and `poc/claude-forge/template/sections/ai/rules/testing.md`, verify the `source_type` detection line already includes `linear_issue` (done in prior work).

- [ ] **Step 5: Commit any remaining fixes**

If any test failures were found and fixed, commit them.

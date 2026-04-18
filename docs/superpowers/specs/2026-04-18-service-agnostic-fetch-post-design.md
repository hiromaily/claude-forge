# Service-Agnostic fetch_needed / post-to-source Design

## Problem

Every time a new external service (GitHub, Jira, Linear, ...) is added, both the Go MCP server **and** SKILL.md must be updated. SKILL.md currently contains service-specific branching logic for:

1. **Fetching issue data** (`fetch_needed` handling) — which MCP tool or CLI command to call, how to map response fields to `external_context`
2. **Posting work reports** (`post-to-source` checkpoint handling) — which tool/command to call to post a comment back to the source

This couples the orchestrator (SKILL.md) to service-specific knowledge that belongs in the MCP server.

## Goal

Make SKILL.md service-agnostic. New service integrations require changes **only** in the Go MCP server code. SKILL.md uses generic logic to execute structured metadata returned by the server.

## Design

### 1. `FetchNeeded` — Structured Fetch Metadata

#### Current Shape

```go
type FetchNeeded struct {
    Type        string   `json:"type"`
    Fields      []string `json:"fields"`
    Instruction string   `json:"instruction"`
}
```

#### New Shape

```go
type FetchNeeded struct {
    Type            string            `json:"type"`
    MCPTool         string            `json:"mcp_tool,omitempty"`
    Command         string            `json:"command,omitempty"`
    MCPParams       map[string]string `json:"mcp_params,omitempty"`
    ResponseMapping map[string]string `json:"response_mapping"`
    Instruction     string            `json:"instruction"`
}
```

| Field | Purpose |
|-------|---------|
| `Type` | Service identifier (unchanged, used for logging/diagnostics) |
| `MCPTool` | MCP tool name to call (e.g., `mcp__linear__get_issue`). Mutually exclusive with `Command` |
| `Command` | Shell command to execute (e.g., `gh issue view {source_url} --json title,body,labels`). Mutually exclusive with `MCPTool` |
| `MCPParams` | Parameters for the MCP tool call |
| `ResponseMapping` | Maps response field names → `external_context` field names |
| `Instruction` | Human-readable fallback instruction (kept for debugging/logging) |

**Removed**: `Fields` — superseded by `ResponseMapping` keys.

#### Per-Service Examples

**GitHub** (CLI command):
```json
{
  "type": "github",
  "command": "gh issue view {source_url} --json title,body,labels",
  "response_mapping": {
    "title": "github_title",
    "body": "github_body",
    "labels": "github_labels"
  },
  "instruction": "fetch github issue fields before calling pipeline_init_with_context"
}
```

**Jira** (CLI command — no standard Jira MCP tool assumed):
```json
{
  "type": "jira",
  "command": "curl -s -u \"$JIRA_USER:$JIRA_TOKEN\" \"https://{jira_domain}/rest/api/3/issue/{source_id}?fields=summary,description,issuetype,story_points\"",
  "response_mapping": {
    "fields.summary": "jira_summary",
    "fields.description": "jira_description",
    "fields.issuetype.name": "jira_issue_type",
    "fields.story_points": "jira_story_points"
  },
  "instruction": "fetch jira issue fields before calling pipeline_init_with_context"
}
```

Note: Jira's `command` has template variables `{jira_domain}` and `{source_id}`. The MCP server extracts these from the URL during `pipeline_init`. However, Jira requires auth tokens and nested JSON extraction, making a CLI command fragile. In practice the orchestrator may still prefer Atlassian MCP tools if available — the `instruction` field serves as fallback guidance.

**Alternative for Jira**: Use `mcp_tool` if Atlassian MCP tools are installed:
```json
{
  "type": "jira",
  "mcp_tool": "mcp__atlassian__get_issue",
  "mcp_params": {"issueKey": "PROJ-123"},
  "response_mapping": { ... },
  "instruction": "..."
}
```

The MCP server can detect available MCP tools at runtime (or use a configuration flag) to choose the appropriate fetch method.

**Linear** (MCP tool):
```json
{
  "type": "linear",
  "mcp_tool": "mcp__linear__get_issue",
  "mcp_params": {"issueId": "DEA-13"},
  "response_mapping": {
    "title": "linear_title",
    "description": "linear_description",
    "priority": "linear_priority",
    "estimate": "linear_estimate",
    "labels": "linear_labels"
  },
  "instruction": "fetch linear issue fields before calling pipeline_init_with_context"
}
```

### 2. `PostMethod` — Structured Post Metadata

#### New Struct

```go
type PostMethod struct {
    MCPTool    string            `json:"mcp_tool,omitempty"`
    Command    string            `json:"command,omitempty"`
    MCPParams  map[string]string `json:"mcp_params,omitempty"`
    BodySource string            `json:"body_source"`
}
```

| Field | Purpose |
|-------|---------|
| `MCPTool` | MCP tool name to call for posting. Mutually exclusive with `Command` |
| `Command` | Shell command for posting. Template vars: `{source_url}`, `{body_source}` |
| `MCPParams` | Parameters for the MCP tool call (body content is passed separately) |
| `BodySource` | File path whose content becomes the comment body |

#### Action Struct Extension

```go
type Action struct {
    // ... existing fields unchanged ...

    // post-to-source metadata — populated only for post-to-source checkpoints
    PostMethod *PostMethod `json:"post_method,omitempty"`
}
```

`NewCheckpointAction` signature is unchanged. `PostMethod` is set by `handlePostToSource` after constructing the base action, or via a new constructor variant.

#### Per-Service Examples

**GitHub**:
```json
{
  "post_method": {
    "command": "gh issue comment {source_url} --body-file {body_source}",
    "body_source": ".specs/20260418-42-fix-auth/summary.md"
  }
}
```

**Jira**:
```json
{
  "post_method": {
    "command": "curl -s -X POST -H 'Content-Type: application/json' -u \"$JIRA_USER:$JIRA_TOKEN\" \"https://{jira_domain}/rest/api/3/issue/{source_id}/comment\" -d @{body_source}",
    "body_source": ".specs/20260418-proj-123-fix/summary.md"
  }
}
```

Note: Jira comment posting requires ADF conversion of the markdown body. The `command` approach is limited here. A more robust approach would use `mcp_tool` with Atlassian MCP, or the SKILL.md `instruction` fallback for ADF conversion. See "Jira Complexity" in the trade-offs section below.

**Linear**:
```json
{
  "post_method": {
    "mcp_tool": "mcp__linear__save_comment",
    "mcp_params": {"issueId": "DEA-13"},
    "body_source": ".specs/20260418-dea-13-fix/summary.md"
  }
}
```

### 3. `handlePostToSource` Changes

Current logic:
1. Read `source_type` → map to label
2. Read `source_url` → if empty, skip
3. Build message string → return checkpoint action

New logic:
1. Read `source_type`, `source_url`, `source_id`
2. If text or empty URL → skip (unchanged)
3. Build `PostMethod` based on `source_type`:
   - GitHub: `Command` = `gh issue comment {source_url} --body-file {body_source}`
   - Jira: `Command` = curl-based (or `MCPTool` if configured)
   - Linear: `MCPTool` = `mcp__linear__save_comment`, `MCPParams` = `{issueId: source_id}`
4. Attach `PostMethod` to checkpoint action

```go
func (e *Engine) handlePostToSource(st *state.State) (Action, error) {
    sourceType := e.sourceTypeReader(st.Workspace)
    sourceURL := e.sourceURLReader(st.Workspace)
    sourceID := e.sourceIDReader(st.Workspace)

    pm := buildPostMethod(sourceType, sourceURL, sourceID, st.Workspace)
    if pm == nil {
        return NewDoneAction(SkipSummaryPrefix+PhasePostToSource, ""), nil
    }

    label := sourceTypeLabel(sourceType)
    msg := fmt.Sprintf(
        "Pipeline complete. Post the final summary as a comment to the %s?\n\nURL: %s",
        label, sourceURL,
    )

    action := NewCheckpointAction(PhasePostToSource, msg, []string{"post", "skip"})
    action.PostMethod = pm
    return action, nil
}
```

### 4. `makeFetchNeeded` Changes

Current logic returns `type`, `fields`, `instruction`. New logic returns `type`, `mcp_tool`/`command`, `mcp_params`, `response_mapping`, `instruction`.

`makeFetchNeeded` needs `sourceID` and `sourceURL` to populate `mcp_params` and `command` templates. Its signature changes:

```go
func makeFetchNeeded(sourceType, sourceURL, sourceID string) *FetchNeeded
```

The caller in `PipelineInitHandler` already has these values.

### 5. SKILL.md Changes

#### fetch_needed Section (Step 1.4a)

Replace the current service-specific instructions:
```
a. If `result.fetch_needed` is non-null: fetch the external data described by `result.fetch_needed`
   (GitHub issue fields, Jira issue fields, or Linear issue fields), then call ...
   - For **Linear** (`fetch_needed.type == "linear"`): use `mcp__linear__get_issue` ...
```

With generic logic:
```
a. If `result.fetch_needed` is non-null:
   1. Fetch the external data:
      - If `fetch_needed.mcp_tool` is set: call the MCP tool with `fetch_needed.mcp_params`.
      - If `fetch_needed.command` is set: execute the command via Bash.
      - If neither is set: follow `fetch_needed.instruction` as a fallback.
   2. Map the response using `fetch_needed.response_mapping`:
      for each (response_key → context_key), set external_context[context_key] = response[response_key].
   3. Call pipeline_init_with_context with the mapped external_context.
```

#### post-to-source Section (Step 2, checkpoint handling)

Replace:
```
- Determine the source type from the URL (GitHub if `github.com`, Jira if `atlassian.net`, Linear if `linear.app`):
  - **GitHub**: run `gh issue comment <url> --body-file ...`
  - **Jira**: Extract the domain and issue key ... curl ...
  - **Linear**: Use `mcp__linear__save_comment` ...
```

With:
```
- If `action.post_method` is present:
  a. Read the body from `action.post_method.body_source`.
  b. If `post_method.mcp_tool` is set: call it with `post_method.mcp_params` and the body content.
  c. If `post_method.command` is set: execute the command via Bash
     (template variables `{source_url}` and `{body_source}` are already resolved).
  d. Report success or failure to the user.
```

### 6. Jira Complexity

Jira is the hardest case because:
- **Fetching** requires auth tokens (`$JIRA_USER:$JIRA_TOKEN`) and deeply nested JSON response
- **Posting** requires converting Markdown to Atlassian Document Format (ADF)

For the initial implementation, Jira retains the `instruction` field as a fallback guide for the orchestrator. The `command` field provides a best-effort CLI approach. If Atlassian MCP tools are available, `mcp_tool` can be used instead.

This is an acceptable trade-off: the goal is to make SKILL.md service-agnostic, not to make every service equally automatable. The `instruction` fallback ensures the orchestrator always has guidance.

### 7. Template Variable Resolution

Both `FetchNeeded.Command` and `PostMethod.Command` may contain template variables. These are resolved by the MCP server (Go code) before returning the JSON, not by the orchestrator.

Available variables:
- `{source_url}` — full issue URL
- `{source_id}` — issue identifier (e.g., "DEA-13", "42")
- `{jira_domain}` — extracted from Jira URL (e.g., "example.atlassian.net")
- `{body_source}` — `{workspace}/{artifact}` path

The MCP server resolves these at construction time in `makeFetchNeeded` and `buildPostMethod`, producing fully-formed commands. The orchestrator executes them as-is.

## Files to Change

| File | Change |
|------|--------|
| `orchestrator/actions.go` | Add `PostMethod` struct, add `PostMethod` field to `Action` |
| `tools/pipeline_init.go` | Restructure `FetchNeeded`, update `makeFetchNeeded` signature and logic |
| `orchestrator/engine.go` | `handlePostToSource` builds `PostMethod`, extract `buildPostMethod` and `sourceTypeLabel` helpers |
| `skills/forge/SKILL.md` | Replace service-specific fetch/post logic with generic logic |
| `tools/pipeline_init_test.go` | Update `FetchNeeded` assertions (new fields, removed `Fields`) |
| `orchestrator/engine_test.go` | Test `PostMethod` in post-to-source action |
| `.claude/rules/testing.md` | Update checklist if needed |

## Files NOT Changed

| File | Reason |
|------|--------|
| `tools/context_fetcher.go` | Internal parsing logic unchanged — still consumes `external_context` map |
| `tools/pipeline_init_with_context.go` | Still receives `external_context` the same way |
| `state/constants.go` | Source type constants unchanged |
| `validation/input.go` | URL validation unchanged |

## Trade-offs

- **Pro**: SKILL.md becomes fully service-agnostic. Adding a new service is a Go-only change.
- **Pro**: `FetchNeeded.ResponseMapping` is self-documenting — no separate field list needed.
- **Con**: `FetchNeeded.Fields` removed — callers relying on it need updating (only SKILL.md, no external consumers).
- **Con**: Jira remains partially manual due to ADF conversion complexity. Acceptable — the instruction fallback covers it.
- **Con**: Template variable resolution in Go adds a small amount of string manipulation code.

## Testing Strategy

- **Unit tests for `makeFetchNeeded`**: verify each service produces correct `mcp_tool`/`command`, `mcp_params`, `response_mapping`
- **Unit tests for `buildPostMethod`**: verify each service produces correct `PostMethod`
- **Unit tests for `handlePostToSource`**: verify `PostMethod` is attached to checkpoint action
- **Integration tests**: verify full pipeline_init → fetch_needed → pipeline_init_with_context flow with new fields
- **SKILL.md consistency check**: verify no service-specific branching remains

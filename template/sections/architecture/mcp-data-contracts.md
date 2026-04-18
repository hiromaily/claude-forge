# MCP Data Contracts

This document specifies the exact JSON payloads exchanged between the Claude orchestrator (SKILL.md) and the Go MCP server (`forge-state-mcp`) during a pipeline run. These four tools drive the entire pipeline lifecycle.

> **Source of truth**: The Go structs in `mcp-server/internal/handler/tools/` and `mcp-server/internal/engine/orchestrator/actions.go`. This document mirrors those definitions — update both when changing schemas.

---

## 1. `pipeline_init` — Input Parsing & Resume Detection

Pure detection tool. Parses the raw `/forge` arguments, detects source type, checks for resume candidates. **No side effects on state.**

### Request

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `arguments` | string | yes | Raw arguments string passed to `/forge` |
| `current_branch` | string | no | Output of `git branch --show-current` |

### Response

**Resume path** (input matches existing `.specs/` directory):

```json
{
  "resume_mode": "auto",
  "workspace": ".specs/20260330-fix-auth-timeout",
  "instruction": "call state_resume_info"
}
```

**New pipeline path**:

```json
{
  "workspace": ".specs/20260401-https-github-com-owner-repo-issues-42",
  "spec_name": "https-github-com-owner-repo-issues-42",
  "source_type": "github_issue",
  "source_url": "https://github.com/owner/repo/issues/42",
  "source_id": "42",
  "core_text": "https://github.com/owner/repo/issues/42",
  "flags": {
    "auto": false,
    "skip_pr": false,
    "debug": false,
    "discuss": false,
    "effort_override": null,
    "current_branch": "main"
  },
  "fetch_needed": {
    "type": "github",
    "fields": ["labels", "title", "body"],
    "instruction": "fetch github issue fields before calling pipeline_init_with_context"
  }
}
```

**Error path** (invalid input):

```json
{
  "errors": ["input too short: minimum 3 characters required"]
}
```

### `source_type` Values

| Value | Trigger |
|-------|---------|
| `github_issue` | URL matching `github.com/.../issues/\d+` |
| `jira_issue` | URL matching `*.atlassian.net/browse/...` |
| `text` | Plain text (default) |
| `workspace` | Input contains `.specs/` |

---

## 2. `pipeline_init_with_context` — Three-Call Confirmation Flow

Implements a multi-call handshake: detect effort → (optional: discuss) → confirm & initialise workspace.

### Request

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `workspace` | string | yes | Workspace path from `pipeline_init` |
| `source_id` | string | no | Source identifier (e.g., `"42"`, `"SOA-123"`) |
| `source_url` | string | no | Original URL (GitHub/Jira) |
| `external_context` | object | no | Fetched GitHub/Jira fields (see below) |
| `flags` | object | no | Parsed flags from `pipeline_init` |
| `task_text` | string | no | Original task text (text source only) |
| `user_confirmation` | object | no | Confirmed effort + branch decision (second call) |
| `discussion_answers` | string | no | User answers to discussion questions |

**`external_context` object:**

```json
{
  "github_labels": ["bug", "priority-high"],
  "github_title": "Fix auth timeout in middleware",
  "github_body": "requests timeout after 30s",
  "jira_issue_type": "Bug",
  "jira_story_points": 3,
  "jira_summary": "Skip minutes job without integration",
  "jira_description": "..."
}
```

**`user_confirmation` object (confirmation call):**

```json
{
  "effort": "M",
  "workspace_slug": "fix-auth-timeout",
  "use_current_branch": false,
  "enriched_request_body": "..."
}
```

### Response — First Call (effort detection)

Returns `needs_user_confirmation` for the orchestrator to present to the user:

```json
{
  "needs_user_confirmation": {
    "detected_effort": "M",
    "effort_options": {
      "S": {
        "skipped_phases": [
          { "phase_id": "phase-2", "label": "Investigation" },
          { "phase_id": "phase-3b", "label": "Design Review" }
        ],
        "recommended": false
      },
      "M": {
        "skipped_phases": [
          { "phase_id": "phase-4b", "label": "Tasks Review" },
          { "phase_id": "checkpoint-b", "label": "Human Reviews Tasks" }
        ],
        "recommended": true
      },
      "L": {
        "skipped_phases": [],
        "recommended": false
      }
    },
    "current_branch": "main",
    "is_main_branch": true,
    "enriched_request_body": "implement login feature",
    "message": "Detected effort=\"M\". ..."
  }
}
```

### Response — First Call with `--discuss` (text source only)

```json
{
  "needs_discussion": {
    "questions": [
      "What is the main goal of this change?",
      "Are there any constraints or dependencies?",
      "What is the expected scope of changes?"
    ],
    "message": "Please answer the following questions..."
  }
}
```

### Response — Confirmation Call (workspace finalised)

```json
{
  "ready": true,
  "workspace": ".specs/20260401-42-fix-auth-timeout",
  "effort": "M",
  "flow_template": "standard",
  "skipped_phases": ["phase-4b", "checkpoint-b"],
  "request_md_content": "---\nsource_type: github_issue\n...",
  "branch": "feature/42-fix-auth-timeout",
  "create_branch": true
}
```

### Call Discriminator

| `discussion_answers` | `user_confirmation` | Path |
|---|---|---|
| absent | absent | First call → detect effort |
| present | absent | Discussion call → enrich body |
| absent | present | Confirmation call → init workspace |
| present | present | **Error** — ambiguous |

---

## 3. `pipeline_next_action` — Action Dispatch

The core loop driver. Reads `state.json`, runs `Engine.NextAction()` deterministically, returns a typed action for the orchestrator to execute.

### Request

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `workspace` | string | yes | Workspace path |
| `previous_action_complete` | boolean | no | True after agent/exec/write_file completes |
| `previous_tokens` | number | no | Token count from previous action |
| `previous_duration_ms` | number | no | Duration in ms of previous action |
| `previous_model` | string | no | Model used for previous action |
| `previous_setup_only` | boolean | no | True if previous exec was setup-only |
| `user_response` | string | no | User response for checkpoint actions |

### Response Structure

Every response wraps an `Action` with optional metadata:

```json
{
  "type": "spawn_agent",
  "warning": "",
  "display_message": "Phase 1: Situation Analysis",
  "report_result": null,
  ...action-specific fields...
}
```

When `report_result` is non-null, the engine has recorded a phase result internally (this happens for `pipeline_report_result` calls routed through `pipeline_next_action`):

```json
{
  "report_result": {
    "next_action_hint": "revision_required",
    "verdict_parsed": "REVISE",
    "findings": [
      { "severity": "CRITICAL", "description": "Missing error handling for..." }
    ],
    "warning": "",
    "display_message": ""
  }
}
```

### Action Types

#### `spawn_agent` — Dispatch an LLM subagent

```json
{
  "type": "spawn_agent",
  "agent": "situation-analyst",
  "prompt": "...4-layer assembled prompt...",
  "model": "sonnet",
  "phase": "phase-1",
  "input_files": ["request.md"],
  "output_file": "analysis.md",
  "parallel_task_ids": null
}
```

The `prompt` field contains the **4-layer assembled prompt** (see below). When `parallel_task_ids` is non-empty, the orchestrator spawns one agent per task ID concurrently.

#### `checkpoint` — Pause for human review

```json
{
  "type": "checkpoint",
  "name": "checkpoint-a",
  "present_to_user": "## Design Review\n\n...",
  "options": ["approve", "reject"]
}
```

#### `exec` — Run a shell command

```json
{
  "type": "exec",
  "phase": "pr-creation",
  "commands": ["gh", "pr", "create", "--title", "feat: ...", "--body", "..."],
  "setup_only": false
}
```

#### `write_file` — Write content to disk

```json
{
  "type": "write_file",
  "phase": "phase-5",
  "path": ".specs/20260401-fix-auth/tasks.md",
  "content": "# Tasks\n\n..."
}
```

#### `human_gate` — Wait for external human action

```json
{
  "type": "human_gate",
  "phase": "phase-5",
  "name": "merge-external-pr",
  "present_to_user": "Task 3 requires merging PR #456 in repo-b...",
  "options": ["done", "skip", "abandon"]
}
```

#### `done` — Pipeline complete

```json
{
  "type": "done",
  "summary": "Pipeline completed: 10 phases, 2 skipped",
  "summary_path": ".specs/20260401-fix-auth/summary.md"
}
```

### 4-Layer Prompt Assembly

The `prompt` field in `spawn_agent` actions is assembled from four layers:

```
┌─────────────────────────────────────────────────────┐
│ Layer 1: Agent Instructions                         │
│   (loaded from agents/{name}.md)                    │
├─────────────────────────────────────────────────────┤
│ Layer 2: Input/Output Artifacts                     │
│   ## Input Files                                    │
│   - {workspace}/request.md                          │
│   - {workspace}/analysis.md                         │
│   ## Output File                                    │
│   - {workspace}/design.md                           │
├─────────────────────────────────────────────────────┤
│ Layer 3: Repository Profile                         │
│   ## Repository Context                             │
│   Languages: Go (82%), TypeScript (15%)             │
│   Build command: make build                         │
│   Test command: go test ./...                       │
│   Linter: golangci-lint                             │
├─────────────────────────────────────────────────────┤
│ Layer 4: Data Flywheel (cross-pipeline learning)    │
│   ## Similar Past Pipelines                         │
│   (BM25-scored matches from .specs/index.json)      │
│   ## Past Review Patterns                           │
│   (Levenshtein-merged review findings)              │
│   ## AI Friction Points                             │
│   (from past improvement.md reports)                │
└─────────────────────────────────────────────────────┘
```

Layers 3 and 4 are injected only when data is available. Layer 2 lists file **paths** only — agents read the files themselves.

---

## 4. `pipeline_report_result` — Phase Result Recording

Records metrics, validates artifacts, parses review verdicts, and advances pipeline state.

### Request

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `workspace` | string | yes | Workspace path |
| `phase` | string | yes | Phase ID (e.g., `"phase-3"`, `"phase-5"`) |
| `tokens_used` | number | no | Tokens consumed by the phase |
| `duration_ms` | number | no | Wall-clock duration in ms |
| `model` | string | no | Model used (e.g., `"sonnet"`, `"opus"`) |
| `setup_only` | boolean | no | True if exec was setup-only (no agent ran) |

### Response

```json
{
  "state_updated": true,
  "artifact_written": "review-design.md",
  "verdict_parsed": "APPROVE_WITH_NOTES",
  "findings": [
    { "severity": "MINOR", "description": "Consider adding error context to..." }
  ],
  "next_action_hint": "proceed",
  "warning": "",
  "display_message": ""
}
```

### `next_action_hint` Values

| Value | Meaning | Orchestrator Action |
|-------|---------|-------------------|
| `proceed` | Phase completed successfully | Continue to next `pipeline_next_action` |
| `revision_required` | Review verdict is REVISE or FAIL | Present findings to user, re-run phase |
| `setup_continue` | Internal setup action completed | Engine re-enters `NextAction` automatically |

### Verdict Parsing

The MCP server parses review verdicts from artifact content:

| Phase | Verdicts | Source |
|-------|----------|--------|
| phase-3b (Design Review) | `APPROVE`, `APPROVE_WITH_NOTES`, `REVISE` | `review-design.md` |
| phase-4b (Tasks Review) | `APPROVE`, `APPROVE_WITH_NOTES`, `REVISE` | `review-tasks.md` |
| phase-6 (Code Review) | `PASS`, `PASS_WITH_NOTES`, `FAIL` | `review-{N}.md` |

---

## Orchestrator Loop — Complete Data Flow

The following shows the exact sequence of MCP tool calls and their payloads for a single phase:

```
Orchestrator                          MCP Server                     Disk
    │                                     │                           │
    │─── pipeline_next_action ───────────►│                           │
    │    { workspace, previous_* }        │── read state.json ───────►│
    │                                     │◄── state data ────────────│
    │                                     │── Engine.NextAction() ────│
    │                                     │── read agent .md ────────►│
    │                                     │── 4-layer prompt build ───│
    │◄── { type: spawn_agent, ... } ──────│                           │
    │                                     │                           │
    │─── Agent(prompt=...) ──────────────────────────────────────────►│
    │                                     │       (agent reads files) │
    │◄── agent output ───────────────────────────────────────────────│
    │                                     │                           │
    │─── Write(analysis.md) ────────────────────────────────────────►│
    │                                     │                           │
    │─── pipeline_next_action ───────────►│                           │
    │    { previous_action_complete: true  │── handleReportResult ────│
    │      previous_tokens: 15000         │── validate artifact ─────►│
    │      previous_duration_ms: 45000 }  │── parse verdict ──────────│
    │                                     │── advance state ─────────►│
    │◄── { type: spawn_agent, ... } ──────│   (next phase action)     │
    │         (next phase)                │                           │
```

> **Key invariant**: The orchestrator never decides which phase to run. It executes the action returned by `pipeline_next_action` and reports the result back. All control flow lives in `Engine.NextAction()`.

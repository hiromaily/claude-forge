# forge-queue: Autonomous Task Queue Design

Status: draft v3 (2026-04-17)

## Overview

`forge-queue` enables sequential batch execution of issue-based tasks.
Users create `.specs/queue.yaml` with a list of issue URLs; the MCP server
manages the queue state and the existing `forge` pipeline processes each task.

## Architecture

```text
SKILL.md (forge-queue)
  │
  │  queue_init(queue_path)          ← parse + validate YAML
  │        │
  │        ▼
  │  queue_next(queue_path)          ← return next task + pre-generated workspace slug
  │        │
  │        ▼
  │  claude -p "/forge {url} --auto" ← isolated subprocess, forge unchanged
  │        │                            SKILL.md passes workspace_slug in user_confirmation
  │        ▼
  │  queue_report(queue_path, index) ← find workspace by slug, read state.json
  │        │
  │        ▼
  │  queue_next(queue_path)          ← next task, or "all done"
  │        │
  │        ...
```

The four new MCP tools are thin YAML I/O wrappers. All pipeline logic
remains in the existing `pipeline_init` / `pipeline_next_action` /
`pipeline_report_result` chain.

## Skills

Two separate skills with distinct responsibilities:

- `/forge-queue-create` — generate `.specs/queue.yaml`
- `/forge-queue` — execute `.specs/queue.yaml`

### `/forge-queue-create`

Generates a `queue.yaml` file. Supports two input modes:

**Mode A: URL direct specification**

```text
/forge-queue-create https://jira.example.com/browse/DEA-123 https://jira.example.com/browse/DEA-456
```

The skill validates each URL via the `queue_create` MCP tool, asks the user
for effort levels (or accepts a default), and writes the YAML file.

**Mode B: Search-based collection**

```text
/forge-queue-create --jira-project DEA --jira-status "To Do"
/forge-queue-create --gh-label "bug" --gh-state "open"
```

The skill uses existing tools to search for issues:

- **Jira**: Atlassian MCP tools (if available) or Jira REST API
  via `curl` (same pattern as forge's Jira integration).
- **GitHub**: `gh issue list --label <label> --state <state> --json url,title`

The skill collects matching issues, presents them to the user for
confirmation (select/deselect), asks for effort per task (or a default),
then calls `queue_create` to write the YAML file.

**Why this split**: Mode A is deterministic (URL → YAML), handled entirely
by a MCP tool. Mode B requires external API calls and user interaction
(issue selection), which are skill-level concerns — the MCP tool should not
make API calls to Jira/GitHub or interact with the user.

### `/forge-queue`

Executes the queue. See [Skill Design](#skill-design) below.

## MCP Tools

### `queue_create`

**Purpose**: Generate a new `.specs/queue.yaml` from a list of URLs.

**Parameters**:

- `queue_path` (string, required): Output path for the queue YAML file.
- `tasks` (array, required): List of task objects, each with:
  - `url` (string, required): Issue URL.
  - `effort` (string, optional): `S`, `M`, or `L`. When omitted, forge
    selects the recommended effort automatically (`--auto` behavior).

**Behavior**:

1. Validate each entry:
   - `url` matches a known source type (GitHub issue, Jira issue).
   - `effort`, if present, is one of `S`, `M`, `L`.
2. If the file already exists, return an error (prevent accidental overwrite).
   User must delete or rename the existing file first.
3. Write the YAML file.

**Returns**:

```json
{
  "created": true,
  "path": ".specs/queue.yaml",
  "task_count": 3,
  "errors": []
}
```

### `queue_init`

**Purpose**: Parse and validate `.specs/queue.yaml`.

**Parameters**:

- `queue_path` (string, required): Path to the queue YAML file.

**Behavior**:

1. Read and parse the YAML file.
2. Validate every entry:
   - `url` is present, non-empty, and matches a known source type
     (GitHub issue, Jira issue).
   - `effort`, if present, is one of `S`, `M`, `L`.
3. Return a summary: total count, completed count, failed count, pending count.

**Returns**:

```json
{
  "total": 4,
  "completed": 1,
  "failed": 0,
  "pending": 3,
  "errors": []
}
```

If `errors` is non-empty, the queue is invalid and should not be processed.

### `queue_next`

**Purpose**: Return the next unprocessed task from the queue, pre-generating
a workspace slug so the workspace path is deterministic.

**Parameters**:

- `queue_path` (string, required): Path to the queue YAML file.

**Behavior**:

1. Read and parse the YAML file.
2. Find the first entry where `status` is absent **or** `status` is
   `in_progress` (resume after interruption).
3. For new tasks:
   - Pre-generate a workspace slug from the URL. The slug is derived
     from the issue identifier:
     - Jira: `dea-123` (from `https://jira.example.com/browse/DEA-123`)
     - GitHub: `42` (from `https://github.com/org/repo/issues/42`)
     The slug is a stable, deterministic value derived solely from the URL.
   - Write `status: in_progress`, `started_at: <ISO8601>`, and
     `workspace_slug: <generated slug>` to queue.yaml.
   For `in_progress` entries: no changes (idempotent — `started_at` and
   `workspace_slug` are preserved from the previous attempt).
4. Return the task details including `workspace_slug`.

**How the workspace slug reaches forge**: The forge subprocess SKILL.md
passes `workspace_slug` in the `user_confirmation` object to
`pipeline_init_with_context`. This is an **existing feature** —
`pipeline_init_with_context` already accepts `workspace_slug` in
`user_confirmation` and applies it via `applyWorkspaceSlug` (line 277-283
of `pipeline_init_with_context.go`). No forge code changes needed.

The actual workspace path is determined by forge:
`YYYYMMDD-{source_id}-{workspace_slug}` or
`YYYYMMDD-{source_id}-{issue_title_slug}` (when slug is refined by
external context). `queue_report` locates the workspace by scanning
`.specs/` for directories matching the date + source_id prefix.

**Resume semantics**: An `in_progress` entry means the previous session was
interrupted mid-pipeline. `queue_next` returns it as the next task so the
forge pipeline's existing resume logic (`pipeline_init` auto-resume) handles
recovery. No special queue-level retry logic is needed.

**Returns** (new task with effort):

```json
{
  "has_next": true,
  "index": 2,
  "resuming": false,
  "url": "https://github.com/org/repo/issues/42",
  "effort": "S",
  "workspace_slug": "42",
  "forge_arguments": "https://github.com/org/repo/issues/42 --auto effort:S"
}
```

**Returns** (new task without effort):

```json
{
  "has_next": true,
  "index": 3,
  "resuming": false,
  "url": "https://jira.example.com/browse/DEA-789",
  "effort": null,
  "workspace_slug": "dea-789",
  "forge_arguments": "https://jira.example.com/browse/DEA-789 --auto"
}
```

`forge_arguments` is the pre-built string that can be passed directly to
`pipeline_init(arguments=...)`. The `--auto` flag is always included.
When `effort` is absent, the `effort:` flag is omitted — forge's
`pipeline_init_with_context` selects the recommended effort automatically
in `--auto` mode.

`forge_arguments` does NOT include the workspace slug. The slug is passed
separately — the forge-queue SKILL.md embeds it in the `claude -p` prompt
so that the subprocess's forge SKILL.md includes it in `user_confirmation`.

**Returns** (resuming interrupted task):

```json
{
  "has_next": true,
  "index": 1,
  "resuming": true,
  "url": "https://jira.example.com/browse/DEA-456",
  "effort": "S",
  "workspace_slug": "dea-456",
  "workspace": ".specs/20260417-dea-456-add-export-feature",
  "forge_arguments": ".specs/20260417-dea-456-add-export-feature"
}
```

When `resuming` is true, `forge_arguments` contains the workspace path
instead of the URL. forge's `pipeline_init` detects this as a resume
candidate and proceeds via auto-resume. The `workspace` field is read
from queue.yaml (written by `queue_report` after the first attempt).

**Returns** (no more tasks):

```json
{
  "has_next": false,
  "summary": {
    "total": 4,
    "completed": 3,
    "failed": 1,
    "results": [
      {"url": "...", "status": "completed", "pr": 2891},
      {"url": "...", "status": "failed", "reason": "..."}
    ]
  }
}
```

### `queue_report`

**Purpose**: Determine the outcome of a completed task and record it back
to `queue.yaml`. The caller does not need to interpret pipeline results —
this tool reads `state.json` directly and makes the determination itself
(deterministic, no LLM judgment).

**Parameters**:

- `queue_path` (string, required): Path to the queue YAML file.
- `index` (number, required): The task index returned by `queue_next`.

**Behavior**:

1. Read `queue.yaml`, find the entry at `index`.
2. Read `workspace_slug` from the entry (written by `queue_next`).
3. Locate the workspace directory in `.specs/`:
   - Extract the date prefix from `started_at` (e.g., `20260417`).
   - Extract the source ID from the URL (e.g., `dea-123` for Jira,
     `42` for GitHub).
   - Scan `.specs/` for directories matching the pattern
     `{date_prefix}-{source_id}*`. Since execution is sequential and
     source IDs are unique per queue entry, at most one directory matches.
   - If no match found: mark the task as `failed` with reason
     `"workspace not found"`.
4. Read `{workspace}/state.json`.
5. Determine the outcome deterministically:
   - `currentPhase == "completed"` → `status: completed`.
   - Any other phase → `status: failed`.
     Reason: `"{currentPhase}: {error.message}"` from `state.json`.
     If `state.Error` is nil (e.g., abandoned pipeline without recorded
     error), reason is `"{currentPhase}: abandoned"`.
6. Read `state.json.branch` for the branch name.
7. Update the queue.yaml entry:
   - `status`: completed or failed
   - `workspace`: actual directory name (e.g., `20260417-dea-123-fix-login`)
   - `branch`: git branch name (e.g., `feature/20260417-dea-123-fix-login`)
   - `reason`: failure reason (failed only)
   - `finished_at`: ISO8601 timestamp
8. Write `queue.yaml` back atomically.

**Returns**:

```json
{
  "status": "completed",
  "branch": "feature/20260417-dea-123-fix-login-validation",
  "workspace": "20260417-dea-123-fix-login-validation",
  "remaining": 2
}
```

### `queue_update_pr`

**Purpose**: Write the PR number to a queue.yaml entry. Called by the
skill after looking up the PR via `gh pr list`.

**Parameters**:

- `queue_path` (string, required): Path to the queue YAML file.
- `index` (number, required): The task index.
- `pr` (number, required): The PR number.

**Behavior**:

1. Read `queue.yaml`, find the entry at `index`.
2. Write `pr: <number>` to the entry.
3. Write `queue.yaml` back atomically.

**Returns**:

```json
{
  "updated": true
}
```

**Design rationale**: PR number lookup requires `gh pr list` (a shell
command), which must not run inside an MCP tool (Constraint #12). The
skill runs the shell command and passes the result to this tool for
atomic YAML write. This keeps the MCP tools pure Go while ensuring
queue.yaml writes are always atomic (Constraint #6).

## Skill Design

`forge-queue` is a separate skill (`/forge-queue`) that knows nothing about
forge internals. Each task runs in an **isolated `claude -p` subprocess**,
ensuring a clean context window per task with zero cross-task contamination.

```markdown
## Step 1: Initialize

1. Call `queue_init(queue_path=".specs/queue.yaml")`.
2. If errors: report and stop.
3. Report queue status (e.g. "4 tasks: 1 completed, 1 failed, 2 pending").

## Step 2: Process Loop

1. Call `queue_next(queue_path=".specs/queue.yaml")`.
2. If `has_next` is false: output summary and stop.
3. If NOT resuming (`resuming` is false):
   Run `git checkout main && git pull --rebase`.
4. Run forge as a subprocess via Bash:
   `claude -p "/forge {forge_arguments}" --allowedTools "Bash,Read,Write,Edit,Glob,Grep,Agent,Skill,mcp__plugin_claude-forge_forge-state__*"`
   - For new tasks, append to the prompt:
     "Use workspace_slug '{workspace_slug}' in user_confirmation."
   - Each subprocess starts a fresh session with an empty context window.
   - forge runs autonomously (--auto) and exits on completion or failure.
5. Call `queue_report(queue_path=".specs/queue.yaml", index=<index>)`.
6. If `status == "completed"` and `branch` is present:
   a. Run `gh pr list --head {branch} --json number --jq '.[0].number'`
   b. If PR number is found:
      Call `queue_update_pr(queue_path, index, pr=<number>)`.
7. Return to step 1.
```

### Why subprocess isolation

- **Context separation**: Each task gets a clean context window. Previous
  task's code, errors, and design decisions do not leak into the next task.
- **No /clear needed**: `/clear` is a CLI-only interactive command and
  cannot be called programmatically. `claude -p` achieves the same effect
  by starting a new session per task.
- **forge unchanged**: From forge's perspective, each subprocess invocation
  is identical to a user typing `/forge {url} --auto` in a fresh terminal.

### Subprocess MCP server availability (verified)

`claude -p` subprocess has full access to the `forge-state` MCP server
when run from the same repository root where `claude-forge` is installed
as a plugin. **Verified**: all 44 `mcp__plugin_claude-forge_forge-state__*`
tools are available in `claude -p` sessions (tested 2026-04-17).

Authentication (`gh` CLI, Jira credentials) is inherited from the parent
shell environment.

### Workspace slug flow

The workspace slug flows through the system without modifying forge:

```text
queue_next                    queue.yaml          subprocess (forge)
  │                               │                     │
  │ pre-generate slug             │                     │
  │ from URL (e.g. "dea-123")    │                     │
  │──write workspace_slug───────▶│                     │
  │                               │                     │
  │ return workspace_slug         │                     │
  │◀──────────────────────────────│                     │
  │                               │                     │
  │ embed slug in claude -p       │                     │
  │ prompt instruction            │                     │
  │──────────────────────────────────────────────────▶ │
  │                               │    forge SKILL.md   │
  │                               │    passes slug in   │
  │                               │    user_confirmation│
  │                               │    .workspace_slug  │
  │                               │         │           │
  │                               │         ▼           │
  │                               │    pipeline_init_   │
  │                               │    with_context     │
  │                               │    applies slug     │
  │                               │    (existing code   │
  │                               │     L277-283)       │
  │                               │         │           │
  │                               │         ▼           │
  │                               │    workspace created│
  │                               │    .specs/20260417- │
  │                               │    dea-123-fix-login│
  │                               │                     │

queue_report                  queue.yaml
  │                               │
  │ read workspace_slug           │
  │◀──────────────────────────────│
  │                               │
  │ scan .specs/ for              │
  │ {date}-{source_id}*           │
  │ → finds 20260417-dea-123-...  │
  │                               │
  │ read state.json               │
  │ determine status              │
  │──write workspace, branch─────▶│
```

### Resume behavior

When the user interrupts a queue run (Ctrl+C, closes terminal, etc.):

1. The current task's `status` remains `in_progress` in `queue.yaml`,
   with `workspace_slug` and `started_at` already recorded.
2. Completed tasks are already `completed` or `failed`.
3. Remaining tasks have no `status`.

To resume, the user simply runs `/forge-queue .specs/queue.yaml` again:

1. `queue_init` reports the current state (N completed, M failed, 1 in progress, K pending).
2. `queue_next` returns the `in_progress` task with its existing `workspace_slug`.
3. If `workspace` is set (written by `queue_report` after first partial run):
   `forge_arguments` contains the workspace path, triggering forge's
   auto-resume via `pipeline_init`.
4. If `workspace` is not yet set (interrupted before `queue_report` ran):
   `forge_arguments` contains the URL. forge creates a new workspace.
   The workspace slug ensures the same slug is used, but `pipeline_init`
   may generate a slightly different workspace name (date may differ).
   `queue_report` handles this via the date-prefix scan.
5. After the resumed task completes, `queue_report` records the result
   and the loop continues with the next pending task.

### Separation of concerns

`forge-queue` does NOT know:

- How forge's main loop works (`pipeline_next_action` dispatch)
- What action types exist (`spawn_agent`, `checkpoint`, `exec`, etc.)
- How phases, revisions, or reviews work
- How PR creation or post-to-source works

`forge-queue` ONLY knows:

- How to read/validate a YAML queue (`queue_init`)
- How to pick the next task and generate a slug (`queue_next`)
- How to spawn a `claude -p` subprocess with `forge_arguments`
- How to instruct the subprocess to use a specific `workspace_slug`
- How to record results (`queue_report`)
- How to look up PR numbers via `gh pr list` (shell command)
- How to write PR numbers back atomically (`queue_update_pr`)
- How to return to main branch between tasks

## queue.yaml Schema

```yaml
tasks:
  - url: https://jira.example.com/browse/DEA-123    # required
    effort: M                                        # optional: S | M | L (auto-selected if omitted)
    # — fields below are managed by forge-queue —
    status: completed                                # completed | failed | in_progress
    workspace_slug: dea-123                          # pre-generated slug (set by queue_next)
    workspace: 20260417-dea-123-fix-login             # actual .specs/ directory name (set by queue_report)
    branch: feature/20260417-dea-123-fix-login        # git branch name (set by queue_report)
    pr: 2891                                         # PR number (set by skill via queue_update_pr)
    reason: "phase-3: design rejected"               # failure reason (set by queue_report)
    started_at: "2026-04-17T10:30:00Z"               # ISO8601 (set by queue_next)
    finished_at: "2026-04-17T10:45:00Z"              # ISO8601 (set by queue_report)
```

## Design Constraints

1. **Sequential only** — no parallel execution. Users open multiple
   terminals for parallelism.
2. **`--auto` forced** — no checkpoints; each task runs autonomously.
3. **Link-only input** — tasks must be issue URLs (Jira, GitHub).
   Free-text tasks are not supported in queue mode.
4. **No forge internals changes** — the five queue tools are additive.
   `pipeline_init`, `pipeline_next_action`, `pipeline_report_result`
   are not modified. The workspace slug is communicated via the existing
   `user_confirmation.workspace_slug` field (already supported by
   `pipeline_init_with_context`).
5. **State in queue.yaml** — the YAML file is both input and state tracker.
   No separate state file.
6. **Atomic writes** — all queue.yaml mutations go through MCP tools
   (`queue_next`, `queue_report`, `queue_update_pr`). The skill never
   writes queue.yaml directly.
7. **Branch isolation** — each task gets its own branch. The skill runs
   `git checkout main && git pull --rebase` between tasks.
8. **Fail-forward** — a failed task is abandoned and the next task begins.
9. **Deterministic result determination** — `queue_report` reads state.json
   directly. The SKILL.md never interprets pipeline outcomes.
10. **Resumable** — `queue_next` treats `in_progress` entries as candidates,
    enabling recovery from interrupted sessions.
11. **Session isolation** — each task runs in a separate `claude -p`
    subprocess. Context window is clean per task; no cross-task contamination.
12. **MCP tools are pure Go** — no `os/exec` calls to external commands
    (`gh`, `curl`, etc.) inside MCP tools. Shell commands run in the skill
    layer only.
13. **Workspace slug known before subprocess** — `queue_next` pre-generates
    the slug and writes it to queue.yaml. The subprocess passes it to
    forge via the existing `user_confirmation.workspace_slug` mechanism.
    `queue_report` locates the workspace by date + source_id prefix scan.

## Go Package Location

```text
mcp-server/internal/queue/         ← YAML parse/validate/read/write + workspace scan
mcp-server/internal/tools/
  queue_create.go                  ← MCP handler (generate queue.yaml)
  queue_init.go                    ← MCP handler (validate existing queue.yaml)
  queue_next.go                    ← MCP handler (pick next task + slug)
  queue_report.go                  ← MCP handler (record result)
  queue_update_pr.go               ← MCP handler (write PR number)
skills/
  forge-queue/SKILL.md             ← queue executor skill
  forge-queue-create/SKILL.md      ← queue generator skill
```

### Dependency Direction

`queue` package imports `state.ReadState` (read-only) to determine pipeline
outcomes in `queue_report`. This is a one-way dependency:

```text
tools → queue → state (ReadState only)
```

This follows the existing layering rule (`tools → ... → state`).
No reverse dependency is introduced. The `queue` package does not import
`orchestrator` or `tools`.

### URL validation reuse

Both `queue_create` and `queue_init` validate URLs using source type
detection. The `validation` package (used by `pipeline_init`) exposes
`ValidateInput` which includes source type detection. Since `queue`
imports `state` only (not `tools` or `validation`), the URL validation
logic is extracted into a shared function in the `validation` package
(which `queue` can import without violating the DAG):

```text
tools → queue → validation (URL validation)
                      ↑
              tools → validation (existing)
```

## Test Strategy

### Go unit tests (`mcp-server/internal/queue/`)

- YAML parse/write round-trip (preserves field order)
- Validation: missing URL, invalid effort, invalid URL format, duplicate URLs
- `queue_next` state transitions: absent → in_progress, in_progress → idempotent
- `queue_next` with all tasks completed → `has_next: false`
- `queue_next` slug generation: Jira URL → lowercase key, GitHub URL → issue number
- Workspace scan: date + source_id prefix matching with 0, 1, and multiple candidates
- `queue_report` status determination from state.json:
  - `currentPhase == "completed"` → completed
  - `currentPhase != "completed"`, `Error` present → failed with message
  - `currentPhase != "completed"`, `Error` nil → failed with "abandoned"
- `queue_report` workspace slug refinement: pre-generated slug vs actual directory
- Atomic write: verify file integrity after write

### MCP handler tests (`mcp-server/internal/tools/`)

- `queue_create`: validates URLs, rejects existing file, writes valid YAML
- `queue_init`: returns correct counts for mixed-status queues
- `queue_next`: returns correct `forge_arguments` with/without effort
- `queue_next`: returns correct `workspace_slug` for Jira and GitHub URLs
- `queue_report`: reads state.json, writes correct status and branch
- `queue_update_pr`: writes PR number to correct entry

### Integration test (manual)

- End-to-end: create queue → run `/forge-queue` → verify queue.yaml updated
- Interrupt mid-task → resume → verify `in_progress` task is picked up
- Verify `workspace_slug` in `user_confirmation` produces expected workspace path

## Tool Count Impact

Current: 44 tools. After: 49 tools (+5: `queue_create`, `queue_init`,
`queue_next`, `queue_report`, `queue_update_pr`).
Update counts in `CLAUDE.md`, `scripts/README.md`, and `README.md`.

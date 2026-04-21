# Anthropic Go SDK Integration

Status: **research** (2026-04-21)

## Overview

This document evaluates how [anthropic-sdk-go](https://github.com/anthropics/anthropic-sdk-go)
can be integrated into claude-forge and what capabilities it unlocks. The primary
motivation is Phase 2 of [Remote Dashboard Control](./remote-dashboard-control.md),
which requires an Agent runtime that can execute forge pipelines without Claude Code.

## Problem: Claude Code Dependency

claude-forge currently relies entirely on Claude Code for LLM interactions:

```text
Claude Code CLI
  └── Agent tool (spawns subagent per phase)
        └── Each subagent: full Claude Code session
              ├── system prompt construction
              ├── tool loading (Edit, Glob, Grep, Bash, …)
              └── MCP tool calls (forge-state)
```

This means:

1. **No headless execution** — pipelines cannot run without an interactive Claude
   Code session (terminal or IDE).
2. **No external task submission** — the Dashboard can monitor and approve
   checkpoints, but cannot start new pipelines.
3. **No CI/CD integration** — automated pipelines (e.g., triggered by GitHub
   webhook or cron) are not possible.

## What anthropic-sdk-go Enables

### 1. In-Process Agent Runtime (Phase 2 Task Runner)

The Go SDK enables the `taskrunner` package (defined in
[remote-dashboard-control.md §3.3](./remote-dashboard-control.md#_3-3-go-package-layout))
to run Agent sessions directly inside the `forge-state-mcp` process:

```text
forge-state-mcp (single Go binary)
  ├── MCP server (stdio)        — state management (47 tools)
  ├── Dashboard server (HTTP)   — SSE + checkpoint approval
  └── Task Runner               — NEW: Agent SDK sessions
        ├── POST /api/task/submit → enqueue task
        ├── goroutine pool picks up task
        ├── anthropic-sdk-go multi-turn session
        │     ├── tool_use: read/write files
        │     ├── tool_use: forge-state MCP calls (in-process)
        │     └── tool_use: shell commands
        ├── publishes events to same EventBus
        └── Dashboard SSE shows progress in real time
```

Key benefit: **unified control plane**. SDK-run pipelines and interactive Claude
Code pipelines share the same EventBus, SSE stream, and checkpoint approval flow.

### 2. Claude Code–Free Pipeline Execution

With the SDK, forge pipelines can be triggered from:

- **Dashboard Web UI** — submit a GitHub issue URL from a smartphone
- **CI/CD** — GitHub Actions workflow calls `POST /api/task/submit`
- **Cron** — scheduled pipelines via the existing task queue
- **CLI** — a lightweight `forge-run` command that calls the HTTP API

None of these require Claude Code to be installed or running.

### 3. Fine-Grained Model Control

The SDK exposes parameters that Claude Code's Agent tool does not:

| Parameter | Claude Code Agent | anthropic-sdk-go |
|-----------|-------------------|------------------|
| `temperature` | Not configurable | Per-request |
| `max_tokens` | Not configurable | Per-request |
| `tool_choice` | Not configurable | `auto` / `any` / `tool` |
| `model` | Limited (`sonnet`, `opus`, `haiku`) | Any model ID |
| Extended thinking `budget_tokens` | Not configurable | Per-request |
| `stop_sequences` | Not configurable | Per-request |

This enables phase-specific optimization:

- **Design reviewer**: low temperature, `tool_choice: {"type": "tool", "name": "submit_verdict"}` for structured output
- **Brainstorming**: higher temperature
- **Implementer**: extended thinking with large budget

### 4. Accurate Token Tracking

The SDK returns `usage.input_tokens` and `usage.output_tokens` in every response.
Currently `analytics_pipeline_summary` estimates tokens from output text length.
With SDK-run pipelines, token and cost tracking become exact.

### 5. Structured Tool Use for Review Verdicts

Currently, review agents (design-reviewer, task-reviewer, impl-reviewer) output
free-text verdicts that are parsed via string matching (`APPROVE`, `REVISE`,
`PASS`, `FAIL`). With the SDK, verdicts can be enforced via `tool_choice`:

```go
// Force the model to call the verdict tool — no free-text parsing needed
messages.New().
    Tool(anthropic.ToolParam{
        Name: "submit_verdict",
        InputSchema: verdictSchema, // {"verdict": "APPROVE"|"REVISE", "findings": [...]}
    }).
    ToolChoice(anthropic.ToolChoiceParam{
        Type: anthropic.ToolChoiceTypeTool,
        Name: anthropic.String("submit_verdict"),
    })
```

This eliminates the verdict parsing failure mode documented in
`pipeline_report_result.go`.

## What It Does NOT Replace

The SDK is not a replacement for Claude Code in all scenarios:

| Capability | Claude Code | anthropic-sdk-go |
|-----------|-------------|------------------|
| File editing with conflict detection | Built-in (Edit tool) | Must implement |
| Codebase search (Glob, Grep) | Built-in | Must implement or shell out |
| Git operations | Built-in | Must shell out |
| LSP integration | Built-in | Not available |
| User interaction (AskUserQuestion) | Built-in | Dashboard UI only |

For the **implementer phase** (phase-5), which requires heavy file editing and
codebase navigation, the tradeoff is:

- **Interactive sessions**: keep using Claude Code subagents (current approach) —
  best developer experience, full tool suite
- **Headless/CI sessions**: SDK + shell-out for file operations — less ergonomic
  but functional for automated pipelines

## Integration Architecture

```text
                    ┌─────────────────────────────┐
                    │     forge-state-mcp          │
                    │     (single Go binary)       │
                    ├─────────────────────────────┤
                    │  MCP Server (stdio)          │ ← Claude Code interactive
                    ├─────────────────────────────┤
                    │  Dashboard Server (HTTP)     │ ← browser / smartphone
                    │    ├── SSE /events           │
                    │    ├── POST /api/task/submit  │
                    │    └── POST /api/checkpoint   │
                    ├─────────────────────────────┤
                    │  Task Runner                 │
                    │    ├── anthropic-sdk-go       │ ← headless pipelines
                    │    ├── in-process EventBus    │
                    │    └── in-process StateManager│
                    └─────────────────────────────┘
                              │
              ┌───────────────┼───────────────┐
              ▼               ▼               ▼
         .specs/          EventBus         state.json
       (artifacts)     (SSE to browser)   (pipeline state)
```

## Dependency and Go Module Impact

Adding `anthropic-sdk-go` to `mcp-server/go.mod`:

```bash
cd mcp-server && go get github.com/anthropics/anthropic-sdk-go
```

The SDK has minimal transitive dependencies. It does not pull in large frameworks.
The `forge-state-mcp` binary size increase is expected to be negligible.

## Relationship to Existing Research

| Document | Relationship |
|----------|-------------|
| [Remote Dashboard Control](./remote-dashboard-control.md) | Phase 2 §3.5 identifies Go SDK as the preferred runtime. This document details what that choice enables. |
| [Forge Queue Design](./queue-design.md) | Queue design uses `claude -p` (stateless). SDK enables multi-turn sessions, making queued tasks run full pipelines instead of single-shot. |

## Recommendation

Adopt `anthropic-sdk-go` for the Phase 2 Task Runner implementation. The SDK is
the only option that keeps the entire system as a single Go binary with in-process
EventBus integration. Node.js/Python subprocess alternatives (documented in
remote-dashboard-control.md §3.5 as fallbacks) add runtime dependencies and
require cross-process event coordination.

## Next Steps

1. Add `anthropic-sdk-go` to `mcp-server/go.mod` and validate basic API calls
2. Implement `mcp-server/internal/taskrunner/` with SDK-based agent sessions
3. Wire `TaskRunner` into `dashboard.StartOptions` (per remote-dashboard-control.md §3.4)
4. Add `POST /api/task/submit` and `GET /api/tasks` endpoints
5. Dashboard UI: task submission form and task list panel

# Quick Start

## Basic Usage

Invoke the skill from any Claude Code session where the plugin is installed:

```text
/forge <describe your task here>
/forge https://github.com/org/repo/issues/123
/forge https://myorg.atlassian.net/browse/PROJ-456
```

When given a GitHub Issue or Jira URL, the pipeline fetches the issue details and posts the final summary back as a comment.

## Examples

```text
# Simple task with auto-approve
/forge --effort=S --auto Fix the null pointer crash in auth middleware

# Medium task, skip PR creation
/forge --nopr Add retry logic to the API client

# Large task with debug diagnostics
/forge --effort=L --debug Add a new validation layer
```

## Flags

| Flag | Description |
| --- | --- |
| `--effort=<S\|M\|L>` | Force effort level. Determines flow template (light/standard/full). Default: `M`. |
| `--auto` | Skip human checkpoints when AI verdict is APPROVE. REVISE still pauses. |
| `--nopr` | Skip PR creation. Changes committed and pushed but no PR opened. |
| `--debug` | Append Debug Report to `summary.md` with execution diagnostics. |
| `--discuss` | Trigger a pre-pipeline clarification dialogue for plain-text input. |
| _(auto-detected)_ | Resume by providing the spec directory name. No flag needed. |

## Resume an Interrupted Pipeline

Pass the spec directory name. Resume is auto-detected from `.specs/` directory existence:

```text
/forge 20260320-fix-auth-timeout
```

## Abandon a Pipeline

Use the MCP tool:

```text
mcp__forge-state__abandon with workspace: .specs/20260320-fix-auth-timeout
```

Or delete the state file:

```bash
rm .specs/20260320-fix-auth-timeout/state.json
```

## What Happens During a Run

1. **Input validation** — deterministic + semantic checks
2. **Workspace setup** — creates `request.md` and `state.json` in `.specs/`
3. **Analysis phases** — situation analysis and investigation (read-only)
4. **Design** — architect creates `design.md`, reviewer approves or revises
5. **Human checkpoint** — you review the design
6. **Task decomposition** — tasks broken down, reviewed
7. **Implementation** — each task implemented with TDD, then code-reviewed
8. **Verification** — comprehensive review + final build/test verification
9. **PR creation** — commits, pushes, opens PR
10. **Summary** — `summary.md` with improvement report

For detailed phase descriptions, see [Pipeline Flow](/guide/pipeline-flow). For effort-based phase selection, see [Flow Templates](/guide/flow-templates).

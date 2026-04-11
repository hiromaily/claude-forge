## Quick start

Invoke the skill from any Claude Code session where the plugin is installed:

```text
/forge <describe your task here>
/forge https://github.com/org/repo/issues/123
/forge https://myorg.atlassian.net/browse/PROJ-456
```

When given a GitHub Issue or Jira URL, the pipeline fetches the issue details as context and posts the final summary back as a comment. Plain text input works too — it just skips the posting step.

### Flags

| Flag | Description |
| --- | --- |
| `--effort=<effort>` | Force an effort level: `S`, `M`, `L`. Determines flow template (light/standard/full). Skips heuristic detection. Default: `M`. XS is not supported. |
| `--auto` | Skip human checkpoints when the AI reviewer verdict is APPROVE. REVISE verdicts still pause for human input. |
| `--nopr` | Skip PR creation. Changes are committed and pushed to the feature branch, but no pull request is opened. |
| `--debug` | Append a `## Debug Report` section to `summary.md` with execution flow diagnostics (token outliers, retries, revision cycles, missing phase-log entries). Note: `## Improvement Report` is always appended regardless of this flag. |
| `--discuss` | Trigger a pre-pipeline clarification dialogue for plain-text input. Ignored for GitHub Issue and Jira URLs. Suppressed when combined with `--auto`. |
| _(auto-detected)_ | Resume an interrupted pipeline by providing the spec directory name (e.g. `/forge 20260320-fix-auth-timeout`). If the directory exists under `.specs/`, resume is auto-detected. `--resume` is accepted for backward compatibility but has no effect. |

```text
/forge --effort=S --auto Fix the null pointer crash in auth middleware
/forge --nopr Add retry logic to the API client
/forge --debug Add a new validation layer
/forge --discuss Add caching to the search endpoint
```

### Resume an interrupted pipeline

Pass the spec directory name (the folder under `.specs/`). Resume is auto-detected:

```text
/forge 20260320-fix-auth-timeout
```

### Abandon a pipeline

Use the MCP tool from Claude Code:

```text
mcp__forge-state__abandon with workspace: .specs/20260320-fix-auth-timeout
```

Or delete the state file manually:

```bash
rm .specs/20260320-fix-auth-timeout/state.json
```

---

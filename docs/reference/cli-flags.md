# CLI Flags

## Usage

```text
/forge [flags] <task description or URL>
```

## Flags

### `--effort=<S|M|L>`

Force an effort level, which determines the flow template:

| Effort | Template | Skipped Phases |
| --- | --- | --- |
| S | `light` | Task review (4b), Checkpoint B, Comprehensive Review (7) |
| M | `standard` | Task review (4b), Checkpoint B |
| L | `full` | None — all checkpoints mandatory |

Default: `M`. XS is not supported.

### `--auto`

Skip human checkpoints when the AI reviewer verdict is APPROVE or APPROVE_WITH_NOTES (with no CRITICAL findings). REVISE verdicts still pause for human input.

Ignored for effort L — human approval is always required.

### `--nopr`

Skip PR creation. Changes are committed and pushed to the feature branch, but no pull request is opened.

### `--debug`

Append a `## Debug Report` section to `summary.md` with execution flow diagnostics:
- Token outliers
- Retry counts
- Revision cycles
- Missing phase-log entries

Note: `## Improvement Report` is always appended regardless of this flag.

### `--resume`

Resume an interrupted pipeline from its current phase. Provide the spec directory name as the input:

```text
/forge 20260320-fix-auth-timeout --resume
```

Skips the confirmation prompt and enters the pipeline loop immediately.

## Examples

```text
# Small task, auto-approve checkpoints
/forge --effort=S --auto Fix the null pointer crash in auth middleware

# Medium task, no PR
/forge --nopr Add retry logic to the API client

# Large task with debug output
/forge --effort=L --debug Add a new validation layer

# From GitHub Issue
/forge https://github.com/org/repo/issues/123

# From Jira
/forge https://myorg.atlassian.net/browse/PROJ-456

# Resume interrupted pipeline
/forge 20260320-fix-auth-timeout --resume
```

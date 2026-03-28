# Pipeline Summary: mcp-pipeline-init

**Task type:** feature
**Effort:** M
**Flow template:** standard
**Branch:** feature/mcp-pipeline-init
**PR:** https://github.com/hiromaily/claude-forge/pull/84
**Source:** https://github.com/hiromaily/claude-forge/issues/72

## What Was Built

Two new MCP tool handlers that deterministically replace 13 branching decisions (Decisions 1–13) previously made by the LLM when reading `SKILL.md`:

- **`pipeline_init`** — pure detection handler (no side effects): parses flags (`--type`, `--effort`, `--auto`, `--nopr`, `--debug`), detects source type (github_issue/jira_issue/text/workspace), generates workspace path with `YYYYMMDD-<slug>` format, returns structured JSON
- **`pipeline_init_with_context`** — two-call stateful handler: first call returns auto-detected `task_type`, `effort`, `flow_template`, `skipped_phases`, and `needs_user_confirmation` block; second call writes `state.json` and `request.md`, initialises the workspace

## Tasks Completed

| # | Title | Status |
|---|-------|--------|
| 1 | `pipeline_init.go` — pure detection handler | completed |
| 2 | `pipeline_init_with_context.go` — stateful confirmation handler | completed |
| 3 | `pipeline_init_test.go` | completed |
| 4 | `pipeline_init_with_context_test.go` | completed |
| 5 | Register tools in `registry.go`; update `main.go` count | completed |
| 6 | Update tool-count assertions in test files | completed |
| 7 | Update documentation count references | completed |
| 8 | Full build and test verification | completed |

## Comprehensive Review Fixes

- **`source_url` semantics**: `SourceURL` now only populated for `github_issue`/`jira_issue` source types; plain text was incorrectly set as URL for `text`/`workspace` types
- **`request.md` front matter**: Added `source_url` and `source_id` fields required by `SKILL.md` for PR `Closes #<source_id>` and post-to-source phase
- **File permissions**: `os.MkdirAll` `0755→0750` (gosec G301); `os.WriteFile` `0644→0600` (gosec G306)
- **Linter**: `slugify` elseif simplified; `deriveSpecName` uses `strings.Cut`

## Execution Stats

| Metric | Value |
|--------|-------|
| Total tokens | 932,033 |
| Total duration | 2,585.6s (~43 min) |
| Phases run | 17 phase log entries |
| Model | claude-sonnet-4-6 |

## Verification Results

- `go build ./...` — PASS
- `go test -race ./tools/...` — PASS (36 tools, all assertions)
- `go test -race ./...` — PASS (8 packages)
- `bash scripts/test-hooks.sh` — PASS (327 tests)

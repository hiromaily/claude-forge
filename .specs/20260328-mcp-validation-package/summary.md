# Pipeline Summary: MCP Validation Package (Issue #71)

## Request
Port `validate-input.sh` and pipeline artifact validation into a Go MCP package, exposing two new tools: `validate_input` and `validate_artifact`. Remove the shell-based Rule 6 from `pre-tool-hook.sh`.

## What Was Built

### New: `mcp-server/validation/` package
- **`input.go`** — `ValidateInput(arguments string) InputResult`: replicates `validate-input.sh` checks 1–8 in Go. Flag stripping with word-boundary regex, URL pattern matching (GitHub Issues, Jira), workspace detection via `.specs/` substring.
- **`artifact.go`** — `ValidateArtifacts(workspace, phase string) []ArtifactResult`: checks phase artifact files for required headings, verdicts (`APPROVE`/`APPROVE_WITH_NOTES`/`REVISE` for review phases; `PASS`/`PASS_WITH_NOTES`/`FAIL` for phase-6), and non-empty content. Always returns a JSON array.
- **12 testdata fixtures** + comprehensive unit tests (all parallel)

### New MCP tools (total: 32 → 34)
- `validate_input` — MCP handler for `ValidateInput`; returns structured JSON with `valid`, `errors`, `parsed` fields
- `validate_artifact` — MCP handler for `ValidateArtifacts`; always returns a JSON array regardless of phase

### Rule 6 Removal (atomic)
- Removed `check_task_init_guard` function and its call site from `pre-tool-hook.sh`
- Removed 12 Rule 6 test cases from `test-hooks.sh` (339 → 327 tests)
- Added deprecation comment to `validate-input.sh`

### SKILL.md updated
- Step 1 of Input Validation now calls `mcp__forge-state__validate_input` instead of `bash scripts/validate-input.sh`

## Verification
- `go build ./...`: PASS
- `go test ./...` (all 8 packages): PASS, 0 failures
- `golangci-lint run ./...`: 0 issues
- `bash scripts/test-hooks.sh`: 327 passed, 0 failed

## PR
hiromaily/claude-forge#83

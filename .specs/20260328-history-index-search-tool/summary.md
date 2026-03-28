# Pipeline Summary

**Request:** Add history index + `history_search` MCP tool (TF-IDF/BM25 over .specs/)
**Feature branch:** `feature/history-index-search-tool`
**Pull Request:** #88 (https://github.com/hiromaily/claude-forge/pull/88)
**Date:** 20260328

## Tasks

| # | Title | Verdict |
|---|-------|---------|
| 1 | Create `history` package — index types and `Build` | PASS |
| 2 | Create `history` package — `Search` function | PASS |
| 3 | Write unit tests for `history/index.go` | PASS |
| 4 | Write unit tests for `history/search.go` | PASS |
| 5 | Create `tools/history_search.go` handler | PASS |
| 6 | Write unit tests for `tools/history_search.go` | PASS |
| 7 | Wire `HistoryIndex` into `main.go` and `tools/registry.go` | PASS |
| 8 | Update existing test files for new `RegisterAll` signature | PASS |
| 9 | Update documentation counts | PASS |

## Comprehensive Review

**Verdict: CLEAN** — No issues found. Notable observations: `SearchWithSpecsDir` was added as an exported function beyond the design spec (sound extension for testability); `Build()` mutates `h.specsDir` with the resolved path (deterministic, desirable behavior); differential update test uses `futureTime +2s` (works in practice).

## Notes

- The feature branch originally had a pre-push lint failure (`revive` line-length on `RegisterAll` signature). Fixed by reformatting the multi-parameter signature to multi-line style.
- IDE showed false-positive LSP diagnostics throughout (module root confusion — `mcp-server/` is a separate Go module). All actual `go build`/`go test` results were clean.
- Task 5 agent added `SearchWithSpecsDir` and `SpecsDir()` accessor to `history/search.go` to enable clean `specsDir` pass-through in the handler's testable variant.

## Test Results

- Go tests: 9 packages, all PASS (`go test ./... -race`)
- Shell tests: 246 PASS (`bash scripts/test-hooks.sh`)
- Lint: 0 issues (`golangci-lint run`)
- Build: clean (`go build ./...`)

## Execution Stats

| Phase | Tokens | Duration | Model |
|-------|--------|----------|-------|
| phase-1 | 63,886 | 125s | sonnet |
| phase-2 | 97,944 | 412s | sonnet |
| phase-3 | 47,474 | 125s | sonnet |
| phase-3b (×2) | 84,907 | 134s | sonnet |
| phase-4 | 46,031 | 70s | sonnet |
| phase-4b (×2) | 75,412 | 136s | sonnet |
| task-1-impl | 95,785 | 508s | sonnet |
| task-2-impl | 80,255 | 322s | sonnet |
| task-5-impl | 73,340 | 231s | sonnet |
| task-7-impl | 55,231 | 152s | sonnet |
| task-8-impl | 54,993 | 151s | sonnet |
| task-9-impl | 36,281 | 140s | sonnet |
| phase-7 | 62,051 | 155s | sonnet |
| final-verification | 19,235 | 44s | sonnet |
| **TOTAL** | **892,825** | **2,705s** | |

## Improvement Report

*Retrospective on what would have made this work easier.*

### Documentation

- The `mcp-server/` Go module structure (separate `go.mod`) is not documented in CLAUDE.md or README.md as a potential source of IDE confusion. Adding a note that the LSP must be pointed at `mcp-server/` as the module root would save debugging time.
- The `search.IndexEntry` schema (used by `search_patterns`) vs the new `history.IndexEntry` schema are easy to confuse — a short comment at the top of `search/bm25.go` clarifying "this schema is for build-specs-index.sh output, not for history indexing" would help.

### Code Readability

- `tools/registry.go` has grown large (39 tool registrations). A comment grouping tools by category (Lifecycle, Phase, Config, Query, Validation, Utility) at the top would make navigation easier.
- The `resolveSpecsDir` / `resolveAgentDir` pattern appears in two places now. A shared helper in a `util` or `resolve` package would reduce duplication.

### AI Agent Support (Skills / Rules)

- The Go module boundary (`mcp-server/` as a separate module) should be called out in `.claude/rules/golang.md` alongside the existing toolchain setup note — e.g., "all `go` commands must be run from `mcp-server/`, not the repo root."
- A note that `go test ./...` from the repo root will fail (the `history` package path won't be found) would prevent repeated confusion.

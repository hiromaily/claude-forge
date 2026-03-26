# Pipeline Summary

**Request:** Add Go-based BM25 pattern scorer as MCP tool, replacing shell keyword scorer as primary path
**Feature branch:** `feature/bm25-pattern-scoring`
**Pull Request:** #62 (https://github.com/hiromaily/claude-forge/pull/62)
**Date:** 20260326

## Tasks

| # | Title | Verdict |
|---|-------|---------|
| 1 | Implement BM25 scoring package | PASS |
| 2 | Write unit tests for the BM25 package | PASS |
| 3 | Implement the `search_patterns` MCP handler | PASS |
| 4 | Register `search_patterns` tool and update count comments | PASS |
| 5 | Write handler integration tests | PASS |
| 6 | Update SKILL.md call sites | PASS |
| 7 | Update documentation counts and tables | PASS |

## Comprehensive Review

CLEAN — one cosmetic blank-line fix in `handlers_test.go`. No structural issues. All documentation counts internally consistent: `state-manager.sh` = 26 subcommands, total MCP tools = 27.

## Notes

- `TestSearchPatternsHandler` placed in `search_patterns_test.go` rather than `handlers_test.go` — functionally equivalent (same package), better structurally
- `strPtr` helper defined independently in `package search` tests and `package tools` tests — different packages, no conflict
- IDE diagnostics flagged false-positive undefined-reference errors throughout; all packages built and tested clean

## Test Results

- Go tests: all packages pass (`mcp-server/search`, `mcp-server/state`, `mcp-server/tools`)
- Hook tests: 336 passed, 0 failed
- Build: `make build` produces `bin/forge-state-mcp` cleanly

## Execution Stats

| Phase | Tokens | Duration | Model |
|-------|--------|----------|-------|
| phase-1 | 51,536 | 115.7s | sonnet |
| phase-2 | 92,517 | 494.2s | sonnet |
| phase-3 | 39,391 | 141.1s | sonnet |
| phase-3b (×5) | 175,158 | 441.1s | sonnet |
| phase-4 | 40,332 | 86.5s | sonnet |
| phase-4b (×2) | 61,875 | 85.9s | sonnet |
| task-1-impl | 34,094 | 156.1s | sonnet |
| task-2-impl | 34,094 | 156.1s | sonnet |
| task-3-impl | 61,360 | 192.8s | sonnet |
| task-4-impl | 42,763 | 119.0s | sonnet |
| task-5-impl | 72,609 | 326.7s | sonnet |
| task-6-impl | 32,161 | 70.4s | sonnet |
| task-7-impl | 57,540 | 257.6s | sonnet |
| phase-7 | 57,235 | 177.1s | sonnet |
| final-verification | 17,562 | 42.1s | sonnet |
| **TOTAL** | **870,227** | **2863.1s** | |

## Improvement Report

*Retrospective on what would have made this work easier.*

### Documentation

The `index.json` schema is not formally documented anywhere — the investigator had to reverse-engineer `build-specs-index.sh` to determine field names. The `implOutcomes` field caused a CRITICAL design revision (wrong field name `taskTitle` vs actual `reviewFile`) because the schema exists only in the build script's code. A `docs/index-json-schema.md` or inline comment block in `build-specs-index.sh` would have prevented that revision cycle.

### Code Readability

The shell scorer's output format constants (header strings, bullet format strings) are embedded in `printf` calls with no named constants or documentation. The Go handler had to be verified by cross-referencing the shell script character-by-character to ensure parity. Extracting the format strings into named constants in `query-specs-index.sh` (via bash variables) would make parity verification much easier.

### AI Agent Support (Skills / Rules)

The `mcp-server/` module runs `go test` from within the `mcp-server/` directory, not the project root. This caused repeated confusion (`./...` from project root does not find the Go module). A note in `CLAUDE.md` under the "Go module setup" section explicitly stating "run `go test ./...` from inside `mcp-server/`" would prevent this in future Go work.

### Other

The IDE (LSP) produced false-positive `UndeclaredName` diagnostics on every task that created new Go files. These are artifacts of incremental analysis before the package is fully built. A note in `CLAUDE.md` that IDE diagnostics in `mcp-server/` should be verified with `go build ./...` before treating as real errors would save investigation time.

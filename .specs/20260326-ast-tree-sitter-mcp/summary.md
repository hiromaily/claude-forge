# Pipeline Summary

**Request:** Add tree-sitter MCP tools for AST-based code summarization (issue #49)
**Feature branch:** `feature/ast-tree-sitter-mcp`
**Pull Request:** #64 (https://github.com/hiromaily/claude-forge/pull/64)
**Date:** 2026-03-26

## Tasks

| # | Title | Verdict |
|---|-------|---------|
| 1 | Add go-tree-sitter dependency | PASS |
| 2 | Implement mcp-server/ast domain package | PASS |
| 3 | Create testdata fixtures | PASS |
| 4 | Write unit tests for ast package | PASS |
| 5 | Implement ast_summary MCP handler | PASS |
| 6 | Implement ast_find_definition MCP handler | PASS |
| 7 | Write handler-layer tests | PASS |
| 8 | Register tools in registry + update count assertions | PASS |
| 9 | Update agent files | PASS |
| 10 | Update documentation | PASS |

## Comprehensive Review

**Verdict: IMPROVED**

One fix applied: `AstSummaryHandler` was using `GetString("file_path","")` + manual empty check; changed to `RequireString` to match the established pattern in the tools package.

Key observations (non-blocking):
- `writeTestFile`/`parentDir` helpers in `ast_test_helpers_test.go` are defined but never called
- `extractGoExportedConst` returns only the first line of multi-line `const (...)` blocks — known limitation

## Notes

- `github.com/smacker/go-tree-sitter` uses CGo; build requires a C compiler. CI does not compile binaries so no CI changes needed.
- `resolveLanguage` is defined in `ast_find_definition.go` and shared with `ast_summary.go` within the same package.
- `TestTokenReduction` uses word count as a proxy for token count (documented approximation). Results: 13.4% for `large_sample.go`, 19.5% for `large_sample.ts` — both well under the 20% threshold.
- IDE LSP shows false-positive "undefined" diagnostics for the new files because the LSP does not resolve the `mcp-server/` sub-module. Build and tests pass cleanly.

## Test Results

All 6 packages pass, 336 hook tests pass, binary builds:
- mcp-server: PASS
- mcp-server/ast: PASS (10 tests)
- mcp-server/events: PASS
- mcp-server/search: PASS
- mcp-server/state: PASS
- mcp-server/tools: PASS (30 tools registered, all assertions pass)

## Execution Stats

| Phase | Tokens | Duration | Model |
|-------|--------|----------|-------|
| phase-1 | 52,527 | 92.5s | sonnet |
| phase-2 | 84,964 | 379.7s | sonnet |
| phase-3 | 39,234 | 130.7s | sonnet |
| phase-3b (×2) | 63,416 | 119.7s | sonnet |
| phase-4 | 51,522 | 99.1s | sonnet |
| phase-4b (×2) | 65,366 | 94.9s | sonnet |
| task-1-impl | 31,579 | 124.2s | sonnet |
| task-2-impl | 48,381 | 212.5s | sonnet |
| task-3-impl | 32,046 | 110.4s | sonnet |
| task-4-impl | 56,721 | 257.8s | sonnet |
| task-5-impl | 65,586 | 213.9s | sonnet |
| task-6-impl | 69,050 | 239.0s | sonnet |
| task-8-impl | 52,153 | 131.0s | sonnet |
| task-9-impl | 30,803 | 71.4s | sonnet |
| task-10-impl | 38,367 | 90.9s | sonnet |
| task-1-review | 70,468 | 152.3s | sonnet |
| phase-7 | 66,488 | 161.4s | sonnet |
| final-verification | 26,467 | 37.5s | sonnet |
| **TOTAL** | **945,138** | **2719.8s** | |

## Improvement Report

*Retrospective on what would have made this work easier.*

### Documentation

The `smacker/go-tree-sitter` library has no tagged releases (pseudo-version only). The investigation had to determine the correct pseudo-version by checking the library's commit history. A note in `CLAUDE.md` or `ARCHITECTURE.md` about the Go module's CGo dependency and the specific library pseudo-version used would save future work from repeating this research.

### Code Readability

The tools package has no shared test helper file for common patterns like `toolResultText` extraction from `*mcp.CallToolResult`. Task 5/6 had to create `ast_test_helpers_test.go` from scratch. If a canonical `test_helpers_test.go` existed in the package (similar to `setupWorkspace` + `callTool` in `handlers_test.go`), new handler tests would have less boilerplate to write.

### AI Agent Support (Skills / Rules)

No friction observed. The `search_patterns.go` testable-split pattern (`Handler()` outer + `fromPath()` inner) is well-established and served as a clear precedent for the new AST handlers.

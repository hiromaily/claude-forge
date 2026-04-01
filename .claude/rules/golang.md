---
paths: ["**/*.go"]
---

# Go File Rules

## Go version

This project uses **Go 1.26**. Prefer modern language and standard library features:

- `for range` over integers (`for i := range n`) — available since Go 1.22
- `slices` and `maps` packages from the standard library
- `any` instead of `interface{}`
- `t.Context()` in tests instead of `context.Background()`
- `errors.New` for static error strings; `fmt.Errorf("...: %w", err)` for error wrapping
- `new(expr)` for pointer literals — available since Go 1.26

## Pointer literals

Go 1.26 allows passing expressions directly to `new`, eliminating the need for a temporary variable or helper functions like `ToPtr()`:

```go
// Before Go 1.26 — create a temp variable first
n := int64(300)
ptr := &n

// Also before — helper function workaround
func ToPtr[T any](v T) *T { return &v }
ptr := ToPtr(int64(300))

// Go 1.26+ — pass the expression directly
ptr := new(int64(300))
```

**Rules:**
- Do **not** define or use `ToPtr`, `Ptr`, or similar pointer-helper functions.
- Do **not** create a temporary variable solely to take its address.
- Use `new(expr)` for all pointer literals to optional/nullable fields.

## Toolchain setup

All Go commands run from `mcp-server/`. The module lives at `github.com/hiromaily/claude-forge/mcp-server`.

To set up the dev environment (installs lefthook hooks and pins golangci-lint as a `tool` in `go.mod`):

```bash
cd mcp-server && make install
```

`golangci-lint` is pinned as a Go tool dependency (`go get -tool` — available since Go 1.24). Run it via `go tool golangci-lint`, not a globally installed binary. The version is locked in `go.mod` under the `tool` directive.

## Running tests

```bash
cd mcp-server && go test ./...
```

## Git hooks (lefthook)

Lefthook runs automatically on git operations. Both hooks only trigger when `**/*.go` files are staged/pushed, and both run from `mcp-server/`.

| Hook | Command | Behaviour |
|---|---|---|
| `pre-commit` | `make go-fmt` | Formats staged `.go` files and re-stages the fixes (`stage_fixed: true`) |
| `pre-push` | `make go-lint` | Lints and auto-fixes `.go` files before push |

**pre-push detail**: `make go-lint` runs `golangci-lint run --fix`. If it fixes all issues it exits 0 and the push succeeds — but the auto-fixed changes remain unstaged in the working tree. If unfixable issues remain it exits non-zero and blocks the push. After a blocked push: commit the auto-fixed files, then re-push.

## Makefile commands (mcp-server/)

```bash
make go-fmt             # Format all Go files (~2.5s)
make go-lint            # Lint and auto-fix (~65s full run)
make go-lint-check      # Lint without fixing (check-only)
make go-lint-fast       # Lint fast-only linters and auto-fix (~6s)
make go-lint-fast-check # Lint fast-only linters, check-only
make go-lint-verify-config  # Verify .golangci.yml is valid — run after editing it
make go-clean-lint-cache    # Clear golangci-lint cache when results look stale
```

After modifying `.golangci.yml`, always run `make go-lint-verify-config` before committing.

## Linter configuration

Config lives at `mcp-server/.golangci.yml`. Key decisions:

- **depguard is disabled** — no import restrictions; use any package in `go.mod`.
- **Test files are excluded** from: `errcheck`, `errchkjson`, `bodyclose`, `gocyclo`, `dogsled`.
- **gosec excludes** G112 (ReadHeaderTimeout), G204 (subprocess), G304 (file inclusion) — expected patterns for this MCP server.
- **gocyclo threshold is 16.** For inherently complex dispatch tables (large switch statements), suppress with `//nolint:gocyclo // complexity is inherent in the dispatch table` rather than refactoring.
- **revive line-length limit is 200 characters.** Break long string literals with `+` concatenation.

## Error handling conventions

- Always check errors in production code. For deferred close calls where the error is intentionally discarded: `defer func() { _ = f.Close() }()`.
- For `fmt.Fprintf` to an `http.ResponseWriter`: `_, _ = fmt.Fprintf(...)` — write errors on a streaming response cannot be acted on.
- `json.Marshal` errors may be discarded with `_` in test code only. In production code, check the error.

## Receiver naming

- Use a meaningful single-letter receiver name (`m`, `s`, etc.) on all methods, even when the receiver is unused in the body.
- Do **not** use `_` as a receiver name — staticcheck ST1006 rejects it.
- If a receiver is genuinely unused (e.g., a stub method), keep the named receiver and add `//nolint:revive // m intentionally unused; stub` to suppress `unused-receiver`.

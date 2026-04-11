---
paths: ["**/*_test.go"]
---

# Go Test Rules

These rules apply to all `*_test.go` files in `mcp-server/`. Read them before writing any test.

## Package declaration

Choose based on whether the test needs unexported symbols:

- **Same package** (`package orchestrator`): when the test needs access to unexported types, functions, or variables. Used in `orchestrator/` and other pure-logic packages.
- **External package** (`package state_test`): when tests should only touch the exported API. Used in `state/` and `tools/` to keep tests honest about the public surface.

Do not mix both styles in the same directory.

## Parallelism

Every test function and every `t.Run` subtest **must** call `t.Parallel()` as its first statement. This is enforced by the `tparallel` linter.

```go
func TestFoo(t *testing.T) {
    t.Parallel()

    t.Run("case", func(t *testing.T) {
        t.Parallel() // required inside every t.Run subtest
        ...
    })
}
```

Exception: tests that mutate OS-level global state (environment variables, working directory) must NOT call `t.Parallel()`. Add a comment explaining why.

In Go 1.22+, the loop variable in `for _, tc := range tests` is scoped per iteration — `tc := tc` capture workarounds are not needed and must not be added.

## Table-driven tests

Use table-driven tests with `t.Run` when two or more cases share the same function signature and assertion pattern. Do not write individual `TestFoo_CaseA`, `TestFoo_CaseB` functions for structurally identical cases — merge them into one table.

```go
tests := []struct {
    name  string
    input string
    want  string
}{
    {name: "foo", input: "a", want: "b"},
    {name: "bar", input: "c", want: "d"},
}
for _, tc := range tests {
    t.Run(tc.name, func(t *testing.T) {
        t.Parallel()
        got := fn(tc.input)
        if got != tc.want {
            t.Errorf("fn(%q) = %q, want %q", tc.input, got, tc.want)
        }
    })
}
```

For 2D matrix tests (e.g. `sourceType × effort`), use `tc.sourceType+"/"+tc.effort` as the subtest name — the `/` creates a hierarchy that makes `-run` filtering precise.

## Subtest naming

Use lowercase, underscore-separated names: `"flag_override"`, `"jira_bug"`, `"default_empty_inputs"`. Names must be unique within the table. Avoid names that are just a number or a raw enum value with no context.

## Struct comparison

Use `reflect.DeepEqual` to compare structs, slices, or maps in one assertion rather than checking each field individually. Import `"reflect"` in the import block.

```go
if !reflect.DeepEqual(got, want) {
    t.Errorf("got %+v, want %+v", got, want)
}
```

When a `want` struct only sets the fields a constructor should populate, unset fields default to zero values — this implicitly asserts cross-variant fields are zero without extra `if` statements.

## Fatal vs. Error

- `t.Fatalf` / `t.Fatal`: use when subsequent assertions would panic or be meaningless if this check fails (e.g., checking `len(slice)` before indexing).
- `t.Errorf` / `t.Error`: use for independent assertions that should all run even if one fails.

```go
if len(findings) != 2 {
    t.Fatalf("findings count = %d, want 2; got %v", len(findings), findings)
}
// safe to index now
if findings[0].Severity != SeverityMinor {
    t.Errorf("findings[0].Severity = %q, want %q", findings[0].Severity, SeverityMinor)
}
```

## Error message format

Follow `got X, want Y` order. Include the inputs that produced the result so a failing test is self-explanatory without reading the source:

```go
t.Errorf("fn(%q, %q) = %q, want %q", input1, input2, got, want)
```

## Temporary directories

Use `t.TempDir()` for scratch directories. The directory and its contents are deleted automatically after the test. Never call `os.MkdirAll` or `os.Remove` manually.

```go
dir := t.TempDir()
path := filepath.Join(dir, "state.json")
```

## Test helpers

Mark helper functions with `t.Helper()` so failure lines point to the call site, not the helper body. Name helpers with a verb (`loadState`, `writeFileForTest`, `newManager`).

```go
func loadState(t *testing.T, workspace string) State {
    t.Helper()
    data, err := os.ReadFile(filepath.Join(workspace, "state.json"))
    if err != nil {
        t.Fatalf("loadState: %v", err)
    }
    var s State
    if err := json.Unmarshal(data, &s); err != nil {
        t.Fatalf("loadState unmarshal: %v", err)
    }
    return s
}
```

## Cleanup

Use `t.Cleanup` to register teardown logic that must run regardless of test outcome. Prefer it over deferred calls when the cleanup needs access to `t` for error reporting.

```go
srv := startTestServer(t)
t.Cleanup(func() { srv.Close() })
```

## Stateful object setup

For packages with stateful objects, define a `newFoo()` factory that returns a fresh zero-state instance. Call `t.TempDir()` in the test body, not in the factory.

```go
func newManager() *StateManager {
    return NewStateManager()
}

func TestSomething(t *testing.T) {
    t.Parallel()
    dir := t.TempDir()
    m := newManager()
    if err := m.Init(dir, "spec"); err != nil {
        t.Fatalf("Init: %v", err)
    }
    ...
}
```

## Context

Use `t.Context()` instead of `context.Background()`. The test context is cancelled when the test ends, which surfaces context-leak bugs early. See also `golang.md`.

## Testdata fixtures

Place fixture files in a `testdata/` directory adjacent to the test file (e.g., `orchestrator/testdata/`). Go tooling ignores this directory during regular builds. Use a helper to build the path:

```go
func testdataPath(name string) string {
    return filepath.Join("testdata", name)
}
```

Name fixtures after their content, not their index: `review-design-approve.md`, not `fixture1.md`.

## Race detector

Run tests with the race detector when testing concurrent code or stateful managers:

```bash
cd mcp-server && go test -race ./...
```

The CI pipeline always runs with `-race`. If a test is not safe to run with `-race`, it is a bug — fix the production code, not the test.

## What not to do

- Do not use `assert` or `require` from testify — use the stdlib `testing` package only.
- Do not use `os.Exit` or `log.Fatal` — use `t.Fatalf`.
- Do not add `t.Skip(...)` without a comment explaining the condition under which the skip should be removed.
- Do not share mutable package-level variables between parallel tests.
- Do not add `tc := tc` loop capture — it is unnecessary in Go 1.22+ and adds noise.
- `json.Marshal` errors may be discarded with `_` in test code; in production code they must be checked.

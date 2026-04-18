# Go Package Layering

The `mcp-server/internal/` packages form a strict one-way import DAG. Violating this direction causes an import cycle and a build failure; `import_cycle_test.go` enforces it.

```
tools  →  orchestrator  →  state
  │            │               ↑
  │            ↓               │
  ├──→  sourcetype  ──→  maputil
  │
  └──→  (shared: history, profile, prompt, validation, events)
```

## Packages

| Package | Purpose | May import |
|---------|---------|-----------|
| `state` | Persistence layer — `State` struct, `StateManager`, phase constants, artifact names | stdlib only |
| `maputil` | Generic map field extraction (`StringField`, `IntFieldAlt`, `StringArray`, `ToMap`) | stdlib only (leaf package) |
| `sourcetype` | Source type Handler interface + registry (GitHub, Jira, Linear) — URL classification, fetch/post config, external context parsing | `state`, `maputil` |
| `orchestrator` | Pipeline state machine (`Engine.NextAction`), action types, effort detection | `state`, `sourcetype` |
| `tools` | MCP handlers wrapping `orchestrator` with enrichment (agent prompts, history) | `state`, `sourcetype`, `maputil`, `orchestrator`, shared packages |
| `validation` | Input validation (URL format, flags, length checks) | `sourcetype` |
| Shared (`history`, `profile`, `prompt`, `events`) | Cross-cutting utilities | `state` |

## Rules

- `state` must never import `orchestrator`, `tools`, or `sourcetype`.
- `maputil` must never import any internal package (leaf package).
- `sourcetype` must never import `orchestrator` or `tools`.
- `orchestrator` must never import `tools`.
- `tools` may import any package below it.
- Shared packages (`history`, `profile`, `prompt`, `validation`, `events`) may import `state` and `sourcetype` but must not import `orchestrator` or `tools`.

## Adding a New Source Type

Adding a new source type (e.g., Asana) requires **exactly one file + one registration**:

1. Create `internal/sourcetype/asana.go` implementing the `Handler` interface
2. Add `func init() { register(&AsanaHandler{}) }` in that file

The `Handler` interface enforces all required methods at compile time. No other files need changes — `validation`, `tools`, and `orchestrator` all dispatch through the `sourcetype` registry.

## Why

`state` is the persistence layer with no domain logic. `maputil` is a pure utility leaf package. `sourcetype` centralises all source-type-specific knowledge behind a single interface, eliminating scattered switch statements. `orchestrator` contains the pipeline state machine (`Engine.NextAction`). `tools` wraps `orchestrator` in MCP handlers and adds enrichment (agent prompts, history search). Keeping this direction one-way ensures each layer can be tested without mocking its dependents.

Go MCP handlers are NOT fail-open for their own operations — guard failures return `IsError=true`. However, the MCP server being unavailable does not block shell-level operations (the two layers are independent).

## Enforcement

`import_cycle_test.go` in `mcp-server/` verifies the DAG on every `go test` run. Adding a reverse import will fail the test with a cycle error.

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
| `engine/state` | Persistence layer — `State` struct, `StateManager`, phase constants, artifact names | stdlib only |
| `pkg/maputil` | Generic map field extraction (`StringField`, `IntFieldAlt`, `StringArray`, `ToMap`) | stdlib only (leaf package) |
| `engine/sourcetype` | Source type Handler interface + registry (GitHub, Jira, Linear) — URL classification, fetch/post config, external context parsing | `engine/state`, `pkg/maputil` |
| `engine/orchestrator` | Pipeline state machine (`Engine.NextAction`), action types, effort detection | `engine/state`, `engine/sourcetype` |
| `handler/tools` | MCP handlers wrapping `engine/orchestrator` with enrichment (agent prompts, history) | `engine/state`, `engine/sourcetype`, `pkg/maputil`, `engine/orchestrator`, shared packages |
| `handler/validation` | Input validation (URL format, flags, length checks) | `engine/sourcetype` |
| Shared (`history`, `profile`, `prompt`, `events`) | Cross-cutting utilities | `engine/state` |

## Rules

- `engine/state` must never import `engine/orchestrator`, `handler/tools`, or `engine/sourcetype`.
- `pkg/maputil` must never import any internal package (leaf package).
- `engine/sourcetype` must never import `engine/orchestrator` or `handler/tools`.
- `engine/orchestrator` must never import `handler/tools`.
- `handler/tools` may import any package below it.
- Shared packages (`history`, `profile`, `prompt`, `handler/validation`, `events`) may import `engine/state` and `engine/sourcetype` but must not import `engine/orchestrator` or `handler/tools`.

## Adding a New Source Type

Adding a new source type (e.g., Asana) requires **exactly one file + one registration**:

1. Create `internal/engine/sourcetype/asana.go` implementing the `Handler` interface
2. Add `func init() { register(&AsanaHandler{}) }` in that file

The `Handler` interface enforces all required methods at compile time. No other files need changes — `handler/validation`, `handler/tools`, and `engine/orchestrator` all dispatch through the `engine/sourcetype` registry.

## Why

`engine/state` is the persistence layer with no domain logic. `pkg/maputil` is a pure utility leaf package. `engine/sourcetype` centralises all source-type-specific knowledge behind a single interface, eliminating scattered switch statements. `engine/orchestrator` contains the pipeline state machine (`Engine.NextAction`). `handler/tools` wraps `engine/orchestrator` in MCP handlers and adds enrichment (agent prompts, history search). Keeping this direction one-way ensures each layer can be tested without mocking its dependents.

Go MCP handlers are NOT fail-open for their own operations — guard failures return `IsError=true`. However, the MCP server being unavailable does not block shell-level operations (the two layers are independent).

## Enforcement

`import_cycle_test.go` in `mcp-server/` verifies the DAG on every `go test` run. Adding a reverse import will fail the test with a cycle error.

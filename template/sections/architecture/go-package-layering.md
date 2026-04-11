# Go Package Layering

The `mcp-server/internal/` packages form a strict one-way import DAG. Violating this direction causes an import cycle and a build failure; `import_cycle_test.go` enforces it.

```
tools  →  orchestrator  →  state
  │              │
  └──────────────┴──→  (shared packages: history, profile, prompt, validation, events)
```

## Rules

- `state` must never import `orchestrator` or `tools`.
- `orchestrator` must never import `tools`.
- `tools` may import any package below it.
- Shared packages (`history`, `profile`, `prompt`, `validation`, `events`) may import `state` but must not import `orchestrator` or `tools`.

## Why

`state` is the persistence layer with no domain logic. `orchestrator` contains the pipeline state machine (`Engine.NextAction`). `tools` wraps `orchestrator` in MCP handlers and adds enrichment (agent prompts, history search). Keeping this direction one-way ensures each layer can be tested without mocking its dependents.

Go MCP handlers are NOT fail-open for their own operations — guard failures return `IsError=true`. However, the MCP server being unavailable does not block shell-level operations (the two layers are independent).

## Enforcement

`import_cycle_test.go` in `mcp-server/` verifies the DAG on every `go test` run. Adding a reverse import will fail the test with a cycle error.

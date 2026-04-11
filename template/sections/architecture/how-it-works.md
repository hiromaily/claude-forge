## How it works

The pipeline is built on three core principles:

1. **Files are the API** — Each phase writes a markdown artifact to `.specs/{date}-{name}/`. The next phase reads those files, never the conversation history. This keeps every agent's context small and focused.
2. **State on disk** — All progress is tracked in `state.json`, so pipelines survive context compaction and session restarts. Hooks read this state to enforce constraints.
3. **Engine-driven control** — The Go MCP server (`forge-state-mcp`) owns all orchestration decisions: which phase runs next, skip conditions, retry limits, artifact validation, and checkpoint gating. The LLM follows typed actions returned by `pipeline_next_action` — it cannot invent or skip steps. Shell hooks enforce a complementary set of OS-level invariants (read-only analysis, no parallel commits, session stop guards) that hold regardless of the LLM's behavior.

For the full data flow, state machine, hook architecture, agent input/output matrix, and concurrency model, browse [`docs/architecture/`](../../../docs/architecture/) directly.

---

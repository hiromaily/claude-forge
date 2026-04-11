## How it works

The pipeline is built on three core principles:

1. **Files are the API** — Each phase writes a markdown artifact to `.specs/{date}-{name}/`. The next phase reads those files, never the conversation history. This keeps every agent's context small and focused.
2. **State on disk** — All progress is tracked in `state.json`, so pipelines survive context compaction and session restarts. Hooks read this state to enforce constraints.
3. **Two-layer compliance** — Critical invariants (read-only analysis, no parallel commits, checkpoint gates) are enforced both by agent instructions (probabilistic) and hook scripts (deterministic, fail-open).

For the full data flow, state machine, hook architecture, agent input/output matrix, and concurrency model, browse [`docs/architecture/`](../../../docs/architecture/) directly.

---

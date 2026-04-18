## Design decisions

Key choices that shape the plugin's architecture:

- **Agents inherit the user's configured model** — no `model:` key is set in agent frontmatter. Users control model selection via their Claude Code configuration. Pin individual agents to a specific model by adding `model: <name>` to their frontmatter if needed.
- **The orchestrator never reads source code** — only small artifact files, keeping its context window lean.
- **Parallel implementation with mkdir-based locking** — macOS lacks `flock`, so atomic `mkdir` is used instead. Parallel agents skip `git commit`; the orchestrator batch-commits after the group finishes.

See [docs/architecture/technical-decisions.md](../../../docs/architecture/technical-decisions.md) for full rationale on these and other decisions (fail-open hooks, file-based state, agent separation).

---

# Claude-Forge Plugin — Architecture

Full architecture documentation lives in [`docs/architecture/`](docs/architecture/). This file is an index.

## Index

| Document | Contents |
|---|---|
| [Overview](docs/architecture/overview.md) | Component diagram, responsibility matrix, directory structure |
| [Design Principles](docs/architecture/design-principles.md) | Files-are-the-API, separation of concerns, state on disk, two-layer compliance, fail-open hooks |
| [Runtime Flow](docs/architecture/runtime-flow.md) | Component interaction (single phase), MCP pipeline tool mapping, initialisation flow |
| [Pipeline Sequence](docs/architecture/pipeline-sequence.md) | Full pipeline sequence diagram (effort L — all phases) |
| [Data Flow](docs/architecture/data-flow.md) | Linear data flow diagram, what each agent reads, file-writing responsibility, specs index system |
| [State Management](docs/architecture/state-management.md) | State machine, state.json schema, MCP tool categories, MCP handler guards |
| [Effort-driven Flow](docs/architecture/effort-flow.md) | Effort levels, flow templates, skip sets, upfront-skip pattern, effort detection, resume behaviour |
| [Concurrency Model](docs/architecture/concurrency.md) | Phase 5 parallel execution, mutex locking, hook enforcement |
| [Hooks & Guardrails](docs/architecture/hooks.md) | Hook types, exit code semantics, PreToolUse rules, stop hook, testing |
| [Human Interaction Points](docs/architecture/human-interaction.md) | All points where user input is required or pipeline pauses |
| [Key Technical Decisions](docs/architecture/technical-decisions.md) | Rationale for mkdir locking, fail-open hooks, sonnet-only agents, orchestrator token economy, guard migration pattern |
| [Guard Catalogue](docs/architecture/guard-catalogue.md) | Complete enforcement reference: blocking guards, warnings, engine decisions, artifact validation |
| [Go Package Layering](docs/architecture/go-package-layering.md) | `tools → orchestrator → state` import DAG and enforcement |

# Introduction

## What is claude-forge?

claude-forge is a **Claude Code plugin** that orchestrates a multi-phase development pipeline using isolated subagents. It automates the handoff chain between analysis, design, implementation, and review — replacing manual orchestration with a structured system.

## The Problem

The AI development landscape has evolved through three phases:

1. **Vibe coding** — "Write me a function that does X." Works for small tasks. Breaks as complexity grows.
2. **Spec-Driven Development (SDD)** — Write a spec first, then hand it to AI. Better, but you're still the orchestrator managing every handoff.
3. **Pipeline automation** — You describe a task once; the system runs the full workflow, enforces constraints, and self-reports on where it got stuck.

claude-forge is built for phase 3. It addresses the deployment overhang identified in Anthropic's research — models can handle far more autonomy than humans actually grant them.

## Key Differentiators

### SDD is manual — claude-forge isn't

SDD tells you *what* to do at each phase. It doesn't *run* the phases. claude-forge automates the full handoff chain. Each phase writes a markdown artifact. The next phase reads it. No context sharing — just structured files as the API between agents.

### Automatic improvement loop

After every run, claude-forge emits an **Improvement Report** identifying:
- Documentation gaps that slowed agents down
- Missing conventions that caused clarification loops
- Token-heavy phases caused by poorly structured context

### Effort-aware flow selection

Not every task needs all phases. claude-forge selects the pipeline template based on effort level (S / M / L) — from a lean light pipeline to a full run with 10+ agents and mandatory checkpoints.

### Deterministic guardrails

Critical constraints are enforced at the shell level via Claude Code hooks — not just prompt instructions:
- **Read-only guard** — blocks source edits during analysis phases
- **Commit guard** — prevents git commits during parallel task execution
- **Checkpoint gate** — blocks progression until artifacts exist and human approval is recorded

## Feature List

- Multi-phase pipeline with 10 specialist agents
- Effort-aware scaling (S/M/L → light/standard/full flow templates)
- Deterministic hook guardrails (PreToolUse/PostToolUse/Stop)
- AI review loops (APPROVE/REVISE cycles)
- Parallel implementation with mkdir-based atomic locking
- Human checkpoints with optional auto-approve
- Disk-based state machine (47 MCP tools)
- Resume and abandon support
- Input validation (deterministic + semantic)
- Per-phase token/duration metrics
- GitHub Issue and Jira Issue integration
- Automatic PR creation
- Past implementation pattern injection (BM25 scoring)
- Comprehensive test suite (58+ hook tests + Go MCP server tests)
- Debug report mode

## Next Steps

- [Installation](/guide/installation) — set up the plugin
- [Quick Start](/guide/quick-start) — run your first pipeline
- [Pipeline Flow](/guide/pipeline-flow) — understand the phases

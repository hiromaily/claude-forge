# Agents Overview

claude-forge uses 10 specialist agents, each handling a single phase of the pipeline. Every agent runs in an isolated context window with its own system prompt defined in a `.md` file.

## Agent Roster

| Agent | Phase | Role |
|-------|-------|------|
| [situation-analyst](/agents/situation-analyst) | 1 | Read-only codebase explorer — maps files, interfaces, types, data flows |
| [investigator](/agents/investigator) | 2 | Deep-dive researcher — root causes, edge cases, integration points |
| [architect](/agents/architect) | 3 | Software designer — approach, architecture, data model, test strategy |
| [design-reviewer](/agents/design-reviewer) | 3b | Design quality gate — APPROVE or REVISE with findings |
| [task-decomposer](/agents/task-decomposer) | 4 | Breaks design into numbered, dependency-aware tasks |
| [task-reviewer](/agents/task-reviewer) | 4b | Task list quality gate — APPROVE or REVISE |
| [implementer](/agents/implementer) | 5 | TDD developer — tests first, then code, one task at a time |
| [impl-reviewer](/agents/impl-reviewer) | 6 | Diff-based code reviewer — PASS, PASS_WITH_NOTES, or FAIL |
| [comprehensive-reviewer](/agents/comprehensive-reviewer) | 7 | Cross-task holistic reviewer — naming, duplication, coherence |
| [verifier](/agents/verifier) | Final | Runs full typecheck and test suite, fixes new failures |

## Invocation

Agents are invoked by the orchestrator using the **Agent tool** with the agent's `name`. The orchestrator passes only runtime parameters (workspace path, task number) — agent instructions are self-contained.

## Phase Skipping by Effort Level

Which phases (and their agents) execute depends on the **effort level**, not task type:

| Effort | Template | Skipped Phases |
|--------|----------|----------------|
| S | light | phase-4b (task-reviewer), checkpoint-b, phase-7 (comprehensive-reviewer) |
| M | standard | phase-4b (task-reviewer), checkpoint-b |
| L | full | _(none)_ |

## Model Configuration

Agents inherit the user's configured model by default — no `model:` key is set in agent frontmatter. This means agents run on whatever model the user has selected in their Claude Code configuration. To pin a specific agent to a particular model, add `model: <name>` to that agent's `.md` frontmatter.

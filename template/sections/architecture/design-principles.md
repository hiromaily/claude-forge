# Design Principles

## 1. Files Are the API

Every phase writes its output to a markdown file in the workspace directory. Subsequent phases read only those files — never the conversation history.

This means:
- Each subagent starts with a **clean context window**
- The orchestrator never accumulates large code blocks
- Interruption and resume are possible because all progress is on disk

```
.specs/20260320-fix-auth/
├── request.md              ← user input
├── analysis.md             ← Phase 1 output
├── investigation.md        ← Phase 2 output
├── design.md               ← Phase 3 output
├── review-design.md        ← Phase 3b output
├── tasks.md                ← Phase 4 output
├── review-tasks.md         ← Phase 4b output
├── impl-1.md               ← Phase 5 output (task 1)
├── review-1.md             ← Phase 6 output (task 1)
├── comprehensive-review.md ← Phase 7 output
├── summary.md              ← final-summary phase output
└── state.json              ← Pipeline state
```

> No artifact files are created for `final-verification` or `pr-creation` phases — they operate directly on the codebase and git.

## 2. Separation of Concerns

The orchestrator passes only `{workspace}`, `{N}`, `{spec-name}`, etc. Agents know what files to read and what format to output from their own definitions.

This separation ensures:
- Agent instructions are **self-contained** — no duplication in SKILL.md
- The orchestrator remains **lightweight** — it never reads source code
- Agents can be **upgraded independently** without touching the orchestrator

## 3. State on Disk, Not in Memory

`state.json` is the single source of truth for pipeline progress. This solves:

- **Context compaction** — when Claude compresses conversation history, state survives
- **Session restart** — re-invoke the skill with a workspace path to resume
- **Hook coordination** — hooks read state.json to know what phase is active

## 4. Two-Layer Compliance

Critical invariants are enforced both by agent instructions (probabilistic) and hook scripts (deterministic):

| Invariant | Prompt (probabilistic) | Hook (deterministic) |
|-----------|----------------------|---------------------|
| Phase 1-2 read-only | "Do NOT write files" in agent.md | PreToolUse blocks Edit/Write |
| No parallel git commit | "Do NOT commit" in agent.md | PreToolUse blocks git commit |
| Checkpoint before complete | "Call checkpoint" in SKILL.md | MCP handler blocks phase_complete |
| Artifact before advance | "Write artifact" in SKILL.md | MCP handler blocks phase_complete |
| Pipeline completion | "Write summary.md" in SKILL.md | Stop hook blocks premature stop |

**Prefer deterministic enforcement over prompt instructions.** When adding pipeline behavior, first consider whether a hook script can enforce it.

## 5. Fail-Open Hooks

All hooks are fail-open: if jq is missing or state.json can't be read, the action is **allowed**. This prevents hooks from breaking non-pipeline work.

## 6. Cost-Optimized Agent Selection

All agents use `model: sonnet` by default — cost optimization for 10+ agent invocations per run. Individual agents can be upgraded to `opus` when stronger reasoning is needed.

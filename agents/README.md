# Claude-Forge Agents

Specialized agents for the [claude-forge](../skills/forge/SKILL.md) orchestrator. Each agent handles a single phase of the pipeline, keeping concerns separated and context windows clean.

## Agent Roster

| Agent | Phase | Role | Task-type notes |
|-------|-------|------|-----------------|
| [`situation-analyst`](situation-analyst.md) | 1 | Read-only codebase explorer. Maps relevant files, interfaces, types, data flows, and tests for a given task. Produces a structural index — never proposes changes. | All types |
| [`investigator`](investigator.md) | 2 | Deep-dive researcher. Builds on the situation analysis to uncover root causes, edge cases, integration points, deletion/rename impacts, and open questions requiring human decisions. | Skipped for `docs` |
| [`analyst`](analyst.md) | 1+2 (lite) | Merged situation analysis + investigation for the lite flow template. Reads request.md, produces both analysis.md and investigation.md in a single pass. Only invoked when flowTemplate == "lite". | Lite template only |
| [`architect`](architect.md) | 3 | Software designer. Synthesizes analysis and investigation into a concrete design document: approach rationale, architectural changes, data model updates, test strategy, and risk mitigation. `investigation.md` may be absent for task types that skip Phase 2. | Skipped for `docs` (stub `design.md` written by orchestrator) |
| [`design-reviewer`](design-reviewer.md) | 3b | Critical quality gate for designs. Checks coverage, completeness, consistency, test strategy, contradictions, and scope creep. Outputs **APPROVE** or **REVISE** (CRITICAL findings only) with specific findings. | Skipped for `bugfix`, `docs`, `refactor` |
| [`task-decomposer`](task-decomposer.md) | 4 | Breaks an approved design into numbered, dependency-aware implementation tasks with file assignments, acceptance criteria, and parallel/sequential markers. | Skipped for `bugfix`, `investigation`, `docs` (stub `tasks.md` written by orchestrator) |
| [`task-reviewer`](task-reviewer.md) | 4b | Critical quality gate for task lists. Verifies design coverage, deletion tasks, test updates, dependency correctness, parallel safety, and acceptance criteria specificity. Outputs **APPROVE** or **REVISE** (CRITICAL findings only). | Skipped for `bugfix`, `investigation`, `docs` |
| [`implementer`](implementer.md) | 5 | Focused developer. Implements exactly one task using TDD — writes tests first, then code, then verifies. Respects commit strategy (self-commit for sequential, skip for parallel). For `bugfix`/`docs`, reads stub `tasks.md` and `design.md` from the orchestrator. | Skipped for `investigation` |
| [`impl-reviewer`](impl-reviewer.md) | 6 | Code reviewer. Evaluates a completed task against acceptance criteria, design alignment, test quality, code quality, and regression status. Outputs **PASS**, **PASS_WITH_NOTES**, or **FAIL**. | Skipped for `investigation` |
| [`comprehensive-reviewer`](comprehensive-reviewer.md) | 7 | Holistic cross-task reviewer. Examines the full feature branch diff for naming consistency, code duplication, interface coherence, completeness, and test coverage gaps. Fixes issues directly. Outputs **CLEAN** or **IMPROVED**. | Skipped for `bugfix`, `investigation`, `docs` |
| [`verifier`](verifier.md) | Final | Final quality gate. Runs full typecheck and test suite on the feature branch, distinguishes pre-existing failures from new ones, and fixes new failures before declaring the branch clean. | Skipped for `investigation` |

## Invocation

Agents are invoked by the orchestrator (`forge` skill) using the **Agent tool** with the agent's `name`. Each agent's `.md` file serves as its system prompt. The orchestrator passes only runtime parameters (workspace path, task number, etc.) — agent instructions are self-contained.

See [SKILL.md - Agent Invocation Convention](../skills/forge/SKILL.md#agent-invocation-convention) for details.

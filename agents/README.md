# Dev-Pipeline Agents

Specialized agents for the [dev-pipeline](../skills/dev-pipeline/SKILL.md) orchestrator. Each agent handles a single phase of the pipeline, keeping concerns separated and context windows clean.

## Agent Roster

| Agent | Phase | Role |
|-------|-------|------|
| [`situation-analyst`](situation-analyst.md) | 1 | Read-only codebase explorer. Maps relevant files, interfaces, types, data flows, and tests for a given task. Produces a structural index — never proposes changes. |
| [`investigator`](investigator.md) | 2 | Deep-dive researcher. Builds on the situation analysis to uncover root causes, edge cases, integration points, deletion/rename impacts, and open questions requiring human decisions. |
| [`architect`](architect.md) | 3 | Software designer. Synthesizes analysis and investigation into a concrete design document: approach rationale, architectural changes, data model updates, test strategy, and risk mitigation. |
| [`design-reviewer`](design-reviewer.md) | 3b | Critical quality gate for designs. Checks coverage, completeness, consistency, test strategy, contradictions, and scope creep. Outputs **APPROVE** or **REVISE** with specific findings. |
| [`task-decomposer`](task-decomposer.md) | 4 | Breaks an approved design into numbered, dependency-aware implementation tasks with file assignments, acceptance criteria, and parallel/sequential markers. |
| [`task-reviewer`](task-reviewer.md) | 4b | Critical quality gate for task lists. Verifies design coverage, deletion tasks, test updates, dependency correctness, parallel safety, and acceptance criteria specificity. Outputs **APPROVE** or **REVISE**. |
| [`implementer`](implementer.md) | 5 | Focused developer. Implements exactly one task using TDD — writes tests first, then code, then verifies. Respects commit strategy (self-commit for sequential, skip for parallel). |
| [`impl-reviewer`](impl-reviewer.md) | 6 | Code reviewer. Evaluates a completed task against acceptance criteria, design alignment, test quality, code quality, and regression status. Outputs **PASS**, **PASS_WITH_NOTES**, or **FAIL**. |
| [`verifier`](verifier.md) | Final | Final quality gate. Runs full typecheck and test suite on the feature branch, distinguishes pre-existing failures from new ones, and fixes new failures before declaring the branch clean. |

## Invocation

Agents are invoked by the orchestrator (`dev-pipeline` skill) using the **Agent tool** with the agent's `name`. Each agent's `.md` file serves as its system prompt. The orchestrator passes only runtime parameters (workspace path, task number, etc.) — agent instructions are self-contained.

See [SKILL.md - Agent Invocation Convention](../skills/dev-pipeline/SKILL.md#agent-invocation-convention) for details.

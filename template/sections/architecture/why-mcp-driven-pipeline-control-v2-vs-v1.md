## Architecture: MCP-driven pipeline engine (v2)

claude-forge v1 used shell scripts (`state-manager.sh`) for state management and relied on SKILL.md prompt instructions for all orchestration decisions — which phase to run next, whether to retry, when to skip. The LLM was both the executor *and* the decision-maker.

v2 replaced this with a Go MCP server (`forge-state-mcp`) that owns all pipeline logic. The LLM orchestrator now follows a strict **ask → execute → report** loop: it calls `pipeline_next_action` to get the next action, executes it, and calls `pipeline_report_result` to advance state. It never decides what to do next.

### The shift: LLM as executor, not decision-maker

```
v1: User → SKILL.md (LLM decides next phase) → shell scripts (state I/O)
v2: User → SKILL.md (LLM executes actions)  → Go Engine (decides next phase) → MCP tools (state + guards)
```

In v1, if the LLM misinterpreted a skip condition or forgot to call `phase-complete`, the pipeline broke silently. In v2, the Engine returns a typed action (`spawn_agent`, `checkpoint`, `exec`, `write_file`, `done`) — the LLM cannot invent steps or skip them.

### Comparison

| Aspect | v1 (Skill + shell scripts) | v2 (MCP Engine) |
| --- | --- | --- |
| **Who decides the next phase** | LLM interprets SKILL.md instructions | `Engine.NextAction()` in Go — deterministic |
| **State management** | Shell script (`state-manager.sh`) with `jq` | Go `StateManager` with typed fields and mutex locking |
| **Guard enforcement** | Shell hooks only (exit 2) | Two-layer: Go MCP handler guards + shell hooks |
| **Phase transition reliability** | Probabilistic — LLM may skip or misordering | Deterministic — Engine enforces canonical PHASES order |
| **Retry / revision logic** | SKILL.md prose ("if REVISE, re-run Phase 3") | Engine tracks `designRevisions`, `taskRevisions`, `implRetries` with hard limits |
| **Artifact validation** | None — LLM checks file existence ad-hoc | `validation/artifact.go` — content rules per phase, blocking |
| **Parallel task dispatch** | SKILL.md instructions + hook blocking | Engine returns `parallel_task_ids`; hook blocks git commit |
| **Auto-approve logic** | SKILL.md conditional ("if --auto and APPROVE…") | Engine evaluates `autoApprove` + verdict + findings deterministically |
| **Skip-phase computation** | LLM reads effort table and calls skip-phase | `SkipsForEffort()` returns canonical skip list |
| **Resume** | LLM reads state.json and figures out where it was | `pipeline_next_action` reads state and returns the exact next step |
| **Analytics** | None | `analytics.Collector`, `Estimator`, `Reporter` — token/cost/duration tracking |
| **Cross-pipeline learning** | None | `history.HistoryIndex` (BM25), `KnowledgeBase` (patterns, friction) |
| **Repo profiling** | None | `profile.RepoProfiler` — language, CI, linter detection for prompt enrichment |
| **Tool count** | ~10 shell commands | 44 typed MCP tools |
| **Error handling** | Shell exit codes, often swallowed | Go errors with typed responses (`IsError=true`) |

### What this means in practice

**Fewer pipeline failures.** v1's most common failure mode was the LLM misinterpreting a phase transition — skipping a checkpoint, forgetting to call `phase-complete`, or miscounting retry attempts. v2 eliminates this entire class of errors because the Engine makes all transition decisions.

**Cheaper retries.** When a v1 pipeline stalled, resuming required the LLM to re-read state.json and reconstruct its understanding of where it was. v2's `pipeline_next_action` returns the exact next step — no interpretation needed.

**Richer context for agents.** v2 injects historical data (past review patterns, similar implementations, repo profile) into agent prompts via MCP tools. v1 agents worked in isolation with no cross-pipeline knowledge.

**Auditable decisions.** Every Engine decision is a deterministic function of `state.json`. You can reproduce any pipeline's control flow by replaying `NextAction()` calls against the saved state — something impossible with v1's LLM-driven decisions.

---

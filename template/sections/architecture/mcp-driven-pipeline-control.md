## Architecture: MCP-driven pipeline engine

claude-forge's defining design principle: the LLM is the **executor**, not the decision-maker.

A Go MCP server (`forge-state-mcp`) owns all pipeline logic — which phase runs next, whether to retry, when to skip, and what to validate. The LLM orchestrator follows a strict **ask → execute → report** loop:

```
User → SKILL.md (LLM executes) → Go Engine (decides next phase) → MCP tools (state + guards)
```

1. Call `pipeline_next_action` — receive a typed action: `spawn_agent`, `checkpoint`, `human_gate`, `exec`, `write_file`, or `done`
2. Execute the action
3. Call `pipeline_report_result` — Engine advances state

The Engine returns typed actions. The LLM cannot invent steps or skip them. If a phase transition condition isn't met — artifact missing, review verdict REVISE, retry limit reached — the Engine enforces it, not a prompt instruction.

### What this means in practice

**Deterministic phase transitions.** Every pipeline decision is a deterministic function of `state.json`. The Engine enforces canonical phase order, tracks revision counts with hard limits, and validates artifacts before advancing. Any pipeline's control flow is reproducible by replaying `NextAction()` calls against saved state.

**Reliable resume.** `pipeline_next_action` returns the exact next step after any interruption — context compaction, session restart, or manual pause. No re-interpretation needed.

**Cross-pipeline knowledge.** The MCP server injects historical data into agent prompts — past review patterns, similar implementations, repo profile. Agents are informed by every prior run, not just the current session.

**Auditable decisions.** Every control-flow decision is logged in `state.json` — what ran, what was skipped, retry counts, timestamps. Fully traceable without digging into conversation history.

### MCP tool surface

The `forge-state` server exposes **46 typed MCP tools** across six categories:

| Category | Examples |
| --- | --- |
| Lifecycle | `pipeline_init`, `pipeline_next_action`, `pipeline_report_result` |
| Phase | `phase_start`, `phase_complete`, `phase_fail`, `skip_phase` |
| Validation | `validate_input`, `validate_artifact` |
| History | `history_search`, `history_get_patterns`, `history_get_friction_map` |
| Analytics | `analytics_pipeline_summary`, `analytics_repo_dashboard`, `analytics_estimate` |
| Code analysis | `ast_summary`, `ast_find_definition`, `dependency_graph`, `impact_scope` |

---

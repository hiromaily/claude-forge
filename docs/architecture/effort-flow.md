# Effort-driven Flow

The pipeline adapts its execution based on the effort level. The orchestrator skips non-applicable phases upfront during Workspace Setup using the `skip-phase` command, so `currentPhase` already points past all skipped phases before the first real phase begins.

## Effort Levels and Phase Skip Tables

Three effort levels are supported. `L` runs the full pipeline. Lower levels skip phases:

| Effort | Template | Phases to skip |
|--------|----------|----------------|
| `S` | `light` | `phase-4b`, `checkpoint-b`, `phase-7` |
| `M` | `standard` | `phase-4b`, `checkpoint-b` |
| `L` | `full` | (none) |

**Rationale by effort level:**

- **`S` (light)**: Skips the task-review quality gate (`phase-4b`, `checkpoint-b`) and Comprehensive Review (`phase-7`). Suitable for small, focused tasks where task decomposition is straightforward and comprehensive post-implementation review is not warranted.
- **`M` (standard)**: Skips the task-review quality gate only. Phase 7 (Comprehensive Review) runs. Suitable for medium-sized features where implementation review is valuable but the task breakdown is simple enough not to require a separate quality gate.
- **`L` (full)**: All phases run including both checkpoints and Comprehensive Review. Suitable for large, complex tasks where every quality gate adds value.

## state.json Schema Additions

Several top-level fields have been added to `state.json` beyond the initial v1 schema:

```json
{
  "version": 1,
  "effort": "S | M | L | null",
  "flowTemplate": "light | standard | full | null",
  "skippedPhases": ["phase-4b", "checkpoint-b", "phase-7"],
  "autoApprove": false,
  "phaseLog": [
    {"phase": "phase-1", "tokens": 5000, "duration_ms": 30000, "model": "sonnet", "timestamp": "..."}
  ],
  ...
}
```

- `effort` is `null` until set during Workspace Setup. Set via `mcp__forge-state__set_effort`. Valid values: `S`, `M`, `L` (XS is not supported).
- `flowTemplate` is `null` until set during Workspace Setup. Set via `mcp__forge-state__set_flow_template`. Valid values: `light`, `standard`, `full`. Stored in state (not re-derived) to guarantee resume consistency.
- `skippedPhases` is `[]` until populated. Each call to `skip-phase` appends one phase ID to this array.
- `autoApprove` defaults to `false`. Set via `set-auto-approve` when `--auto` flag is present.
- `phaseLog` records per-phase metrics (tokens, duration, model) via `phase-log`. Used by `phase-stats` and the Final Summary Execution Stats table.
- `version` remains `1` — old state files simply lack these fields and the orchestrator treats absence as `null`/`[]`/`false` via the `resume-info` defaults.

**Invariant:** `completedPhases` and `skippedPhases` are mutually exclusive. A phase ID appears in at most one of these arrays. `phase-complete` adds to `completedPhases`; `skip-phase` adds to `skippedPhases`. Neither command modifies the other array.

## The `skip-phase` Command vs `phase-complete`

`phase-complete` and `skip-phase` are both mechanisms for advancing `currentPhase` to the next entry in the canonical PHASES array. They differ in their semantic meaning and side effects:

| Aspect | `phase-complete` | `skip-phase` |
|--------|-----------------|--------------|
| Meaning | Phase ran successfully | Phase was intentionally bypassed |
| Records in | `completedPhases` | `skippedPhases` |
| Advances `currentPhase` | Yes, via `next_phase()` | Yes, via the same `next_phase()` logic |
| Sets `currentPhaseStatus` | `"pending"` for next phase | `"pending"` for next phase |
| When called | After the phase agent completes | During Workspace Setup, before the phase runs |

Because `skip-phase` uses the same `next_phase()` ordering logic as `phase-complete`, the same ordering invariant applies: phases must be processed in canonical PHASES-array order, one call at a time, without gaps.

## Upfront-Skip Pattern

All `skip-phase` calls happen **upfront during Workspace Setup**, in canonical PHASES-array order, before the first real phase begins. This means:

1. The orchestrator determines `{effort}` during Workspace Setup.
2. It calls `mcp__forge-state__set_effort` with `{workspace}` and `{effort}`.
3. For each phase in the skip table (in canonical order), it calls `mcp__forge-state__skip_phase` with `{workspace}` and `<phase>`.
4. By the time the orchestrator reaches the first phase block, `currentPhase` already points past all skipped phases.

The orchestrator still checks a skip gate at each phase block — if the effort level maps to skipping that phase, it proceeds directly to the next block without calling `phase-start` or spawning an agent.

## Effort Detection Priority

The orchestrator detects `{effort}` using this priority order during Workspace Setup:

1. **Explicit flag**: `--effort=<value>` in `$ARGUMENTS` (strip from args before writing `request.md`; valid values: `S`, `M`, `L`; `XS` is rejected at input validation time)
2. **Jira story points**: read `customfield_10016` from the fetched Jira issue. If absent, None, non-numeric, or zero, fall through. Mapping: SP ≤ 4 → S, SP ≤ 12 → M, SP > 12 → L.
3. **Heuristic**: infer from task description complexity.
4. **Default**: `M` (safe fallback — matches current behavior for pipelines started before this feature was deployed)

After detection, call: `$SM set-effort {workspace} {effort}`

## Flow Template Selection

The effort level alone determines the `flowTemplate` string stored in state. XS effort is not supported; the minimum supported effort is S. After lookup, call: `$SM set-flow-template {workspace} {flow_template}`

| Effort | Template | Skipped phases |
|--------|----------|----------------|
| S | `light` | `phase-4b`, `checkpoint-b`, `phase-7` |
| M | `standard` | `phase-4b`, `checkpoint-b` |
| L | `full` | _(none)_ |

New Go helper functions:
- `EffortToTemplate(effort string) string` — maps effort to template name
- `SkipsForEffort(effort string) []string` — returns the canonical skip list for the given effort level

### Template definitions

| Template | Phases run | Agent count |
|----------|-----------|-------------|
| `light` | Phase 1 → Phase 2 → Phase 3 → Phase 3b → Checkpoint A → Phase 4 → Phase 5 → Phase 6 → Verification → PR | 5+ |
| `standard` | Full pipeline (all phases, both checkpoints except 4b/checkpoint-b) | 10+ |
| `full` | Standard + all checkpoints mandatory (auto-approve disabled even with `--auto`) | 10+ |

### Skip-set computation

The skip set for any pipeline run is determined entirely by the effort level. Skip sets are emitted as `skip-phase` calls in canonical PHASES-array order during Workspace Setup. The orchestrator computes the list upfront — no runtime re-computation is needed.

## Consolidated Artifact Availability

Single reference for which workspace artifact files are present after a completed pipeline. Derived from the effort-to-template table and the skip sets above.

**Legend:** `✓` agent-produced · `S` orchestrator stub · `—` not produced

`summary.md` is always produced and is omitted from the table.

| effort | template | `analysis.md` | `investigation.md` | `design.md` | `review-design.md` | `tasks.md` | `review-tasks.md` | `impl-{N}.md` | `review-{N}.md` | `comprehensive-review.md` |
|--------|----------|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| S | `light` | ✓ | ✓ | ✓ | ✓ | ✓ | — | ✓ | ✓ | — |
| M | `standard` | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| L | `full` | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |

## Resume Behaviour

On resume, the orchestrator restores `{effort}` from `resume_info.effort`, `{flow_template}` from `resume_info.flowTemplate`, and `{skipped_phases}` from `resume_info.skippedPhases`. Fallback rules:

- If `effort` is null (pipeline started before effort-only flow was deployed): default to `M` **in-context only** and log a note. Do NOT call `set-effort` — the `skippedPhases` already recorded in state remain authoritative.
- If `flowTemplate` is null: re-derive from effort using `EffortToTemplate` and store **in-context only**. Do NOT call `set-flow-template` — the original `skippedPhases` remain authoritative.
- Retain `{effort}` and `{flow_template}` as in-context variables for the duration of the resumed pipeline.

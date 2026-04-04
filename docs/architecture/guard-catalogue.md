# Guard Catalogue — Enforcement Reference

This is the authoritative reference for every enforcement mechanism in claude-forge. Each entry documents **what** is enforced, **which layer** enforces it, and **whether it is deterministic**.

## Enforcement Layers

claude-forge uses four enforcement layers, listed from most to least reliable:

| Layer | Mechanism | Determinism | Failure mode |
|---|---|---|---|
| **Go MCP handler** | Guard functions in `tools/guards.go` called by handlers in `tools/handlers.go`. Return `error` (blocking) or `string` (warning). | Deterministic | `IsError=true` MCP response; state not mutated |
| **Go engine** | Decision logic in `orchestrator/engine.go`. Controls phase transitions, auto-approve, retry limits, skip gates. | Deterministic | Returns specific action types; orchestrator must follow |
| **Shell hook** | Bash scripts in `scripts/`. Fire on tool calls via Claude Code hook system. Exit 2 = block. | Deterministic (fail-open if `jq` missing) | Exit 2 blocks the tool call; exit 0 allows |
| **Prompt instruction** | Text in `SKILL.md` or agent `.md` files. Followed by the LLM non-deterministically. | **Non-deterministic** | LLM may skip or misinterpret |

**Design principle:** All critical invariants (data integrity, human approval gates, safety constraints) are enforced by code (layers 1–3). Prompt instructions (layer 4) are used only for orchestration protocol compliance where code enforcement is impractical.

## Blocking Guards (prevent state mutation)

These guards return errors and halt progression. The pipeline cannot advance until the condition is satisfied.

| ID | Invariant | Layer | Code location | Trigger |
|---|---|---|---|---|
| 3a | Artifact file must exist before phase-complete | MCP handler | `guards.go:Guard3aArtifactExists` | `phase_complete` for phases with required artifacts |
| 3b | Review file must exist before marking task review as passed | MCP handler | `guards.go:Guard3bReviewFileExists` | `task_update` with `reviewStatus=completed_pass` |
| 3c | Tasks must be initialized before phase-5 starts | MCP handler | `guards.go:Guard3cTasksNonEmpty` | `phase_start` for `phase-5` |
| 3e | Checkpoint requires `awaiting_human` status before completion | MCP handler | `guards.go:Guard3eCheckpointAwaitingHuman` | `phase_complete` for `checkpoint-a`, `checkpoint-b` |
| 3g | Checkpoint B must be done/skipped before task initialization | MCP handler | `guards.go:Guard3gCheckpointBDoneOrSkipped` | `task_init` |
| 3j | Pending revision must be cleared before checkpoint completion | MCP handler | `guards.go:Guard3jCheckpointRevisionPending` | `phase_complete` for `checkpoint-a`, `checkpoint-b` |
| — | Init requires prior input validation | MCP handler | `guards.go:GuardInitValidated` | `init` with `validated=false` |
| — | Artifact content must pass validation | MCP handler | `pipeline_report_result.go` | `pipeline_report_result` for review phases |
| R1 | Source edits blocked during phase-1/2 (read-only) | Shell hook | `pre-tool-hook.sh` Rule 1 | `Edit`/`Write` tool targeting files outside workspace |
| R2 | Git commits blocked during parallel phase-5 execution | Shell hook | `pre-tool-hook.sh` Rule 2 | `Bash` tool with `git commit` while parallel tasks active |
| R5 | Git checkout/switch to main/master blocked during active pipeline | Shell hook | `pre-tool-hook.sh` Rule 5 | `Bash` tool with `git checkout main` or `git switch master` |
| — | Stop signal blocked during active pipeline | Shell hook | `stop-hook.sh` | Claude Code stop when status not in `completed`, `abandoned`, `awaiting_human` |

## Non-blocking Warnings (alert but allow)

These checks inject warnings into the conversation but do not prevent the action.

| ID | Check | Layer | Code location | Trigger |
|---|---|---|---|---|
| 3d | Duplicate phase-log entry | MCP handler | `guards.go:Warn3dPhaseLogDuplicate` | `phase_log` |
| 3f | Missing phase-log entry at phase completion | MCP handler | `guards.go:Warn3fPhaseLogMissing` | `phase_complete` for phases requiring logs |
| 3h | Task number not found in state | MCP handler | `guards.go:Warn3hTaskNotFound` | `task_update` |
| 3i | Phase status not `in_progress` at completion | MCP handler | `guards.go:Warn3iPhaseNotInProgress` | `phase_complete` |
| — | Agent output too short (< 50 chars) | Shell hook | `post-agent-hook.sh` | `Agent` tool return during active phase |
| — | Review agent output missing verdict keyword | Shell hook | `post-agent-hook.sh` | `Agent` tool return during `phase-3b`, `phase-4b` |
| — | Impl review output missing PASS/FAIL keyword | Shell hook | `post-agent-hook.sh` | `Agent` tool return during `phase-6` |

## Engine Decisions (deterministic branching)

The orchestrator engine (`orchestrator/engine.go`) makes all phase-transition decisions deterministically based on state. The LLM orchestrator receives an action to execute; it does not choose what to do next.

| ID | Decision | Condition | Behavior | Code location |
|---|---|---|---|---|
| D14 | Phase skip | Phase in `skippedPhases` | Returns `done` action with `skip:` prefix; orchestrator calls `phase_complete` | `engine.go` top of `NextAction` |
| D20 | Auto-approve (checkpoint bypass) | `autoApprove == true` AND verdict is `APPROVE` or `APPROVE_WITH_NOTES` | Spawns next phase agent, bypassing checkpoint | `engine.go` phase-3b/4b handlers |
| D21 | Retry limit (2×) | `designRevisions >= 2` or `taskRevisions >= 2` or `implRetries >= 2` | Forces human checkpoint (approve or abandon) | `engine.go` phase-3b/4b/6 handlers |
| D22 | Parallel task dispatch | First pending task has `executionMode == "parallel"` | Spawns all consecutive parallel tasks simultaneously | `engine.go` phase-5 handler |
| D23 | Impl review verdict routing | Parsed verdict from `review-N.md` | FAIL → re-spawn implementer; PASS → next task | `engine.go` phase-6 handler |
| D24 | PR skip | `skipPr == true` | Returns `done` action, bypasses PR creation | `engine.go` pr-creation handler |
| D26 | Post-to-source dispatch | `source_type` from `request.md` frontmatter | GitHub → `gh` command; Jira → checkpoint; text → done | `engine.go` post-to-source handler |

## Artifact Validation (deterministic content checks)

The `validation/artifact.go` package validates artifact content when `pipeline_report_result` is called. Validation failures block phase advancement.

| Phase | Required artifact | Content rule |
|---|---|---|
| phase-1 | `analysis.md` | Must contain `## ` heading |
| phase-2 | `investigation.md` | Must contain `## ` heading |
| phase-3 | `design.md` | Must contain `## ` heading |
| phase-3b | `review-design.md` | Must contain `APPROVE`, `APPROVE_WITH_NOTES`, or `REVISE` |
| phase-4 | `tasks.md` | Must contain `## Task` heading |
| phase-4b | `review-tasks.md` | Must contain `APPROVE`, `APPROVE_WITH_NOTES`, or `REVISE` |
| phase-6 | `review-N.md` | Must contain `PASS`, `PASS_WITH_NOTES`, or `FAIL` |
| phase-7 | `comprehensive-review.md` | Must be non-empty |
| final-summary | `summary.md` | Must exist |

Findings markers (`[CRITICAL]`, `[MINOR]`) are counted and accumulated into the pattern knowledge base for historical analysis. This is non-blocking.

## Automated Side Effects (deterministic actions)

| Action | Layer | Code location | Trigger |
|---|---|---|---|
| Final commit: amend `summary.md` + `state.json` into last commit, then force-push | Shell hook (v1) / Engine exec action (v2) | `post-bash-hook.sh` (v1 legacy) / `engine.go` final-commit action (v2) | After `post-to-source` phase completes; `pipeline_report_result` called first so state.json is in "completed" state when committed |
| Revision counter increment | MCP handler | `pipeline_report_result.go` | `REVISE` verdict in review phases |
| Pattern knowledge accumulation | MCP handler | `pipeline_report_result.go` | Any review phase completion with findings |

## Prompt-only Instructions (non-deterministic)

These behaviors are enforced **only** by LLM instructions. They cannot be guaranteed by code because they involve orchestration-level decisions that are impractical to express as state guards.

| Instruction | Location | Why not code-enforced |
|---|---|---|
| Never pass `isolation: "worktree"` to Agent calls | SKILL.md | Claude Code Agent tool parameter; no hook intercept point for Agent tool arguments |
| Always call `pipeline_report_result` after `spawn_agent`, `exec`, `write_file` | SKILL.md | Omission is a no-op (missing metric), not a state corruption; adding a timeout guard would add complexity disproportionate to risk |
| Wait for human response before calling `phase_complete` on checkpoints | SKILL.md | The **wait** itself is prompt-only, but the **gate** is deterministic: Guard 3e blocks `phase_complete` unless `checkpoint()` was called first, so the LLM cannot skip the checkpoint even if it doesn't wait |
| Parse `skip:` prefix from `done` action and call `phase_complete` | SKILL.md | Engine returns the skip signal; if the orchestrator fails to parse it, `pipeline_next_action` returns the same skip signal again (self-correcting loop) |

## Dual-layer Enforcement Map

Some invariants are enforced at both the shell hook layer and the Go MCP handler layer. This provides defense-in-depth: the MCP handler fires first on MCP tool calls; the shell hook fires independently on `Bash`/`Edit`/`Write` tool calls.

| Invariant | Shell hook | MCP handler |
|---|---|---|
| Artifact must exist before phase advancement | — | Guard 3a (`phase_complete`) + `pipeline_report_result` validation |
| Checkpoint requires human approval | — | Guard 3e (`phase_complete` requires `awaiting_human`) |
| Phase 1-2 read-only | Rule 1 (`pre-tool-hook.sh`) | — (agents don't call MCP tools to edit files) |
| No parallel git commits | Rule 2 (`pre-tool-hook.sh`) | — (git commits go through `Bash` tool, not MCP) |
| No checkout to main/master | Rule 5 (`pre-tool-hook.sh`) | — (branch operations go through `Bash` tool) |
| Review verdict extraction | Shell warning (`post-agent-hook.sh`) | Artifact content validation (`validation/artifact.go`) |

## Fail-open Guarantees

All shell hooks are fail-open: if `jq` is not installed or `state.json` cannot be read, the hook exits 0 (allow). This ensures the plugin never blocks legitimate non-pipeline work.

```bash
# Every hook starts with this pattern:
command -v jq >/dev/null 2>&1 || exit 0
```


# Claude-Forge Plugin — Backlog

Known issues, improvement candidates, and future direction.

---

## Priority Queue

Ordered by priority. Higher rows should be tackled first.

| # | ID | Issue | Title | Type | Effort | Why now |
|---|-----|-------|-------|------|--------|---------|
| 4 | **F10** | [#12](https://github.com/hiromaily/claude-forge/issues/12) | Partial execution (`--until`/`--from`) | Feature | M | `--until=design` for scoping only, `--from=phase-5` for re-implementation. Combines with `--auto` for autonomous scoping reports. |
| 5 | **F9** | [#13](https://github.com/hiromaily/claude-forge/issues/13) | Structured acceptance criteria | Feature | M | Improves PASS/FAIL consistency. Currently depends on impl-reviewer's subjective interpretation. |
| 6 | **F12** | [#14](https://github.com/hiromaily/claude-forge/issues/14) | Checkpoint diff preview | Feature | S | Nice-to-have. `--auto` reduces checkpoint frequency, lowering the priority. |
| 7 | **F8** | [#15](https://github.com/hiromaily/claude-forge/issues/15) | Past pipeline reference (data flywheel) | Feature | L | Uses `.specs/` history to improve future pipelines. The accumulated data is a moat — competitors can copy code but not execution history. |
| 8 | **F17** | [#16](https://github.com/hiromaily/claude-forge/issues/16) | Repository profiling | Feature | M | First-run analysis of repo structure, test strategy, CI config → persisted profile that tunes agent prompts. Hard to replicate without per-repo data. |
| 9 | **F18** | [#17](https://github.com/hiromaily/claude-forge/issues/17) | Improvement report → test case generation | Feature | S | Auto-generate hook guard test cases from friction points found in improvement reports. Accelerates deterministic guard accumulation. |
| 10 | **F19** | [#18](https://github.com/hiromaily/claude-forge/issues/18) | CI feedback loop (post-PR auto-fix) | Feature | L | After PR creation, monitor CI results and auto-trigger fix flow on failure. Closes the quality loop beyond the pipeline boundary. |
| 11 | **F6** | [#19](https://github.com/hiromaily/claude-forge/issues/19) | Adaptive model routing | Feature | L | Needs phase-stats data before deciding. F13 (effort axis) provides the foundation for model selection. |
| 12 | **F2** | [#20](https://github.com/hiromaily/claude-forge/issues/20) | Execution log (JSONL) | Feature | M | Basic coverage via phase-log. Full JSONL log deferred until the need is confirmed. |

**Effort key:** XS = < 30min, S = 1-2h, M = half day, L = 1+ day

**Prioritization criteria:**

1. **Blocking bug** — fix first
2. **Determinism** — hook guards to cover AI non-determinism
3. **Dev loop acceleration** — high ROI (F10)
4. **Competitive moat** — data flywheel and per-repo learning (F8, F17, F18)
5. **Cost reduction** — validate with phase-stats data (F5, F6)
6. **Future features** — after data accumulation (F12, F19)

---

## Deterministic Orchestration — Move SKILL.md Logic into MCP Server

**Problem:** SKILL.md delegates many mechanical, state-machine operations to the LLM orchestrator via prose instructions. Since the orchestrator is an LLM, it non-deterministically skips, misorders, or misparametrizes these operations. Observed failures from a production run (SOA-2984):

| Failure | Root cause | Impact |
|---------|-----------|--------|
| Parallel tasks not committed | Orchestrator didn't execute `batch_commit` action | Review FAIL on all parallel tasks |
| Infinite impl retry loop | Stale review file re-read; `ImplRetries` never incremented | Pipeline stuck until manual state intervention |
| Checkpoint phase name mismatch | Engine returned `post-to-jira` but `Checkpoint()` expected `post-to-source` | MCP error, manual workaround needed |
| `final_commit` git add blocked | `.specs/` in `.gitignore`; SKILL.md didn't use `git add -f` | Final commit silently failed |
| Review status not persisted | Engine checked `ReviewStatus==""` but never wrote passing status | Re-dispatched reviewers for already-reviewed tasks |

**Root insight:** Most of what SKILL.md instructs is purely mechanical and requires no LLM judgment. The LLM orchestrator should only be responsible for three things:

1. **Spawning agents** (Claude Code Agent tool constraint)
2. **User interaction** (checkpoints, AskUserQuestion)
3. **External API calls** (GitHub CLI, Jira MCP — requires judgment)

Everything else can be absorbed into `pipeline_next_action` / `pipeline_report_result`:

### Proposed changes

#### P1: Absorb `skip:` phase completion into `pipeline_next_action`

**Current:** Engine returns `done` with `skip:phase-id` → SKILL.md tells orchestrator to call `phase_complete` → orchestrator calls `pipeline_next_action` again.

**Proposed:** `pipeline_next_action` internally calls `PhaseComplete` for skipped phases and re-enters the dispatch loop, returning only actionable items (spawn_agent, checkpoint, exec) to the orchestrator. The orchestrator never sees `skip:` actions.

**Benefit:** Eliminates a class of "forgot to call phase_complete" bugs. Reduces SKILL.md complexity.

#### P2: Absorb `task_init` parsing into MCP server

**Current:** SKILL.md tells orchestrator to parse `tasks.md` markdown, extract task metadata, and pass it as JSON to `task_init`.

**Proposed:** `task_init` MCP tool reads and parses `tasks.md` directly from the workspace. No orchestrator involvement needed.

**Benefit:** Eliminates markdown parsing errors from LLM. The MCP server has deterministic access to the file.

#### P3: Absorb `batch_commit` into MCP server

**Current:** SKILL.md tells orchestrator to run `git status`, `git add` specific files, `git commit`.

**Proposed:** `batch_commit` becomes a dedicated MCP tool (or an internal step in `pipeline_report_result`) that reads task file lists from state.json, runs git commands, and returns the commit hash.

**Benefit:** Eliminates the most impactful failure observed — parallel tasks going uncommitted.

#### P4: Absorb `final_commit` into MCP server

**Current:** SKILL.md tells orchestrator to call `pipeline_report_result`, then `git add -f`, `git commit --amend`, `git push`.

**Proposed:** Single MCP tool `final_commit` that executes the entire sequence atomically, handling `.gitignore` and error recovery internally.

**Benefit:** Eliminates git-related edge cases (gitignore, amend ordering).

#### P5: Merge `pipeline_report_result` into `pipeline_next_action`

**Current:** After every action, orchestrator must call `pipeline_report_result` with correct parameters, then call `pipeline_next_action`. Two separate round-trips.

**Proposed:** `pipeline_next_action` accepts optional `previous_result` parameters (tokens, duration, model). Internally calls `report_result` logic before computing the next action. Single round-trip per cycle.

**Benefit:** Eliminates "forgot to call report_result" and "wrong phase parameter" bugs. Halves MCP round-trips.

#### Resulting minimal SKILL.md

After P1–P5, the orchestrator loop becomes:

```
1. Call pipeline_next_action(workspace, previous_tokens, previous_duration, previous_model)
2. Based on action.type:
   - spawn_agent → call Agent tool → go to 1
   - checkpoint → present to user → go to 1
   - post_to_source → determine source type from URL, call API → go to 1
   - pr_creation → run gh pr create → go to 1
   - done → stop
```

No skip handling, no task_init parsing, no batch_commit, no final_commit, no report_result calls. The LLM orchestrator becomes a thin dispatcher for agent spawning and user interaction.

**Effort:** L (multiple MCP tool changes, SKILL.md rewrite, test updates)

**Priority:** High — addresses the most fundamental source of non-determinism in the pipeline.

---

## Phase Registry: Deferred Scatter Points

The **declarative phase registry** refactor (`feature/declarative-phase-registry`) consolidated the six per-phase edit sites in `orchestrator/` into two (`state/state.go` + `orchestrator/registry.go`). Two additional scatter points were intentionally left out of scope to avoid cross-package coupling:

| Location | Symbol(s) | Notes |
|---|---|---|
| `mcp-server/internal/validation/artifact.go` | `artifactRules` | Per-phase lookup table of expected artifact filenames and required headings. Moving into `PhaseDescriptor` would force `orchestrator` to import `validation` (or vice versa), inverting the current clean dependency direction. |
| `mcp-server/internal/tools/guards.go` | `phaseArtifacts`, `phaseLogRequired` | Per-phase guard maps consulted by MCP tool handlers. Encoding these in the descriptor would require `orchestrator` to depend on `tools`, which itself imports `orchestrator` — creating a cycle. |

**Future direction:** If a registry package (`orchestrator/registry`) is ever extracted as a leaf package (no imports of `validation` or `tools`), both tables could be merged into extended `PhaseDescriptor` fields. Until then, keep the tables in their respective packages and rely on `TestPhaseRegistryConsistency` + the `initRegistry()` panic to detect ID-set drift.

---

## Improvement Candidates

| Issue | Title | Notes |
|-------|-------|-------|
| [#21](https://github.com/hiromaily/claude-forge/issues/21) | Model selection per agent | Use opus for architect, design-reviewer, implementer. ~2× cost increase. |
| [#22](https://github.com/hiromaily/claude-forge/issues/22) | Agent-level retry with context carry-forward | Use `resume` parameter to preserve agent reasoning across retries. Needs feasibility testing. |
| [#23](https://github.com/hiromaily/claude-forge/issues/23) | Parallel Phase 5-6 interleaving | Spawn Phase 6 review immediately after each Phase 5 task. Complex state tracking. |
| [#24](https://github.com/hiromaily/claude-forge/issues/24) | Workspace directory naming | Rename `.specs/` → `.claude-forge/` to avoid collision with kiro specs. Breaking change — migration needed. |
| [#25](https://github.com/hiromaily/claude-forge/issues/25) | Hook-based progress notifications | Log phase transitions to `progress.log`; optional Slack webhook. |
| [#26](https://github.com/hiromaily/claude-forge/issues/26) | State schema versioning | Bump version + add migration functions when schema changes in breaking ways. |
| [#27](https://github.com/hiromaily/claude-forge/issues/27) | Per-project setup wizard | Interactive first-run wizard persisting project conventions to a profile file. Complements F17 (automated profiling). Source: aaddrick/claude-pipeline. |
| [#28](https://github.com/hiromaily/claude-forge/issues/28) | JSON-driven agent configuration | Declarative `agents.json` schema for agent metadata — eliminates drift across roster tables. Source: catlog22/Claude-Code-Workflow. |
| [#29](https://github.com/hiromaily/claude-forge/issues/29) | Cold start optimization | Reduce XS/S pipeline startup overhead via lazy workspace setup and merged validation passes. Source: barkain. |
| [#30](https://github.com/hiromaily/claude-forge/issues/30) | Agent Teams mode (Phase 5 inter-agent comms) | Collaborative mode with `comms.json` for real-time coordination. High complexity — defer until pain confirmed by phase-stats data. Source: barkain. |
| [#31](https://github.com/hiromaily/claude-forge/issues/31) | Linear integration | Add `linear_issue` source type alongside GitHub Issues and Jira. Source: levnikolaevich. |
| [#32](https://github.com/hiromaily/claude-forge/issues/32) | Native plan mode integration at checkpoints | Use EnterPlanMode at Checkpoint A/B for structured task/PR review. Source: barkain. |

---

> **Testing Checklist** has been moved to `.claude/rules/testing.md` for automatic reference during changes.

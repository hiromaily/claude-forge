
# Claude-Forge Plugin — Backlog

Known issues, improvement candidates, and future direction.

---

## Priority Queue

Ordered by priority. Higher rows should be tackled first.

| # | ID | Issue | Title | Type | Effort | Why now |
|---|-----|-------|-------|------|--------|---------|
| 1 | **B1** | — | Dynamic checkpoint UX: user visibility + resume | Bug/Feature | M | **Blocking bug.** Dynamic checkpoints (`design-approved`, `design-retry-limit`, `tasks-approved`, `task-retry-limit`, `design-review-unknown`, `task-review-unknown`, `impl-retry-limit-*`) are not properly handled when AutoApprove=false. Users see sudden abandons with no context about what phase/event caused it. Resume cannot recover from dynamic checkpoint states. |
| 2 | **F10** | [#12](https://github.com/hiromaily/claude-forge/issues/12) | Partial execution (`--until`/`--from`) | Feature | M | `--until=design` for scoping only, `--from=phase-5` for re-implementation. Combines with `--auto` for autonomous scoping reports. |
| 3 | **F9** | [#13](https://github.com/hiromaily/claude-forge/issues/13) | Structured acceptance criteria | Feature | M | Improves PASS/FAIL consistency. Currently depends on impl-reviewer's subjective interpretation. |
| 4 | **F12** | [#14](https://github.com/hiromaily/claude-forge/issues/14) | Checkpoint diff preview | Feature | S | Nice-to-have. `--auto` reduces checkpoint frequency, lowering the priority. |
| 5 | **F18** | [#17](https://github.com/hiromaily/claude-forge/issues/17) | Improvement report → test case generation | Feature | S | Auto-generate hook guard test cases from friction points found in improvement reports. Builds on the existing `history_get_friction_map` data. |
| 6 | **F19** | [#18](https://github.com/hiromaily/claude-forge/issues/18) | CI feedback loop (post-PR auto-fix) | Feature | L | After PR creation, monitor CI results and auto-trigger fix flow on failure. Closes the quality loop beyond the pipeline boundary. |
| 7 | **F6** | [#19](https://github.com/hiromaily/claude-forge/issues/19) | Adaptive model routing | Feature | L | Needs phase-stats data before deciding. Could now be informed by the accumulated `analytics_*` metrics. |
| 8 | **F2** | [#20](https://github.com/hiromaily/claude-forge/issues/20) | Execution log (JSONL) | Feature | M | Basic coverage via phase-log. Full JSONL log deferred until the need is confirmed. |
| 9 | **F20** | — | Shared events log rotation / pruning | Maintenance | S | `~/.claude/forge-events.jsonl` is append-only and grows indefinitely. `SetEventLog` loads the entire file into memory on startup. After weeks of use across many projects the file and in-memory `history` slice will become large. Add max-age trimming (e.g. keep last 30 days) or a file-size cap with rollover to `forge-events.jsonl.old`. |
| 10 | **P1** | — | ~~Pipeline execution speed: REVISE loop cap + parallel dispatch fix + branch name mismatch~~ | Performance/Bug | M | ✅ **All 6 actions completed.** REVISE cap (MaxDesignReviseRounds=2), dependency-aware parallel dispatch, branch auto-checkout validation. |

**Effort key:** XS = < 30min, S = 1-2h, M = half day, L = 1+ day

**Prioritization criteria:**

1. **Blocking bug** — fix first
2. **Determinism** — hook guards to cover AI non-determinism
3. **Dev loop acceleration** — high ROI (F10)
4. **Data flywheel extensions** — leverage the accumulated `history_*` and `profile_*` data (F18)
5. **Cost reduction** — validate with phase-stats data (F6)
6. **Future features** — after data accumulation (F12, F19)

---

## B1: Dynamic Checkpoint UX — User Visibility + Resume

### Problem

When `AutoApprove=false`, dynamic checkpoints returned by the engine (`design-approved`, `design-retry-limit`, etc.) are not properly handled. The pipeline silently fails or abandons, leaving users with no context about what happened or which phase caused the issue. Resume cannot recover from these states.

**Three concrete failure modes:**
1. **No user visibility** — When a dynamic checkpoint is returned, the orchestrator (LLM) has no clear instructions for how to handle it. The result is a sudden abandon with no explanation of what phase or event triggered it.
2. **Resume broken** — Dynamic checkpoint names (`design-approved`, etc.) are not formal phase IDs (`checkpoint-a`, etc.), so `resume_info` cannot correctly restore pipeline state.
3. **P8 scope gap** — The 2026-04-17 fix added P8 to handle checkpoint responses deterministically, but `isCheckpointPhase()` only recognizes `checkpoint-a` and `checkpoint-b`. All other checkpoint types fall through unhandled.

**Affected dynamic checkpoints:**

| Checkpoint name | Source | Options | Trigger condition |
|---|---|---|---|
| `design-approved` | `handlePhaseThreeB` | proceed, revise | AutoApprove=false, verdict=APPROVE |
| `design-retry-limit` | `handlePhaseThreeB` | approve, abandon | DesignRevisions >= MaxRetries |
| `design-review-unknown` | `handlePhaseThreeB` | approve, revise, abandon | Unrecognized verdict |
| `tasks-approved` | `handlePhaseFourB` | proceed, revise | AutoApprove=false, verdict=APPROVE |
| `task-retry-limit` | `handlePhaseFourB` | approve, abandon | TaskRevisions >= MaxRetries |
| `task-workflow-rules-retry-limit` | `handlePhaseFour` | approve, abandon | TaskRevisions >= MaxRetries (workflow rules) |
| `task-review-unknown` | `handlePhaseFourB` | approve, revise, abandon | Unrecognized verdict |
| `impl-retry-limit-{N}` | `handlePhaseFive` | approve, abandon | ImplRetries >= MaxRetries |
| `post-to-source` | `handlePostToSource` | post, skip | Source URL present |

### Design direction

**Option A: Absorb dynamic checkpoints into P8**
- Extend `isCheckpointPhase` to recognize all dynamic checkpoint names.
- Handle each `user_response` (proceed/approve/revise/abandon/post/skip) inside the engine.
- Pro: SKILL.md stays simple (always pass `user_response`).
- Con: P8 logic grows complex; each dynamic checkpoint has different rewind targets and semantics.

**Option B: Eliminate dynamic checkpoints; use formal phase transitions**
- Remove the `design-approved` checkpoint from `handlePhaseThreeB`; let `reportResultCore` call `PhaseComplete` on verdict=APPROVE, reaching `checkpoint-a` directly.
- Retry-limit and unknown-verdict cases become state flags with dedicated phase handling.
- Pro: Clean architecture; no P8 changes needed.
- Con: Retry-limit cases ("human judgment required") need a new mechanism since they cannot map to existing phases.

**Option C: Promote dynamic checkpoints to formal phase IDs**
- Add `design-approved`, `task-retry-limit`, etc. to `ValidPhases`.
- Resume recognizes them as proper phases.
- Pro: Resume works naturally.
- Con: Phase count explodes (18 → 25+); test and documentation impact is large.

### Context

- 2026-04-17: `checkpoint-a` / `checkpoint-b` revise flow fixed (P8 block added to `pipeline_next_action.go`).
- `AutoApprove=true` bypasses dynamic checkpoints entirely, so the problem only manifests with `AutoApprove=false`.
- `AutoApprove=false` is the default when running `/forge` with the `full` template without the `--auto` flag.

---

## Phase Registry: Deferred Scatter Points

The **declarative phase registry** refactor (`feature/declarative-phase-registry`) consolidated the six per-phase edit sites in `orchestrator/` into two (`state/state.go` + `orchestrator/registry.go`). Two additional per-phase tables were intentionally left out of scope to avoid cross-package coupling:

| Location | Symbol | Notes |
|---|---|---|
| `mcp-server/internal/handler/validation/artifact.go` | `artifactRules` | Per-phase lookup table of expected artifact filenames and required headings. Moving into `PhaseDescriptor` would force `engine/orchestrator` to import `handler/validation` (or vice versa), inverting the current clean dependency direction. |
| `mcp-server/internal/engine/state/manager.go` | `PhaseArtifacts` | Per-phase map of artifact filenames used by both `tools.Guard3aArtifactExists` and `state.PhaseCompleteArtifactCheck`. Lives in `engine/state` because both callers already depend on `engine/state`; lifting into a registry would create a circular import between `engine/orchestrator` and `engine/state`. |
| `mcp-server/internal/handler/tools/guards.go` | `phaseLogRequired` | Per-phase guard map consulted by MCP tool handlers. Encoding this in the descriptor would require `engine/orchestrator` to depend on `handler/tools`, which itself imports `engine/orchestrator` — creating a cycle. |

**Future direction:** If a registry package (`orchestrator/registry`) is ever extracted as a leaf package (no imports of `handler/validation`, `engine/state`, or `handler/tools`), all three tables could be merged into extended `PhaseDescriptor` fields. Until then, keep the tables in their respective packages and rely on `TestPhaseRegistryConsistency` + `TestPhaseRegistryLength` + the `initRegistry()` panic to detect ID-set drift.

---

## Codex Integration (upstream-blocked)

Publishing claude-forge as an OpenAI Codex plugin is blocked by upstream gaps in the Codex 0.121.0 plugin surface. Full analysis in [`docs/research/codex-integration.md`](./docs/research/codex-integration.md).

Unblock conditions:

1. Resolution of [openai/codex#15250](https://github.com/openai/codex/issues/15250) — `spawn_agent` accepts named custom agents.
2. `.codex-plugin/plugin.json` schema adds `agents` and `hooks` fields.
3. `CODEX_PLUGIN_ROOT` (or equivalent) env var is documented for hook scripts.
4. `PreToolUse` / `PostToolUse` coverage extends to `apply_patch` (Write / Edit equivalent).

Revisit the research doc whenever any upstream item changes.

---

## Devin-Class Autonomy: Gap Analysis

A reference comparison against Cognition's Devin (autonomous AI software engineer) to clarify what claude-forge would need to become a "set it and forget it" agent rather than a Claude Code-attached pipeline. The gaps split cleanly into three layers — Layer A (execution substrate) is a separate product, Layer B/C are extensions of the existing Go MCP server.

### Concept mapping

Status legend: ✅ shipped · ⏳ in-flight · ⬜ todo · 🚫 out-of-scope (separate product or upstream-blocked).

| Devin capability | Status | claude-forge today | Concrete asset that exists | Concrete missing piece |
|---|---|---|---|---|
| Cloud sandbox VM with shell + browser + editor | 🚫 | Runs in the user's Claude Code session on the developer's laptop | `forge-state` MCP is a stdio binary (`mcp-server/cmd/main.go`) that can run in any host with a TTY | A long-running orchestrator host (container / serverless job runtime) that owns the workspace, secrets, and CLI process between user prompts |
| Async trigger (Slack mention, Linear assignment, PR comment) | 🚫 | Synchronous `/forge <text>` only | `pipeline_init` accepts `github_issue` / `jira_issue` URLs (`mcp-server/internal/handler/tools/pipeline_init.go`); `events.SlackNotifier` posts outbound notifications (`mcp-server/pkg/events/slack.go`) | An inbound webhook receiver that turns a Slack/Linear/GitHub event into `pipeline_init_with_context` and dispatches to a runner pool |
| Real-time observability dashboard | ✅ | Embedded `/` HTML served by the dashboard package; subscribes to `/events` | `mcp-server/internal/dashboard/{server,handler,dashboard.html}.{go,html}`; opt-in via `FORGE_EVENTS_PORT`; URL is logged on startup | (none — first-cut shipped) |
| Mid-task intervention channel | ✅ | `POST /api/checkpoint/approve` and `POST /api/pipeline/abandon` driven by Approve / Abandon buttons in the dashboard | `mcp-server/internal/dashboard/intervention.go` (loopback + Origin-allowlist guard, structural URL parse), wired to `StateManager.PhaseComplete` / `Abandon` | Branch / fork action; richer "stop without abandon" semantics (currently abandon-only) |
| Multi-task parallelism (one agent, many tickets) | 🚫 | One Claude Code session = one pipeline; only Phase 5 implementers parallelize within a pipeline | Workspace is filesystem-isolated under `.specs/<spec-name>/`; state.json is per-workspace | A scheduler that pins each pipeline to a sandbox and load-balances across runners; required only after Layer A |
| Long-term knowledge ("Devin Knowledge") | ⬜ | `history_*` MCP tools surface past pipeline patterns and friction (`mcp-server/internal/intelligence/history/`) | `KnowledgeBase` indexes `.specs/` (`history/knowledge_base.go:18`), `prompt.BuildPrompt` already injects Layer 4 context with an 8 KT budget guard (`prompt/builder.go:11,29`) | Org-level knowledge: hand-written guidance, API contracts, code-review preferences that persist across repos and feed agent prompts |
| Repository awareness | ⬜ | `profile_get` analyses languages, CI, linters once per repo and caches (`mcp-server/internal/intelligence/profile/analyzer.go`) | Already injected as Layer 3 of the prompt | Per-developer / per-team overrides; profile invalidation strategy when `package.json` / `go.mod` changes |
| CI feedback loop | ⬜ | Pipeline ends at `pr-creation`; no monitoring of GitHub Actions afterwards | `gh pr create` is the last step in `SKILL.md`; `executeFinalCommit` does the push (`tools/git_ops.go:177`) | A post-PR monitor: poll `gh run watch` (or webhook), feed failures back into a new Phase 5 revision; this is BACKLOG **F19** with concrete shape |
| Budget guardrails | ⬜ | `effort` (S/M/L) chooses a flow template; `tokenBudget = 8_000` is only for prompt assembly (`prompt/builder.go:11`) | `analytics_estimate` returns P50/P90 token / cost predictions per `(task_type, effort)` (`analytics/estimator.go`) | Runtime enforcement: compare cumulative `phase-log` totals against an estimate; auto-checkpoint or auto-abandon at threshold. The estimator output is unused by `pipeline_next_action` today |
| Session forking / replay | ⬜ | `revision-bump` retries a single phase, `inline-revision-bump` retries within a phase | State migration helpers (`state/migration.go`) are in place | A `pipeline_fork(workspace, from_phase)` MCP tool that snapshots state.json + workspace dir into a sibling spec, enabling "what if we tried approach B from Phase 3" |
| Secrets management | 🚫 | Inherits the developer's shell environment and `~/.config` files | None | A vault adapter (e.g. 1Password Connect, AWS Secrets Manager, GitHub Encrypted Secrets) that materializes credentials into the sandbox per pipeline |
| Team handoff | ⏳ | `.specs/<spec-name>/` is committed to git, available to anyone with repo access; the dashboard SSE stream is reachable by any teammate on the same loopback | `post-to-source` checkpoint posts `summary.md` to GitHub / Jira; loopback-only intervention API blocks remote teammates today | Same-LAN "watching" mode: optionally bind the dashboard to a non-loopback interface, with auth, so a teammate can attach without owning the runner |
| Slack inbound | 🚫 | None | `SlackNotifier` is outbound-only; filters to `phase-complete` / `phase-fail` / `abandon` (`events/slack.go:40`) | A Slack Events API receiver that interprets `@forge run "<task>"` and threads progress back to the originating channel |
| Linear / Notion sources | ⬜ | GitHub Issues + Jira only | `pipeline_init` source-type detection lives in `pipeline_init.go` | New source-type branches; **Linear is BACKLOG #31**, Notion is unscoped |
| CLI portability | ⬜ | Claude Code only | MCP protocol is host-agnostic; the binary already runs against any MCP host | Codex, Cursor, JetBrains support — Codex is **upstream-blocked** (see Codex Integration section above) |

### Layered roadmap

The thirteen capabilities above are not all of equal scope. They cluster into three layers; the layer dictates whether the work belongs in this repo at all.

**Layer A — Execution substrate (a new product, not an extension of this plugin).** 🚫 out of scope for this BACKLOG.
Maps to: cloud sandbox, async triggers, multi-task parallelism, secrets management, team handoff (server side).
Why separate: Devin's primary value is "the agent runs while you sleep." That requires owning a process lifecycle, a filesystem, and a credentials store independent of any developer's CLI session. Building this *inside* claude-forge (a Claude Code plugin) creates an architectural conflict — the plugin model assumes a foreground host. A new repository (`claude-forge-runner` / `forge-cloud`) should consume `forge-state` as a library and expose an HTTP control plane, webhook receivers, a sandbox driver (Docker / Firecracker / GCP Cloud Run), and a secrets adapter.
Estimated scope: project-sized (multi-month), out of scope for this BACKLOG.

**Layer B — Observability and intervention (extensions to existing assets).** ⏳ partially shipped.
Maps to: dashboard, intervention channel, browser automation, session forking / replay, team handoff (client side).
Why fits here: every prerequisite exists. `EventBus` + `SSEHandler` already publish a typed event stream; `pipeline_next_action` already returns checkpoint payloads; state.json is already the authoritative store.
Done so far (lives entirely under `mcp-server/internal/dashboard/`):
- ✅ Dashboard MVP (single embedded HTML, opt-in via `FORGE_EVENTS_PORT`, click-through URL printed at startup).
- ✅ Intervention API + UI: `POST /api/checkpoint/approve`, `POST /api/pipeline/abandon`, plus inline Approve and header Abandon buttons. Loopback-only with structural Origin allowlist.
Remaining:
- ⬜ **`pipeline_fork(workspace, from_phase)` MCP tool** — snapshot state.json + workspace dir into a sibling spec to enable "what if we tried approach B from Phase 3".
- ⬜ **Stop-without-abandon intervention** — pause a running pipeline at the next safe boundary instead of marking it abandoned (today's only termination option).
- 🔒 **LAN-watch mode** — on hold. opt-in bind to a non-loopback interface with auth so a teammate can subscribe to the SSE stream without owning the runner. Revisit when multi-user demand is confirmed.

**Layer D — Autonomous task queue (batch execution).** ⬜ new (2026-04-17).
Maps to: sequential multi-task execution, Devin-style autonomous PR creation from a backlog of tickets.
Why a separate layer: Layers A–C extend the *single-pipeline* model. Layer D wraps the existing forge pipeline in an outer loop, processing a user-curated list of tasks without modifying forge internals.

Concept:
- User creates `.specs/queue.yaml` containing a list of issue URLs (Jira, GitHub, Linear, etc.) with effort levels.
- A new skill (`forge-queue`) parses the YAML, picks the first unprocessed task, and invokes the existing `forge` skill with `--auto` + the URL + effort.
- On completion: writes `status: completed` + PR number back to `queue.yaml`, moves to next task.
- On failure: writes `status: failed` + reason, abandons the pipeline, moves to next task.
- Terminates when all tasks are processed.

```yaml
# .specs/queue.yaml
tasks:
  - url: https://jira.example.com/browse/DEA-123
    effort: M
    status: completed
    pr: 2891
    workspace: 2026-04-17-dea-123-fix-login-validation
  - url: https://jira.example.com/browse/DEA-456
    effort: S
    status: failed
    reason: "phase-3: architect could not produce viable design"
    workspace: 2026-04-17-dea-456-add-export-feature
  - url: https://github.com/legalforce/dealon-app/issues/42
    effort: S
  - url: https://jira.example.com/browse/DEA-789
    effort: L
```

Design constraints:
- **Sequential only** — parallel execution is handled by the user opening multiple terminals.
- **`--auto` forced** — no checkpoints; each task runs to completion or failure autonomously.
- **Link-based input only** — tasks are specified as issue URLs; free-text tasks are not supported in queue mode.
- **No forge internals changes** — `forge-queue` is purely an outer loop that calls forge as-is.
- **State lives in `queue.yaml`** — no separate state file; the YAML is both input and status tracker.

Implementation:
- Two new skills: `skills/forge-queue/SKILL.md` (executor), `skills/forge-queue-create/SKILL.md` (generator).
- Five new MCP tools: `queue_create`, `queue_init`, `queue_next`, `queue_report`, `queue_update_pr` (YAML I/O + state.json read).
- New Go package: `mcp-server/internal/queue/`.
- Each task runs in an isolated `claude -p` subprocess (clean context per task).
- Effort: M.
- Full design: `docs/research/queue-design.md`.

**Layer C — Learning and self-recovery (extensions to history & analytics).** 🔒 on hold (2026-04-17).
Maps to: long-term knowledge, repository awareness deltas, CI feedback loop, budget guardrails, runtime estimator enforcement.
Why fits here: claude-forge already collects most of the data — `history_*`, `profile_get`, `analytics_*`, `phase-log`. What is missing is *closing the loop* so the data influences the running pipeline:

- 🔒 **CI feedback (BACKLOG F19)** — on hold. Post-PR CI watching is less valuable than strengthening pre-PR local verification (lint, test, build, typecheck via `profile_get` commands in `final-verification`). Revisit only after local verification is robust.
- 🔒 **Budget enforcement** — on hold. `analytics_estimate` relies on historical P50/P90, but cold-start (no data) and effort-only granularity (ignores task complexity) make the threshold unreliable. Needs a fallback constant design and richer prediction inputs before implementation.
- 🔒 **Org knowledge (`knowledge_search`)** — on hold. Largest unknown (embedding store choice). Defer until other layers produce enough data to know which knowledge sources are actually missing.
- 🔒 **Profile invalidation** — on hold. Lowest risk item but deferred along with the rest of Layer C.

These are all in-scope for the existing Go MCP server when unblocked.

### Implementation status snapshot

A glanceable view of remaining work. Effort is **post-Layer-B-MVP estimate**: the existing infrastructure (HTTP listener, StateManager guards, history index, etc.) absorbs much of the up-front cost.

| Layer | Item | Status | Effort | Blocks / depends on |
|---|---|---|---|---|
| B | Dashboard MVP (timeline, SSE) | ✅ done | — | — |
| B | Intervention API + Approve / Abandon UI | ✅ done | — | — |
| B | `pipeline_fork` MCP tool | ⬜ todo | M | StateManager snapshot helper |
| B | Stop-without-abandon | ⬜ todo | S | Add `StatusPaused` + matching guards |
| B | LAN-watch mode (auth) | 🔒 on hold | M | Multi-user demand unconfirmed |
| C | F19 — CI feedback loop | 🔒 on hold | L | Pre-PR local verification preferred |
| C | Budget enforcement | 🔒 on hold | M | Cold-start + granularity issues |
| C | `knowledge_search` MCP tool | 🔒 on hold | L | Embedding store decision |
| C | Profile invalidation on lockfile drift | 🔒 on hold | S | Layer C deferred as a whole |
| D | `forge-queue` (autonomous task queue) | ⬜ todo | M | 5 MCP tools + 2 skills + Go package |
| A | Cloud sandbox / runner / secrets / scheduler | 🚫 separate product | XL | Whole new repo |

### Recommended sequence (updated 2026-04-17)

Layer B Phase 1 (dashboard + intervention) is shipped on `feature/sse-dashboard-mvp`. Layer C is **on hold** pending foundational improvements (local verification, prediction accuracy). The next steps focus on Layer B:

1. **Layer D / `forge-queue`** (S–M). Highest immediate ROI — enables autonomous batch execution of stocked tasks with zero changes to forge internals. New skill only.
2. **Layer B / `pipeline_fork`** (M). Pairs naturally with the intervention UI — a "fork" button next to Approve / Abandon completes the intervention triad.
3. **Layer B / Stop-without-abandon** (S). One-line addition to the existing intervention API; differentiates "I want to look at this" from "kill it."
4. **Layer B / LAN-watch mode** (M). On hold — revisit when multi-user demand is confirmed.
5. **Layer A** stays out of scope unless a multi-developer offering becomes a strategic goal — that's a separate product decision, not a BACKLOG item.
6. **Layer C** — revisit when: (a) pre-PR local verification (`final-verification` using `profile_get` commands) is robust, (b) `analytics_estimate` has enough historical data and richer inputs (task complexity, language, file count) for reliable predictions.

### What "Devin-class" explicitly does *not* require

For honesty: claude-forge already matches Devin in several places that look like gaps but are not.

- **Multi-phase orchestration with isolated subagents** — already core, in fact stronger than Devin's flat planner.
- **State persistence across restarts** — `state.json` + the 26 state-management commands cover this; Devin's session resume is no more sophisticated.
- **AI review loops (design-reviewer / impl-reviewer / comprehensive-reviewer)** — claude-forge's APPROVE/REVISE cycle has no documented equivalent inside Devin.
- **Effort-aware flow templates** — the `light` / `standard` / `full` template selection is more transparent than Devin's opaque scoping.

The deficit is therefore not in *what the agent can decide* but in *where and when it can run, and how a human watches it*. Layer A and Layer B together close that perception gap; Layer C closes the substantive quality gap once both are in place.

---

## P1: Pipeline Execution Speed — 3-4× Slower Than Superpowers

### Problem

Real-world pipeline run on `dealon-app` DEA-221 (33 PostgreSQL ENUM → TEXT + CHECK migration, Effort M) took **~210 minutes** end-to-end. The same task with superpowers (brainstorm → plan → execute) would take an estimated **50-60 minutes**. The 3-4× slowdown has five root causes, three of which are fixable without architectural changes.

### Time breakdown (DEA-221, 2026-04-30)

| Phase | Duration | Notes |
|-------|----------|-------|
| Phase 1: Situation Analysis | ~5min | Reasonable |
| Phase 2: Investigation | ~8min | Reasonable |
| Phase 3 + 3b: Design + Review (initial) | ~9min | |
| Phase 3 + 3b: REVISE round 1 | ~10min | Missing `default_job_generation_guideline_templates` table |
| Phase 3 + 3b: REVISE round 2 | ~7min | Missing `chat_repository.py` + cross-file DROP TYPE ambiguity |
| Phase 3 + 3b: REVISE round 3 | ~6min | Missing `task_proposals.py` + `test_save_task_title.py` |
| Phase 3 + 3b: Final APPROVE_WITH_NOTES | ~2min | |
| Checkpoint A (user wait) | ~4min | |
| Phase 4: Task Decomposition | ~2min | |
| Phase 5: Tasks 1-7 (parallel OK) | ~8min | Parallel dispatch worked for initial batch |
| Phase 5: Task 8 (sqlc-gen, sequential) | ~5min | |
| Phase 5: Tasks 9-16 (sequential, should be parallel) | ~120min | **Biggest bottleneck.** Includes one 2-hour agent timeout |
| Phase 5: Task 17 (verification) | ~26min | |
| **Total** | **~210min** | |

### Root cause 1: Design Review REVISE loop — ~25min wasted

**Symptom:** Design reviewer found 2 CRITICAL findings → REVISE → architect fixed those 2 → reviewer found 2 *new* CRITICALs → REVISE → architect fixed → reviewer found 2 *more* → REVISE again. Three full REVISE cycles before APPROVE.

**Root cause:** Architect listed affected files from investigation.md without grepping the codebase. Each reviewer pass found Python files (`chat_repository.py`, `task_proposals.py`, `test_save_task_title.py`) that import generated ENUM classes — these were never mentioned in investigation.md because the investigator also didn't grep comprehensively.

**Partial fix applied (2026-04-30):** Added "Comprehensive Impact Scan for Deletions and Type Changes" section to `agents/architect.md` requiring a full codebase grep before listing affected files.

**Remaining fix — REVISE cap with auto-escalation:**

Currently `handlePhaseThreeB` (in `engine.go`) allows unlimited REVISE cycles. After 2 REVISE rounds, the probability that a 3rd review will find a *new* CRITICAL is low — the architect should have done a comprehensive scan by then. Proposal:

1. Add a configurable `MaxDesignRevisions` (default: 2) to state or preferences.
2. After `MaxDesignRevisions` REVISE verdicts, auto-promote to `APPROVE_WITH_NOTES` with a warning: "REVISE cap reached after N rounds — proceeding with remaining MINOR findings."
3. Inject remaining CRITICAL findings into the implementer's prompt as "known issues to address during implementation" so they are not silently dropped.

This caps the worst case at 2 revision rounds (~15min) instead of unbounded.

**Files to change:**
- `mcp-server/internal/engine/orchestrator/engine.go` — `handlePhaseThreeB`: add revision counter check
- `mcp-server/internal/engine/state/state.go` — add `MaxDesignRevisions` preference field
- `agents/architect.md` — already done (grep requirement)

### Root cause 2: Parallel task dispatch failure — ~90min wasted

**Symptom:** Tasks 9-16 were all marked `mode: parallel` in tasks.md with `depends_on: [8]` (correct). After Task 8 completed, `pipeline_next_action` returned Task 9 as a *single* `spawn_agent` without `parallel_task_ids`. Tasks 10-16 were dispatched one by one.

**Root cause (hypothesis — needs investigation):** The parallel detection in `handlePhaseFive` (engine.go L537-556) works correctly when tested in isolation. The likely cause is one of:
1. **`task_init` parse failure:** `ParseTasksMd` may have failed to parse `mode: parallel` for some tasks due to formatting variations (e.g., `mode:parallel` without space, or `mode: parallel` with trailing whitespace). The task decomposer wrote `mode: parallel` but the parser may require exact formatting.
2. **Dependency resolution:** Tasks 9-16 all depend on Task 8. If `depends_on` is not cleared after Task 8 completes, the tasks remain "blocked" and are not included in `pendingKeys`.
3. **State inconsistency after manual state.json edit:** The orchestrator manually edited state.json to fix the batch commit failure (Root cause 3). This may have corrupted task state, breaking the parallel detection.

**Investigation needed:**
- Add debug logging to `handlePhaseFive` showing `pendingKeys`, `firstTask.ExecutionMode`, and the parallel group detection result.
- Add a test case to `engine_test.go` that reproduces: 8 tasks where 1 is sequential (depends on nothing) and 7 are parallel (depend on 1). After task 1 completes, the engine should return a `NewParallelSpawnAction` with 7 task IDs.
- Review `ParseTasksMd` for whitespace sensitivity in `mode:` field parsing.

**Files to investigate:**
- `mcp-server/internal/engine/orchestrator/engine.go` — `handlePhaseFive` parallel detection
- `mcp-server/internal/engine/state/tasks_parser.go` — `ParseTasksMd` mode parsing
- `mcp-server/internal/handler/tools/pipeline_next_action.go` — task_init and reporting

### Root cause 3: Batch commit pathspec failure + stuck state — ~15min wasted (manual recovery)

**Symptom:** `executeBatchCommit` failed with `fatal: pathspec 'backend/pkg/db/query/mail_receiving/mail_receiving.sql' did not match any files`. The `needsBatchCommit` flag remained `true`, and every subsequent `pipeline_next_action` call retried the same failing commit. Required manual state.json editing to recover.

**Root cause:** Task decomposer listed `mail_receiving/mail_receiving.sql` in the `files:` field, but the actual files were `mail_receiving/emails.sql`, `mail_receiving/tenants.sql`, etc. `executeBatchCommit` collected file paths from `task.Files` and passed them all to `git add --`, which failed on the non-existent path.

**Fix applied (2026-04-30):** Three changes:
1. `agents/task-decomposer.md` — added file path verification requirement
2. `mcp-server/internal/handler/tools/git_ops.go` — `executeBatchCommit` now filters non-existent paths via `os.Stat` before `git add`, falls back to `git diff --name-only HEAD` when all paths are invalid
3. Tests added: `TestExecuteBatchCommit_MixedValidInvalidPaths`, `TestExecuteBatchCommit_AllInvalidPaths`, `TestExecuteBatchCommit_AllValidPaths`

### Root cause 4: Branch name mismatch in implementer prompt

**Symptom:** `pipeline_init_with_context` returned `branch: "feature/dea-221-db-enum-to-text-check-migration"`, and the orchestrator created this branch. But `pipeline_next_action` injected `fix/dea-221-db-enum-to-text-check-migration` into the implementer prompt. Implementer agents tried `git checkout fix/...` and failed, wasting a few minutes each.

**Root cause (hypothesis):** The branch name in the implementer prompt template uses a `{branch}` placeholder replaced by `pipeline_next_action.go`. The replacement value may come from `branchClassified` (the initial classification of source_type → branch prefix) rather than `st.Branch` (the actual branch created). If the user overrides the branch prefix during `pipeline_init_with_context` (e.g., by choosing `feature/` over `fix/`), `branchClassified` and `st.Branch` diverge.

**Investigation needed:**
- Trace the `{branch}` replacement in `pipeline_next_action.go` — confirm it reads from `st.Branch` (correct) or from `branchClassified` (bug).
- If the replacement uses `st.Branch`, the issue may be in `DeriveBranchName` applying a different prefix classification than what was actually created.

**Files to investigate:**
- `mcp-server/internal/handler/tools/pipeline_next_action.go` — `enrichPrompt` or template replacement logic
- `mcp-server/internal/handler/tools/pipeline_init_with_context.go` — `DeriveBranchName`

### Root cause 5: Per-phase overhead accumulation

**Not a bug, but a structural cost.** Each phase involves: MCP call to `pipeline_next_action` → state.json read → agent prompt enrichment → agent spawn → artifact write → MCP call to `pipeline_report_result` → state.json update. For Effort M (13 active phases), this adds ~1-2 min of overhead per phase = ~15-25 min total. Superpowers has zero phase management overhead.

**No immediate fix needed**, but this context explains why forge will always be slower than superpowers for small/well-specified tasks where the design and task decomposition phases add no value. The right mitigation is using Effort S aggressively for tasks with detailed external specs (Linear/Jira issues that already contain the design).

### Summary of actions

| # | Action | Status | Effort | Impact |
|---|--------|--------|--------|--------|
| 1 | Architect comprehensive grep requirement | ✅ Done | — | Prevents REVISE loops |
| 2 | Task decomposer file path verification | ✅ Done | — | Prevents batch commit failure |
| 3 | `executeBatchCommit` path filtering | ✅ Done | — | Graceful degradation on bad paths |
| 4 | REVISE cap (MaxDesignReviseRounds=2) | ✅ Done | S | Caps worst-case REVISE time at ~15min. Auto-promotes to APPROVE_WITH_NOTES after 2 REVISE rounds, passing remaining findings to implementers via `DesignReviseCapReached` state flag and `review-design.md` as input artifact. |
| 5 | Parallel task dispatch — dependency-aware filtering | ✅ Done | M | `handlePhaseFive` now filters `pendingKeys` by `DependsOn` satisfaction, preventing tasks with unmet dependencies from being dispatched. Debug logging added (gated by `st.Debug`). |
| 6 | Branch name mismatch — auto-checkout validation | ✅ Done | S | `pipeline_next_action` validates `st.Branch` against `git rev-parse --abbrev-ref HEAD` before dispatching agents. Auto-checkouts the correct branch on mismatch with a warning. |

---

## Recently Resolved (2026-04-17)

Five issues identified during a real-world `/forge` pipeline run on `dealon-app` (DEA-13: Proto Enum sync improvement). All fixed in a single batch:

| # | Issue | Fix location | Description |
|---|-------|-------------|-------------|
| 1 | Design revision loop stuck | `verdict_parser.go` determineTransition | REVISE verdict spawned architect but stale `review-design.md` caused infinite loop. Fix: detect post-revision stale review via mtime comparison and delete before re-dispatching reviewer. |
| 2 | Checkpoint revise didn't clean review files | `pipeline_next_action.go` P8 block | User choosing "revise" at checkpoint-a rewound to phase-3 but left stale `review-design.md`, preventing re-review. Fix: delete review files on checkpoint rewind. |
| 3 | Common Review Findings showed stale entries | `pipeline_next_action.go` enrichPrompt | Architect saw "seen N times" for already-resolved CRITICALs. Fix: filter patterns matching current review-design.md findings before injecting into prompt. |
| 4 | Architect didn't verify code assumptions | `agents/architect.md` | Architect wrote design details about APIs/types without reading actual source code, causing repeated CRITICAL findings. Fix: added "Verify Before You Write" section with explicit instructions. |
| 5 | Pipeline state opaque during debugging | `pipeline_next_action.go` response struct | `pipeline_next_action` response didn't include current phase/status, making it hard to diagnose state issues. Fix: added `current_phase` and `current_phase_status` fields to response. |

---

## Improvement Candidates

| Issue | Title | Notes |
|-------|-------|-------|
| [#21](https://github.com/hiromaily/claude-forge/issues/21) | Model selection per agent | Use opus for architect, design-reviewer, implementer. ~2× cost increase. |
| [#22](https://github.com/hiromaily/claude-forge/issues/22) | Agent-level retry with context carry-forward | Use `resume` parameter to preserve agent reasoning across retries. Needs feasibility testing. |
| [#23](https://github.com/hiromaily/claude-forge/issues/23) | Parallel Phase 5-6 interleaving | Spawn Phase 6 review immediately after each Phase 5 task. Complex state tracking. |
| [#24](https://github.com/hiromaily/claude-forge/issues/24) | Workspace directory naming | Rename `.specs/` → `.claude-forge/` to avoid collision with kiro specs. Breaking change — migration needed. |
| [#25](https://github.com/hiromaily/claude-forge/issues/25) | Hook-based progress notifications | Log phase transitions to `progress.log`; optional Slack webhook. |
| [#27](https://github.com/hiromaily/claude-forge/issues/27) | Per-project setup wizard | Interactive first-run wizard persisting project conventions to a profile file. Complements the existing `profile_get` automated profiling. Source: aaddrick/claude-pipeline. |
| [#28](https://github.com/hiromaily/claude-forge/issues/28) | JSON-driven agent configuration | Declarative `agents.json` schema for agent metadata — eliminates drift across roster tables. Source: catlog22/Claude-Code-Workflow. |
| [#29](https://github.com/hiromaily/claude-forge/issues/29) | Cold start optimization | Reduce XS/S pipeline startup overhead via lazy workspace setup and merged validation passes. Source: barkain. |
| [#30](https://github.com/hiromaily/claude-forge/issues/30) | Agent Teams mode (Phase 5 inter-agent comms) | Collaborative mode with `comms.json` for real-time coordination. High complexity — defer until pain confirmed by phase-stats data. Source: barkain. |
| [#31](https://github.com/hiromaily/claude-forge/issues/31) | Linear integration | Add `linear_issue` source type alongside GitHub Issues and Jira. Source: levnikolaevich. |
| [#32](https://github.com/hiromaily/claude-forge/issues/32) | Native plan mode integration at checkpoints | Use EnterPlanMode at Checkpoint A/B for structured task/PR review. Source: barkain. |

---

> **Testing Checklist** has been moved to `.claude/rules/testing.md` for automatic reference during changes.

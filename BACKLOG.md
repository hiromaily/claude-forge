
# Claude-Forge Plugin — Backlog

Known issues, improvement candidates, and future direction.

---

## Priority Queue

Ordered by priority. Higher rows should be tackled first.

| # | ID | Issue | Title | Type | Effort | Why now |
|---|-----|-------|-------|------|--------|---------|
| 1 | **F10** | [#12](https://github.com/hiromaily/claude-forge/issues/12) | Partial execution (`--until`/`--from`) | Feature | M | `--until=design` for scoping only, `--from=phase-5` for re-implementation. Combines with `--auto` for autonomous scoping reports. |
| 2 | **F9** | [#13](https://github.com/hiromaily/claude-forge/issues/13) | Structured acceptance criteria | Feature | M | Improves PASS/FAIL consistency. Currently depends on impl-reviewer's subjective interpretation. |
| 3 | **F12** | [#14](https://github.com/hiromaily/claude-forge/issues/14) | Checkpoint diff preview | Feature | S | Nice-to-have. `--auto` reduces checkpoint frequency, lowering the priority. |
| 4 | **F18** | [#17](https://github.com/hiromaily/claude-forge/issues/17) | Improvement report → test case generation | Feature | S | Auto-generate hook guard test cases from friction points found in improvement reports. Builds on the existing `history_get_friction_map` data. |
| 5 | **F19** | [#18](https://github.com/hiromaily/claude-forge/issues/18) | CI feedback loop (post-PR auto-fix) | Feature | L | After PR creation, monitor CI results and auto-trigger fix flow on failure. Closes the quality loop beyond the pipeline boundary. |
| 6 | **F6** | [#19](https://github.com/hiromaily/claude-forge/issues/19) | Adaptive model routing | Feature | L | Needs phase-stats data before deciding. Could now be informed by the accumulated `analytics_*` metrics. |
| 7 | **F2** | [#20](https://github.com/hiromaily/claude-forge/issues/20) | Execution log (JSONL) | Feature | M | Basic coverage via phase-log. Full JSONL log deferred until the need is confirmed. |

**Effort key:** XS = < 30min, S = 1-2h, M = half day, L = 1+ day

**Prioritization criteria:**

1. **Blocking bug** — fix first
2. **Determinism** — hook guards to cover AI non-determinism
3. **Dev loop acceleration** — high ROI (F10)
4. **Data flywheel extensions** — leverage the accumulated `history_*` and `profile_*` data (F18)
5. **Cost reduction** — validate with phase-stats data (F6)
6. **Future features** — after data accumulation (F12, F19)

---

## Phase Registry: Deferred Scatter Points

The **declarative phase registry** refactor (`feature/declarative-phase-registry`) consolidated the six per-phase edit sites in `orchestrator/` into two (`state/state.go` + `orchestrator/registry.go`). Two additional per-phase tables were intentionally left out of scope to avoid cross-package coupling:

| Location | Symbol | Notes |
|---|---|---|
| `mcp-server/internal/validation/artifact.go` | `artifactRules` | Per-phase lookup table of expected artifact filenames and required headings. Moving into `PhaseDescriptor` would force `orchestrator` to import `validation` (or vice versa), inverting the current clean dependency direction. |
| `mcp-server/internal/state/manager.go` | `PhaseArtifacts` | Per-phase map of artifact filenames used by both `tools.Guard3aArtifactExists` and `state.PhaseCompleteArtifactCheck`. Lives in `state` because both callers already depend on `state`; lifting into a registry would create a circular import between `orchestrator` and `state`. |
| `mcp-server/internal/tools/guards.go` | `phaseLogRequired` | Per-phase guard map consulted by MCP tool handlers. Encoding this in the descriptor would require `orchestrator` to depend on `tools`, which itself imports `orchestrator` — creating a cycle. |

**Future direction:** If a registry package (`orchestrator/registry`) is ever extracted as a leaf package (no imports of `validation`, `state`, or `tools`), all three tables could be merged into extended `PhaseDescriptor` fields. Until then, keep the tables in their respective packages and rely on `TestPhaseRegistryConsistency` + `TestPhaseRegistryLength` + the `initRegistry()` panic to detect ID-set drift.

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
| Async trigger (Slack mention, Linear assignment, PR comment) | 🚫 | Synchronous `/forge <text>` only | `pipeline_init` accepts `github_issue` / `jira_issue` URLs (`mcp-server/internal/tools/pipeline_init.go`); `events.SlackNotifier` posts outbound notifications (`mcp-server/internal/events/slack.go`) | An inbound webhook receiver that turns a Slack/Linear/GitHub event into `pipeline_init_with_context` and dispatches to a runner pool |
| Real-time observability dashboard | ✅ | Embedded `/` HTML served by the dashboard package; subscribes to `/events` | `mcp-server/internal/dashboard/{server,handler,dashboard.html}.{go,html}`; opt-in via `FORGE_EVENTS_PORT`; URL is logged on startup | (none — first-cut shipped) |
| Mid-task intervention channel | ✅ | `POST /api/checkpoint/approve` and `POST /api/pipeline/abandon` driven by Approve / Abandon buttons in the dashboard | `mcp-server/internal/dashboard/intervention.go` (loopback + Origin-allowlist guard, structural URL parse), wired to `StateManager.PhaseComplete` / `Abandon` | Branch / fork action; richer "stop without abandon" semantics (currently abandon-only) |
| Multi-task parallelism (one agent, many tickets) | 🚫 | One Claude Code session = one pipeline; only Phase 5 implementers parallelize within a pipeline | Workspace is filesystem-isolated under `.specs/<spec-name>/`; state.json is per-workspace | A scheduler that pins each pipeline to a sandbox and load-balances across runners; required only after Layer A |
| Long-term knowledge ("Devin Knowledge") | ⬜ | `history_*` MCP tools surface past pipeline patterns and friction (`mcp-server/internal/history/`) | `KnowledgeBase` indexes `.specs/` (`history/knowledge_base.go:18`), `prompt.BuildPrompt` already injects Layer 4 context with an 8 KT budget guard (`prompt/builder.go:11,29`) | Org-level knowledge: hand-written guidance, API contracts, code-review preferences that persist across repos and feed agent prompts |
| Repository awareness | ⬜ | `profile_get` analyses languages, CI, linters once per repo and caches (`mcp-server/internal/profile/analyzer.go`) | Already injected as Layer 3 of the prompt | Per-developer / per-team overrides; profile invalidation strategy when `package.json` / `go.mod` changes |
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
- ⬜ **Browser MCP tool** — Playwright / Chromium DevTools wrapper so an agent can read auth-walled docs, scrape internal portals, and screenshot UI state. Largest single Layer B item.
- ⬜ **`pipeline_fork(workspace, from_phase)` MCP tool** — snapshot state.json + workspace dir into a sibling spec to enable "what if we tried approach B from Phase 3".
- ⬜ **Stop-without-abandon intervention** — pause a running pipeline at the next safe boundary instead of marking it abandoned (today's only termination option).
- ⬜ **LAN-watch mode** — opt-in bind to a non-loopback interface with auth so a teammate can subscribe to the SSE stream without owning the runner.

**Layer C — Learning and self-recovery (extensions to history & analytics).** ⬜ entirely todo.
Maps to: long-term knowledge, repository awareness deltas, CI feedback loop, budget guardrails, runtime estimator enforcement.
Why fits here: claude-forge already collects most of the data — `history_*`, `profile_get`, `analytics_*`, `phase-log`. What is missing is *closing the loop* so the data influences the running pipeline:

- ⬜ **CI feedback (BACKLOG F19)** — concretely: after `pr-creation`, spawn a watcher that polls `gh run list --branch <branch> --json status,conclusion`, parses failures from the run log, and re-enters Phase 5 with the failure as a revision finding.
- ⬜ **Budget enforcement** — concretely: `pipeline_report_result` already accumulates `tokensUsed` per phase; add a `BudgetGuard` step that consults `analytics_estimate` and returns `action.type == "checkpoint"` with `present_to_user = "Budget exceeded P90 — continue?"` when cumulative tokens > P90 prediction × 1.2.
- ⬜ **Org knowledge (`knowledge_search`)** — concretely: a new MCP tool that queries an embedding index over user-supplied markdown (e.g. `docs/conventions/*.md`, ADRs); `prompt.BuildPrompt` adds a Layer 5 with the same budget-truncation discipline.
- ⬜ **Profile invalidation** — concretely: `profile.Cache` already exists; add a hash check on `package.json` / `go.mod` / `pnpm-lock.yaml` modification times and re-run analysis on drift.

These are all in-scope for the existing Go MCP server.

### Implementation status snapshot

A glanceable view of remaining work. Effort is **post-Layer-B-MVP estimate**: the existing infrastructure (HTTP listener, StateManager guards, history index, etc.) absorbs much of the up-front cost.

| Layer | Item | Status | Effort | Blocks / depends on |
|---|---|---|---|---|
| B | Dashboard MVP (timeline, SSE) | ✅ done | — | — |
| B | Intervention API + Approve / Abandon UI | ✅ done | — | — |
| B | Browser MCP (Playwright) | ⬜ todo | L | None — independent MCP tool |
| B | `pipeline_fork` MCP tool | ⬜ todo | M | StateManager snapshot helper |
| B | Stop-without-abandon | ⬜ todo | S | Add `StatusPaused` + matching guards |
| B | LAN-watch mode (auth) | ⬜ todo | M | Requires auth scheme decision |
| C | F19 — CI feedback loop | ⬜ todo | L | `gh` CLI or GitHub webhook receiver |
| C | Budget enforcement | ⬜ todo | M | `analytics_estimate` (already shipped) |
| C | `knowledge_search` MCP tool | ⬜ todo | L | Embedding store decision |
| C | Profile invalidation on lockfile drift | ⬜ todo | S | None |
| A | Cloud sandbox / runner / secrets / scheduler | 🚫 separate product | XL | Whole new repo |

### Recommended sequence (updated 2026-04-16)

Layer B Phase 1 (dashboard + intervention) is shipped on `feature/sse-dashboard-mvp`. The natural next steps, in order of expected ROI:

1. **Layer C / Budget enforcement** (M, ~1 day). Smallest Layer C item, immediate Devin-flavoured win: the dashboard already shows phase costs via SSE, so an auto-checkpoint at P90 × 1.2 is visible the moment it triggers. Closes the loop on data the repo already collects.
2. **Layer C / F19 CI feedback** (L, 2–3 days). Highest-value pipeline-quality improvement. Extends the value chain from "PR opened" to "PR mergeable."
3. **Layer B / `pipeline_fork`** (M). Pairs naturally with the intervention UI — a "fork" button next to Approve / Abandon completes the intervention triad.
4. **Layer B / Stop-without-abandon** (S). One-line addition to the existing intervention API; differentiates "I want to look at this" from "kill it."
5. **Layer C / `knowledge_search`** (L). Largest unknown (embedding store choice). Defer until budget/CI loops have produced enough data to know which knowledge sources are actually missing.
6. **Layer B / Browser MCP** (L). Independent track; pick up when an agent task requires it.
7. **Layer A** stays out of scope unless a multi-developer offering becomes a strategic goal — that's a separate product decision, not a BACKLOG item.

### What "Devin-class" explicitly does *not* require

For honesty: claude-forge already matches Devin in several places that look like gaps but are not.

- **Multi-phase orchestration with isolated subagents** — already core, in fact stronger than Devin's flat planner.
- **State persistence across restarts** — `state.json` + the 26 state-management commands cover this; Devin's session resume is no more sophisticated.
- **AI review loops (design-reviewer / impl-reviewer / comprehensive-reviewer)** — claude-forge's APPROVE/REVISE cycle has no documented equivalent inside Devin.
- **Effort-aware flow templates** — the `light` / `standard` / `full` template selection is more transparent than Devin's opaque scoping.

The deficit is therefore not in *what the agent can decide* but in *where and when it can run, and how a human watches it*. Layer A and Layer B together close that perception gap; Layer C closes the substantive quality gap once both are in place.

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

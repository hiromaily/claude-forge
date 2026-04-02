
# Claude-Forge Plugin — Backlog

Known issues, improvement candidates, and future direction.

---

## Priority Queue

Ordered by priority. Higher rows should be tackled first.

| # | ID | Issue | Title | Type | Effort | Why now |
|---|-----|-------|-------|------|--------|---------|
| 1 | **B2** | — | Final summary output filename inconsistency: engine writes `final-summary.md` but spec says `summary.md` | Bug | XS | `engine.go` and `artifact.go` use `final-summary.md`; `guards.go` maps `final-summary` → `summary.md`; BACKLOG checklist and verifier agent prompt reference `summary.md`. Decide canonical name, then align all four locations: `engine.go:554`, `validation/artifact.go:49`, `tools/guards.go:28`, and `agents/verifier.md`. |
| 2 | **B3** | — | `pipeline_next_action` response too large — triggers "Large MCP response" error | Bug | S | `pipeline_next_action` embeds full artifact file contents (`tasks.md`, `design.md`) inside the returned `prompt` field. For large tasks files the response exceeds Claude Code's MCP response limit, causing the response to be saved to a temp file rather than returned inline. **Fix:** strip embedded artifact content from the MCP response. Instead of inlining `tasks.md`/`design.md` text, return only the agent name, phase, output file path, and structured parameters. The orchestrator already has access to the workspace path and can read artifacts directly. This reduces `pipeline_next_action` response size from ~50 KB to ~1–2 KB for typical pipelines. |
| 3 | **F10** | [#12](https://github.com/hiromaily/claude-forge/issues/12) | Partial execution (`--until`/`--from`) | Feature | M | `--until=design` for scoping only, `--from=phase-5` for re-implementation. Combines with `--auto` for autonomous scoping reports. |
| 4 | **F9** | [#13](https://github.com/hiromaily/claude-forge/issues/13) | Structured acceptance criteria | Feature | M | Improves PASS/FAIL consistency. Currently depends on impl-reviewer's subjective interpretation. |
| 5 | **F12** | [#14](https://github.com/hiromaily/claude-forge/issues/14) | Checkpoint diff preview | Feature | S | Nice-to-have. `--auto` reduces checkpoint frequency, lowering the priority. |
| 6 | **F8** | [#15](https://github.com/hiromaily/claude-forge/issues/15) | Past pipeline reference (data flywheel) | Feature | L | Uses `.specs/` history to improve future pipelines. The accumulated data is a moat — competitors can copy code but not execution history. |
| 7 | **F17** | [#16](https://github.com/hiromaily/claude-forge/issues/16) | Repository profiling | Feature | M | First-run analysis of repo structure, test strategy, CI config → persisted profile that tunes agent prompts. Hard to replicate without per-repo data. |
| 8 | **F18** | [#17](https://github.com/hiromaily/claude-forge/issues/17) | Improvement report → test case generation | Feature | S | Auto-generate hook guard test cases from friction points found in improvement reports. Accelerates deterministic guard accumulation. |
| 9 | **F19** | [#18](https://github.com/hiromaily/claude-forge/issues/18) | CI feedback loop (post-PR auto-fix) | Feature | L | After PR creation, monitor CI results and auto-trigger fix flow on failure. Closes the quality loop beyond the pipeline boundary. |
| 10 | **F6** | [#19](https://github.com/hiromaily/claude-forge/issues/19) | Adaptive model routing | Feature | L | Needs phase-stats data before deciding. F13 (effort axis) provides the foundation for model selection. |
| 11 | **F2** | [#20](https://github.com/hiromaily/claude-forge/issues/20) | Execution log (JSONL) | Feature | M | Basic coverage via phase-log. Full JSONL log deferred until the need is confirmed. |

**Effort key:** XS = < 30min, S = 1-2h, M = half day, L = 1+ day

**Prioritization criteria:**

1. **Blocking bug** — fix first
2. **Determinism** — hook guards to cover AI non-determinism
3. **Dev loop acceleration** — high ROI (F10)
4. **Competitive moat** — data flywheel and per-repo learning (F8, F17, F18)
5. **Cost reduction** — validate with phase-stats data (F5, F6)
6. **Future features** — after data accumulation (F12, F19)

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

## Testing Checklist

When making changes to this plugin, verify:

- [ ] Go state manager: all commands work — run `cd mcp-server && go test ./state/...` to verify init, phase-start, phase-complete, phase-fail, checkpoint, task-init, task-update, revision-bump, set-branch, skip-phase, set-auto-approve, set-skip-pr, phase-log, phase-stats, abandon, resume-info, get, set-effort, set-flow-template
- [ ] Go state manager: PHASES array includes phase-7, pr-creation, post-to-source — `cd mcp-server && go test ./state/...`
- [ ] Go state manager: numeric fields (implRetries, reviewRetries) stay as numbers after task-update — `cd mcp-server && go test ./state/...`
- [ ] Go state manager: special characters in spec-name don't break JSON — `cd mcp-server && go test ./state/...`
- [ ] Go state manager: `set_auto_approve` sets `autoApprove: true` and updates `lastUpdated` — `cd mcp-server && go test ./state/...`
- [ ] Go state manager: `resume_info` projects `autoApprove` with false default — `cd mcp-server && go test ./state/...`
- [ ] Go state manager: `set_skip_pr` sets `skipPr: true` and updates `lastUpdated` — `cd mcp-server && go test ./state/...`
- [ ] Go state manager: `resume_info` projects `skipPr` with false default — `cd mcp-server && go test ./state/...`
- [ ] Go state manager: `set_effort` — S/M/L accepted; XS and invalid values rejected — `cd mcp-server && go test ./state/...`
- [ ] Go state manager: `set_flow_template` — all three templates accepted (light/standard/full); invalid value rejected — `cd mcp-server && go test ./state/...`
- [ ] Go state manager: `resume_info` — `effort` and `flowTemplate` fields present and null-safe — `cd mcp-server && go test ./state/...`
- [ ] Go state manager: `phase_log` appends to `phaseLog` array with correct fields — `cd mcp-server && go test ./state/...`
- [ ] Go state manager: `phase_stats` outputs formatted table — `cd mcp-server && go test ./state/...`
- [ ] `pre-tool-hook.sh`: Edit/Write blocked during Phase 1-2 (exit 2)
- [ ] `pre-tool-hook.sh`: Edit/Write allowed for workspace files during Phase 1-2 (exit 0)
- [ ] `pre-tool-hook.sh`: git commit blocked during parallel Phase 5 (exit 2)
- [ ] `pre-tool-hook.sh`: git commit allowed during sequential Phase 5 (exit 0)
- [ ] `pre-tool-hook.sh`: no-op when no active pipeline (exit 0)
- [ ] `pre-tool-hook.sh`: no-op when pipeline is abandoned (exit 0)
- [ ] `pre-tool-hook.sh`: blocks `phase-complete checkpoint-a/b` when `currentPhaseStatus != "awaiting_human"` (exit 2)
- [ ] `pre-tool-hook.sh`: allows `phase-complete` for non-checkpoint phases regardless of status (exit 0)
- [ ] `pre-tool-hook.sh`: artifact guard blocks `phase-complete` when required artifact file is missing (exit 2)
- [ ] `pre-tool-hook.sh`: Rule 3f — `phase-start phase-1` when `effort` is null emits warning to stderr and exits 0 (non-blocking)
- [ ] `pre-tool-hook.sh`: Rule 3f — `phase-start phase-1` when effort is set emits no warning and exits 0
- [ ] `post-agent-hook.sh`: warns on empty agent output
- [ ] `post-agent-hook.sh`: warns on missing verdict in review phases
- [ ] `stop-hook.sh`: blocks stop when pipeline active (exit 2)
- [ ] `stop-hook.sh`: allows stop at checkpoints (exit 0)
- [ ] `stop-hook.sh`: allows stop when pipeline completed (exit 0)
- [ ] `stop-hook.sh`: allows stop when pipeline abandoned (exit 0)
- [ ] SKILL.md Agent Roster matches each agent's actual Input section (10 agents)
- [ ] All phase IDs in SKILL.md exist in the Go MCP server's PHASES list (`cd mcp-server && go test ./state/...`)
- [ ] `comprehensive-reviewer.md` agent frontmatter has correct name, description, model
- [ ] SKILL.md: source_type detection logic covers github_issue, jira_issue, text
- [ ] SKILL.md: PR creation step includes gh pr create + PR number capture
- [ ] SKILL.md: Final Summary includes PR number in summary.md template
- [ ] SKILL.md: Post to Source correctly dispatches on source_type
- [ ] SKILL.md: `--auto` flag detection in Workspace Setup step 5b
- [ ] SKILL.md: `--nopr` flag detection in Workspace Setup step 5b-ii
- [ ] SKILL.md: Resume Check restores `{auto_approve}` from `resume_info.autoApprove`
- [ ] SKILL.md: Resume Check restores `{skip_pr}` from `resume_info.skipPr`
- [ ] SKILL.md: PR Creation has two-gate skip structure (task-type + --nopr)
- [ ] SKILL.md: Final Summary omits PR line when `{pr-number}` is `none`
- [ ] SKILL.md: Checkpoint A and B have two-gate skip structure (task-type + auto-approve)
- [ ] SKILL.md: Mandatory Calls section lists set-effort, phase-log, checkpoint
- [ ] SKILL.md: `full` template + `--auto` flag — `autoApprove` stays `false` when conflict prompt is accepted (orchestrator must NOT call `set-auto-approve` in this case)

---


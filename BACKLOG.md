
# Claude-Forge Plugin — Backlog

Known issues, improvement candidates, and future direction.

---

## Priority Queue

Ordered by priority. Higher rows should be tackled first.

| # | ID | Issue | Title | Type | Effort | Why now |
|---|-----|-------|-------|------|--------|---------|
| 1 | **P22** | [#9](https://github.com/hiromaily/claude-forge/issues/9) | ARCHITECTURE.md "What Each Agent Reads" table incomplete | Docs | XS | Final Summary row was missing — caused implementation deviation during F16. Keep table complete for all phases including orchestrator-driven ones. |
| 2 | **F15** | [#10](https://github.com/hiromaily/claude-forge/issues/10) | Inline revision shortcut for MINOR findings | Feature | S | When all review findings are MINOR, orchestrator edits artifacts directly + re-reviews, instead of re-spawning the full authoring agent. |
| 3 | **F5** | [#11](https://github.com/hiromaily/claude-forge/issues/11) | Diff-based review (token reduction) | Feature | M | 60-80% token reduction for review agents. Higher ROI on large codebases. |
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

- [ ] `state-manager.sh`: all commands work (init, phase-start, phase-complete, phase-fail, checkpoint, task-init, task-update, revision-bump, set-branch, set-task-type, skip-phase, set-auto-approve, set-skip-pr, phase-log, phase-stats, abandon, resume-info, get, set-effort, set-flow-template)
- [ ] `state-manager.sh`: PHASES array includes phase-7, pr-creation, post-to-source
- [ ] `state-manager.sh`: numeric fields (implRetries, reviewRetries) stay as numbers after task-update
- [ ] `state-manager.sh`: special characters in spec-name don't break JSON
- [ ] `state-manager.sh`: `set-auto-approve` sets `autoApprove: true` and updates `lastUpdated`
- [ ] `state-manager.sh`: `resume-info` projects `autoApprove` with `// false` default
- [ ] `state-manager.sh`: `set-skip-pr` sets `skipPr: true` and updates `lastUpdated`
- [ ] `state-manager.sh`: `resume-info` projects `skipPr` with `// false` default
- [ ] `state-manager.sh`: `set-effort <workspace> <value>` — XS/S/M/L accepted; invalid value rejected (exit 1)
- [ ] `state-manager.sh`: `set-flow-template <workspace> <value>` — all five templates accepted (direct/lite/light/standard/full); invalid value rejected (exit 1)
- [ ] `state-manager.sh`: `resume-info` — `effort` and `flowTemplate` fields present and null-safe (missing fields return null, not error)
- [ ] `state-manager.sh`: `phase-log` appends to `phaseLog` array with correct fields
- [ ] `state-manager.sh`: `phase-stats` outputs formatted table
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
- [ ] All phase IDs in SKILL.md exist in state-manager.sh PHASES array
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
- [ ] SKILL.md: Mandatory Calls section lists set-task-type, phase-log, checkpoint
- [ ] SKILL.md: `full` template + `--auto` flag — `autoApprove` stays `false` when conflict prompt is accepted (orchestrator must NOT call `set-auto-approve` in this case)

---

## Resolved

All items below are implemented and verified. One-line summaries for reference.

| ID | Title | Resolution |
|----|-------|------------|
| **P21** | SKILL.md size reduction / split | Removed Mermaid diagram (73 lines), compressed skip gate blockquotes (already terse in live file), removed flow template matrix (13 lines), consolidated Final Summary shared steps (3 lines). Net: 89 lines reduced (1,646 → 1,557). Remaining size reduction opportunities exist via stub file extraction if needed. |
| **P20** | Consolidated artifact availability table | Added a 20-row lookup table to ARCHITECTURE.md (§ Consolidated Artifact Availability) showing which workspace files are present for every `(task_type, effort)` cell. Replaces the need to cross-reference the flow template matrix, template base skip sets, and task-type supplemental skip sets manually. |
| **P19** | SKILL.md forward-reference fragility | Resolved via three-technique approach: inline HTML-comment anchors (`<!-- anchor: <token> -->`) on target headings, structural consolidation of the dispatch epilogue (duplicated steps 4–5 extracted into a shared Post-dispatch epilogue block), and step-reference rewrites replacing ordinal references with prose labels and anchor tokens. |
| **F16** | Improvement Report | Retrospective analysis of workspace artifacts for documentation gaps, code readability issues, and AI agent support needs. Always-on, appended to summary.md. |
| **F14** | Checkpoint summary in reviewer output | `## Orchestrator Summary` section added to reviewer agents; checkpoints read summary instead of full artifacts. |
| **F13** | Effort-aware pipeline flow | 2-axis `(task_type, effort)` with 5 flow templates (direct/lite/light/standard/full). 20-cell skip matrix, `analyst.md` for merged Phase 1+2. Subsumes F7. |
| **F4** | Task-type-aware pipeline flow | 5 task types with per-type phase skip tables, stub synthesis for docs/bugfix flows. |
| **F3** | Skip human checkpoints (`--auto`) | `autoApprove` field, two-gate skip at Checkpoint A/B, REVISE still requires human. |
| **F1** | Token consumption visibility | `phase-log`/`phase-stats` commands, Execution Stats in Final Summary. |
| **F7** | Merge Phase 1-2 for simple tasks | Subsumed by F13 — `lite` template implements merged Phase 1+2 via `analyst.md`. |
| **P18** | Test count hardcoded in docs | Removed all hardcoded counts, replaced with dynamic pointers to `bash scripts/test-hooks.sh`. |
| **P16+P17** | Block main checkout + verifier rewrite | Rule 5 in pre-tool-hook.sh blocks checkout to main/master; verifier rewritten to test on current branch only. |
| **P15** | Checkpoint-B approval skipped | Rule 3g blocks `task-init` without prior checkpoint-b completion; "STOP AND WAIT" markers in SKILL.md. |
| **P14** | Implementer creates wrong branch | Rule 4 hook guard blocks divergent `git checkout -b`; implementer prompt passes `{branch}` explicitly. |
| **P13** | Orchestrator artifact write undocumented | Documented Write tool constraint and workaround patterns in SKILL.md. |
| **P12** | Reviewer REVISE threshold too low | Added `APPROVE_WITH_NOTES` verdict with CRITICAL/MINOR severity classification. |
| **P11** | Architect agent writes files directly | Added "Do NOT write any files" rule to `architect.md`. |
| **P10** | Hook-based deterministic guards | Three sub-items: taskType null guard, phase-log missing guard, checkpoint guard — all implemented in pre-tool-hook.sh. |
| **P9** | Mandatory state-manager calls | "Mandatory Calls — Never Skip" section in SKILL.md + P10-3 hook guard for two-layer defense. |
| **P8** | Reviewer REVISE on non-critical findings | CRITICAL/MINOR severity classification in design-reviewer and task-reviewer agents. |
| **P7** | Agent roster incomplete for task types | Added "Task-type notes" column to Agent Roster table in `agents/README.md`. |
| **P6** | Bugfix REVISE loop conflict | Documented: bugfix skips Phase 3b, so REVISE loop never fires. |
| **P5** | Final-summary not task-type-aware | Per-task-type dispatch with 5 template variants + artifact guard hooks. |
| **P4** | Multiple simultaneous pipelines | `abandon` command; hooks skip abandoned pipelines. |
| **P3** | Plugin agent invocation unverified | Confirmed via official Claude Code docs; all agent frontmatter verified. |
| **P2** | No automated tests for hooks | Created `test-hooks.sh` with comprehensive test coverage. |
| **P1** | Hook stdin JSON field names wrong | Fixed to use `tool_name`, `tool_input`, `tool_response`. |

---

## Version History

### 1.0.0 (2026-03-20)

- Effort-aware pipeline flow: 2-axis `(task_type, effort)` with 5 flow templates — direct, lite, light, standard, full (F13)
- `--effort=XS|S|M|L` flag with Jira story points fallback and heuristic detection
- `set-effort`, `set-flow-template` commands in `state-manager.sh` with validation
- 20-cell canonical skip sequences table in SKILL.md (union rule: template base ∪ task-type supplemental)
- New `analyst` agent for merged Phase 1+2 in lite template
- Rule 3f non-blocking effort-null guard in `pre-tool-hook.sh`
- `full` template forces manual checkpoints even with `--auto`
- `direct` template stub synthesis (`analysis.md`, `design.md`, `tasks.md`)
- `--nopr` flag for skipping PR creation
- Resume handles pre-F13 pipelines with in-context defaults
- 128 automated tests (up from 100)
- Task-type-aware pipeline flow: 5 types (feature, bugfix, investigation, docs, refactor) with per-type phase skip tables (F4)
- Per-task-type Final Summary templates with dispatch logic (P5)
- `--auto` flag for autonomous checkpoint approval with two-gate skip structure (F3)
- Phase metrics: `phase-log`, `phase-stats` commands and Execution Stats in Final Summary (F1)
- CRITICAL/MINOR severity classification for design and task reviewers (P8)
- Artifact guard hooks preventing state advancement without required files
- Checkpoint guard hook blocking `phase-complete` without prior `$SM checkpoint` call (P10-3)
- "Mandatory Calls — Never Skip" section in SKILL.md for orchestrator compliance (P9)
- `abandon` command for clean pipeline termination (P4)
- 100 automated tests (up from 43)
- Initial implementation: 9 named agents, SKILL.md orchestrator, state management, hooks
- Agent extraction from inline prompts to dedicated .md files
- State manager with jq-based JSON operations and mkdir file locking
- Three hooks: PreToolUse (read-only + commit blocking), PostToolUse (output validation), Stop (completion guard)
- Resume logic: re-invoke skill with workspace path to pick up from state.json
- Review fixes: stop hook exit code, task-update numeric types, find_active_workspace sorting, git commit regex, arg validation, jq guard in all hooks, cmd_init injection safety

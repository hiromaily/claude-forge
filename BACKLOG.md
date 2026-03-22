
# Claude-Forge Plugin — Backlog

Known issues, improvement candidates, and future direction.

---

## Priority Queue

Ordered by priority. Higher rows should be tackled first.

| # | ID | Title | Type | Effort | Why now |
|---|-----|-------|------|--------|---------|
| 1 | **P22** | ARCHITECTURE.md "What Each Agent Reads" table incomplete | Docs | XS | Final Summary row was missing — caused implementation deviation during F16. Keep table complete for all phases including orchestrator-driven ones. |
| 2 | **F15** | Inline revision shortcut for MINOR findings | Feature | S | When all review findings are MINOR, orchestrator edits artifacts directly + re-reviews, instead of re-spawning the full authoring agent. |
| 3 | **F5** | Diff-based review (token reduction) | Feature | M | 60-80% token reduction for review agents. Higher ROI on large codebases. |
| 4 | **F10** | Partial execution (`--until`/`--from`) | Feature | M | `--until=design` for scoping only, `--from=phase-5` for re-implementation. Combines with `--auto` for autonomous scoping reports. |
| 5 | **F9** | Structured acceptance criteria | Feature | M | Improves PASS/FAIL consistency. Currently depends on impl-reviewer's subjective interpretation. |
| 6 | **F12** | Checkpoint diff preview | Feature | S | Nice-to-have. `--auto` reduces checkpoint frequency, lowering the priority. |
| 7 | **F8** | Past pipeline reference (data flywheel) | Feature | L | Uses `.specs/` history to improve future pipelines. The accumulated data is a moat — competitors can copy code but not execution history. |
| 8 | **F17** | Repository profiling | Feature | M | First-run analysis of repo structure, test strategy, CI config → persisted profile that tunes agent prompts. Hard to replicate without per-repo data. |
| 9 | **F18** | Improvement report → test case generation | Feature | S | Auto-generate hook guard test cases from friction points found in improvement reports. Accelerates deterministic guard accumulation. |
| 10 | **F19** | CI feedback loop (post-PR auto-fix) | Feature | L | After PR creation, monitor CI results and auto-trigger fix flow on failure. Closes the quality loop beyond the pipeline boundary. |
| 11 | **F6** | Adaptive model routing | Feature | L | Needs phase-stats data before deciding. F13 (effort axis) provides the foundation for model selection. |
| 12 | **F2** | Execution log (JSONL) | Feature | M | Basic coverage via phase-log. Full JSONL log deferred until the need is confirmed. |

**Effort key:** XS = < 30min, S = 1-2h, M = half day, L = 1+ day

**Prioritization criteria:**

1. **Blocking bug** — fix first
2. **Determinism** — hook guards to cover AI non-determinism
3. **Dev loop acceleration** — high ROI (F10)
4. **Competitive moat** — data flywheel and per-repo learning (F8, F17, F18)
5. **Cost reduction** — validate with phase-stats data (F5, F6)
6. **Future features** — after data accumulation (F12, F19)

---

## Feature Requests

### F2: Execution log output to workspace

**Want**: Write a structured execution log to `{workspace}/pipeline.log` as the pipeline runs.
**Why**: When a pipeline fails or produces unexpected results, there is no audit trail. The conversation may be compacted, and state.json only tracks phase status, not what happened within each phase.
**Ideas**:

- Append timestamped entries for: phase start/complete/fail, agent spawn/return, checkpoint decisions, errors
- Format: one JSON object per line (JSONL) for easy parsing
- Could be implemented via a PostToolUse hook that appends to the log, or directly by the orchestrator before/after each phase
- Include agent descriptions and key outputs (verdict, retry count) — but not full artifact content (too large)

### F5: Diff-based review (token reduction)

**Want**: Phase 6 (impl-reviewer) and Phase 7 (comprehensive-reviewer) receive only `git diff` output instead of reading full source files.
**Why**: Review agents currently read entire files to understand changes, consuming 5-10x more tokens than necessary. The reviewer only needs to see what changed, not the unchanged surrounding code.
**Ideas**:

- Pass `git diff main...HEAD -- <changed files>` output as context instead of having agents read files
- For impl-reviewer (per-task): `git diff HEAD~1` to see only that task's changes
- For comprehensive-reviewer: `git diff main...HEAD` for the full feature diff
- Include file-level context (function signatures, class definitions) via AST summary for surrounding context without full file reads
- Estimated token savings: 60-80% for review phases on large codebases

### F6: Adaptive model routing

**Want**: Dynamically select the model (haiku/sonnet/opus) per agent based on task complexity and token budget.
**Why**: All agents use sonnet, but some phases are simple enough for haiku (situation-analyst on small repos, verifier running commands) while others benefit from opus (architect on complex designs). Static model assignment wastes budget.
**Ideas**:

- situation-analyst, verifier → **haiku** by default (structural reads and command execution)
- implementer → haiku if change scope < 50 lines, sonnet otherwise
- architect, design-reviewer → opus if design.md > 200 lines or revision count > 0
- Add `tokenBudget` field to state.json; orchestrator checks remaining budget before each phase and downgrades model if budget is low
- Allow per-run override: `--model=opus` to force all agents to use a specific model
- Log actual model used per phase in token-log.json (ties into F1)

### F8: Past pipeline reference (data flywheel)

**Want**: Use accumulated `.specs/` history to improve future pipeline runs — design quality, review accuracy, and implementation speed all improve with usage.
**Why**: This is the primary competitive moat. Competitors can copy plugin code, but not the execution history accumulated per repository. A pipeline that gets smarter over time is fundamentally harder to replicate than one that starts from zero each run.
**Ideas**:

- At Phase 1, scan `.specs/*/request.md` for semantic similarity to the current request
- If a close match is found, include relevant `design.md` and `review-*.md` excerpts as additional context for the architect
- Store a lightweight index file (`.specs/index.json`) mapping spec-name → one-line description + tags for fast lookup
- Accumulate review finding patterns across runs → feed to reviewer agents as "common issues in this repo"
- Accumulate improvement report findings → build a per-repo "AI friction map" that gets passed to all agents
- Limit context injection to top 2-3 matches to avoid bloat
- Use file modification dates to prefer recent pipelines over old ones

### F17: Repository profiling

**Want**: On first pipeline run in a new repository, analyze repo structure, test strategy, CI configuration, and coding conventions. Persist the profile and use it to tune agent prompts in subsequent runs.
**Why**: Generic agent prompts waste tokens discovering things that are stable across runs (e.g., "this repo uses pytest", "CI runs on GitHub Actions", "imports are sorted with isort"). A persisted profile makes every subsequent run more efficient and accurate.
**Ideas**:

- Generate `.specs/repo-profile.json` on first run (or when stale) with: language, test framework, CI system, linter config, directory conventions, branch naming
- Pass relevant profile sections to each agent (e.g., implementer gets test framework + linter config, architect gets directory conventions)
- Update profile incrementally when improvement reports flag new conventions
- Skip profile generation on `direct` template (too small to benefit)

### F18: Improvement report → test case generation

**Want**: Automatically convert friction points from improvement reports into hook guard test cases or regression tests.
**Why**: The improvement report (F16) identifies pipeline failure modes, but currently these findings sit in `summary.md` as prose. Converting them to executable tests accelerates the deterministic guard accumulation that is claude-forge's core strength.
**Ideas**:

- After improvement report generation, parse friction points for patterns that map to hook guards (e.g., "agent wrote to wrong file" → artifact guard test, "agent skipped checkpoint" → checkpoint guard test)
- Generate test case stubs in a `suggested-tests/` directory for human review
- Track which improvement report findings have been converted to tests vs. which remain unaddressed
- Ties into F8: accumulated test cases from past runs form a regression safety net

### F19: CI feedback loop (post-PR auto-fix)

**Want**: After PR creation, monitor CI results and auto-trigger a fix flow on failure.
**Why**: Currently the pipeline ends at PR creation. If CI fails (lint, type check, integration tests), the user must manually intervene. Closing this loop makes the pipeline truly end-to-end.
**Ideas**:

- After `gh pr create`, poll CI status with `gh pr checks`
- On failure, extract the failing check output and feed it to a lightweight fix pipeline (`direct` template with CI error as context)
- Limit auto-fix to N attempts (e.g., 2) to avoid infinite loops
- Push fix commits to the same PR branch
- Log CI fix attempts in `state.json` for metrics

### F9: Structured acceptance criteria validation

**Want**: Formalize acceptance criteria in tasks.md as a machine-verifiable checklist. Have implementer report pass/fail per criterion, and impl-reviewer validate against the checklist.
**Why**: Current acceptance criteria are free-text prose. The impl-reviewer interprets them subjectively, leading to inconsistent PASS/FAIL decisions. Structured criteria make reviews faster, more precise, and reproducible.
**Ideas**:

- task-decomposer outputs acceptance criteria as a numbered checklist per task
- implementer includes a checklist in `impl-{N}.md` with PASS/FAIL per criterion
- impl-reviewer validates each AC against the actual code, reducing review to a verification task
- Task-reviewer (Phase 4b) validates that each AC is testable/observable as part of its review

### F10: Partial execution mode (`--until`, `--from`)

**Want**: Run only a subset of the pipeline, stopping at or starting from a specified phase.
**Why**: Users often want to scope a task (analysis + investigation + design only) without committing to implementation. Or they want to re-run just implementation after manually editing design.md. The full pipeline is all-or-nothing today.
**Ideas**:

- Parse `--until=<phase>` and `--from=<phase>` from `$ARGUMENTS`
- Store in state.json as `untilPhase` / `fromPhase`
- Orchestrator checks before each phase: if `currentPhase > untilPhase`, skip to Final Summary
- `--from` skips to the specified phase (validates that prerequisite artifacts exist)
- Common shortcuts: `--until=design` (scoping), `--from=phase-5` (re-implement), `--until=phase-4b` (get approved tasks without implementing)
- Combine with `--auto`: `--until=design --auto` = fully autonomous scoping report

### F12: Checkpoint diff preview

**Want**: At Checkpoint A and B, show not just the artifact text but also relevant diffs so the human can make better-informed decisions.
**Why**: At Checkpoint A, the user sees design.md text but has no visibility into what the AI based its design on. At Checkpoint B after revisions, the user can't easily see what changed from the previous version. After Phase 5-6, showing the code diff would help the user judge quality.
**Ideas**:

- Checkpoint A: show `investigation.md` key findings alongside design.md
- Checkpoint A (revision): show `diff` between previous and revised design.md
- Checkpoint B: show task count, estimated complexity, and which files will be touched
- After Phase 6: optionally show `git diff main...HEAD --stat` for change scope overview

### F15: Inline revision shortcut for MINOR findings

**Want**: When all review findings are MINOR (no CRITICAL), the orchestrator edits the artifact directly instead of re-spawning the authoring agent.
**Why**: During the F13 pipeline run, task-reviewer's Round 1 findings were all criterion-wording improvements — no structural changes. Re-spawning task-decomposer for this was unnecessary.
**Ideas**:

- Add a decision branch in SKILL.md after review: if verdict is APPROVE_WITH_NOTES and all findings are MINOR, orchestrator applies fixes inline with Edit tool, then re-runs review only (skip authoring agent)
- If verdict is REVISE with CRITICAL findings, re-spawn the authoring agent as today
- Track "inline revision" vs "full revision" in state.json for metrics
- Risk: orchestrator may make incorrect fixes without the authoring agent's full context — mitigated by the subsequent re-review catching errors

---

## Improvement Candidates

### Model selection per agent

**Current**: All 10 agents use `model: sonnet`.
**Improvement**: Use `opus` for agents that benefit from stronger reasoning:

- `architect` (complex design decisions)
- `design-reviewer` (finding subtle design flaws)
- `implementer` (complex code generation)

Leave `sonnet` for straightforward agents (situation-analyst, investigator, verifier).
**Trade-off**: Cost increase. A full pipeline with 3 opus agents + 7 sonnet is ~2x the cost of all-sonnet.

### Agent-level retry with context carry-forward

**Current**: When a phase fails, the orchestrator spawns a fresh agent with the previous output appended.
**Improvement**: Use Claude Code's `resume` parameter on the Agent tool to continue a failed agent with its full context preserved. This would give the retry agent access to its own reasoning, not just the output.
**Risk**: Resume may not be supported for plugin-defined agents. Needs testing.

### Parallel Phase 5-6 interleaving

**Current**: All Phase 5 tasks run, then all Phase 6 reviews run.
**Improvement**: As soon as a Phase 5 task completes, immediately spawn its Phase 6 review. This overlaps implementation and review, reducing total wall-clock time.
**Complexity**: State tracking becomes more complex — a task can be in (impl: completed, review: in_progress) while other tasks are still in (impl: in_progress).

### Workspace directory naming

**Current**: `.specs/{date}-{spec-name}/`
**Improvement**: Consider using `.claude-forge/` instead of `.specs/` to avoid naming collision with kiro specs.

### Hook-based progress notifications

**Current**: No external visibility into pipeline progress.
**Improvement**: Add a `SubagentStart`/`SubagentStop` hook that logs phase transitions to a progress file or sends notifications (Slack, etc.).

### State schema versioning

**Current**: `state.json` has `"version": 1` but no migration logic. New fields were added without bumping the version — `resume-info` provides defaults for missing fields.
**Improvement**: When the schema changes in a breaking way, add a migration function to state-manager.sh that upgrades old state files. Check version on every `read_state` call.

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

### 1.2.0 (2026-03-20)

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

### 1.1.0 (2026-03-20)

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

### 1.0.0 (2026-03-20)

- Initial implementation: 9 named agents, SKILL.md orchestrator, state management, hooks
- Agent extraction from inline prompts to dedicated .md files
- State manager with jq-based JSON operations and mkdir file locking
- Three hooks: PreToolUse (read-only + commit blocking), PostToolUse (output validation), Stop (completion guard)
- Resume logic: re-invoke skill with workspace path to pick up from state.json
- Review fixes: stop hook exit code, task-update numeric types, find_active_workspace sorting, git commit regex, arg validation, jq guard in all hooks, cmd_init injection safety

# Situation Analysis: P19 — SKILL.md Forward-Reference Fragility

## 1. Relevant Files and Directories

| File | Purpose |
|------|---------|
| `/Users/hiroki.yasui/work/hiromaily/claude-forge/skills/forge/SKILL.md` | The sole orchestrator instruction file — ~1,640 lines. Contains all forward-references in scope. |
| `/Users/hiroki.yasui/work/hiromaily/claude-forge/BACKLOG.md` | Defines the task (P19 entry, line 14) and describes the failure mode. |
| `/Users/hiroki.yasui/work/hiromaily/claude-forge/ARCHITECTURE.md` | Design rationale; not a primary target but may need updates if anchoring strategy is documented there. |

No hook scripts, state-manager, or agent files are directly affected by this task.

---

## 2. Key Interfaces, Types, and Data Flows Touched by the Task

The task is purely documentary — it affects the prose structure of SKILL.md, not any runtime behavior. The fragile forward-references fall into four categories:

### Category A — Named sub-section cross-references (highest fragility)
Three dispatch blocks (feature/refactor, bugfix/docs, investigation) each contain a prose instruction that points to two later sub-sections by their exact heading text:

- Line 1360: `execute the ### Debug Report (conditional — all task types) block below`
- Line 1361: `execute ### Improvement Report (all task types) block`
- Line 1403 and 1448: identical pattern repeated in the other two dispatch blocks

The referenced sections are `### Debug Report (conditional — all task types)` (line 1463) and `### Improvement Report (all task types)` (line 1536). If either heading is renamed or moved, all three dispatch blocks silently break.

- Line 1465: `proceed to the ### Improvement Report (all task types) block` (from inside Debug Report itself)
- Line 1532: `proceed to the ### Improvement Report (all task types) block` (at end of Debug Report)

### Category B — Numbered step forward/backward references
- Line 1054: `Proceed to step 6 below` — refers to step 6 inside the same Checkpoint B section (line 1083). This is a within-section forward reference; step renumbering breaks it.
- Line 1083: `runs after human approval at step 7 OR after auto-approve skip gate 2 above` — backward reference to step 7 (line 1078) and to the auto-approve skip gate above.
- Line 1262: `run steps 1-2 (stage, commit, push) but skip step 3-4` — references numbered steps within the PR Creation section (lines 1271–1302).
- Line 1474: `reuse the phase-stats output already captured in step 2 of the dispatch block` — refers to step 2 in each per-task-type dispatch block, which is always `$SM phase-stats`. This cross-references structure inside the three dispatch blocks.

### Category C — Section-name prose references with "below" / "above"
- Line 484: `After effort detection (see step 5f below)` — anchors to the `Effort detection chain (step 5f)` sub-header.
- Line 523: `(Prompt will fire — see combined prompt logic above)` — backward reference to the combined-prompt block starting at line 485.
- Line 569: `see "Mandatory Calls" section` — refers to the `## Mandatory Calls — Never Skip` section heading (line 685).
- Line 831: `stub synthesis was done in Workspace Setup step 7` — refers to step 7 of the Workspace Setup section.
- Line 1316: `select exactly one block below` — generic forward reference spanning ~140 lines.

### Category D — Sequential flow prose using phase names
Most phase blocks use `continue to <Phase Name>` or `proceed to the next phase block` — these are less fragile because they use stable phase names rather than section numbers. Examples: lines 936, 937, 967, 1031, 1032.

---

## 3. Existing Tests for Affected Code

There are no automated tests for SKILL.md prose structure. The testing checklist in BACKLOG.md (lines 222–270) contains a manual checklist for SKILL.md, but the relevant items are behavioral guards, not forward-reference correctness:

- Line 254: `SKILL.md Agent Roster matches each agent's actual Input section`
- Line 261–269: various SKILL.md behavioral checks (PR creation, Final Summary, flag detection)

None of the checklist items verify that prose cross-references point to extant, correctly-named sections.

The test file `/Users/hiroki.yasui/work/hiromaily/claude-forge/scripts/test-hooks.sh` covers only the shell hook scripts — it has no coverage of SKILL.md prose.

---

## 4. Known Constraints and Technical Debt

**Constraint 1 — LLM interpretation of anchors.** SKILL.md is consumed by an LLM orchestrator, not a Markdown renderer. Any anchor/label scheme must be interpretable as plain text by an LLM. HTML anchors (`<a id="...">`) would be invisible in rendered Markdown but visible in raw text; heading text is the only reliably LLM-visible identifier.

**Constraint 2 — File size.** SKILL.md is ~1,640 lines. P21 in the backlog (line 19) flags this size as a known problem; splitting is deferred but intersects with any anchoring strategy. Changes to forward-reference structure should not further increase overall line count.

**Constraint 3 — No heading ID standard defined.** The document has no existing convention for stable labels or anchors. Any new scheme must be introduced consistently or the problem recurs.

**Constraint 4 — Three near-identical dispatch blocks.** The `feature/refactor`, `bugfix/docs`, and `investigation` Final Summary blocks (lines 1320–1461) duplicate the same step 4 instruction three times (lines 1360, 1403, 1448). The forward-reference fragility in Category A is tripled by this duplication.

**Constraint 5 — Numbered steps are structural, not labeled.** Checkpoint B (lines 1062–1089) uses numbered steps 1–8. "Step 6" and "step 7" are referenced by number without a named label, so any insertion or deletion of steps silently breaks the cross-references on lines 1054 and 1083.

**Constraint 6 — BACKLOG.md cites one specific example.** The backlog entry ("proceed to commit-amend step") is illustrative but that exact phrase does not appear in the current SKILL.md. The most likely candidate is the `commit --amend` block at lines 1363–1368, which is referenced indirectly through "step 7" in the dispatch blocks. The actual fragile locations are the Category A and B references documented above.

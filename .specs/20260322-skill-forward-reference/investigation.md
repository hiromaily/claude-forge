# Investigation Report: P19 — SKILL.md Forward-Reference Fragility

---

## 1. Root Cause

The forward-reference fragility stems from a single architectural choice: SKILL.md uses prose descriptions of target locations (heading text, step numbers, directional words like "above" / "below") rather than stable, immutable labels. Because the LLM orchestrator interprets the file as plain text, there is no renderer or linker to catch broken references at load time. All failures are silent — the orchestrator simply follows wrong or absent instructions with no error signal.

There are four active mechanisms that produce fragility:

1. **Heading-text coupling** — three dispatch blocks each embed the exact heading string of two downstream sections. Any rename of those headings breaks all three callers simultaneously.
2. **Step-number coupling** — Checkpoint B's human-approval path references "step 6" and "step 7" by ordinal position inside the same numbered list. Any insertion or reordering of steps invalidates those references.
3. **Section-relative coupling** — "above" / "below" references in Workspace Setup steps assume no content is inserted between the referencing sentence and its target.
4. **Cross-block step coupling** — the Debug Report section assumes "step 2 of the dispatch block" is always `$SM phase-stats`. This is true today across all three dispatch blocks, but a future edit to any one block could break this assumption.

---

## 2. Integration Points Affected

All forward references reside entirely within `/Users/hiroki.yasui/work/hiromaily/claude-forge/skills/forge/SKILL.md`. No other file in the repository imports from or parses SKILL.md at runtime. The hooks read `state.json`, not SKILL.md. The state manager is a pure CLI. Agents receive SKILL.md content only when the LLM loads the skill.

The only consumers of forward references are:
- The LLM orchestrator instance that executes the skill
- Human developers reading SKILL.md when modifying the file

---

## 3. Precise Inventory of All Forward References

### Category A — Heading-text forward references (highest breakage risk)

These six lines embed the exact heading text of two downstream sections. Because three dispatch blocks duplicate the identical instruction, any rename of either heading requires editing six lines.

| Line | Text | Target heading |
|------|------|----------------|
| 1360 | `execute the ### Debug Report (conditional — all task types) block below` | Line 1463 `### Debug Report (conditional — all task types)` |
| 1360 | `execute ### Improvement Report (all task types) block below` | Line 1536 `### Improvement Report (all task types)` |
| 1361 | `execute ### Improvement Report (all task types) block` | Line 1536 |
| 1403 | (identical to 1360 — bugfix/docs dispatch block) | Lines 1463 and 1536 |
| 1404 | (identical to 1361) | Line 1536 |
| 1448 | (identical to 1360 — investigation dispatch block) | Lines 1463 and 1536 |
| 1449 | (identical to 1361) | Line 1536 |
| 1465 | `proceed to the ### Improvement Report (all task types) block` | Line 1536 |
| 1532 | `proceed to the ### Improvement Report (all task types) block` | Line 1536 |

Both target headings are referenced a combined nine times across six sites. A rename of either heading breaks all nine references without any compile-time or load-time signal.

The Debug Report heading includes the parenthetical `(conditional — all task types)`, which is both long and easily mistyped in future edits.

### Category B — Step-number forward/backward references

| Line | Text | Actual target |
|------|------|---------------|
| 1054 | `Proceed to step 6 below` (inside auto-approve skip gate 2) | Line 1077: step 6 = "If the user requests changes…" |
| 1083 | `runs after human approval at step 7 OR after auto-approve skip gate 2 above` | Line 1078: step 7 = "Once the user approves, call…"; skip gate 2 = lines 1040–1054 |
| 1262 | `run steps 1-2 (stage, commit, push) but skip step 3-4 (PR creation)` | Lines 1271–1302: steps 1 (stage/commit), 2 (push), 3 (gh pr create), 4 (capture PR number) |
| 1474 | `reuse the phase-stats output already captured in step 2 of the dispatch block` | Step 2 of all three dispatch blocks (lines 1323, 1377, 1420) = `$SM phase-stats` |

The Checkpoint B numbered list (lines 1062–1089) is the highest-risk site: it contains 8 ordered steps, and lines 1054 and 1083 both reference steps by number. If any step is inserted before step 6, or if steps are reordered, both references silently mispoint.

Line 1474 cross-references structure inside the three separately-maintained dispatch blocks. If any dispatch block is restructured to move `phase-stats` to a different step position, the Debug Report section breaks silently.

### Category C — Prose relative references

| Line | Text | Target |
|------|------|--------|
| 484 | `After effort detection (see step 5f below)` | Sub-header `Effort detection chain (step 5f)` at line 501 |
| 523 | `(Prompt will fire — see combined prompt logic above.)` | Lines 484–498: "After effort detection…" block |
| 569 | `see "Mandatory Calls" section` | `## Mandatory Calls — Never Skip` at line 685 |
| 831 | `stub synthesis was done in Workspace Setup step 7` | Step 7 of Workspace Setup section at line 569 |
| 1316 | `select exactly one block below` | Three dispatch blocks spanning lines 1320–1461 |

These references are less likely to break than Category A and B because they use stable sub-header labels (`step 5f` is embedded in the heading itself) or section names that are unlikely to be renamed. The exception is line 831's reference to "Workspace Setup step 7" — if the step numbering in Workspace Setup changes, this prose note silently becomes incorrect.

### Category D — Phase-name sequential flow references (low fragility)

Lines 936, 937, 967, 1031, 1032, and others use `continue to Checkpoint A`, `proceed to Phase 3b`, etc. These reference stable phase names rather than step numbers or exact heading text. They are low risk and do not require remediation.

---

## 4. Edge Cases and Risks

### Risk 1 — Silent failure mode
No automated detection exists for broken forward references in SKILL.md. The test suite (`scripts/test-hooks.sh`) covers only the shell hook scripts. There is no prose linting, Markdown link checker, or heading-existence validator that would catch a broken Category A reference. A renamed heading would cause the orchestrator to silently skip the Debug Report or Improvement Report with no error output.

### Risk 2 — Tripled Category A breakage surface
The three dispatch blocks (`feature/refactor`, `bugfix/docs`, `investigation`) contain identical Category A references. When a change is made to one block, it is easy for a developer to update that block and forget to update the other two. The analysis identified this as Constraint 4 — the tripling is structural, not accidental.

### Risk 3 — Checkpoint B step-number brittleness
Checkpoint B's human-approval path is the most active area of SKILL.md (every non-auto-approve run passes through it). The step count is 8 today. If F15 (inline revision shortcut) is implemented, it will likely modify this section — adding, removing, or renaming steps — directly threatening lines 1054 and 1083.

### Risk 4 — Debug Report's cross-dispatch assumption
Line 1474 assumes "step 2 of the dispatch block" is always `$SM phase-stats`. This is true in all three dispatch blocks today (lines 1323, 1377, 1420). However, the Debug Report section is not adjacent to the dispatch blocks — it is separated by approximately 60 lines. A future edit that reorders steps in one dispatch block (e.g., moving `phase-stats` to step 1) would break line 1474 in a way that is not obvious when reading the dispatch block in isolation.

### Risk 5 — Heading verbosity invites typos
The heading `### Debug Report (conditional — all task types)` is 43 characters. Nine prose references copy this string. If a future developer abbreviates the heading to `### Debug Report` (which is cleaner), all nine references break.

### Risk 6 — No stable-label convention exists
The codebase has no existing convention for stable, LLM-interpretable labels (e.g., `<!-- @anchor: debug-report -->`). Any anchoring scheme introduced for this task must define a new convention from scratch. If the convention is not applied consistently to all new sections added in future work, the problem recurs incrementally.

---

## 5. External Dependencies and API Contracts

No external contracts are affected. SKILL.md is a self-contained LLM instruction document. The forward references are internal prose only; no hook script, state manager command, or agent `.md` file parses or imports any section of SKILL.md by heading name or step number.

The only coupling to other files is indirect: SKILL.md section names like "Mandatory Calls" and phase names like "Phase 3b" appear in `ARCHITECTURE.md`, `BACKLOG.md`, and `agents/README.md` as human-readable references. These are documentation prose, not code references, and would need to be updated if section headings in SKILL.md are significantly renamed — but they are not brittle forward references in the sense of P19.

---

## 6. Prior Art — Similar Patterns in the Codebase

No other file in the repository uses a comparable prose cross-reference scheme that would serve as a model for the fix. The closest analogies are:

- **Phase ID strings** (e.g., `phase-3b`, `checkpoint-a`) — these are stable identifiers used across SKILL.md, state-manager.sh, and the hook scripts. They survive because they are machine-parsed constants, not prose descriptions.
- **Task-type strings** (e.g., `bugfix`, `investigation`) — same pattern: stable constants used as dispatch keys.
- **Agent names** (e.g., `situation-analyst`, `investigator`) — stable identifiers in the Agent Roster table and in SKILL.md's phase blocks.

The pattern that makes these stable is their use as machine-checked constants rather than human-composed prose. Any labeling scheme adopted for P19 could borrow this principle — short, lowercase, hyphenated stable tokens that are easier to search for and harder to accidentally rename.

---

## 7. Deletion/Rename Impact Search

No deletions or renames are proposed by P19 — it is a refactoring of prose cross-reference style within SKILL.md only. If the remediation work renames either of the two target headings (`### Debug Report (conditional — all task types)` or `### Improvement Report (all task types)`), the full impact is:

**`### Debug Report (conditional — all task types)`** — referenced at:
- Lines 1360, 1403, 1448 (three dispatch blocks, step 4)
- Line 1465 (inside Debug Report's own skip-guard prose)

**`### Improvement Report (all task types)`** — referenced at:
- Lines 1360, 1361, 1403, 1404, 1448, 1449 (three dispatch blocks, steps 4 and 5)
- Line 1465 (inside Debug Report's skip-guard prose)
- Line 1532 (inside Debug Report's step 4)

All of these are within SKILL.md. No external file references these heading names.

---

## 8. Ambiguities Requiring a Human Decision

### Ambiguity 1 — Scope: fix only Category A and B, or all four categories?

Category A (heading-text) and Category B (step-number) references are the most fragile and the most likely to break silently. Category C references ("above" / "below" with sub-header labels) are lower risk. Category D (phase-name sequential flow) requires no action. The designer must decide whether to address all four categories or restrict the fix to A and B only, given the effort constraint (P19 is rated S = 1-2 hours).

### Ambiguity 2 — What is the stable-label scheme for an LLM audience?

The situation analysis identified Constraint 1: SKILL.md is consumed by an LLM, not a Markdown renderer, so HTML anchors (`<a id="...">`) are invisible to the renderer but visible in raw text. Options include:

- **Inline label tags** in headings: e.g., `### Debug Report <!-- @debug-report -->` — visible in raw text, searchable, but requires a new convention.
- **Short alias names** embedded in heading text: e.g., `### [debug-report] Debug Report (conditional)` — immediately LLM-visible but changes the displayed heading text.
- **Prose rewrite** — replace heading-name references with unique prose keys: e.g., `execute the "debug-report" block` where the heading begins with `<!-- debug-report -->`.
- **Structural consolidation** — instead of three dispatch blocks each referencing two downstream sections, move the epilogue execution instruction out of the dispatch blocks into shared prose after the dispatch closes, eliminating the duplication entirely.

The designer must choose which of these (or another approach) is appropriate given the LLM-as-consumer constraint and the no-line-count-increase constraint (Constraint 2).

### Ambiguity 3 — Should Checkpoint B step numbering be replaced with named steps?

Checkpoint B uses a numbered list (steps 1–8) and references steps 6 and 7 by number. An alternative is to give each step a named anchor instead of a position. This would resolve the Category B risk at Checkpoint B, but it also changes the readability of the section. The designer must decide whether to denumber the steps (replacing ordinals with named anchors) or to keep numbers and instead add a cross-reference note that makes the coupling explicit and visible.

### Ambiguity 4 — Should the three dispatch blocks be partially merged?

The three dispatch blocks share steps 4 and 5 verbatim (the debug-epilogue and improvement-report execution instructions). Merging these shared steps into shared prose after the dispatch would eliminate the triple-duplication of Category A references. However, it changes the dispatch-block structure from "each block is self-contained" to "each block plus shared epilogue." The designer must decide whether structural consolidation is in scope for P19 or should be deferred (perhaps to P21, which targets SKILL.md size reduction).

### Ambiguity 5 — Does line 1361 (step 5 "Run improvement report") represent duplication or intent?

Each dispatch block contains both a step 4 ("Run debug epilogue — …then always execute the Improvement Report block") and a separate step 5 ("Run improvement report — execute Improvement Report block"). Step 5 is redundant: the `then always execute` clause in step 4 already mandates it, and line 1465 inside the Debug Report section also issues the same instruction. It is unclear whether step 5 is intentional redundancy for emphasis or accidental duplication. The designer should decide whether to retain or remove the redundant step — removing it reduces the surface area for Category A references.

---

## Open Questions

1. **Scope boundary** — Should P19 address only Category A and B (highest risk), or all categories including C? Given the S-effort estimate, full coverage of all four categories may exceed budget.

2. **Label scheme selection** — Which stable-label mechanism works for an LLM that reads raw Markdown? This is the core design decision and has no prior art in this codebase to draw from.

3. **Structural consolidation** — Is merging the shared epilogue steps (Category A triplication) in scope, or is it a separate P21 task? The two tasks are related but have different risk profiles.

4. **Step 5 redundancy** — Should lines 1361, 1404, and 1449 (the redundant "Run improvement report" step) be removed as part of this fix? Doing so reduces Category A reference count from nine to six.

5. **BACKLOG.md update** — Once P19 is resolved, the backlog entry must be moved from the Priority Queue to the Resolved section, and ARCHITECTURE.md may need a note documenting the stable-label convention. Is ARCHITECTURE.md update in scope for this task?

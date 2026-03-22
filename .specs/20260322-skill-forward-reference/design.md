# Design Document: P19 — SKILL.md Forward-Reference Fragility (Revised v2)

## Summary of Revisions

Addresses all four MINOR findings from AI review:
1. Dispatch-block step renumbering scan performed — no internal step-number back-references found in remaining steps; explicitly documented.
2. `---` separator specified before new `### Post-dispatch epilogue` block.
3. ARCHITECTURE.md section heading verified — `## Key Technical Decisions` (line 630).
4. Test item 5 updated with explicit BACKLOG.md line numbers (254, 261–269).

---

## 1. Chosen Approach and Rationale

Three techniques combined:

**Technique 1 — Inline HTML comment anchors on target headings.**
```
### Debug Report (conditional — all task types) <!-- anchor: debug-report -->
### Improvement Report (all task types) <!-- anchor: improvement-report -->
```
Visible in raw text (to LLM), invisible to Markdown renderers, short, grep-searchable. All prose references rewritten to use anchor tokens.

**Technique 2 — Structural consolidation: narrow post-dispatch epilogue.**
Extract the identical steps 4 and 5 (debug-report + improvement-report invocation) from all three dispatch blocks into one shared block. Only the invocations move — "Present summary.md" and commit-amend remain per-dispatch-block.

**Technique 3 — Purpose-clarifying language at headings and call site.**
- Debug Report: *forge skill operation* — pipeline metrics, token outliers, retries, revision cycles.
- Improvement Report: *target-repository friction* — doc gaps, readability, conventions that would have helped the task.

---

## 2. Architectural Changes

### `skills/forge/SKILL.md`

**Change 1 — Anchors + purpose descriptions on two target headings.**

```markdown
### Debug Report (conditional — all task types) <!-- anchor: debug-report -->

_Reports on the **operation of the forge skill itself**: pipeline execution flow, phase
metrics, token outliers, retry counts, and revision cycles. Triggered only when
`{debug_mode}` is `true`._
```

```markdown
### Improvement Report (all task types) <!-- anchor: improvement-report -->

_Reports on friction in the **target repository** — documentation gaps, code readability
issues, or conventions — that would have helped complete the assigned task. Always runs._
```

**Change 2 — Debug Report self-reference (line 1465).**
Before: `"proceed to the ### Improvement Report (all task types) block."`
After: `"proceed to the improvement-report block."`

**Change 3 — Debug Report epilogue instruction (line 1532).**
Before: `"proceed to the ### Improvement Report (all task types) block."`
After: `"proceed to the improvement-report block."`

**Change 4 — Collapse steps 4 and 5 in all three dispatch blocks.**

Step-renumbering scan result (verified during design): remaining steps after removal have no internal step-number back-references. Renumbering is clean.

`feature/refactor` and `bugfix/docs` blocks after removal:
1. Read review files.
2. Run `$SM phase-stats` and capture output.
3. Write `{workspace}/summary.md`.
4. Present the contents of `summary.md` to the user.
5. Update the commit (git add / commit --amend / push --force-with-lease).

`investigation` block after removal:
1. Read analysis.md and investigation.md.
2. Run `$SM phase-stats` and capture output.
3. Write `{workspace}/summary.md`.
4. Present the contents of `summary.md` to the user.
5. Do NOT run commit-amend or push.

**Change 5 — Add shared Post-dispatch epilogue block.**

Placement: after the "If none of the above" error block, before `### Debug Report`. Preceded by a `---` separator (consistent with existing convention between Final Summary subsections).

```markdown
---

### Post-dispatch epilogue <!-- anchor: final-summary-epilogue -->

Runs for all task types after the per-type dispatch block above completes.

1. **Run debug-report** (reports on forge skill operation — pipeline metrics, anomalies,
   token outliers, revision cycles): if `{debug_mode}` is `true`, execute the
   debug-report block below; otherwise skip it.

2. **Run improvement-report** (reports on target-repository friction — documentation
   gaps, code readability, conventions — that would have helped complete the task):
   always execute the improvement-report block below.
```

**Change 6 — Fix Category B: Checkpoint B step references.**
- "Proceed to step 6 below." → "Proceed to the change-request step below."
- Add label to step 6: "6. **Change-request step** — If the user requests changes: …"
- "runs after human approval at step 7 OR after auto-approve skip gate 2 above" → "runs after human approval OR after the auto-approve path above"

**Change 7 — Fix Category B: PR Creation skip-gate.**
Before: `"run steps 1-2 (stage, commit, push) but skip step 3-4 (PR creation)"`
After: `"run the stage-commit step and the push step, but skip the gh-pr-create and capture-PR-number steps"`

**Change 8 — Fix Category B: Debug Report cross-dispatch reference.**
Before: `"step 2 of the dispatch block"`
After: `"the dispatch block above"`

**Change 9 — Fix Category C: Workspace Setup step reference.**
Before: `"Workspace Setup step 7"`
After: `"the Initialize-state step of Workspace Setup"`
(Verified: step 7 of Workspace Setup carries the label "**Initialize state**".)

---

### `ARCHITECTURE.md`

**Change 10 — Stable anchor convention paragraph.**

Append after the final subsection under `## Key Technical Decisions` (line 630, confirmed):

```markdown
### Why inline comment anchors for SKILL.md cross-references?

SKILL.md is consumed by an LLM reading raw Markdown, not a renderer. HTML anchors
(`<a id="...">`) would be invisible in rendered view but visible to the LLM in raw text.
The chosen convention appends `<!-- anchor: <token> -->` to target headings and uses the
token (not the heading text) in all prose references. Tokens are short, lowercase,
hyphenated, and can be searched with `grep anchor:`. This is the stable-label convention
for SKILL.md. When adding new cross-referenced sections, follow this pattern.
```

---

### `BACKLOG.md`

**Change 11 — Move P19 to Resolved; note P21 intersection.**

---

## 3. Data Model or Type Changes

None. No state.json fields, hook scripts, or agent files affected.

---

## 4. Test Strategy

1. **Post-edit anchor grep** — `grep -n "### Debug Report\|### Improvement Report" skills/forge/SKILL.md` — exactly two heading lines with anchors expected.
2. **Step-number grep** — search for "step 6", "step 7", "steps 1-2", "steps 3-4" in Checkpoint B and PR Creation contexts. Expected: zero ordinal-only results.
3. **Existing test suite** — `bash scripts/test-hooks.sh` — all tests pass.
4. **Manual readthrough** — post-dispatch epilogue → Debug Report → Improvement Report. Verify: (a) anchor tokens resolve; (b) purpose descriptions distinct; (c) commit-amend and present-summary remain per-type; (d) `---` separator before `### Post-dispatch epilogue`.
5. **BACKLOG.md checklist** — verify BACKLOG.md lines 254 and 261–269 (SKILL.md behavioral invariants: Agent Roster, flag detection, PR creation, Final Summary, Checkpoint A/B, Mandatory Calls).

---

## 5. Risk Mitigation

| Risk | Mitigation |
|------|-----------|
| Silent failure mode | Post-edit grep checks |
| Tripled Category A breakage surface | Structural consolidation → two anchor tokens in one block |
| Checkpoint B step-number brittleness | Named prose labels replace ordinals |
| Debug Report cross-dispatch assumption | Replaced with positional description |
| Heading verbosity invites typos | Heading text removed from all cross-references |
| No stable-label convention | Documented in ARCHITECTURE.md `## Key Technical Decisions` |
| Debug/Improvement Report purpose ambiguity | Purpose descriptions at headings and post-dispatch call site |
| Epilogue visual separation | `---` separator enforces existing subsection convention |

---

## Files Modified

- `skills/forge/SKILL.md` — 9 prose edits + structural consolidation + purpose descriptions
- `ARCHITECTURE.md` — anchor convention paragraph under `## Key Technical Decisions`
- `BACKLOG.md` — P19 resolved; P21 intersection noted

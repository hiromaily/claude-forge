## Task 1: Add anchor tokens and purpose descriptions to Debug Report and Improvement Report headings [sequential]

**Design ref:** Change 1
**Depends on:** None
**Files:** `skills/forge/SKILL.md`
**Acceptance criteria:**
- [ ] `### Debug Report` heading ends with `<!-- anchor: debug-report -->` and is followed by purpose description ("operation of the forge skill itself")
- [ ] `### Improvement Report` heading ends with `<!-- anchor: improvement-report -->` and is followed by purpose description ("friction in the target repository")
- [ ] `grep -n "### Debug Report\|### Improvement Report" skills/forge/SKILL.md` returns exactly two lines, both containing their anchor comment

---

## Task 2: Fix Debug Report internal self-references [sequential]

**Design ref:** Changes 2 and 3
**Depends on:** Task 1
**Files:** `skills/forge/SKILL.md`
**Acceptance criteria:**
- [ ] Skip-guard self-reference reads `"proceed to the improvement-report block."` (not heading text)
- [ ] Debug Report epilogue instruction reads `"proceed to the improvement-report block."` (not heading text)
- [ ] No occurrence of `### Improvement Report (all task types)` remains anywhere in the file

---

## Task 3: Collapse steps 4 and 5 from all three dispatch blocks and add shared Post-dispatch epilogue [sequential]

**Design ref:** Changes 4 and 5
**Depends on:** Task 2
**Files:** `skills/forge/SKILL.md`
**Acceptance criteria:**
- [ ] All three dispatch blocks have old steps 4 (Run debug epilogue) and 5 (Run improvement report) removed; remaining steps renumbered correctly ending with present-summary and commit-amend/skip as steps 4 and 5
- [ ] `### Post-dispatch epilogue <!-- anchor: final-summary-epilogue -->` block exists after "If none of the above" error block and before `### Debug Report`, preceded by `---`, with two-step epilogue using anchor-token references
- [ ] No heading-text reference to `### Debug Report (conditional…)` or `### Improvement Report (all task types)` appears as prose in the three dispatch blocks

---

## Task 4: Fix Category B — Checkpoint B step-number references [sequential]

**Design ref:** Change 6
**Depends on:** Task 3
**Files:** `skills/forge/SKILL.md`
**Acceptance criteria:**
- [ ] Auto-approve skip gate: `"Proceed to step 6 below."` → `"Proceed to the change-request step below."`
- [ ] Step 6 of Checkpoint B begins with `**Change-request step**`
- [ ] Populate-task-state note no longer references ordinal "step 7" — reads `"runs after human approval OR after the auto-approve path above"`

---

## Task 5: Fix Category B — PR Creation skip-gate step reference [sequential]

**Design ref:** Change 7
**Depends on:** Task 3
**Files:** `skills/forge/SKILL.md`
**Acceptance criteria:**
- [ ] `--nopr` skip gate no longer reads `"run steps 1-2 … skip step 3-4"`
- [ ] Replacement names steps by prose labels: `"run the stage-commit step and the push step, but skip the gh-pr-create and capture-PR-number steps"`

---

## Task 6: Fix Category B — Debug Report cross-dispatch step reference [sequential]

**Design ref:** Change 8
**Depends on:** Task 3
**Files:** `skills/forge/SKILL.md`
**Acceptance criteria:**
- [ ] Debug Report no longer contains `"step 2 of the dispatch block"` — replaced with positional prose
- [ ] Surrounding meaning preserved: reuse the phase-stats output captured during the dispatch block

---

## Task 7: Fix Category C — Workspace Setup step 7 reference [sequential]

**Design ref:** Change 9
**Depends on:** Task 3
**Files:** `skills/forge/SKILL.md`
**Acceptance criteria:**
- [ ] `"Workspace Setup step 7"` → `"the Initialize-state step of Workspace Setup"`
- [ ] Step 7 of Workspace Setup still carries its `**Initialize state**` label

---

## Task 8: Append anchor convention paragraph to ARCHITECTURE.md [parallel]

**Design ref:** Change 10
**Depends on:** None
**Files:** `ARCHITECTURE.md`
**Acceptance criteria:**
- [ ] New subsection `### Why inline comment anchors for SKILL.md cross-references?` added as last subsection under `## Key Technical Decisions`
- [ ] Body includes: the `<!-- anchor: <token> -->` convention, LLM-reads-raw-Markdown rationale, instruction for future authors
- [ ] `grep "anchor:" ARCHITECTURE.md` returns at least one result

---

## Task 9: Update BACKLOG.md — resolve P19 and note P21 intersection [parallel]

**Design ref:** Change 11
**Depends on:** None
**Files:** `BACKLOG.md`
**Acceptance criteria:**
- [ ] P19 removed from Priority Queue; moved to Resolved section with resolution summary
- [ ] P19 Resolved entry notes the P21 structural consolidation intersection

---

## Task 10: Verify test suite and run post-edit acceptance checks [sequential]

**Design ref:** Test Strategy items 1–5
**Depends on:** Tasks 1, 2, 3, 4, 5, 6, 7, 8, 9
**Files:** (read-only verification)
**Acceptance criteria:**
- [ ] `bash scripts/test-hooks.sh` exits 0
- [ ] `grep -n "anchor:" skills/forge/SKILL.md` shows entries for `debug-report`, `improvement-report`, `final-summary-epilogue`
- [ ] Search for `"step 2 of the dispatch"`, `"step 6"` (in Checkpoint B context), `"step 7"` (in Checkpoint B context), `"steps 1-2"`, `"steps 3-4"`, `"Workspace Setup step 7"`, `"### Debug Report (conditional"`, `"### Improvement Report (all task types)"` returns zero matches

# Pipeline Summary

**Request:** [Architecture] Declarative pipeline phase registry to reduce change scatter (Issue #125)
**Feature branch:** `forge/declarative-phase-registry`
**Pull Request:** #126 (https://github.com/hiromaily/claude-forge/pull/126)
**Date:** 20260404

## Tasks

| # | Title | Verdict |
|---|-------|---------|
| 1 | Remove duplicate template constants from `flow_templates.go` | PASS |
| 2 | Introduce `PhaseDescriptor` type and `phaseRegistry` slice in new `registry.go` | PASS |
| 3 | Implement `initRegistry()` and wire `init()` — compute all derived vars | PASS |
| 4 | Update `phases_test.go` — replace magic count, add consistency and skip-table tests | PASS |
| 5 | Update `BACKLOG.md` to document deferred scatter points | PASS |

## What Changed

Introduced a declarative phase registry in `mcp-server/internal/orchestrator/registry.go`:

- **New `PhaseDescriptor` struct** — captures `ID`, `Skippable`, `Label`, and `TemplateSkips` for each phase
- **`phaseRegistry []PhaseDescriptor`** — 18-entry ordered slice, single source of truth
- **`initRegistry()` called from `init()`** — populates `AllPhases`, `nonSkippable`, `allPhasesSet`, `skipTable`, and `phaseLabels` at package load time; panics with a descriptive message if the registry diverges from `state.ValidPhases`
- **Template constant deduplication** — `flow_templates.go` local `TemplateLight/Standard/Full` string literals replaced with aliases to `state.Template*`
- **New tests**: `TestPhaseRegistryConsistency`, `TestSkipTableDerivedFromRegistry`, `TestAllPhasesCount` now uses `len(phaseRegistry)` instead of magic `18`
- **`BACKLOG.md`** updated with deferred scatter points (`validation/artifact.go:artifactRules`, `tools/guards.go`)

Adding a new phase now requires edits in only 2 places (`state/constants.go` + `orchestrator/registry.go`) instead of 6+.

Also fixed a pre-existing test failure in `internal/profile` (`TestAnalyzeOrUpdate_this_repo`): subdir `go.mod` detection now precedes `package.json` fallback, and a root `Makefile` `test:` target was added.

## Comprehensive Review

**Verdict:** CLEAN — no issues found. Two minor observations noted (both by-design): `TestPhaseRegistryLength` retains a `const wantCount = 18` (redundant but correct), and `skipTable` ordering relies on phaseRegistry iteration order (correct and tested).

## Test Results

All 13 packages: PASS (`go test -race ./...`), 0 linter issues.

## Pipeline Statistics

- Total tokens: 670,421
- Total duration: 1,824,592 ms (~30.4 min)
- Estimated cost: $4.02
- Phases executed: 14
- Phases skipped: 2 (phase-4b, checkpoint-b)
- Retries: 0
- Review findings: 0 critical, 6 minor

## Improvement Report

_Retrospective on what would have made this work easier._

### Documentation

The import-cycle constraint between `state` and `orchestrator` is enforced by a test (`orchestrator/import_cycle_test.go`) but is not documented in `CLAUDE.md` or `ARCHITECTURE.md` in a way that would surface early during the design phase. The investigator had to discover it by reading `engine.go` comments and the test file. A short paragraph in `ARCHITECTURE.md` under "Package layering" that explicitly lists the allowed import directions (and specifically calls out that `state` must never import `orchestrator`) would shorten the investigation phase for any future structural change.

The deferred scatter points in `validation/artifact.go` and `tools/guards.go` were not mentioned anywhere in the existing documentation — they were found only by grep during investigation. If these had been pre-catalogued in `BACKLOG.md` (which this feature now adds), the investigator would not have needed to discover them independently. This is a documentation gap that the feature itself closes, but it highlights the value of keeping `BACKLOG.md` current after each structural change.

### Code Readability

The `skipTable` in `flow_templates.go` used a template-centric layout (one entry per template) while the proposed registry uses a phase-centric layout (one `TemplateSkips` map per phase). The inversion is conceptually straightforward but required the investigator to spend time on section 2.9 of the investigation clarifying that the `TemplateFull` empty-slice semantics would need explicit handling in the new init path. A short comment on the `skipTable` declaration noting the nil-vs-empty-slice invariant for the `full` key would have made this immediately clear.

The duplicate `TemplateLight/Standard/Full` constants in `flow_templates.go` had no comment explaining why they existed separately from `state/constants.go`. A one-line comment such as `// local aliases; state.Template* is the canonical source` would have flagged the duplication as intentional (or not) before investigation.

### AI Agent Support (Skills / Rules)

The `CLAUDE.md` "What NOT to do" section correctly warns against creating circular imports but does not give the full allowed-import DAG. Adding a rule like "Import direction: `tools` → `orchestrator` → `state`; never reverse" would let the designer skip the import-cycle investigation step entirely for changes in this cluster of packages.

The testing rules in `.claude/rules/testing.md` do not mention `TestAllPhasesCount`'s magic-number pattern as a known fragility. If the testing checklist had included "check `phases_test.go` for hardcoded `wantCount`" as a note, the design task might have been more focused from the start.

No missing skill definitions were identified. The existing agent prompts (situation-analyst, investigator, architect) handled the problem well given the available documentation.

### Other

The analytics tool returned slightly different cumulative totals depending on when it was called (the comprehensive-review.md contained an earlier snapshot at 562,385 tokens / $3.37, while the final `analytics_pipeline_summary` call returned 670,421 tokens / $4.02). This is expected — phases complete after the review agent writes its artifact — but it meant the final summary needed to decide which figure to use. A convention note in `SKILL.md` or `verifier.md` stating "always use the value from `analytics_pipeline_summary` at the time the final-summary phase runs" would remove the ambiguity.

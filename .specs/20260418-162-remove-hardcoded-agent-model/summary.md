## Summary

This change removes the hardcoded `model: sonnet` field from the frontmatter of all 11 agent `.md` files in `agents/`, allowing agents to respect the user's configured default Claude Code model rather than always forcing Sonnet. Documentation SSOT templates are updated to reflect the new model-inheritance behaviour.

Key changes:
- Removed `model: sonnet` from all 11 agent frontmatter files (`agents/*.md`); frontmatter now contains only `name:` and `description:`.
- Updated 4 English SSOT template files (`template/sections/architecture/technical-decisions.md`, `design-principles.md`, `design-decisions.md`, `template/sections/agents/overview.md`) to replace prescriptive "all agents use `model: sonnet`" language with model-inheritance language.
- Updated 3 Japanese SSOT template files (counterparts under `template/sections/ja/`) to match.
- Regenerated `README.md` and `ARCHITECTURE.md` via `make docs`; both are clean of old prescriptive model text.
- No Go source changes — `state.DefaultModel = "sonnet"` in `constants.go` is intentionally retained as the analytics label; accurate runtime model logging is deferred to a future analytics improvement.

## Verification Report

### Part A: Build Verification

#### Typecheck
- Status: PASS
- Errors: none

#### Test Suite
- Total: 14 Go packages pass, 0 failed, 0 skipped; 62 hook tests pass, 0 failed
- Failures: none

### Overall: PASS

## Pipeline Statistics
- Total tokens: 643,857
- Total duration: 19m 31s
- Estimated cost: $3.86
- Phases executed: 14
- Phases skipped: 2
- Retries: 0
- Review findings: 0 critical, 3 minor

## Improvement Report

_Retrospective on what would have made this work easier._

### Documentation

The investigation identified that five template files (plus Japanese counterparts) explicitly state "all agents use `model: sonnet`" as a design principle, but discovering which of those files are SSOT sources versus generated outputs required manual cross-referencing between `investigation.md`, the `Makefile`, and the VitePress build config. A short note in `CLAUDE.md` or in the `template/` directory clarifying "these template sections are the SSOT; `README.md`, `CLAUDE.md`, and `docs/` pages are generated — edit only under `template/`" would have made the scope of documentation changes immediately clear without a grep-and-verify pass.

The analysis also notes that `template/sections/ja/architecture/` does not contain a `design-decisions.md` counterpart, which is a pre-existing structural asymmetry. This is not documented anywhere and required the investigator to manually enumerate the Japanese counterpart files before concluding the task list was correct. A brief index listing which English template sections have Japanese counterparts would eliminate this uncertainty.

### Code Readability

The two independent hardcoding sites (agent frontmatter vs. `state.DefaultModel` in the Go server) use the same string value `"sonnet"`, making it non-obvious from a quick search which site controls execution and which controls analytics. A comment at the `state.DefaultModel` declaration in `mcp-server/internal/state/constants.go` and at the `NewSpawnAgentAction` call sites in `engine.go` explaining that `Action.Model` is an analytics label (not the execution model) would have reduced the investigation time spent confirming that removing frontmatter alone was sufficient.

### AI Agent Support (Skills / Rules)

The analysis phase correctly identified all 11 agent files and the two independent hardcoding sites. However, the investigation had to independently re-verify the same information (especially the `engine.go` call-site count and test assertions) because there is no cached structural summary of how `engine.go` interfaces with `state.DefaultModel`. A `search_patterns` or `ast_summary` entry capturing the "model flow" through the engine would have allowed the investigation to proceed faster.

No missing CLAUDE.md rules were identified. The `.claude/rules/testing.md` checklist was directly useful for confirming which tests needed review.

### Other

The `make docs` regeneration step (needed to propagate template changes to `README.md` and `ARCHITECTURE.md`) is not mentioned in the implementation instructions or task list template. Implementers working on documentation-only changes would need to know to run `make docs` to keep generated files consistent. Adding "run `make docs` after editing any file under `template/`" to the task template or to a `.claude/rules/docs.md` reminder would prevent generated-vs-source drift going unnoticed until the comprehensive review phase.

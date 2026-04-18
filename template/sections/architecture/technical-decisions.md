# Key Technical Decisions

## Why mkdir-based locking instead of flock?

macOS doesn't ship flock. The mkdir-based lock uses `mkdir` as an atomic operation (POSIX guarantee), with a 5-second timeout and force-break for stale locks. A trap ensures cleanup on unexpected exit.

## Why fail-open hooks?

The plugin may be installed in environments without `jq`, or `state.json` may be missing during non-pipeline work. Fail-closed would block legitimate operations. Each hook checks `command -v jq` and exits 0 if missing.

## Why agents inherit the user's configured model instead of hardcoding sonnet?

Flexibility and user control. Previously all agents specified `model: sonnet` in their frontmatter, forcing every pipeline run onto Sonnet regardless of the user's configured default. Removing the `model:` field from agent frontmatter lets Claude Code use the user's active model — the same model selected by `/model` or the Claude Code default. Users who want to pin a specific model can add `model: <name>` back to individual agent frontmatter files (the per-agent selection mechanism from BACKLOG #21). For cost control, the user's own model configuration is the appropriate lever.

## Why the orchestrator doesn't read code?

Token economy. If the orchestrator read implementation files, its context would grow with each phase, degrading reasoning quality. By only reading small artifact files (~500 lines total across all phases), the orchestrator stays fast and focused.

This rule extends to diff output: review agents (Phase 6 impl-reviewer, Phase 7 comprehensive-reviewer) self-execute `git diff main...HEAD` inside their own agent context rather than having the orchestrator pre-compute and inject the diff. The diff is consumed in the agent's context, not the orchestrator's — the Token Economy Rule is satisfied.

## Why separate agent files instead of inline prompts?

1. Each agent has a persistent, versionable system prompt
2. Agents can be reused from other skills
3. Model can be configured per-agent in frontmatter
4. The orchestrator SKILL.md stays small (~500 lines vs ~900 with inline prompts)

## Guard migration pattern (shell → Go MCP handler)

When a new guard is added to `pre-tool-hook.sh` (e.g., Rule 3a–3j), the same invariant must also be enforced inside the corresponding Go MCP tool handler in `mcp-server/tools/`.

The pattern:

1. **Shell hook** (`pre-tool-hook.sh`): guards that fire on bash/edit tool calls. These use `state.json` on disk, read with `jq`. They block the bash command via exit 2. This layer is always active — even when the MCP server is not installed.

2. **Go handler** (`tools/guards.go`): guards that fire when MCP tools are called. These read `state.State` already loaded from `state.ReadState()`. Blocking guards return `IsError=true`; non-blocking warnings are included under the `"warning"` JSON key.

The two layers are **independent and complementary**. When the MCP server is in use, the Go handler fires first. The shell hook still fires on any `Bash` tool calls.

**Migration checklist** when adding a new guard:
- [ ] Add a named check function to `pre-tool-hook.sh` (do not inline in dispatch block)
- [ ] Add a corresponding function in `mcp-server/tools/guards.go` (blocking: returns `error`; warning: returns `string`)
- [ ] Call it from the relevant handler(s) in `mcp-server/tools/handlers.go`
- [ ] Add tests for both the shell guard (`test-hooks.sh`) and the Go guard (`tools/guards_test.go`)
- [ ] Document the new rule in [Guard Catalogue](guard-catalogue.md)

## Why are analysis.md and investigation.md separate files?

The two Phase 1–2 output files have distinct roles and a strict data dependency between them:

- **analysis.md** (Phase 1 — situation-analyst): maps the *current state* of the codebase — relevant files, interfaces, types, data flows, and existing tests. It is a read-only survey with no opinion on what should change.
- **investigation.md** (Phase 2 — investigator): builds *on top of* analysis.md — the investigator agent reads analysis.md as an explicit input before adding root cause analysis, edge cases, risks, external dependencies, prior art, ambiguities, and deletion/rename impact.

Merging them into one file would break this sequential dependency: the investigator would have to both read and write the same file, or the Phase 1 content would have to be injected into its prompt instead of residing on disk (violating the Files-Are-the-API principle).

Four additional reasons the split is load-bearing:

1. **Resume semantics** — each file is a separate phase checkpoint. If Phase 2 fails, Phase 1's analysis.md is already on disk and the investigator can retry without re-running the situation analyst.
2. **Consumer granularity** — `task-decomposer` (Phase 4) reads only `investigation.md`; `architect` and `design-reviewer` read both. Separate files let each consumer load exactly what it needs.
3. **Artifact guards** — `pipeline_report_result` validates `analysis.md` on Phase 1 completion and `investigation.md` on Phase 2 completion independently. A single merged file would require one guard to validate two distinct sections, coupling the guard logic to content structure.
4. **Investigation flow** — when the pipeline is run as an investigation (no implementation phases), it ends after Phase 2 and presents both files as the final deliverable to the user. Keeping them separate makes the output navigable as two named documents.

The two-file split is maintained regardless of effort level. Even though both files are produced in the same pipeline run, they serve distinct roles and are consumed by different downstream agents.

## Why inline comment anchors for SKILL.md cross-references?

SKILL.md is consumed by an LLM reading raw Markdown, not a renderer. HTML anchors (`<a id="...">`) would be invisible in rendered view but visible to the LLM in raw text. The chosen convention appends `<!-- anchor: <token> -->` to target headings and uses the token (not the heading text) in all prose references. Tokens are short, lowercase, hyphenated, and can be searched with `grep anchor:`. This is the stable-label convention for SKILL.md. When adding new cross-referenced sections, follow this pattern.

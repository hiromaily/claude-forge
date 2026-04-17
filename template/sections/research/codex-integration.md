# Codex Integration Feasibility Analysis

> **Research date:** 2026-04-16
> **Question:** Can claude-forge be published as an OpenAI Codex plugin equivalent to its current Claude Code plugin form?

## Background

claude-forge ships the following artifacts as a single Claude Code plugin:

- `.claude-plugin/plugin.json` + `marketplace.json`
- `.mcp.json` (forge-state MCP server registration)
- `agents/*.md` — 10 named subagent definitions
- `skills/forge/SKILL.md` — orchestrator skill
- `hooks/hooks.json` — `PreToolUse` / `PostToolUse` / `Stop` / `Setup`
- `scripts/*.sh` — hook implementations referencing `${CLAUDE_PLUGIN_ROOT}`
- `mcp-server/` — Go binary, 47 tools

This document evaluates whether the same bundle can be published as a Codex plugin as of April 2026.

## Key Finding

Codex now ships a plugin system (`codex marketplace add`, v0.121.0). Plugin bundles officially support `skills`, `mcpServers`, and `apps` — but **not** `agents` or `hooks` fields. Two open-issue blockers prevent a faithful port of claude-forge's core design.

## Comparison Matrix

| claude-forge element | Claude Code | Codex (2026-04) | Parity | Notes |
|---|---|---|---|---|
| Plugin packaging | `.claude-plugin/plugin.json` + marketplace | `.codex-plugin/plugin.json`, `codex marketplace add` (v0.121.0) | ◎ | GitHub / git / local / URL install sources |
| Bundle contents | MCP + agents + hooks + skills + commands + scripts | `skills` + `mcpServers` + `apps` only (official schema) | △ | `agents` / `hooks` bundling unsupported |
| MCP server | `.mcp.json`, stdio + http | `[mcp_servers.<name>]` TOML, `codex mcp add`, stdio + streamable HTTP | ◎ | `env`, `bearer_token_env_var`, OAuth supported |
| Subagent definition | `agents/*.md` with frontmatter | `~/.codex/agents/*.toml` or `.codex/agents/*.toml` | ○ | Fields: `name` / `description` / `developer_instructions` / `model` / `model_reasoning_effort` / `sandbox_mode` |
| **Subagent spawn from orchestrator** | `Agent` / `Task` tool, custom name | `spawn_agent` **cannot invoke named custom subagents** from tool-backed sessions ([#15250](https://github.com/openai/codex/issues/15250), open) | ✕ | **Blocker #1 — breaks core design** |
| Isolated context / parallel | One `Agent` call per spawn | `agents.max_threads=6`, `max_depth=1` (default) | ○ | Background progress streaming added in v0.119 |
| Subagent plugin distribution | `agents/` inside plugin | **No `agents` field in plugin.json schema** | ✕ | Users must manually copy TOML files to `~/.codex/agents/` |
| Skills / slash | `skills/*/SKILL.md` | Same pattern, `/skills` or `$skill` invocation | ◎ | Plugin-bundleable |
| Custom prompts | `commands/*.md` | `custom-prompts` deprecated → migrate to skills | ○ | |
| Hook events | PreToolUse / PostToolUse / Stop / Setup / UserPromptSubmit | Same names | ○ | Same stdin JSON; `exit 2 = block` |
| **Hook event coverage** | All tools | **PreToolUse / PostToolUse fires for `shell` (Bash) only** ([#14754](https://github.com/openai/codex/issues/14754), [#16732](https://github.com/openai/codex/issues/16732)) | ✕ | **Blocker #2 — Phase 1/2 Write/Edit guard cannot fire** |
| Hook plugin distribution | `hooks/hooks.json` inside plugin | No `hooks` field in plugin.json schema | △ | Plugin-layer hook bundling is not officially supported |
| `${CLAUDE_PLUGIN_ROOT}` equivalent | Env var injected by host | Not documented for Codex | ✕ | Hook scripts cannot reference the plugin install dir |
| Project-memory file | `CLAUDE.md` (project + user) | `AGENTS.md` (auto-loaded; project + user) | ○ | No plugin-level API to append project instructions |
| Tool name surface | `Agent` / `Bash` / `Edit` / `Write` / `Read` / `Glob` / `Grep` / `TodoWrite` | `shell` / `apply_patch` / `spawn_agent` / `spawn_agents_on_csv` (no direct Read/Glob/Grep/TodoWrite) | △ | Agent prompts must be rewritten |

**Legend:** ◎ equivalent / ○ workable alternative / △ partial / ✕ missing.

## Critical Blockers

### Blocker #1 — Named subagent spawn unavailable from tool-backed sessions

[openai/codex#15250](https://github.com/openai/codex/issues/15250) (open as of 2026-04). `.codex/agents/*.toml` custom agents are invocable via the natural-language CLI surface, but the `spawn_agent` tool — the surface a skill orchestrator must use — only accepts generic agent types. claude-forge's design hinges on the orchestrator skill spawning specific named subagents per phase (10 specialists across the pipeline); that pattern cannot be expressed with the current tool API.

A community workaround pattern (see [leonardsellem/codex-subagents-mcp](https://github.com/leonardsellem/codex-subagents-mcp)) reads the TOML files and injects `developer_instructions` into a generic worker via an external MCP. That sacrifices native subagent isolation semantics.

### Blocker #2 — Hooks do not fire for Write / Edit / apply_patch

Per [openai/codex#14754](https://github.com/openai/codex/issues/14754) and [openai/codex#16732](https://github.com/openai/codex/issues/16732), `PreToolUse` and `PostToolUse` events are emitted only for the `shell` tool. claude-forge's `pre-tool-hook.sh` enforces Phase 1/2 read-only mode by blocking `Edit` and `Write` during situation analysis and investigation — that enforcement point does not exist in Codex today.

## Compatible Surfaces

The following parts of claude-forge port with little friction:

- **MCP server** — the Go binary (`forge-state`, 47 tools) works unchanged; only the registration format changes (`.codex-plugin/plugin.json` `mcpServers` entry or `[mcp_servers.*]` in `~/.codex/config.toml`).
- **`SKILL.md`** — the Codex skill schema closely matches Claude Code; `skills/forge/SKILL.md` can be reused with minimal edits.
- **Hook payload / exit codes** — stdin JSON contract and `exit 2 = block` are identical. `pre-tool-hook.sh` rules that gate the `Bash` tool (Phase 5 parallel `git commit` block, `git checkout` to `main`/`master` block) still fire on Codex's `shell`. Only the `Edit` / `Write` rules go silent.
- **AGENTS.md** — already a Codex-native convention; can be derived from `CLAUDE.md`.

## Verdict

**Publishing claude-forge as a Codex plugin is not recommended as of 2026-04.** Wait for upstream resolution of the blockers, or accept a reduced-fidelity port.

Upstream milestones that would change this decision:

1. Resolution of [openai/codex#15250](https://github.com/openai/codex/issues/15250) — `spawn_agent` accepts named custom agents.
2. `agents` and `hooks` fields added to `.codex-plugin/plugin.json` schema.
3. Documented `CODEX_PLUGIN_ROOT` (or equivalent) env var for hook scripts.
4. PreToolUse / PostToolUse coverage extended to `apply_patch` (Write / Edit equivalent).

Until then, a partial port (~60–70 % fidelity) would ship `forge-state` MCP + `SKILL.md`, require users to manually place subagent TOML files, and omit the Phase 1/2 write-guard. That pipeline no longer delivers the "isolated subagent, context-contamination prevention" value proposition that defines claude-forge.

## References

- [Codex Plugins — Build guide](https://developers.openai.com/codex/plugins/build)
- [Codex Subagents](https://developers.openai.com/codex/subagents)
- [Codex Skills](https://developers.openai.com/codex/skills)
- [Codex Hooks](https://developers.openai.com/codex/hooks)
- [Codex MCP](https://developers.openai.com/codex/mcp)
- [Codex Changelog](https://developers.openai.com/codex/changelog)
- [Codex AGENTS.md guide](https://developers.openai.com/codex/guides/agents-md)
- [openai/codex#15250 — named subagents not accessible from tool-backed sessions](https://github.com/openai/codex/issues/15250)
- [openai/codex#14754 — PreToolUse / PostToolUse coverage for non-Bash tools](https://github.com/openai/codex/issues/14754)
- [openai/codex#16732 — ApplyPatchHandler hook events](https://github.com/openai/codex/issues/16732)
- [leonardsellem/codex-subagents-mcp — workaround pattern](https://github.com/leonardsellem/codex-subagents-mcp)

## Caveats

The findings above were collected via web research on 2026-04-16 and have **not** been independently verified by exercising Codex 0.121.0 locally. Before acting on this analysis:

1. Re-check [openai/codex#15250](https://github.com/openai/codex/issues/15250) status — it is the single most important blocker.
2. Re-check the official `.codex-plugin/plugin.json` schema for `agents` / `hooks` field additions.
3. Re-check hook event coverage for `apply_patch` (Write / Edit equivalent).
4. Confirm whether a `CODEX_PLUGIN_ROOT` env var has been documented.

Trust the current Codex docs over this document when they disagree, and update this document accordingly.

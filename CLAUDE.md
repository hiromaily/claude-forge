# Claude-Forge Plugin — AI Agent Guide

This is a Claude Code plugin that orchestrates a multi-phase development pipeline using isolated subagents.

## Directory Structure

```
claude-forge/
├── CLAUDE.md              ← you are here (auto-loaded by Claude Code)
├── ARCHITECTURE.md        ← design decisions, data flows, rationale
├── BACKLOG.md             ← known issues, improvement candidates
├── .claude-plugin/
│   └── plugin.json        ← plugin metadata (name, version)
├── .claude/
│   └── rules/
│       ├── git.md         ← Git practices enforced in this repo
│       └── shell-script.md ← Shell scripting conventions for *.sh files
├── agents/                ← 11 named agent definitions (.md files)
│   ├── README.md          ← agent roster with roles
│   ├── situation-analyst.md  (Phase 1: codebase mapping)
│   ├── investigator.md       (Phase 2: deep-dive research)
│   ├── analyst.md            (Phase 1+2 merged: lite flow template)
│   ├── architect.md          (Phase 3: design)
│   ├── design-reviewer.md    (Phase 3b: design quality gate)
│   ├── task-decomposer.md    (Phase 4: task breakdown)
│   ├── task-reviewer.md      (Phase 4b: task quality gate)
│   ├── implementer.md        (Phase 5: TDD implementation)
│   ├── impl-reviewer.md      (Phase 6: code review)
│   └── verifier.md           (Final: full build/test verification)
├── hooks/
│   └── hooks.json         ← hook definitions (PreToolUse, PostToolUse, Stop)
├── scripts/
│   ├── state-manager.sh   ← core state management CLI (26 commands, jq-based)
│   ├── build-specs-index.sh ← scans .specs/ directories and builds .specs/index.json
│   ├── query-specs-index.sh ← keyword-score matching against .specs/index.json, stdout markdown or empty
│   ├── validate-input.sh  ← deterministic input validation (empty, too short, URL format)
│   ├── pre-tool-hook.sh   ← read-only, commit blocking, checkpoint, artifact & input validation guards
│   ├── post-agent-hook.sh ← agent output quality validation
│   ├── post-bash-hook.sh  ← auto-commits state.json+summary.md after phase-complete post-to-source
│   ├── stop-hook.sh       ← pipeline completion guard
│   └── test-hooks.sh      ← automated test suite (339 tests; run bash scripts/test-hooks.sh to verify)
└── skills/
    └── forge/
        └── SKILL.md       ← orchestrator instructions (the main skill)
```

## How the Pieces Connect

```
SKILL.md (orchestrator)
  ├── calls scripts/validate-input.sh before workspace setup
  ├── invokes agents/ by name via Agent tool
  ├── calls scripts/state-manager.sh for state transitions
  └── hooks/ enforce constraints automatically
       ├── pre-tool-hook.sh reads state.json → blocks writes in Phase 1-2,
       │     blocks git commit in parallel Phase 5, checkpoint guard,
       │     artifact guard, input validation guard (blocks init without
       │     prior validate-input.sh call)
       ├── post-agent-hook.sh reads state.json → warns on bad output
       ├── post-bash-hook.sh reads state.json → auto-commits state.json+summary.md after post-to-source
       └── stop-hook.sh reads state.json → blocks premature stop
```

## Rules for Modifying This Plugin

### Consistency requirements
- When adding/changing an agent's input files → update BOTH the agent .md file AND the Agent Roster table in SKILL.md
- When adding a new phase → add its ID to the PHASES array in state-manager.sh
- When changing hook behavior → verify the hook's exit code semantics (exit 0 = allow, exit 2 = block)
- When changing state.json schema → ensure state-manager.sh, all hook scripts, and SKILL.md are aligned

### Testing
- state-manager.sh: test all commands manually with a temp directory
  ```bash
  mkdir -p /tmp/test-sm
  bash scripts/state-manager.sh init /tmp/test-sm test
  bash scripts/state-manager.sh phase-start /tmp/test-sm phase-1
  bash scripts/state-manager.sh resume-info /tmp/test-sm
  bash scripts/state-manager.sh abandon /tmp/test-sm
  rm -rf /tmp/test-sm
  ```
- Task-type state commands:
  ```bash
  bash scripts/state-manager.sh set-task-type /tmp/test-sm bugfix
  bash scripts/state-manager.sh get /tmp/test-sm taskType        # → "bugfix"
  bash scripts/state-manager.sh skip-phase /tmp/test-sm phase-3b
  bash scripts/state-manager.sh get /tmp/test-sm skippedPhases   # → ["phase-3b"]
  bash scripts/state-manager.sh resume-info /tmp/test-sm | jq '{taskType, skippedPhases}'
  bash scripts/state-manager.sh set-auto-approve /tmp/test-sm
  bash scripts/state-manager.sh get /tmp/test-sm autoApprove       # → true
  bash scripts/state-manager.sh resume-info /tmp/test-sm | jq '{autoApprove}'
  bash scripts/state-manager.sh set-skip-pr /tmp/test-sm
  bash scripts/state-manager.sh get /tmp/test-sm skipPr            # → true
  bash scripts/state-manager.sh resume-info /tmp/test-sm | jq '{skipPr}'
  ```
- Effort and flow template commands:
  ```bash
  bash scripts/state-manager.sh set-effort /tmp/test-sm S
  bash scripts/state-manager.sh get /tmp/test-sm effort              # → "S"
  bash scripts/state-manager.sh set-effort /tmp/test-sm INVALID      # → exit 1
  bash scripts/state-manager.sh set-flow-template /tmp/test-sm lite
  bash scripts/state-manager.sh get /tmp/test-sm flowTemplate        # → "lite"
  bash scripts/state-manager.sh set-flow-template /tmp/test-sm bad   # → exit 1
  bash scripts/state-manager.sh resume-info /tmp/test-sm | jq '{effort, flowTemplate}'
  ```
- Debug flag command:
  ```bash
  bash scripts/state-manager.sh set-debug /tmp/test-sm
  bash scripts/state-manager.sh get /tmp/test-sm debug              # → true
  bash scripts/state-manager.sh resume-info /tmp/test-sm | jq '{debug, tasksWithRetries}'
  ```
- Use-current-branch commands:
  ```bash
  bash scripts/state-manager.sh set-use-current-branch /tmp/test-sm feature/existing
  bash scripts/state-manager.sh get /tmp/test-sm useCurrentBranch   # → true
  bash scripts/state-manager.sh get /tmp/test-sm branch              # → "feature/existing"
  bash scripts/state-manager.sh resume-info /tmp/test-sm | jq '{useCurrentBranch, branch}'
  ```
- Phase metrics commands:
  ```bash
  bash scripts/state-manager.sh phase-log /tmp/test-sm phase-1 5000 30000 sonnet
  bash scripts/state-manager.sh phase-stats /tmp/test-sm
  bash scripts/state-manager.sh resume-info /tmp/test-sm | jq '{phaseLogEntries, totalTokens, totalDuration_ms}'
  ```
- Hook scripts: pipe sample JSON to stdin and check exit code
  ```bash
  echo '{"tool_name":"Edit","tool_input":{"file_path":"/src/foo.go"}}' | bash scripts/pre-tool-hook.sh
  echo $?  # should be 0 (no active pipeline) or 2 (blocked)
  ```
- **Full test suite** (run after any change):
  ```bash
  bash scripts/test-hooks.sh   # run to see current test count
  ```

### Key design decisions (see ARCHITECTURE.md for details)
- All agents use `model: sonnet` — cost optimization. Change to `opus` for agents that need stronger reasoning.
- Hooks are fail-open — if jq is missing or parsing fails, the action is allowed rather than blocked.
- State is file-based (state.json) — survives context compaction and session restarts.
- The orchestrator never reads source code — only small artifact files. This is a token economy rule.
- **Prefer deterministic enforcement over prompt instructions.** The orchestrator (SKILL.md) is an LLM and may skip or misinterpret instructions non-deterministically. When adding or changing pipeline behavior, first consider whether a hook script can enforce it deterministically (exit 2 = block). Use prompt instructions only for behavior that cannot be expressed as a state-based guard. Examples: checkpoint guards (P10-3), artifact guards, read-only phase enforcement — all implemented as hooks, not just prose.

### Script structure conventions

**state-manager.sh** — dispatch calls `locked_update` directly. There is no intermediate `cmd_*` wrapper layer. Write commands must always go through `locked_update` to be safe under parallel Phase 5 execution. Read-only commands that only call `read_state` + `jq` do not need locking.

**pre-tool-hook.sh** — Rule 3 sub-checks (3a–3j) are each extracted into a named function (`check_phase1_warnings`, `check_task_init_guard`, etc.). The main dispatch block calls these functions in sequence. Do not inline new sub-checks into the dispatch block — add a named function and call it from there.

**find_active_workspace** — this function is duplicated across `pre-tool-hook.sh`, `post-agent-hook.sh`, and `stop-hook.sh`. **The copies are intentionally different**: each script uses a slightly different filter predicate suited to its own enforcement context. Do not unify them into a shared library. Each copy carries a comment explaining the divergence — read it before modifying.

**Subcommand count** — `state-manager.sh` currently has **26** dispatch entries, and the `forge-state` MCP server exposes the same **26** commands as typed tool calls. When adding or removing a command, update the count in `CLAUDE.md` (here), `scripts/README.md`, and `README.md`. The count drifted to "22" in documentation before and was caught only in a comprehensive review pass — keep it accurate.

### What NOT to do
- Do NOT add `isolation: "worktree"` to any Agent tool call — breaks inter-task visibility
- Do NOT duplicate agent instructions in SKILL.md prompts — agents have their own system prompts
- Do NOT store state in memory/conversation — use state.json via state-manager.sh
- Do NOT use bare `flock` without a mkdir fallback — macOS lacks `flock` by default. The existing `locked_update` helper in state-manager.sh already handles both cases; use it instead of reimplementing locking

## MCP Server Registration

The `forge-state` MCP server replaces direct `bash scripts/state-manager.sh` calls with typed MCP tool calls. To use it:

### 1. Build and install the binary

```bash
make install
```

This compiles `mcp-server/` and copies the binary (`forge-state-mcp`) to `$(GOBIN)` or `~/.local/bin`.

### 2. Register the server in `.claude/settings.json`

Add the following `mcpServers` entry to your `.claude/settings.json`:

```json
{
  "mcpServers": {
    "forge-state": {
      "command": "forge-state-mcp",
      "args": [],
      "env": {}
    }
  }
}
```

After saving, restart Claude Code. The `mcp__forge-state__*` tool calls in `SKILL.md` will route to the running server process.

### Fallback

`scripts/state-manager.sh` remains fully functional as a fallback. All 26 commands still execute correctly. The script includes a deprecation notice at the top pointing to this section.

---

## Before You Start Working

1. Read `BACKLOG.md` for known issues and improvement candidates
2. Read `ARCHITECTURE.md` for design rationale if making structural changes
3. Check `agents/README.md` for the current agent roster
4. Run `bash scripts/state-manager.sh` with no args to see available commands
5. Read `.claude/rules/git.md` for Git branch and commit conventions
6. Read `.claude/rules/shell-script.md` for Bash scripting conventions before editing any `.sh` file

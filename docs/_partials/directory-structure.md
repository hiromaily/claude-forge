<!-- SSOT: claude-forge repository directory structure.
     Included by:
       docs/architecture/overview.md,
       README.md (reference only — not VitePress-processed),
       CLAUDE.md (reference only — not VitePress-processed)
     Edit only this file when the directory structure changes.

     Note for Claude Code: This file is included via VitePress <!--@include:--> directives
     in docs/ files. Claude Code cannot follow those directives, so read this file directly
     when you need the canonical directory structure. -->

```
claude-forge/
├── CLAUDE.md              ← AI agent guide (auto-loaded by Claude Code)
├── ARCHITECTURE.md        ← index (full docs in docs/architecture/)
├── BACKLOG.md             ← known issues, improvement candidates
├── README.md              ← project overview and quick start
├── .claude-plugin/
│   └── plugin.json        ← plugin metadata (name, version)
├── .claude/
│   └── rules/
│       ├── git.md         ← Git practices enforced in this repo
│       ├── shell-script.md ← Shell scripting conventions for *.sh files
│       └── docs.md        ← Documentation rules (SSOT, bilingual, VitePress)
├── agents/                ← 10 named agent definitions (.md files)
│   ├── README.md          ← agent roster with roles
│   ├── situation-analyst.md
│   ├── investigator.md
│   ├── architect.md
│   ├── design-reviewer.md
│   ├── task-decomposer.md
│   ├── task-reviewer.md
│   ├── implementer.md
│   ├── impl-reviewer.md
│   ├── comprehensive-reviewer.md
│   └── verifier.md
├── docs/
│   ├── _partials/         ← SSOT content fragments (included by docs/)
│   └── architecture/      ← architecture documentation (13 focused files)
├── hooks/
│   └── hooks.json         ← hook definitions (PreToolUse, PostToolUse, Stop)
├── mcp-server/            ← Go MCP server source (forge-state binary)
├── scripts/
│   ├── common.sh          ← shared find_active_workspace helper
│   ├── launch-mcp.sh      ← self-healing MCP launcher
│   ├── pre-tool-hook.sh   ← read-only guard, commit blocking, checkout blocking
│   ├── post-agent-hook.sh ← agent output quality validation
│   ├── post-bash-hook.sh  ← auto-commits state.json+summary.md (v1 legacy)
│   ├── setup.sh           ← downloads forge-state-mcp binary from GitHub Releases
│   ├── stop-hook.sh       ← pipeline completion guard
│   └── test-hooks.sh      ← automated test suite (62 tests)
└── skills/
    └── forge/
        └── SKILL.md       ← orchestrator instructions (the main skill)
```

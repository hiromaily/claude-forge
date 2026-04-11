## How the Pieces Connect

```
SKILL.md (orchestrator)
  ├── calls mcp__forge-state__validate_input before workspace setup
  ├── invokes agents/ by name via Agent tool
  ├── calls mcp__forge-state__* MCP tools for state transitions
  └── hooks/ enforce constraints automatically
       ├── pre-tool-hook.sh reads state.json → blocks writes in Phase 1-2,
       │     blocks git commit in parallel Phase 5,
       │     blocks checkout/switch to main/master
       ├── post-agent-hook.sh reads state.json → warns on bad output
       ├── post-bash-hook.sh reads state.json → auto-commits state.json+summary.md after post-to-source
       └── stop-hook.sh reads state.json → blocks premature stop
```

## Installation

For the complete step-by-step guide, see [SETUP.md](../../../SETUP.md).

### Quick start — Plugin users (recommended)

```bash
# Step 1: Register the marketplace (one-time)
/plugin marketplace add hiromaily/claude-forge

# Step 2: Install the plugin (binary downloaded automatically)
/plugin install claude-forge
/reload-plugins

# Step 3: Restart Claude Code and verify
/mcp   # forge-state should show as Connected
```

> **Note:** `/plugin marketplace add` only registers the source — you must also run `/plugin install` to activate the plugin and trigger the binary download.

### Quick start — Local development

For contributors building from source:

```bash
# From the claude-forge directory
make setup

# Restart Claude Code and verify
/mcp   # forge-state should show as Connected
```

### Prerequisites

- **Go** — required to build the MCP server binary
- **jq** — required for state management and hook scripts. Install via `brew install jq` (macOS) or your package manager.

### Environment variables

Environment variables are configured automatically when using `make setup`. For manual setup, pass them via `claude mcp add --env`:

| Variable | Required | Description |
| --- | --- | --- |
| `FORGE_AGENTS_PATH` | Yes | Absolute path to the `agents/` directory. Required for `pipeline_next_action` to resolve agent `.md` files at runtime. Set automatically by `make setup`. |
| `FORGE_SPECS_DIR` | No | Override the default `.specs/` directory used by the engine. |
| `FORGE_EVENTS_PORT` | No | Port for the SSE events endpoint (`/events`) and the bundled web dashboard (`/`). |

---

# claude-forge setup

## Prerequisites

- **Go** — required to build the MCP server binary
- **jq** — required for state management and hook scripts (`brew install jq` on macOS)

## Step 1: Install the plugin

Start a new Claude Code session and run:

```
# Register the marketplace (one-time)
/plugin marketplace add hiromaily/claude-forge

# Install the plugin
/plugin install claude-forge
```

Alternative installation methods:

```
# Install from a local clone
git clone https://github.com/hiromaily/claude-forge.git
claude plugins install ~/path/to/claude-forge

# One-time session only (no persistent install)
claude --plugin-dir ~/path/to/claude-forge
```

## Step 2: Build, install, and register the MCP server

The `forge-state` MCP server is required for all pipeline operations. From the claude-forge directory:

```bash
make setup
```

This single command:
1. Compiles the Go binary (`forge-state-mcp`) and installs it to `$GOBIN` or `~/.local/bin`
2. Registers the MCP server with Claude Code (using `claude mcp add`)
3. Configures `FORGE_AGENTS_PATH` automatically

The command is idempotent — safe to re-run after pulling updates.

<details>
<summary>Manual setup (if you need custom options)</summary>

```bash
# Build and install the binary
make install

# Register with Claude Code
claude mcp add forge-state \
  --transport stdio \
  --scope user \
  --env FORGE_AGENTS_PATH=/absolute/path/to/claude-forge/agents \
  -- forge-state-mcp
```

> Replace `/absolute/path/to/claude-forge/agents` with the actual absolute path to the `agents/` directory in your clone.
>
> Use `--scope user` to make the server available across all projects, or `--scope local` for the current project only.

</details>

## Step 3: Restart Claude Code

The MCP server is loaded at session startup. Restart your Claude Code session to activate it.

## Step 4: Verify the setup

After restarting, confirm the server is connected:

```
/mcp
```

You should see `forge-state` listed with status `Connected`. If it shows as disconnected, check:

1. The binary exists at the installed path (`which forge-state-mcp`)
2. The `FORGE_AGENTS_PATH` points to a valid directory containing agent `.md` files
3. Run `claude mcp get forge-state` to inspect the current configuration

## Updating

```
# Update the plugin
claude plugin update claude-forge@claude-forge

# Rebuild the MCP server after pulling new changes
make install

# Reload plugins without restarting (does not reload MCP servers)
/reload-plugins
```

## Uninstalling

```
# Remove the MCP server
claude mcp remove forge-state

# Remove the plugin
claude plugins uninstall claude-forge@claude-forge
```

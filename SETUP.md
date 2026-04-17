# Installation

## Prerequisites

- **Claude Code** — the CLI must be installed
- **jq** — required for hook scripts (`brew install jq` on macOS)
- **Go** — required only for local development builds (not needed for plugin install)

## Plugin Install (Recommended)

Install the plugin and everything is configured automatically:

```bash
# Register the marketplace (one-time)
/plugin marketplace add hiromaily/claude-forge

# Install the plugin
/plugin install claude-forge
/reload-plugins
```

After installation, restart Claude Code and verify:

```bash
/mcp   # forge-state should show as Connected
```

### What happens automatically

When the plugin is installed, Claude Code:

1. **Registers the MCP server** — `.mcp.json` declares the `forge-state` server
2. **Runs the Setup hook** — downloads the pre-built `forge-state-mcp` binary from GitHub Releases
3. **Falls back to source build** — if the release binary is unavailable, builds from source using `go build`

```
plugin.json                        ← declares "mcpServers": "./.mcp.json"
  └─> .mcp.json                    ← defines forge-state server (stdio transport)
        └─> scripts/launch-mcp.sh  ← self-healing launcher
              └─> bin/forge-state-mcp ← binary downloaded by Setup hook
```

### Alternative installation

```bash
# Install from a local clone
claude plugins install ~/path/to/claude-forge

# One-time session only (no persistent install)
claude --plugin-dir ~/path/to/claude-forge
```

## Local Development Setup

For contributors working on claude-forge itself:

```bash
# Build and install binary, then register with Claude Code
make setup-manual
```

This registers the MCP server with `--scope local` (written to `.claude/settings.local.json`, gitignored).

## Environment Variables

| Variable | Required | Description |
| --- | --- | --- |
| `FORGE_AGENTS_PATH` | Yes | Absolute path to `agents/` directory. Set automatically by `make setup`. |
| `FORGE_SPECS_DIR` | No | Override the default `.specs/` directory. |
| `FORGE_EVENTS_PORT` | No | Port for the SSE events endpoint and the bundled web dashboard (`http://localhost:<port>/`). |

## Troubleshooting

### Two forge-state entries, one failing

When working inside the claude-forge dev repo with the plugin also installed, run `make setup-manual` to register a local-scope override.

### forge-state shows as "Failed to connect"

1. Check if the binary exists
2. Check `FORGE_AGENTS_PATH` points to a valid directory
3. Test the binary directly: `echo '{}' | forge-state-mcp`
4. Re-run setup

### Setup hook didn't run

Force re-run by removing the version marker:

```bash
rm -f $(claude plugins path)/claude-forge/bin/.installed-version
```

## Updating

```bash
claude plugin update claude-forge@claude-forge
/reload-plugins
# Restart Claude Code to reload MCP servers
```

## Uninstalling

```bash
claude plugins uninstall claude-forge@claude-forge
# If manually registered:
claude mcp remove forge-state -s user
```

## Releasing a new version

When cutting a new release:

1. Update the version in `plugin.json`:
   ```bash
   make update-tag new=1.5.0 old=1.4.0
   ```

2. Commit and push:
   ```bash
   git add -A && git commit -m "chore: bump version to 1.5.0"
   git push origin main
   ```

3. Create and push the tag:
   ```bash
   make update-git-tag new=1.5.0
   ```

4. GitHub Actions (`release.yml`) automatically:
   - Cross-compiles `forge-state-mcp` for darwin/arm64, darwin/amd64, linux/amd64, linux/arm64
   - Creates a GitHub Release with the gzipped binaries attached
   - Generates release notes from commits

When users update the plugin, the Setup hook re-runs and downloads the new binary.

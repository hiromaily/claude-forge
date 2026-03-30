# claude-forge setup

## Prerequisites

- **Go** — required only for local development builds (not needed for plugin install)
- **jq** — required for hook scripts (`brew install jq` on macOS)

## Quick Start (Plugin Install — Recommended)

Install the plugin and everything is configured automatically:

```bash
# Register the marketplace (one-time)
/plugin marketplace add hiromaily/claude-forge

# Install the plugin
/plugin install claude-forge
```

Alternative installation methods:

```bash
# Install from a local clone
claude plugins install ~/path/to/claude-forge

# One-time session only (no persistent install)
claude --plugin-dir ~/path/to/claude-forge
```

### What happens automatically

When the plugin is installed, Claude Code:

1. **Registers the MCP server** — `.mcp.json` declares the `forge-state` MCP server, and the `mcpServers` field in `plugin.json` tells Claude Code to auto-register it
2. **Runs the Setup hook** — `hooks/hooks.json` defines a `Setup` event that runs `scripts/setup.sh`
3. **Downloads the binary** — `setup.sh` detects the platform (OS/arch), downloads the pre-built `forge-state-mcp` binary from the matching GitHub Release, and places it at `$CLAUDE_PLUGIN_ROOT/bin/forge-state-mcp`
4. **Falls back to source build** — if the release binary is not available (e.g., unreleased version), `setup.sh` builds from source using `go build` (requires Go)

After installation, restart Claude Code and verify with `/mcp` — `forge-state` should show as `Connected`.

### How auto-registration works

```
plugin.json                        ← declares "mcpServers": "./.mcp.json"
  └─> .mcp.json                    ← defines forge-state server (stdio transport)
        └─> scripts/launch-mcp.sh  ← self-healing launcher (runs setup.sh if binary missing)
              └─> bin/forge-state-mcp ← binary downloaded by Setup hook or launch-mcp.sh

hooks/hooks.json
  └─> Setup event                  ← triggers scripts/setup.sh on install
        └─> scripts/setup.sh       ← downloads binary from GitHub Releases
```

Key files:

| File | Role |
|------|------|
| `.claude-plugin/plugin.json` | Plugin metadata + `mcpServers` pointer |
| `.mcp.json` | MCP server definition (type, command, env) |
| `hooks/hooks.json` | Setup hook that triggers binary download |
| `scripts/setup.sh` | Binary downloader with source-build fallback |
| `scripts/launch-mcp.sh` | Self-healing launcher: runs `setup.sh` if binary is missing, then execs it |

### Version-aware caching

`setup.sh` writes a `.installed-version` marker alongside the binary. On subsequent plugin updates, it compares the marker against `plugin.json` version and re-downloads only when the version changes.

## Local Development Setup

For contributors working on claude-forge itself, build and register manually:

```bash
# Build and install binary to $GOBIN or ~/.local/bin, then register with Claude Code
make setup-manual
```

This registers the MCP server with `--scope local` (written to `.claude/settings.local.json`, which is gitignored). The local scope takes precedence over the project `.mcp.json`, so you won't get a duplicate failing `forge-state` entry when working inside this repo.

> **Note:** If you also have the claude-forge plugin installed, you'll still see `plugin:claude-forge:forge-state` (Connected) alongside the local dev entry. The local dev entry (`forge-state`) will use your locally built binary.

After registration, restart Claude Code and verify with `/mcp`.

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

## Troubleshooting

### Two forge-state entries, one failing

When working **inside the claude-forge dev repo** with the plugin also installed, you may see two entries in `/mcp`:

- `plugin:claude-forge:forge-state` — Connected (plugin-managed, correct)
- `forge-state: ${CLAUDE_PLUGIN_ROOT}/...` — Failed (project `.mcp.json`, `CLAUDE_PLUGIN_ROOT` not set in dev context)

Fix: run `make setup-manual`. This registers a local-scope override that supersedes the project `.mcp.json` entry and resolves to your locally built binary.

### forge-state shows as "Failed to connect"

1. Check if the binary exists:
   ```bash
   # For plugin install:
   ls -la $(claude plugins path)/claude-forge/bin/forge-state-mcp

   # For local dev:
   which forge-state-mcp
   ```

2. Check `FORGE_AGENTS_PATH` points to a valid directory:
   ```bash
   claude mcp get forge-state
   ```

3. Test the binary directly:
   ```bash
   echo '{}' | forge-state-mcp
   ```
   If it crashes, rebuild: `cd mcp-server && go build -o ../bin/forge-state-mcp .`

4. Re-run setup:
   ```bash
   # Plugin users: reinstall the plugin
   # Local dev: make setup-manual
   ```

### Setup hook didn't run

The Setup hook runs only on first install or plugin update. To force re-run:
```bash
# Remove the version marker to trigger re-download
rm -f $(claude plugins path)/claude-forge/bin/.installed-version
```

Then restart Claude Code.

## Updating

```bash
# Update the plugin
claude plugin update claude-forge@claude-forge

# Reload plugins without restarting (does not reload MCP servers)
/reload-plugins

# Restart Claude Code to reload MCP servers
```

## Uninstalling

```bash
# Remove the plugin (also removes auto-registered MCP server)
claude plugins uninstall claude-forge@claude-forge

# If manually registered, also remove:
claude mcp remove forge-state -s user
```

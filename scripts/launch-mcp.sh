#!/usr/bin/env bash
set -uo pipefail

# launch-mcp.sh — Self-healing launcher for forge-state-mcp.
#
# Called by Claude Code as the MCP server command (see .mcp.json).
# Delegates version checking and installation to setup.sh (which exits 0
# immediately if the correct version is already installed), then exec's the binary.
#
# Environment:
#   CLAUDE_PLUGIN_ROOT  — set by Claude Code to the plugin install directory

PLUGIN_ROOT="${CLAUDE_PLUGIN_ROOT:-}"
if [ -z "$PLUGIN_ROOT" ]; then
  echo "launch-mcp.sh: CLAUDE_PLUGIN_ROOT is not set" >&2
  exit 1
fi

BINARY="${PLUGIN_ROOT}/bin/forge-state-mcp"

# Delegate version check and installation to setup.sh.
# setup.sh exits 0 immediately if the correct version is already installed,
# so this is a no-op on the hot path and handles missing/stale binaries.
if ! bash "${PLUGIN_ROOT}/scripts/setup.sh" >&2; then
  echo "launch-mcp.sh: setup failed, exiting" >&2
  exit 1
fi

exec "$BINARY"

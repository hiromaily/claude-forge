#!/usr/bin/env bash
set -uo pipefail

# launch-mcp.sh — Self-healing launcher for forge-state-mcp.
#
# Called by Claude Code as the MCP server command (see .mcp.json).
# If the binary is missing (e.g. Setup hook did not fire on install),
# this script runs setup.sh to download/build it first, then exec's it.
#
# Environment:
#   CLAUDE_PLUGIN_ROOT  — set by Claude Code to the plugin install directory

PLUGIN_ROOT="${CLAUDE_PLUGIN_ROOT:-}"
if [ -z "$PLUGIN_ROOT" ]; then
  echo "launch-mcp.sh: CLAUDE_PLUGIN_ROOT is not set" >&2
  exit 1
fi

BINARY="${PLUGIN_ROOT}/bin/forge-state-mcp"

if [ ! -x "$BINARY" ]; then
  echo "launch-mcp.sh: binary not found, running setup..." >&2
  bash "${PLUGIN_ROOT}/scripts/setup.sh" >&2
fi

exec "$BINARY"

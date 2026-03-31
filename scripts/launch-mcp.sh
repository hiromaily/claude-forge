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
BIN_DIR="${PLUGIN_ROOT}/bin"
MARKER="${BIN_DIR}/.installed-version"

# Detect expected version from plugin.json
EXPECTED_VERSION=""
if command -v jq >/dev/null 2>&1; then
  EXPECTED_VERSION="$(jq -r '.version // empty' "${PLUGIN_ROOT}/.claude-plugin/plugin.json" 2>/dev/null || true)"
fi
if [ -z "$EXPECTED_VERSION" ]; then
  EXPECTED_VERSION="$(grep -o '"version"[[:space:]]*:[[:space:]]*"[^"]*"' "${PLUGIN_ROOT}/.claude-plugin/plugin.json" 2>/dev/null \
    | head -1 | sed 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/' || true)"
fi

# Run setup if binary is missing or version is stale
INSTALLED_VERSION="$(cat "$MARKER" 2>/dev/null || true)"
if [ ! -x "$BINARY" ] || [ "$INSTALLED_VERSION" != "$EXPECTED_VERSION" ]; then
  echo "launch-mcp.sh: binary missing or version mismatch (installed=${INSTALLED_VERSION:-none}, expected=${EXPECTED_VERSION:-unknown}), running setup..." >&2
  bash "${PLUGIN_ROOT}/scripts/setup.sh" >&2
fi

exec "$BINARY"

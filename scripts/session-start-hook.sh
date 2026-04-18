#!/usr/bin/env bash
set -uo pipefail

# session-start-hook.sh — Display dashboard URL at session start.
#
# Called by Claude Code's SessionStart hook. Outputs JSON with:
#   - systemMessage: displayed in the user's terminal
#
# System prompt injection is handled by the __IMPORTANT MCP tool
# (see mcp-server/internal/handler/tools/important.go), so this hook only
# provides the terminal-visible message.
#
# The dashboard URL is derived from FORGE_EVENTS_PORT (default 8099).
# If the dashboard is not reachable, the URL is still shown (the MCP
# server may start after this hook runs).

PORT="${FORGE_EVENTS_PORT:-8099}"
URL="http://localhost:${PORT}/"

# Build the systemMessage — displayed in the user's terminal.
MSG="claude-forge dashboard: ${URL}"

if command -v jq >/dev/null 2>&1; then
  jq -n \
    --arg msg "$MSG" \
    '{systemMessage:$msg}'
else
  cat <<EOF
{
  "systemMessage": "${MSG}"
}
EOF
fi

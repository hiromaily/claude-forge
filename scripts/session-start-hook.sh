#!/usr/bin/env bash
set -uo pipefail

# session-start-hook.sh — Display dashboard URL at session start.
#
# Called by Claude Code's SessionStart hook. Outputs JSON with:
#   - hookSpecificOutput.additionalContext: injected into system prompt
#   - systemMessage: displayed in the user's terminal
#
# The dashboard URL is derived from FORGE_EVENTS_PORT (default 8099).
# If the dashboard is not reachable, the URL is still shown (the MCP
# server may start after this hook runs).

PORT="${FORGE_EVENTS_PORT:-8099}"
URL="http://localhost:${PORT}/"

# Build the systemMessage — displayed in the user's terminal.
MSG="claude-forge dashboard: ${URL}"

# Build the additionalContext — injected into the system prompt.
CONTEXT="claude-forge dashboard is available at ${URL}"

if command -v jq >/dev/null 2>&1; then
  jq -n \
    --arg context "$CONTEXT" \
    --arg msg "$MSG" \
    '{hookSpecificOutput:{hookEventName:"SessionStart",additionalContext:$context},systemMessage:$msg}'
else
  cat <<EOF
{
  "hookSpecificOutput": {
    "hookEventName": "SessionStart",
    "additionalContext": "${CONTEXT}"
  },
  "systemMessage": "${MSG}"
}
EOF
fi

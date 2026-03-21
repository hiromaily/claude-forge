#!/usr/bin/env bash
# post-agent-hook.sh — PostToolUse hook for Agent tool calls
#
# After an Agent tool completes, check if it was a claude-forge agent
# and provide additional context if the output appears empty or malformed.
#
# This is a SAFETY NET — the primary state updates come from SKILL.md orchestrator.
# This hook validates output quality and injects retry guidance if needed.

set -uo pipefail

# Fail-open: if jq is not installed, skip validation
command -v jq >/dev/null 2>&1 || exit 0

INPUT="$(cat)"

TOOL_NAME="$(echo "$INPUT" | jq -r '.tool_name // empty' 2>/dev/null || true)"
[ "$TOOL_NAME" = "Agent" ] || exit 0

# Find active pipeline workspace (most recently updated in_progress pipeline)
find_active_workspace() {
  local project_dir="${CLAUDE_PROJECT_DIR:-.}"
  local latest_file=""
  local latest_ts=""
  for state_file in "${project_dir}"/.specs/*/state.json; do
    [ -f "$state_file" ] || continue
    local status ts
    status="$(jq -r '.currentPhaseStatus // empty' "$state_file" 2>/dev/null || true)"
    if [ "$status" = "in_progress" ] && [ -n "$status" ]; then
      ts="$(jq -r '.timestamps.lastUpdated // ""' "$state_file" 2>/dev/null || true)"
      if [ -z "$latest_ts" ] || [[ "$ts" > "$latest_ts" ]]; then
        latest_ts="$ts"
        latest_file="$state_file"
      fi
    fi
  done
  [ -n "$latest_file" ] && dirname "$latest_file" && return 0
  return 1
}

WORKSPACE="$(find_active_workspace 2>/dev/null || true)"
[ -z "$WORKSPACE" ] && exit 0

STATE_FILE="${WORKSPACE}/state.json"
[ -f "$STATE_FILE" ] || exit 0

CURRENT_PHASE="$(jq -r '.currentPhase' "$STATE_FILE" 2>/dev/null || true)"

# Check agent output quality for phases that return artifacts
# PostToolUse stdin provides agent result in .tool_response field
TOOL_OUTPUT="$(echo "$INPUT" | jq -r '.tool_response // empty' 2>/dev/null || true)"

# Empty output check
if [ -z "$TOOL_OUTPUT" ] || [ ${#TOOL_OUTPUT} -lt 50 ]; then
  # Output JSON with additional context for the orchestrator
  jq -n \
    --arg phase "$CURRENT_PHASE" \
    '{
      "hookSpecificOutput": {
        "hookEventName": "PostToolUse",
        "additionalContext": ("WARNING: Agent output for " + $phase + " appears empty or too short (< 50 chars). Consider retrying this phase per Error Handling rules.")
      }
    }'
  exit 0
fi

# For review phases, check that output contains a verdict
case "$CURRENT_PHASE" in
  phase-3b|phase-4b)
    if ! echo "$TOOL_OUTPUT" | grep -qiE '(APPROVE|APPROVE_WITH_NOTES|REVISE)'; then
      jq -n \
        --arg phase "$CURRENT_PHASE" \
        '{
          "hookSpecificOutput": {
            "hookEventName": "PostToolUse",
            "additionalContext": ("WARNING: Review agent output for " + $phase + " does not contain APPROVE, APPROVE_WITH_NOTES, or REVISE verdict. The output may be malformed.")
          }
        }'
      exit 0
    fi
    ;;
  phase-6)
    if ! echo "$TOOL_OUTPUT" | grep -qiE '(PASS|FAIL)'; then
      jq -n '{
        "hookSpecificOutput": {
          "hookEventName": "PostToolUse",
          "additionalContext": "WARNING: Implementation review output does not contain PASS or FAIL verdict. The output may be malformed."
        }
      }'
      exit 0
    fi
    ;;
esac

exit 0

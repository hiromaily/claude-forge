#!/usr/bin/env bash
# stop-hook.sh — Stop hook for claude-forge
#
# Prevents the pipeline from stopping before completion.
# If an active pipeline exists and summary.md is not written yet,
# inject a warning to continue working.
#
# Also plays a notification sound when the pipeline pauses for human input.

set -uo pipefail

# Play macOS notification sound (non-blocking) when pipeline needs human attention.
# Override sound: export DEV_PIPELINE_NOTIFY_SOUND="/path/to/sound.aiff"
# Disable:        export DEV_PIPELINE_NOTIFY_SOUND=off
notify_human() {
  local sound="${DEV_PIPELINE_NOTIFY_SOUND:-/System/Library/Sounds/Glass.aiff}"
  [ "$sound" = "off" ] && return 0
  [ -f "$sound" ] && ( afplay "$sound" &>/dev/null & )
  return 0
}

# Fail-open: if jq is not installed, allow stop
command -v jq >/dev/null 2>&1 || exit 0

INPUT="$(cat)"

# Find active pipeline workspace (most recently updated)
find_active_workspace() {
  local project_dir="${CLAUDE_PROJECT_DIR:-.}"
  local latest_file=""
  local latest_ts=""
  for state_file in "${project_dir}"/.specs/*/state.json; do
    [ -f "$state_file" ] || continue
    local status ts notify_flag
    {
      read -r status
      read -r notify_flag
    } < <(jq -r '(.currentPhaseStatus // ""), (.notifyOnStop // false)' "$state_file" 2>/dev/null)
    status=${status:-""}
    notify_flag=${notify_flag:-false}
    if { [ "$status" != "completed" ] && [ "$status" != "abandoned" ] && [ -n "$status" ]; } || \
       [ "$notify_flag" = "true" ]; then
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
CURRENT_STATUS="$(jq -r '.currentPhaseStatus' "$STATE_FILE" 2>/dev/null || true)"
NOTIFY_ON_STOP="$(jq -r '.notifyOnStop // false' "$STATE_FILE" 2>/dev/null || true)"

case "$CURRENT_STATUS" in
  completed)
    if [ "$NOTIFY_ON_STOP" = "true" ]; then
      notify_human
      jq '.notifyOnStop = false' "$STATE_FILE" > "${STATE_FILE}.tmp" && mv "${STATE_FILE}.tmp" "$STATE_FILE"
    fi
    exit 0
    ;;
  abandoned)
    exit 0
    ;;
  awaiting_human)
    notify_human
    exit 0
    ;;
esac

# Pipeline is active and not at a safe stop point — block with exit 2
COMPLETED_PHASES="$(jq -r '.completedPhases | join(", ")' "$STATE_FILE" 2>/dev/null || echo "none")"

echo "Claude-forge is still active at ${CURRENT_PHASE} (${CURRENT_STATUS}). Completed: [${COMPLETED_PHASES}]. Workspace: ${WORKSPACE}. Continue the pipeline or explicitly abandon it before stopping." >&2
exit 2

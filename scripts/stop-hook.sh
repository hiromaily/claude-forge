#!/usr/bin/env bash
# stop-hook.sh — Stop hook for claude-forge
#
# Prevents the pipeline from stopping before completion.
# If an active pipeline exists and summary.md is not written yet,
# inject a warning to continue working.

set -uo pipefail

# Fail-open: if jq is not installed, allow stop
command -v jq >/dev/null 2>&1 || exit 0

INPUT="$(cat)"

# Find active pipeline workspace (most recently updated)
find_active_workspace() {
  local project_dir="${CLAUDE_PROJECT_DIR:-.}"
  local latest_file="" latest_ts=""
  for state_file in "${project_dir}"/.specs/*/state.json; do
    [ -f "$state_file" ] || continue
    local status ts
    { read -r status; read -r ts; } < <(jq -r '(.currentPhaseStatus // ""), (.timestamps.lastUpdated // "")' "$state_file" 2>/dev/null)
    if [ "$status" != "completed" ] && [ "$status" != "abandoned" ] && [ -n "$status" ]; then
      if [ -z "$latest_ts" ] || [[ "$ts" > "$latest_ts" ]]; then
        latest_ts="$ts"; latest_file="$state_file"
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

case "$CURRENT_STATUS" in
  completed)
    exit 0
    ;;
  abandoned)
    exit 0
    ;;
  awaiting_human)
    exit 0
    ;;
esac

# Pipeline is active and not at a safe stop point — block with exit 2
COMPLETED_PHASES="$(jq -r '.completedPhases | join(", ")' "$STATE_FILE" 2>/dev/null || echo "none")"

echo "Claude-forge is still active at ${CURRENT_PHASE} (${CURRENT_STATUS}). Completed: [${COMPLETED_PHASES}]. Workspace: ${WORKSPACE}. Continue the pipeline or run: bash scripts/state-manager.sh abandon ${WORKSPACE}" >&2
exit 2

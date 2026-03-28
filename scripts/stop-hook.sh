#!/usr/bin/env bash
# stop-hook.sh — Stop hook for claude-forge
# Blocks stop when an active pipeline is not at a safe stop point.
# Exit 0 = allow, Exit 2 + stderr = block. Fail-open on missing jq or parse errors.
set -uo pipefail
command -v jq >/dev/null 2>&1 || exit 0
# Source shared helpers; returning 1 means "no active workspace" (safe fail-open default).
_COMMON="${BASH_SOURCE%/*}/common.sh"
# shellcheck source=scripts/common.sh
if [ -f "$_COMMON" ]; then source "$_COMMON"; else find_active_workspace() { return 1; }; fi
WORKSPACE="$(find_active_workspace 2>/dev/null || true)"
[ -z "$WORKSPACE" ] && exit 0
STATE_FILE="${WORKSPACE}/state.json"
[ -f "$STATE_FILE" ] || exit 0
CURRENT_PHASE="$(jq -r '.currentPhase' "$STATE_FILE" 2>/dev/null || true)"
CURRENT_STATUS="$(jq -r '.currentPhaseStatus' "$STATE_FILE" 2>/dev/null || true)"
case "$CURRENT_STATUS" in
  completed|abandoned|awaiting_human) exit 0 ;;
esac
echo "Claude-forge is still active at ${CURRENT_PHASE} (${CURRENT_STATUS}). Workspace: ${WORKSPACE}. Continue the pipeline or run: bash scripts/state-manager.sh abandon ${WORKSPACE}" >&2
exit 2

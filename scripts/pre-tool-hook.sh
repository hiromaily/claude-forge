#!/usr/bin/env bash
# pre-tool-hook.sh — PreToolUse hook for claude-forge
# Rules: 1 (read-only phase-1/2), 2 (no git commit parallel phase-5), 5 (no checkout main/master)
# Exit 0 = allow, Exit 2 + stderr = block. Fail-open on missing jq or parse errors.
set -uo pipefail
command -v jq >/dev/null 2>&1 || exit 0
# Source shared helpers; returning 1 means "no active workspace" (safe fail-open default).
_COMMON="${BASH_SOURCE%/*}/common.sh"
# shellcheck source=scripts/common.sh
if [ -f "$_COMMON" ]; then source "$_COMMON"; else find_active_workspace() { return 1; }; fi
INPUT="$(cat)"
TOOL_NAME="$(echo "$INPUT" | jq -r '.tool_name // empty' 2>/dev/null || true)"
[ -z "$TOOL_NAME" ] && exit 0
WORKSPACE="$(find_active_workspace 2>/dev/null || true)"
[ -z "$WORKSPACE" ] && exit 0
STATE_FILE="${WORKSPACE}/state.json"
[ -f "$STATE_FILE" ] || exit 0
CURRENT_PHASE="$(jq -r '.currentPhase' "$STATE_FILE" 2>/dev/null || true)"
CURRENT_STATUS="$(jq -r '.currentPhaseStatus' "$STATE_FILE" 2>/dev/null || true)"
# Rule 1: Read-only enforcement for Phase 1-2
if [ "$CURRENT_PHASE" = "phase-1" ] || [ "$CURRENT_PHASE" = "phase-2" ]; then
  if [ "$CURRENT_STATUS" = "in_progress" ]; then
    case "$TOOL_NAME" in
      Edit|Write)
        TARGET_FILE="$(echo "$INPUT" | jq -r '.tool_input.file_path // empty' 2>/dev/null || true)"
        if [ -n "$TARGET_FILE" ]; then
          case "$TARGET_FILE" in
            "${WORKSPACE}"*) ;;
            *) echo "Phase 1-2 is read-only. File modifications to source code are blocked during analysis/investigation." >&2; exit 2 ;;
          esac
        fi ;;
    esac
  fi
fi
# Rules 2 and 5: Bash command checks
if [ "$TOOL_NAME" = "Bash" ]; then
  COMMAND="$(echo "$INPUT" | jq -r '.tool_input.command // empty' 2>/dev/null || true)"
  # Rule 2: Block git commit during parallel Phase 5
  if [ "$CURRENT_PHASE" = "phase-5" ] && [ "$CURRENT_STATUS" = "in_progress" ]; then
    if echo "$COMMAND" | grep -qE '\bgit\b.*\bcommit\b' && ! echo "$COMMAND" | grep -qE '\bgit\b.*\bcommit-tree\b'; then
      HAS_PARALLEL_ACTIVE="$(jq -r '[.tasks | to_entries[] | select(.value.executionMode == "parallel" and .value.implStatus == "in_progress")] | length' "$STATE_FILE" 2>/dev/null | tr -d '[:space:]')"
      if [ "${HAS_PARALLEL_ACTIVE:-0}" -gt 0 ]; then
        echo "git commit is blocked during parallel task execution. The orchestrator will batch-commit after all parallel tasks complete." >&2; exit 2
      fi
    fi
  fi
  # Rule 5: Block git checkout/switch to main or master during active pipeline
  STRIPPED_CMD="$(echo "$COMMAND" | sed -E "s/'[^']*'//g; s/\"[^\"]*\"//g")"
  if echo "$STRIPPED_CMD" | grep -qE '\bgit\b.*(checkout|switch).*\b(main|master)\b'; then
    echo "BLOCKED: git checkout/switch to main or master is not allowed during an active pipeline. Stay on the feature branch." >&2; exit 2
  fi
fi
exit 0

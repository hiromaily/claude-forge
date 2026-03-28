#!/usr/bin/env bash
# common.sh — Shared helpers for pre-tool-hook.sh and stop-hook.sh
#
# IMPORTANT: post-agent-hook.sh uses a DIFFERENT find_active_workspace predicate
# (status == "in_progress" only) and intentionally does NOT source this file.
# See the INTENTIONAL DIVERGENCE comment in post-agent-hook.sh.

# find_active_workspace — locate the most recently updated pipeline workspace
# whose currentPhaseStatus is not "completed" or "abandoned".
# Accepted statuses: in_progress, pending, awaiting_human.
#
# Outputs the workspace directory path on stdout and returns 0 on success;
# returns 1 when no active workspace is found.
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

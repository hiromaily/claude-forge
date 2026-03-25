#!/usr/bin/env bash
# post-bash-hook.sh — PostToolUse hook for Bash tool calls
#
# When `phase-complete {workspace} post-to-source` is detected, automatically
# amends the branch's last commit to include state.json and summary.md.
#
# This provides deterministic enforcement of the final-commit step that the
# LLM orchestrator previously had to remember to run. The hook fires immediately
# after the Bash tool completes, with no LLM involvement required.
#
# Exit semantics (PostToolUse):
#   exit 0  — allow (always; this hook never blocks)
#   JSON output with hookSpecificOutput — inject context into the conversation
#
# Skip conditions (all exit 0):
#   - jq not installed
#   - Not a Bash tool call
#   - Command does not contain state-manager.sh phase-complete post-to-source
#   - taskType is "investigation" (no feature branch exists)
#   - summary.md does not exist
#   - state.json and summary.md are already committed (nothing to do)
#   - git command fails (fail-open)

set -uo pipefail

command -v jq  >/dev/null 2>&1 || exit 0
command -v git >/dev/null 2>&1 || exit 0

INPUT="$(cat)"

TOOL_NAME="$(echo "$INPUT" | jq -r '.tool_name // empty' 2>/dev/null || true)"
[ "$TOOL_NAME" = "Bash" ] || exit 0

COMMAND="$(echo "$INPUT" | jq -r '.tool_input.command // empty' 2>/dev/null || true)"
[ -n "$COMMAND" ] || exit 0

# Only act on phase-complete ... post-to-source
echo "$COMMAND" | grep -qF 'state-manager.sh' || exit 0
PC_MATCH="$(echo "$COMMAND" | grep -oE 'phase-complete[[:space:]]+[^[:space:]]+[[:space:]]+post-to-source' | head -1 || true)"
[ -n "$PC_MATCH" ] || exit 0

# Extract and resolve workspace path
PC_WS="$(echo "$PC_MATCH" | awk '{print $2}')"
if [[ "$PC_WS" != /* ]]; then
  PC_WS="${CLAUDE_PROJECT_DIR:-.}/${PC_WS}"
fi

STATE_FILE="${PC_WS}/state.json"
[ -f "$STATE_FILE" ] || exit 0

# Skip for investigation task type (no feature branch)
TASK_TYPE="$(jq -r '.taskType // empty' "$STATE_FILE" 2>/dev/null || true)"
[ "$TASK_TYPE" = "investigation" ] && exit 0

SUMMARY_FILE="${PC_WS}/summary.md"
[ -f "$SUMMARY_FILE" ] || exit 0

# Run git commands from the project root
PROJECT_DIR="${CLAUDE_PROJECT_DIR:-.}"
cd "$PROJECT_DIR" || exit 0

# Only proceed if there are uncommitted changes to these two files
STATUS_OUT="$(git status --porcelain "$SUMMARY_FILE" "$STATE_FILE" 2>/dev/null || true)"
[ -n "$STATUS_OUT" ] || exit 0

# Amend the last commit to include the final state
git add "$SUMMARY_FILE" "$STATE_FILE" 2>/dev/null || exit 0
if ! git commit --amend --no-edit 2>/dev/null; then
  jq -n '{
    "hookSpecificOutput": {
      "hookEventName": "PostToolUse",
      "additionalContext": "WARNING: post-bash-hook could not amend commit for state.json/summary.md. Amend and push manually."
    }
  }'
  exit 0
fi

# Push (non-fatal — remote branch may not exist yet in edge cases)
PUSH_ERR="$(git push --force-with-lease 2>&1 || true)"

if echo "$PUSH_ERR" | grep -qi 'error\|rejected\|failed'; then
  jq -n \
    --arg err "$PUSH_ERR" \
    '{
      "hookSpecificOutput": {
        "hookEventName": "PostToolUse",
        "additionalContext": ("Auto-committed state.json and summary.md, but push failed: " + $err + ". Run git push --force-with-lease manually.")
      }
    }'
else
  jq -n '{
    "hookSpecificOutput": {
      "hookEventName": "PostToolUse",
      "additionalContext": "Auto-committed state.json and summary.md to branch (pipeline finalized)."
    }
  }'
fi

exit 0

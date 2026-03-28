#!/usr/bin/env bash
# DEPRECATED: Use the mcp__forge-state__validate_input MCP tool instead.
# This script is kept as a fallback for non-MCP flows.
# BEHAVIORAL NOTE: This script also accepts workspace paths where state.json exists
# at the path. The MCP tool uses string-only detection: only paths containing ".specs/"
# are classified as workspace paths. An absolute workspace path that does not contain
# ".specs/" but has a valid state.json will be treated differently by the MCP tool.
# validate-input.sh — Deterministic input validation for claude-forge
#
# Usage: bash scripts/validate-input.sh <arguments>
#
# Validates that the pipeline input is minimally well-formed before
# any workspace creation or subagent spawning occurs.
#
# Exit codes:
#   0 = valid input, proceed
#   1 = invalid input (message on stderr)
#
# Checks:
#   1. Non-empty input
#   2. Minimum length after flag stripping (5 chars, unless URL/workspace path)
#   3. URL format validation (if input looks like a URL)
#
# Semantic validation (gibberish, non-dev tasks) is NOT handled here —
# that requires LLM judgment and is specified in SKILL.md.

set -uo pipefail

INPUT="${1:-}"

# --- Check 1: Empty input ---
STRIPPED="$(echo "$INPUT" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
if [ -z "$STRIPPED" ]; then
  echo "ERROR: No task description provided. Please provide a development task, GitHub Issue URL, or Jira Issue URL." >&2
  exit 1
fi

# --- Strip known flags to get the core task description ---
CORE="$STRIPPED"
# Remove --type=<value>
CORE="$(echo "$CORE" | sed -E 's/--type=[^[:space:]]+//g')"
# Remove --effort=<value>
CORE="$(echo "$CORE" | sed -E 's/--effort=[^[:space:]]+//g')"
# Remove bare flags
CORE="$(echo "$CORE" | sed -E 's/--auto($|[[:space:]])/ /g; s/--nopr($|[[:space:]])/ /g; s/--debug($|[[:space:]])/ /g')"
# Trim whitespace
CORE="$(echo "$CORE" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"

# --- Check: Only flags, no actual task ---
if [ -z "$CORE" ]; then
  echo "ERROR: Only flags provided, no task description. Please provide a development task after the flags." >&2
  exit 1
fi

# --- Classify input type ---
IS_URL=false
IS_WORKSPACE=false

if echo "$CORE" | grep -qE '^https?://'; then
  IS_URL=true
elif echo "$CORE" | grep -qE '\.specs/' || [ -f "${CORE}/state.json" ] 2>/dev/null; then
  IS_WORKSPACE=true
fi

# --- Check 2: Minimum length (skip for URLs and workspace paths) ---
if [ "$IS_URL" = false ] && [ "$IS_WORKSPACE" = false ]; then
  CORE_LEN="${#CORE}"
  if [ "$CORE_LEN" -lt 5 ]; then
    echo "ERROR: Task description too short (${CORE_LEN} chars). Please provide a more specific description (minimum 5 characters)." >&2
    exit 1
  fi
fi

# --- Check 3: URL format validation ---
if [ "$IS_URL" = true ]; then
  # GitHub Issue URL
  if echo "$CORE" | grep -qE '^https://github\.com/'; then
    if ! echo "$CORE" | grep -qE '^https://github\.com/[^/]+/[^/]+/issues/[0-9]+'; then
      echo "ERROR: Invalid GitHub URL format. Expected: https://github.com/{owner}/{repo}/issues/{number}" >&2
      exit 1
    fi
  # Jira Issue URL
  elif echo "$CORE" | grep -qE '^https://.*\.atlassian\.net/'; then
    if ! echo "$CORE" | grep -qE '^https://[^/]+\.atlassian\.net/browse/[A-Z]+-[0-9]+'; then
      echo "ERROR: Invalid Jira URL format. Expected: https://{org}.atlassian.net/browse/{KEY}-{number}" >&2
      exit 1
    fi
  # Unknown URL
  else
    echo "ERROR: Unrecognised URL format. Supported formats:" >&2
    echo "  - GitHub Issue: https://github.com/{owner}/{repo}/issues/{number}" >&2
    echo "  - Jira Issue:   https://{org}.atlassian.net/browse/{KEY}-{number}" >&2
    exit 1
  fi
fi

exit 0

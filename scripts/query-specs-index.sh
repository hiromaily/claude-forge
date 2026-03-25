#!/usr/bin/env bash
# query-specs-index.sh — Query .specs/index.json for past review feedback or impl patterns
#
# Usage: bash scripts/query-specs-index.sh <workspace> [task_type] [mode]
#
#   workspace   Path to the current pipeline workspace directory.
#               Index path is derived as $(dirname "$workspace")/index.json.
#   task_type   Optional (e.g. feature, bugfix). Exact match adds +2 to scoring.
#   mode        Optional. When "impl", queries for past implementation patterns instead of
#               review feedback. Any other value (or absent) uses review-feedback mode.
#
# Stdout:
#   Empty string when no matches or index absent.
#   Formatted markdown block when matches found:
#     ## Past Review Feedback (from similar pipelines)    [default mode]
#     ## Similar Past Implementations (from similar pipelines)  [impl mode]
#     ...
#
# Exit codes:
#   0 = always (even when index missing, no matches, or empty output)
#   1 = only on fatal programmer error (jq missing)

set -euo pipefail

WORKSPACE="${1:-}"
TASK_TYPE="${2:-}"
MODE="${3:-}"

# --- helpers ---

die() { echo "query-specs-index: $*" >&2; exit 1; }

require_jq() {
  command -v jq >/dev/null 2>&1 || die "jq is required but not installed"
}

# --- Guards ---

if [ -z "${WORKSPACE}" ]; then
  die "Usage: query-specs-index.sh <workspace> [task_type] [mode]"
fi

require_jq

INDEX_FILE="$(dirname "${WORKSPACE}")/index.json"

# Graceful no-op: index absent
if [ ! -f "${INDEX_FILE}" ]; then
  exit 0
fi

# Graceful no-op: empty array
entry_count="$(jq 'length' "${INDEX_FILE}" 2>/dev/null || echo "0")"
if [ "${entry_count}" -eq 0 ]; then
  exit 0
fi

# --- Extract keywords from request.md ---

REQUEST_FILE="${WORKSPACE}/request.md"
KEYWORDS_JSON="[]"

if [ -f "${REQUEST_FILE}" ]; then
  # Strip YAML frontmatter using same awk state machine as build-specs-index.sh
  request_body="$(awk '
    BEGIN { state=0 }
    /^---[[:space:]]*$/ {
      if (state == 0) { state=1; next }
      if (state == 1) { state=2; next }
    }
    state == 2 { print }
    state == 0 { print }
  ' "${REQUEST_FILE}")"

  # Collapse to single line, split on non-word chars, lowercase, filter length >= 4
  # Output as a JSON array for use with jq --argjson
  KEYWORDS_JSON="$(printf '%s' "${request_body}" \
    | tr '[:upper:]' '[:lower:]' \
    | tr -cs 'a-z0-9' '\n' \
    | awk 'length >= 4' \
    | sort -u \
    | jq -R '.' \
    | jq -s '.')"
fi

# --- impl mode: filter, sort, and emit implementation patterns ---

if [ "${MODE}" = "impl" ]; then
  # Scoring:
  #   +2 if taskType matches TASK_TYPE (exact, case-sensitive; skipped if TASK_TYPE is empty)
  #   +1 for each keyword that appears (case-insensitively) in entry.requestSummary
  # Filter: outcome == "completed" AND score >= 2 (no reviewFeedback requirement)
  # Sort: descending score, then descending timestamp
  # Take: top 2 entries

  MATCHED_JSON="$(jq -r \
    --argjson keywords "${KEYWORDS_JSON}" \
    --arg task_type "${TASK_TYPE}" \
    '
    [ .[] |
      . as $entry |
      (if ($task_type != "" and $entry.taskType == $task_type) then 2 else 0 end) as $type_score |
      ([$keywords[] |
         if ($entry.requestSummary | ascii_downcase | test(.; "i")) then 1 else 0 end
       ] | add // 0) as $kw_score |
      ($type_score + $kw_score) as $score |
      select($entry.outcome == "completed" and $score >= 2) |
      $entry + {score: $score}
    ] |
    sort_by([.score, .timestamp]) | reverse |
    .[:2]
    ' "${INDEX_FILE}" 2>/dev/null || echo "[]")"

  matched_count="$(printf '%s' "${MATCHED_JSON}" | jq 'length' 2>/dev/null || echo "0")"
  if [ "${matched_count}" -eq 0 ]; then
    exit 0
  fi

  # Build one bullet per implPatterns element, capped at 6 total bullets.
  # Use .implPatterns // [] to handle pre-existing entries that lack the field.
  # File names are joined with ", ".
  output="$(printf '%s' "${MATCHED_JSON}" | jq -r '
    [ .[] |
      . as $entry |
      $entry.specName as $spec_name |
      $entry.taskType as $task_type |
      ($entry.implPatterns // [])[] |
      . as $pattern |
      ($pattern.filesModified | join(", ")) as $files |
      "- **\($spec_name)** (\($task_type // "unknown")): \($pattern.taskTitle) — files: \($files)"
    ] | .[:6] | .[]
  ' 2>/dev/null || true)"

  if [ -z "${output}" ]; then
    exit 0
  fi

  printf '## Similar Past Implementations (from similar pipelines)\n\nThe following past pipelines implemented similar work. Use their file patterns as reference.\n\n%s\n' "${output}"
  exit 0
fi

# --- review-feedback mode (default) ---
#
# Scoring:
#   +2 if taskType matches TASK_TYPE (exact, case-sensitive; skipped if TASK_TYPE is empty)
#   +1 for each keyword that appears (case-insensitively) in entry.requestSummary
# Filter: reviewFeedback non-empty AND score >= 2
# Sort: descending score, then descending timestamp (ISO 8601 strings sort lexicographically)
# Take: first 3 entries

MATCHED_JSON="$(jq -r \
  --argjson keywords "${KEYWORDS_JSON}" \
  --arg task_type "${TASK_TYPE}" \
  '
  [ .[] |
    . as $entry |
    (if ($task_type != "" and $entry.taskType == $task_type) then 2 else 0 end) as $type_score |
    ([$keywords[] |
       if ($entry.requestSummary | ascii_downcase | test(.; "i")) then 1 else 0 end
     ] | add // 0) as $kw_score |
    ($type_score + $kw_score) as $score |
    select(($entry.reviewFeedback | length) > 0 and $score >= 2) |
    $entry + {score: $score}
  ] |
  sort_by([.score, .timestamp]) | reverse |
  .[:3]
  ' "${INDEX_FILE}" 2>/dev/null || echo "[]")"

# --- Format output ---

bullet_count="$(printf '%s' "${MATCHED_JSON}" | jq 'length' 2>/dev/null || echo "0")"
if [ "${bullet_count}" -eq 0 ]; then
  exit 0
fi

# Build flat bulleted list from all findings in all reviewFeedback entries across top-3 matches.
# Bullets are emitted up to a total of 3 across all entries and feedback sources.
output="$(printf '%s' "${MATCHED_JSON}" | jq -r '
  [ .[] |
    . as $entry |
    $entry.specName as $spec_name |
    $entry.reviewFeedback[] |
    . as $fb |
    $fb.source as $source |
    $fb.findings[] |
    "- **[\($source)]** \(.) _(from: \($spec_name))_"
  ] | .[:3] | .[]
' 2>/dev/null || true)"

if [ -z "${output}" ]; then
  exit 0
fi

printf '## Past Review Feedback (from similar pipelines)\n\nThe following issues were flagged in REVISE reviews for similar past work. Avoid repeating them.\n\n%s\n' "${output}"

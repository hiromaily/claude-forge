#!/usr/bin/env bash
# build-specs-index.sh — Scan .specs/ directories and build .specs/index.json
#
# Usage: bash scripts/build-specs-index.sh [specs_dir]
#
#   specs_dir  Optional. Path to the .specs/ directory. Defaults to
#              <script_dir>/../.specs resolved relative to this script.
#
# Output: writes .specs/index.json (JSON array, one object per workspace)
#
# Exit codes:
#   0 = success (index.json written)
#   1 = fatal error (jq not installed, etc.)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SPECS_DIR="${1:-${SCRIPT_DIR}/../.specs}"
# Resolve to absolute path
SPECS_DIR="$(cd "${SPECS_DIR}" && pwd)"

# --- helpers ---

die() { echo "build-specs-index: $*" >&2; exit 1; }

require_jq() {
  command -v jq >/dev/null 2>&1 || die "jq is required but not installed"
}

# extract_request_summary <workspace_dir>
# Reads request.md, skips YAML frontmatter (--- ... ---), returns first 200 chars of body.
extract_request_summary() {
  local dir="$1"
  local req_file="${dir}/request.md"

  if [ ! -f "${req_file}" ]; then
    echo ""
    return
  fi

  # Use awk to skip YAML frontmatter delimited by --- on its own line.
  # State machine: before first ---, between two ---, after second ---.
  # Only output lines after the second ---.
  local body
  body="$(awk '
    BEGIN { state=0 }
    /^---[[:space:]]*$/ {
      if (state == 0) { state=1; next }
      if (state == 1) { state=2; next }
    }
    state == 2 { print }
    state == 0 { print }
  ' "${req_file}")"

  # Collapse to a single line (normalize whitespace runs) and take first 200 chars
  local summary
  summary="$(printf '%s' "${body}" | tr '\n' ' ' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//' | sed 's/[[:space:]]\{2,\}/ /g')"
  printf '%s' "${summary:0:200}"
}

# extract_review_feedback <workspace_dir>
# Returns a JSON array of REVISE-verdict feedback objects from review-design.md and review-tasks.md.
extract_review_feedback() {
  local dir="$1"
  local entries=""

  for source_key in "review-design" "review-tasks"; do
    local review_file="${dir}/${source_key}.md"
    if [ ! -f "${review_file}" ]; then
      continue
    fi

    # Detect verdict — match APPROVE(_WITH_NOTES)? or REVISE in the file
    local verdict
    verdict="$(grep -oE 'APPROVE(_WITH_NOTES)?|REVISE' "${review_file}" 2>/dev/null | head -1 || true)"

    if [ "${verdict}" != "REVISE" ]; then
      continue
    fi

    # Extract findings: **N. [CRITICAL] Title** or **N. [MINOR] Title**
    local findings_json
    findings_json="$(grep -oE '\*\*[0-9]+\. \[(CRITICAL|MINOR)\][^*]+\*\*' "${review_file}" 2>/dev/null \
      | sed 's/^\*\*//;s/\*\*$//' \
      | jq -R '.' \
      | jq -s '.' || echo '[]')"

    local entry
    entry="$(jq -n \
      --arg source "${source_key}" \
      --arg verdict "REVISE" \
      --argjson findings "${findings_json}" \
      '{source: $source, verdict: $verdict, findings: $findings}')"

    if [ -z "${entries}" ]; then
      entries="${entry}"
    else
      entries="${entries}"$'\n'"${entry}"
    fi
  done

  if [ -z "${entries}" ]; then
    echo "[]"
  else
    printf '%s\n' "${entries}" | jq -s '.'
  fi
}

# extract_impl_outcomes <workspace_dir>
# Returns a JSON array of impl outcome objects from review-[0-9]*.md files.
# NOTE: PASS_WITH_NOTES must appear before PASS in the alternation — ERE takes the leftmost match.
# Reversing these would incorrectly tokenise PASS_WITH_NOTES as PASS. Do NOT reorder.
extract_impl_outcomes() {
  local dir="$1"
  local entries=""

  # Glob: review-[0-9]*.md — digit anchor excludes review-design.md, review-tasks.md, comprehensive-review.md
  for review_file in "${dir}"/review-[0-9]*.md; do
    if [ ! -f "${review_file}" ]; then
      continue
    fi

    local basename_file
    basename_file="$(basename "${review_file}")"

    # Detect verdict — PASS_WITH_NOTES must come before PASS (leftmost match wins in ERE)
    local verdict
    verdict="$(grep -oE 'PASS_WITH_NOTES|PASS|FAIL' "${review_file}" 2>/dev/null | head -1 || true)"

    if [ -z "${verdict}" ]; then
      continue
    fi

    local entry
    entry="$(jq -n \
      --arg reviewFile "${basename_file}" \
      --arg verdict "${verdict}" \
      '{reviewFile: $reviewFile, verdict: $verdict}')"

    if [ -z "${entries}" ]; then
      entries="${entry}"
    else
      entries="${entries}"$'\n'"${entry}"
    fi
  done

  if [ -z "${entries}" ]; then
    echo "[]"
  else
    printf '%s\n' "${entries}" | jq -s '.'
  fi
}

# derive_outcome <state_json_content>
# Maps currentPhase/currentPhaseStatus to a canonical outcome string.
# Returns: completed | in_progress | abandoned | failed | unknown
derive_outcome() {
  local state_json="$1"

  if [ -z "${state_json}" ]; then
    echo "unknown"
    return
  fi

  local phase status
  read -r phase status <<< "$(printf '%s' "${state_json}" | jq -r '[.currentPhase // "", .currentPhaseStatus // ""] | @tsv' 2>/dev/null || echo "	")"

  if [ "${status}" = "abandoned" ]; then
    echo "abandoned"
  elif [ "${status}" = "failed" ]; then
    echo "failed"
  elif [ "${phase}" = "post-to-source" ] && [ "${status}" = "completed" ]; then
    echo "completed"
  elif [ "${phase}" = "completed" ] && [ "${status}" = "completed" ]; then
    # Treat the 'completed' pseudo-phase as completed outcome
    echo "completed"
  elif [ -n "${phase}" ]; then
    echo "in_progress"
  else
    echo "unknown"
  fi
}

# build_entry <workspace_dir>
# Assembles one JSON record for a workspace directory.
build_entry() {
  local dir="$1"
  local dir_basename
  dir_basename="$(basename "${dir}")"

  # Read state.json if present
  local state_json=""
  local state_file="${dir}/state.json"
  if [ -f "${state_file}" ]; then
    state_json="$(cat "${state_file}" 2>/dev/null || true)"
  fi

  # Extract fields from state.json with fallbacks
  local spec_name timestamp task_type
  if [ -n "${state_json}" ]; then
    spec_name="$(printf '%s' "${state_json}" | jq -r '.specName // empty' 2>/dev/null || true)"
    timestamp="$(printf '%s' "${state_json}" | jq -r '.timestamps.created // empty' 2>/dev/null || true)"
    task_type="$(printf '%s' "${state_json}" | jq -r '.taskType // empty' 2>/dev/null || true)"
  fi

  # Apply fallbacks
  spec_name="${spec_name:-${dir_basename}}"
  timestamp="${timestamp:-unknown}"

  # Derive outcome
  local outcome
  outcome="$(derive_outcome "${state_json}")"

  # Extract requestSummary (frontmatter stripped, ≤200 chars)
  local request_summary
  request_summary="$(extract_request_summary "${dir}")"

  # Extract reviewFeedback array
  local review_feedback
  review_feedback="$(extract_review_feedback "${dir}")"

  # Extract implOutcomes array
  local impl_outcomes
  impl_outcomes="$(extract_impl_outcomes "${dir}")"

  # Assemble JSON record
  jq -n \
    --arg specName "${spec_name}" \
    --arg timestamp "${timestamp}" \
    --argjson taskType "$([ -n "${task_type:-}" ] && printf '"%s"' "${task_type}" || echo 'null')" \
    --arg requestSummary "${request_summary}" \
    --argjson reviewFeedback "${review_feedback}" \
    --argjson implOutcomes "${impl_outcomes}" \
    --arg outcome "${outcome}" \
    '{
      specName: $specName,
      timestamp: $timestamp,
      taskType: $taskType,
      requestSummary: $requestSummary,
      reviewFeedback: $reviewFeedback,
      implOutcomes: $implOutcomes,
      outcome: $outcome
    }'
}

main() {
  require_jq

  local output_file="${SPECS_DIR}/index.json"
  local entries=""
  local count=0

  # Iterate over immediate subdirectories of SPECS_DIR
  for workspace_dir in "${SPECS_DIR}"/*/; do
    # Skip if glob didn't expand (no subdirs)
    [ -d "${workspace_dir}" ] || continue

    # Skip index.json itself if it appears as a "directory" (shouldn't happen, but guard)
    local bname
    bname="$(basename "${workspace_dir%/}")"
    [ "${bname}" = "index.json" ] && continue

    local entry
    entry="$(build_entry "${workspace_dir%/}")"

    if [ -z "${entries}" ]; then
      entries="${entry}"
    else
      entries="${entries}"$'\n'"${entry}"
    fi
    count=$((count + 1))
  done

  if [ "${count}" -eq 0 ]; then
    printf '%s\n' "[]" > "${output_file}"
  else
    printf '%s\n' "${entries}" | jq -s '.' > "${output_file}"
  fi
}

main

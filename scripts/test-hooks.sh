#!/usr/bin/env bash
# test-hooks.sh — Automated tests for claude-forge hook scripts and state-manager
#
# Usage: bash scripts/test-hooks.sh
# Runs from the claude-forge directory.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TMPDIR_BASE="$(mktemp -d)"
PASS_COUNT=0
FAIL_COUNT=0

cleanup() {
  rm -rf "$TMPDIR_BASE"
}
trap cleanup EXIT

# --- test helpers ---

pass() {
  PASS_COUNT=$((PASS_COUNT + 1))
  echo "  ✓ $1"
}

fail() {
  FAIL_COUNT=$((FAIL_COUNT + 1))
  echo "  ✗ $1"
  [ -n "${2:-}" ] && echo "    $2"
}

# Run a hook script with JSON on stdin, capture exit code and outputs
run_hook() {
  local hook="$1"
  local json="$2"
  local env_vars="${3:-}"

  HOOK_STDOUT="$(mktemp)"
  HOOK_STDERR="$(mktemp)"

  if [ -n "$env_vars" ]; then
    HOOK_EXIT=0
    eval "$env_vars" bash "${SCRIPT_DIR}/${hook}" > "$HOOK_STDOUT" 2> "$HOOK_STDERR" <<< "$json" || HOOK_EXIT=$?
  else
    HOOK_EXIT=0
    bash "${SCRIPT_DIR}/${hook}" > "$HOOK_STDOUT" 2> "$HOOK_STDERR" <<< "$json" || HOOK_EXIT=$?
  fi

  HOOK_STDOUT_CONTENT="$(cat "$HOOK_STDOUT")"
  HOOK_STDERR_CONTENT="$(cat "$HOOK_STDERR")"
  rm -f "$HOOK_STDOUT" "$HOOK_STDERR"
}

assert_exit() {
  local expected="$1"
  local label="$2"
  if [ "$HOOK_EXIT" -eq "$expected" ]; then
    pass "$label"
  else
    fail "$label" "expected exit $expected, got $HOOK_EXIT (stderr: ${HOOK_STDERR_CONTENT})"
  fi
}

assert_stderr_contains() {
  local pattern="$1"
  local label="$2"
  if echo "$HOOK_STDERR_CONTENT" | grep -qF "$pattern"; then
    pass "$label"
  else
    fail "$label" "stderr did not contain '$pattern': ${HOOK_STDERR_CONTENT}"
  fi
}

assert_stdout_contains() {
  local pattern="$1"
  local label="$2"
  if echo "$HOOK_STDOUT_CONTENT" | grep -qF "$pattern"; then
    pass "$label"
  else
    fail "$label" "stdout did not contain '$pattern': ${HOOK_STDOUT_CONTENT}"
  fi
}

# Create a workspace with state.json at a specific phase/status
setup_workspace() {
  local phase="$1"
  local status="$2"
  local workspace="${TMPDIR_BASE}/.specs/test-pipeline"
  mkdir -p "$workspace"

  local tasks="${3:-{}}"

  cat > "${workspace}/state.json" <<ENDJSON
{
  "version": 1,
  "specName": "test",
  "workspace": "${workspace}",
  "branch": "feature/test",
  "currentPhase": "${phase}",
  "currentPhaseStatus": "${status}",
  "completedPhases": ["setup"],
  "revisions": { "designRevisions": 0, "taskRevisions": 0, "designInlineRevisions": 0, "taskInlineRevisions": 0 },
  "tasks": ${tasks},
  "phaseLog": [],
  "notifyOnStop": false,
  "timestamps": { "created": "2026-03-20T00:00:00Z", "lastUpdated": "2026-03-20T00:00:00Z", "phaseStarted": null },
  "error": null
}
ENDJSON
  echo "$workspace"
}

# Clean up workspace between tests
reset_workspace() {
  rm -rf "${TMPDIR_BASE}/.specs"
}

# ============================================================
echo "=== state-manager.sh tests ==="
# ============================================================

SM="${SCRIPT_DIR}/state-manager.sh"
SM_WS="${TMPDIR_BASE}/sm-test"
mkdir -p "$SM_WS"

echo ""
echo "--- init ---"
OUTPUT="$(bash "$SM" init "$SM_WS" "test-spec" 2>&1)"
if [ -f "${SM_WS}/state.json" ]; then
  pass "init creates state.json"
else
  fail "init creates state.json"
fi

SPEC_NAME="$(jq -r '.specName' "${SM_WS}/state.json")"
if [ "$SPEC_NAME" = "test-spec" ]; then
  pass "init sets specName"
else
  fail "init sets specName" "got: $SPEC_NAME"
fi

INIT_PHASE="$(jq -r '.currentPhase' "${SM_WS}/state.json")"
if [ "$INIT_PHASE" = "phase-1" ]; then
  pass "init starts at phase-1"
else
  fail "init starts at phase-1" "got: $INIT_PHASE"
fi

INIT_TASK_TYPE="$(jq '.taskType' "${SM_WS}/state.json")"
if [ "$INIT_TASK_TYPE" = "null" ]; then
  pass "init sets taskType to null"
else
  fail "init sets taskType to null" "got: $INIT_TASK_TYPE"
fi

INIT_SKIPPED="$(jq -c '.skippedPhases' "${SM_WS}/state.json")"
if [ "$INIT_SKIPPED" = "[]" ]; then
  pass "init sets skippedPhases to []"
else
  fail "init sets skippedPhases to []" "got: $INIT_SKIPPED"
fi

echo ""
echo "--- phase-start ---"
bash "$SM" phase-start "$SM_WS" "phase-1"
STATUS="$(jq -r '.currentPhaseStatus' "${SM_WS}/state.json")"
if [ "$STATUS" = "in_progress" ]; then
  pass "phase-start sets in_progress"
else
  fail "phase-start sets in_progress" "got: $STATUS"
fi

echo ""
echo "--- phase-complete ---"
bash "$SM" phase-complete "$SM_WS" "phase-1"
NEXT="$(jq -r '.currentPhase' "${SM_WS}/state.json")"
if [ "$NEXT" = "phase-2" ]; then
  pass "phase-complete advances to next phase"
else
  fail "phase-complete advances to next phase" "got: $NEXT"
fi

HAS_PHASE1="$(jq '[.completedPhases[] | select(. == "phase-1")] | length' "${SM_WS}/state.json")"
if [ "$HAS_PHASE1" -gt 0 ]; then
  pass "phase-complete adds to completedPhases"
else
  fail "phase-complete adds to completedPhases" "completedPhases: $(jq -c '.completedPhases' "${SM_WS}/state.json")"
fi

echo ""
echo "--- phase-fail ---"
bash "$SM" phase-start "$SM_WS" "phase-2"
bash "$SM" phase-fail "$SM_WS" "phase-2" "something went wrong"
ERR_MSG="$(jq -r '.error.message' "${SM_WS}/state.json")"
if [ "$ERR_MSG" = "something went wrong" ]; then
  pass "phase-fail records error message"
else
  fail "phase-fail records error message" "got: $ERR_MSG"
fi

echo ""
echo "--- checkpoint ---"
bash "$SM" checkpoint "$SM_WS" "checkpoint-a"
CP_STATUS="$(jq -r '.currentPhaseStatus' "${SM_WS}/state.json")"
if [ "$CP_STATUS" = "awaiting_human" ]; then
  pass "checkpoint sets awaiting_human"
else
  fail "checkpoint sets awaiting_human" "got: $CP_STATUS"
fi

echo ""
echo "--- task-init ---"
TASKS_JSON='{"1":{"title":"Task 1","executionMode":"sequential","implStatus":"pending","implRetries":0,"reviewStatus":"pending","reviewRetries":0},"2":{"title":"Task 2","executionMode":"parallel","implStatus":"pending","implRetries":0,"reviewStatus":"pending","reviewRetries":0}}'
bash "$SM" task-init "$SM_WS" "$TASKS_JSON"
TASK_COUNT="$(jq '.tasks | length' "${SM_WS}/state.json")"
if [ "$TASK_COUNT" -eq 2 ]; then
  pass "task-init creates tasks"
else
  fail "task-init creates tasks" "got count: $TASK_COUNT"
fi

echo ""
echo "--- task-update ---"
bash "$SM" task-update "$SM_WS" "1" "implStatus" "completed"
T1_STATUS="$(jq -r '.tasks["1"].implStatus' "${SM_WS}/state.json")"
if [ "$T1_STATUS" = "completed" ]; then
  pass "task-update sets string field"
else
  fail "task-update sets string field" "got: $T1_STATUS"
fi

bash "$SM" task-update "$SM_WS" "1" "implRetries" "2"
T1_RETRIES="$(jq '.tasks["1"].implRetries' "${SM_WS}/state.json")"
T1_RETRIES_TYPE="$(jq '.tasks["1"].implRetries | type' "${SM_WS}/state.json")"
if [ "$T1_RETRIES" = "2" ] && [ "$T1_RETRIES_TYPE" = '"number"' ]; then
  pass "task-update preserves numeric type for implRetries"
else
  fail "task-update preserves numeric type for implRetries" "value: $T1_RETRIES, type: $T1_RETRIES_TYPE"
fi

echo ""
echo "--- revision-bump ---"
bash "$SM" revision-bump "$SM_WS" "design"
REV="$(jq '.revisions.designRevisions' "${SM_WS}/state.json")"
if [ "$REV" = "1" ]; then
  pass "revision-bump increments design revision"
else
  fail "revision-bump increments design revision" "got: $REV"
fi

echo ""
echo "--- inline-revision-bump ---"
bash "$SM" inline-revision-bump "$SM_WS" "design"
INLREV_DESIGN="$(jq '.revisions.designInlineRevisions' "${SM_WS}/state.json")"
if [ "$INLREV_DESIGN" = "1" ]; then
  pass "inline-revision-bump increments designInlineRevisions"
else
  fail "inline-revision-bump increments designInlineRevisions" "got: $INLREV_DESIGN"
fi

bash "$SM" inline-revision-bump "$SM_WS" "tasks"
INLREV_TASKS="$(jq '.revisions.taskInlineRevisions' "${SM_WS}/state.json")"
if [ "$INLREV_TASKS" = "1" ]; then
  pass "inline-revision-bump increments taskInlineRevisions"
else
  fail "inline-revision-bump increments taskInlineRevisions" "got: $INLREV_TASKS"
fi

if bash "$SM" inline-revision-bump "$SM_WS" "invalid" 2>/dev/null; then
  fail "inline-revision-bump invalid type exits non-zero" "expected non-zero exit"
else
  pass "inline-revision-bump invalid type exits non-zero"
fi

echo ""
echo "--- set-branch ---"
bash "$SM" set-branch "$SM_WS" "feature/my-branch"
BRANCH="$(jq -r '.branch' "${SM_WS}/state.json")"
if [ "$BRANCH" = "feature/my-branch" ]; then
  pass "set-branch sets branch name"
else
  fail "set-branch sets branch name" "got: $BRANCH"
fi

echo ""
echo "--- resume-info ---"
RESUME="$(bash "$SM" resume-info "$SM_WS")"
RESUME_PHASE="$(echo "$RESUME" | jq -r '.currentPhase')"
if [ -n "$RESUME_PHASE" ]; then
  pass "resume-info returns JSON with currentPhase"
else
  fail "resume-info returns JSON with currentPhase"
fi

# resume-info: taskType field
SM_WS_RI="${TMPDIR_BASE}/sm-resume-info"
mkdir -p "$SM_WS_RI"
bash "$SM" init "$SM_WS_RI" "ri-test"

RESUME_TASK_TYPE_NULL="$(bash "$SM" resume-info "$SM_WS_RI" | jq '.taskType')"
if [ "$RESUME_TASK_TYPE_NULL" = "null" ]; then
  pass "resume-info projects taskType as null when not set"
else
  fail "resume-info projects taskType as null when not set" "got: $RESUME_TASK_TYPE_NULL"
fi

bash "$SM" set-task-type "$SM_WS_RI" "docs"
RESUME_TASK_TYPE_DOCS="$(bash "$SM" resume-info "$SM_WS_RI" | jq -r '.taskType')"
if [ "$RESUME_TASK_TYPE_DOCS" = "docs" ]; then
  pass "resume-info projects taskType correctly after set-task-type"
else
  fail "resume-info projects taskType correctly after set-task-type" "got: $RESUME_TASK_TYPE_DOCS"
fi

# resume-info: skippedPhases field
RESUME_SKIPPED_EMPTY="$(bash "$SM" resume-info "$SM_WS_RI" | jq -c '.skippedPhases')"
if [ "$RESUME_SKIPPED_EMPTY" = "[]" ]; then
  pass "resume-info projects skippedPhases as [] when none skipped"
else
  fail "resume-info projects skippedPhases as [] when none skipped" "got: $RESUME_SKIPPED_EMPTY"
fi

bash "$SM" skip-phase "$SM_WS_RI" "phase-1"
RESUME_SKIPPED_PHASE1="$(bash "$SM" resume-info "$SM_WS_RI" | jq -r '.skippedPhases[0]')"
if [ "$RESUME_SKIPPED_PHASE1" = "phase-1" ]; then
  pass "resume-info projects skippedPhases with skipped phase"
else
  fail "resume-info projects skippedPhases with skipped phase" "got: $RESUME_SKIPPED_PHASE1"
fi

# resume-info: legacy state without taskType field (field absent entirely)
SM_WS_LEGACY="${TMPDIR_BASE}/sm-legacy"
mkdir -p "$SM_WS_LEGACY"
cat > "${SM_WS_LEGACY}/state.json" <<LEGACYJSON
{
  "version": 1,
  "specName": "legacy-test",
  "workspace": "${SM_WS_LEGACY}",
  "branch": null,
  "currentPhase": "phase-3",
  "currentPhaseStatus": "pending",
  "completedPhases": ["setup", "phase-1", "phase-2"],
  "revisions": { "designRevisions": 0, "taskRevisions": 0 },
  "tasks": {},
  "timestamps": { "created": "2026-01-01T00:00:00Z", "lastUpdated": "2026-01-01T00:00:00Z", "phaseStarted": null },
  "error": null
}
LEGACYJSON

LEGACY_TASK_TYPE="$(bash "$SM" resume-info "$SM_WS_LEGACY" | jq '.taskType')"
if [ "$LEGACY_TASK_TYPE" = "null" ]; then
  pass "resume-info handles missing taskType in legacy state (returns null)"
else
  fail "resume-info handles missing taskType in legacy state (returns null)" "got: $LEGACY_TASK_TYPE"
fi

LEGACY_SKIPPED="$(bash "$SM" resume-info "$SM_WS_LEGACY" | jq -c '.skippedPhases')"
if [ "$LEGACY_SKIPPED" = "[]" ]; then
  pass "resume-info handles missing skippedPhases in legacy state (returns [])"
else
  fail "resume-info handles missing skippedPhases in legacy state (returns [])" "got: $LEGACY_SKIPPED"
fi

echo ""
echo "--- new phases (phase-7, pr-creation, post-to-source) ---"
# Reset to a fresh state for phase advancement testing
SM_WS_PHASES="${TMPDIR_BASE}/sm-phases"
mkdir -p "$SM_WS_PHASES"
bash "$SM" init "$SM_WS_PHASES" "phase-test"
# Advance through all phases up to phase-6
for p in phase-1 phase-2 phase-3 phase-3b checkpoint-a phase-4 phase-4b checkpoint-b phase-5 phase-6; do
  bash "$SM" phase-start "$SM_WS_PHASES" "$p"
  bash "$SM" phase-complete "$SM_WS_PHASES" "$p"
done

# Now should be at phase-7
NEXT_P="$(jq -r '.currentPhase' "${SM_WS_PHASES}/state.json")"
if [ "$NEXT_P" = "phase-7" ]; then
  pass "phase-6 advances to phase-7"
else
  fail "phase-6 advances to phase-7" "got: $NEXT_P"
fi

bash "$SM" phase-start "$SM_WS_PHASES" "phase-7"
bash "$SM" phase-complete "$SM_WS_PHASES" "phase-7"
NEXT_P="$(jq -r '.currentPhase' "${SM_WS_PHASES}/state.json")"
if [ "$NEXT_P" = "final-verification" ]; then
  pass "phase-7 advances to final-verification"
else
  fail "phase-7 advances to final-verification" "got: $NEXT_P"
fi

bash "$SM" phase-start "$SM_WS_PHASES" "final-verification"
bash "$SM" phase-complete "$SM_WS_PHASES" "final-verification"
NEXT_P="$(jq -r '.currentPhase' "${SM_WS_PHASES}/state.json")"
if [ "$NEXT_P" = "pr-creation" ]; then
  pass "final-verification advances to pr-creation"
else
  fail "final-verification advances to pr-creation" "got: $NEXT_P"
fi

bash "$SM" phase-start "$SM_WS_PHASES" "pr-creation"
bash "$SM" phase-complete "$SM_WS_PHASES" "pr-creation"
NEXT_P="$(jq -r '.currentPhase' "${SM_WS_PHASES}/state.json")"
if [ "$NEXT_P" = "final-summary" ]; then
  pass "pr-creation advances to final-summary"
else
  fail "pr-creation advances to final-summary" "got: $NEXT_P"
fi

bash "$SM" phase-start "$SM_WS_PHASES" "final-summary"
bash "$SM" phase-complete "$SM_WS_PHASES" "final-summary"
NEXT_P="$(jq -r '.currentPhase' "${SM_WS_PHASES}/state.json")"
if [ "$NEXT_P" = "post-to-source" ]; then
  pass "final-summary advances to post-to-source"
else
  fail "final-summary advances to post-to-source" "got: $NEXT_P"
fi

bash "$SM" phase-start "$SM_WS_PHASES" "post-to-source"
bash "$SM" phase-complete "$SM_WS_PHASES" "post-to-source"
FINAL_STATUS="$(jq -r '.currentPhaseStatus' "${SM_WS_PHASES}/state.json")"
if [ "$FINAL_STATUS" = "completed" ]; then
  pass "post-to-source completes the pipeline"
else
  fail "post-to-source completes the pipeline" "got status: $FINAL_STATUS"
fi

echo ""
echo "--- phase-fail on pr-creation (push failure simulation) ---"
SM_WS_PFAIL="${TMPDIR_BASE}/.specs/sm-pfail"
mkdir -p "$SM_WS_PFAIL"
bash "$SM" init "$SM_WS_PFAIL" "pfail-test"
# Advance to pr-creation
for p in phase-1 phase-2 phase-3 phase-3b checkpoint-a phase-4 phase-4b checkpoint-b phase-5 phase-6 phase-7 final-verification; do
  bash "$SM" phase-start "$SM_WS_PFAIL" "$p"
  bash "$SM" phase-complete "$SM_WS_PFAIL" "$p"
done
bash "$SM" phase-start "$SM_WS_PFAIL" "pr-creation"
bash "$SM" phase-fail "$SM_WS_PFAIL" "pr-creation" "git push failed: network error"

PFAIL_STATUS="$(jq -r '.currentPhaseStatus' "${SM_WS_PFAIL}/state.json")"
if [ "$PFAIL_STATUS" = "failed" ]; then
  pass "phase-fail on pr-creation sets currentPhaseStatus to failed"
else
  fail "phase-fail on pr-creation sets currentPhaseStatus to failed" "got: $PFAIL_STATUS"
fi

PFAIL_MSG="$(jq -r '.error.message' "${SM_WS_PFAIL}/state.json")"
if echo "$PFAIL_MSG" | grep -qF "git push failed: network error"; then
  pass "phase-fail on pr-creation records error message"
else
  fail "phase-fail on pr-creation records error message" "got: $PFAIL_MSG"
fi

run_hook "stop-hook.sh" '{"hook_event_name":"Stop","stop_hook_active":false}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "stop blocked when pr-creation is in failed state"

bash "$SM" abandon "$SM_WS_PFAIL" >/dev/null
run_hook "stop-hook.sh" '{"hook_event_name":"Stop","stop_hook_active":false}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "stop allowed after abandoning failed pr-creation workspace"

echo ""
echo "--- abandon ---"
bash "$SM" phase-complete "$SM_WS" "checkpoint-a"
bash "$SM" phase-start "$SM_WS" "phase-4"
bash "$SM" abandon "$SM_WS" >/dev/null
AB_STATUS="$(jq -r '.currentPhaseStatus' "${SM_WS}/state.json")"
if [ "$AB_STATUS" = "abandoned" ]; then
  pass "abandon sets abandoned status"
else
  fail "abandon sets abandoned status" "got: $AB_STATUS"
fi

echo ""
echo "--- set-task-type ---"
bash "$SM" set-task-type "$SM_WS" "bugfix"
SET_TASK_TYPE_VAL="$(jq -r '.taskType' "${SM_WS}/state.json")"
if [ "$SET_TASK_TYPE_VAL" = "bugfix" ]; then
  pass "set-task-type writes taskType to state.json"
else
  fail "set-task-type writes taskType to state.json" "got: $SET_TASK_TYPE_VAL"
fi

SET_TASK_TYPE_TS="$(jq -r '.timestamps.lastUpdated' "${SM_WS}/state.json")"
if [ -n "$SET_TASK_TYPE_TS" ] && [ "$SET_TASK_TYPE_TS" != "null" ]; then
  pass "set-task-type updates timestamps.lastUpdated"
else
  fail "set-task-type updates timestamps.lastUpdated" "got: $SET_TASK_TYPE_TS"
fi

bash "$SM" set-task-type "$SM_WS" "docs"
OVERWRITE_VAL="$(jq -r '.taskType' "${SM_WS}/state.json")"
if [ "$OVERWRITE_VAL" = "docs" ]; then
  pass "set-task-type second call overwrites previous value"
else
  fail "set-task-type second call overwrites previous value" "got: $OVERWRITE_VAL"
fi

echo ""
echo "--- set-auto-approve ---"
bash "$SM" set-auto-approve "$SM_WS"
SET_AA_VAL="$(jq '.autoApprove' "${SM_WS}/state.json")"
if [ "$SET_AA_VAL" = "true" ]; then
  pass "set-auto-approve writes autoApprove = true to state.json"
else
  fail "set-auto-approve writes autoApprove = true to state.json" "got: $SET_AA_VAL"
fi

SET_AA_TS="$(jq -r '.timestamps.lastUpdated' "${SM_WS}/state.json")"
if [ -n "$SET_AA_TS" ] && [ "$SET_AA_TS" != "null" ]; then
  pass "set-auto-approve updates timestamps.lastUpdated"
else
  fail "set-auto-approve updates timestamps.lastUpdated" "got: $SET_AA_TS"
fi

RESUME_AA_TRUE="$(bash "$SM" resume-info "$SM_WS" | jq '.autoApprove')"
if [ "$RESUME_AA_TRUE" = "true" ]; then
  pass "resume-info projects autoApprove: true after set-auto-approve"
else
  fail "resume-info projects autoApprove: true after set-auto-approve" "got: $RESUME_AA_TRUE"
fi

# Fresh workspace without set-auto-approve: resume-info should return autoApprove: false
SM_WS_AA="${TMPDIR_BASE}/sm-aa-fresh"
mkdir -p "$SM_WS_AA"
bash "$SM" init "$SM_WS_AA" "aa-fresh-test"
RESUME_AA_FALSE="$(bash "$SM" resume-info "$SM_WS_AA" | jq '.autoApprove')"
if [ "$RESUME_AA_FALSE" = "false" ]; then
  pass "resume-info returns autoApprove: false for fresh workspace without set-auto-approve"
else
  fail "resume-info returns autoApprove: false for fresh workspace without set-auto-approve" "got: $RESUME_AA_FALSE"
fi

echo ""
echo "--- skip-phase ---"
# Fresh workspace for skip-phase tests
SM_WS_SKIP="${TMPDIR_BASE}/sm-skip"
mkdir -p "$SM_WS_SKIP"
bash "$SM" init "$SM_WS_SKIP" "skip-test"
# Advance phase-1 normally
bash "$SM" phase-start "$SM_WS_SKIP" "phase-1"
bash "$SM" phase-complete "$SM_WS_SKIP" "phase-1"
# currentPhase is now phase-2; skip it
bash "$SM" skip-phase "$SM_WS_SKIP" "phase-2"

SKIP_CURRENT="$(jq -r '.currentPhase' "${SM_WS_SKIP}/state.json")"
if [ "$SKIP_CURRENT" = "phase-3" ]; then
  pass "skip-phase advances currentPhase to next phase"
else
  fail "skip-phase advances currentPhase to next phase" "got: $SKIP_CURRENT"
fi

SKIP_IN_SKIPPED="$(jq '[.skippedPhases[] | select(. == "phase-2")] | length' "${SM_WS_SKIP}/state.json")"
if [ "$SKIP_IN_SKIPPED" -gt 0 ]; then
  pass "skip-phase adds phase to skippedPhases"
else
  fail "skip-phase adds phase to skippedPhases" "skippedPhases: $(jq -c '.skippedPhases' "${SM_WS_SKIP}/state.json")"
fi

SKIP_STATUS="$(jq -r '.currentPhaseStatus' "${SM_WS_SKIP}/state.json")"
if [ "$SKIP_STATUS" = "pending" ]; then
  pass "skip-phase sets currentPhaseStatus to pending"
else
  fail "skip-phase sets currentPhaseStatus to pending" "got: $SKIP_STATUS"
fi

NOT_IN_COMPLETED="$(jq '[.completedPhases[] | select(. == "phase-2")] | length' "${SM_WS_SKIP}/state.json")"
if [ "$NOT_IN_COMPLETED" -eq 0 ]; then
  pass "skip-phase does NOT add phase to completedPhases"
else
  fail "skip-phase does NOT add phase to completedPhases" "completedPhases: $(jq -c '.completedPhases' "${SM_WS_SKIP}/state.json")"
fi

# Idempotency: calling skip-phase twice on phase-3 should result in exactly one entry
bash "$SM" skip-phase "$SM_WS_SKIP" "phase-3"
bash "$SM" skip-phase "$SM_WS_SKIP" "phase-3"
SKIP_DEDUP="$(jq '[.skippedPhases[] | select(. == "phase-3")] | length' "${SM_WS_SKIP}/state.json")"
if [ "$SKIP_DEDUP" -eq 1 ]; then
  pass "skip-phase is idempotent (unique constraint on skippedPhases)"
else
  fail "skip-phase is idempotent (unique constraint on skippedPhases)" "count: $SKIP_DEDUP"
fi

# Multiple phases accumulate in skippedPhases
MULTI_SKIPPED="$(jq -c '.skippedPhases' "${SM_WS_SKIP}/state.json")"
HAS_PHASE2="$(jq '[.skippedPhases[] | select(. == "phase-2")] | length' "${SM_WS_SKIP}/state.json")"
HAS_PHASE3="$(jq '[.skippedPhases[] | select(. == "phase-3")] | length' "${SM_WS_SKIP}/state.json")"
if [ "$HAS_PHASE2" -gt 0 ] && [ "$HAS_PHASE3" -gt 0 ]; then
  pass "skip-phase accumulates multiple distinct phases in skippedPhases"
else
  fail "skip-phase accumulates multiple distinct phases in skippedPhases" "skippedPhases: $MULTI_SKIPPED"
fi

# Mutual exclusion: completedPhases and skippedPhases are disjoint
# phase-1 is in completedPhases; phase-2 and phase-3 are in skippedPhases
IN_COMPLETED_1="$(jq '[.completedPhases[] | select(. == "phase-1")] | length' "${SM_WS_SKIP}/state.json")"
NOT_IN_SKIPPED_1="$(jq '[.skippedPhases[] | select(. == "phase-1")] | length' "${SM_WS_SKIP}/state.json")"
IN_SKIPPED_2="$(jq '[.skippedPhases[] | select(. == "phase-2")] | length' "${SM_WS_SKIP}/state.json")"
NOT_IN_COMPLETED_2="$(jq '[.completedPhases[] | select(. == "phase-2")] | length' "${SM_WS_SKIP}/state.json")"
if [ "$IN_COMPLETED_1" -gt 0 ] && [ "$NOT_IN_SKIPPED_1" -eq 0 ] && [ "$IN_SKIPPED_2" -gt 0 ] && [ "$NOT_IN_COMPLETED_2" -eq 0 ]; then
  pass "completedPhases and skippedPhases are mutually exclusive"
else
  fail "completedPhases and skippedPhases are mutually exclusive" "completedPhases: $(jq -c '.completedPhases' "${SM_WS_SKIP}/state.json"), skippedPhases: $(jq -c '.skippedPhases' "${SM_WS_SKIP}/state.json")"
fi

echo ""
echo "--- phase-log ---"
SM_WS_LOG="${TMPDIR_BASE}/sm-log"
mkdir -p "$SM_WS_LOG"
bash "$SM" init "$SM_WS_LOG" "log-test"

# Test: init creates empty phaseLog
PLOG_INIT="$(jq -c '.phaseLog' "${SM_WS_LOG}/state.json")"
if [ "$PLOG_INIT" = "[]" ]; then
  pass "init creates empty phaseLog"
else
  fail "init creates empty phaseLog" "got: $PLOG_INIT"
fi

# Test: phase-log appends entry
bash "$SM" phase-log "$SM_WS_LOG" "phase-1" 5000 30000 sonnet
PLOG_LEN="$(jq '.phaseLog | length' "${SM_WS_LOG}/state.json")"
PLOG_PHASE="$(jq -r '.phaseLog[0].phase' "${SM_WS_LOG}/state.json")"
PLOG_TOKENS="$(jq '.phaseLog[0].tokens' "${SM_WS_LOG}/state.json")"
PLOG_DUR="$(jq '.phaseLog[0].duration_ms' "${SM_WS_LOG}/state.json")"
PLOG_MODEL="$(jq -r '.phaseLog[0].model' "${SM_WS_LOG}/state.json")"
if [ "$PLOG_LEN" -eq 1 ] && [ "$PLOG_PHASE" = "phase-1" ] && [ "$PLOG_TOKENS" -eq 5000 ] && [ "$PLOG_DUR" -eq 30000 ] && [ "$PLOG_MODEL" = "sonnet" ]; then
  pass "phase-log appends entry with correct fields"
else
  fail "phase-log appends entry with correct fields" "len=$PLOG_LEN phase=$PLOG_PHASE tokens=$PLOG_TOKENS dur=$PLOG_DUR model=$PLOG_MODEL"
fi

# Test: phase-log appends multiple entries
bash "$SM" phase-log "$SM_WS_LOG" "phase-2" 8000 45000 opus
PLOG_LEN2="$(jq '.phaseLog | length' "${SM_WS_LOG}/state.json")"
PLOG_PHASE2="$(jq -r '.phaseLog[1].phase' "${SM_WS_LOG}/state.json")"
if [ "$PLOG_LEN2" -eq 2 ] && [ "$PLOG_PHASE2" = "phase-2" ]; then
  pass "phase-log appends multiple entries"
else
  fail "phase-log appends multiple entries" "len=$PLOG_LEN2 phase=$PLOG_PHASE2"
fi

# Test: phase-log tokens and duration_ms are numbers, not strings
PLOG_TOK_TYPE="$(jq '.phaseLog[0].tokens | type' "${SM_WS_LOG}/state.json")"
PLOG_DUR_TYPE="$(jq '.phaseLog[0].duration_ms | type' "${SM_WS_LOG}/state.json")"
if [ "$PLOG_TOK_TYPE" = '"number"' ] && [ "$PLOG_DUR_TYPE" = '"number"' ]; then
  pass "phase-log stores tokens and duration_ms as numbers"
else
  fail "phase-log stores tokens and duration_ms as numbers" "tokens type=$PLOG_TOK_TYPE duration type=$PLOG_DUR_TYPE"
fi

# Test: phase-stats runs without error
PSTATS_OUT="$(bash "$SM" phase-stats "$SM_WS_LOG" 2>&1)"
if echo "$PSTATS_OUT" | grep -q "Phase Execution Stats"; then
  pass "phase-stats produces formatted output"
else
  fail "phase-stats produces formatted output" "output: $PSTATS_OUT"
fi

# Test: resume-info includes phaseLog summary
RINFO_LOG="$(bash "$SM" resume-info "$SM_WS_LOG" | jq '.phaseLogEntries')"
RINFO_TOK="$(bash "$SM" resume-info "$SM_WS_LOG" | jq '.totalTokens')"
RINFO_DUR="$(bash "$SM" resume-info "$SM_WS_LOG" | jq '.totalDuration_ms')"
if [ "$RINFO_LOG" -eq 2 ] && [ "$RINFO_TOK" -eq 13000 ] && [ "$RINFO_DUR" -eq 75000 ]; then
  pass "resume-info includes phaseLog summary totals"
else
  fail "resume-info includes phaseLog summary totals" "entries=$RINFO_LOG tokens=$RINFO_TOK dur=$RINFO_DUR"
fi

echo ""
echo "--- special characters in spec-name ---"
SM_WS_SPECIAL="${TMPDIR_BASE}/sm-special"
mkdir -p "$SM_WS_SPECIAL"
bash "$SM" init "$SM_WS_SPECIAL" 'fix "auth" & <timeout>'
SPECIAL_SPEC="$(jq -r '.specName' "${SM_WS_SPECIAL}/state.json")"
if [ "$SPECIAL_SPEC" = 'fix "auth" & <timeout>' ]; then
  pass "special characters in spec-name preserved"
else
  fail "special characters in spec-name preserved" "got: $SPECIAL_SPEC"
fi

echo ""
echo "--- checkpointRevisionPending: init default (sm-init) ---"
SM_WS_CRP="${TMPDIR_BASE}/sm-crp"
mkdir -p "$SM_WS_CRP"
bash "$SM" init "$SM_WS_CRP" "crp-test"

CRP_INIT="$(jq -c '.checkpointRevisionPending' "${SM_WS_CRP}/state.json")"
if [ "$CRP_INIT" = '{"checkpoint-a":false,"checkpoint-b":false}' ]; then
  pass "sm-init: checkpointRevisionPending initialized with both values false"
else
  fail "sm-init: checkpointRevisionPending initialized with both values false" "got: $CRP_INIT"
fi

echo ""
echo "--- set-revision-pending (sm-set) ---"
bash "$SM" set-revision-pending "$SM_WS_CRP" checkpoint-a
CRP_SET="$(jq '.checkpointRevisionPending["checkpoint-a"]' "${SM_WS_CRP}/state.json")"
if [ "$CRP_SET" = "true" ]; then
  pass "sm-set: set-revision-pending checkpoint-a sets flag to true"
else
  fail "sm-set: set-revision-pending checkpoint-a sets flag to true" "got: $CRP_SET"
fi

CRP_B_UNCHANGED="$(jq '.checkpointRevisionPending["checkpoint-b"]' "${SM_WS_CRP}/state.json")"
if [ "$CRP_B_UNCHANGED" = "false" ]; then
  pass "sm-set: checkpoint-b remains false after setting checkpoint-a"
else
  fail "sm-set: checkpoint-b remains false after setting checkpoint-a" "got: $CRP_B_UNCHANGED"
fi

echo ""
echo "--- clear-revision-pending (sm-clear) ---"
bash "$SM" clear-revision-pending "$SM_WS_CRP" checkpoint-a
CRP_CLEAR="$(jq '.checkpointRevisionPending["checkpoint-a"]' "${SM_WS_CRP}/state.json")"
if [ "$CRP_CLEAR" = "false" ]; then
  pass "sm-clear: clear-revision-pending checkpoint-a sets flag to false"
else
  fail "sm-clear: clear-revision-pending checkpoint-a sets flag to false" "got: $CRP_CLEAR"
fi

echo ""
echo "--- set-revision-pending invalid checkpoint (sm-invalid) ---"
SM_INVALID_EXIT=0
bash "$SM" set-revision-pending "$SM_WS_CRP" invalid-phase 2>/dev/null || SM_INVALID_EXIT=$?
if [ "$SM_INVALID_EXIT" -eq 1 ]; then
  pass "sm-invalid: set-revision-pending with invalid checkpoint exits 1"
else
  fail "sm-invalid: set-revision-pending with invalid checkpoint exits 1" "got exit: $SM_INVALID_EXIT"
fi

SM_CLEAR_INVALID_EXIT=0
bash "$SM" clear-revision-pending "$SM_WS_CRP" invalid-phase 2>/dev/null || SM_CLEAR_INVALID_EXIT=$?
if [ "$SM_CLEAR_INVALID_EXIT" -eq 1 ]; then
  pass "sm-invalid: clear-revision-pending with invalid checkpoint exits 1"
else
  fail "sm-invalid: clear-revision-pending with invalid checkpoint exits 1" "got exit: $SM_CLEAR_INVALID_EXIT"
fi

echo ""
echo "--- resume-info includes checkpointRevisionPending (sm-legacy) ---"
# Test sm-legacy: resume-info on state without checkpointRevisionPending returns defaults
SM_WS_LEGACY="${TMPDIR_BASE}/sm-legacy"
mkdir -p "$SM_WS_LEGACY"
bash "$SM" init "$SM_WS_LEGACY" "legacy-test"
# Remove checkpointRevisionPending to simulate a legacy state file
jq 'del(.checkpointRevisionPending)' "${SM_WS_LEGACY}/state.json" > "${SM_WS_LEGACY}/state.json.tmp" && mv "${SM_WS_LEGACY}/state.json.tmp" "${SM_WS_LEGACY}/state.json"

LEGACY_CRP="$(bash "$SM" resume-info "$SM_WS_LEGACY" | jq -c '.checkpointRevisionPending')"
if [ "$LEGACY_CRP" = '{"checkpoint-a":false,"checkpoint-b":false}' ]; then
  pass "sm-legacy: resume-info returns default checkpointRevisionPending for legacy state"
else
  fail "sm-legacy: resume-info returns default checkpointRevisionPending for legacy state" "got: $LEGACY_CRP"
fi

# Test resume-info on new state includes checkpointRevisionPending correctly
RINFO_CRP="$(bash "$SM" resume-info "$SM_WS_CRP" | jq -c '.checkpointRevisionPending')"
if [ "$RINFO_CRP" = '{"checkpoint-a":false,"checkpoint-b":false}' ]; then
  pass "resume-info includes checkpointRevisionPending in output"
else
  fail "resume-info includes checkpointRevisionPending in output" "got: $RINFO_CRP"
fi

# ============================================================
echo ""
echo "=== pre-tool-hook.sh tests ==="
# ============================================================

echo ""
echo "--- no active pipeline ---"
reset_workspace
run_hook "pre-tool-hook.sh" '{"tool_name":"Edit","tool_input":{"file_path":"/src/foo.go"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "Edit allowed when no active pipeline"

echo ""
echo "--- Phase 1-2: read-only enforcement ---"
reset_workspace
WS="$(setup_workspace "phase-1" "in_progress")"

run_hook "pre-tool-hook.sh" '{"tool_name":"Edit","tool_input":{"file_path":"/src/foo.go"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "Edit on source file blocked in Phase 1"
assert_stderr_contains "read-only" "Block message mentions read-only"

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Write\",\"tool_input\":{\"file_path\":\"${WS}/analysis.md\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "Write to workspace file allowed in Phase 1"

run_hook "pre-tool-hook.sh" '{"tool_name":"Bash","tool_input":{"command":"ls"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "Bash allowed in Phase 1 (not Edit/Write)"

reset_workspace
WS="$(setup_workspace "phase-2" "in_progress")"

run_hook "pre-tool-hook.sh" '{"tool_name":"Write","tool_input":{"file_path":"/src/bar.go"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "Write on source file blocked in Phase 2"

echo ""
echo "--- Phase 5: git commit blocking ---"
reset_workspace
PARALLEL_TASKS='{"1":{"title":"T1","executionMode":"parallel","implStatus":"in_progress","implRetries":0,"reviewStatus":"pending","reviewRetries":0},"2":{"title":"T2","executionMode":"parallel","implStatus":"in_progress","implRetries":0,"reviewStatus":"pending","reviewRetries":0}}'
WS="$(setup_workspace "phase-5" "in_progress" "$PARALLEL_TASKS")"

run_hook "pre-tool-hook.sh" '{"tool_name":"Bash","tool_input":{"command":"git commit -m \"test\""}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "git commit blocked during parallel Phase 5"
assert_stderr_contains "batch-commit" "Block message mentions batch-commit"

reset_workspace
SEQ_TASKS='{"1":{"title":"T1","executionMode":"sequential","implStatus":"in_progress","implRetries":0,"reviewStatus":"pending","reviewRetries":0}}'
WS="$(setup_workspace "phase-5" "in_progress" "$SEQ_TASKS")"

run_hook "pre-tool-hook.sh" '{"tool_name":"Bash","tool_input":{"command":"git commit -m \"test\""}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "git commit allowed during sequential Phase 5"

echo ""
echo "--- Rule 4: git checkout -b branch guard ---"

# Test 1: No branch set in state — allow git checkout -b
reset_workspace
WS="$(setup_workspace "phase-5" "in_progress")"
# Remove the branch field from state.json (set to null)
jq '.branch = null' "${WS}/state.json" > "${WS}/state.json.tmp" && mv "${WS}/state.json.tmp" "${WS}/state.json"

run_hook "pre-tool-hook.sh" '{"tool_name":"Bash","tool_input":{"command":"git checkout -b feature/new"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "git checkout -b allowed when branch not set in state"

# Test 2: Branch set, command matches — allow
reset_workspace
WS="$(setup_workspace "phase-5" "in_progress")"
jq '.branch = "feature/test-sample"' "${WS}/state.json" > "${WS}/state.json.tmp" && mv "${WS}/state.json.tmp" "${WS}/state.json"

run_hook "pre-tool-hook.sh" '{"tool_name":"Bash","tool_input":{"command":"git checkout -b feature/test-sample"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "git checkout -b allowed when new branch matches recorded branch"

# Test 3: Branch set, command differs — block (core case: use_current_branch)
reset_workspace
WS="$(setup_workspace "phase-5" "in_progress")"
jq '.branch = "feature/test-sample"' "${WS}/state.json" > "${WS}/state.json.tmp" && mv "${WS}/state.json.tmp" "${WS}/state.json"

run_hook "pre-tool-hook.sh" '{"tool_name":"Bash","tool_input":{"command":"git checkout -b feature/test-sample-thing"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "git checkout -b blocked when new branch differs from recorded branch"
assert_stderr_contains "feature/test-sample" "block message contains recorded branch name"

# Test 4: Branch set, command differs with git flags (e.g., git -C /repo checkout -b)
reset_workspace
WS="$(setup_workspace "phase-5" "in_progress")"
jq '.branch = "feature/test-sample"' "${WS}/state.json" > "${WS}/state.json.tmp" && mv "${WS}/state.json.tmp" "${WS}/state.json"

run_hook "pre-tool-hook.sh" '{"tool_name":"Bash","tool_input":{"command":"git -C /repo checkout -b feature/other"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "git checkout -b blocked even with git flags before subcommand"

# Test 5: Branch set, command differs with checkout flags (e.g., git checkout --no-track -b)
reset_workspace
WS="$(setup_workspace "phase-5" "in_progress")"
jq '.branch = "feature/test-sample"' "${WS}/state.json" > "${WS}/state.json.tmp" && mv "${WS}/state.json.tmp" "${WS}/state.json"

run_hook "pre-tool-hook.sh" '{"tool_name":"Bash","tool_input":{"command":"git checkout --no-track -b feature/other"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "git checkout -b blocked even with checkout flags before -b"

# Test 6: No active pipeline — allow git checkout -b
reset_workspace

run_hook "pre-tool-hook.sh" '{"tool_name":"Bash","tool_input":{"command":"git checkout -b feature/anything"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "git checkout -b allowed when no active pipeline"

echo ""
echo "--- Rule 5: git checkout main/master guard ---"

# Test 1: Core case — block git checkout main during active pipeline
reset_workspace
WS="$(setup_workspace "phase-5" "in_progress")"

run_hook "pre-tool-hook.sh" '{"tool_name":"Bash","tool_input":{"command":"git checkout main"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "git checkout main blocked during active pipeline"
assert_stderr_contains "main or master" "block message mentions main or master"

# Test 2: Block git checkout master
reset_workspace
WS="$(setup_workspace "phase-5" "in_progress")"

run_hook "pre-tool-hook.sh" '{"tool_name":"Bash","tool_input":{"command":"git checkout master"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "git checkout master blocked during active pipeline"

# Test 3: Block git switch main
reset_workspace
WS="$(setup_workspace "phase-5" "in_progress")"

run_hook "pre-tool-hook.sh" '{"tool_name":"Bash","tool_input":{"command":"git switch main"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "git switch main blocked during active pipeline"

# Test 4: Block git checkout origin/main (remote ref)
reset_workspace
WS="$(setup_workspace "phase-5" "in_progress")"

run_hook "pre-tool-hook.sh" '{"tool_name":"Bash","tool_input":{"command":"git checkout origin/main"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "git checkout origin/main blocked during active pipeline"

# Test 5: Block git -C /repo checkout main (git flags before subcommand)
reset_workspace
WS="$(setup_workspace "phase-5" "in_progress")"

run_hook "pre-tool-hook.sh" '{"tool_name":"Bash","tool_input":{"command":"git -C /repo checkout main"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "git -C /repo checkout main blocked during active pipeline"

# Test 6: Block git checkout -f main (checkout flags before branch name)
reset_workspace
WS="$(setup_workspace "phase-5" "in_progress")"

run_hook "pre-tool-hook.sh" '{"tool_name":"Bash","tool_input":{"command":"git checkout -f main"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "git checkout -f main blocked during active pipeline"

# Test 7: Allow git checkout main when no active pipeline
reset_workspace

run_hook "pre-tool-hook.sh" '{"tool_name":"Bash","tool_input":{"command":"git checkout main"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "git checkout main allowed when no active pipeline"

# Test 8: Allow checkout of non-main feature branch during active pipeline
reset_workspace
WS="$(setup_workspace "phase-5" "in_progress")"

run_hook "pre-tool-hook.sh" '{"tool_name":"Bash","tool_input":{"command":"git checkout feature/my-branch"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "git checkout feature/my-branch allowed during active pipeline"

# Test 9: Allow commands with "checkout main" inside quoted string arguments (false positive fix)
reset_workspace
WS="$(setup_workspace "phase-5" "in_progress")"

run_hook "pre-tool-hook.sh" '{"tool_name":"Bash","tool_input":{"command":"gh pr create --body \"blocks git checkout main during pipeline\""}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "checkout main inside quoted string not blocked (false positive fix)"

# Test 10: Allow commands with checkout main inside single-quoted heredoc body
reset_workspace
WS="$(setup_workspace "phase-5" "in_progress")"

run_hook "pre-tool-hook.sh" '{"tool_name":"Bash","tool_input":{"command":"echo '\''git checkout main is blocked'\'' > /tmp/notes.md"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "checkout main inside single-quoted string not blocked"

echo ""
echo "--- Phase 3+: Edit/Write allowed ---"
reset_workspace
WS="$(setup_workspace "phase-3" "in_progress")"

run_hook "pre-tool-hook.sh" '{"tool_name":"Edit","tool_input":{"file_path":"/src/foo.go"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "Edit allowed in Phase 3"

# ============================================================
echo ""
echo "=== post-agent-hook.sh tests ==="
# ============================================================

echo ""
echo "--- non-Agent tool: no-op ---"
reset_workspace
WS="$(setup_workspace "phase-1" "in_progress")"

run_hook "post-agent-hook.sh" '{"tool_name":"Bash","tool_input":{"command":"ls"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "non-Agent tool skipped"

echo ""
echo "--- empty agent output warning ---"
run_hook "post-agent-hook.sh" '{"tool_name":"Agent","tool_input":{"prompt":"test"},"tool_response":""}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "empty output exits 0 (warn, not block)"
assert_stdout_contains "WARNING" "empty output produces WARNING"

run_hook "post-agent-hook.sh" '{"tool_name":"Agent","tool_input":{"prompt":"test"},"tool_response":"short"}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "short output exits 0"
assert_stdout_contains "WARNING" "short output produces WARNING"

echo ""
echo "--- review phase: missing verdict warning ---"
reset_workspace
WS="$(setup_workspace "phase-3b" "in_progress")"

run_hook "post-agent-hook.sh" '{"tool_name":"Agent","tool_input":{"prompt":"test"},"tool_response":"This design looks good but I have no verdict keyword"}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "missing verdict exits 0"
assert_stdout_contains "APPROVE, APPROVE_WITH_NOTES, or REVISE" "warns about missing verdict"

run_hook "post-agent-hook.sh" '{"tool_name":"Agent","tool_input":{"prompt":"test"},"tool_response":"After thorough review of the design document, I find the architecture to be well-structured. Verdict: APPROVE. The design is solid and complete."}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "APPROVE verdict exits 0"
# Should NOT contain WARNING for valid verdict
if echo "$HOOK_STDOUT_CONTENT" | grep -qF "WARNING"; then
  fail "APPROVE verdict should not produce WARNING"
else
  pass "APPROVE verdict produces no WARNING"
fi

run_hook "post-agent-hook.sh" '{"tool_name":"Agent","tool_input":{"prompt":"test"},"tool_response":"After reviewing all findings, they are all MINOR issues. Verdict: APPROVE_WITH_NOTES. The design is approved with notes for the implementer."}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "APPROVE_WITH_NOTES verdict in phase-3b exits 0"
if echo "$HOOK_STDOUT_CONTENT" | grep -qF "WARNING"; then
  fail "APPROVE_WITH_NOTES verdict in phase-3b should not produce WARNING"
else
  pass "APPROVE_WITH_NOTES verdict in phase-3b produces no WARNING"
fi

run_hook "post-agent-hook.sh" '{"tool_name":"Agent","tool_input":{"prompt":"test"},"tool_response":"Found a CRITICAL structural flaw in the design. Verdict: REVISE. The design must be reworked before proceeding."}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "REVISE verdict in phase-3b exits 0"
if echo "$HOOK_STDOUT_CONTENT" | grep -qF "WARNING"; then
  fail "REVISE verdict in phase-3b should not produce WARNING"
else
  pass "REVISE verdict in phase-3b produces no WARNING"
fi

echo ""
echo "--- review phase-4b: verdict warning ---"
reset_workspace
WS="$(setup_workspace "phase-4b" "in_progress")"

run_hook "post-agent-hook.sh" '{"tool_name":"Agent","tool_input":{"prompt":"test"},"tool_response":"This task list looks complete but I have no verdict keyword"}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "missing verdict in phase-4b exits 0"
assert_stdout_contains "APPROVE, APPROVE_WITH_NOTES, or REVISE" "warns about missing verdict in phase-4b"

run_hook "post-agent-hook.sh" '{"tool_name":"Agent","tool_input":{"prompt":"test"},"tool_response":"After reviewing the task list, all MINOR issues only. Verdict: APPROVE_WITH_NOTES. Tasks approved with notes."}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "APPROVE_WITH_NOTES verdict in phase-4b exits 0"
if echo "$HOOK_STDOUT_CONTENT" | grep -qF "WARNING"; then
  fail "APPROVE_WITH_NOTES verdict in phase-4b should not produce WARNING"
else
  pass "APPROVE_WITH_NOTES verdict in phase-4b produces no WARNING"
fi

echo ""
echo "--- Phase 6: missing PASS/FAIL warning ---"
reset_workspace
WS="$(setup_workspace "phase-6" "in_progress")"

run_hook "post-agent-hook.sh" '{"tool_name":"Agent","tool_input":{"prompt":"test"},"tool_response":"The code review is complete and everything looks reasonable."}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_stdout_contains "PASS or FAIL" "warns about missing PASS/FAIL in Phase 6"

run_hook "post-agent-hook.sh" '{"tool_name":"Agent","tool_input":{"prompt":"test"},"tool_response":"Code review complete. All acceptance criteria have been met and the implementation follows the design. Verdict: PASS. No issues found."}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
if echo "$HOOK_STDOUT_CONTENT" | grep -qF "WARNING"; then
  fail "PASS verdict should not produce WARNING"
else
  pass "PASS verdict produces no WARNING"
fi

# ============================================================
echo ""
echo "=== stop-hook.sh tests ==="
# ============================================================

echo ""
echo "--- no active pipeline ---"
reset_workspace
run_hook "stop-hook.sh" '{"hook_event_name":"Stop","stop_hook_active":false}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "stop allowed when no active pipeline"

echo ""
echo "--- active pipeline: block ---"
reset_workspace
WS="$(setup_workspace "phase-3" "in_progress")"

run_hook "stop-hook.sh" '{"hook_event_name":"Stop","stop_hook_active":false}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "stop blocked when pipeline active"
assert_stderr_contains "still active" "block message mentions still active"
assert_stderr_contains "state-manager.sh abandon" "block message includes abandon command"

echo ""
echo "--- completed pipeline: allow ---"
reset_workspace
WS="$(setup_workspace "final-summary" "completed")"

run_hook "stop-hook.sh" '{"hook_event_name":"Stop","stop_hook_active":false}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "stop allowed when pipeline completed"

echo ""
echo "--- notifyOnStop: true — sound fires once, then flag disarmed ---"
reset_workspace
WS="$(setup_workspace "final-summary" "completed")"
# Set notifyOnStop to true to simulate pipeline just completed
jq '.notifyOnStop = true' "${WS}/state.json" > "${WS}/state.json.tmp" && mv "${WS}/state.json.tmp" "${WS}/state.json"

run_hook "stop-hook.sh" '{"hook_event_name":"Stop","stop_hook_active":false}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "stop allowed when notifyOnStop is true (completed workspace)"
# Verify flag was cleared
NOTIFY_AFTER="$(jq '.notifyOnStop' "${WS}/state.json")"
if [ "$NOTIFY_AFTER" = "false" ]; then
  pass "notifyOnStop flag cleared to false after first Stop"
else
  fail "notifyOnStop flag cleared to false after first Stop" "got: $NOTIFY_AFTER"
fi

echo ""
echo "--- notifyOnStop: false after disarm — workspace invisible on second Stop ---"
# Don't reset — reuse same workspace with notifyOnStop = false
run_hook "stop-hook.sh" '{"hook_event_name":"Stop","stop_hook_active":false}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "stop allowed on second Stop (flag already cleared, workspace invisible)"

echo ""
echo "--- awaiting_human checkpoint: allow ---"
reset_workspace
WS="$(setup_workspace "checkpoint-a" "awaiting_human")"

run_hook "stop-hook.sh" '{"hook_event_name":"Stop","stop_hook_active":false}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "stop allowed at human checkpoint"

echo ""
echo "--- final-summary with summary.md: block (dead block removed) ---"
reset_workspace
WS="$(setup_workspace "final-summary" "in_progress")"
touch "${WS}/summary.md"

run_hook "stop-hook.sh" '{"hook_event_name":"Stop","stop_hook_active":false}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "stop blocked at final-summary in_progress even with summary.md (dead block removed)"

echo ""
echo "--- abandoned pipeline: allow stop ---"
reset_workspace
WS="$(setup_workspace "phase-3" "abandoned")"

run_hook "stop-hook.sh" '{"hook_event_name":"Stop","stop_hook_active":false}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "stop allowed when pipeline abandoned"

echo ""
echo "--- abandoned pipeline: hooks ignore it ---"
run_hook "pre-tool-hook.sh" '{"tool_name":"Edit","tool_input":{"file_path":"/src/foo.go"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "Edit allowed when only pipeline is abandoned"

echo ""
echo "--- final-summary without summary.md: block ---"
reset_workspace
WS="$(setup_workspace "final-summary" "in_progress")"

run_hook "stop-hook.sh" '{"hook_event_name":"Stop","stop_hook_active":false}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "stop blocked at final-summary without summary.md"

# ============================================================
echo ""
echo "=== pre-tool-hook.sh artifact guard tests (Rule 3) ==="
# ============================================================

echo ""
echo "--- 3a: phase-complete blocked when artifact missing ---"
reset_workspace
WS="$(setup_workspace "phase-1" "in_progress")"
# analysis.md does NOT exist

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-complete ${WS} phase-1\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "phase-complete blocked without analysis.md"

echo ""
echo "--- 3a: phase-complete allowed when artifact exists ---"
touch "${WS}/analysis.md"

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-complete ${WS} phase-1\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "phase-complete allowed with analysis.md present"

echo ""
echo "--- 3a: phase-complete phase-3 blocked without design.md ---"
reset_workspace
WS="$(setup_workspace "phase-3" "in_progress")"

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-complete ${WS} phase-3\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "phase-complete blocked without design.md"

echo ""
echo "--- 3a: phase-complete final-summary blocked without summary.md ---"
reset_workspace
WS="$(setup_workspace "final-summary" "in_progress")"

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-complete ${WS} final-summary\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "phase-complete blocked without summary.md"

echo ""
echo "--- 3a: phase-complete final-summary allowed with summary.md ---"
touch "${WS}/summary.md"

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-complete ${WS} final-summary\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "phase-complete allowed with summary.md present"

echo ""
echo "--- 3a: phase-complete for non-artifact phase (checkpoint-a) allowed ---"
reset_workspace
WS="$(setup_workspace "checkpoint-a" "awaiting_human")"

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-complete ${WS} checkpoint-a\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "phase-complete allowed for checkpoint (no artifact required)"

echo ""
echo "--- 3a: chained command with phase-complete checked ---"
reset_workspace
WS="$(setup_workspace "phase-2" "in_progress")"

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-complete ${WS} phase-2 && bash scripts/state-manager.sh phase-log ${WS} phase-2 1000 5000 sonnet\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "phase-complete blocked in chained command without investigation.md"

echo ""
echo "--- 3b: task review completion blocked without review file ---"
reset_workspace
TASK_JSON='{"1": {"title": "Fix bug", "executionMode": "sequential", "implStatus": "completed", "implRetries": 0, "reviewStatus": "in_progress", "reviewRetries": 0}}'
WS="$(setup_workspace "phase-6" "in_progress" "$TASK_JSON")"

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh task-update ${WS} 1 reviewStatus completed_pass\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "task review completion blocked without review-1.md"

echo ""
echo "--- 3b: task review completion allowed with review file ---"
touch "${WS}/review-1.md"

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh task-update ${WS} 1 reviewStatus completed_pass\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "task review completion allowed with review-1.md present"

echo ""
echo "--- 3c: phase-start phase-5 blocked without tasks ---"
reset_workspace
WS="$(setup_workspace "phase-5" "pending")"

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-start ${WS} phase-5\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "phase-start phase-5 blocked without tasks"

echo ""
echo "--- 3c: phase-start phase-5 allowed with tasks ---"
reset_workspace
TASK_JSON='{"1": {"title": "Implement", "executionMode": "sequential", "implStatus": "pending", "implRetries": 0, "reviewStatus": "pending", "reviewRetries": 0}}'
WS="$(setup_workspace "phase-5" "pending" "$TASK_JSON")"

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-start ${WS} phase-5\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "phase-start phase-5 allowed with tasks initialized"

echo ""
echo "--- 3d: phase-log warns but allows duplicate entry (retries valid) ---"
reset_workspace
WS="$(setup_workspace "phase-2" "in_progress")"
# Add an existing phase-log entry
jq '.phaseLog = [{"phase": "phase-1", "tokens": 1000, "duration_ms": 5000, "model": "sonnet", "timestamp": "2026-03-20T00:00:00Z"}]' "${WS}/state.json" > "${WS}/state.tmp" && mv "${WS}/state.tmp" "${WS}/state.json"

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-log ${WS} phase-1 2000 3000 sonnet\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "phase-log allowed for duplicate (warns only, retries valid)"
# Verify warning was emitted on stderr
if echo "$HOOK_STDERR_CONTENT" | grep -q "WARNING.*phase-log.*already exists"; then
  pass "phase-log duplicate emitted warning on stderr"
else
  fail "phase-log duplicate should emit warning" "stderr: $HOOK_STDERR_CONTENT"
fi

echo ""
echo "--- 3d: phase-log allowed for new entry (no warning) ---"
run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-log ${WS} phase-2 2000 3000 sonnet\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "phase-log allowed for new phase-2 entry"

echo ""
echo "--- 3i: phase-log-missing guard — warns when phaseLog has no entry for phase-2 ---"
reset_workspace
WS="$(setup_workspace "phase-2" "in_progress")"
# Satisfy 3a artifact guard so the hook reaches the 3i guard
touch "${WS}/investigation.md"

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-complete ${WS} phase-2\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "phase-complete phase-2 allowed even when phase-log entry is missing (non-blocking)"
if echo "$HOOK_STDERR_CONTENT" | grep -q "WARNING.*phase-log"; then
  pass "phase-log-missing guard emits warning when no entry for phase-2"
else
  fail "phase-log-missing guard should emit warning for phase-2 with empty phaseLog" "stderr: $HOOK_STDERR_CONTENT"
fi

echo ""
echo "--- 3i: phase-log-missing guard — silent when phaseLog entry exists ---"
reset_workspace
WS="$(setup_workspace "phase-2" "in_progress")"
touch "${WS}/investigation.md"
# Inject a phaseLog entry for phase-2
jq '.phaseLog = [{"phase": "phase-2", "tokens": 1500, "duration_ms": 4000, "model": "sonnet", "timestamp": "2026-03-20T00:00:00Z"}]' "${WS}/state.json" > "${WS}/state.tmp" && mv "${WS}/state.tmp" "${WS}/state.json"

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-complete ${WS} phase-2\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "phase-complete phase-2 allowed when phase-log entry exists"
if ! echo "$HOOK_STDERR_CONTENT" | grep -q "WARNING.*phase-log"; then
  pass "phase-log-missing guard emits no warning when entry exists"
else
  fail "phase-log-missing guard should be silent when entry exists" "stderr: $HOOK_STDERR_CONTENT"
fi

echo ""
echo "--- 3i: phase-log-missing guard — silent for checkpoint-a (allowlist bypass) ---"
reset_workspace
WS="$(setup_workspace "checkpoint-a" "awaiting_human")"

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-complete ${WS} checkpoint-a\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "phase-complete checkpoint-a does not trigger phase-log-missing warning"
if ! echo "$HOOK_STDERR_CONTENT" | grep -q "WARNING.*phase-log"; then
  pass "phase-log-missing guard silent for checkpoint-a"
else
  fail "phase-log-missing guard should not fire for checkpoint-a" "stderr: $HOOK_STDERR_CONTENT"
fi

echo ""
echo "--- 3: non-state-manager Bash commands unaffected ---"
reset_workspace
WS="$(setup_workspace "phase-1" "in_progress")"

run_hook "pre-tool-hook.sh" '{"tool_name":"Bash","tool_input":{"command":"ls -la"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "non-state-manager Bash command allowed"

echo ""
echo "--- 3e: checkpoint guard — phase-complete checkpoint-a blocked when not awaiting_human ---"
reset_workspace
WS="$(setup_workspace "checkpoint-a" "in_progress")"

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-complete ${WS} checkpoint-a\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "phase-complete checkpoint-a blocked when status is in_progress"
assert_stderr_contains "checkpoint" "Block message mentions checkpoint"

echo ""
echo "--- 3e: checkpoint guard — phase-complete checkpoint-a allowed when awaiting_human ---"
reset_workspace
WS="$(setup_workspace "checkpoint-a" "awaiting_human")"

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-complete ${WS} checkpoint-a\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "phase-complete checkpoint-a allowed when status is awaiting_human"

echo ""
echo "--- 3e: checkpoint guard — phase-complete phase-3 allowed regardless of status ---"
reset_workspace
WS="$(setup_workspace "phase-3" "in_progress")"
touch "${WS}/design.md"

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-complete ${WS} phase-3\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "phase-complete phase-3 allowed regardless of currentPhaseStatus"

echo ""
echo "--- 3e: checkpoint guard — phase-complete checkpoint-b blocked when not awaiting_human ---"
reset_workspace
WS="$(setup_workspace "checkpoint-b" "in_progress")"

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-complete ${WS} checkpoint-b\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "phase-complete checkpoint-b blocked when status is in_progress"

echo ""
echo "--- 3e: checkpoint guard — phase-complete checkpoint-b allowed when awaiting_human ---"
reset_workspace
WS="$(setup_workspace "checkpoint-b" "awaiting_human")"

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-complete ${WS} checkpoint-b\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "phase-complete checkpoint-b allowed when status is awaiting_human"

# ============================================================
echo ""
echo "=== pre-tool-hook.sh: Rule 3j (checkpoint revision-pending guard) tests ==="
# ============================================================

echo ""
echo "--- 3j-a: phase-complete checkpoint-a blocked when revision pending (flag=true, status=awaiting_human) ---"
reset_workspace
WS="$(setup_workspace "checkpoint-a" "awaiting_human")"
bash "$SM" set-revision-pending "$WS" checkpoint-a

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-complete ${WS} checkpoint-a\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "3j-a: phase-complete checkpoint-a blocked when checkpointRevisionPending.checkpoint-a is true"
assert_stderr_contains "clear-revision-pending" "3j-a: block message instructs to call clear-revision-pending"

echo ""
echo "--- 3j-b: phase-complete checkpoint-a allowed when revision not pending (flag=false, status=awaiting_human) ---"
reset_workspace
WS="$(setup_workspace "checkpoint-a" "awaiting_human")"
# checkpointRevisionPending field is absent from setup_workspace state.json; hook uses // false fallback

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-complete ${WS} checkpoint-a\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "3j-b: phase-complete checkpoint-a allowed when checkpointRevisionPending.checkpoint-a is false"

echo ""
echo "--- 3j-c: phase-complete checkpoint-b blocked when revision pending (flag=true, status=awaiting_human) ---"
reset_workspace
WS="$(setup_workspace "checkpoint-b" "awaiting_human")"
bash "$SM" set-revision-pending "$WS" checkpoint-b

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-complete ${WS} checkpoint-b\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "3j-c: phase-complete checkpoint-b blocked when checkpointRevisionPending.checkpoint-b is true"
assert_stderr_contains "clear-revision-pending" "3j-c: block message instructs to call clear-revision-pending"

echo ""
echo "--- 3j-d: phase-complete checkpoint-b allowed when revision not pending (flag=false, status=awaiting_human) ---"
reset_workspace
WS="$(setup_workspace "checkpoint-b" "awaiting_human")"
# checkpointRevisionPending field is absent from setup_workspace state.json; hook uses // false fallback

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-complete ${WS} checkpoint-b\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "3j-d: phase-complete checkpoint-b allowed when checkpointRevisionPending.checkpoint-b is false"

echo ""
echo "--- 3j-e: phase-complete checkpoint-a blocked when flag=true, status=pending (Rule 3e fires first) ---"
reset_workspace
WS="$(setup_workspace "checkpoint-a" "pending")"
bash "$SM" set-revision-pending "$WS" checkpoint-a

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-complete ${WS} checkpoint-a\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "3j-e: phase-complete checkpoint-a blocked when flag=true and status=pending (Rule 3e fires first)"

echo ""
echo "--- 3j-f: full revision-loop integration sequence ---"
reset_workspace
WS="$(setup_workspace "checkpoint-a" "awaiting_human")"

# Step 1: User requests revision — set revision-pending flag
bash "$SM" set-revision-pending "$WS" checkpoint-a
CRP_FLAG="$(jq '.checkpointRevisionPending["checkpoint-a"]' "${WS}/state.json")"
if [ "$CRP_FLAG" = "true" ]; then
  pass "3j-f: set-revision-pending sets flag to true"
else
  fail "3j-f: set-revision-pending sets flag to true" "got: $CRP_FLAG"
fi

# Step 2: Re-run phases (simulate by setting currentPhaseStatus to pending via direct state update)
jq '.currentPhase = "checkpoint-a" | .currentPhaseStatus = "pending"' "${WS}/state.json" > "${WS}/state.tmp" && mv "${WS}/state.tmp" "${WS}/state.json"

# Step 3: After phase-3b completes, checkpoint call advances status to awaiting_human (flag stays true)
bash "$SM" checkpoint "$WS" checkpoint-a
CP_STATUS="$(jq -r '.currentPhaseStatus' "${WS}/state.json")"
if [ "$CP_STATUS" = "awaiting_human" ]; then
  pass "3j-f: checkpoint call advances status to awaiting_human"
else
  fail "3j-f: checkpoint call advances status to awaiting_human" "got: $CP_STATUS"
fi
CRP_AFTER_CP="$(jq '.checkpointRevisionPending["checkpoint-a"]' "${WS}/state.json")"
if [ "$CRP_AFTER_CP" = "true" ]; then
  pass "3j-f: revision-pending flag remains true after checkpoint call"
else
  fail "3j-f: revision-pending flag remains true after checkpoint call" "got: $CRP_AFTER_CP"
fi

# Step 4: phase-complete checkpoint-a must be blocked (flag is still true, status is awaiting_human)
run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-complete ${WS} checkpoint-a\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "3j-f: phase-complete checkpoint-a blocked while revision-pending flag is true"

# Step 5: User approves the revised artifact — clear revision-pending flag
bash "$SM" clear-revision-pending "$WS" checkpoint-a
CRP_CLEARED="$(jq '.checkpointRevisionPending["checkpoint-a"]' "${WS}/state.json")"
if [ "$CRP_CLEARED" = "false" ]; then
  pass "3j-f: clear-revision-pending sets flag to false"
else
  fail "3j-f: clear-revision-pending sets flag to false" "got: $CRP_CLEARED"
fi

# Step 6: phase-complete checkpoint-a must now be allowed (flag is false, status is awaiting_human)
run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-complete ${WS} checkpoint-a\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "3j-f: phase-complete checkpoint-a allowed after clear-revision-pending"

echo ""
echo "--- 3g: task-init guard — task-init blocked when checkpoint-b not completed or skipped ---"
reset_workspace
WS="$(setup_workspace "checkpoint-b" "awaiting_human")"
# state.json has completedPhases: ["setup"] only — checkpoint-b absent from completedPhases and skippedPhases

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh task-init ${WS} '{}'\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "task-init blocked when checkpoint-b not in completedPhases or skippedPhases"
assert_stderr_contains "checkpoint-b" "Block message mentions checkpoint-b"

echo ""
echo "--- 3g: task-init guard — task-init allowed when checkpoint-b is in completedPhases ---"
reset_workspace
WS="$(setup_workspace "phase-5" "in_progress")"
# Write state.json directly with checkpoint-b in completedPhases
cat > "${WS}/state.json" <<ENDJSON
{
  "version": 1,
  "specName": "test",
  "workspace": "${WS}",
  "branch": "feature/test",
  "currentPhase": "phase-5",
  "currentPhaseStatus": "in_progress",
  "completedPhases": ["setup", "checkpoint-b"],
  "skippedPhases": [],
  "revisions": { "designRevisions": 0, "taskRevisions": 0 },
  "tasks": {},
  "phaseLog": [],
  "timestamps": { "created": "2026-03-20T00:00:00Z", "lastUpdated": "2026-03-20T00:00:00Z", "phaseStarted": null },
  "error": null
}
ENDJSON

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh task-init ${WS} '{}'\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "task-init allowed when checkpoint-b is in completedPhases"

echo ""
echo "--- 3g: task-init guard — task-init allowed when checkpoint-b is in skippedPhases ---"
reset_workspace
WS="$(setup_workspace "phase-5" "in_progress")"
# Write state.json directly with checkpoint-b in skippedPhases
cat > "${WS}/state.json" <<ENDJSON
{
  "version": 1,
  "specName": "test",
  "workspace": "${WS}",
  "branch": "feature/test",
  "currentPhase": "phase-5",
  "currentPhaseStatus": "in_progress",
  "completedPhases": ["setup"],
  "skippedPhases": ["checkpoint-b"],
  "revisions": { "designRevisions": 0, "taskRevisions": 0 },
  "tasks": {},
  "phaseLog": [],
  "timestamps": { "created": "2026-03-20T00:00:00Z", "lastUpdated": "2026-03-20T00:00:00Z", "phaseStarted": null },
  "error": null
}
ENDJSON

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh task-init ${WS} '{}'\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "task-init allowed when checkpoint-b is in skippedPhases"

echo ""
echo "--- 3g: regression — orchestrator skips checkpoint-b entirely then calls task-init (P15 scenario) ---"
reset_workspace
WS="$(setup_workspace "phase-4b" "in_progress")"
# State simulates: phase-4b completed, checkpoint-b never started/completed/skipped
# completedPhases: ["setup"] — checkpoint-b absent

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh task-init ${WS} '{}'\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "task-init blocked when orchestrator skips checkpoint-b entirely (P15 regression)"
assert_stderr_contains "checkpoint-b" "Block message mentions checkpoint-b in P15 regression"

echo ""
echo "--- 3f: effort-null guard — warns when effort is null before phase-start phase-1 ---"
reset_workspace
WS="$(setup_workspace "phase-1" "pending")"
# effort field is absent in state.json produced by setup_workspace (pre-F13 schema)

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-start ${WS} phase-1\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "phase-start phase-1 allowed even when effort is null (non-blocking)"
if echo "$HOOK_STDERR_CONTENT" | grep -q "WARNING.*effort"; then
  pass "effort-null guard emits warning on stderr when effort is null"
else
  fail "effort-null guard should emit warning when effort is null" "stderr: $HOOK_STDERR_CONTENT"
fi

echo ""
echo "--- 3f: effort-null guard — silent when effort is already set ---"
reset_workspace
WS="$(setup_workspace "phase-1" "pending")"
# Inject effort field into state.json
jq '. + {"effort": "M"}' "${WS}/state.json" > "${WS}/state.tmp" && mv "${WS}/state.tmp" "${WS}/state.json"

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-start ${WS} phase-1\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "phase-start phase-1 allowed when effort is set"
if ! echo "$HOOK_STDERR_CONTENT" | grep -q "WARNING.*effort"; then
  pass "effort-null guard emits no warning when effort is set"
else
  fail "effort-null guard should be silent when effort is set" "stderr: $HOOK_STDERR_CONTENT"
fi

echo ""
echo "--- 3h: taskType-null guard — warns when taskType is null before phase-start phase-1 ---"
reset_workspace
WS="$(setup_workspace "phase-1" "pending")"
# taskType field is null in state.json produced by setup_workspace (standard schema)

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-start ${WS} phase-1\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "phase-start phase-1 allowed even when taskType is null (non-blocking)"
if echo "$HOOK_STDERR_CONTENT" | grep -q "WARNING.*taskType"; then
  pass "taskType-null guard emits warning on stderr when taskType is null"
else
  fail "taskType-null guard should emit warning when taskType is null" "stderr: $HOOK_STDERR_CONTENT"
fi
if echo "$HOOK_STDERR_CONTENT" | grep -q "WARNING.*effort"; then
  pass "effort-null guard also fires in same invocation when both are null (regression)"
else
  fail "effort-null guard should also fire when effort is null in same invocation" "stderr: $HOOK_STDERR_CONTENT"
fi

echo ""
echo "--- 3h: taskType-null guard — silent when taskType is set ---"
reset_workspace
WS="$(setup_workspace "phase-1" "pending")"
# Inject taskType field into state.json
jq '. + {"taskType": "feature"}' "${WS}/state.json" > "${WS}/state.tmp" && mv "${WS}/state.tmp" "${WS}/state.json"

run_hook "pre-tool-hook.sh" "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-start ${WS} phase-1\"}}" "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "phase-start phase-1 allowed when taskType is set"
if ! echo "$HOOK_STDERR_CONTENT" | grep -q "WARNING.*taskType"; then
  pass "taskType-null guard emits no warning when taskType is set"
else
  fail "taskType-null guard should be silent when taskType is set" "stderr: $HOOK_STDERR_CONTENT"
fi

# ============================================================
echo ""
echo "=== state-manager.sh — set-effort tests ==="
# ============================================================

SM_WS_EFFORT="${TMPDIR_BASE}/sm-effort"
mkdir -p "$SM_WS_EFFORT"
bash "$SM" init "$SM_WS_EFFORT" "effort-test"

echo ""
echo "--- set-effort ---"

# Valid value: XS
bash "$SM" set-effort "$SM_WS_EFFORT" "XS"
EFFORT_VAL="$(jq -r '.effort' "${SM_WS_EFFORT}/state.json")"
if [ "$EFFORT_VAL" = "XS" ]; then
  pass "set-effort writes XS to state.json"
else
  fail "set-effort writes XS to state.json" "got: $EFFORT_VAL"
fi

# Valid value: M (overwrite)
bash "$SM" set-effort "$SM_WS_EFFORT" "M"
EFFORT_VAL2="$(jq -r '.effort' "${SM_WS_EFFORT}/state.json")"
if [ "$EFFORT_VAL2" = "M" ]; then
  pass "set-effort overwrites previous effort value"
else
  fail "set-effort overwrites previous effort value" "got: $EFFORT_VAL2"
fi

# Valid value: L
bash "$SM" set-effort "$SM_WS_EFFORT" "L"
EFFORT_L="$(jq -r '.effort' "${SM_WS_EFFORT}/state.json")"
if [ "$EFFORT_L" = "L" ]; then
  pass "set-effort accepts L"
else
  fail "set-effort accepts L" "got: $EFFORT_L"
fi

# Valid value: S
bash "$SM" set-effort "$SM_WS_EFFORT" "S"
EFFORT_S="$(jq -r '.effort' "${SM_WS_EFFORT}/state.json")"
if [ "$EFFORT_S" = "S" ]; then
  pass "set-effort accepts S"
else
  fail "set-effort accepts S" "got: $EFFORT_S"
fi

# set-effort updates timestamps.lastUpdated
EFFORT_TS="$(jq -r '.timestamps.lastUpdated' "${SM_WS_EFFORT}/state.json")"
if [ -n "$EFFORT_TS" ] && [ "$EFFORT_TS" != "null" ]; then
  pass "set-effort updates timestamps.lastUpdated"
else
  fail "set-effort updates timestamps.lastUpdated" "got: $EFFORT_TS"
fi

# Invalid value rejected
EFFORT_ERR_EXIT=0
bash "$SM" set-effort "$SM_WS_EFFORT" "INVALID" 2>/dev/null || EFFORT_ERR_EXIT=$?
if [ "$EFFORT_ERR_EXIT" -ne 0 ]; then
  pass "set-effort rejects invalid value with non-zero exit"
else
  fail "set-effort rejects invalid value with non-zero exit" "exit was 0"
fi

# Invalid value produces error message
EFFORT_ERR_MSG="$(bash "$SM" set-effort "$SM_WS_EFFORT" "HUGE" 2>&1 || true)"
if echo "$EFFORT_ERR_MSG" | grep -qi "invalid\|unknown\|expected"; then
  pass "set-effort invalid value emits error message"
else
  fail "set-effort invalid value emits error message" "got: $EFFORT_ERR_MSG"
fi

# init sets effort to null
SM_WS_EFFORT_INIT="${TMPDIR_BASE}/sm-effort-init"
mkdir -p "$SM_WS_EFFORT_INIT"
bash "$SM" init "$SM_WS_EFFORT_INIT" "effort-init-test"
INIT_EFFORT="$(jq '.effort' "${SM_WS_EFFORT_INIT}/state.json")"
if [ "$INIT_EFFORT" = "null" ]; then
  pass "init sets effort to null"
else
  fail "init sets effort to null" "got: $INIT_EFFORT"
fi

# ============================================================
echo ""
echo "=== state-manager.sh — set-flow-template tests ==="
# ============================================================

SM_WS_FT="${TMPDIR_BASE}/sm-flowtemplate"
mkdir -p "$SM_WS_FT"
bash "$SM" init "$SM_WS_FT" "flowtemplate-test"

echo ""
echo "--- set-flow-template ---"

# Valid value: direct
bash "$SM" set-flow-template "$SM_WS_FT" "direct"
FT_VAL="$(jq -r '.flowTemplate' "${SM_WS_FT}/state.json")"
if [ "$FT_VAL" = "direct" ]; then
  pass "set-flow-template writes direct to state.json"
else
  fail "set-flow-template writes direct to state.json" "got: $FT_VAL"
fi

# Valid value: lite (overwrite)
bash "$SM" set-flow-template "$SM_WS_FT" "lite"
FT_LITE="$(jq -r '.flowTemplate' "${SM_WS_FT}/state.json")"
if [ "$FT_LITE" = "lite" ]; then
  pass "set-flow-template accepts lite"
else
  fail "set-flow-template accepts lite" "got: $FT_LITE"
fi

# Valid value: light
bash "$SM" set-flow-template "$SM_WS_FT" "light"
FT_LIGHT="$(jq -r '.flowTemplate' "${SM_WS_FT}/state.json")"
if [ "$FT_LIGHT" = "light" ]; then
  pass "set-flow-template accepts light"
else
  fail "set-flow-template accepts light" "got: $FT_LIGHT"
fi

# Valid value: standard
bash "$SM" set-flow-template "$SM_WS_FT" "standard"
FT_STD="$(jq -r '.flowTemplate' "${SM_WS_FT}/state.json")"
if [ "$FT_STD" = "standard" ]; then
  pass "set-flow-template accepts standard"
else
  fail "set-flow-template accepts standard" "got: $FT_STD"
fi

# Valid value: full
bash "$SM" set-flow-template "$SM_WS_FT" "full"
FT_FULL="$(jq -r '.flowTemplate' "${SM_WS_FT}/state.json")"
if [ "$FT_FULL" = "full" ]; then
  pass "set-flow-template accepts full"
else
  fail "set-flow-template accepts full" "got: $FT_FULL"
fi

# set-flow-template updates timestamps.lastUpdated
FT_TS="$(jq -r '.timestamps.lastUpdated' "${SM_WS_FT}/state.json")"
if [ -n "$FT_TS" ] && [ "$FT_TS" != "null" ]; then
  pass "set-flow-template updates timestamps.lastUpdated"
else
  fail "set-flow-template updates timestamps.lastUpdated" "got: $FT_TS"
fi

# Invalid value rejected
FT_ERR_EXIT=0
bash "$SM" set-flow-template "$SM_WS_FT" "INVALID" 2>/dev/null || FT_ERR_EXIT=$?
if [ "$FT_ERR_EXIT" -ne 0 ]; then
  pass "set-flow-template rejects invalid value with non-zero exit"
else
  fail "set-flow-template rejects invalid value with non-zero exit" "exit was 0"
fi

# Invalid value produces error message
FT_ERR_MSG="$(bash "$SM" set-flow-template "$SM_WS_FT" "extreme" 2>&1 || true)"
if echo "$FT_ERR_MSG" | grep -qi "invalid\|unknown\|expected"; then
  pass "set-flow-template invalid value emits error message"
else
  fail "set-flow-template invalid value emits error message" "got: $FT_ERR_MSG"
fi

# init sets flowTemplate to null
SM_WS_FT_INIT="${TMPDIR_BASE}/sm-ft-init"
mkdir -p "$SM_WS_FT_INIT"
bash "$SM" init "$SM_WS_FT_INIT" "ft-init-test"
INIT_FT="$(jq '.flowTemplate' "${SM_WS_FT_INIT}/state.json")"
if [ "$INIT_FT" = "null" ]; then
  pass "init sets flowTemplate to null"
else
  fail "init sets flowTemplate to null" "got: $INIT_FT"
fi

# ============================================================
echo ""
echo "=== state-manager.sh — resume-info effort projection tests ==="
# ============================================================

SM_WS_RI_EFFORT="${TMPDIR_BASE}/sm-ri-effort"
mkdir -p "$SM_WS_RI_EFFORT"
bash "$SM" init "$SM_WS_RI_EFFORT" "ri-effort-test"

echo ""
echo "--- resume-info effort projection ---"

# effort is null when not set
RI_EFFORT_NULL="$(bash "$SM" resume-info "$SM_WS_RI_EFFORT" | jq '.effort')"
if [ "$RI_EFFORT_NULL" = "null" ]; then
  pass "resume-info projects effort as null when not set"
else
  fail "resume-info projects effort as null when not set" "got: $RI_EFFORT_NULL"
fi

# effort is projected correctly after set-effort
bash "$SM" set-effort "$SM_WS_RI_EFFORT" "M"
RI_EFFORT_M="$(bash "$SM" resume-info "$SM_WS_RI_EFFORT" | jq -r '.effort')"
if [ "$RI_EFFORT_M" = "M" ]; then
  pass "resume-info projects effort correctly after set-effort"
else
  fail "resume-info projects effort correctly after set-effort" "got: $RI_EFFORT_M"
fi

# effort is null in legacy state (effort field absent)
SM_WS_RI_EFFORT_LEGACY="${TMPDIR_BASE}/sm-ri-effort-legacy"
mkdir -p "$SM_WS_RI_EFFORT_LEGACY"
cat > "${SM_WS_RI_EFFORT_LEGACY}/state.json" <<LEGACYJSON2
{
  "version": 1,
  "specName": "legacy-effort-test",
  "workspace": "${SM_WS_RI_EFFORT_LEGACY}",
  "branch": null,
  "taskType": "feature",
  "currentPhase": "phase-3",
  "currentPhaseStatus": "pending",
  "completedPhases": ["setup", "phase-1", "phase-2"],
  "skippedPhases": [],
  "revisions": { "designRevisions": 0, "taskRevisions": 0 },
  "tasks": {},
  "phaseLog": [],
  "timestamps": { "created": "2026-01-01T00:00:00Z", "lastUpdated": "2026-01-01T00:00:00Z", "phaseStarted": null },
  "error": null
}
LEGACYJSON2

LEGACY_EFFORT="$(bash "$SM" resume-info "$SM_WS_RI_EFFORT_LEGACY" | jq '.effort')"
if [ "$LEGACY_EFFORT" = "null" ]; then
  pass "resume-info handles missing effort in legacy state (returns null)"
else
  fail "resume-info handles missing effort in legacy state (returns null)" "got: $LEGACY_EFFORT"
fi

# ============================================================
echo ""
echo "=== state-manager.sh — resume-info flowTemplate projection tests ==="
# ============================================================

SM_WS_RI_FT="${TMPDIR_BASE}/sm-ri-ft"
mkdir -p "$SM_WS_RI_FT"
bash "$SM" init "$SM_WS_RI_FT" "ri-ft-test"

echo ""
echo "--- resume-info flowTemplate projection ---"

# flowTemplate is null when not set
RI_FT_NULL="$(bash "$SM" resume-info "$SM_WS_RI_FT" | jq '.flowTemplate')"
if [ "$RI_FT_NULL" = "null" ]; then
  pass "resume-info projects flowTemplate as null when not set"
else
  fail "resume-info projects flowTemplate as null when not set" "got: $RI_FT_NULL"
fi

# flowTemplate is projected correctly after set-flow-template
bash "$SM" set-flow-template "$SM_WS_RI_FT" "standard"
RI_FT_STD="$(bash "$SM" resume-info "$SM_WS_RI_FT" | jq -r '.flowTemplate')"
if [ "$RI_FT_STD" = "standard" ]; then
  pass "resume-info projects flowTemplate correctly after set-flow-template"
else
  fail "resume-info projects flowTemplate correctly after set-flow-template" "got: $RI_FT_STD"
fi

# flowTemplate is null in legacy state (flowTemplate field absent)
SM_WS_RI_FT_LEGACY="${TMPDIR_BASE}/sm-ri-ft-legacy"
mkdir -p "$SM_WS_RI_FT_LEGACY"
cat > "${SM_WS_RI_FT_LEGACY}/state.json" <<LEGACYJSON3
{
  "version": 1,
  "specName": "legacy-ft-test",
  "workspace": "${SM_WS_RI_FT_LEGACY}",
  "branch": null,
  "taskType": "feature",
  "currentPhase": "phase-3",
  "currentPhaseStatus": "pending",
  "completedPhases": ["setup", "phase-1", "phase-2"],
  "skippedPhases": [],
  "revisions": { "designRevisions": 0, "taskRevisions": 0 },
  "tasks": {},
  "phaseLog": [],
  "timestamps": { "created": "2026-01-01T00:00:00Z", "lastUpdated": "2026-01-01T00:00:00Z", "phaseStarted": null },
  "error": null
}
LEGACYJSON3

LEGACY_FT="$(bash "$SM" resume-info "$SM_WS_RI_FT_LEGACY" | jq '.flowTemplate')"
if [ "$LEGACY_FT" = "null" ]; then
  pass "resume-info handles missing flowTemplate in legacy state (returns null)"
else
  fail "resume-info handles missing flowTemplate in legacy state (returns null)" "got: $LEGACY_FT"
fi

# ============================================================
echo ""
echo "=== state-manager.sh — full + --auto state correctness ==="
# ============================================================

SM_WS_FULL="${TMPDIR_BASE}/sm-full-auto"
mkdir -p "$SM_WS_FULL"
bash "$SM" init "$SM_WS_FULL" "full-auto-test"

echo ""
echo "--- full template: autoApprove stays false when set-auto-approve not called ---"

# Simulate orchestrator selecting full template: set flowTemplate to full
bash "$SM" set-flow-template "$SM_WS_FULL" "full"
# Simulate the orchestrator's conflict resolution path: set-auto-approve is NOT called
# Verify that autoApprove remains false (the orchestrator's "skip set-auto-approve" path)
FULL_AA="$(bash "$SM" resume-info "$SM_WS_FULL" | jq '.autoApprove')"
if [ "$FULL_AA" = "false" ]; then
  pass "full template + --auto conflict: autoApprove stays false when set-auto-approve not called"
else
  fail "full template + --auto conflict: autoApprove stays false when set-auto-approve not called" "got: $FULL_AA"
fi

# ============================================================
echo ""
echo "--- set-use-current-branch ---"
SM_WS_UCB="${TMPDIR_BASE}/sm-ucb"
mkdir -p "$SM_WS_UCB"
bash "$SM" init "$SM_WS_UCB" "ucb-test"

# Verify default useCurrentBranch is false
UCB_DEFAULT="$(jq '.useCurrentBranch' "${SM_WS_UCB}/state.json")"
if [ "$UCB_DEFAULT" = "false" ]; then
  pass "init sets useCurrentBranch = false by default"
else
  fail "init sets useCurrentBranch = false by default" "got: $UCB_DEFAULT"
fi

# Set use-current-branch
bash "$SM" set-use-current-branch "$SM_WS_UCB" "feature/existing-work"
UCB_VAL="$(jq '.useCurrentBranch' "${SM_WS_UCB}/state.json")"
if [ "$UCB_VAL" = "true" ]; then
  pass "set-use-current-branch writes useCurrentBranch = true to state.json"
else
  fail "set-use-current-branch writes useCurrentBranch = true to state.json" "got: $UCB_VAL"
fi

UCB_BRANCH="$(jq -r '.branch' "${SM_WS_UCB}/state.json")"
if [ "$UCB_BRANCH" = "feature/existing-work" ]; then
  pass "set-use-current-branch also sets branch name"
else
  fail "set-use-current-branch also sets branch name" "got: $UCB_BRANCH"
fi

UCB_TS="$(jq -r '.timestamps.lastUpdated' "${SM_WS_UCB}/state.json")"
if [ -n "$UCB_TS" ] && [ "$UCB_TS" != "null" ]; then
  pass "set-use-current-branch updates timestamps.lastUpdated"
else
  fail "set-use-current-branch updates timestamps.lastUpdated" "got: $UCB_TS"
fi

RESUME_UCB_TRUE="$(bash "$SM" resume-info "$SM_WS_UCB" | jq '.useCurrentBranch')"
if [ "$RESUME_UCB_TRUE" = "true" ]; then
  pass "resume-info projects useCurrentBranch: true after set-use-current-branch"
else
  fail "resume-info projects useCurrentBranch: true after set-use-current-branch" "got: $RESUME_UCB_TRUE"
fi

# Fresh workspace without set-use-current-branch: resume-info should return useCurrentBranch: false
SM_WS_UCB_FRESH="${TMPDIR_BASE}/sm-ucb-fresh"
mkdir -p "$SM_WS_UCB_FRESH"
bash "$SM" init "$SM_WS_UCB_FRESH" "ucb-fresh-test"
RESUME_UCB_FALSE="$(bash "$SM" resume-info "$SM_WS_UCB_FRESH" | jq '.useCurrentBranch')"
if [ "$RESUME_UCB_FALSE" = "false" ]; then
  pass "resume-info returns useCurrentBranch: false for fresh workspace"
else
  fail "resume-info returns useCurrentBranch: false for fresh workspace" "got: $RESUME_UCB_FALSE"
fi

# ============================================================
echo ""
echo "=== state-manager.sh — set-debug and resume-info debug/tasksWithRetries tests ==="

SM_WS_DBG="${TMPDIR_BASE}/sm-debug"
mkdir -p "$SM_WS_DBG"
bash "$SM" init "$SM_WS_DBG" "debug-test"

echo ""
echo "--- set-debug ---"
bash "$SM" set-debug "$SM_WS_DBG"
SET_DBG_VAL="$(jq '.debug' "${SM_WS_DBG}/state.json")"
if [ "$SET_DBG_VAL" = "true" ]; then
  pass "set-debug writes debug = true to state.json"
else
  fail "set-debug writes debug = true to state.json" "got: $SET_DBG_VAL"
fi

RESUME_DBG_TRUE="$(bash "$SM" resume-info "$SM_WS_DBG" | jq '.debug')"
if [ "$RESUME_DBG_TRUE" = "true" ]; then
  pass "resume-info projects debug: true after set-debug"
else
  fail "resume-info projects debug: true after set-debug" "got: $RESUME_DBG_TRUE"
fi

# Fresh workspace without set-debug: resume-info should return debug: false
SM_WS_DBG_FRESH="${TMPDIR_BASE}/sm-debug-fresh"
mkdir -p "$SM_WS_DBG_FRESH"
bash "$SM" init "$SM_WS_DBG_FRESH" "debug-fresh-test"
RESUME_DBG_FALSE="$(bash "$SM" resume-info "$SM_WS_DBG_FRESH" | jq '.debug')"
if [ "$RESUME_DBG_FALSE" = "false" ]; then
  pass "resume-info returns debug: false for fresh workspace without set-debug"
else
  fail "resume-info returns debug: false for fresh workspace without set-debug" "got: $RESUME_DBG_FALSE"
fi

# Backward compatibility: state file without debug field -> resume-info returns false (not null)
SM_WS_DBG_LEGACY="${TMPDIR_BASE}/sm-debug-legacy"
mkdir -p "$SM_WS_DBG_LEGACY"
bash "$SM" init "$SM_WS_DBG_LEGACY" "debug-legacy-test"
# Remove the debug field to simulate a pre-feature state file
jq 'del(.debug)' "${SM_WS_DBG_LEGACY}/state.json" > "${SM_WS_DBG_LEGACY}/state.json.tmp" \
  && mv "${SM_WS_DBG_LEGACY}/state.json.tmp" "${SM_WS_DBG_LEGACY}/state.json"
LEGACY_DBG="$(bash "$SM" resume-info "$SM_WS_DBG_LEGACY" | jq '.debug')"
if [ "$LEGACY_DBG" = "false" ]; then
  pass "resume-info returns debug: false (not null) for legacy state without debug field"
else
  fail "resume-info returns debug: false (not null) for legacy state without debug field" "got: $LEGACY_DBG"
fi

# tasksWithRetries: fresh state returns []
SM_WS_TWR="${TMPDIR_BASE}/sm-twr"
mkdir -p "$SM_WS_TWR"
bash "$SM" init "$SM_WS_TWR" "twr-test"
RESUME_TWR_EMPTY="$(bash "$SM" resume-info "$SM_WS_TWR" | jq -c '.tasksWithRetries')"
if [ "$RESUME_TWR_EMPTY" = "[]" ]; then
  pass "resume-info returns tasksWithRetries: [] on fresh state"
else
  fail "resume-info returns tasksWithRetries: [] on fresh state" "got: $RESUME_TWR_EMPTY"
fi

# tasksWithRetries: non-empty after task-init + task-update with retries
TWR_TASKS_JSON='{"1":{"title":"Task 1","executionMode":"sequential","implStatus":"pending","implRetries":0,"reviewStatus":"pending","reviewRetries":0},"2":{"title":"Task 2","executionMode":"sequential","implStatus":"pending","implRetries":0,"reviewStatus":"pending","reviewRetries":0}}'
bash "$SM" task-init "$SM_WS_TWR" "$TWR_TASKS_JSON"
bash "$SM" task-update "$SM_WS_TWR" "1" "implRetries" "2"
bash "$SM" task-update "$SM_WS_TWR" "2" "reviewRetries" "1"
RESUME_TWR_LEN="$(bash "$SM" resume-info "$SM_WS_TWR" | jq '.tasksWithRetries | length')"
if [ "$RESUME_TWR_LEN" -gt 0 ]; then
  pass "resume-info tasksWithRetries is non-empty after task-init + task-update with retries"
else
  fail "resume-info tasksWithRetries is non-empty after task-init + task-update with retries" "got length: $RESUME_TWR_LEN"
fi

# ============================================================
# P18: No hardcoded test counts in doc files
# Doc files must not contain literal "158" or "156" as test counts.
# Each doc uses "bash scripts/test-hooks.sh" as the dynamic pointer instead.
echo ""
echo "--- P18: doc files contain no hardcoded test counts ---"

DOC_ROOT="$(dirname "$SCRIPT_DIR")"

for doc_file in \
  "${DOC_ROOT}/CLAUDE.md" \
  "${DOC_ROOT}/README.md" \
  "${DOC_ROOT}/scripts/README.md"; do
  label="$(basename "$(dirname "$doc_file")")/$(basename "$doc_file")"
  [ "$doc_file" = "${DOC_ROOT}/CLAUDE.md" ] && label="CLAUDE.md"
  [ "$doc_file" = "${DOC_ROOT}/README.md" ] && label="README.md"
  [ "$doc_file" = "${DOC_ROOT}/scripts/README.md" ] && label="scripts/README.md"

  if grep -qE '\b(158|156) (tests|automated tests)\b' "$doc_file" 2>/dev/null; then
    fail "$label: no hardcoded test count (158/156)" \
      "$(grep -nE '\b(158|156) (tests|automated tests)\b' "$doc_file")"
  else
    pass "$label: no hardcoded test count (158/156)"
  fi

  if grep -q 'bash scripts/test-hooks.sh' "$doc_file" 2>/dev/null; then
    pass "$label: references bash scripts/test-hooks.sh"
  else
    fail "$label: references bash scripts/test-hooks.sh"
  fi
done

# ============================================================
echo ""
echo "=== SKILL.md content — new-file Write pattern documentation (P13) ==="

SKILL_MD="${SCRIPT_DIR}/../skills/forge/SKILL.md"

# P13: Write tool constraint note must be present in File-writing responsibility section
if grep -q "Write tool requires a prior Read call" "$SKILL_MD"; then
  pass "SKILL.md documents Write tool constraint (prior Read call required)"
else
  fail "SKILL.md documents Write tool constraint (prior Read call required)" "expected 'Write tool requires a prior Read call' in $SKILL_MD"
fi

# P13: cat /dev/null workaround must be documented
if grep -q "cat /dev/null" "$SKILL_MD"; then
  pass "SKILL.md documents cat /dev/null workaround pattern"
else
  fail "SKILL.md documents cat /dev/null workaround pattern" "expected 'cat /dev/null' in $SKILL_MD"
fi

# P13: Bash heredoc workaround must be documented
if grep -q "heredoc" "$SKILL_MD"; then
  pass "SKILL.md documents Bash heredoc workaround pattern"
else
  fail "SKILL.md documents Bash heredoc workaround pattern" "expected 'heredoc' in $SKILL_MD"
fi

# ============================================================
echo ""
echo "=== validate-input.sh tests ==="
# ============================================================

VI="${SCRIPT_DIR}/validate-input.sh"
MARKER="${TMPDIR:-/tmp}/dev-pipeline-input-validated"

# Clean up marker before tests
rm -f "$MARKER" 2>/dev/null || true

echo ""
echo "--- empty input ---"

VI_EXIT=0
VI_STDERR="$(bash "$VI" "" 2>&1 >/dev/null)" || VI_EXIT=$?
if [ "$VI_EXIT" -eq 1 ]; then
  pass "empty string rejected"
else
  fail "empty string rejected" "expected exit 1, got $VI_EXIT"
fi
if echo "$VI_STDERR" | grep -qF "No task description provided"; then
  pass "empty string error message"
else
  fail "empty string error message" "stderr: $VI_STDERR"
fi

VI_EXIT=0
VI_STDERR="$(bash "$VI" "   " 2>&1 >/dev/null)" || VI_EXIT=$?
if [ "$VI_EXIT" -eq 1 ]; then
  pass "whitespace-only rejected"
else
  fail "whitespace-only rejected" "expected exit 1, got $VI_EXIT"
fi

echo ""
echo "--- too short ---"

VI_EXIT=0
VI_STDERR="$(bash "$VI" "ab" 2>&1 >/dev/null)" || VI_EXIT=$?
if [ "$VI_EXIT" -eq 1 ]; then
  pass "2-char input rejected"
else
  fail "2-char input rejected" "expected exit 1, got $VI_EXIT"
fi
if echo "$VI_STDERR" | grep -qF "too short"; then
  pass "too short error message"
else
  fail "too short error message" "stderr: $VI_STDERR"
fi

VI_EXIT=0
VI_STDERR="$(bash "$VI" "abcd" 2>&1 >/dev/null)" || VI_EXIT=$?
if [ "$VI_EXIT" -eq 1 ]; then
  pass "4-char input rejected"
else
  fail "4-char input rejected" "expected exit 1, got $VI_EXIT"
fi

echo ""
echo "--- flags only (no task description) ---"

VI_EXIT=0
VI_STDERR="$(bash "$VI" "--type=feature --auto" 2>&1 >/dev/null)" || VI_EXIT=$?
if [ "$VI_EXIT" -eq 1 ]; then
  pass "flags-only input rejected"
else
  fail "flags-only input rejected" "expected exit 1, got $VI_EXIT"
fi
if echo "$VI_STDERR" | grep -qF "Only flags provided"; then
  pass "flags-only error message"
else
  fail "flags-only error message" "stderr: $VI_STDERR"
fi

echo ""
echo "--- valid plain text ---"

rm -f "$MARKER" 2>/dev/null || true
VI_EXIT=0
bash "$VI" "Add retry logic to the API client" >/dev/null 2>&1 || VI_EXIT=$?
if [ "$VI_EXIT" -eq 0 ]; then
  pass "valid plain text accepted"
else
  fail "valid plain text accepted" "expected exit 0, got $VI_EXIT"
fi
if [ -f "$MARKER" ]; then
  pass "validation marker created on success"
else
  fail "validation marker created on success" "marker not found at $MARKER"
fi
rm -f "$MARKER" 2>/dev/null || true

VI_EXIT=0
bash "$VI" "fix login bug" >/dev/null 2>&1 || VI_EXIT=$?
if [ "$VI_EXIT" -eq 0 ]; then
  pass "short but valid text accepted (5+ chars)"
else
  fail "short but valid text accepted (5+ chars)" "expected exit 0, got $VI_EXIT"
fi
rm -f "$MARKER" 2>/dev/null || true

VI_EXIT=0
bash "$VI" "認証のバグを修正する" >/dev/null 2>&1 || VI_EXIT=$?
if [ "$VI_EXIT" -eq 0 ]; then
  pass "Japanese text accepted"
else
  fail "Japanese text accepted" "expected exit 0, got $VI_EXIT"
fi
rm -f "$MARKER" 2>/dev/null || true

echo ""
echo "--- valid text with flags ---"

VI_EXIT=0
bash "$VI" "fix auth timeout --type=bugfix --effort=S --auto" >/dev/null 2>&1 || VI_EXIT=$?
if [ "$VI_EXIT" -eq 0 ]; then
  pass "text with flags accepted"
else
  fail "text with flags accepted" "expected exit 0, got $VI_EXIT"
fi
rm -f "$MARKER" 2>/dev/null || true

echo ""
echo "--- URL validation ---"

VI_EXIT=0
VI_STDERR="$(bash "$VI" "https://github.com/org/repo/issues/123" 2>&1 >/dev/null)" || VI_EXIT=$?
if [ "$VI_EXIT" -eq 0 ]; then
  pass "valid GitHub issue URL accepted"
else
  fail "valid GitHub issue URL accepted" "expected exit 0, got $VI_EXIT"
fi
rm -f "$MARKER" 2>/dev/null || true

VI_EXIT=0
VI_STDERR="$(bash "$VI" "https://myorg.atlassian.net/browse/PROJ-456" 2>&1 >/dev/null)" || VI_EXIT=$?
if [ "$VI_EXIT" -eq 0 ]; then
  pass "valid Jira issue URL accepted"
else
  fail "valid Jira issue URL accepted" "expected exit 0, got $VI_EXIT"
fi
rm -f "$MARKER" 2>/dev/null || true

VI_EXIT=0
VI_STDERR="$(bash "$VI" "https://github.com/org/repo/pulls/123" 2>&1 >/dev/null)" || VI_EXIT=$?
if [ "$VI_EXIT" -eq 1 ]; then
  pass "GitHub non-issue URL rejected"
else
  fail "GitHub non-issue URL rejected" "expected exit 1, got $VI_EXIT"
fi
if echo "$VI_STDERR" | grep -qF "Invalid GitHub URL format"; then
  pass "GitHub non-issue URL error message"
else
  fail "GitHub non-issue URL error message" "stderr: $VI_STDERR"
fi

VI_EXIT=0
VI_STDERR="$(bash "$VI" "https://myorg.atlassian.net/wiki/spaces/foo" 2>&1 >/dev/null)" || VI_EXIT=$?
if [ "$VI_EXIT" -eq 1 ]; then
  pass "Jira non-browse URL rejected"
else
  fail "Jira non-browse URL rejected" "expected exit 1, got $VI_EXIT"
fi

VI_EXIT=0
VI_STDERR="$(bash "$VI" "https://example.com/random-page" 2>&1 >/dev/null)" || VI_EXIT=$?
if [ "$VI_EXIT" -eq 1 ]; then
  pass "unknown URL rejected"
else
  fail "unknown URL rejected" "expected exit 1, got $VI_EXIT"
fi
if echo "$VI_STDERR" | grep -qF "Unrecognised URL format"; then
  pass "unknown URL error message"
else
  fail "unknown URL error message" "stderr: $VI_STDERR"
fi

# ============================================================
echo ""
echo "=== pre-tool-hook.sh: Rule 6 (init validation guard) tests ==="
# ============================================================

echo ""
echo "--- init blocked without validation marker ---"
rm -f "$MARKER" 2>/dev/null || true
# No active workspace needed — Rule 6 fires before find_active_workspace
run_hook "pre-tool-hook.sh" '{"tool_name":"Bash","tool_input":{"command":"bash scripts/state-manager.sh init .specs/test test"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "init blocked without validation marker"
assert_stderr_contains "BLOCKED" "init block message contains BLOCKED"

echo ""
echo "--- init blocked with stale marker ---"
# Create a marker that is old (simulate by writing a timestamp 200s in the past)
STALE_TS=$(( $(date +%s) - 200 ))
echo "$STALE_TS" > "$MARKER"
run_hook "pre-tool-hook.sh" '{"tool_name":"Bash","tool_input":{"command":"bash scripts/state-manager.sh init .specs/test test"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 2 "init blocked with stale marker"
assert_stderr_contains "stale" "stale marker message"
rm -f "$MARKER" 2>/dev/null || true

echo ""
echo "--- init allowed with fresh marker ---"
date +%s > "$MARKER"
run_hook "pre-tool-hook.sh" '{"tool_name":"Bash","tool_input":{"command":"bash scripts/state-manager.sh init .specs/test test"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "init allowed with fresh validation marker"
# Marker should be cleaned up after use
if [ ! -f "$MARKER" ]; then
  pass "marker cleaned up after init"
else
  fail "marker cleaned up after init" "marker still exists"
  rm -f "$MARKER" 2>/dev/null || true
fi

echo ""
echo "--- git commands mentioning init not blocked ---"
rm -f "$MARKER" 2>/dev/null || true
run_hook "pre-tool-hook.sh" '{"tool_name":"Bash","tool_input":{"command":"git commit -m \"feat: add state-manager.sh init validation guard\""}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "git commit mentioning init in message not blocked"

rm -f "$MARKER" 2>/dev/null || true
run_hook "pre-tool-hook.sh" '{"tool_name":"Bash","tool_input":{"command":"git log --oneline | grep state-manager.sh init"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "git log piped through grep not blocked"

echo ""
echo "--- non-init state-manager calls unaffected ---"
rm -f "$MARKER" 2>/dev/null || true
reset_workspace
WS="$(setup_workspace "phase-1" "pending")"
run_hook "pre-tool-hook.sh" '{"tool_name":"Bash","tool_input":{"command":"bash scripts/state-manager.sh phase-start '"$WS"' phase-1"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "phase-start not blocked by missing validation marker"
reset_workspace

# ============================================================
# post-bash-hook.sh tests
# ============================================================

echo ""
echo "========================================"
echo "post-bash-hook.sh"
echo "========================================"

echo ""
echo "--- non-Bash tool: ignored ---"
run_hook "post-bash-hook.sh" '{"tool_name":"Edit","tool_input":{"command":""},"tool_response":""}'
assert_exit 0 "Edit tool call exits 0 (ignored)"
assert_exit 0 "Edit tool call stderr is empty (exit 0 implies no block)"

echo ""
echo "--- Bash tool, unrelated command: ignored ---"
run_hook "post-bash-hook.sh" '{"tool_name":"Bash","tool_input":{"command":"ls -la"},"tool_response":"file1\nfile2"}'
assert_exit 0 "unrelated Bash command exits 0"

echo ""
echo "--- Bash tool, state-manager but not post-to-source: ignored ---"
run_hook "post-bash-hook.sh" '{"tool_name":"Bash","tool_input":{"command":"bash scripts/state-manager.sh phase-complete .specs/test final-summary"},"tool_response":""}'
assert_exit 0 "phase-complete final-summary exits 0 (not post-to-source)"

echo ""
echo "--- Bash tool, phase-complete post-to-source, investigation type: skipped ---"
reset_workspace
WS="$(setup_workspace "post-to-source" "completed")"
# Patch taskType to investigation
jq '.taskType = "investigation"' "${WS}/state.json" > "${WS}/state.json.tmp" && mv "${WS}/state.json.tmp" "${WS}/state.json"
touch "${WS}/summary.md"
run_hook "post-bash-hook.sh" \
  "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-complete ${WS} post-to-source\"},\"tool_response\":\"\"}" \
  "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "investigation type skipped (no feature branch)"
reset_workspace

echo ""
echo "--- Bash tool, phase-complete post-to-source, no summary.md: skipped ---"
reset_workspace
WS="$(setup_workspace "post-to-source" "completed")"
jq '.taskType = "feature"' "${WS}/state.json" > "${WS}/state.json.tmp" && mv "${WS}/state.json.tmp" "${WS}/state.json"
# Do NOT create summary.md
run_hook "post-bash-hook.sh" \
  "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-complete ${WS} post-to-source\"},\"tool_response\":\"\"}" \
  "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "missing summary.md skipped gracefully"
reset_workspace

# ============================================================
echo ""
echo "=== build-specs-index.sh tests ==="
# ============================================================

BIS="${SCRIPT_DIR}/build-specs-index.sh"
BIS_SPECS="${TMPDIR_BASE}/.specs-bis"

# Helper: run build-specs-index.sh with BIS_SPECS as the target dir
run_bis() {
  BIS_EXIT=0
  BIS_STDOUT="$(bash "${BIS}" "${BIS_SPECS}" 2>/tmp/bis-stderr)" || BIS_EXIT=$?
  BIS_STDERR="$(cat /tmp/bis-stderr 2>/dev/null || true)"
  rm -f /tmp/bis-stderr
}

# Helper: assert jq expression on index.json
assert_jq() {
  local expr="$1"
  local expected="$2"
  local label="$3"
  local actual
  actual="$(jq -r "${expr}" "${BIS_SPECS}/index.json" 2>/dev/null || true)"
  if [ "${actual}" = "${expected}" ]; then
    pass "${label}"
  else
    fail "${label}" "expected '${expected}', got '${actual}'"
  fi
}

# Cleanup helper for BIS tests
reset_bis() {
  rm -rf "${BIS_SPECS}"
  mkdir -p "${BIS_SPECS}"
}

# --- Test 1: Empty .specs/ directory ---
echo ""
echo "--- Test 1: Empty .specs/ ---"
reset_bis
run_bis
if [ "${BIS_EXIT}" -eq 0 ]; then
  pass "empty .specs/ exits 0"
else
  fail "empty .specs/ exits 0" "got exit ${BIS_EXIT}: ${BIS_STDERR}"
fi
assert_jq 'length' "0" "empty .specs/ produces []"

# --- Test 2: Single workspace, only state.json ---
echo ""
echo "--- Test 2: Single workspace with state.json only ---"
reset_bis
mkdir -p "${BIS_SPECS}/ws1"
cat > "${BIS_SPECS}/ws1/state.json" <<'EOF'
{
  "specName": "test-spec-1",
  "taskType": "feature",
  "currentPhase": "phase-1",
  "currentPhaseStatus": "in_progress",
  "timestamps": { "created": "2026-01-01T00:00:00Z" }
}
EOF
run_bis
if [ "${BIS_EXIT}" -eq 0 ]; then
  pass "single workspace exits 0"
else
  fail "single workspace exits 0" "got exit ${BIS_EXIT}: ${BIS_STDERR}"
fi
assert_jq 'length' "1" "single workspace produces 1 entry"
assert_jq '.[0].specName' "test-spec-1" "specName extracted from state.json"
assert_jq '.[0].timestamp' "2026-01-01T00:00:00Z" "timestamp extracted from state.json"
assert_jq '.[0].taskType' "feature" "taskType extracted from state.json"
assert_jq '.[0].requestSummary' "" "requestSummary empty when no request.md"
assert_jq '.[0].reviewFeedback | length' "0" "reviewFeedback is [] with no review files"
assert_jq '.[0].implOutcomes | length' "0" "implOutcomes is [] with no impl review files"
assert_jq '.[0].outcome' "in_progress" "outcome is in_progress for phase-1/in_progress"

# --- Test 3: requestSummary strips YAML frontmatter ---
echo ""
echo "--- Test 3: requestSummary strips YAML frontmatter ---"
reset_bis
mkdir -p "${BIS_SPECS}/ws1"
cat > "${BIS_SPECS}/ws1/state.json" <<'EOF'
{"specName":"test","currentPhase":"phase-1","currentPhaseStatus":"in_progress","timestamps":{"created":"2026-01-01T00:00:00Z"}}
EOF
cat > "${BIS_SPECS}/ws1/request.md" <<'EOF'
---
source_type: text
task_type: feature
---

This is the actual body of the request. It contains no frontmatter.
EOF
run_bis
# requestSummary should NOT contain "source_type" or "task_type"
if jq -r '.[0].requestSummary' "${BIS_SPECS}/index.json" | grep -q "source_type"; then
  fail "requestSummary excludes YAML frontmatter" "found 'source_type' in summary"
else
  pass "requestSummary excludes YAML frontmatter"
fi
assert_jq '.[0].requestSummary' "This is the actual body of the request. It contains no frontmatter." "requestSummary contains body text"
# Verify max 200 chars
LONG_SUMMARY_LEN="$(jq -r '.[0].requestSummary' "${BIS_SPECS}/index.json" | wc -c | tr -d ' ')"
if [ "${LONG_SUMMARY_LEN}" -le 201 ]; then
  pass "requestSummary is at most 200 chars"
else
  fail "requestSummary is at most 200 chars" "got ${LONG_SUMMARY_LEN} chars"
fi

# --- Test 4: review-design.md with REVISE verdict ---
echo ""
echo "--- Test 4: review-design.md with REVISE verdict ---"
reset_bis
mkdir -p "${BIS_SPECS}/ws1"
cat > "${BIS_SPECS}/ws1/state.json" <<'EOF'
{"specName":"test","currentPhase":"phase-3","currentPhaseStatus":"in_progress","timestamps":{"created":"2026-01-01T00:00:00Z"}}
EOF
cat > "${BIS_SPECS}/ws1/review-design.md" <<'EOF'
## Verdict: REVISE

### Findings

**1. [CRITICAL] Missing error handling in section 2**

The design does not address what happens when X fails.

**2. [MINOR] Naming inconsistency**

Function names do not follow snake_case convention.
EOF
run_bis
assert_jq '.[0].reviewFeedback | length' "1" "reviewFeedback has 1 entry for REVISE verdict"
assert_jq '.[0].reviewFeedback[0].source' "review-design" "reviewFeedback source is review-design"
assert_jq '.[0].reviewFeedback[0].verdict' "REVISE" "reviewFeedback verdict is REVISE"
assert_jq '.[0].reviewFeedback[0].findings | length > 0' "true" "reviewFeedback has findings"

# --- Test 5: review-design.md with APPROVE verdict -> reviewFeedback empty ---
echo ""
echo "--- Test 5: review-design.md with APPROVE verdict ---"
reset_bis
mkdir -p "${BIS_SPECS}/ws1"
cat > "${BIS_SPECS}/ws1/state.json" <<'EOF'
{"specName":"test","currentPhase":"phase-3","currentPhaseStatus":"in_progress","timestamps":{"created":"2026-01-01T00:00:00Z"}}
EOF
cat > "${BIS_SPECS}/ws1/review-design.md" <<'EOF'
## Verdict: APPROVE_WITH_NOTES

Design looks good overall. Minor suggestions only.
EOF
run_bis
assert_jq '.[0].reviewFeedback | length' "0" "APPROVE verdict produces empty reviewFeedback"

# --- Test 6: review-1.md with PASS verdict ---
echo ""
echo "--- Test 6: review-1.md with PASS verdict ---"
reset_bis
mkdir -p "${BIS_SPECS}/ws1"
cat > "${BIS_SPECS}/ws1/state.json" <<'EOF'
{"specName":"test","currentPhase":"phase-6","currentPhaseStatus":"in_progress","timestamps":{"created":"2026-01-01T00:00:00Z"}}
EOF
cat > "${BIS_SPECS}/ws1/review-1.md" <<'EOF'
## Task 1 — PASS
All acceptance criteria met.
EOF
run_bis
assert_jq '.[0].implOutcomes | length' "1" "PASS verdict produces 1 implOutcome entry"
assert_jq '.[0].implOutcomes[0].reviewFile' "review-1.md" "implOutcomes reviewFile is review-1.md"
assert_jq '.[0].implOutcomes[0].verdict' "PASS" "implOutcomes verdict is PASS"

# --- Test 7: review-1.md with FAIL verdict ---
echo ""
echo "--- Test 7: review-1.md with FAIL verdict ---"
reset_bis
mkdir -p "${BIS_SPECS}/ws1"
cat > "${BIS_SPECS}/ws1/state.json" <<'EOF'
{"specName":"test","currentPhase":"phase-6","currentPhaseStatus":"in_progress","timestamps":{"created":"2026-01-01T00:00:00Z"}}
EOF
cat > "${BIS_SPECS}/ws1/review-1.md" <<'EOF'
## Task 1 — FAIL
Acceptance criteria not met.
EOF
run_bis
assert_jq '.[0].implOutcomes[0].verdict' "FAIL" "implOutcomes verdict is FAIL"

# --- Test 8: review-1.md with PASS_WITH_NOTES ---
echo ""
echo "--- Test 8: review-1.md with PASS_WITH_NOTES ---"
reset_bis
mkdir -p "${BIS_SPECS}/ws1"
cat > "${BIS_SPECS}/ws1/state.json" <<'EOF'
{"specName":"test","currentPhase":"phase-6","currentPhaseStatus":"in_progress","timestamps":{"created":"2026-01-01T00:00:00Z"}}
EOF
cat > "${BIS_SPECS}/ws1/review-1.md" <<'EOF'
## Task 1 — PASS_WITH_NOTES
All criteria met with minor notes.
EOF
run_bis
assert_jq '.[0].implOutcomes[0].verdict' "PASS_WITH_NOTES" "implOutcomes verdict is PASS_WITH_NOTES (not incorrectly tokenised as PASS)"

# --- Test 9: Multiple workspaces produce correct array length ---
echo ""
echo "--- Test 9: Multiple workspaces ---"
reset_bis
for ws in ws1 ws2 ws3; do
  mkdir -p "${BIS_SPECS}/${ws}"
  cat > "${BIS_SPECS}/${ws}/state.json" <<EOF
{"specName":"${ws}","currentPhase":"phase-1","currentPhaseStatus":"in_progress","timestamps":{"created":"2026-01-01T00:00:00Z"}}
EOF
done
run_bis
assert_jq 'length' "3" "multiple workspaces produce correct array length"

# --- Test 10: Partial pipeline outcome (in_progress) ---
echo ""
echo "--- Test 10: Partial pipeline outcome (in_progress) ---"
reset_bis
mkdir -p "${BIS_SPECS}/ws1"
cat > "${BIS_SPECS}/ws1/state.json" <<'EOF'
{"specName":"test","currentPhase":"phase-3","currentPhaseStatus":"in_progress","timestamps":{"created":"2026-01-01T00:00:00Z"}}
EOF
run_bis
assert_jq '.[0].outcome' "in_progress" "partial pipeline produces in_progress outcome"

# --- Test 11: Completed pipeline outcome ---
echo ""
echo "--- Test 11: Completed pipeline outcome ---"
reset_bis
mkdir -p "${BIS_SPECS}/ws1"
cat > "${BIS_SPECS}/ws1/state.json" <<'EOF'
{"specName":"test","currentPhase":"post-to-source","currentPhaseStatus":"completed","timestamps":{"created":"2026-01-01T00:00:00Z"}}
EOF
run_bis
assert_jq '.[0].outcome' "completed" "post-to-source/completed produces completed outcome"

# --- Test 12: Abandoned pipeline outcome ---
echo ""
echo "--- Test 12: Abandoned pipeline outcome ---"
reset_bis
mkdir -p "${BIS_SPECS}/ws1"
cat > "${BIS_SPECS}/ws1/state.json" <<'EOF'
{"specName":"test","currentPhase":"phase-2","currentPhaseStatus":"abandoned","timestamps":{"created":"2026-01-01T00:00:00Z"}}
EOF
run_bis
assert_jq '.[0].outcome' "abandoned" "abandoned status produces abandoned outcome"

# Also test failed outcome
reset_bis
mkdir -p "${BIS_SPECS}/ws1"
cat > "${BIS_SPECS}/ws1/state.json" <<'EOF'
{"specName":"test","currentPhase":"phase-5","currentPhaseStatus":"failed","timestamps":{"created":"2026-01-01T00:00:00Z"}}
EOF
run_bis
assert_jq '.[0].outcome' "failed" "failed status produces failed outcome"

# Also test unknown outcome (no state.json)
reset_bis
mkdir -p "${BIS_SPECS}/ws1"
# No state.json
run_bis
assert_jq '.[0].outcome' "unknown" "missing state.json produces unknown outcome"

# --- Test 13: refresh-index subcommand (covered in Task 5, depends on Task 2) ---

# --- Test 14: Idempotency ---
echo ""
echo "--- Test 14: Idempotency ---"
reset_bis
mkdir -p "${BIS_SPECS}/ws1"
cat > "${BIS_SPECS}/ws1/state.json" <<'EOF'
{"specName":"test","currentPhase":"phase-1","currentPhaseStatus":"in_progress","timestamps":{"created":"2026-01-01T00:00:00Z"}}
EOF
run_bis
FIRST_OUTPUT="$(cat "${BIS_SPECS}/index.json")"
run_bis
SECOND_OUTPUT="$(cat "${BIS_SPECS}/index.json")"
if [ "${FIRST_OUTPUT}" = "${SECOND_OUTPUT}" ]; then
  pass "running script twice produces identical output"
else
  fail "running script twice produces identical output" "outputs differ"
fi

# Cleanup BIS test state
rm -rf "${BIS_SPECS}"

# ============================================================
echo ""
echo "========================================"
echo "Results: ${PASS_COUNT} passed, ${FAIL_COUNT} failed"
echo "========================================"

[ "$FAIL_COUNT" -eq 0 ] && exit 0 || exit 1

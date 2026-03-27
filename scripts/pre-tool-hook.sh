#!/usr/bin/env bash
# pre-tool-hook.sh — PreToolUse hook for claude-forge
#
# Enforces:
# 1. Read-only mode during Phase 1-2 (blocks Edit/Write on non-workspace files)
# 2. No git commit during parallel Phase 5 tasks
# 3. Artifact guards — deterministic checks that prevent state advancement
# 4. No git checkout -b when state.json.branch is already set
# 5. No git checkout/switch to main/master during active pipeline
#    3a. phase-complete requires the phase's artifact file to exist
#    3b. task-update reviewStatus completed_pass requires review-{N}.md to exist
#    3c. phase-start phase-5 requires tasks to be initialized (non-empty)
#    3d. phase-log warns on duplicate entries (non-blocking, retries are valid)
#    3e. phase-complete checkpoint-a/checkpoint-b requires currentPhaseStatus == "awaiting_human"
#    3f. Warn when set-effort has not been called before phase-start phase-1
#        (non-blocking, same pattern as 3d)
#    3g. task-init requires checkpoint-b to be completed or skipped
#        (blocking, prevents task decomposition before design is approved)
#    3h. Warn when taskType not set before phase-start phase-1
#        (non-blocking, same pattern as 3f)
#    3i. Warn when phase-log entry is missing before phase-complete
#        (non-blocking, warning only for logged phases)
#    3j. phase-complete checkpoint-a/b blocked when checkpointRevisionPending[<checkpoint>]
#        is true; orchestrator must call clear-revision-pending first (blocking)
#
# Receives JSON on stdin from Claude Code. Outputs JSON to stdout for flow control.
# Exit 0 = allow (default), Exit 2 + stderr = block.
#
# Design: fail-open. If jq is missing or any parsing fails, we allow the action
# rather than blocking legitimate work.

set -uo pipefail

# Fail-open: if jq is not installed, allow
command -v jq >/dev/null 2>&1 || exit 0

# Read hook input
INPUT="$(cat)"

# Extract tool name from input
TOOL_NAME="$(echo "$INPUT" | jq -r '.tool_name // empty' 2>/dev/null || true)"
[ -z "$TOOL_NAME" ] && exit 0

# Helper: resolve workspace path (may be relative to CLAUDE_PROJECT_DIR)
resolve_ws() {
  local ws="$1"
  if [[ "$ws" != /* ]]; then
    echo "${CLAUDE_PROJECT_DIR:-.}/${ws}"
  else
    echo "$ws"
  fi
}

# Find active pipeline workspace (most recently updated non-completed pipeline)
#
# INTENTIONAL DIVERGENCE from post-agent-hook.sh and stop-hook.sh:
#   Filter predicate: status != "completed" AND status != "abandoned" AND non-empty
#   Accepted statuses: in_progress, pending, awaiting_human
#
#   Rationale: pre-tool-hook.sh must fire for all active pipelines regardless of
#   the exact phase status, so that read-only enforcement (Rule 1), commit blocking
#   (Rule 2), and artifact guards (Rule 3) apply at every step of the pipeline,
#   including when the pipeline is paused at a checkpoint (awaiting_human) or has
#   not yet started its first phase (pending).
find_active_workspace() {
  local project_dir="${CLAUDE_PROJECT_DIR:-.}"
  local latest_file=""
  local latest_ts=""
  for state_file in "${project_dir}"/.specs/*/state.json; do
    [ -f "$state_file" ] || continue
    local status ts
    status="$(jq -r '.currentPhaseStatus // empty' "$state_file" 2>/dev/null || true)"
    if [ "$status" != "completed" ] && [ "$status" != "abandoned" ] && [ -n "$status" ]; then
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

# --- Rule 1: Read-only enforcement for Phase 1-2 ---
if [ "$CURRENT_PHASE" = "phase-1" ] || [ "$CURRENT_PHASE" = "phase-2" ]; then
  if [ "$CURRENT_STATUS" = "in_progress" ]; then
    case "$TOOL_NAME" in
      Edit|Write)
        # Allow writes to workspace directory (artifact files)
        TARGET_FILE="$(echo "$INPUT" | jq -r '.tool_input.file_path // empty' 2>/dev/null || true)"
        if [ -n "$TARGET_FILE" ]; then
          case "$TARGET_FILE" in
            "${WORKSPACE}"*) ;; # workspace file — allow
            *)
              echo "Phase 1-2 is read-only. File modifications to source code are blocked during analysis/investigation." >&2
              exit 2
              ;;
          esac
        fi
        ;;
    esac
  fi
fi

# --- Rule 3 sub-check functions ---

# 3a + 3i: Artifact existence before phase-complete (3a blocking) +
#           warn when phase-log entry is missing before phase-complete (3i non-blocking)
check_artifact_guard() {
  [ -z "$PC_MATCH" ] && return 0

  REQUIRED=""
  case "$PC_PHASE" in
    phase-1)       REQUIRED="analysis.md" ;;
    phase-2)       REQUIRED="investigation.md" ;;
    phase-3)       REQUIRED="design.md" ;;
    phase-3b)      REQUIRED="review-design.md" ;;
    phase-4)       REQUIRED="tasks.md" ;;
    phase-4b)      REQUIRED="review-tasks.md" ;;
    phase-7)       REQUIRED="comprehensive-review.md" ;;
    final-summary) REQUIRED="summary.md" ;;
  esac

  if [ -n "$REQUIRED" ] && [ ! -f "${PC_WS}/${REQUIRED}" ]; then
    echo "BLOCKED: ${REQUIRED} must exist before completing ${PC_PHASE}. Write the artifact file first." >&2
    exit 2
  fi

  # 3i. Warn when phase-log entry is missing before phase-complete (non-blocking)
  # Only fires for phases that are expected to call phase-log before phase-complete.
  # Checkpoint phases and admin phases are excluded from the allowlist.
  PLOG_PHASES="phase-1 phase-2 phase-3 phase-3b phase-4 phase-4b phase-7 final-verification"
  if echo "$PLOG_PHASES" | grep -qw "$PC_PHASE"; then
    PLOG_COUNT="$(jq -r --arg p "$PC_PHASE" '[.phaseLog[] | select(.phase == $p)] | length' "${PC_WS}/state.json" 2>/dev/null || echo "0")"
    PLOG_COUNT="$(echo "$PLOG_COUNT" | tr -d '[:space:]')"
    if [ "${PLOG_COUNT:-0}" -eq 0 ]; then
      echo "WARNING: no phase-log entry for '${PC_PHASE}'. Call 'phase-log <workspace> <phase> <tokens> <duration_ms> <model>' before completing this phase." >&2
    fi
  fi
}

# 3b: review-{N}.md must exist before marking review as passed
check_review_file_guard() {
  TU_MATCH="$(echo "$COMMAND" | grep -oE 'task-update[[:space:]]+[^[:space:]]+[[:space:]]+[0-9]+[[:space:]]+reviewStatus[[:space:]]+completed_pass' | head -1 || true)"
  if [ -n "$TU_MATCH" ]; then
    TU_WS="$(echo "$TU_MATCH" | awk '{print $2}')"
    TASK_NUM="$(echo "$TU_MATCH" | awk '{print $3}')"
    TU_WS="$(resolve_ws "$TU_WS")"

    if [ ! -f "${TU_WS}/review-${TASK_NUM}.md" ]; then
      echo "BLOCKED: review-${TASK_NUM}.md must exist before marking task ${TASK_NUM} review as passed. Write the review file first." >&2
      exit 2
    fi
  fi
}

# 3c: Tasks must be initialized before phase-5 starts
check_phase5_task_init_guard() {
  PS_MATCH="$(echo "$COMMAND" | grep -oE 'phase-start[[:space:]]+[^[:space:]]+[[:space:]]+phase-5' | head -1 || true)"
  if [ -n "$PS_MATCH" ]; then
    PS_WS="$(echo "$PS_MATCH" | awk '{print $2}')"
    PS_WS="$(resolve_ws "$PS_WS")"

    TASK_COUNT="$(jq -r '.tasks | length' "${PS_WS}/state.json" 2>/dev/null || echo "0")"
    TASK_COUNT="$(echo "$TASK_COUNT" | tr -d '[:space:]')"
    if [ "${TASK_COUNT:-0}" -eq 0 ]; then
      echo "BLOCKED: No tasks initialized in state.json. Run task-init before starting phase-5." >&2
      exit 2
    fi
  fi
}

# 3d: Warn on duplicate phase-log entries (non-blocking)
# Phases and tasks may legitimately retry (REVISE loops, FAIL retries),
# so duplicate entries are warned about but not blocked.
check_phase_log_dup_warn() {
  PL_MATCH="$(echo "$COMMAND" | grep -oE 'phase-log[[:space:]]+[^[:space:]]+[[:space:]]+[^[:space:]]+' | head -1 || true)"
  if [ -n "$PL_MATCH" ]; then
    PL_WS="$(echo "$PL_MATCH" | awk '{print $2}')"
    PL_PHASE="$(echo "$PL_MATCH" | awk '{print $3}')"
    PL_WS="$(resolve_ws "$PL_WS")"

    EXISTING="$(jq -r --arg p "$PL_PHASE" '[.phaseLog[] | select(.phase == $p)] | length' "${PL_WS}/state.json" 2>/dev/null || echo "0")"
    EXISTING="$(echo "$EXISTING" | tr -d '[:space:]')"
    if [ "${EXISTING:-0}" -gt 0 ]; then
      echo "WARNING: phase-log entry for '${PL_PHASE}' already exists. This may be a legitimate retry or an accidental duplicate." >&2
      # Non-blocking: allow the command to proceed (retries are valid)
    fi
  fi
}

# 3f + 3h: Warn when effort not set before phase-start phase-1 (non-blocking)
#           Warn when taskType not set before phase-start phase-1 (non-blocking)
check_phase1_warnings() {
  PS1_MATCH="$(echo "$COMMAND" | grep -oE 'phase-start[[:space:]]+[^[:space:]]+[[:space:]]+phase-1' | head -1 || true)"
  if [ -n "$PS1_MATCH" ]; then
    PS1_WS="$(echo "$PS1_MATCH" | awk '{print $2}')"
    PS1_WS="$(resolve_ws "$PS1_WS")"
    local EFFORT_VAL TASK_TYPE_VAL
    { read -r EFFORT_VAL; read -r TASK_TYPE_VAL; } < <(jq -r '.effort // "null", .taskType // "null"' "${PS1_WS}/state.json" 2>/dev/null || printf 'null\nnull\n')
    if [ "$EFFORT_VAL" = "null" ]; then
      echo "WARNING: effort not set in state.json. Call 'set-effort <workspace> <effort>' during Workspace Setup. Defaulting to M." >&2
    fi
    # 3h. Warn when taskType not set before phase-start phase-1 (non-blocking)
    if [ "$TASK_TYPE_VAL" = "null" ]; then
      echo "WARNING: taskType not set in state.json. Call 'set-task-type <workspace> <type>' during Workspace Setup. Valid values: feature, bugfix, investigation, docs, refactor." >&2
    fi
  fi
}

# 3g: task-init guard: checkpoint-b must be completed or skipped before task-init
# Prevents the orchestrator from initializing tasks without completing Checkpoint B,
# which is the human's last review gate before implementation begins.
check_task_init_guard() {
  TI_MATCH="$(echo "$COMMAND" | grep -oE 'task-init[[:space:]]+[^[:space:]]+' | head -1 || true)"
  if [ -n "$TI_MATCH" ]; then
    TI_WS="$(echo "$TI_MATCH" | awk '{print $2}')"
    TI_WS="$(resolve_ws "$TI_WS")"

    local CP_B_COMPLETED CP_B_SKIPPED
    { read -r CP_B_COMPLETED; read -r CP_B_SKIPPED; } < <(jq -r '([.completedPhases[] | select(. == "checkpoint-b")] | length), ([(.skippedPhases // [])[] | select(. == "checkpoint-b")] | length)' "${TI_WS}/state.json" 2>/dev/null || printf '0\n0\n')
    CP_B_COMPLETED="$(echo "$CP_B_COMPLETED" | tr -d '[:space:]')"
    CP_B_SKIPPED="$(echo "$CP_B_SKIPPED" | tr -d '[:space:]')"

    if [ "${CP_B_COMPLETED:-0}" -eq 0 ] && [ "${CP_B_SKIPPED:-0}" -eq 0 ]; then
      echo "BLOCKED: task-init requires checkpoint-b to be completed or skipped first. Complete Checkpoint B (human approval or auto-approve) before initializing tasks." >&2
      exit 2
    fi
  fi
}

# 3e: Checkpoint guard: phase-complete checkpoint-a/checkpoint-b requires awaiting_human
# Prevents the orchestrator from calling phase-complete on a checkpoint without first
# calling 'state-manager.sh checkpoint' to set currentPhaseStatus = awaiting_human.
# This ensures humans always have the opportunity to review before the checkpoint passes.
# Reads PC_MATCH/PC_WS/PC_PHASE from outer scope.
check_checkpoint_status_guard() {
  [ -z "$PC_MATCH" ] && return 0
  case "$PC_PHASE" in
    checkpoint-a|checkpoint-b)
      CP_STATUS="$(jq -r '.currentPhaseStatus // empty' "${PC_WS}/state.json" 2>/dev/null || true)"
      if [ "$CP_STATUS" != "awaiting_human" ]; then
        echo "BLOCKED: phase-complete ${PC_PHASE} requires currentPhaseStatus == \"awaiting_human\". Call '\$SM checkpoint {workspace} ${PC_PHASE}' first to register the human review pause before completing this checkpoint." >&2
        exit 2
      fi
      ;;
  esac
}

# 3j: Checkpoint revision-pending guard: phase-complete checkpoint-a/b blocked when
# checkpointRevisionPending[<checkpoint>] == true. The orchestrator must call
# '$SM clear-revision-pending <workspace> <checkpoint>' after receiving explicit user
# approval before calling phase-complete. This prevents the orchestrator from completing
# a checkpoint without the user seeing the revised artifact after a REVISE cycle.
# Reads PC_MATCH/PC_WS/PC_PHASE from outer scope.
check_checkpoint_revision_guard() {
  [ -z "$PC_MATCH" ] && return 0
  case "$PC_PHASE" in
    checkpoint-a|checkpoint-b)
      REV_PENDING="$(jq -r --arg k "$PC_PHASE" '.checkpointRevisionPending[$k] // false' "${PC_WS}/state.json" 2>/dev/null || echo "false")"
      if [ "$REV_PENDING" = "true" ]; then
        echo "BLOCKED: phase-complete ${PC_PHASE} requires 'clear-revision-pending' to be called first. The user requested a revision — call '\$SM clear-revision-pending {workspace} ${PC_PHASE}' after receiving explicit user approval, then call phase-complete." >&2
        exit 2
      fi
      ;;
  esac
}

# --- Rule 2 & 3: Bash command checks ---
# Hoist COMMAND extraction once for all Bash rules (avoids redundant jq parsing).
if [ "$TOOL_NAME" = "Bash" ]; then
  COMMAND="$(echo "$INPUT" | jq -r '.tool_input.command // empty' 2>/dev/null || true)"

  # Rule 2: Block git commit during parallel Phase 5
  if [ "$CURRENT_PHASE" = "phase-5" ] && [ "$CURRENT_STATUS" = "in_progress" ]; then
    # Match git commit as a subcommand, handling flags between git and commit
    # e.g., "git commit", "git -c ... commit", "git -C /path commit"
    if echo "$COMMAND" | grep -qE '\bgit\b.*\bcommit\b' && ! echo "$COMMAND" | grep -qE '\bgit\b.*\bcommit-tree\b'; then
      # Check if any parallel tasks are in_progress
      HAS_PARALLEL_ACTIVE="$(jq -r '[.tasks | to_entries[] | select(.value.executionMode == "parallel" and .value.implStatus == "in_progress")] | length' "$STATE_FILE" 2>/dev/null | tr -d '[:space:]')"
      HAS_PARALLEL_ACTIVE="${HAS_PARALLEL_ACTIVE:-0}"
      if [ "$HAS_PARALLEL_ACTIVE" -gt 0 ]; then
        echo "git commit is blocked during parallel task execution. The orchestrator will batch-commit after all parallel tasks complete." >&2
        exit 2
      fi
    fi
  fi

  # Rule 4: Block git checkout -b when state.json.branch is already set and differs
  # Prevents implementer agents from creating a new branch when the orchestrator
  # has already recorded the correct branch in state.json.
  # The guard captures the token immediately following -b, handling:
  #   git checkout -b feature/foo
  #   git -C /path checkout -b feature/foo
  #   git checkout --no-track -b feature/foo
  if echo "$COMMAND" | grep -qE '\bgit\b.*\bcheckout\b.*\s-b\s'; then
    RECORDED_BRANCH="$(jq -r '.branch // empty' "$STATE_FILE" 2>/dev/null || true)"
    if [ -n "$RECORDED_BRANCH" ]; then
      # Extract the branch name: the token immediately after -b
      NEW_BRANCH="$(echo "$COMMAND" | grep -oE '\-b\s+[^[:space:]]+' | head -1 | awk '{print $2}')"
      if [ -n "$NEW_BRANCH" ] && [ "$NEW_BRANCH" != "$RECORDED_BRANCH" ]; then
        echo "BLOCKED: git checkout -b ${NEW_BRANCH} rejected. Branch ${RECORDED_BRANCH} is already recorded in state.json. Check out ${RECORDED_BRANCH} instead." >&2
        exit 2
      fi
    fi
  fi

  # Rule 5: Block git checkout/switch to main or master during active pipeline
  # Prevents agents from switching to main mid-pipeline (P16).
  # Matches: git checkout main, git checkout master, git switch main/master,
  #   git checkout origin/main, git -C /path checkout main, git checkout -f main.
  # Uses strip-quotes to avoid false positives on string arguments (e.g., PR body text).
  STRIPPED_CMD="$(echo "$COMMAND" | sed -E "s/'[^']*'//g; s/\"[^\"]*\"//g")"
  if echo "$STRIPPED_CMD" | grep -qE '\bgit\b.*(checkout|switch).*\b(main|master)\b'; then
    echo "BLOCKED: git checkout/switch to main or master is not allowed during an active pipeline. Stay on the feature branch." >&2
    exit 2
  fi

  # Rule 3: Artifact guards for state-manager commands
  # Intercepts Bash calls to state-manager.sh and enforces preconditions
  # that LLM orchestrators may forget (non-deterministic execution).
  # Scoped to the active workspace found above (line 53) — only fires when
  # a pipeline is active, consistent with Rule 1 and Rule 2.
  if echo "$COMMAND" | grep -qF 'state-manager.sh'; then
    PC_MATCH="$(echo "$COMMAND" | grep -oE 'phase-complete[[:space:]]+[^[:space:]]+[[:space:]]+[^[:space:]&;]+' | head -1 || true)"
    if [ -n "$PC_MATCH" ]; then
      PC_WS="$(echo "$PC_MATCH" | awk '{print $2}')"
      PC_PHASE="$(echo "$PC_MATCH" | awk '{print $3}')"
      PC_WS="$(resolve_ws "$PC_WS")"
    fi

    check_artifact_guard
    check_review_file_guard
    check_phase5_task_init_guard
    check_phase_log_dup_warn
    check_phase1_warnings
    check_task_init_guard
    check_checkpoint_status_guard
    check_checkpoint_revision_guard
  fi
fi

# Default: allow
exit 0

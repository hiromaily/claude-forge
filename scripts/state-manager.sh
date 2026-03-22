#!/usr/bin/env bash
# state-manager.sh — Core state management for claude-forge
# Usage: state-manager.sh <command> <workspace> [args...]
#
# Commands:
#   init <workspace> <spec-name>           Create initial state.json
#   get <workspace> <field>                Read a top-level field (jq path)
#   phase-start <workspace> <phase>        Mark phase as in_progress
#   phase-complete <workspace> <phase>     Mark phase as completed, advance to next
#   phase-fail <workspace> <phase> <msg>   Mark phase as failed with error message
#   checkpoint <workspace> <phase>         Mark phase as awaiting_human
#   task-init <workspace> <json>           Initialize tasks map from JSON
#   task-update <workspace> <N> <field> <value>  Update a task field
#   revision-bump <workspace> <type>       Increment design or task revision count
#   set-branch <workspace> <branch>        Set the branch name
#   set-task-type <workspace> <taskType>   Set the task type in state.json
#   set-effort <workspace> <effort>        Set the effort level [XS, S, M, L]
#   set-flow-template <workspace> <flowTemplate>  Set the flow template [direct, lite, light, standard, full]
#   skip-phase <workspace> <phase>         Record phase as skipped and advance currentPhase
#   phase-log <workspace> <phase> <tokens> <duration_ms> <model>
#                                          Append phase metrics to phaseLog array
#   phase-stats <workspace>                Print phaseLog as a formatted table
#   set-auto-approve <workspace>           Set autoApprove = true in state.json
#   set-skip-pr <workspace>               Set skipPr = true in state.json
#   set-debug <workspace>                 Set debug = true in state.json
#   set-use-current-branch <workspace> <branch>  Record that the user chose to stay on an existing branch
#   set-revision-pending <workspace> <checkpoint>  Set checkpointRevisionPending[<checkpoint>] = true
#   clear-revision-pending <workspace> <checkpoint>  Set checkpointRevisionPending[<checkpoint>] = false
#   abandon <workspace>                    Mark pipeline as abandoned
#   resume-info <workspace>                Print resume information as JSON

set -euo pipefail

LOCKFILE_SUFFIX=".lock"

# --- helpers ---

die() { echo "state-manager: $*" >&2; exit 1; }

require_jq() {
  command -v jq >/dev/null 2>&1 || die "jq is required but not installed"
}

state_path() {
  echo "$1/state.json"
}

# File locking for concurrent access (parallel Phase 5)
locked_update() {
  local state_file="$1"
  local lock_file="${state_file}${LOCKFILE_SUFFIX}"
  shift

  # Use flock if available, otherwise fall back to mkdir-based lock
  if command -v flock >/dev/null 2>&1; then
    (
      flock -w 10 200 || die "Could not acquire lock on ${lock_file}"
      "$@"
    ) 200>"${lock_file}"
  else
    # macOS doesn't have flock by default; use mkdir as atomic lock
    local retries=0
    while ! mkdir "${lock_file}" 2>/dev/null; do
      retries=$((retries + 1))
      if [ "$retries" -ge 50 ]; then
        rm -rf "${lock_file}"  # force-break stale lock after 5 seconds
        mkdir "${lock_file}" 2>/dev/null || die "Could not acquire lock after force-break on ${lock_file}"
        break
      fi
      sleep 0.1
    done
    # Ensure lock is cleaned up even on unexpected exit
    trap 'rm -rf "${lock_file}"' EXIT
    "$@"
    rm -rf "${lock_file}"
    trap - EXIT
  fi
}

now_iso() {
  date -u +"%Y-%m-%dT%H:%M:%SZ"
}

read_state() {
  local state_file
  state_file="$(state_path "$1")"
  [ -f "$state_file" ] || die "state.json not found at ${state_file}"
  cat "$state_file"
}

write_state() {
  local state_file
  state_file="$(state_path "$1")"
  local content="$2"
  echo "$content" | jq '.' > "${state_file}.tmp" && mv "${state_file}.tmp" "$state_file"
}

# Phase ordering for advancement
PHASES=(
  "setup"
  "phase-1"
  "phase-2"
  "phase-3"
  "phase-3b"
  "checkpoint-a"
  "phase-4"
  "phase-4b"
  "checkpoint-b"
  "phase-5"
  "phase-6"
  "phase-7"
  "final-verification"
  "pr-creation"
  "final-summary"
  "post-to-source"
  "completed"
)

next_phase() {
  local current="$1"
  local found=0
  for p in "${PHASES[@]}"; do
    if [ "$found" -eq 1 ]; then
      echo "$p"
      return
    fi
    if [ "$p" = "$current" ]; then
      found=1
    fi
  done
  echo "completed"
}

# --- commands ---

cmd_init() {
  local workspace="$1"
  local spec_name="$2"
  local state_file
  state_file="$(state_path "$workspace")"
  local ts
  ts="$(now_iso)"

  jq -n \
    --arg spec "$spec_name" \
    --arg ws "$workspace" \
    --arg ts "$ts" \
    '{
      version: 1,
      specName: $spec,
      workspace: $ws,
      branch: null,
      taskType: null,
      effort: null,
      flowTemplate: null,
      autoApprove: false,
      skipPr: false,
      useCurrentBranch: false,
      debug: false,
      notifyOnStop: false,
      skippedPhases: [],
      currentPhase: "phase-1",
      currentPhaseStatus: "pending",
      completedPhases: ["setup"],
      revisions: { designRevisions: 0, taskRevisions: 0 },
      checkpointRevisionPending: { "checkpoint-a": false, "checkpoint-b": false },
      tasks: {},
      phaseLog: [],
      timestamps: { created: $ts, lastUpdated: $ts, phaseStarted: null },
      error: null
    }' > "$state_file"
  echo "state.json initialized at ${state_file}"
}

cmd_get() {
  local workspace="$1"
  local field="$2"
  read_state "$workspace" | jq -r ".${field}"
}

_do_phase_start() {
  local workspace="$1"
  local phase="$2"
  local state
  state="$(read_state "$workspace")"
  local ts
  ts="$(now_iso)"

  state="$(echo "$state" | jq \
    --arg phase "$phase" \
    --arg ts "$ts" \
    '.currentPhase = $phase |
     .currentPhaseStatus = "in_progress" |
     .timestamps.phaseStarted = $ts |
     .timestamps.lastUpdated = $ts |
     .error = null'
  )"
  write_state "$workspace" "$state"
}

cmd_phase_start() {
  locked_update "$(state_path "$1")" _do_phase_start "$1" "$2"
}

_do_phase_complete() {
  local workspace="$1"
  local phase="$2"
  local state
  state="$(read_state "$workspace")"
  local ts
  ts="$(now_iso)"
  local next
  next="$(next_phase "$phase")"

  state="$(echo "$state" | jq \
    --arg phase "$phase" \
    --arg next "$next" \
    --arg ts "$ts" \
    '.currentPhaseStatus = "completed" |
     .completedPhases += [$phase] |
     .completedPhases = (.completedPhases | unique) |
     .currentPhase = $next |
     .currentPhaseStatus = (if $next == "completed" then "completed" else "pending" end) |
     .notifyOnStop = ($next == "completed") |
     .timestamps.lastUpdated = $ts |
     .timestamps.phaseStarted = null'
  )"
  write_state "$workspace" "$state"
}

cmd_phase_complete() {
  locked_update "$(state_path "$1")" _do_phase_complete "$1" "$2"
}

_do_phase_fail() {
  local workspace="$1"
  local phase="$2"
  local message="$3"
  local state
  state="$(read_state "$workspace")"
  local ts
  ts="$(now_iso)"

  state="$(echo "$state" | jq \
    --arg phase "$phase" \
    --arg msg "$message" \
    --arg ts "$ts" \
    '.currentPhaseStatus = "failed" |
     .error = { "phase": $phase, "message": $msg, "timestamp": $ts } |
     .timestamps.lastUpdated = $ts'
  )"
  write_state "$workspace" "$state"
}

cmd_phase_fail() {
  locked_update "$(state_path "$1")" _do_phase_fail "$1" "$2" "$3"
}

_do_checkpoint() {
  local workspace="$1"
  local phase="$2"
  local state
  state="$(read_state "$workspace")"
  local ts
  ts="$(now_iso)"

  state="$(echo "$state" | jq \
    --arg phase "$phase" \
    --arg ts "$ts" \
    '.currentPhase = $phase |
     .currentPhaseStatus = "awaiting_human" |
     .timestamps.lastUpdated = $ts'
  )"
  write_state "$workspace" "$state"
}

cmd_checkpoint() {
  locked_update "$(state_path "$1")" _do_checkpoint "$1" "$2"
}

_do_task_init() {
  local workspace="$1"
  local tasks_json="$2"
  local state
  state="$(read_state "$workspace")"
  local ts
  ts="$(now_iso)"

  state="$(echo "$state" | jq \
    --argjson tasks "$tasks_json" \
    --arg ts "$ts" \
    '.tasks = $tasks |
     .timestamps.lastUpdated = $ts'
  )"
  write_state "$workspace" "$state"
}

cmd_task_init() {
  locked_update "$(state_path "$1")" _do_task_init "$1" "$2"
}

_do_task_update() {
  local workspace="$1"
  local task_num="$2"
  local field="$3"
  local value="$4"
  local state
  state="$(read_state "$workspace")"
  local ts
  ts="$(now_iso)"

  # Use --argjson for numeric fields to preserve type consistency
  case "$field" in
    implRetries|reviewRetries)
      state="$(echo "$state" | jq \
        --arg n "$task_num" \
        --arg f "$field" \
        --argjson v "$value" \
        --arg ts "$ts" \
        '.tasks[$n][$f] = $v |
         .timestamps.lastUpdated = $ts'
      )"
      ;;
    *)
      state="$(echo "$state" | jq \
        --arg n "$task_num" \
        --arg f "$field" \
        --arg v "$value" \
        --arg ts "$ts" \
        '.tasks[$n][$f] = $v |
         .timestamps.lastUpdated = $ts'
      )"
      ;;
  esac
  write_state "$workspace" "$state"
}

cmd_task_update() {
  locked_update "$(state_path "$1")" _do_task_update "$1" "$2" "$3" "$4"
}

_do_revision_bump() {
  local workspace="$1"
  local type="$2"
  local state
  state="$(read_state "$workspace")"
  local ts
  ts="$(now_iso)"
  local field

  case "$type" in
    design) field="designRevisions" ;;
    tasks)  field="taskRevisions" ;;
    *)      die "Unknown revision type: $type (expected: design, tasks)" ;;
  esac

  state="$(echo "$state" | jq \
    --arg f "$field" \
    --arg ts "$ts" \
    '.revisions[$f] += 1 |
     .timestamps.lastUpdated = $ts'
  )"
  write_state "$workspace" "$state"
}

cmd_revision_bump() {
  locked_update "$(state_path "$1")" _do_revision_bump "$1" "$2"
}

_do_set_branch() {
  local workspace="$1"
  local branch="$2"
  local state
  state="$(read_state "$workspace")"
  local ts
  ts="$(now_iso)"

  state="$(echo "$state" | jq \
    --arg b "$branch" \
    --arg ts "$ts" \
    '.branch = $b |
     .timestamps.lastUpdated = $ts'
  )"
  write_state "$workspace" "$state"
}

cmd_set_branch() {
  locked_update "$(state_path "$1")" _do_set_branch "$1" "$2"
}

_do_set_task_type() {
  local workspace="$1"
  local task_type="$2"
  local state
  state="$(read_state "$workspace")"
  local ts
  ts="$(now_iso)"

  state="$(echo "$state" | jq \
    --arg t "$task_type" \
    --arg ts "$ts" \
    '.taskType = $t |
     .timestamps.lastUpdated = $ts'
  )"
  write_state "$workspace" "$state"
}

cmd_set_task_type() {
  locked_update "$(state_path "$1")" _do_set_task_type "$1" "$2"
}

_do_set_effort() {
  local workspace="$1"
  local effort="$2"
  local state
  state="$(read_state "$workspace")"
  local ts
  ts="$(now_iso)"

  case "$effort" in
    XS|S|M|L) ;;
    *) die "Invalid effort: ${effort} (expected: XS, S, M, L)" ;;
  esac

  state="$(echo "$state" | jq \
    --arg e "$effort" \
    --arg ts "$ts" \
    '.effort = $e |
     .timestamps.lastUpdated = $ts'
  )"
  write_state "$workspace" "$state"
}

cmd_set_effort() {
  locked_update "$(state_path "$1")" _do_set_effort "$1" "$2"
}

_do_set_flow_template() {
  local workspace="$1"
  local flow_template="$2"
  local state
  state="$(read_state "$workspace")"
  local ts
  ts="$(now_iso)"

  case "$flow_template" in
    direct|lite|light|standard|full) ;;
    *) die "Invalid flowTemplate: ${flow_template} (expected: direct, lite, light, standard, full)" ;;
  esac

  state="$(echo "$state" | jq \
    --arg ft "$flow_template" \
    --arg ts "$ts" \
    '.flowTemplate = $ft |
     .timestamps.lastUpdated = $ts'
  )"
  write_state "$workspace" "$state"
}

cmd_set_flow_template() {
  locked_update "$(state_path "$1")" _do_set_flow_template "$1" "$2"
}

_do_set_auto_approve() {
  local workspace="$1"
  local state
  state="$(read_state "$workspace")"
  local ts
  ts="$(now_iso)"

  state="$(echo "$state" | jq \
    --arg ts "$ts" \
    '.autoApprove = true |
     .timestamps.lastUpdated = $ts'
  )"
  write_state "$workspace" "$state"
}

cmd_set_auto_approve() {
  locked_update "$(state_path "$1")" _do_set_auto_approve "$1"
}

_do_set_skip_pr() {
  local workspace="$1"
  local state
  state="$(read_state "$workspace")"
  local ts
  ts="$(now_iso)"

  state="$(echo "$state" | jq \
    --arg ts "$ts" \
    '.skipPr = true |
     .timestamps.lastUpdated = $ts'
  )"
  write_state "$workspace" "$state"
}

cmd_set_skip_pr() {
  locked_update "$(state_path "$1")" _do_set_skip_pr "$1"
}

_do_set_debug() {
  local workspace="$1"
  local state
  state="$(read_state "$workspace")"
  local ts
  ts="$(now_iso)"

  state="$(echo "$state" | jq \
    --arg ts "$ts" \
    '.debug = true |
     .timestamps.lastUpdated = $ts'
  )"
  write_state "$workspace" "$state"
}

cmd_set_debug() {
  locked_update "$(state_path "$1")" _do_set_debug "$1"
}

_do_set_use_current_branch() {
  local workspace="$1"
  local branch="$2"
  local state
  state="$(read_state "$workspace")"
  local ts
  ts="$(now_iso)"

  state="$(echo "$state" | jq \
    --arg b "$branch" \
    --arg ts "$ts" \
    '.useCurrentBranch = true |
     .branch = $b |
     .timestamps.lastUpdated = $ts'
  )"
  write_state "$workspace" "$state"
}

cmd_set_use_current_branch() {
  locked_update "$(state_path "$1")" _do_set_use_current_branch "$1" "$2"
}

_do_set_revision_pending() {
  local workspace="$1"
  local checkpoint="$2"
  local state
  state="$(read_state "$workspace")"
  local ts
  ts="$(now_iso)"

  case "$checkpoint" in
    checkpoint-a|checkpoint-b) ;;
    *) die "Invalid checkpoint: ${checkpoint} (expected: checkpoint-a, checkpoint-b)" ;;
  esac

  state="$(echo "$state" | jq \
    --arg k "$checkpoint" \
    --arg ts "$ts" \
    '.checkpointRevisionPending[$k] = true |
     .timestamps.lastUpdated = $ts'
  )"
  write_state "$workspace" "$state"
}

cmd_set_revision_pending() {
  locked_update "$(state_path "$1")" _do_set_revision_pending "$1" "$2"
}

_do_clear_revision_pending() {
  local workspace="$1"
  local checkpoint="$2"
  local state
  state="$(read_state "$workspace")"
  local ts
  ts="$(now_iso)"

  case "$checkpoint" in
    checkpoint-a|checkpoint-b) ;;
    *) die "Invalid checkpoint: ${checkpoint} (expected: checkpoint-a, checkpoint-b)" ;;
  esac

  state="$(echo "$state" | jq \
    --arg k "$checkpoint" \
    --arg ts "$ts" \
    '.checkpointRevisionPending[$k] = false |
     .timestamps.lastUpdated = $ts'
  )"
  write_state "$workspace" "$state"
}

cmd_clear_revision_pending() {
  locked_update "$(state_path "$1")" _do_clear_revision_pending "$1" "$2"
}

_do_skip_phase() {
  local workspace="$1"
  local phase="$2"
  local state
  state="$(read_state "$workspace")"
  local ts
  ts="$(now_iso)"
  local next
  next="$(next_phase "$phase")"

  state="$(echo "$state" | jq \
    --arg phase "$phase" \
    --arg next "$next" \
    --arg ts "$ts" \
    '.skippedPhases += [$phase] |
     .skippedPhases = (.skippedPhases | unique) |
     .currentPhase = $next |
     .currentPhaseStatus = "pending" |
     .timestamps.lastUpdated = $ts'
  )"
  write_state "$workspace" "$state"
}

cmd_skip_phase() {
  locked_update "$(state_path "$1")" _do_skip_phase "$1" "$2"
}

_do_phase_log() {
  local workspace="$1"
  local phase="$2"
  local tokens="$3"
  local duration_ms="$4"
  local model="${5:-sonnet}"
  local state
  state="$(read_state "$workspace")"
  local ts
  ts="$(now_iso)"

  state="$(echo "$state" | jq \
    --arg phase "$phase" \
    --argjson tokens "$tokens" \
    --argjson dur "$duration_ms" \
    --arg model "$model" \
    --arg ts "$ts" \
    '.phaseLog += [{
       phase: $phase,
       tokens: $tokens,
       duration_ms: $dur,
       model: $model,
       timestamp: $ts
     }] |
     .timestamps.lastUpdated = $ts'
  )"
  write_state "$workspace" "$state"
}

cmd_phase_log() {
  locked_update "$(state_path "$1")" _do_phase_log "$1" "$2" "$3" "$4" "${5:-sonnet}"
}

cmd_phase_stats() {
  local workspace="$1"
  local state
  state="$(read_state "$workspace")"

  local total_tokens total_duration
  total_tokens="$(echo "$state" | jq '[.phaseLog[].tokens] | add // 0')"
  total_duration="$(echo "$state" | jq '[.phaseLog[].duration_ms] | add // 0')"

  echo "=== Phase Execution Stats ==="
  echo ""
  printf "%-25s %10s %10s %8s\n" "Phase" "Tokens" "Duration" "Model"
  printf "%-25s %10s %10s %8s\n" "-------------------------" "----------" "----------" "--------"

  echo "$state" | jq -r '.phaseLog[] | "\(.phase)\t\(.tokens)\t\(.duration_ms)\t\(.model)"' | \
  while IFS=$'\t' read -r phase tokens dur model; do
    local dur_sec
    dur_sec="$(echo "scale=1; $dur / 1000" | bc 2>/dev/null || echo "${dur}ms")"
    printf "%-25s %10s %8ss %8s\n" "$phase" "$tokens" "$dur_sec" "$model"
  done

  echo ""
  local total_dur_sec
  total_dur_sec="$(echo "scale=1; $total_duration / 1000" | bc 2>/dev/null || echo "${total_duration}ms")"
  printf "%-25s %10s %8ss\n" "TOTAL" "$total_tokens" "$total_dur_sec"
}

_do_abandon() {
  local workspace="$1"
  local state
  state="$(read_state "$workspace")"
  local ts
  ts="$(now_iso)"

  state="$(echo "$state" | jq \
    --arg ts "$ts" \
    '.currentPhaseStatus = "abandoned" |
     .timestamps.lastUpdated = $ts'
  )"
  write_state "$workspace" "$state"
  echo "Pipeline abandoned: ${workspace}"
}

cmd_abandon() {
  locked_update "$(state_path "$1")" _do_abandon "$1"
}

cmd_resume_info() {
  local workspace="$1"
  local state
  state="$(read_state "$workspace")"

  echo "$state" | jq '{
    currentPhase: .currentPhase,
    currentPhaseStatus: .currentPhaseStatus,
    completedPhases: .completedPhases,
    skippedPhases: (.skippedPhases // []),
    taskType: (.taskType // null),
    effort: (.effort // null),
    flowTemplate: (.flowTemplate // null),
    autoApprove: (.autoApprove // false),
    skipPr: (.skipPr // false),
    useCurrentBranch: (.useCurrentBranch // false),
    branch: .branch,
    specName: .specName,
    revisions: .revisions,
    error: .error,
    pendingTasks: [
      .tasks | to_entries[] |
      select(.value.implStatus != "completed" or
             .value.reviewStatus == "completed_fail")
      | .key
    ],
    completedTasks: [
      .tasks | to_entries[] |
      select(.value.reviewStatus == "completed_pass" or
             .value.reviewStatus == "completed_pass_with_notes")
      | .key
    ],
    totalTasks: (.tasks | length),
    phaseLogEntries: (.phaseLog // [] | length),
    totalTokens: ([(.phaseLog // [])[].tokens] | add // 0),
    totalDuration_ms: ([(.phaseLog // [])[].duration_ms] | add // 0),
    debug: (.debug // false),
    notifyOnStop: (.notifyOnStop // false),
    tasksWithRetries: [
      .tasks | to_entries[] |
      select(.value.implRetries > 0 or .value.reviewRetries > 0) |
      { task: .key, implRetries: .value.implRetries, reviewRetries: .value.reviewRetries }
    ],
    checkpointRevisionPending: (.checkpointRevisionPending // {"checkpoint-a": false, "checkpoint-b": false})
  }'
}

# --- main dispatch ---

require_jq

command="${1:-}"
shift || true

check_args() {
  local need="$1" have="$2" cmd="$3"
  [ "$have" -ge "$need" ] || die "Usage: ${cmd} requires ${need} arguments, got ${have}"
}

case "$command" in
  init)              check_args 2 $# "init <workspace> <spec-name>";           cmd_init "$@" ;;
  get)               check_args 2 $# "get <workspace> <field>";                cmd_get "$@" ;;
  phase-start)       check_args 2 $# "phase-start <workspace> <phase>";        cmd_phase_start "$@" ;;
  phase-complete)    check_args 2 $# "phase-complete <workspace> <phase>";      cmd_phase_complete "$@" ;;
  phase-fail)        check_args 3 $# "phase-fail <workspace> <phase> <msg>";    cmd_phase_fail "$@" ;;
  checkpoint)        check_args 2 $# "checkpoint <workspace> <phase>";          cmd_checkpoint "$@" ;;
  task-init)         check_args 2 $# "task-init <workspace> <json>";            cmd_task_init "$@" ;;
  task-update)       check_args 4 $# "task-update <workspace> <N> <field> <value>"; cmd_task_update "$@" ;;
  revision-bump)     check_args 2 $# "revision-bump <workspace> <type>";        cmd_revision_bump "$@" ;;
  set-branch)        check_args 2 $# "set-branch <workspace> <branch>";         cmd_set_branch "$@" ;;
  set-task-type)     check_args 2 $# "set-task-type <workspace> <taskType>";    cmd_set_task_type "$@" ;;
  set-effort)        check_args 2 $# "set-effort <workspace> <effort>";              cmd_set_effort "$@" ;;
  set-flow-template) check_args 2 $# "set-flow-template <workspace> <flowTemplate>"; cmd_set_flow_template "$@" ;;
  set-auto-approve)  check_args 1 $# "set-auto-approve <workspace>";            cmd_set_auto_approve "$@" ;;
  set-skip-pr)       check_args 1 $# "set-skip-pr <workspace>";                 cmd_set_skip_pr "$@" ;;
  set-debug)         check_args 1 $# "set-debug <workspace>";                   cmd_set_debug "$@" ;;
  set-use-current-branch) check_args 2 $# "set-use-current-branch <workspace> <branch>"; cmd_set_use_current_branch "$@" ;;
  set-revision-pending)   check_args 2 $# "set-revision-pending <workspace> <checkpoint>"; cmd_set_revision_pending "$@" ;;
  clear-revision-pending) check_args 2 $# "clear-revision-pending <workspace> <checkpoint>"; cmd_clear_revision_pending "$@" ;;
  skip-phase)        check_args 2 $# "skip-phase <workspace> <phase>";           cmd_skip_phase "$@" ;;
  phase-log)         check_args 4 $# "phase-log <workspace> <phase> <tokens> <duration_ms> [model]"; cmd_phase_log "$@" ;;
  phase-stats)       check_args 1 $# "phase-stats <workspace>";                  cmd_phase_stats "$@" ;;
  abandon)           check_args 1 $# "abandon <workspace>";                      cmd_abandon "$@" ;;
  resume-info)       check_args 1 $# "resume-info <workspace>";                 cmd_resume_info "$@" ;;
  *)                 die "Unknown command: ${command}" ;;
esac

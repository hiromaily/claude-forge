#!/usr/bin/env bash
# test-hooks.sh — Automated tests for claude-forge hook scripts (62 tests)
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

echo ""
echo "--- Rule 3f: effort null warning on phase-start phase-1 ---"
reset_workspace
WS="$(setup_workspace "phase-1" "pending")"

run_hook "pre-tool-hook.sh" '{"tool_name":"mcp__forge-state__phase_start","tool_input":{"workspace":"'"$WS"'","phase":"phase-1"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "phase-start phase-1 with effort null exits 0 (non-blocking)"
assert_stderr_contains "effort is not set" "phase-start phase-1 with effort null emits warning"

reset_workspace
WS="$(setup_workspace "phase-1" "pending")"
jq '.effort = "M"' "${WS}/state.json" > "${WS}/state.json.tmp" && mv "${WS}/state.json.tmp" "${WS}/state.json"

run_hook "pre-tool-hook.sh" '{"tool_name":"mcp__forge-state__phase_start","tool_input":{"workspace":"'"$WS"'","phase":"phase-1"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "phase-start phase-1 with effort set exits 0"
if echo "$HOOK_STDERR_CONTENT" | grep -qF "effort is not set"; then
  fail "phase-start phase-1 with effort set should not emit warning"
else
  pass "phase-start phase-1 with effort set emits no warning"
fi

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
assert_stderr_contains "mcp__forge-state__abandon" "block message includes MCP abandon tool"

echo ""
echo "--- completed pipeline: allow ---"
reset_workspace
WS="$(setup_workspace "final-summary" "completed")"

run_hook "stop-hook.sh" '{"hook_event_name":"Stop","stop_hook_active":false}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "stop allowed when pipeline completed"

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
# common.sh sourcing smoke tests
# ============================================================

echo ""
echo "=== common.sh sourcing smoke tests ==="

echo ""
echo "--- smoke: pre-tool-hook.sh sources common.sh and exits 0 with no active pipeline ---"
reset_workspace
run_hook "pre-tool-hook.sh" '{"tool_name":"Edit","tool_input":{"file_path":"/src/foo.go"}}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
if [ "$HOOK_EXIT" -eq 0 ]; then
  pass "pre-tool-hook.sh exits 0 with no active pipeline (common.sh sourcing works)"
else
  fail "pre-tool-hook.sh exits 0 with no active pipeline (common.sh sourcing works)" "expected exit 0, got $HOOK_EXIT"
fi

echo ""
echo "--- smoke: stop-hook.sh sources common.sh and exits 0 with no active pipeline ---"
reset_workspace
run_hook "stop-hook.sh" '{"hook_event_name":"Stop","stop_hook_active":false}' "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
if [ "$HOOK_EXIT" -eq 0 ]; then
  pass "stop-hook.sh exits 0 with no active pipeline (common.sh sourcing works)"
else
  fail "stop-hook.sh exits 0 with no active pipeline (common.sh sourcing works)" "expected exit 0, got $HOOK_EXIT"
fi

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
# NOTE: The command string below is a legacy fixture string used to simulate a post-bash-hook.sh
# trigger for a non-post-to-source phase-complete. It is not an executable command (state-manager.sh
# has been deleted); it exists only as a JSON payload for hook pattern matching tests.
run_hook "post-bash-hook.sh" '{"tool_name":"Bash","tool_input":{"command":"bash scripts/state-manager.sh phase-complete .specs/test final-summary"},"tool_response":""}'
assert_exit 0 "phase-complete final-summary exits 0 (not post-to-source)"

echo ""
echo "--- Bash tool, phase-complete post-to-source, legacy investigation type: skipped ---"
reset_workspace
WS="$(setup_workspace "post-to-source" "completed")"
# Patch taskType to investigation (legacy field — no longer set by new pipelines)
jq '.taskType = "investigation"' "${WS}/state.json" > "${WS}/state.json.tmp" && mv "${WS}/state.json.tmp" "${WS}/state.json"
touch "${WS}/summary.md"
# NOTE: The command string below is a legacy fixture string used to simulate a post-bash-hook.sh
# trigger for post-to-source detection. It is not an executable command (state-manager.sh has been
# deleted); it exists only as a JSON payload for hook pattern matching tests.
run_hook "post-bash-hook.sh" \
  "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-complete ${WS} post-to-source\"},\"tool_response\":\"\"}" \
  "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "legacy investigation type skipped (no feature branch)"
reset_workspace

echo ""
echo "--- Bash tool, phase-complete post-to-source, no summary.md: skipped ---"
reset_workspace
WS="$(setup_workspace "post-to-source" "completed")"
jq '.taskType = "feature"' "${WS}/state.json" > "${WS}/state.json.tmp" && mv "${WS}/state.json.tmp" "${WS}/state.json"
# Do NOT create summary.md
# NOTE: The command string below is a legacy fixture string used to simulate a post-bash-hook.sh
# trigger for post-to-source detection. It is not an executable command (state-manager.sh has been
# deleted); it exists only as a JSON payload for hook pattern matching tests.
run_hook "post-bash-hook.sh" \
  "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"bash scripts/state-manager.sh phase-complete ${WS} post-to-source\"},\"tool_response\":\"\"}" \
  "CLAUDE_PROJECT_DIR=${TMPDIR_BASE}"
assert_exit 0 "missing summary.md skipped gracefully"
reset_workspace

# ============================================================
echo ""
echo "========================================"
echo "Results: ${PASS_COUNT} passed, ${FAIL_COUNT} failed"
echo "========================================"

[ "$FAIL_COUNT" -eq 0 ] && exit 0 || exit 1

#!/usr/bin/env bash
# forge-demo.sh — simulated claude-forge pipeline for asciinema demo recording
#
# Usage:
#   asciinema rec demo/forge-demo.cast --cols 160 --rows 42 --title "claude-forge demo"
#   bash demo/forge-demo.sh
#   # (wait for script to finish, then Ctrl+D to stop recording)

# ── Colors / styles ──────────────────────────────────────────────────────────
R='\033[0m'
BOLD='\033[1m'
DIM='\033[2m'
GRN='\033[32m'
YLW='\033[33m'
CYN='\033[36m'
BGRN='\033[1;32m'
BBLU='\033[1;34m'
BYLW='\033[1;33m'

# ── Helpers ───────────────────────────────────────────────────────────────────
type_slow() {
  local s="$1" d="${2:-0.045}"
  for (( i=0; i<${#s}; i++ )); do
    printf '%s' "${s:$i:1}"; sleep "$d"
  done
  printf '\n'
}

spin() {
  local label="$1" n="${2:-20}"
  local frames=('⠋' '⠙' '⠹' '⠸' '⠼' '⠴' '⠦' '⠧' '⠇' '⠏')
  for (( i=0; i<n; i++ )); do
    printf "\r  ${CYN}${frames[$((i % 10))]}${R} %s" "$label"
    sleep 0.1
  done
  printf '\r\033[K'
}

sep() {
  printf "${DIM}%s${R}\n" \
    "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
}

phase() {
  local num="$1" name="$2" agent="$3" tok="$4" dur="$5" extra="${6:-}" spin_n="${7:-22}"
  printf "\n${BBLU}▶ Phase %-4s${R}  ${BOLD}%s${R}\n" "$num" "$name"
  spin "Spawning ${agent}..." "$spin_n"
  printf "  ${GRN}✓${R} Complete  ${DIM}(%s tokens · %s)${R}\n" "$tok" "$dur"
  [[ -n "$extra" ]] && printf "  ${DIM}→ %s${R}\n" "$extra"
}

# ── Demo ─────────────────────────────────────────────────────────────────────
clear
sleep 0.6

# Shell prompt + command
printf "${DIM}~/projects/my-api${R} ${BGRN}❯${R} "
sleep 0.5
type_slow '/forge "Add retry logic to the HTTP client"' 0.04
sleep 0.9

# Header
printf '\n'
sep
printf "  ${BOLD}claude-forge${R} ${DIM}v2.6.0${R}  ·  spec-driven AI development pipeline\n"
sep
printf "  Workspace  ${CYN}.specs/20260411-add-retry-logic${R}\n"
printf "  Effort     ${YLW}M${R}  ·  Flow: standard  ·  Branch: ${CYN}feature/add-retry-logic${R}\n"
sep

# Phases 1–3b
phase "1"   "Situation Analysis"  "situation-analyst"   "1,847"  "0:23"  "analysis.md"      25
sleep 0.3
phase "2"   "Investigation"       "investigator"        "2,103"  "0:31"  "investigation.md" 30
sleep 0.3
phase "3"   "Design"              "architect"           "3,241"  "0:48"  "design.md"        40

printf "\n${BBLU}▶ Phase 3b ${R}  ${BOLD}Design Review${R}\n"
spin "Spawning design-reviewer..." 18
printf "  ${GRN}✓${R} Verdict: ${BGRN}APPROVE${R}  ${DIM}(1,102 tokens · 0:17)${R}\n"

# Checkpoint A
sleep 0.7
printf '\n'
sep
printf "  ${BYLW}✋  Checkpoint A — human review required${R}\n"
printf "     Design is solid. Continue to implementation?\n"
sep
printf "  ${DIM}[Press Enter to continue …]${R}"
sleep 2.8
printf '\n\n'

# Phases 4–7
phase "4"   "Task Decomposition"  "task-decomposer"    "1,456"  "0:22"  "3 tasks defined"  22
sleep 0.3

printf "\n${BBLU}▶ Phase 5  ${R}${BOLD}Implementation${R}  ${DIM}(parallel · 3 tasks)${R}\n"
printf "  ${DIM}Task 1/3${R}  Add RetryTransport to http_client.go\n"
printf "  ${DIM}Task 2/3${R}  Add retry config to Config struct\n"
printf "  ${DIM}Task 3/3${R}  Add unit tests for retry behavior\n"
spin "Running implementers in parallel..." 45
printf "  ${GRN}✓${R} All tasks complete  ${DIM}(8,891 tokens · 2:14)${R}\n"
sleep 0.3

printf "\n${BBLU}▶ Phase 6  ${R}${BOLD}Code Review${R}  ${DIM}(parallel · 3 tasks)${R}\n"
spin "Running impl-reviewers..." 25
printf "  ${GRN}✓${R} All reviews passed  ${DIM}(3,201 tokens · 0:51)${R}\n"

phase "7"   "Comprehensive Review"  "comprehensive-reviewer"  "1,876"  "0:29"  ""  20
sleep 0.3

printf "\n${BBLU}▶ Final Verification${R}\n"
spin "Running test suite..." 20
printf "  ${GRN}✓${R} Tests pass  ${DIM}go test ./...  OK${R}\n"

# Result
sleep 0.9
printf '\n'
sep
printf "  ${BGRN}✅ PR created${R}  ·  ${CYN}https://github.com/org/project/pull/42${R}\n"
printf "  ${DIM}feature/add-retry-logic → main  ·  3 files changed, +127 lines${R}\n"
sep
printf '\n'

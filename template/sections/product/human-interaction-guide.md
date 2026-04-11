# Human Interaction Points

The pipeline pauses and returns control to the user at specific points. Points marked **blocking** require a response before the pipeline can continue.

## Input Validation

| # | Trigger | What the user sees | Blocking |
|---|---------|-------------------|---------|
| 1 | `validate_input` returns `valid: false` | Error messages; pipeline stops | Yes — aborts |
| 2 | LLM judges input as gibberish | Rejection message with examples | Yes — aborts |
| 3 | Jira URL but plugin unavailable | Error with install instructions | Yes — aborts |

## Workspace Setup

| # | Trigger | What the user sees | Blocking |
|---|---------|-------------------|---------|
| 4 | Current branch is not `main`/`master` | Branch name; choice to use or create new | Yes |
| 5 | Every run — effort level selection | Task type + effort level selection (S/M/L) | Yes |

## Checkpoint A — Design Review

| # | Trigger | What the user sees | Blocking |
|---|---------|-------------------|---------|
| 6 | Auto-approve conditions met | One-line notice | No |
| 7 | Human approval required | Design summary with AI verdict; sound notification | Yes — **STOP AND WAIT** |

## Checkpoint B — Tasks Review

| # | Trigger | What the user sees | Blocking |
|---|---------|-------------------|---------|
| 8 | Auto-approve conditions met | One-line notice | No |
| 9 | Human approval required | Task overview with AI verdict; sound notification | Yes — **STOP AND WAIT** |

## Implementation (Phase 5–6)

| # | Trigger | What the user sees | Blocking |
|---|---------|-------------------|---------|
| 10 | Task retry limit (2) exhausted | Failure report | Yes |
| 11 | Subagent returns empty/incoherent output | Failure recorded | Yes — stalls |
| 12 | Test suite fails after implementation | Failure output | Yes — stalls |

## Final Verification

| # | Trigger | What the user sees | Blocking |
|---|---------|-------------------|---------|
| 13 | Verifier finds unfixable failures | Failure report | Yes — stalls |

## Pipeline End

| # | Trigger | What the user sees | Blocking |
|---|---------|-------------------|---------|
| 14 | `summary.md` written | Full summary with stats; sound notification | No |

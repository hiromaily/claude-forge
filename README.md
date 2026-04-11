<!--
⚠️ AUTO-GENERATED FILE — DO NOT EDIT
Source: template/pages/README.tpl.md · Run `make docs` to regenerate.
-->
# claude-forge

**From `/forge` to PR — automated.**

[![Claude Code Plugin](https://img.shields.io/badge/Claude_Code-plugin-blueviolet?logo=anthropic&logoColor=white)](https://docs.anthropic.com/en/docs/claude-code)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**[📖 Documentation](https://hiromaily.github.io/claude-forge/)** | **[📖 ドキュメント (日本語)](https://hiromaily.github.io/claude-forge/ja/)**

```text
/forge "Add retry logic to the API client"
    │
    ├─ Phase 1-2  Situation Analysis + Investigation
    ├─ Phase 3    Design  ──→  Design Review (APPROVE/REVISE)
    ├─ ✋ Checkpoint A — human approval
    ├─ Phase 4    Task Decomposition  ──→  Tasks Review
    ├─ Phase 5-6  Implement + Code Review  (parallel per task)
    ├─ Phase 7    Comprehensive Review
    └─ ✅  Final Verification  →  PR created  →  Summary posted
```

---

Spec-Driven Development got you most of the way there.

You write the spec. AI does the implementation. You review. It works — until you realize you're still managing every handoff manually. You kick off analysis, wait for output, hand off context to the next prompt, watch for mistakes, review intermediate work, decide when to proceed — on every task, on every run.

**The bottleneck is no longer prompting. It's orchestration.**

I built **claude-forge** to automate that layer.

It's a Claude Code plugin that replaces ad-hoc AI development workflows with a structured, multi-phase pipeline — isolated subagents, deterministic guardrails, and state that survives restarts.

Instead of writing better prompts, you build a system where AI development can run predictably.

---

> **Documentation** is managed as a Single Source of Truth using [docs-ssot](https://github.com/hiromaily/docs-ssot). Files such as `README.md`, `CLAUDE.md`, and `ARCHITECTURE.md` are auto-generated — edit the source files under `template/` and run `make docs` to regenerate.

## Installation

For the complete step-by-step guide, see [SETUP.md](./SETUP.md).

### Quick start — Plugin users (recommended)

```bash
# Step 1: Register the marketplace (one-time)
/plugin marketplace add hiromaily/claude-forge

# Step 2: Install the plugin (binary downloaded automatically)
/plugin install claude-forge
/reload-plugins

# Step 3: Restart Claude Code and verify
/mcp   # forge-state should show as Connected
```

> **Note:** `/plugin marketplace add` only registers the source — you must also run `/plugin install` to activate the plugin and trigger the binary download.

### Quick start — Local development

For contributors building from source:

```bash
# From the claude-forge directory
make setup

# Restart Claude Code and verify
/mcp   # forge-state should show as Connected
```

### Prerequisites

- **Go** — required to build the MCP server binary
- **jq** — required for state management and hook scripts. Install via `brew install jq` (macOS) or your package manager.

### Environment variables

Environment variables are configured automatically when using `make setup`. For manual setup, pass them via `claude mcp add --env`:

| Variable | Required | Description |
| --- | --- | --- |
| `FORGE_AGENTS_PATH` | Yes | Absolute path to the `agents/` directory. Required for `pipeline_next_action` to resolve agent `.md` files at runtime. Set automatically by `make setup`. |
| `FORGE_SPECS_DIR` | No | Override the default `.specs/` directory used by the engine. |
| `FORGE_EVENTS_PORT` | No | Port for the SSE events endpoint (used by `subscribe_events`). |

---

## Quick start

Invoke the skill from any Claude Code session where the plugin is installed:

```text
/forge <describe your task here>
/forge https://github.com/org/repo/issues/123
/forge https://myorg.atlassian.net/browse/PROJ-456
```

When given a GitHub Issue or Jira URL, the pipeline fetches the issue details as context and posts the final summary back as a comment. Plain text input works too — it just skips the posting step.

### Flags

| Flag | Description |
| --- | --- |
| `--effort=<effort>` | Force an effort level: `S`, `M`, `L`. Determines flow template (light/standard/full). Skips heuristic detection. Default: `M`. XS is not supported. |
| `--auto` | Skip human checkpoints when the AI reviewer verdict is APPROVE. REVISE verdicts still pause for human input. |
| `--nopr` | Skip PR creation. Changes are committed and pushed to the feature branch, but no pull request is opened. |
| `--debug` | Append a `## Debug Report` section to `summary.md` with execution flow diagnostics (token outliers, retries, revision cycles, missing phase-log entries). Note: `## Improvement Report` is always appended regardless of this flag. |
| _(auto-detected)_ | Resume an interrupted pipeline by providing the spec directory name (e.g. `/forge 20260320-fix-auth-timeout`). If the directory exists under `.specs/`, resume is auto-detected. `--resume` is accepted for backward compatibility but has no effect. |

```text
/forge --effort=S --auto Fix the null pointer crash in auth middleware
/forge --nopr Add retry logic to the API client
/forge --debug Add a new validation layer
```

### Resume an interrupted pipeline

Pass the spec directory name (the folder under `.specs/`). Resume is auto-detected:

```text
/forge 20260320-fix-auth-timeout
```

### Abandon a pipeline

Use the MCP tool from Claude Code:

```text
mcp__forge-state__abandon with workspace: .specs/20260320-fix-auth-timeout
```

Or delete the state file manually:

```bash
rm .specs/20260320-fix-auth-timeout/state.json
```

---

## The problem with SDD today

The AI development landscape has evolved through three phases:

**1. Vibe coding** — "Write me a function that does X." Works for small tasks. Breaks as complexity grows. The model loses focus, context fills up, nothing is reproducible.

**2. Spec-Driven Development (SDD)** — Write a spec first, then hand it to AI. Better. But you're still the orchestrator. You manage each handoff, watch for quality regressions, decide when to move on. It's an improvement — but it's still manual.

**3. Pipeline automation** — You describe a task once; the system runs the full workflow, enforces constraints, reviews its own output, and self-reports on where it got stuck.

Anthropic's own research puts it plainly: ["Measuring Agent Autonomy in Practice"](https://www.anthropic.com/research/measuring-agent-autonomy) found a significant _deployment overhang_ — models can handle far more autonomy than humans actually grant them. The bottleneck isn't model intelligence. It's how humans structure workflows around the models.

claude-forge is built for phase 3.

---

## Four things that make it different

### 1. SDD is still manual — claude-forge isn't

SDD tells you *what* to do at each phase. It doesn't *run* the phases. You still decide when to move from analysis to design, when to approve, when to iterate.

claude-forge automates the full handoff chain. Each phase writes a markdown artifact. The next phase reads it. No context sharing, no conversation history — just structured files as the API between agents.

### 2. Improvement loop — automatic, not optional

Most teams measure AI output by the artifact: did it ship? But the real cost is invisible.

AI spent 40% of its tokens re-reading docs it couldn't find quickly. Context had to be re-established multiple times because agents shared a session. You never see this. You just see a PR.

After every run, claude-forge emits an **Improvement Report** — appended to `summary.md` — identifying exactly where the pipeline got stuck:

- Documentation gaps that slowed agents down
- Missing conventions that caused repeated clarification loops
- Token-heavy phases caused by poorly structured context

Most teams de-prioritize this under deadline pressure. claude-forge makes it automatic on every run.

To act on it, feed the report back into a new pipeline:

```text
/forge Review and implement the improvement suggestions in .specs/{date}-{name}/summary.md
```

This turns every completed run into a compounding investment — the codebase progressively gets easier for both humans and future AI runs.

### 3. Flow optimization — effort-aware scaling

Not every task needs 11 phases and 3 review cycles.

claude-forge selects the pipeline template based on effort level (S / M / L) — from a lean light pipeline to a full 11-phase run with mandatory human checkpoints.

A small task doesn't go through task review. A large one doesn't skip it. The workflow adapts to the effort, not the other way around.

### 4. Deterministic guardrails — hooks, not just prompts

LLM instructions are probabilistic. A well-prompted agent *usually* follows them. But "usually" isn't enough when the cost of a mistake is high.

claude-forge enforces critical constraints at the shell level via Claude Code hooks:

- **Read-only guard** — blocks source edits during analysis phases (exit 2)
- **Commit guard** — prevents git commits during parallel task execution
- **Checkpoint gate** — blocks progression until required artifacts exist and human approval is recorded

These aren't instructions the agent can misinterpret. They're hard stops.

---

## Overview

| Dimension | SDD / Single-conversation | claude-forge |
| --- | --- | --- |
| **Context management** | One growing conversation; quality degrades as context fills | Each phase runs in an isolated subagent with a clean context window |
| **State persistence** | Lost on session restart or context compaction | Disk-based `state.json` — resume anytime, survives compaction |
| **Constraint enforcement** | Prompt instructions only (probabilistic) | Two-layer: prompt instructions + deterministic hook scripts |
| **Adaptability** | One-size-fits-all workflow | 3 effort levels (S/M/L) → 3 flow templates (light/standard/full) |
| **Quality gates** | Manual review at the end | Built-in AI review loops (APPROVE/REVISE) + human checkpoints |
| **Concurrency** | Sequential only | Parallel task implementation with atomic locking |
| **Observability** | None | Per-phase token count, duration, and model tracking |
| **Reproducibility** | Depends on conversation history | All artifacts written to `.specs/` — fully auditable |
| **Integration** | Standalone | GitHub Issues, Jira, automatic PR creation, issue commenting |
| **Testing** | Framework itself is untested | Comprehensive automated test suite — run `bash scripts/test-hooks.sh` for count |

---

## Pipeline flow

```mermaid
flowchart TD
    START(["▶ /forge &lt;input&gt;"])

    %% ── Initialisation ──
    START --> PARSE["🛡️ Input parsing &<br>resume detection"]
    PARSE --> ISRESUME{Resume?}
    ISRESUME -->|yes| RESUME(("Resume at<br>current phase"))
    ISRESUME -->|no| VALID{Input valid?}
    VALID -->|no| REJECT(["❌ Reject — show error"])
    VALID -->|yes| DETECT["🔍 Effort auto-detection<br>& branch state check"]
    DETECT --> ASK{{"👤 Confirm all at once:<br>1. Effort S / M / L<br>2. Branch: new or current<br>3. Workspace slug"}}
    ASK --> INIT["📂 Workspace init<br>state.json + request.md"]
    INIT --> BRANCH["🌿 Create feature branch"]

    %% ── Analysis & Design ──
    BRANCH --> P1
    RESUME -.-> P1
    P1["🔍 Phase 1 — Situation Analysis<br><i>situation-analyst → analysis.md</i>"]
    P1 --> P2
    P2["🔍 Phase 2 — Investigation<br><i>investigator → investigation.md</i>"]
    P2 --> P3

    P3["📐 Phase 3 — Design<br><i>architect → design.md</i>"]
    P3 --> P3R
    P3R["🔎 Phase 3b — Design Review<br><i>design-reviewer → review-design.md</i>"]
    P3R --> DREV{APPROVE?}
    DREV -->|REVISE| P3
    DREV -->|APPROVE| CPA

    CPA{{"👤🔊 Checkpoint A<br>Human reviews design"}}
    CPA -->|approved| P4
    CPA -->|rejected| P3

    %% ── Task Planning ──
    P4["📋 Phase 4 — Task Decomposition<br><i>task-decomposer → tasks.md</i>"]
    P4 --> P4R
    P4R["🔎 Phase 4b — Tasks Review<br><i>task-reviewer → review-tasks.md</i>"]
    P4R --> TREV{APPROVE?}
    TREV -->|REVISE| P4
    TREV -->|APPROVE| CPB

    CPB{{"👤🔊 Checkpoint B<br>Human reviews tasks"}}
    CPB -->|approved| P5
    CPB -->|rejected| P4

    %% ── Implementation ──
    subgraph loop ["🔄 Repeat for each task"]
        P5["⚙️ Phase 5 — Implementation<br><i>implementer → impl-N.md</i>"]
        P5 --> P6
        P6["🔎 Phase 6 — Code Review<br><i>impl-reviewer → review-N.md</i>"]
        P6 --> RESULT{PASS?}
        RESULT -->|"FAIL (≤2 retries)"| P5
    end
    RESULT -->|all PASS| P7

    %% ── Finalisation ──
    P7["🔬 Phase 7 — Comprehensive Review<br><i>comprehensive-reviewer → comprehensive-review.md</i>"]
    P7 --> FV

    FV["✅ Final Verification<br><i>verifier → final-verification.md</i>"]
    FV --> PR["🚀 PR Creation<br>git push · gh pr create"]
    PR --> FS["📝 Final Summary<br><i>verifier → summary.md<br>(includes PR # + Improvement Report)</i>"]
    FS --> FC["🔒 Final Commit<br>amend + force-push<br>(summary.md → PR branch)"]
    FC --> POST{"Source type?"}
    POST -->|GitHub Issue| GH["💬 Post to GitHub Issue"]
    POST -->|Jira Issue| JIRA["💬 Post to Jira Issue"]
    POST -->|Plain text| DONE(["✔🔊 Done"])
    GH --> DONE
    JIRA --> DONE
```

> **Effort level** determines which phases are skipped: Phase 4b and Checkpoint B are skipped for S and M; Phase 7 is additionally skipped for S. See [Effort Levels](#effort-levels-and-skipped-phases) for details.
>
> **Branch creation** happens immediately after workspace init — before any analysis phase begins. The branch name is derived from the workspace slug confirmed by the user.

---

## Pipeline Phase Table

| Phase | Task                      | Agent                  | Input Artifact              | Output Artifact                 | Human Interaction |
| ----- | ------------------------- | ---------------------- | --------------------------- | ------------------------------- | ----------------- |
| 0     | Input Validation          | validate-input + LLM   | User input                  | validation result               | No                |
| 1     | Workspace Setup           | orchestrator           | validated input             | request.md, state.json          | Yes               |
| 2     | Detect Effort Level       | orchestrator           | request.md                  | effort in state.json            | Yes               |
| 3     | Situation Analysis        | situation-analyst      | request.md                  | analysis.md                     | No                |
| 4     | Investigation             | investigator           | analysis.md                 | investigation.md                | No                |
| 5     | Design                    | architect              | investigation.md            | design.md                       | No                |
| 6     | Design Review             | design-reviewer        | design.md                   | review-design.md                | No                |
| 7     | Checkpoint A              | human                  | design.md, review-design.md | approval / revision             | Yes               |
| 8     | Task Decomposition        | task-decomposer        | design.md                   | tasks.md                        | No                |
| 9     | Tasks Review              | task-reviewer          | tasks.md                    | review-tasks.md                 | No                |
| 10    | Checkpoint B              | human                  | tasks.md, review-tasks.md   | approval / revision             | Yes               |
| 11    | Implementation            | implementer            | task spec                   | impl-N.md                       | No                |
| 12    | Code Review               | impl-reviewer          | impl-N.md                   | review-N.md                     | No                |
| 13    | Comprehensive Review      | comprehensive-reviewer | all impl + reviews          | comprehensive-review.md         | No                |
| 14    | Final Verification        | verifier               | comprehensive-review.md     | verification result             | No                |
| 15    | PR Creation               | orchestrator           | commits                     | PR (PR # confirmed)             | No                |
| 16    | Final Summary             | orchestrator           | all artifacts + PR #        | summary.md (includes PR #)      | No                |
| 17    | Final Commit              | orchestrator           | summary.md, state.json      | amend last commit + force-push  | No                |
| 18    | Post to Issue             | orchestrator           | summary.md                  | issue comment                   | No                |
| 19    | Done                      | system                 | summary.md                  | —                               | No                |

---

## Pipeline Phase Execution by Effort Level

Which phases run is primarily determined by effort level. ✅ = phase runs; blank = skipped.

| Phase | Task | Effort S (`light`) | Effort M (`standard`) | Effort L (`full`) |
| ----- | ------------------------- | --------- | -------- | ------------ |
| 0 | Input Validation | ✅ | ✅ | ✅ |
| 1 | Workspace Setup | ✅ | ✅ | ✅ |
| 2 | Detect Effort | ✅ | ✅ | ✅ |
| 3 | Situation Analysis | ✅ | ✅ | ✅ |
| 4 | Investigation | * | ✅ | ✅ |
| 5 | Design | ✅ | ✅ | ✅ |
| 6 | Design Review | ✅ | ✅ | ✅ |
| 7 | Checkpoint A | ✅ | ✅ | ✅ |
| 8 | Task Decomposition | | ✅ | ✅ |
| 9 | Tasks Review | | | ✅ |
| 10 | Checkpoint B | | | ✅ |
| 11 | Implementation | ✅ | ✅ | ✅ |
| 12 | Code Review | ✅ | ✅ | ✅ |
| 13 | Comprehensive Review | | ✅ | ✅ |
| 14 | Final Verification | ✅ | ✅ | ✅ |
| 15 | PR Creation | ✅ | ✅ | ✅ |
| 16 | Final Summary | ✅ | ✅ | ✅ |
| 17 | Final Commit | ✅ | ✅ | ✅ |
| 18 | Post to Source | ✅ | ✅ | ✅ |
| 19 | Done | ✅ | ✅ | ✅ |

> XS effort is not supported; use S for small tasks.
> For effort S, Phase 4 (Investigation) is merged into Phase 3 (Situation Analysis) as a single combined pass. Phase 8 (Task Decomposition) is skipped; a single implementation task is synthesized from the design document instead.
> Checkpoint A is always blocking when design ran. Checkpoint B runs only for effort L. Use `--auto` to allow AI reviewer verdict to auto-approve Checkpoint A.

---

## Human interaction points

The pipeline pauses and returns control to the user at the following points. Points marked **blocking** require a response before the pipeline can continue; points marked **informational** present output with no further input needed.

### Input Validation

| # | Trigger | What the user sees | Blocking |
|---|---------|-------------------|---------|
| 1 | `mcp__forge-state__validate_input` returns an error (empty, too short, malformed URL) | Error messages; pipeline stops | Yes — pipeline aborts |
| 2 | LLM judges input as gibberish or unrelated to software development | Rejection message with specific reason and valid-input examples; pipeline stops | Yes — pipeline aborts |
| 3 | Jira URL provided but `mcp__atlassian__getJiraIssue` tool unavailable | Error with plugin install instructions; pipeline stops | Yes — pipeline aborts |

### Workspace Setup

| # | Trigger | What the user sees | Blocking |
|---|---------|-------------------|---------|
| 4 | Current git branch is not `main`/`master` | Branch name shown; choice to use the current branch or create a new one | Yes — waits for choice |
| 5 | Effort level selection (always required) | User selects effort level (S / M / L) and sees which phases will execute for that choice | Yes — waits for selection |
| 6 | `full` template and `--auto` flag used together | Warning that `full` mandates manual checkpoints; asked to continue without auto-approve or abort | Yes — waits for choice |

### Checkpoint A — Design Review

| # | Trigger | What the user sees | Blocking |
|---|---------|-------------------|---------|
| 7 | Auto-approve conditions met (`--auto` + AI verdict APPROVE or APPROVE_WITH_NOTES, no CRITICAL findings) | One-line notice: "Auto-approving Checkpoint A (AI verdict: …)" | No — informational |
| 8 | Human approval required (AI returned REVISE, or no `--auto`, or `full` template) | Design summary: approach, key changes, risk level, AI verdict, any MINOR findings, workspace path. Asked to approve or give feedback. Sound notification plays. After each revision cycle the updated design is re-presented and the pipeline stops again | Yes — **STOP AND WAIT** |

### Checkpoint B — Tasks Review

| # | Trigger | What the user sees | Blocking |
|---|---------|-------------------|---------|
| 9 | Auto-approve conditions met | One-line notice: "Auto-approving Checkpoint B (AI verdict: …)" | No — informational |
| 10 | Human approval required | Task overview: task count, risk level, AI verdict, any MINOR findings, workspace path. Asked to approve or give feedback. Sound notification plays. After each revision cycle the updated task list is re-presented and the pipeline stops again | Yes — **STOP AND WAIT** |

### Implementation (Phase 5–6 loop)

| # | Trigger | What the user sees | Blocking |
|---|---------|-------------------|---------|
| 11 | A task's impl-reviewer returns FAIL and the per-task retry limit (2) is exhausted | Failure report for that task; asked how to proceed | Yes — waits for instruction |
| 12 | A subagent returns empty or incoherent output and the single retry also fails | Failure reported; `phase-fail` recorded in state | Yes — pipeline stalls until user intervenes |
| 13 | Test suite fails after implementation completes | Failure output presented; `phase-fail` recorded in state | Yes — pipeline stalls |

### Final Verification

| # | Trigger | What the user sees | Blocking |
|---|---------|-------------------|---------|
| 14 | Verifier finds failures it cannot fix | Failure report presented to user | Yes — pipeline stalls |

### Pipeline End

| # | Trigger | What the user sees | Blocking |
|---|---------|-------------------|---------|
| 15 | `summary.md` written successfully | Full contents of `summary.md` displayed (request, branch, PR, task table, improvement report, execution stats). Sound notification plays. | No — informational |

---

> **Skipped checkpoints:** Checkpoint B is skipped for effort S and M (only effort L runs Checkpoint B). Phase 4b (task reviewer) is also skipped for effort S and M. Use `--auto` to allow the AI reviewer verdict to auto-approve Checkpoint A (not available with `full` template).

---

## Feature list

- **Effort-aware scaling** — effort level (S/M/L) selects one of 3 flow templates (light/standard/full), from a lean pipeline to a full 10+ agent run with mandatory checkpoints
- **Deterministic hook guardrails** — PreToolUse hooks block source edits during analysis, block git commits during parallel execution, and block checkout to main/master during an active pipeline
- **AI review loops** — Design and task plans go through APPROVE/REVISE cycles with dedicated reviewer agents before implementation begins
- **Multi-phase pipeline** — 10 specialist agents across up to 12 phases (analysis → investigation → design → review → tasks → review → implementation → code review → comprehensive review → verification → PR → summary)
- **Parallel implementation** — Tasks marked `[parallel]` run concurrently with mkdir-based atomic locking for state updates
- **Human checkpoints** — Pause for human approval at design and task decomposition stages; skippable with `--auto` (except `full` template)
- **Improvement report** — Always-on retrospective appended to `summary.md` identifying documentation gaps, code readability friction, and AI agent support issues encountered during the run
- **Past implementation pattern injection** — Before each implementer invocation, `mcp__forge-state__search_patterns` (BM25 scorer) scans the specs index for similar past pipelines and injects their file-modification patterns into the prompt, surfacing real implementation examples rather than generic guidance
- **Disk-based state machine** — All progress tracked in `state.json` via the Go MCP server (44 MCP tools including `search_patterns`, `subscribe_events`, `ast_summary`, `ast_find_definition`, `dependency_graph`, `impact_scope`, `validate_input`, `validate_artifact`, `pipeline_init`, `pipeline_init_with_context`, `pipeline_next_action`, `pipeline_report_result`, `profile_get`, `history_search`, `history_get_patterns`, `history_get_friction_map`, `analytics_pipeline_summary`, `analytics_repo_dashboard`, and `analytics_estimate`); pipelines survive context compaction and session restarts
- **Resume and abandon** — Resume an interrupted pipeline from any phase; abandon cleanly when needed
- **Input validation** — Two-layer guard: deterministic `mcp__forge-state__validate_input` MCP tool (empty, too-short, malformed URL) + LLM semantic check blocks nonsensical or non-development requests before any tokens are spent on workspace setup
- **Phase metrics** — Every agent invocation logged with token count, duration, and model; included in the Final Summary
- **Source integration** — Accepts GitHub Issue URLs or Jira Issue URLs as input; posts the final summary back as a comment
- **Automatic PR creation** — Commits, pushes, and opens a GitHub PR with a structured summary; skippable with `--nopr`
- **Debug report** — `--debug` flag appends a `## Debug Report` to `summary.md` with execution flow diagnostics: token outliers, retry counts, revision cycles, and missing phase-log entries
- **Comprehensive test suite** — Automated tests covering state management, all hook scripts, and edge cases
- **Fail-open hooks** — Hooks never block non-pipeline work; gracefully degrade if `jq` is missing

---

## Flow templates

The effort level determines the flow template. XS effort is not supported; use S for small tasks.

| Effort | Template | Skipped phases |
| --- | --- | --- |
| **S** | `light` | Task review (4b), Checkpoint B, Comprehensive Review (7) |
| **M** | `standard` | Task review (4b), Checkpoint B |
| **L** | `full` | _(none)_ — all checkpoints mandatory, `--auto` ignored |

Effort is detected from: `--effort=` flag > Jira story points > heuristic > default `M`.

---

## Repository workflow rules (`.specs/instructions.md`)

You can commit a `.specs/instructions.md` file to your repository to enforce
deterministic workflow rules at phase-4 completion. When a task matches a
rule but is missing `mode: human_gate`, the engine automatically triggers
REVISE and re-runs task-decomposer with the violation findings.

### Quick example — claude-forge

```markdown
---
rules:
  - id: main-proto
    when:
      files_match:
        - "backend/**/*.proto"
        - "backend/gen/proto/**"
    require: human_gate
    reason: "make sure PR for main-proto repository"

  - id: destructive-migration
    when:
      files_match:
        - "backend/migrations/**/*.sql"
      title_matches: "(?i)drop\\s+(table|column)"
    require: human_gate
    reason: "Stakeholder verification is required for this destructive migration."
---
```

**Scope:** workflow rules only — not coding style, domain knowledge, or
personal preferences. Keep those in `CLAUDE.md` / `AGENTS.md` /
`.kiro/steering/`.

See [`docs/reference/workflow-instructions.md`](./docs/reference/workflow-instructions.md)
for the full schema, evaluation flow, and failure modes.

---

## How it works

The pipeline is built on three core principles:

1. **Files are the API** — Each phase writes a markdown artifact to `.specs/{date}-{name}/`. The next phase reads those files, never the conversation history. This keeps every agent's context small and focused.
2. **State on disk** — All progress is tracked in `state.json`, so pipelines survive context compaction and session restarts. Hooks read this state to enforce constraints.
3. **Two-layer compliance** — Critical invariants (read-only analysis, no parallel commits, checkpoint gates) are enforced both by agent instructions (probabilistic) and hook scripts (deterministic, fail-open).

For the full data flow, state machine, hook architecture, agent input/output matrix, and concurrency model, browse [`docs/architecture/`](./docs/architecture) directly.

---

## Directory structure

```
claude-forge/
├── CLAUDE.md              ← AI agent guide (auto-loaded by Claude Code)
├── ARCHITECTURE.md        ← index (full docs in docs/architecture/)
├── BACKLOG.md             ← known issues, improvement candidates
├── README.md              ← project overview and quick start
├── .claude-plugin/
│   └── plugin.json        ← plugin metadata (name, version)
├── .claude/
│   └── rules/
│       ├── git.md         ← Git practices enforced in this repo
│       ├── shell-script.md ← Shell scripting conventions for *.sh files
│       └── docs.md        ← Documentation rules (SSOT, bilingual, VitePress)
├── agents/                ← 10 named agent definitions (.md files)
│   ├── README.md          ← agent roster with roles
│   ├── situation-analyst.md
│   ├── investigator.md
│   ├── architect.md
│   ├── design-reviewer.md
│   ├── task-decomposer.md
│   ├── task-reviewer.md
│   ├── implementer.md
│   ├── impl-reviewer.md
│   ├── comprehensive-reviewer.md
│   └── verifier.md
├── docs/
│   ├── _partials/         ← SSOT content fragments (included by docs/)
│   └── architecture/      ← architecture documentation (13 focused files)
├── hooks/
│   └── hooks.json         ← hook definitions (PreToolUse, PostToolUse, Stop)
├── mcp-server/            ← Go MCP server source (forge-state binary)
├── scripts/
│   ├── common.sh          ← shared find_active_workspace helper
│   ├── launch-mcp.sh      ← self-healing MCP launcher
│   ├── pre-tool-hook.sh   ← read-only guard, commit blocking, checkout blocking
│   ├── post-agent-hook.sh ← agent output quality validation
│   ├── post-bash-hook.sh  ← auto-commits state.json+summary.md (v1 legacy)
│   ├── setup.sh           ← downloads forge-state-mcp binary from GitHub Releases
│   ├── stop-hook.sh       ← pipeline completion guard
│   └── test-hooks.sh      ← automated test suite (62 tests)
└── skills/
    └── forge/
        └── SKILL.md       ← orchestrator instructions (the main skill)
```

---

## Design decisions

Key choices that shape the plugin's architecture:

- **All agents use `model: sonnet`** — cost optimization for 10+ agent invocations per run. Upgrade individual agents to `opus` if needed.
- **The orchestrator never reads source code** — only small artifact files, keeping its context window lean.
- **Parallel implementation with mkdir-based locking** — macOS lacks `flock`, so atomic `mkdir` is used instead. Parallel agents skip `git commit`; the orchestrator batch-commits after the group finishes.

See [docs/architecture/technical-decisions.md](./docs/architecture/technical-decisions.md) for full rationale on these and other decisions (fail-open hooks, file-based state, agent separation).

---

## Running tests

```bash
# Hook script tests (62 tests)
cd claude-forge
bash scripts/test-hooks.sh

# Go MCP server tests
cd claude-forge/mcp-server
go test -race ./...
```

The hook test suite covers all hook scripts (`pre-tool-hook.sh`, `post-agent-hook.sh`, `stop-hook.sh`, `post-bash-hook.sh`, `common.sh`), pre-tool-hook rules (read-only, commit blocking, main/master checkout block), and edge cases like abandoned pipelines and special characters in spec names. The Go test suite covers all 26 state-management commands and MCP-only tools.

## Architecture: MCP-driven pipeline engine (v2)

claude-forge v1 used shell scripts (`state-manager.sh`) for state management and relied on SKILL.md prompt instructions for all orchestration decisions — which phase to run next, whether to retry, when to skip. The LLM was both the executor *and* the decision-maker.

v2 replaced this with a Go MCP server (`forge-state-mcp`) that owns all pipeline logic. The LLM orchestrator now follows a strict **ask → execute → report** loop: it calls `pipeline_next_action` to get the next action, executes it, and calls `pipeline_report_result` to advance state. It never decides what to do next.

### The shift: LLM as executor, not decision-maker

```
v1: User → SKILL.md (LLM decides next phase) → shell scripts (state I/O)
v2: User → SKILL.md (LLM executes actions)  → Go Engine (decides next phase) → MCP tools (state + guards)
```

In v1, if the LLM misinterpreted a skip condition or forgot to call `phase-complete`, the pipeline broke silently. In v2, the Engine returns a typed action (`spawn_agent`, `checkpoint`, `exec`, `write_file`, `done`) — the LLM cannot invent steps or skip them.

### Comparison

| Aspect | v1 (Skill + shell scripts) | v2 (MCP Engine) |
| --- | --- | --- |
| **Who decides the next phase** | LLM interprets SKILL.md instructions | `Engine.NextAction()` in Go — deterministic |
| **State management** | Shell script (`state-manager.sh`) with `jq` | Go `StateManager` with typed fields and mutex locking |
| **Guard enforcement** | Shell hooks only (exit 2) | Two-layer: Go MCP handler guards + shell hooks |
| **Phase transition reliability** | Probabilistic — LLM may skip or misordering | Deterministic — Engine enforces canonical PHASES order |
| **Retry / revision logic** | SKILL.md prose ("if REVISE, re-run Phase 3") | Engine tracks `designRevisions`, `taskRevisions`, `implRetries` with hard limits |
| **Artifact validation** | None — LLM checks file existence ad-hoc | `validation/artifact.go` — content rules per phase, blocking |
| **Parallel task dispatch** | SKILL.md instructions + hook blocking | Engine returns `parallel_task_ids`; hook blocks git commit |
| **Auto-approve logic** | SKILL.md conditional ("if --auto and APPROVE…") | Engine evaluates `autoApprove` + verdict + findings deterministically |
| **Skip-phase computation** | LLM reads effort table and calls skip-phase | `SkipsForEffort()` returns canonical skip list |
| **Resume** | LLM reads state.json and figures out where it was | `pipeline_next_action` reads state and returns the exact next step |
| **Analytics** | None | `analytics.Collector`, `Estimator`, `Reporter` — token/cost/duration tracking |
| **Cross-pipeline learning** | None | `history.HistoryIndex` (BM25), `KnowledgeBase` (patterns, friction) |
| **Repo profiling** | None | `profile.RepoProfiler` — language, CI, linter detection for prompt enrichment |
| **Tool count** | ~10 shell commands | 44 typed MCP tools |
| **Error handling** | Shell exit codes, often swallowed | Go errors with typed responses (`IsError=true`) |

### What this means in practice

**Fewer pipeline failures.** v1's most common failure mode was the LLM misinterpreting a phase transition — skipping a checkpoint, forgetting to call `phase-complete`, or miscounting retry attempts. v2 eliminates this entire class of errors because the Engine makes all transition decisions.

**Cheaper retries.** When a v1 pipeline stalled, resuming required the LLM to re-read state.json and reconstruct its understanding of where it was. v2's `pipeline_next_action` returns the exact next step — no interpretation needed.

**Richer context for agents.** v2 injects historical data (past review patterns, similar implementations, repo profile) into agent prompts via MCP tools. v1 agents worked in isolation with no cross-pipeline knowledge.

**Auditable decisions.** Every Engine decision is a deterministic function of `state.json`. You can reproduce any pipeline's control flow by replaying `NextAction()` calls against the saved state — something impossible with v1's LLM-driven decisions.

---

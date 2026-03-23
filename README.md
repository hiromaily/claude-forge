# claude-forge

Claude Code is getting good enough that you start delegating more and more development work to it.

At that point, even structured approaches like Spec-Driven Development (SDD) start to feel manual.

You don’t just want better prompts — you want the whole workflow to run itself.

And the focus shifts to things like:
- reducing token usage
- avoiding context pollution
- making outputs reproducible
- structuring information so AI can actually use it

I built **claude-forge** to automate that layer.

It’s a Claude Code plugin that turns AI development into a structured, multi-phase pipeline:

- isolated subagents per phase (no context pollution)
- disk-based state (resume anytime)
- deterministic guardrails via hooks
- review loops before implementation
- artifacts structured for both humans and AI

Instead of managing prompts and workflows manually, claude-forge manages the system around the model.

---

## Why claude-forge?

Recent research from Anthropic highlights a key insight about AI agents in real-world usage.

In the paper **[“Measuring Agent Autonomy in Practice”](https://www.anthropic.com/research/measuring-agent-autonomy)**, Anthropic analyzed millions of interactions with agentic systems and observed that current deployments often **under-utilize the autonomy that models are capable of**.

> “We observe a significant *deployment overhang*, where the autonomy models are capable of handling exceeds what they exercise in practice.”

In other words, the limiting factor for autonomous systems is increasingly **not model intelligence**, but **how humans structure workflows around the models**.

The study also shows that:

* Autonomous sessions are becoming **longer and more capable**
* Experienced users increasingly **trust agents with fewer manual approvals**
* A large portion of agent usage already occurs in **software engineering tasks**

This indicates that the main challenge is shifting from **interactive prompting** to **structured autonomous workflows** — which is exactly the gap claude-forge is designed to close.

---

## Overview

Most AI-assisted development frameworks (including [SDD Framework](https://github.com/zhimin-z/Awesome-Spec-Driven-Development)) rely on a single conversation with structured prompts. This works for small tasks but breaks down as complexity grows — the context window fills up, the model loses focus, and there is no mechanism to enforce constraints beyond “please follow these instructions.”

claude-forge takes a fundamentally different approach:

| Dimension | SDD / Single-conversation | claude-forge |
| --- | --- | --- |
| **Context management** | One growing conversation; quality degrades as context fills | Each phase runs in an isolated subagent with a clean context window |
| **State persistence** | Lost on session restart or context compaction | Disk-based `state.json` — resume anytime, survives compaction |
| **Constraint enforcement** | Prompt instructions only (probabilistic) | Two-layer: prompt instructions + deterministic hook scripts |
| **Adaptability** | One-size-fits-all workflow | 5 task types × 4 effort levels → 5 flow templates (direct/lite/light/standard/full) |
| **Quality gates** | Manual review at the end | Built-in AI review loops (APPROVE/REVISE) + human checkpoints |
| **Concurrency** | Sequential only | Parallel task implementation with atomic locking |
| **Observability** | None | Per-phase token count, duration, and model tracking |
| **Reproducibility** | Depends on conversation history | All artifacts written to `.specs/` — fully auditable |
| **Integration** | Standalone | GitHub Issues, Jira, automatic PR creation, issue commenting |
| **Testing** | Framework itself is untested | Comprehensive automated test suite — run `bash scripts/test-hooks.sh` for count |

---

## Flow

```mermaid
flowchart TD
    START(["▶ /forge"])
    START --> RC{state.json<br>exists?}
    RC -->|yes| RESUME[Load state.json<br>restore variables]
    RC -->|no| IV["🛡️ Input Validation<br>validate-input.sh + LLM check"]
    IV -->|invalid| REJECT(["❌ Reject — show error"])
    IV -->|valid| WS[Workspace Setup<br>request.md + state.json]
    RESUME --> REJOIN(("resume at<br>current phase"))
    WS --> BC{"On main<br>branch?"}
    BC -->|"yes (new branch<br>created before Phase 5)"| TE
    BC -->|no| BCASK["👤 Use current branch<br>or create new?"]
    BCASK --> TE

    TE["🔍 Detect task type & effort<br>(👤 confirm if heuristic)"]
    TE --> P1

    REJOIN -.-> P1
    P1["🔍 Phase 1 — Situation Analysis<br><i>situation-analyst</i>"]
    P1 -->|analysis.md| P2
    P2["🔍 Phase 2 — Investigation<br><i>investigator</i>"]

    P2 -->|investigation.md| P3
    P3["📐 Phase 3 — Design<br><i>architect</i>"]
    P3 -->|design.md| P3R
    P3R["🔎 Phase 3b — Design Review<br><i>design-reviewer</i>"]
    P3R -->|review-design.md| DREV{APPROVE?}
    DREV -->|REVISE| P3
    DREV -->|APPROVE| CPA

    CPA{{"👤🔊 Checkpoint A<br>Human reviews design"}}
    CPA -->|approved| P4
    CPA -->|rejected| P3

    P4["📋 Phase 4 — Task Decomposition<br><i>task-decomposer</i>"]
    P4 -->|tasks.md| P4R
    P4R["🔎 Phase 4b — Tasks Review<br><i>task-reviewer</i>"]
    P4R -->|review-tasks.md| TREV{APPROVE?}
    TREV -->|REVISE| P4
    TREV -->|APPROVE| CPB

    CPB{{"👤🔊 Checkpoint B<br>Human reviews tasks"}}
    CPB -->|approved| GITBR["🌿 Create/use<br>feature branch"]
    CPB -->|rejected| P4

    GITBR --> P5

    subgraph loop ["🔄 Repeat for each task"]
        P5["⚙️ Phase 5 — Implementation<br><i>implementer</i>"]
        P5 -->|"impl-N.md"| P6
        P6["🔎 Phase 6 — Code Review<br><i>impl-reviewer</i>"]
        P6 -->|"review-N.md"| RESULT{PASS?}
        RESULT -->|"FAIL (≤2 retries)"| P5
    end
    RESULT -->|all PASS| P7

    P7["🔬 Phase 7 — Comprehensive Review<br><i>comprehensive-reviewer</i>"]
    P7 -->|comprehensive-review.md| FV

    FV["✅ Final Verification<br><i>verifier</i>"]
    FV --> PR["🚀 PR Creation<br>commit · push · gh pr create"]
    PR --> FS["📝 Final Summary<br>summary.md + Improvement Report"]
    FS --> POST{"Source type?"}
    POST -->|GitHub Issue| GH["💬 Post to GitHub Issue"]
    POST -->|Jira Issue| JIRA["💬 Post to Jira Issue"]
    POST -->|Plain text| DONE(["✔🔊 Done"])
    GH --> DONE
    JIRA --> DONE
```

> The diagram above shows the full `feature` flow. Other task types skip phases — see [Task types](#task-types) below.

### Improvement feedback loop

Each run appends an `## Improvement Report` to `.specs/{date}-{name}/summary.md` — a retrospective on friction encountered during the pipeline: documentation gaps, unclear conventions, missing tooling, or issues that slowed down the AI agents.

To act on those suggestions, feed the report back into a new pipeline:

```text
/forge Review and implement the improvement suggestions in .specs/{date}-{name}/summary.md
```

This turns every completed run into a compounding investment — the codebase progressively gets easier for both humans and future AI runs.

---

## Feature list

- **Effort-aware scaling** — `(task_type, effort)` matrix determines one of 5 flow templates, from a 2-agent direct fix to a 10+ agent full pipeline with mandatory checkpoints
- **Task-type adaptation** — 5 task types (feature, bugfix, investigation, docs, refactor) with tailored phase skip tables
- **Deterministic hook guardrails** — PreToolUse hooks block source edits during analysis, block git commits during parallel execution, and enforce checkpoint/artifact completion
- **AI review loops** — Design and task plans go through APPROVE/REVISE cycles with dedicated reviewer agents before implementation begins
- **Multi-phase pipeline** — 11 specialist agents across up to 12 phases (analysis → investigation → design → review → tasks → review → implementation → code review → comprehensive review → verification → PR → summary)
- **Parallel implementation** — Tasks marked `[parallel]` run concurrently with mkdir-based atomic locking for state updates
- **Human checkpoints** — Pause for human approval at design and task decomposition stages; skippable with `--auto` (except `full` template)
- **Improvement report** — Always-on retrospective appended to `summary.md` identifying documentation gaps, code readability friction, and AI agent support issues encountered during the run
- **Disk-based state machine** — All progress tracked in `state.json` via a 22-command CLI; pipelines survive context compaction and session restarts
- **Resume and abandon** — Resume an interrupted pipeline from any phase; abandon cleanly when needed
- **Input validation** — Two-layer guard: deterministic `validate-input.sh` (empty, too-short, malformed URL) + LLM semantic check blocks nonsensical or non-development requests before any tokens are spent on workspace setup
- **Phase metrics** — Every agent invocation logged with token count, duration, and model; included in the Final Summary
- **Source integration** — Accepts GitHub Issue URLs or Jira Issue URLs as input; posts the final summary back as a comment
- **Automatic PR creation** — Commits, pushes, and opens a GitHub PR with a structured summary; skippable with `--nopr`
- **Sound notification** — macOS notification sound (`afplay Glass.aiff`) plays automatically when the pipeline pauses at a human checkpoint and when the pipeline completes, so you don't need to watch the terminal
- **Debug report** — `--debug` flag appends a `## Debug Report` to `summary.md` with execution flow diagnostics: token outliers, retry counts, revision cycles, and missing phase-log entries
- **Comprehensive test suite** — Automated tests covering state management, all hook scripts, and edge cases
- **Fail-open hooks** — Hooks never block non-pipeline work; gracefully degrade if `jq` is missing

---

## Installation

```
/plugin marketplace add hiromaily/claude-forge
/plugin install claude-forge
```

### Prerequisites

- **jq** — required for state management and hook scripts. Install via `brew install jq` (macOS) or your package manager.

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
| `--type=<type>` | Force a task type: `feature`, `bugfix`, `investigation`, `docs`, `refactor`. Skips heuristic detection and user confirmation. |
| `--effort=<effort>` | Force an effort level: `XS`, `S`, `M`, `L`. Determines flow template (direct/lite/light/standard/full). Skips heuristic detection. Default: `M`. |
| `--auto` | Skip human checkpoints when the AI reviewer verdict is APPROVE. REVISE verdicts still pause for human input. |
| `--nopr` | Skip PR creation. Changes are committed and pushed to the feature branch, but no pull request is opened. |
| `--debug` | Append a `## Debug Report` section to `summary.md` with execution flow diagnostics (token outliers, retries, revision cycles, missing phase-log entries). Note: `## Improvement Report` is always appended regardless of this flag. |

```text
/forge --type=bugfix --auto Fix the null pointer crash in auth middleware
/forge --nopr Add retry logic to the API client
/forge --debug Add a new validation layer
```

### Resume an interrupted pipeline

```text
/forge .specs/20260320-fix-auth-timeout
```

### Abandon a pipeline

```bash
bash claude-forge/scripts/state-manager.sh abandon .specs/20260320-fix-auth-timeout
```

---

## Task types

The pipeline adapts its execution based on the detected task type and effort level. The combination `(task_type, effort)` determines one of 5 flow templates:

| Template | Phases | When used |
| --- | --- | --- |
| **direct** | Implementation → Verification → PR | Tiny changes (bugfix/XS, docs/XS-S) |
| **lite** | Merged Analysis → Design → Tasks → Implementation → Verification → PR | Small tasks (feature/XS, bugfix/S) |
| **light** | Full analysis → Design → Implementation → Review → Verification → PR | Medium tasks without checkpoints |
| **standard** | Full pipeline (current default) | Medium-large tasks (feature/M) |
| **full** | Standard + mandatory checkpoints (ignores `--auto`) | Large tasks (feature/L) |

Task type is detected from: `--type=` flag > Jira issue type > GitHub labels > heuristic.
Effort is detected from: `--effort=` flag > Jira story points > heuristic > default `M`.

---

## How it works

The pipeline is built on three core principles:

1. **Files are the API** — Each phase writes a markdown artifact to `.specs/{date}-{name}/`. The next phase reads those files, never the conversation history. This keeps every agent's context small and focused.
2. **State on disk** — All progress is tracked in `state.json`, so pipelines survive context compaction and session restarts. Hooks read this state to enforce constraints.
3. **Two-layer compliance** — Critical invariants (read-only analysis, no parallel commits, checkpoint gates) are enforced both by agent instructions (probabilistic) and hook scripts (deterministic, fail-open).

For the full data flow, state machine, hook architecture, agent input/output matrix, and concurrency model, see [ARCHITECTURE.md](ARCHITECTURE.md).

---

## Directory structure

```text
claude-forge/
  agents/             11 specialist agents (.md files with YAML frontmatter)
  hooks/              Hook definitions (hooks.json)
  scripts/
    state-manager.sh  State management CLI (22 commands)
    pre-tool-hook.sh  Read-only, commit blocking, checkpoint & artifact guards
    post-agent-hook.sh  Agent output quality validation
    stop-hook.sh      Pipeline completion guard
    test-hooks.sh     Automated test suite (run to see current count)
  skills/
    forge/
      SKILL.md        Orchestrator instructions (the main skill)
  ARCHITECTURE.md     Design decisions and data flow diagrams
  BACKLOG.md          Known issues and improvement candidates
  CLAUDE.md           Guide for AI agents modifying this plugin
```

---

## Design decisions

Key choices that shape the plugin's architecture:

- **All agents use `model: sonnet`** — cost optimization for 10+ agent invocations per run. Upgrade individual agents to `opus` if needed.
- **The orchestrator never reads source code** — only small artifact files, keeping its context window lean.
- **Parallel implementation with mkdir-based locking** — macOS lacks `flock`, so atomic `mkdir` is used instead. Parallel agents skip `git commit`; the orchestrator batch-commits after the group finishes.

See [ARCHITECTURE.md](ARCHITECTURE.md) for full rationale on these and other decisions (fail-open hooks, file-based state, agent separation).

---

## Running tests

```bash
cd claude-forge
bash scripts/test-hooks.sh
```

This runs automated tests covering `state-manager.sh` (including `set-effort`, `set-flow-template`, `set-debug`), all three hook scripts, checkpoint guards, artifact guards, effort-null guard (Rule 3f), and edge cases like abandoned pipelines and special characters in spec names.

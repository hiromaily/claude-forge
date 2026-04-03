# Pipeline Flow

## Overview Diagram

```mermaid
flowchart TD
    START(["forge start"])
    START --> PI["pipeline_init"]
    PI --> RESUME{resume_mode<br>= auto?}
    RESUME -->|yes| RI["resume_info"]
    RESUME -->|no| ERR{errors?}
    ERR -->|yes| REJECT(["Reject"])
    ERR -->|no| FETCH{fetch_needed?}
    FETCH -->|yes| EXT["Fetch external context<br>GitHub / Jira"]
    FETCH -->|no| PIC1["pipeline_init_with_context"]
    EXT --> PIC1
    PIC1 --> CONFIRM["User confirms effort + slug"]
    CONFIRM --> PIC2["pipeline_init_with_context<br>with user_confirmation"]
    PIC2 --> SETUP["setup phase:<br>init, write request.md,<br>set effort/template, task_init"]
    RI --> LOOP

    SETUP --> LOOP

    LOOP["pipeline_next_action"]
    LOOP --> TYPE{action.type}

    TYPE -->|spawn_agent| AGENT["Execute Agent"]
    TYPE -->|checkpoint| CP["checkpoint + present to user"]
    TYPE -->|exec| EXEC["Execute command"]
    TYPE -->|write_file| WF["Write file"]
    TYPE -->|done: skip| SKIP["phase_complete for skipped phase"]
    TYPE -->|done| DONE(["Complete"])

    AGENT --> RPT["pipeline_report_result"]
    EXEC --> RPT
    WF --> RPT
    CP --> PC["phase_complete"]
    SKIP --> LOOP

    RPT --> HINT{next_action_hint}
    HINT -->|revision_required| USER["Present findings to user"]
    HINT -->|setup_continue| LOOP
    HINT -->|normal| LOOP
    PC --> LOOP
    USER --> LOOP
```

## Phase Table

18 phases in execution order. Phases may be skipped based on effort level (flow template).

| # | Phase ID | Description | Actor | Artifact |
|---|----------|-------------|-------|----------|
| 1 | `setup` | Init workspace, write request.md, detect effort, set template | Orchestrator | request.md, state.json |
| 2 | `phase-1` | Situation Analysis — read-only codebase mapping | situation-analyst | analysis.md |
| 3 | `phase-2` | Investigation — deep-dive research, edge cases | investigator | investigation.md |
| 4 | `phase-3` | Design — architecture and approach | architect | design.md |
| 5 | `phase-3b` | Design Review — AI quality gate | design-reviewer | review-design.md |
| 6 | `checkpoint-a` | Human review of design | User | approval / revision |
| 7 | `phase-4` | Task Decomposition — numbered task list | task-decomposer | tasks.md |
| 8 | `phase-4b` | Tasks Review — AI quality gate | task-reviewer | review-tasks.md |
| 9 | `checkpoint-b` | Human review of tasks | User | approval / revision |
| 10 | `phase-5` | Implementation — TDD per task (sequential or parallel) | implementer | impl-N.md |
| 11 | `phase-6` | Code Review — per task, up to 2 retries | impl-reviewer | review-N.md |
| 12 | `phase-7` | Comprehensive Review — cross-cutting concerns | comprehensive-reviewer | comprehensive-review.md |
| 13 | `final-verification` | Full build + test suite verification | verifier | final-verification.md |
| 14 | `pr-creation` | Create PR via `gh pr create` | Orchestrator | PR URL |
| 15 | `final-summary` | Generate summary.md with PR number | Orchestrator | summary.md |
| 16 | `final-commit` | Amend last commit with summary.md + force-push | Orchestrator | — |
| 17 | `post-to-source` | Post summary to GitHub/Jira issue | Orchestrator | issue comment |
| 18 | `completed` | Pipeline done | — | — |

## Effort Levels and Skipped Phases

| Effort | Flow Template | Skipped Phases |
|--------|---------------|----------------|
| S | light | phase-4b (Tasks Review), checkpoint-b (Tasks Checkpoint), phase-7 (Comprehensive Review) |
| M | standard | phase-4b (Tasks Review), checkpoint-b (Tasks Checkpoint) |
| L | full | _(none)_ |

## Sequence Diagram — Orchestrator / MCP Server Interaction

```mermaid
sequenceDiagram
    actor User
    participant Orch as Orchestrator<br>(SKILL.md)
    participant MCP as MCP Server<br>(forge-state)
    participant Agent as Subagent
    participant FS as .specs/

    Note over User,FS: Step 1 — Initialize or Resume

    User->>Orch: /forge <args>
    Orch->>MCP: pipeline_init(arguments)
    MCP-->>Orch: PipelineInitResult

    alt resume_mode = "auto"
        Orch->>MCP: resume_info(workspace)
        MCP-->>Orch: ResumeInfoResult
        Note over Orch: Skip to Step 2
    else new pipeline
        opt fetch_needed (GitHub/Jira)
            Orch->>Orch: Fetch external context
        end
        Orch->>MCP: pipeline_init_with_context(workspace, flags)
        MCP-->>Orch: needs_user_confirmation
        Orch->>User: Present effort options
        User-->>Orch: Confirm effort + slug
        Orch->>MCP: pipeline_init_with_context(+ user_confirmation)
        MCP->>FS: create state.json, request.md
        MCP-->>Orch: confirmed workspace
    end

    Note over User,FS: Step 2 — Main Loop

    loop until done
        Orch->>MCP: pipeline_next_action(workspace)
        MCP-->>Orch: Action{type, phase, prompt, ...}

        alt type = spawn_agent
            Orch->>Agent: Agent(prompt)
            Agent->>FS: write artifact
            Agent-->>Orch: result
            Orch->>MCP: pipeline_report_result(phase, tokens, duration)
            MCP->>FS: update state.json, validate artifact
            MCP-->>Orch: next_action_hint

        else type = checkpoint
            Orch->>MCP: checkpoint(workspace, phase)
            MCP->>FS: set status = awaiting_human
            Orch->>User: Present for review
            User-->>Orch: Approved / Feedback
            Orch->>MCP: phase_complete(phase)

        else type = exec
            Orch->>Orch: Execute command (git, task_init, etc.)
            Orch->>MCP: pipeline_report_result(phase, tokens, duration)
            MCP-->>Orch: next_action_hint

        else type = done (skip)
            Orch->>MCP: phase_complete(skipped phase)

        else type = done
            Note over Orch: Pipeline complete
        end
    end
```

## Revision Loop Detail

Design (phase-3/3b) and Tasks (phase-4/4b) phases support revision loops
when the AI reviewer returns a REVISE verdict. Maximum 2 revisions per loop.

```mermaid
sequenceDiagram
    participant Orch as Orchestrator
    participant MCP as MCP Server
    participant A as Architect / Decomposer
    participant R as Design / Tasks Reviewer

    Orch->>MCP: pipeline_next_action
    MCP-->>Orch: spawn_agent (architect)
    Orch->>A: Design task
    A-->>Orch: design.md
    Orch->>MCP: pipeline_report_result

    Orch->>MCP: pipeline_next_action
    MCP-->>Orch: spawn_agent (design-reviewer)
    Orch->>R: Review design.md
    R-->>Orch: review-design.md (REVISE)
    Orch->>MCP: pipeline_report_result
    MCP-->>Orch: next_action_hint = revision_required

    Note over Orch: Present findings, bump revision counter
    Orch->>MCP: pipeline_next_action
    MCP-->>Orch: spawn_agent (architect, revision 2)
    Note over Orch: Loop repeats (max 2 revisions)
```

## Implementation Loop Detail

Each task goes through implementation (phase-5) and code review (phase-6).
Failed reviews retry up to 2 times.

```mermaid
sequenceDiagram
    participant Orch as Orchestrator
    participant MCP as MCP Server
    participant Impl as Implementer
    participant Rev as Impl Reviewer

    loop for each task
        Orch->>MCP: pipeline_next_action
        MCP-->>Orch: spawn_agent (implementer, task N)
        Orch->>Impl: Implement task N
        Impl-->>Orch: impl-N.md
        Orch->>MCP: pipeline_report_result

        Orch->>MCP: pipeline_next_action
        MCP-->>Orch: spawn_agent (impl-reviewer, task N)
        Orch->>Rev: Review impl-N.md
        Rev-->>Orch: review-N.md

        alt PASS / PASS_WITH_NOTES
            Orch->>MCP: pipeline_report_result
            Note over Orch: Next task
        else FAIL (retries < 2)
            Orch->>MCP: pipeline_report_result
            Note over Orch: Retry task N
        end
    end
```

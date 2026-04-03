# Pipeline Flow

## Overview Diagram

```mermaid
flowchart TD
    START(["▶ /forge"])
    START --> RC{state.json<br>exists?}
    RC -->|yes| RESUME[Load state.json<br>restore variables]
    RC -->|no| IV["🛡️ Input Validation"]
    IV -->|invalid| REJECT(["❌ Reject"])
    IV -->|valid| WS[Workspace Setup]
    RESUME --> REJOIN(("resume"))
    WS --> TE["🔍 Detect task type & effort"]
    TE --> P1

    REJOIN -.-> P1
    P1["Phase 1 — Situation Analysis"]
    P1 -->|analysis.md| P2
    P2["Phase 2 — Investigation"]

    P2 -->|investigation.md| P3
    P3["Phase 3 — Design"]
    P3 -->|design.md| P3R
    P3R["Phase 3b — Design Review"]
    P3R -->|review-design.md| DREV{APPROVE?}
    DREV -->|REVISE| P3
    DREV -->|APPROVE| CPA

    CPA{{"👤 Checkpoint A"}}
    CPA -->|approved| P4
    CPA -->|rejected| P3

    P4["Phase 4 — Task Decomposition"]
    P4 -->|tasks.md| P4R
    P4R["Phase 4b — Tasks Review"]
    P4R -->|review-tasks.md| TREV{APPROVE?}
    TREV -->|REVISE| P4
    TREV -->|APPROVE| CPB

    CPB{{"👤 Checkpoint B"}}
    CPB -->|approved| GITBR["Create feature branch"]
    CPB -->|rejected| P4

    GITBR --> P5

    subgraph loop ["🔄 Per task"]
        P5["Phase 5 — Implementation"]
        P5 -->|impl-N.md| P6
        P6["Phase 6 — Code Review"]
        P6 -->|review-N.md| RESULT{PASS?}
        RESULT -->|"FAIL (≤2 retries)"| P5
    end
    RESULT -->|all PASS| P7

    P7["Phase 7 — Comprehensive Review"]
    P7 --> FV["Final Verification"]
    FV --> PR["PR Creation"]
    PR --> FS["Final Summary<br>(includes PR #)"]
    FS --> FC["Final Commit<br>amend + force-push"]
    FC --> DONE(["✔ Done"])
```

## Phase Table

| Phase | Task | Agent | Input | Output | Human |
| ----- | ---- | ----- | ----- | ------ | ----- |
| 0 | Input Validation | validate-input + LLM | User input | validation result | No |
| 1 | Workspace Setup | orchestrator | validated input | request.md, state.json | Yes |
| 2 | Detect Task Type & Effort | orchestrator | request.md | state.json | Yes |
| 3 | Situation Analysis | situation-analyst | request.md | analysis.md | No |
| 4 | Investigation | investigator | analysis.md | investigation.md | No |
| 5 | Design | architect | investigation.md | design.md | No |
| 6 | Design Review | design-reviewer | design.md | review-design.md | No |
| 7 | Checkpoint A | human | design.md | approval / revision | Yes |
| 8 | Task Decomposition | task-decomposer | design.md | tasks.md | No |
| 9 | Tasks Review | task-reviewer | tasks.md | review-tasks.md | No |
| 10 | Checkpoint B | human | tasks.md | approval / revision | Yes |
| 11 | Implementation | implementer | task spec | impl-N.md | No |
| 12 | Code Review | impl-reviewer | impl-N.md | review-N.md | No |
| 13 | Comprehensive Review | comprehensive-reviewer | all impl + reviews | comprehensive-review.md | No |
| 14 | Final Verification | verifier | comprehensive-review.md | verification result | No |
| 15 | PR Creation | orchestrator | commits | PR (PR # confirmed) | No |
| 16 | Final Summary | orchestrator | all artifacts + PR # | summary.md (includes PR #) | No |
| 17 | Final Commit | orchestrator | summary.md, state.json | amend last commit + force-push | No |
| 18 | Post to Issue | orchestrator | summary.md | issue comment | No |
| 19 | Done | system | summary.md | — | No |

## Sequence Diagram

```mermaid
sequenceDiagram
    actor User
    participant Orch as Orchestrator
    participant SM as MCP Server
    participant Hook as Hooks
    participant FS as Workspace (.specs/)
    participant Agent as Subagents

    User->>Orch: /forge <args>
    Orch->>Orch: validate_input + LLM check

    Orch->>SM: init {workspace}
    SM->>FS: create state.json
    Orch->>FS: write request.md

    rect rgb(230, 245, 255)
    Note over Orch,Agent: Phase 1-2 — Analysis (read-only)
    Orch->>SM: phase-start phase-1
    Orch->>Agent: situation-analyst
    Note over Hook: Blocks Edit/Write
    Agent-->>Orch: analysis output
    Orch->>FS: write analysis.md
    Orch->>SM: phase-complete phase-1
    end

    rect rgb(255, 245, 230)
    Note over Orch,Agent: Phase 3/3b — Design + Review Loop
    loop max 2 revisions
        Orch->>Agent: architect → design.md
        Orch->>Agent: design-reviewer → review-design.md
        break APPROVE
        end
    end
    end

    rect rgb(255, 230, 230)
    Note over Orch,User: Checkpoint A — Human Review
    Orch->>User: Present design
    User-->>Orch: Approved / Feedback
    end

    rect rgb(230, 255, 230)
    Note over Orch,Agent: Phase 5/6 — Implementation per task
    loop for each task
        Orch->>Agent: implementer → impl-N.md
        Orch->>Agent: impl-reviewer → review-N.md
    end
    end

    Orch->>Agent: comprehensive-reviewer
    Orch->>Agent: verifier
    Orch->>Orch: git push + gh pr create
    Note over Orch: PR # is now known
    Orch->>FS: write summary.md (includes PR #)
    Orch->>Orch: git commit --amend --no-edit
    Orch->>Orch: git push --force-with-lease
    Note over Orch: PR branch now includes summary.md
    Orch->>User: Done
```

## Task Types

Different task types skip certain phases:

| Task Type | Description | Skipped Phases |
| --- | --- | --- |
| `feature` | New capability or behavior | _(none — full pipeline)_ |
| `bugfix` | Bug fix with known reproduction | Design Review (3b), Task Decomposition (4), Tasks Review (4b), Comprehensive Review (7) |
| `refactor` | Code restructuring without behavior change | Design Review (3b), Comprehensive Review uses different criteria |
| `docs` | Documentation-only changes | Investigation (2), Design (3), Design Review (3b), Task Decomposition (4), Tasks Review (4b) |
| `investigation` | Analysis-only — no code changes | All implementation phases (5-7, 14-15) — produces analysis only |

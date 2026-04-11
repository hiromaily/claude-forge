# Data Flow

> **Note:** Shows the full linear flow for effort `L` (`full` template). Lower effort levels (S, M) skip labelled phases — see the [Effort-driven Flow](effort-flow.md) section for the skip tables.

```
$ARGUMENTS
    │
    ▼
┌──────────────────┐
│ Input Validation  │ mcp__forge-state__validate_input (deterministic)
│                   │ + LLM coherence check (semantic)
└──────┬───────────┘
       │ invalid → stop with error
       ▼
┌──────────────────┐
│ Workspace Setup   │ → request.md, state.json
│ (detects effort,  │   (also sets effort/flowTemplate and calls
│  sets flow        │    skip-phase for each skipped phase upfront)
│  template)        │
└──────┬───────────┘
       │
       ▼
┌──────────────────┐
│ Phase 1           │ situation-analyst → analysis.md
│ Phase 2           │ investigator → investigation.md
└──────┬───────────┘
       │
       ▼
┌──────────────────────────────────────────────────┐
│ Phase 3 ←→ Phase 3b (APPROVE/REVISE loop)         │
│ architect → design.md                              │
│ design-reviewer → review-design.md                 │
└──────┬───────────────────────────────────────────┘
       │ Checkpoint A (human approval)
       ▼
┌──────────────────────────────────────────────────┐
│ Phase 4 ←→ Phase 4b (APPROVE/REVISE loop)         │
│ task-decomposer → tasks.md                         │
│ task-reviewer → review-tasks.md                    │
└──────┬───────────────────────────────────────────┘
       │ [phase-4b, checkpoint-b skipped for effort S and M]
       │ Checkpoint B (human approval; effort L only)
       ▼
┌──────────────────────────────────────────────────┐
│ Phase 5-6 (per task, parallel where safe)          │
│ implementer → code files + impl-{N}.md             │
│ impl-reviewer → review-{N}.md                      │
│ (FAIL → retry, max 2 attempts)                     │
└──────┬───────────────────────────────────────────┘
       ▼
┌──────────────────────────────────────────────────┐
│ Phase 7 — Comprehensive Review                     │
│ comprehensive-reviewer → comprehensive-review.md   │
└──────┬───────────────────────────────────────────┘
       │ [phase-7 skipped for effort S]
       ▼
┌──────────────────┐
│ Final Verification│ verifier (typecheck + test suite)
└──────┬───────────┘
       │
       ▼
┌──────────────────┐
│ PR Creation       │ git push + gh pr create → PR #
└──────┬───────────┘
       │
       ▼
┌──────────────────┐
│ Final Summary     │ → summary.md (includes PR #, Improvement Report)
└──────┬───────────┘
       │
       ▼
┌──────────────────┐
│ Post to Source    │ → GitHub/Jira comment (if applicable)
└──────┬───────────┘
       │
       ▼
┌──────────────────┐
│ Final Commit      │ pipeline_report_result → state.json = "completed"
│                   │ git add summary.md state.json
│                   │ git commit --amend --no-edit
│                   │ git push --force-with-lease
│                   │ (PR branch includes summary.md + state.json in final state)
└──────────────────┘
```

## What Each Agent Reads

The information flow is strictly forward — no agent reads output from a later phase.

| Agent | Reads from workspace |
|-------|---------------------|
| situation-analyst | request.md |
| investigator | request.md, analysis.md |
| architect | request.md, analysis.md, investigation.md (+review-design.md on revision) |
| design-reviewer | request.md, analysis.md, investigation.md, design.md |
| Checkpoint A (orchestrator) | design.md, review-design.md (to present summary to human) |
| task-decomposer | request.md, design.md, investigation.md (+review-tasks.md on revision) |
| task-reviewer | request.md, design.md, investigation.md, tasks.md |
| Checkpoint B (orchestrator) | tasks.md, review-tasks.md (to present summary to human) |
| implementer | request.md, design.md, tasks.md, review-{dep}.md (+review-{N}.md on retry) — plus `## Similar Past Implementations` block injected by orchestrator via `mcp__forge-state__search_patterns` (BM25) |
| impl-reviewer | request.md, tasks.md, design.md, impl-{N}.md, git diff (file-scoped, main...HEAD) |
| comprehensive-reviewer | request.md, design.md, tasks.md, all impl-{N}.md, all review-{N}.md, git diff + selective structural reads |
| verifier | (reads code on feature branch directly) |
| PR Creation (orchestrator) | request.md, design.md, tasks.md (for PR title and body) |
| Final Summary (orchestrator) | reads analysis.md and investigation.md (where present) for the Improvement Report epilogue; fixed input file list regardless of effort level |
| Post to Source (orchestrator) | summary.md, request.md (source metadata for comment target) |

## File-Writing Responsibility

- **Phases 1–4b, 6**: Agent returns output string → orchestrator writes the file
- **Phase 5**: Agent writes code files and impl-{N}.md directly (filesystem interaction required)
- **Phase 7**: Agent writes code fixes directly and returns comprehensive-review.md content
- **Final Verification**: Agent fixes issues directly, no artifact file
- **PR Creation**: Orchestrator handles directly (git push + gh pr create)
- **Final Summary**: Orchestrator writes summary.md (includes PR # obtained from PR Creation)
- **Post to Source**: Orchestrator handles directly (post comment to GitHub/Jira)
- **Final Commit**: Orchestrator calls `pipeline_report_result` first (advances state.json to "completed"), then amends last commit to include summary.md + state.json, then force-pushes (PR branch now includes summary.md + state.json in final state)

## Specs Index System

The specs index provides cross-pipeline learning — surfacing patterns from past runs to guide current agents.

**Components:**

| Component | Role |
|--------|------|
| `indexer.BuildSpecsIndex` | Go function in `mcp-server/indexer/specs_index.go`. Scans all workspace subdirectories within `.specs/` and writes `.specs/index.json`. Extracts `requestSummary`, `reviewFeedback` (from `review-*.md` REVISE verdicts), `implOutcomes`, `implPatterns` (from `impl-*.md` file-modification sections), and `outcome`. Invoked by `mcp__forge-state__refresh_index` after each completed pipeline. |
| `mcp__forge-state__search_patterns` | **Primary scoring path.** BM25 scorer exposed as an MCP tool. Reads `.specs/index.json` and `{workspace}/request.md`, scores past entries using BM25 (IDF-weighted term frequency with length normalisation; `k1=1.5`, `b=0.75`), and emits formatted markdown. Supports two modes: **review-feedback** (default) emits a `## Past Review Feedback` block; **impl** mode emits a `## Similar Past Implementations` block. MCP-only — no shell fallback exists. |

**Data flow:**

```
Completed pipeline
  └─► mcp__forge-state__refresh_index
        └─► indexer.BuildSpecsIndex → .specs/index.json

Next pipeline, Phase 3:
  orchestrator → mcp__forge-state__search_patterns(workspace, top_k=3, mode="review-feedback")
    → injects "## Past Review Feedback" into architect prompt

Next pipeline, Phase 4:
  orchestrator → mcp__forge-state__search_patterns(workspace, top_k=3, mode="review-feedback")
    → injects "## Past Review Feedback" into task-decomposer prompt

Next pipeline, Phase 5 (before each task):
  orchestrator → mcp__forge-state__search_patterns(workspace, top_k=2, mode="impl")
    → injects "## Similar Past Implementations" into implementer prompt
```

This system is append-only and read-only from the agents' perspective. Agents never write to the index; they only consume it via the orchestrator.
